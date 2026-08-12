package speech

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// The engine and the player are both subprocesses, so the pipeline is testable
// without piper: a shell script that reads lines and writes bytes is a valid
// engine by the contract this package defines, which is the point of defining the
// contract that narrowly.

func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeEngine reads a line at a time, appends it to a transcript, and emits a
// little PCM for each - exactly the contract. It records "closed" when its stdin
// goes away, which is how the idle-release test sees the engine let go.
func fakeEngine(t *testing.T, transcript string) string {
	t.Helper()
	return fakeEngineBytes(t, transcript, 64)
}

// fakeEngineBytes is the same engine with the size of an utterance as a knob, so a
// test can ask for a line that takes a known time to play: the pipeline converts
// bytes to a duration through the rate, and both ends of that are the test's.
func fakeEngineBytes(t *testing.T, transcript string, n int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engine.sh")
	script := fmt.Sprintf(`#!/bin/sh
while IFS= read -r line; do
  printf '%%s\n' "$line" >> %q
  dd if=/dev/zero bs=%d count=1 2>/dev/null
done
printf 'closed\n' >> %q
`, transcript, n, transcript)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakePlayer swallows the PCM and records that it received some, so a test can
// tell "the engine spoke" from "the audio actually went somewhere".
func fakePlayer(t *testing.T, sink string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aplay")
	script := fmt.Sprintf("#!/bin/sh\ncat >> %q\n", sink)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func lookFor(player string) func(string) (string, error) {
	return func(name string) (string, error) {
		if name == "aplay" {
			return player, nil
		}
		return "", exec.ErrNotFound
	}
}

func waitForFile(t *testing.T, path, want string, d time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil && strings.Contains(string(b), want) {
			return string(b)
		}
		time.Sleep(10 * time.Millisecond)
	}
	b, _ := os.ReadFile(path)
	t.Fatalf("waited %v for %q; transcript is:\n%s", d, want, b)
	return ""
}

// waitForBytes waits for a file to be non-empty, which is NOT what
// waitForFile(path, "") does: every string contains the empty substring, so that
// call returns the moment the file exists, however empty it is. On a fast machine
// the writer has always got there first and the difference never shows; on a
// two-core CI runner it showed as "no audio reached the player" with a stat that
// found the file perfectly well.
func waitForBytes(t *testing.T, path string, d time.Duration) int64 {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return info.Size()
		}
		time.Sleep(10 * time.Millisecond)
	}
	info, err := os.Stat(path)
	size := int64(-1)
	if err == nil {
		size = info.Size()
	}
	t.Fatalf("waited %v for bytes in %s; stat err=%v size=%d", d, path, err, size)
	return 0
}

func newTestSpeaker(t *testing.T, opt Options) (*Speaker, string, string) {
	t.Helper()
	dir := t.TempDir()
	transcript := filepath.Join(dir, "said.txt")
	audio := filepath.Join(dir, "audio.raw")
	engine := fakeEngine(t, transcript)
	player := fakePlayer(t, audio)

	s := New(quietLog(), lookFor(player))
	t.Cleanup(s.Close)
	if opt.Argv == nil {
		opt.Argv = []string{engine}
	}
	if opt.Rate == 0 {
		opt.Rate = 22050
	}
	if opt.Channels == 0 {
		opt.Channels = 1
	}
	s.Configure(opt, nil)
	return s, transcript, audio
}

func TestSpeakGoesThroughTheEngineAndThePlayer(t *testing.T) {
	s, transcript, audio := newTestSpeaker(t, Options{Enabled: true, MaxChars: 240})

	s.Speak("The staging migration failed. Rolled back.")
	waitForFile(t, transcript, "The staging migration failed. Rolled back.", 5*time.Second)

	// Any bytes at all is the claim: the PCM reached a player. The wait and the
	// assertion are now the same question, which they were not before.
	if n := waitForBytes(t, audio, 5*time.Second); n == 0 {
		t.Error("no audio reached the player")
	}
}

func TestSpeakReusesOneEngine(t *testing.T) {
	// The whole reason this package exists: three lines must not be three engines.
	s, transcript, _ := newTestSpeaker(t, Options{Enabled: true, MaxChars: 240})

	for _, line := range []string{"first", "second", "third"} {
		s.Speak(line)
	}
	got := waitForFile(t, transcript, "third", 5*time.Second)
	if strings.Contains(got, "closed") {
		t.Errorf("the engine was torn down between lines:\n%s", got)
	}
	if want := "first\nsecond\nthird\n"; !strings.Contains(got, want) {
		t.Errorf("lines out of order or missing:\n%s", got)
	}
}

