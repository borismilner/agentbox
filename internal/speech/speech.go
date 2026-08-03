// Package speech says a line out loud, so a notification can carry its meaning
// to somebody who is not looking at the screen.
//
// The engine contract is deliberately narrow, and piper satisfies it as shipped:
// a process that reads one line of UTF-8 text per utterance on stdin and writes
// raw little-endian 16-bit PCM on stdout. agentbox holds that process open and pipes
// it into a system player, and holding it open is the entire reason this package
// exists rather than a shell-out per sentence. Measured with piper and
// en_US-lessac-high on this machine: loading the voice costs ~2.5s, and every
// utterance after that costs ~70ms. Three sentences through one process cost the
// same as one. Spawning per notification would make every notification arrive
// three seconds after the thing it is about.
//
// The rules the earcons follow apply here too (ADR-0006): no cgo, no audio device
// held open by the daemon, a subprocess player resolved at startup. The one thing
// speech adds is that a voice model is ~100MB resident, so the whole pipeline is
// released after a spell with nothing to say and rebuilt on demand.
//
// What is spoken is always something an agent wrote on purpose. agentbox never reads
// a title or a body aloud on its own: an item speaks only if it carries a Speak
// line, which keeps "meaningful" the agent's job rather than a heuristic here.
package speech

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/borismilner/agentbox/internal/logging"
)

// players in preference order, the same three the earcons use (ADR-0006):
// PipeWire native, Pulse compat, bare ALSA. All three read raw PCM on stdin,
// which is verified in speech_test.go against the flags below.
var players = []string{"pw-play", "paplay", "aplay"}

var ErrNoPlayer = errors.New("no audio player found (tried pw-play, paplay, aplay)")

// ErrNoEngine means speech is on but there is nothing to synthesise with. It is
// not fatal anywhere: agentbox without a voice still chimes.
var ErrNoEngine = errors.New("no speech engine found (set [speech] command, or install piper and a voice)")

// resamplerQuality is pw-play's resampler setting, pinned to its maximum. See
// playerArgs for why it is not left at the default and not a knob.
const resamplerQuality = 15

// queueDepth is how many lines may be waiting. Past this the oldest is dropped:
// twenty notifications arriving at once should not become a two-minute monologue
// that is still reading out the first one when you come back.
const queueDepth = 8

// Options is the resolved [speech] configuration.
type Options struct {
	Enabled bool
	// Argv is the engine and its arguments. Empty asks Detect to find one.
	Argv     []string
	Rate     int     // sample rate of the PCM the engine emits
	Channels int     // 1 for every piper voice
	Volume   float64 // 0..1, shared with the earcons
	MaxChars int     // a spoken line is a sentence, not a document
	Idle     time.Duration
	Prewarm  bool // pay the model load at daemon start rather than at the first line
}

// Speaker owns the engine. One goroutine owns the pipeline and drains the queue,
// so there is only ever one voice talking and utterances come out in order.
type Speaker struct {
	log  *slog.Logger
	look func(string) (string, error)

	mu     sync.Mutex
	opt    Options
	player string
	quiet  func(time.Time) bool
	before func()
	closed bool

	lines chan queued
	done  chan struct{}
	// cut interrupts the line being heard right now. Capacity 1 and cleared
	// before every utterance, so a stale signal cannot silence the next line.
	// Signalling only ends the WAIT; what actually stops the sound is run
	// releasing the pipeline, which kills the engine and the player.
	cut chan struct{}
}

// queued is one line waiting for the voice, plus - for SpeakWait - somebody
// waiting for it to be over. done is nil for the ordinary fire-and-forget Speak;
// when it is set it is closed exactly once whatever becomes of the line: spoken,
// dropped from a full queue, or lost with an engine that would not start. A
// waiter left hanging is worse than a line never said.
type queued struct {
	line string
	done chan struct{}
}

// release lets a waiter go. Closing a channel twice panics, so this is the one
// place it happens and every path out of run leads through exactly one call.
func (u queued) release() {
	if u.done != nil {
		close(u.done)
	}
}

