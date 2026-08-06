package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestArgvSurvivesTheTripThroughAOneLineBox(t *testing.T) {
	// The whole reason these settings are arrays and not strings: a path with a
	// space in it is one argument, and a control that edits them as a line has to
	// give that back unbroken.
	cases := []struct {
		line string
		want []string
	}{
		{"", nil},
		{"piper", []string{"piper"}},
		{"nvim +{line} {file}", []string{"nvim", "+{line}", "{file}"}},
		{`code --goto "{file}:{line}:{column}"`, []string{"code", "--goto", "{file}:{line}:{column}"}},
		{`"/opt/My Editor/bin/ed" {file}`, []string{"/opt/My Editor/bin/ed", "{file}"}},
		{"  spaced   out  ", []string{"spaced", "out"}},
		{`say 'it'\''s fine'`, []string{"say", "it's fine"}},
	}
	for _, c := range cases {
		got := SplitArgv(c.line)
		if len(got) != len(c.want) {
			t.Fatalf("SplitArgv(%q) = %q, want %q", c.line, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("SplitArgv(%q) = %q, want %q", c.line, got, c.want)
			}
		}
		// Join then split must land on the same argv, or a knob nobody touched
		// writes itself back on every visit to the settings surface.
		if again := SplitArgv(JoinArgv(got)); len(again) != len(got) {
			t.Fatalf("round trip of %q gave %q", c.line, again)
		} else {
			for i := range again {
				if again[i] != got[i] {
					t.Fatalf("round trip of %q gave %q, want %q", c.line, again, got)
				}
			}
		}
	}
}

// TestArgvKeepsWhatTheFuzzerCaught pins the two defects FuzzArgvSurvivesTheBox
// found on its first run, because a corpus file under testdata is a hash and
// says nothing about what it is guarding.
func TestArgvKeepsWhatTheFuzzerCaught(t *testing.T) {
	// A tab or a newline inside a quoted argument used to come back as the letter
	// t or n: JoinArgv borrowed StringLiteral, which spells them the TOML way,
	// and SplitArgv read a backslash the shell way (\X means X).
	for _, line := range []string{"\"a b\tc\"", "\"a b\nc\"", "\"a b\rc\""} {
		argv := SplitArgv(line)
		if len(argv) != 1 {
			t.Fatalf("SplitArgv(%q) = %q, want one argument", line, argv)
		}
		if back := SplitArgv(JoinArgv(argv)); len(back) != 1 || back[0] != argv[0] {
			t.Fatalf("%q round-tripped %q into %q", line, argv, back)
		}
	}

	// A control character used to be written into the file raw, which TOML
	// rejects - so Load abandoned the whole file and every unrelated knob
	// reverted to its default. One pasted DEL was enough.
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Write(path, []Change{
		{Section: "sound", Key: "volume", Literal: FloatLiteral(0.4)},
		{Section: "editor", Key: "command", Literal: ArgvLiteral([]string{"ed", "\x7f\x01"})},
	}); err != nil {
		t.Fatal(err)
	}
	c, warns, err := Load(path)
	if err != nil || len(warns) != 0 {
		t.Fatalf("a control character made the file unreadable: err=%v warns=%v", err, warns)
	}
	if len(c.Editor.Command) != 2 || c.Editor.Command[1] != "\x7f\x01" {
		t.Fatalf("editor.command read back as %q", c.Editor.Command)
	}
	if c.Sound.Volume != 0.4 {
		t.Fatalf("an unrelated knob reverted: volume=%v", c.Sound.Volume)
	}
}

// FuzzArgvSurvivesTheBox drives the exact path a command knob takes on the
// settings surface: the typed line is split, the argv is rendered back into the
// box by JoinArgv and into the file by ArgvLiteral, and the next visit reads
// both again. Two properties have to hold or a knob nobody touched rewrites
// itself, or worse, quietly changes meaning:
//
//   - split(join(argv)) == argv - the box round-trips.
//   - Load(ArgvLiteral(argv)) == argv - the file round-trips.
//
// Splitting twice is the honest formulation: only argvs SplitArgv can produce
// are ever handed to Join, so the fuzzer explores lines rather than arrays.
func FuzzArgvSurvivesTheBox(f *testing.F) {
	for _, seed := range []string{
		"",
		"piper",
		"nvim +{line} {file}",
		`code --goto "{file}:{line}:{column}"`,
		`"/opt/My Editor/bin/ed" {file}`,
		"  spaced   out  ",
		`say 'it'\''s fine'`,
		`a\\b`,
		`""`,
		`a"b`,
		"tab\tseparated",
		`quote"unclosed`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, line string) {
		argv := SplitArgv(line)
		if !utf8.ValidString(line) {
			// SplitArgv ranges over runes, so invalid bytes become U+FFFD and the
			// line is not what was typed in the first place. Nothing to promise.
			return
		}
		sameArgv := func(what string, got []string) {
			t.Helper()
			if len(got) != len(argv) {
				t.Fatalf("%s of %q: %q, want %q", what, line, got, argv)
			}
			for i := range got {
				if got[i] != argv[i] {
					t.Fatalf("%s of %q: %q, want %q", what, line, got, argv)
				}
			}
		}
		sameArgv("box round trip", SplitArgv(JoinArgv(argv)))

		path := filepath.Join(t.TempDir(), "config.toml")
		if err := Write(path, []Change{
			{Section: "editor", Key: "command", Literal: ArgvLiteral(argv)},
		}); err != nil {
			t.Fatalf("write %q: %v", argv, err)
		}
		c, _, err := Load(path)
		if err != nil {
			t.Fatalf("load after writing %q (%s): %v", argv, ArgvLiteral(argv), err)
		}
		sameArgv("file round trip", c.Editor.Command)
	})
}

func TestArgvLiteralIsWhatTheFileSpells(t *testing.T) {
	if got := ArgvLiteral(nil); got != "[]" {
		t.Fatalf("empty argv literal = %q, want []", got)
	}
	got := ArgvLiteral([]string{"code", "--goto", `a "b" c`})
	want := `["code", "--goto", "a \"b\" c"]`
	if got != want {
		t.Fatalf("literal = %s, want %s", got, want)
	}
}

func TestWritingAnArgvKeyReadsBackAsAnArray(t *testing.T) {
	// The end-to-end the settings control depends on: what Write puts in the file
	// has to come back through Load as the same argv, or the box shows one thing
	// and the daemon runs another.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := Write(path, []Change{
		{Section: "editor", Key: "command", Literal: ArgvLiteral([]string{"/opt/My Editor/ed", "--line", "{line}", "{file}"})},
	}); err != nil {
		t.Fatal(err)
	}
	c, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Editor.Command) != 4 || c.Editor.Command[0] != "/opt/My Editor/ed" || c.Editor.Command[3] != "{file}" {
		t.Fatalf("read back %q", c.Editor.Command)
	}
}
