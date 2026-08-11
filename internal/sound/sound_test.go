package sound

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// stubPlayer writes its argv to a file and sleeps, so tests can observe
// what was played and exercise the busy path.
func stubPlayer(t *testing.T, sleepMS int) (string, string) {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "calls.log")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\nsleep %f\n", logFile, float64(sleepMS)/1000)
	bin := filepath.Join(dir, "fakeplay")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, logFile
}

func newTestPlayer(t *testing.T, sleepMS int) (*Player, string) {
	t.Helper()
	bin, logFile := stubPlayer(t, sleepMS)
	p := New(discard(), t.TempDir(), func(string) (string, error) { return bin, nil })
	if !p.enabled {
		t.Fatal("player should be enabled with a resolvable binary")
	}
	return p, logFile
}

func calls(t *testing.T, logFile string) []string {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestExtractWritesAllEarcons(t *testing.T) {
	dir := t.TempDir()
	New(discard(), dir, func(string) (string, error) { return "", errors.New("none") })
	for _, c := range []Class{ClassInfo, ClassSuccess, ClassWarning, ClassQuestion, ClassError, ClassUrgent} {
		path := filepath.Join(dir, string(c)+".wav")
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s: %v", c, err)
		}
		if st.Size() < 1000 {
			t.Fatalf("%s implausibly small: %d bytes", c, st.Size())
		}
	}
}

func TestNoPlayerDisablesSoundWithoutPanic(t *testing.T) {
	p := New(discard(), t.TempDir(), func(string) (string, error) { return "", errors.New("none") })
	p.Play(ClassQuestion) // must be a no-op, not a panic
	p.Wait()
}

func TestPlayInvokesPlayerWithWavPath(t *testing.T) {
	p, logFile := newTestPlayer(t, 0)
	p.Play(ClassQuestion)
	p.Wait()
	got := calls(t, logFile)
	if len(got) != 1 || !strings.HasSuffix(got[0], "chime.wav") {
		t.Fatalf("calls = %v, want one ending in chime.wav", got)
	}
}

func TestBusySkipsLesserSound(t *testing.T) {
	p, logFile := newTestPlayer(t, 300)
	p.Play(ClassInfo)
	time.Sleep(50 * time.Millisecond)
	p.Play(ClassSuccess) // must be skipped: pop still playing
	p.Wait()
	got := calls(t, logFile)
	if len(got) != 1 {
		t.Fatalf("calls = %v, want only the first sound", got)
	}
}

func TestUrgentReplacesPlayingSound(t *testing.T) {
	p, logFile := newTestPlayer(t, 5000)
	p.Play(ClassInfo)
	time.Sleep(50 * time.Millisecond)
	p.Play(ClassUrgent) // must kill the 5 s sleeper and start immediately

	// The stub sleeps long for urgent too, so completion proves nothing;
	// preemption is proven by insist starting well before pop's 5 s end.
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := calls(t, logFile)
		if len(got) == 2 {
			if !strings.HasSuffix(got[1], "insist.wav") {
				t.Fatalf("calls = %v, want pop then insist", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("urgent never started; calls = %v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The exact argv per player. This exists because the volume knob was folded into
// playArgs when the two non-Linux players were added, and the three Linux dialects
// had to come out of that byte-identical: a chime that plays at the wrong level is
// not a thing anybody notices until it is the urgent one.
func TestPlayArgsSpeakEachPlayersDialect(t *testing.T) {
	const wav = "/run/agentbox/sounds/urgent.wav"
	for _, c := range []struct {
		bin  string
		want []string
	}{
		{"/usr/bin/pw-play", []string{"--volume=0.40", wav}},
		{"/usr/bin/paplay", []string{"--volume=26214", wav}},
		// No volume control at all, which is why it is last of the three.
		{"/usr/bin/aplay", []string{wav}},
		{"/usr/bin/afplay", []string{"-v", "0.40", wav}},
		// Unknown player: the file and nothing else, so a name somebody adds to the
		// list still plays instead of being handed a flag it does not know.
		{"/usr/bin/mystery", []string{wav}},
	} {
		got := playArgs(c.bin, 0.4, wav)
		if len(got) != len(c.want) {
			t.Errorf("%s: %v, want %v", c.bin, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: %v, want %v", c.bin, got, c.want)
				break
			}
		}
	}

	// Windows names the file inside a -Command script rather than as an argument,
	// so the assertion is that the path arrives at all and that the profile is off.
	ps := playArgs("C:\\Windows\\System32\\powershell.exe", 0.4, wav)
	joined := strings.Join(ps, " ")
	for _, want := range []string{"-NoProfile", "PlaySync", wav} {
		if !strings.Contains(joined, want) {
			t.Errorf("powershell args %v do not carry %q", ps, want)
		}
	}
}

func TestClassFor(t *testing.T) {
	cases := []struct {
		kind  proto.Kind
		level proto.Level
		want  Class
	}{
		{proto.KindNotify, proto.LevelInfo, ClassInfo},
		{proto.KindNotify, proto.LevelSuccess, ClassSuccess},
		{proto.KindNotify, proto.LevelWarning, ClassWarning},
		{proto.KindNotify, proto.LevelError, ClassError},
		{proto.KindNotify, proto.LevelUrgent, ClassUrgent},
		{proto.KindNotify, "", ClassInfo},
		{proto.KindChoice, proto.LevelInfo, ClassQuestion},
		{proto.KindConfirm, proto.LevelWarning, ClassQuestion},
		{proto.KindChoice, proto.LevelUrgent, ClassUrgent},
	}
	for _, tc := range cases {
		it := &proto.Item{Kind: tc.kind, Level: tc.level}
		if got := ClassFor(it); got != tc.want {
			t.Errorf("ClassFor(%s,%s) = %s, want %s", tc.kind, tc.level, got, tc.want)
		}
	}
}