// New resolves a player and starts the queue goroutine. It does not start an
// engine: nothing is spawned until there is something to say (or Configure is
// given Prewarm). A missing player disables speech but is not fatal.
func New(log *slog.Logger, look func(string) (string, error)) *Speaker {
	s := &Speaker{
		log:   log,
		look:  look,
		lines: make(chan queued, queueDepth),
		done:  make(chan struct{}),
		cut:   make(chan struct{}, 1),
	}
	for _, candidate := range players {
		if path, err := look(candidate); err == nil {
			s.player = path
			break
		}
	}
	if s.player == "" {
		log.Error(logging.EvSpeechFailed, "component", "speech", "err", ErrNoPlayer.Error())
	}
	go s.run()
	return s
}

// Configure applies the [speech] knobs and is safe to call live, the way
// sound.Player.Configure is. quiet, when non-nil, silences speech during quiet
// hours - all of it, including urgent: an earcon at 3am is a chirp, but a sentence
// read aloud in a dark bedroom is a different kind of event.
func (s *Speaker) Configure(opt Options, quiet func(time.Time) bool) {
	s.mu.Lock()
	if s.player == "" {
		opt.Enabled = false // no player binary trumps config, as with sound
	}
	if opt.Enabled && len(opt.Argv) == 0 {
		argv, rate, err := s.detect()
		switch {
		case err != nil:
			s.log.Error(logging.EvSpeechFailed, "component", "speech", "err", err.Error())
			opt.Enabled = false
		default:
			opt.Argv = argv
			if opt.Rate == 0 {
				opt.Rate = rate
			}
			s.log.Info(logging.EvSpeechStarted, "component", "speech", "detected",
				strings.Join(argv, " "), "rate", opt.Rate)
		}
	}
	prewarm := opt.Prewarm && opt.Enabled && !s.opt.Enabled
	s.opt = opt
	s.quiet = quiet
	s.mu.Unlock()

	if prewarm {
		// An empty line is the warm-up: the engine loads its model and synthesises
		// nothing, so the first real sentence is instant.
		s.enqueue(queued{})
	}
}

// Before registers something to wait on before each utterance. The daemon passes
// sound.Player.Wait, which is what makes speech follow the earcon instead of
// talking over it - the level is still carried by the chime, and the sentence
// arrives behind it.
func (s *Speaker) Before(wait func()) {
	s.mu.Lock()
	s.before = wait
	s.mu.Unlock()
}

// Speak says a line. It never blocks the caller: the work happens on the queue
// goroutine, and a line that arrives with the queue full displaces the oldest one
// waiting. An empty line, speech turned off, or quiet hours all mean silence.
func (s *Speaker) Speak(text string) {
	if line, ok := s.prepare(text); ok {
		s.enqueue(queued{line: line})
	}
}

// SpeakWait says a line and hands back a channel that closes when the audio has
// finished coming out of the speakers. Same queue, same voice; the only
// difference is that the caller can wait for it, which is what a narrated
// sequence needs - the next line starts on the last word of this one rather than
// after a guess at how long it took to read.
//
// The channel is never nil, so `<-s.SpeakWait(line)` is always correct. Silence -
// speech off, quiet hours, nothing left to say - closes it at once.
func (s *Speaker) SpeakWait(text string) <-chan struct{} {
	done := make(chan struct{})
	if line, ok := s.prepare(text); ok {
		s.enqueue(queued{line: line, done: done})
		return done
	}
	close(done)
	return done
}

// ReadWait speaks one passage of a reading and hands back a channel that closes
// when it has been heard. It is SpeakWait with the length cap lifted.
//
// The cap belongs to notifications: an agent that decides a line is worth hearing
// gets one sentence, and Clean truncates past it. A passage the human asked to
// hear is different - it is already whole sentences chosen by Passages, and
// truncating it would silently drop the end of a paragraph. Nothing else differs:
// same queue, same voice, same engine, so a passage sounds exactly as it would
// have if the whole text had been handed over at once.
func (s *Speaker) ReadWait(text string) <-chan struct{} {
	done := make(chan struct{})
	if line, ok := s.prepareLong(text); ok {
		s.enqueue(queued{line: line, done: done})
		return done
	}
	close(done)
	return done
}

// prepare applies everything that means silence - speech off, quiet hours,
// nothing left after cleaning - and yields the one line an engine can read.
func (s *Speaker) prepare(text string) (string, bool) {
	s.mu.Lock()
	enabled, maxChars, quiet := s.opt.Enabled, s.opt.MaxChars, s.quiet
	s.mu.Unlock()
	if !enabled {
		return "", false
	}
	if quiet != nil && quiet(time.Now()) {
		s.log.Debug(logging.EvSpeechSpoke, "component", "speech", "skipped", "quiet_hours")
		return "", false
	}
	line := Clean(text, maxChars)
	return line, line != ""
}