func TestSpeakIsSilentWhenOff(t *testing.T) {
	s, transcript, _ := newTestSpeaker(t, Options{Enabled: false, MaxChars: 240})
	s.Speak("this should not be said")
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(transcript); err == nil {
		b, _ := os.ReadFile(transcript)
		t.Errorf("spoke with speech disabled:\n%s", b)
	}
}

func TestSpeakIsSilentInQuietHours(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "said.txt")
	s := New(quietLog(), lookFor(fakePlayer(t, filepath.Join(dir, "audio.raw"))))
	t.Cleanup(s.Close)
	s.Configure(Options{
		Enabled: true, Argv: []string{fakeEngine(t, transcript)}, Rate: 22050, Channels: 1, MaxChars: 240,
	}, func(time.Time) bool { return true })

	s.Speak("not at three in the morning")
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(transcript); err == nil {
		t.Error("spoke during quiet hours")
	}
}

func TestSpeakWaitsForTheEarcon(t *testing.T) {
	s, transcript, _ := newTestSpeaker(t, Options{Enabled: true, MaxChars: 240})

	released := make(chan struct{})
	s.Before(func() { <-released })
	s.Speak("after the chime")

	time.Sleep(200 * time.Millisecond)
	if b, err := os.ReadFile(transcript); err == nil && strings.Contains(string(b), "after the chime") {
		t.Fatal("spoke before the earcon finished")
	}
	close(released)
	waitForFile(t, transcript, "after the chime", 5*time.Second)
}

func TestSpeakReleasesTheEngineWhenIdle(t *testing.T) {
	// A voice model is ~100MB resident; an idle daemon must not hold one.
	s, transcript, _ := newTestSpeaker(t, Options{
		Enabled: true, MaxChars: 240, Idle: 150 * time.Millisecond,
	})

	s.Speak("say something")
	waitForFile(t, transcript, "say something", 5*time.Second)
	waitForFile(t, transcript, "closed", 5*time.Second)

	// And it comes back on demand.
	s.Speak("say another thing")
	waitForFile(t, transcript, "say another thing", 5*time.Second)
}

func TestQueueIsBounded(t *testing.T) {
	// Twenty notifications at once must not become a monologue, and must not grow
	// without limit either. With the run goroutine parked on Before, everything
	// piles up in the queue where the bound is enforced.
	s, _, _ := newTestSpeaker(t, Options{Enabled: true, MaxChars: 240})
	parked := make(chan struct{})
	s.Before(func() { <-parked })
	defer close(parked)

	for i := range 100 {
		s.Speak(fmt.Sprintf("line %d", i))
	}
	if n := len(s.lines); n > queueDepth {
		t.Errorf("queue holds %d lines, cap is %d", n, queueDepth)
	}
}

// --- waiting for a line to be over ------------------------------------------

func assertReleased(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Errorf("%s: the waiter was never released", what)
	}
}

func TestSpeakWaitReturnsWhenTheAudioIsOver(t *testing.T) {
	// 8000 bytes of s16 mono at 8000 Hz is half a second of sound. The fake player
	// swallows it the instant it arrives, so a wait that only followed the bytes
	// would come back immediately - the half second comes out of the arithmetic.
	dir := t.TempDir()
	transcript := filepath.Join(dir, "said.txt")
	s := New(quietLog(), lookFor(fakePlayer(t, filepath.Join(dir, "audio.raw"))))
	t.Cleanup(s.Close)
	s.Configure(Options{
		Enabled: true, Argv: []string{fakeEngineBytes(t, transcript, 8000)},
		Rate: 8000, Channels: 1, MaxChars: 240,
	}, nil)

	began := time.Now()
	<-s.SpeakWait("half a second of speech")
	took := time.Since(began)
	if took < 400*time.Millisecond {
		t.Errorf("came back after %v, before half a second of audio could have played", took)
	}
	if took > 3*time.Second {
		t.Errorf("took %v to wait out half a second of audio", took)
	}
	waitForFile(t, transcript, "half a second of speech", time.Second)
}

