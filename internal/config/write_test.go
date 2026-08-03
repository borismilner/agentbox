package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readBack reads a file written by a test.
func readBack(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWriteReplacesExistingKey(t *testing.T) {
	p := write(t, "[sound]\nenabled = true\nvolume = 0.4\n")
	if err := Write(p, []Change{{Section: "sound", Key: "volume", Literal: FloatLiteral(0.6)}}); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, p)
	want := "[sound]\nenabled = true\nvolume = 0.6\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestWriteAppendsKeyToExistingSection(t *testing.T) {
	p := write(t, "[sound]\nenabled = true\n")
	if err := Write(p, []Change{{Section: "sound", Key: "quiet_hours", Literal: StringLiteral("22:00-08:00")}}); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, p)
	// Inserted directly under the header so it stays grouped.
	want := "[sound]\nquiet_hours = \"22:00-08:00\"\nenabled = true\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestWriteAppendsMissingSection(t *testing.T) {
	p := write(t, "[sound]\nenabled = true\n")
	if err := Write(p, []Change{{Section: "actions", Key: "enabled", Literal: BoolLiteral(false)}}); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, p)
	want := "[sound]\nenabled = true\n\n[actions]\nenabled = false\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestWritePreservesCommentsAndUntouchedKeys(t *testing.T) {
	src := "# my agentbox config\n" +
		"[sound]\n" +
		"enabled = true   # keep the chime\n" +
		"volume = 0.4      # quiet by default\n" +
		"\n" +
		"# escalation backs off after a while\n" +
		"[escalation]\n" +
		"count = 5\n"
	p := write(t, src)
	if err := Write(p, []Change{
		{Section: "sound", Key: "volume", Literal: FloatLiteral(0.7)},
		{Section: "escalation", Key: "count", Literal: IntLiteral(3)},
	}); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, p)
	want := "# my agentbox config\n" +
		"[sound]\n" +
		"enabled = true   # keep the chime\n" +
		"volume = 0.7      # quiet by default\n" +
		"\n" +
		"# escalation backs off after a while\n" +
		"[escalation]\n" +
		"count = 3\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

// A key that is a prefix of another in the same section must not be confused
// for it, and a key with the same name in a different section is independent.
func TestWriteKeyDisambiguation(t *testing.T) {
	src := "[escalation]\ninterval_s = 60\nurgent_interval_s = 20\n\n[veto]\ninterval_s = 99\n"
	p := write(t, src)
	if err := Write(p, []Change{{Section: "escalation", Key: "interval_s", Literal: IntLiteral(45)}}); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, p)
	want := "[escalation]\ninterval_s = 45\nurgent_interval_s = 20\n\n[veto]\ninterval_s = 99\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestWriteEmptyAndMissingFile(t *testing.T) {
	// Missing file.
	miss := filepath.Join(t.TempDir(), "config.toml")
	if err := Write(miss, []Change{{Section: "actions", Key: "enabled", Literal: BoolLiteral(false)}}); err != nil {
		t.Fatal(err)
	}
	if got, want := readBack(t, miss), "[actions]\nenabled = false\n"; got != want {
		t.Fatalf("missing file: got %q want %q", got, want)
	}
	// Empty file.
	empty := write(t, "")
	if err := Write(empty, []Change{{Section: "sound", Key: "volume", Literal: FloatLiteral(0.5)}}); err != nil {
		t.Fatal(err)
	}
	if got, want := readBack(t, empty), "[sound]\nvolume = 0.5\n"; got != want {
		t.Fatalf("empty file: got %q want %q", got, want)
	}
}

func TestWriteTwoKeysIntoOneNewSection(t *testing.T) {
	p := write(t, "[sound]\nenabled = true\n")
	if err := Write(p, []Change{
		{Section: "presence", Key: "idle_after_s", Literal: IntLiteral(90)},
		{Section: "presence", Key: "hold_when_idle", Literal: BoolLiteral(false)},
	}); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, p)
	// First change creates [presence] + idle_after_s; the second finds the
	// fresh section and inserts under its header.
	want := "[sound]\nenabled = true\n\n[presence]\nhold_when_idle = false\nidle_after_s = 90\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestLiteralFormatting(t *testing.T) {
	cases := []struct{ got, want string }{
		{BoolLiteral(true), "true"},
		{BoolLiteral(false), "false"},
		{IntLiteral(90), "90"},
		{IntLiteral(0), "0"},
		{FloatLiteral(0.6), "0.6"},
		{FloatLiteral(12), "12.0"}, // float field must not get an integer literal
		{FloatLiteral(0.4), "0.4"},
		{StringLiteral("22:00-08:00"), `"22:00-08:00"`},
		{StringLiteral(`a"b\c`), `"a\"b\\c"`},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q want %q", c.got, c.want)
		}
	}
}

// Write then Load returns the written values, comments survive, and no clamp
// warnings fire for in-range values.
func TestWriteLoadRoundTrip(t *testing.T) {
	src := "# hand-edited\n[sound]\nvolume = 0.4 # keep quiet\n"
	p := write(t, src)
	changes := []Change{
		{Section: "sound", Key: "volume", Literal: FloatLiteral(0.8)},
		{Section: "sound", Key: "enabled", Literal: BoolLiteral(false)},
		{Section: "font", Key: "size_pt", Literal: FloatLiteral(14)},
		{Section: "theme", Key: "mode", Literal: StringLiteral("dark")},
		{Section: "escalation", Key: "count", Literal: IntLiteral(3)},
	}
	if err := Write(p, changes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readBack(t, p), "# keep quiet") {
		t.Fatalf("inline comment lost:\n%s", readBack(t, p))
	}
	c, warns, err := Load(p)
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if c.Sound.Volume != 0.8 || c.Sound.Enabled || c.Font.SizePt != 14 ||
		c.Theme.Mode != "dark" || c.Escalation.Count != 3 {
		t.Fatalf("round-trip values wrong: %+v", c)
	}
}