// prepareLong is prepare without the length cap: the same silences apply, only
// the truncation does not. Kept beside prepare so the difference between the two
// is one argument and stays visible.
func (s *Speaker) prepareLong(text string) (string, bool) {
	s.mu.Lock()
	enabled, quiet := s.opt.Enabled, s.quiet
	s.mu.Unlock()
	if !enabled {
		return "", false
	}
	if quiet != nil && quiet(time.Now()) {
		s.log.Debug(logging.EvSpeechSpoke, "component", "speech", "skipped", "quiet_hours")
		return "", false
	}
	line := Clean(text, 0)
	return line, line != ""
}

// enqueue holds the lock across the send, which is what makes it safe against a
// concurrent Close: either the send happens before the queue is closed, or the
// closed flag is already set and the line is dropped. Every send here is
// non-blocking, so holding the lock cannot stall anybody.
func (s *Speaker) enqueue(u queued) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		u.release()
		return
	}
	for {
		select {
		case s.lines <- u:
			return
		default:
		}
		// Full. Drop the oldest and try again; if the run goroutine took one in the
		// meantime the next send succeeds.
		select {
		case dropped := <-s.lines:
			dropped.release() // a dropped line still ends whatever was waiting on it
			s.log.Info(logging.EvSpeechSpoke, "component", "speech", "dropped", truncate(dropped.line, 40))
		default:
		}
	}
}

// Stop silences the voice now: everything still queued is dropped, and the line
// being heard is cut off mid-word rather than allowed to finish. It is what a
// human pressing stop means, so it is deliberately harsher than Close - the
// Speaker stays usable and the next Speak starts a fresh engine.
//
// Waiters are released, because a caller blocked on SpeakWait for a line nobody
// will now hear must not be left hanging (the queued.release contract).
func (s *Speaker) Stop() {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return
	}
	for {
		select {
		case dropped := <-s.lines:
			dropped.release()
		default:
			select {
			case s.cut <- struct{}{}:
			default: // a cut is already pending; one is enough
			}
			s.log.Info(logging.EvSpeechStopped, "component", "speech", "reason", "stopped")
			return
		}
	}
}

// MaxSpokenChars is the configured ceiling on one utterance. A caller reading
// something longer than a sentence needs it to size its own chunks, because
// prepare truncates past it rather than splitting.
func (s *Speaker) MaxSpokenChars() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opt.MaxChars
}

// Close stops the engine and the player. Safe to call twice.
func (s *Speaker) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	close(s.lines)
	<-s.done
}

// run owns the pipeline for its whole life. Everything that touches the engine
// happens here, so there is no lock around a subprocess.
func (s *Speaker) run() {
	defer close(s.done)

	var (
		pipe  *pipeline
		timer *time.Timer
		idle  <-chan time.Time
	)
	forget := func(why string) {
		pipe = nil
		s.log.Info(logging.EvSpeechStopped, "component", "speech", "reason", why)
		if timer != nil {
			timer.Stop()
			timer = nil
		}
		idle = nil
	}
	// Two ways to let a pipeline go, and the difference is audible. release lets it
	// finish what it is already saying (idle, shutdown); kill takes the sound away
	// now, which is the only thing a human pressing stop can mean.
	release := func(why string) {
		if pipe == nil {
			return
		}
		pipe.close()
		forget(why)
	}
	kill := func(why string) {
		if pipe == nil {
			return
		}
		pipe.kill()
		forget(why)
	}
	defer release("shutdown")

	for {
		select {
		case u, ok := <-s.lines:
			if !ok {
				return
			}
			if wait := s.beforeHook(); wait != nil {
				wait()
			}
			var err error
			if pipe == nil {
				if pipe, err = s.start(); err != nil {
					s.log.Error(logging.EvSpeechFailed, "component", "speech", "err", err.Error())
					u.release()
					continue
				}
			}
			if u.line != "" {
				// Clear a cut left over from a previous line, so stopping one
				// utterance cannot silence the one after it.
				select {
				case <-s.cut:
				default:
				}
				from := pipe.written()
				if err = pipe.say(u.line); err != nil {
					// The engine went away (a killed piper, a closed player). One
					// rebuild, then give up on this line rather than spin.
					release("write failed")
					if pipe, err = s.start(); err == nil {
						from = pipe.written()
						err = pipe.say(u.line)
					}
					if err != nil {
						s.log.Error(logging.EvSpeechFailed, "component", "speech", "err", err.Error())
						u.release()
						continue
					}
				}
				s.log.Debug(logging.EvSpeechSpoke, "component", "speech", "chars", len(u.line))
				if u.done != nil {
					// Somebody is waiting for this one, so the queue waits too: the
					// idle timer below then starts when the sound stopped, not when
					// the line was handed to the engine.
					grace, ceiling := waitBudget(u.line)
					waited, interrupted := pipe.drain(from, grace, ceiling, drainSettle, s.cut)
					s.log.Debug(logging.EvSpeechSpoke, "component", "speech",
						"waited_ms", waited.Milliseconds(), "interrupted", interrupted)
					if interrupted {
						// The pipeline holds audio already handed to the player, so
						// ending the wait is not enough to make it quiet: only killing
						// the engine and the player is. Not close() - that shuts stdin
						// and waits, and a neural engine blocked writing a minute of
						// PCM would keep talking for the whole drainGrace. The next
						// line builds a fresh pipeline.
						kill("interrupted")
					}
				} else {
					// Nothing to wait on, so a stop can only take effect between
					// lines. Honour it here rather than at the next utterance.
					select {
					case <-s.cut:
						kill("interrupted")
					default:
					}
				}
			}
			u.release()
			if d := s.idleAfter(); d > 0 {
				if timer == nil {
					timer = time.NewTimer(d)
				} else {
					timer.Reset(d)
				}
				idle = timer.C
			}
		case <-idle:
			release("idle")
		}
	}
}