func TestSpeakStaysNonBlocking(t *testing.T) {
	// Only --wait waits. An ordinary spoken line must not cost its caller the time
	// it takes to read, which is the whole reason the queue exists.
	dir := t.TempDir()
	s := New(quietLog(), lookFor(fakePlayer(t, filepath.Join(dir, "audio.raw"))))
	t.Cleanup(s.Close)
	s.Configure(Options{
		Enabled: true, Argv: []string{fakeEngineBytes(t, filepath.Join(dir, "said.txt"), 8000)},
		Rate: 8000, Channels: 1, MaxChars: 240,
	}, nil)

	began := time.Now()
	s.Speak("half a second of speech")
	if took := time.Since(began); took > 100*time.Millisecond {
		t.Errorf("Speak blocked for %v", took)
	}
}

func TestSpeakWaitDoesNotHangWhenThereIsNothingToSay(t *testing.T) {
	// Every path has to release the waiter. A caller left waiting for a line that
	// was never going to be said is worse than a line that is never said.
	off, _, _ := newTestSpeaker(t, Options{Enabled: false, MaxChars: 240})
	assertReleased(t, off.SpeakWait("not a word"), "speech off")

	s, _, _ := newTestSpeaker(t, Options{Enabled: true, MaxChars: 240})
	assertReleased(t, s.SpeakWait("   \n "), "a line that cleans to nothing")

	dir := t.TempDir()
	night := New(quietLog(), lookFor(fakePlayer(t, filepath.Join(dir, "audio.raw"))))
	t.Cleanup(night.Close)
	night.Configure(Options{
		Enabled: true, Argv: []string{fakeEngine(t, filepath.Join(dir, "said.txt"))},
		Rate: 22050, Channels: 1, MaxChars: 240,
	}, func(time.Time) bool { return true })
	assertReleased(t, night.SpeakWait("not at three in the morning"), "quiet hours")
}

func TestSpeakWaitReleasesADisplacedLine(t *testing.T) {
	// The queue drops its oldest line when it overflows. A dropped line still closes
	// its channel: whoever asked to be told when it ended has to be told that it
	// never will be.
	s, _, _ := newTestSpeaker(t, Options{Enabled: true, MaxChars: 240})
	entered := make(chan struct{}, 1)
	parked := make(chan struct{})
	s.Before(func() {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-parked
	})
	defer close(parked)

	s.Speak("the line that parks the queue")
	<-entered // the run goroutine is now held, so everything after this piles up

	displaced := s.SpeakWait("the line that gets pushed out")
	for i := range queueDepth + 4 {
		s.Speak(fmt.Sprintf("line %d", i))
	}
	assertReleased(t, displaced, "a displaced line")
}

func TestSpeakWaitReleasesEveryLineInOrder(t *testing.T) {
	// Three narrated lines in a row, each waited on: the third must not be spoken
	// before the first has been heard, which is what makes a script readable.
	dir := t.TempDir()
	transcript := filepath.Join(dir, "said.txt")
	s := New(quietLog(), lookFor(fakePlayer(t, filepath.Join(dir, "audio.raw"))))
	t.Cleanup(s.Close)
	s.Configure(Options{
		Enabled: true, Argv: []string{fakeEngineBytes(t, transcript, 1600)},
		Rate: 8000, Channels: 1, MaxChars: 240,
	}, nil)

	began := time.Now()
	for _, line := range []string{"first", "second", "third"} {
		<-s.SpeakWait(line)
	}
	// 100ms of audio each, so three of them cannot be over in much less than 300ms.
	if took := time.Since(began); took < 250*time.Millisecond {
		t.Errorf("three waited lines took %v; they overlapped", took)
	}
	if got := waitForFile(t, transcript, "third", time.Second); !strings.Contains(got, "first\nsecond\nthird\n") {
		t.Errorf("lines out of order:\n%s", got)
	}
}

