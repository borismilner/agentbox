package daemon

import (
	"context"
	"log/slog"
	"sync"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/speech"
)

// aloud reads one region of a screen out loud, on the human's request, with the
// only two controls that cost the speech nothing: play and stop.
//
// It exists because Speak is the wrong shape for this. Speak is one line, capped
// at the configured ceiling and truncated past it, chosen by an agent that
// decided a sentence was worth hearing. A human asking to hear a passage wants
// all of it, and wants to be able to take it back.
//
// It reads a region as ONE utterance, which is the FR72 correction. The first
// version split a step into passages so it could offer pause, rewind and a
// position between them, and the split is what broke the feature: a neural engine
// emits nothing until it has finished a whole line, so the wait for a passage
// timed out long before its audio existed (speech.drainStart carries the
// measurements), the position ran tens of seconds ahead of the sound, and every
// control acting on that position cut words nobody had heard. Regions the human
// picks one at a time turn out to be the better surface anyway: the reader hears
// the paragraph above a code block, reads the code, then asks for the next
// paragraph, instead of racing a voice down the page.
//
// One reading at a time, deliberately: two voices over each other is never what
// was wanted, so starting a reading replaces whatever was being read.
type aloud struct {
	snd Sounder
	log *slog.Logger

	mu      sync.Mutex
	region  string // what the surface called the region being read, "" when idle
	playing bool
	// gen invalidates the reader goroutine. Every command bumps it, so a
	// goroutine that wakes from a finished utterance and finds a different gen
	// knows it has been superseded and exits without touching state. That is
	// cheaper and less error-prone than cancelling a context per utterance.
	gen int
}

func newAloud(snd Sounder, log *slog.Logger) *aloud {
	return &aloud{snd: snd, log: log}
}

// Aloud satisfies webui.Voice: the surface working the controls, over the same
// reader the RPC method drives, so a board button and `agentbox` on the socket cannot
// end up with two readings talking over each other.
func (d *Daemon) Aloud(action, region, text string) proto.AloudResult {
	return d.aloud.Command(action, region, text)
}

// Command applies one action and reports the state that resulted. Every action is
// safe to send at any time: the ones that make no sense in the current state
// (stop with nothing playing) are no-ops that report the truth rather than
// errors, because a button the human can see should not be able to fail.
func (a *aloud) Command(action, region, text string) proto.AloudResult {
	switch action {
	case proto.AloudStart:
		return a.start(region, text)
	case proto.AloudStop:
		return a.stop()
	default: // including AloudState: answer with the truth and change nothing
		return a.state()
	}
}

func (a *aloud) start(region, text string) proto.AloudResult {
	line := speech.Utterance(text)

	a.mu.Lock()
	a.gen++
	gen := a.gen
	a.region, a.playing = region, line != ""
	// The answer is taken HERE, under the lock that set it, rather than re-read
	// after the reader goroutine is loose. A start reports what the start did; by
	// the time it could ask again the reading may already be over, and it would
	// then answer "not playing" about a reading the caller just began. That is not
	// theoretical - it is what made this suite fail once under -race in session 25
	// and again in session 34, and on a machine where the utterance is short or
	// the engine answers instantly it is what the surface would paint: a play
	// control for a region that is being read out loud.
	res := proto.AloudResult{OK: true, Playing: a.playing, Region: a.region}
	a.mu.Unlock()

	// Silence whatever is being said before the new line queues behind it.
	a.snd.StopSpeaking()
	if line == "" {
		return a.stopState()
	}
	a.log.Info(logging.EvSpeechAloud, "component", "daemon", "aloud", "start",
		"region", region, "chars", len(line))
	go a.read(gen, line)
	return res
}

func (a *aloud) stop() proto.AloudResult {
	a.mu.Lock()
	a.gen++
	a.region, a.playing = "", false
	a.mu.Unlock()
	a.snd.StopSpeaking()
	a.log.Info(logging.EvSpeechAloud, "component", "daemon", "aloud", "stop")
	return a.state()
}

// read speaks the one line and reports the reading over unless it has been
// superseded. It holds no lock across the utterance - one can last a minute - so
// it checks whether it is still the live reader on both sides of it.
//
// Before matters as much as after. A stop arriving in the gap between the
// goroutine starting and the line reaching the queue would otherwise be
// swallowed: the Speaker's cut has already been consumed by then, and the run
// loop clears a stale one before it speaks, so the region would be read out after
// the human had pressed stop.
func (a *aloud) read(gen int, line string) {
	a.mu.Lock()
	live := a.gen == gen && a.playing
	a.mu.Unlock()
	if !live {
		return
	}

	a.snd.ReadWait(context.Background(), line)

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.gen != gen {
		// Stopped or replaced while this was being heard: whoever bumped gen owns
		// the state now.
		return
	}
	a.region, a.playing = "", false
	a.log.Info(logging.EvSpeechAloud, "component", "daemon", "aloud", "finished")
}

// stopState is the state of a reading that never started - nothing to say, or
// speech turned off. Reported as not playing, so the surface paints a control
// the human can press again rather than one stuck mid-read.
func (a *aloud) stopState() proto.AloudResult {
	a.mu.Lock()
	a.region, a.playing = "", false
	a.mu.Unlock()
	return a.state()
}

func (a *aloud) state() proto.AloudResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	return proto.AloudResult{OK: true, Playing: a.playing, Region: a.region}
}