func (s *Speaker) beforeHook() func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.before
}

func (s *Speaker) idleAfter() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opt.Idle
}

// --- the pipeline -----------------------------------------------------------

// pipeline is the engine and the player, joined by a counting copy: every byte of
// PCM the engine produces is measured on its way to the player, which is the
// whole of how agentbox knows when a line has finished being heard (see meter).
//
// The join used to be a single OS pipe between the two children, which is cheaper
// and tells Go nothing at all - no byte passed through this process, so "has that
// sentence finished?" had no answer better than a guess at words per second. The
// copy costs one goroutine and a few hundred KB of traffic per sentence.
// Back-pressure survives it: the copy blocks on the player's stdin exactly where
// the kernel used to block the engine.
type pipeline struct {
	synth  *exec.Cmd
	player *exec.Cmd
	in     io.WriteCloser
	meter  *meter
	// Closed when each child has been reaped, so close can wait for a sentence to
	// finish rather than cut it off. exec.Cmd.Wait may only be called once, which
	// is why this is a channel and not a second Wait.
	synthGone chan struct{}
	playGone  chan struct{}
}

// drainGrace is how long a shutting-down pipeline is given to finish what it is
// already saying. A quiet pipeline closes in microseconds - the wait ends when
// the process does - so this only ever costs anything mid-sentence, which is
// exactly when it is worth paying.
const drainGrace = 5 * time.Second