func TestTheCountingCopyLosesNoBytes(t *testing.T) {
	// The engine and the player used to be joined by one kernel pipe; now every byte
	// passes through this process so it can be counted. The count is worth nothing if
	// the audio is not exactly what the engine produced.
	dir := t.TempDir()
	audio := filepath.Join(dir, "audio.raw")
	const per = 4096
	s := New(quietLog(), lookFor(fakePlayer(t, audio)))
	t.Cleanup(s.Close)
	s.Configure(Options{
		Enabled: true, Argv: []string{fakeEngineBytes(t, filepath.Join(dir, "said.txt"), per)},
		Rate: 22050, Channels: 1, MaxChars: 240,
	}, nil)

	lines := []string{"one", "two", "three"}
	for _, line := range lines {
		<-s.SpeakWait(line)
	}
	want := int64(len(lines) * per)
	var got int64
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(audio); err == nil {
			if got = info.Size(); got == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("the player received %d bytes of the engine's %d", got, want)
}

func TestMeterQueuesBehindWhatIsStillPlaying(t *testing.T) {
	// The one decision the meter makes. Two bursts handed over back to back are a
	// second of audio, not half a second heard twice.
	m := &meter{bps: 16000} // 8000 Hz, s16, mono: 8000 bytes is half a second
	m.wrote(8000)
	_, _, first := m.read()
	m.wrote(8000)
	count, _, second := m.read()
	if count != 16000 {
		t.Errorf("counted %d bytes, want 16000", count)
	}
	if gap := second.Sub(first); gap < 450*time.Millisecond || gap > 550*time.Millisecond {
		t.Errorf("the second burst moved the end by %v, want about 500ms", gap)
	}

	// And the other half of it: after a gap the player has run dry, so the next byte
	// starts playing now rather than behind a backlog that has already been heard.
	dry := &meter{bps: 16000}
	dry.wrote(160) // 10ms of audio
	time.Sleep(60 * time.Millisecond)
	before := time.Now()
	dry.wrote(8000)
	_, _, until := dry.read()
	if d := until.Sub(before); d < 450*time.Millisecond || d > 600*time.Millisecond {
		t.Errorf("half a second handed to an idle player ends in %v, want about 500ms", d)
	}
}

func TestDrainWaitsForThePlayerNotJustTheEngine(t *testing.T) {
	// The engine gets ahead of the speakers: half a second of audio in one burst,
	// then silence on the stream. A quiet stream is not a finished line - the sound
	// is at its loudest right then - so the wait runs until the player could have
	// worked through what it was given.
	p := &pipeline{meter: &meter{bps: 16000}}
	p.meter.wrote(8000)
	took, _ := p.drain(0, drainStart, drainCeiling, 0, nil)
	if took < 400*time.Millisecond {
		t.Errorf("drain returned after %v, before the audio could have played", took)
	}
	if took > 1500*time.Millisecond {
		t.Errorf("drain took %v to wait out half a second of audio", took)
	}
}

func TestDrainWaitsWhileTheStreamIsStillFlowing(t *testing.T) {
	// The other direction: bytes still arriving mean the engine has not finished,
	// even though everything handed over so far has been heard. Ending the wait on
	// the arithmetic alone would cut a line off between two bursts of synthesis.
	p := &pipeline{meter: &meter{bps: 16000}}
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(40 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				p.meter.wrote(320) // 20ms of audio every 40ms: heard before the next one
			}
		}
	}()
	go func() {
		time.Sleep(500 * time.Millisecond)
		close(stop)
	}()

	took, _ := p.drain(0, drainStart, drainCeiling, 0, nil)
	if took < 450*time.Millisecond {
		t.Errorf("drain returned after %v, while the engine was still producing", took)
	}
	if took > 2*time.Second {
		t.Errorf("drain took %v to notice the stream had stopped", took)
	}
}

func TestDrainGivesUpOnALineThatMakesNoSound(t *testing.T) {
	// A line of nothing but punctuation synthesises to silence, and a dead engine
	// produces nothing either. Neither may hold a waiter for ever.
	p := &pipeline{meter: &meter{bps: 16000}}
	took, _ := p.drain(0, 100*time.Millisecond, drainCeiling, 0, nil)
	if took < 100*time.Millisecond {
		t.Errorf("gave up after %v, inside the grace period", took)
	}
	if took > time.Second {
		t.Errorf("took %v to give up on a line that made no sound", took)
	}
}

func TestBytesPerSecondFallsBackRatherThanDivideByZero(t *testing.T) {
	// A rate of zero is exactly what arrives from a hand-written config, and it would
	// otherwise be a division by zero on the audio path.
	if got := bytesPerSecond(Options{}); got != 44100 {
		t.Errorf("bytesPerSecond of an empty Options = %v, want the 22050 mono fallback", got)
	}
	if got := bytesPerSecond(Options{Rate: 24000, Channels: 2}); got != 96000 {
		t.Errorf("24kHz stereo = %v bytes/s, want 96000", got)
	}
}

