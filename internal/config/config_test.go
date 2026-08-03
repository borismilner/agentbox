package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMissingFileIsDefaults(t *testing.T) {
	c, warns, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	d := Default()
	// reflect, not ==: Config holds a []string since [speech] command is an argv.
	if !reflect.DeepEqual(c, d) {
		t.Fatalf("missing file != defaults:\n%+v\n%+v", c, d)
	}
	if !c.Sound.Enabled || c.Sound.Volume != 0.4 || c.Ask.UndoGraceS != 3 || c.History.KeepLevel != "warning" {
		t.Fatalf("defaults wrong: %+v", c)
	}
}

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPartialFileOverridesOnlyWhatItNames(t *testing.T) {
	p := write(t, "[sound]\nvolume = 0.8\n\n[escalation]\ncount = 2\n")
	c, warns, err := Load(p)
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if c.Sound.Volume != 0.8 || c.Escalation.Count != 2 {
		t.Fatalf("overrides not applied: %+v", c)
	}
	if c.Escalation.IntervalS != 60 || c.Toast.DurationS != 6 {
		t.Fatalf("defaults lost: %+v", c)
	}
}

func TestUnknownKeysWarnButLoad(t *testing.T) {
	p := write(t, "[sound]\nvolume = 0.7\nbass_boost = true\n")
	c, warns, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "bass_boost") {
		t.Fatalf("warns = %v", warns)
	}
	if c.Sound.Volume != 0.7 {
		t.Fatal("valid keys must still apply")
	}
}

func TestOutOfRangeValuesWarnAndFallBack(t *testing.T) {
	p := write(t, "[sound]\nvolume = 3.0\n\n[font]\nsize_pt = 200\n\n[ask]\nundo_grace_s = 30\n")
	c, warns, _ := Load(p)
	if len(warns) != 3 {
		t.Fatalf("warns = %v", warns)
	}
	if c.Sound.Volume != 0.4 || c.Font.SizePt != 12 {
		t.Fatalf("fallbacks not applied: %+v", c)
	}
	if c.Ask.UndoGraceS != 3 {
		t.Fatalf("undo grace not clamped: %d (must stay 3-5 s tops)", c.Ask.UndoGraceS)
	}
}

func TestBrokenFileFallsBackToDefaults(t *testing.T) {
	p := write(t, "this is not toml [[[")
	c, _, err := Load(p)
	if err == nil {
		t.Fatal("expected a warning error for broken toml")
	}
	if !reflect.DeepEqual(c, Default()) {
		t.Fatal("broken file must yield pure defaults")
	}
}

func TestQuietHours(t *testing.T) {
	var c Config
	c.Sound.QuietHours = "22:30-08:00"
	at := func(h, m int) time.Time {
		return time.Date(2026, 6, 12, h, m, 0, 0, time.Local)
	}
	for _, tc := range []struct {
		h, m int
		want bool
	}{
		{23, 0, true}, {3, 0, true}, {7, 59, true},
		{8, 0, false}, {12, 0, false}, {22, 29, false}, {22, 30, true},
	} {
		if got := c.InQuietHours(at(tc.h, tc.m)); got != tc.want {
			t.Errorf("%02d:%02d = %v, want %v", tc.h, tc.m, got, tc.want)
		}
	}
	c.Sound.QuietHours = ""
	if c.InQuietHours(at(3, 0)) {
		t.Error("empty window must never be quiet")
	}
}

func TestWatchFiresOnChange(t *testing.T) {
	p := write(t, "[sound]\nvolume = 0.5\n")
	got := make(chan Config, 1)
	stop := Watch(p, 30*time.Millisecond, func(c Config, _ []string) { got <- c })
	defer stop()

	time.Sleep(60 * time.Millisecond)
	// mtime granularity can be coarse; force it forward.
	if err := os.WriteFile(p, []byte("[sound]\nvolume = 0.9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(p, future, future)

	select {
	case c := <-got:
		if c.Sound.Volume != 0.9 {
			t.Fatalf("reloaded volume = %v", c.Sound.Volume)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch never fired")
	}
}

func TestArtifactKnobs(t *testing.T) {
	d := Default()
	if !d.Artifact.Enabled || d.Artifact.MaxHeightPx != 640 {
		t.Fatalf("artifacts should be on by default, bounded: %+v", d.Artifact)
	}

	// The trust switch is a real switch: an explicit false stays false.
	c, warns, err := Load(write(t, "[artifact]\nenabled = false\n"))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if c.Artifact.Enabled {
		t.Error("artifact.enabled = false was not applied")
	}
	if c.Artifact.MaxHeightPx != d.Artifact.MaxHeightPx {
		t.Errorf("naming one key should not disturb the other: %+v", c.Artifact)
	}

	// And a nonsense height is a warning plus the default, like every other pixel.
	c, warns, err = Load(write(t, "[artifact]\nmax_height_px = 40000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "artifact.max_height_px") {
		t.Fatalf("warns = %v", warns)
	}
	if c.Artifact.MaxHeightPx != d.Artifact.MaxHeightPx {
		t.Errorf("max_height_px = %d, want the default", c.Artifact.MaxHeightPx)
	}
}