func (s *Speaker) start() (*pipeline, error) {
	s.mu.Lock()
	opt, player := s.opt, s.player
	s.mu.Unlock()

	if len(opt.Argv) == 0 {
		return nil, ErrNoEngine
	}
	if player == "" {
		return nil, ErrNoPlayer
	}

	synth := exec.Command(opt.Argv[0], opt.Argv[1:]...)
	in, err := synth.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("engine stdin: %w", err)
	}
	// Two pipes with the counting copy between them: engine → agentbox → player.
	// os.Pipe rather than Cmd.StdoutPipe because Cmd.Wait closes the pipe it hands
	// out, and the reaper goroutines below call Wait as soon as a child exits -
	// which would shut the read end while the copy was still draining it.
	synthR, synthW, err := os.Pipe()
	if err != nil {
		in.Close()
		return nil, fmt.Errorf("pipe: %w", err)
	}
	playR, playW, err := os.Pipe()
	if err != nil {
		in.Close()
		synthR.Close()
		synthW.Close()
		return nil, fmt.Errorf("pipe: %w", err)
	}
	closeAll := func() {
		in.Close()
		synthR.Close()
		synthW.Close()
		playR.Close()
		playW.Close()
	}
	synth.Stdout = synthW
	// piper narrates its own progress on stderr, one line per utterance. It is not
	// agentbox's log and it must not fill a pipe nobody reads.
	synth.Stderr = nil

	play := exec.Command(player, playerArgs(player, opt)...)
	play.Stdin = playR

	if err := synth.Start(); err != nil {
		closeAll()
		return nil, fmt.Errorf("start %s: %w", opt.Argv[0], err)
	}
	if err := play.Start(); err != nil {
		closeAll()
		synth.Process.Kill()
		synth.Wait()
		return nil, fmt.Errorf("start %s: %w", filepath.Base(player), err)
	}
	// Each child holds its own end now; the parent must let go of those two or
	// nothing ever sees EOF. The other two belong to the copy.
	synthW.Close()
	playR.Close()

	p := &pipeline{
		synth: synth, player: play, in: in,
		meter:     &meter{bps: bytesPerSecond(opt)},
		synthGone: make(chan struct{}),
		playGone:  make(chan struct{}),
	}
	go func() {
		// The engine exiting is no longer the player's EOF, so the copy passes it
		// on: closing the player's stdin here is what lets the last sentence reach
		// the speakers instead of dying in a pipe nobody closed.
		io.Copy(counted{w: playW, m: p.meter}, synthR)
		playW.Close()
		synthR.Close()
	}()
	go func() { synth.Wait(); close(p.synthGone) }()
	go func() { play.Wait(); close(p.playGone) }()

	s.log.Info(logging.EvSpeechStarted, "component", "speech",
		"engine", filepath.Base(opt.Argv[0]), "player", filepath.Base(player), "rate", opt.Rate)
	return p, nil
}

func (p *pipeline) say(text string) error {
	_, err := io.WriteString(p.in, text+"\n")
	return err
}

// close lets the pipeline finish rather than cutting it off. Shutting the engine's
// stdin is a request to stop: the engine synthesises what it already has and
// exits, its exit is the copy's EOF, the copy closes the player's stdin, and that
// is the player's EOF - so the last sentence reaches the speakers. Only a process
// that outstays drainGrace is killed.
//
// It blocks, deliberately. On idle release there is nothing to say so it returns
// at once, and on daemon shutdown blocking is the whole point - agentbox exists so a
// message is not lost, and truncating its own last word would be a poor showing.
func (p *pipeline) close() {
	p.in.Close()
	waitOrKill(p.synthGone, p.synth)
	waitOrKill(p.playGone, p.player)
}

// kill is close's harsh twin: the sound stops now rather than finishing.
//
// Both children go, and the player has to be one of them - it holds a buffer of
// PCM the meter has already counted, so a dead engine alone would leave a second
// or two of speech still coming out. Killing the engine before shutting stdin
// also matters with a neural voice: it emits a whole line's audio in one write,
// so at stop time it is blocked inside that write with most of the reading still
// to go, and close's request-then-wait would talk over the whole drainGrace.
func (p *pipeline) kill() {
	if p.synth.Process != nil {
		p.synth.Process.Kill()
	}
	if p.player.Process != nil {
		p.player.Process.Kill()
	}
	p.in.Close()
	<-p.synthGone
	<-p.playGone
}

func waitOrKill(gone <-chan struct{}, cmd *exec.Cmd) {
	select {
	case <-gone:
	case <-time.After(drainGrace):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		<-gone
	}
}

// --- knowing when the sound stopped -----------------------------------------

// Waiting for a line to be over is the difference between a narration that reads
// like speech and one that talks over itself. The alternative every script starts
// with - sleep for words divided by a words-per-second guess - is wrong in both
// directions at once: it clips a line the engine drew out and it leaves a hole
// after a short one, and the error compounds over a dozen lines.
//
// Raw PCM makes the honest answer cheap. There is no container and no variable
// bitrate, so a byte count converts to a duration exactly: bytes / (2 · rate ·
// channels) seconds, the 2 being s16. meter keeps that arithmetic as one
// wall-clock instant, `until` - when everything handed to the player so far will
// have finished coming out of it. A byte written while the player still has a
// backlog plays after that backlog; a byte written to a player that has run dry
// starts playing now. Which of the two it is, is the only thing this has to
// decide, and the clock decides it.
type meter struct {
	mu    sync.Mutex
	bps   float64   // bytes of PCM per second of audio
	bytes int64     // how many have been handed to the player
	last  time.Time // when the last one was
	until time.Time // when all of them will have been heard
}