func TestClean(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"a plain line", "The build is green.", "The build is green."},
		{"newlines become spaces", "two\nlines\nhere", "two lines here"},
		{"runs of space collapse", "a   b\t\tc", "a b c"},
		{"control characters go", "bell\x07 and null\x00", "bell and null"},
		{"nothing at all", "   \n\t ", ""},
	}
	for _, c := range cases {
		if got := Clean(c.in, 240); got != c.want {
			t.Errorf("%s: Clean(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestCleanTruncatesOnAWord(t *testing.T) {
	got := Clean("the quick brown fox jumps over the lazy dog", 20)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("a cut line should say it was cut: %q", got)
	}
	if strings.Contains(got, "jum") && !strings.Contains(got, "jumps") {
		t.Errorf("cut in the middle of a word: %q", got)
	}
	if len([]rune(got)) > 24 {
		t.Errorf("cut too long: %q", got)
	}
	// A line inside the limit is untouched, ellipsis included.
	if got := Clean("short enough", 240); got != "short enough" {
		t.Errorf("a short line was altered: %q", got)
	}
}

func TestPlayerArgsPerPlayer(t *testing.T) {
	opt := Options{Rate: 22050, Channels: 1, Volume: 0.5}
	for _, c := range []struct {
		bin  string
		want []string
	}{
		{"pw-play", []string{"--rate=22050", "--channels=1", "--format=s16", "--quality=15", "--volume=0.50", "-"}},
		{"paplay", []string{"--raw", "--rate=22050", "--channels=1", "--format=s16le", "--volume=32768"}},
		{"aplay", []string{"-q", "-t", "raw", "-f", "S16_LE", "-r", "22050", "-c", "1"}},
	} {
		got := playerArgs("/usr/bin/"+c.bin, opt)
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Errorf("%s args = %v, want %v", c.bin, got, c.want)
		}
	}
}

// TestPlayerAcceptsRawPCM is the claim the flag table rests on, checked against
// the real binaries. It plays 50ms of silence, so it is inaudible; it skips on a
// machine without that player rather than failing.
func TestPlayerAcceptsRawPCM(t *testing.T) {
	opt := Options{Rate: 22050, Channels: 1, Volume: 0.4}
	silence := make([]byte, 22050/20*2)
	ran := 0
	for _, name := range players {
		bin, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		cmd := exec.Command(bin, playerArgs(bin, opt)...)
		cmd.Stdin = strings.NewReader(string(silence))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s rejected raw PCM: %v\n%s", name, err, out)
			continue
		}
		ran++
	}
	if ran == 0 {
		t.Skip("no audio player on this machine")
	}
}

func TestVoiceScorePrefersEnglishAndHighQuality(t *testing.T) {
	ordered := []string{
		"/v/en_US-lessac-high.onnx",
		"/v/en_US-ryan-medium.onnx",
		"/v/en_GB-alan-low.onnx",
		"/v/en_US-amy-x_low.onnx",
		"/v/en_US-nameless.onnx",
		"/v/de_DE-thorsten-high.onnx",
		"/v/fr_FR-siwis-low.onnx",
	}
	for i := 1; i < len(ordered); i++ {
		if voiceScore(ordered[i-1]) < voiceScore(ordered[i]) {
			t.Errorf("%s should not score below %s", ordered[i-1], ordered[i])
		}
	}
	// The tier has to be read out of the name, since it is the only record of it.
	for name, want := range map[string]string{
		"/v/en_US-lessac-high.onnx": "high",
		"/v/en_US-ryan-medium.onnx": "medium",
		"/v/en_GB-alan-low.onnx":    "low",
		"/v/en_US-amy-x_low.onnx":   "x_low",
		"/v/en_US-nameless.onnx":    "unknown",
	} {
		if got, _ := voiceTier(name); got != want {
			t.Errorf("voiceTier(%s) = %q, want %q", name, got, want)
		}
	}
}