func (m *meter) wrote(n int) {
	now := time.Now()
	d := time.Duration(float64(n) / m.bps * float64(time.Second))
	m.mu.Lock()
	if now.After(m.until) {
		m.until = now.Add(d) // the player had run dry: this starts playing now
	} else {
		m.until = m.until.Add(d) // it queues behind what is still playing
	}
	m.bytes += int64(n)
	m.last = now
	m.mu.Unlock()
}

func (m *meter) read() (bytes int64, last, until time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bytes, m.last, m.until
}

// bytesPerSecond is the PCM format as one number. The fallbacks are the config
// defaults: a zero here would be a division by zero, and a rate knob is exactly
// the sort of thing that arrives as zero from a hand-written file.
func bytesPerSecond(opt Options) float64 {
	rate, channels := opt.Rate, opt.Channels
	if rate <= 0 {
		rate = 22050
	}
	if channels <= 0 {
		channels = 1
	}
	return float64(2 * rate * channels)
}

// counted is the copy's write half: the player gets the bytes, the meter gets
// their size and the time they went by.
type counted struct {
	w io.Writer
	m *meter
}

func (c counted) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		c.m.wrote(n)
	}
	return n, err
}

// drainQuiet is how long the stream must have carried nothing before the engine
// counts as finished. An engine writes a sentence in bursts as it synthesises it,
// so short gaps are normal inside one utterance; this is meant to be longer than
// any of those and shorter than the pause a listener would notice between two
// lines.
const drainQuiet = 120 * time.Millisecond

// drainPoll is how often the two conditions are re-checked. The arithmetic is
// exact, so this is the whole of the error: a line is called over within one tick
// of when it really was.
const drainPoll = 20 * time.Millisecond

// drainStart is the fixed part of how long a line has to produce its first byte
// before the wait gives up on it. A line of nothing but punctuation legitimately
// synthesises to silence, and an engine that died mid-line must not hold a waiter
// for ever.
//
// It is only the fixed part because a neural engine emits NOTHING until it has
// finished the whole line: Kokoro's create() phonemises, runs the model over
// every batch and concatenates before a byte reaches stdout. So "has this line
// started?" cannot be answered on a constant - the honest bound scales with the
// text. Measured on this machine (kokoro-onnx 0.5.0, am_michael, one core):
//
//	 66 chars ->  1.70s synthesis for  4.46s of audio
//	218 chars ->  5.74s synthesis for 14.66s of audio
//	743 chars -> 14.15s synthesis for 46.95s of audio
//
// which is ~0.026 s/char, worst case, and 15 chars per second of speech. A flat
// five seconds was therefore already wrong at two sentences: the wait declared
// the line silent, released its waiter, and the reader moved on while forty
// seconds of speech was still being synthesised - so every position the
// transport tracked was tens of seconds ahead of the sound, and anything that
// acted on one (pause, stop, a step change) cut audio nobody had heard yet. It
// is the whole of FR66's truncation.
const drainStart = 5 * time.Second

// synthPerChar and speechPerChar size the two bounds from the line itself, both
// with better than 2x headroom over the measurements above, because these are
// backstops against a dead engine and not a schedule.
const (
	synthPerChar  = 60 * time.Millisecond
	speechPerChar = 140 * time.Millisecond
)

// drainCeiling is the fixed part of the bound on any single wait. Reaching it
// means something is wrong rather than long-winded, and a caller that asked to
// be told when a line ended is better served by a late answer than by none.
const drainCeiling = 30 * time.Second

// drainSettle is held after both conditions say the line is over, because the
// meter cannot see the whole path. It counts a byte as playing when the byte is
// handed to the player, while the pipe between them and the player's own buffer
// still hold audio the speakers have not reached. The error is one startup
// latency, always in the same direction - the wait ends early - and Kokoro's
// audio ends on the last word with no trailing pad, so anything acting on the
// end of a wait acts while the final word is still in flight.
const drainSettle = 400 * time.Millisecond

// waitBudget is how long one line is given, in two parts: to produce its first
// byte, and then to be over. Both scale with the text for the reason drainStart
// explains at length.
func waitBudget(line string) (start, ceiling time.Duration) {
	n := utf8.RuneCountInString(line)
	start = drainStart + time.Duration(n)*synthPerChar
	return start, start + drainCeiling + time.Duration(n)*speechPerChar
}

// drain blocks until the line just written has been heard, and reports how long
// that took. Only a caller that asked to wait ever reaches it, so the ordinary
// fire-and-forget path pays nothing.
//
// Two conditions, and it takes both. The stream must have been quiet for
// drainQuiet, which says the engine has stopped producing; and the meter's `until`
// must have passed, which says the player has worked through what it was given.
// Quiet alone would end the wait the moment the engine got far enough ahead to
// fill the pipe and block - the sound is then at its loudest. `until` alone would
// end it in a gap between two bursts of synthesis, since a backlog of nothing has
// always finished playing.
//
// from is the byte count taken before the line was written: until the count moves
// past it, none of this line has been produced yet and a drained stream means the
// engine has not started rather than finished. grace is how long that start is
// waited for, and ceiling bounds the whole wait; waitBudget sizes both from the
// line, and a test passes shorter ones.
// drain returns how long it waited, and whether cut ended the wait early rather
// than the audio finishing. A cut caller is expected to release the pipeline:
// this only stops listening, it does not stop the sound.
func (p *pipeline) drain(from int64, grace, ceiling, settle time.Duration, cut <-chan struct{}) (time.Duration, bool) {
	began := time.Now()
	tick := time.NewTicker(drainPoll)
	defer tick.Stop()
	for {
		select {
		case <-cut:
			return time.Since(began), true
		case <-tick.C:
			bytes, last, until := p.meter.read()
			now := time.Now()
			if bytes == from {
				if now.Sub(began) >= grace {
					return now.Sub(began), false // this line synthesised to nothing
				}
				continue
			}
			if now.Sub(last) >= drainQuiet && now.After(until.Add(settle)) {
				return now.Sub(began), false
			}
			if now.Sub(began) >= ceiling {
				return now.Sub(began), false
			}
		}
	}
}

// written is the meter's count, taken before a line is spoken so drain can tell
// this line's bytes from the tail of the one before it.
func (p *pipeline) written() int64 {
	n, _, _ := p.meter.read()
	return n
}

// playerArgs translates the PCM format for the resolved player. The three differ
// in every detail including the spelling of the sample format, which is why this
// is a table and not a format string. aplay has no volume control, so it plays at
// engine level.
func playerArgs(bin string, opt Options) []string {
	rate := strconv.Itoa(opt.Rate)
	channels := strconv.Itoa(opt.Channels)
	switch filepath.Base(bin) {
	case "pw-play":
		return []string{
			"--rate=" + rate, "--channels=" + channels, "--format=s16",
			// Max resampler quality, always. A piper voice is 22.05 kHz and a modern
			// sink runs at 48 kHz, so every utterance is resampled - measured on this
			// machine, the default sink is s32le 48000Hz. PipeWire's default quality
			// is 4 of 15, which is chosen to be cheap on a stream that plays for an
			// hour. agentbox's streams are one sentence long, so the cost is noise and
			// the fidelity is the whole point of asking for a high-quality voice.
			"--quality=" + strconv.Itoa(resamplerQuality),
			fmt.Sprintf("--volume=%.2f", opt.Volume), "-",
		}
	case "paplay":
		return []string{
			"--raw", "--rate=" + rate, "--channels=" + channels, "--format=s16le",
			fmt.Sprintf("--volume=%d", int(opt.Volume*65536)),
		}
	default: // aplay
		return []string{"-q", "-t", "raw", "-f", "S16_LE", "-r", rate, "-c", channels}
	}
}

// --- the spoken line --------------------------------------------------------

// Clean turns whatever an agent wrote into one line an engine can read. The
// engine's protocol is a line per utterance, so a newline would split a sentence
// into two and a control character would confuse it.
func Clean(text string, maxChars int) string {
	text = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r == utf8.RuneError, unicode.IsControl(r):
			return -1
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if maxChars <= 0 {
		return text
	}
	return truncate(text, maxChars)
}

// Utterance is a whole reading as the one line the engine reads it from.
//
// There used to be a splitter here (Passages), and removing it is the FR72
// decision. An utterance is what the engine synthesises in one pass, so its
// prosody is decided as a unit and every split costs something: the half before a
// break gets a falling, finished intonation it should not have. Splitting was
// also what bought the transport its positions, and those positions were the
// defect - drainStart carries the measurements. The owner's rule settles the
// trade outright: "The audio control must under no circumstances harm the quality
// of the speech in any way. If it does even in the smallest amount then I want
// only the parts that don't even if it means only play and stop."
//
// So a press reads one region of the page, whole, and the transport is play and
// stop. A paragraph break inside a region becomes a sentence break, which is
// exactly what the engine would have made of the text handed over at once.
//
// Nothing is truncated: Clean runs with no ceiling, because the ceiling is a
// notification policy and applying it to something a human asked to hear would
// silently drop the end of it.
func Utterance(text string) string {
	return Clean(text, 0)
}