func TestVoiceRateComesFromTheVoice(t *testing.T) {
	model := filepath.Join(t.TempDir(), "voice.onnx")
	if err := os.WriteFile(model+".json", []byte(`{"audio":{"sample_rate":16000}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := voiceRate(model); got != 16000 {
		t.Errorf("voiceRate = %d, want 16000 from the voice's own config", got)
	}

	// A voice with no config, or a broken one, falls back rather than guessing 0 -
	// a rate of zero would make the player refuse the stream outright.
	bare := filepath.Join(t.TempDir(), "bare.onnx")
	if got := voiceRate(bare); got != 22050 {
		t.Errorf("missing config: voiceRate = %d, want the 22050 fallback", got)
	}
	broken := filepath.Join(t.TempDir(), "broken.onnx")
	if err := os.WriteFile(broken+".json", []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := voiceRate(broken); got != 22050 {
		t.Errorf("broken config: voiceRate = %d, want the 22050 fallback", got)
	}
}

func TestNoPlayerDisablesSpeech(t *testing.T) {
	// agentbox without audio still works; speech just does not.
	s := New(quietLog(), func(string) (string, error) { return "", exec.ErrNotFound })
	t.Cleanup(s.Close)
	s.Configure(Options{Enabled: true, Argv: []string{"/nonexistent/engine"}}, nil)
	if s.opt.Enabled {
		t.Error("speech should be off with no player to send PCM to")
	}
	s.Speak("this must not panic")
}

func TestCloseIsIdempotent(t *testing.T) {
	s, _, _ := newTestSpeaker(t, Options{Enabled: true, MaxChars: 240})
	s.Speak("something")
	s.Close()
	s.Close()
}

func TestUtteranceKeepsAWholeRegionAsOneLine(t *testing.T) {
	// The FR72 contract, and the one thing a test can hold: a region is handed to
	// the engine whole. Every split cost the voice something, and the paragraph
	// break becomes a sentence break, which is what the engine would have made of
	// the same text handed over at once.
	text := "First paragraph, one sentence.\n\nSecond paragraph. It has two sentences."
	got := Utterance(text)
	want := "First paragraph, one sentence. Second paragraph. It has two sentences."
	if got != want {
		t.Errorf("region did not survive as one line\n got %q\nwant %q", got, want)
	}
	if strings.ContainsAny(got, "\n\r\t") {
		t.Errorf("a control character reached the engine's line protocol: %q", got)
	}
}

func TestUtteranceNeverTruncatesAndLosesNoWords(t *testing.T) {
	// Clean's cap belongs to notifications. Applying it to something a human asked
	// to hear would drop the end of it silently, which is the one failure nobody
	// can hear happening.
	long := strings.TrimSpace(strings.Repeat("The guard keeps the write additive. ", 40))
	got := Utterance(long)
	if want := strings.Join(strings.Fields(long), " "); got != want {
		t.Errorf("text changed\n got %q\nwant %q", got, want)
	}
	if n := utf8.RuneCountInString(got); n <= 240 {
		t.Fatalf("test text is only %d runes; it must exceed the notification cap to prove anything", n)
	}
}

func TestUtteranceOfNothingIsNothing(t *testing.T) {
	// A region with no prose in it must read as silence rather than as a line the
	// engine has to think about.
	for _, in := range []string{"", "   \n\t ", "\n\n\n"} {
		if got := Utterance(in); got != "" {
			t.Errorf("Utterance(%q) = %q, want empty", in, got)
		}
	}
}

func TestWaitBudgetCoversWhatTheEngineActuallyNeeds(t *testing.T) {
	// The defect FR72 fixed: a flat five-second grace against an engine that emits
	// nothing until a whole line is synthesised. Measured on this machine, 218
	// chars took 5.74s to synthesise and 743 chars took 14.15s, so the grace has to
	// scale with the text or the wait declares a line silent while its audio is
	// still being made - and every position downstream is then ahead of the sound.
	for _, tc := range []struct {
		chars int
		synth time.Duration // measured, kokoro-onnx 0.5.0 on one core
		audio time.Duration
	}{
		{66, 1700 * time.Millisecond, 4460 * time.Millisecond},
		{218, 5740 * time.Millisecond, 14660 * time.Millisecond},
		{743, 14150 * time.Millisecond, 46950 * time.Millisecond},
		{2000, 52 * time.Second, 133 * time.Second}, // extrapolated, a very long region
	} {
		line := strings.Repeat("x", tc.chars)
		start, ceiling := waitBudget(line)
		if start <= tc.synth {
			t.Errorf("%d chars: grace %v does not cover %v of synthesis", tc.chars, start, tc.synth)
		}
		if ceiling <= tc.synth+tc.audio {
			t.Errorf("%d chars: ceiling %v cuts a wait that needs %v", tc.chars, ceiling, tc.synth+tc.audio)
		}
	}
	// And it must still be a bound, not an open wait: a dead engine on a short line
	// releases its waiter inside a few seconds.
	if start, _ := waitBudget("a short line."); start > 10*time.Second {
		t.Errorf("a short line waits %v on a dead engine", start)
	}
}