// truncate cuts to a rune count, preferring the last word boundary so a line ends
// on a word rather than half of one.
func truncate(text string, maxChars int) string {
	if utf8.RuneCountInString(text) <= maxChars {
		return text
	}
	cut := text
	for i, n := 0, 0; i < len(text); {
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
		n++
		if n == maxChars {
			cut = text[:i]
			break
		}
	}
	if space := strings.LastIndexByte(cut, ' '); space > len(cut)/2 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " ,;:-") + "..."
}

// --- finding an engine ------------------------------------------------------

// voiceDirs are where piper voices are kept, in the order they are tried. The
// first is piper's own convention, the second is what people actually do.
var voiceDirs = []string{
	"~/.local/share/piper-voices",
	"~/piper-voices",
	"/usr/share/piper-voices",
	"/usr/local/share/piper-voices",
}

// detect finds piper and a voice so that `[speech] enabled = true` is enough on a
// machine where piper already works. It never guesses at the sample rate: that
// comes out of the voice's own config, because a voice played at the wrong rate
// is not subtly wrong, it is a chipmunk.
func (s *Speaker) detect() (argv []string, rate int, err error) {
	bin, err := s.look("piper")
	if err != nil {
		return nil, 0, ErrNoEngine
	}
	model := findVoice()
	if model == "" {
		return nil, 0, fmt.Errorf("%w: piper is installed but no .onnx voice was found in %s",
			ErrNoEngine, strings.Join(voiceDirs, ", "))
	}
	tier, _ := voiceTier(model)
	s.log.Info(logging.EvSpeechStarted, "component", "speech", "voice", filepath.Base(model),
		"tier", tier, "picked", "highest available")
	return []string{bin, "--model", model, "--output-raw"}, voiceRate(model), nil
}

// findVoice prefers a high-quality English voice, then any high one, then
// whatever is there, so the pick is stable rather than filesystem order.
func findVoice() string {
	var found []string
	for _, dir := range voiceDirs {
		if strings.HasPrefix(dir, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			dir = filepath.Join(home, dir[2:])
		}
		matches, err := filepath.Glob(filepath.Join(dir, "*.onnx"))
		if err != nil {
			continue
		}
		found = append(found, matches...)
	}
	if len(found) == 0 {
		return ""
	}
	sort.Slice(found, func(i, j int) bool {
		if a, b := voiceScore(found[i]), voiceScore(found[j]); a != b {
			return a > b
		}
		return found[i] < found[j]
	})
	return found[0]
}

// tiers are piper's quality levels, worst to best. The tier is part of the model
// filename, which is the only place it is recorded.
var tiers = []string{"x_low", "low", "medium", "high"}

// voiceTier names the quality level in a model's filename, and how good it is.
// Unknown spellings rank below every known one rather than above: a voice that
// does not say what it is gets the benefit of no doubt.
func voiceTier(path string) (string, int) {
	name := strings.ToLower(filepath.Base(path))
	for i := len(tiers) - 1; i >= 0; i-- {
		if strings.Contains(name, "-"+tiers[i]) {
			return tiers[i], i + 1
		}
	}
	return "unknown", 0
}

// voiceScore ranks a voice. Language first, because a perfect German voice
// reading English is not a better outcome than a merely good English one, then
// quality tier - and the tier is why this exists at all: with several voices
// installed, agentbox takes the best one every time rather than whichever the
// filesystem listed first.
func voiceScore(path string) int {
	name := strings.ToLower(filepath.Base(path))
	score := 0
	if strings.HasPrefix(name, "en") {
		score += 10
	}
	_, tier := voiceTier(path)
	return score + tier
}

// voiceRate reads the sample rate out of the voice's companion JSON, which piper
// writes beside every model.
func voiceRate(model string) int {
	const fallback = 22050
	data, err := os.ReadFile(model + ".json")
	if err != nil {
		return fallback
	}
	var cfg struct {
		Audio struct {
			SampleRate int `json:"sample_rate"`
		} `json:"audio"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.Audio.SampleRate <= 0 {
		return fallback
	}
	return cfg.Audio.SampleRate
}
