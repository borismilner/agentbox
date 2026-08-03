package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/config"
)

// tempConfig points config.Path() at a file this test owns and returns its path.
func tempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "agentbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agentbox", "config.toml")
	if body != "" {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	if got := config.Path(); got != path {
		t.Fatalf("config.Path() = %q, want %q", got, path)
	}
	return path
}

// Every knob must be wired at both ends. A knob the descriptor offers but
// valueOf cannot read would show a blank control; one setValue cannot write
// would silently never save. This is the test that catches a half-added knob.
func TestEveryKnobIsFullyWired(t *testing.T) {
	seen := map[string]bool{}
	def := config.Default()

	for _, k := range allKnobs() {
		id := k.id()
		if seen[id] {
			t.Errorf("%s: declared twice", id)
		}
		seen[id] = true

		if k.label == "" || k.kind == "" {
			t.Errorf("%s: needs a label and a kind", id)
		}
		if k.kind == knobEnum && len(k.enum) == 0 {
			t.Errorf("%s: enum knob with no values", id)
		}
		if (k.kind == knobInt || k.kind == knobFloat) && k.max == 0 {
			t.Errorf("%s: numeric knob with no upper bound", id)
		}

		// valueOf must know the id: an unknown one returns "", and no knob's
		// default is empty except the deliberately-empty text fields.
		got := valueOf(def, k)
		if got == "" && k.kind != knobText && k.kind != knobColor {
			t.Errorf("%s: valueOf returned nothing for the default config", id)
		}

		// setValue must round-trip whatever valueOf produced.
		var c config.Config
		if err := setValue(&c, k, got); err != nil {
			t.Errorf("%s: setValue(%q) = %v", id, got, err)
			continue
		}
		if back := valueOf(c, k); back != got {
			t.Errorf("%s: round-trip gave %q, want %q", id, back, got)
		}
	}

	// The knobs the descriptor claims need a restart are exactly the ones the
	// daemon reads once at startup. Theme and font are NOT among them any more -
	// that is the point of the token push.
	wantRestart := map[string]bool{
		"dnd.start_in_dnd": true, "history.retention_days": true, "history.keep_level": true,
		"log.level": true, "log.retention_mb": true,
	}
	for _, k := range allKnobs() {
		if k.restart != wantRestart[k.id()] {
			t.Errorf("%s: restart = %v, want %v", k.id(), k.restart, wantRestart[k.id()])
		}
	}
}

func knobByID(t *testing.T, id string) knob {
	t.Helper()
	for _, k := range allKnobs() {
		if k.id() == id {
			return k
		}
	}
	t.Fatalf("no knob %s", id)
	return knob{}
}

func TestParseKnobClampsNumbers(t *testing.T) {
	tests := []struct {
		id      string
		in      string
		literal string
	}{
		// A slider that refuses its own extreme is worse than one that stops.
		{"theme.radius", "99", "24"},
		{"theme.radius", "-4", "0"},
		{"sound.volume", "5", "1.0"},
		{"sound.volume", "-1", "0.0"},
		{"ask.undo_grace_s", "9", "5"},
		{"font.size_pt", "99", "36.0"},
		{"font.size_pt", "13.5", "13.5"},
		{"escalation.interval_s", "90", "90"},
	}
	for _, tc := range tests {
		k := knobByID(t, tc.id)
		lit, _, err := parseKnob(k, tc.in)
		if err != nil {
			t.Errorf("%s(%q): %v", tc.id, tc.in, err)
			continue
		}
		if lit != tc.literal {
			t.Errorf("%s(%q) literal = %q, want %q", tc.id, tc.in, lit, tc.literal)
		}
	}
}

func TestParseKnobRefusesValuesThatCannotMeanAnything(t *testing.T) {
	tests := []struct {
		id, in, wantIn string
	}{
		{"theme.mode", "sepia", "not one of"},
		{"markdown.code_theme", "solarized", "not one of"},
		{"history.keep_level", "chatty", "not one of"},
		{"theme.accent", "teal", "#rrggbb"},
		{"theme.accent", "#12345", "#rrggbb"},
		{"sound.quiet_hours", "9pm", "HH:MM-HH:MM"},
		{"sound.quiet_hours", "22:00", "HH:MM-HH:MM"},
		{"theme.radius", "round", "not a whole number"},
		{"sound.volume", "loud", "not a number"},
	}
	for _, tc := range tests {
		k := knobByID(t, tc.id)
		if _, _, err := parseKnob(k, tc.in); err == nil {
			t.Errorf("%s(%q) was accepted", tc.id, tc.in)
		} else if !strings.Contains(err.Error(), tc.wantIn) {
			t.Errorf("%s(%q) error = %q, want it to mention %q", tc.id, tc.in, err, tc.wantIn)
		}
	}

	// Empty is how you clear a colour or a font, and a valid window still passes.
	for _, ok := range []struct{ id, in string }{
		{"theme.accent", ""},
		{"theme.accent", "#46B3A5"},
		{"sound.quiet_hours", ""},
		{"sound.quiet_hours", "22:00-07:30"},
		{"font.family", "Inter"},
	} {
		if _, _, err := parseKnob(knobByID(t, ok.id), ok.in); err != nil {
			t.Errorf("%s(%q) refused: %v", ok.id, ok.in, err)
		}
	}

	// A hex colour is stored lowercase so the string comparison against the file
	// does not see a change every time the picker hands back uppercase.
	if _, norm, _ := parseKnob(knobByID(t, "theme.accent"), "#46B3A5"); norm != "#46b3a5" {
		t.Errorf("accent normalised to %q, want lowercase", norm)
	}
}

func TestSettingsReadsTheFileNotTheDefaults(t *testing.T) {
	tempConfig(t, `
# hand written
[sound]
volume = 0.55

[theme]
mode = "light"
ground = "slate"
`)
	u := testUI(&fakeResolver{}, nil)
	got := u.settings()

	if len(got.Sections) == 0 {
		t.Fatal("no sections")
	}
	find := func(id string) wireKnob {
		for _, s := range got.Sections {
			for _, g := range s.Groups {
				for _, k := range g.Knobs {
					if k.ID == id {
						return k
					}
				}
			}
		}
		t.Fatalf("no knob %s on the wire", id)
		return wireKnob{}
	}
	if v := find("sound.volume").Value; v != "0.55" {
		t.Errorf("sound.volume = %q, want 0.55 from the file", v)
	}
	if v := find("theme.ground").Value; v != "slate" {
		t.Errorf("theme.ground = %q, want slate", v)
	}
	// The default travels too, so the surface can show what a reset would mean.
	if d := find("sound.volume").Default; d != "0.4" {
		t.Errorf("sound.volume default = %q, want 0.4", d)
	}
	if k := find("history.keep_level"); !k.Restart || len(k.Enum) == 0 {
		t.Errorf("keep_level should be a restart-tagged enum, got %+v", k)
	}
	if got.Path == "" {
		t.Error("the surface needs the path it is editing")
	}
}

func TestSettingsSurfacesLoaderWarnings(t *testing.T) {
	tempConfig(t, "[theme]\nmode = \"sepia\"\n[sound]\nvolume = 9\n")
	u := testUI(&fakeResolver{}, nil)
	got := u.settings()
	if len(got.Warnings) < 2 {
		t.Fatalf("warnings = %v, want the mode and the volume flagged", got.Warnings)
	}
	// A rejected value must not reach the control as-is.
	for _, s := range got.Sections {
		for _, g := range s.Groups {
			for _, k := range g.Knobs {
				if k.ID == "theme.mode" && k.Value != "auto" {
					t.Errorf("theme.mode = %q, want the default after the warning", k.Value)
				}
			}
		}
	}
}

func TestSaveWritesOnlyWhatChanged(t *testing.T) {
	body := `# agentbox - hand edited, keep these comments
[sound]
enabled = true
volume = 0.55              # louder in the office

[escalation]
interval_s = 90            # I answer slowly
`
	path := tempConfig(t, body)
	u := testUI(&fakeResolver{}, nil)

	// Everything the surface holds goes back, changed or not - the point is that
	// only the differences are written.
	vals := map[string]string{}
	for _, s := range u.settings().Sections {
		for _, g := range s.Groups {
			for _, k := range g.Knobs {
				vals[k.ID] = k.Value
			}
		}
	}
	vals["theme.contrast"] = "high"
	vals["theme.radius"] = "22"

	res := u.saveSettings(vals)
	if res.Err != "" {
		t.Fatalf("save: %s", res.Err)
	}
	if len(res.Written) != 2 {
		t.Fatalf("wrote %v, want just the two changed keys", res.Written)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"# agentbox - hand edited, keep these comments",
		"# louder in the office",
		"# I answer slowly",
		"volume = 0.55",
		"interval_s = 90",
		`contrast = "high"`,
		"radius = 22",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("file lost %q:\n%s", want, text)
		}
	}
	// No default may be materialised: nothing the user did not touch appears.
	for _, unwanted := range []string{"keep_level", "undo_grace_s", "hold_when_idle", "retention_mb"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("file grew an untouched key %q:\n%s", unwanted, text)
		}
	}

	// Saving the same values again writes nothing: the baseline is the file, so
	// a second Save has nothing to say.
	again := u.saveSettings(vals)
	if len(again.Written) != 0 || again.Err != "" {
		t.Errorf("second save wrote %v (err %q), want nothing", again.Written, again.Err)
	}
	if !strings.Contains(again.Note, "Nothing to write") {
		t.Errorf("note = %q, want it to say there was nothing to do", again.Note)
	}
}

// A typo in one field must not swallow a good change made in the same visit.
func TestSaveRefusesOneKeyAndKeepsTheRest(t *testing.T) {
	path := tempConfig(t, "[theme]\nmode = \"dark\"\n")
	u := testUI(&fakeResolver{}, nil)

	res := u.saveSettings(map[string]string{
		"sound.quiet_hours": "9pm",   // refused
		"theme.ground":      "slate", // written
	})
	if len(res.Written) != 1 || !strings.Contains(res.Written[0], "ground") {
		t.Fatalf("written = %v, want the ground change", res.Written)
	}
	if !strings.Contains(res.Err, "HH:MM-HH:MM") {
		t.Errorf("err = %q, want the quiet-hours reason", res.Err)
	}
	out, _ := os.ReadFile(path)
	if strings.Contains(string(out), "quiet_hours") {
		t.Errorf("the refused key was written anyway:\n%s", out)
	}
	if !strings.Contains(string(out), `ground = "slate"`) {
		t.Errorf("the accepted key was not written:\n%s", out)
	}
}

func TestSaveReportsARestartWhenOneIsNeeded(t *testing.T) {
	tempConfig(t, "")
	u := testUI(&fakeResolver{}, nil)

	res := u.saveSettings(map[string]string{"log.level": "debug"})
	if !res.Restart || !strings.Contains(res.Note, "next daemon start") {
		t.Errorf("log.level is read at startup; result = %+v", res)
	}

	res = u.saveSettings(map[string]string{"theme.ground": "ink"})
	if res.Restart {
		t.Errorf("the theme applies live now; result = %+v", res)
	}
}

func TestSaveIgnoresKnobsItDoesNotKnow(t *testing.T) {
	tempConfig(t, "")
	u := testUI(&fakeResolver{}, nil)
	res := u.saveSettings(map[string]string{"theme.hue_rotation": "42", "sound.enabled": "false"})
	if len(res.Written) != 1 || !strings.Contains(res.Written[0], "sound.enabled") {
		t.Fatalf("written = %v, want only the known knob", res.Written)
	}
}

func TestPreviewThemeResolvesPendingValuesWithoutWriting(t *testing.T) {
	path := tempConfig(t, "[theme]\nmode = \"dark\"\nground = \"graphite\"\n")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	u := testUI(&fakeResolver{}, nil)

	plain := u.previewTheme(map[string]string{"theme.mode": "dark"})
	lifted := u.previewTheme(map[string]string{
		"theme.mode": "dark", "theme.contrast": "high",
		"theme.accent": "#46b3a5", "theme.radius": "22",
		"markdown.code_theme": "gruvbox",
	})

	if lifted.Ink2 == plain.Ink2 || lifted.Ink3 == plain.Ink3 || lifted.Edge == plain.Edge {
		t.Error("high contrast should lift the muted inks and the hairlines")
	}
	if lifted.Ground != plain.Ground {
		t.Error("contrast must not move the ground")
	}
	if lifted.Accent != "#46b3a5" {
		t.Errorf("accent = %q", lifted.Accent)
	}
	if lifted.Radius != "22px" {
		t.Errorf("radius = %q", lifted.Radius)
	}
	if lifted.CodeKeyword == plain.CodeKeyword {
		t.Error("a code theme should change the syntax colours")
	}
	// A half-typed value is not worth refusing a repaint over.
	if got := u.previewTheme(map[string]string{"theme.accent": "#4"}).Accent; got == "#4" {
		t.Error("an invalid accent should be skipped, not applied")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Error("previewing wrote to the file")
	}
}

func TestBuildThemeCodeThemesAndContrast(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Mode = "dark"

	auto := BuildTheme(cfg)
	if auto.CodeComment != auto.Ink3 {
		t.Errorf("the default code theme should borrow the ground's muted ink, got %q", auto.CodeComment)
	}

	// The richer roles (2026-07-28): explicit where the palette names them,
	// sibling or ground fallbacks where it does not.
	if auto.CodeOp != auto.Ink2 || auto.CodePunct != auto.Ink3 {
		t.Errorf("auto op/punct should borrow ink2/ink3, got %q/%q", auto.CodeOp, auto.CodePunct)
	}
	if auto.CodeType == "" || auto.CodeConst == "" || auto.CodeAttr == "" || auto.CodeEsc == "" {
		t.Errorf("auto left a role blank: %+v", auto)
	}

	cfg.Markdown.CodeTheme = "nord"
	nord := BuildTheme(cfg)
	if nord.CodeKeyword != "#81a1c1" || nord.CodeComment != "#616e88" {
		t.Errorf("nord = %+v", nord)
	}
	if nord.CodeType != "#8fbcbb" || nord.CodeOp != "#81a1c1" {
		t.Errorf("nord type/op = %q/%q, want the palette's own values", nord.CodeType, nord.CodeOp)
	}

	cfg.Markdown.CodeTheme = "onedark"
	if od := BuildTheme(cfg); od.CodeKeyword != "#c678dd" || od.CodeType != "#e5c07b" {
		t.Errorf("onedark = %q/%q", od.CodeKeyword, od.CodeType)
	}

	// An unknown theme falls back rather than rendering colourless code.
	cfg.Markdown.CodeTheme = "nonsense"
	if fallback := BuildTheme(cfg); fallback.CodeKeyword != auto.CodeKeyword {
		t.Errorf("unknown code theme = %q, want the auto palette", fallback.CodeKeyword)
	}

	cfg.Markdown.CodeTheme = "auto"
	cfg.Theme.Contrast = "high"
	high := BuildTheme(cfg)
	if high.Ink2 == auto.Ink2 {
		t.Error("high contrast did nothing")
	}
	// Lifting means moving toward the ink, not past it.
	if high.Ink2 == high.Ink {
		t.Error("high contrast should not collapse ink2 onto ink")
	}
}

func TestMixHex(t *testing.T) {
	tests := []struct {
		a, b string
		amt  float64
		want string
	}{
		{"#000000", "#ffffff", 0.5, "#808080"},
		{"#000000", "#ffffff", 0, "#000000"},
		{"#000000", "#ffffff", 1, "#ffffff"},
		{"#98a0ad", "#e5e8ee", 0, "#98a0ad"},
		{"not-a-colour", "#ffffff", 0.5, "not-a-colour"}, // unparseable passes through
	}
	for _, tc := range tests {
		if got := mixHex(tc.a, tc.b, tc.amt); got != tc.want {
			t.Errorf("mixHex(%q,%q,%v) = %q, want %q", tc.a, tc.b, tc.amt, got, tc.want)
		}
	}
	if _, _, _, ok := parseHex("#ggghhh"); ok {
		t.Error("parseHex accepted non-hex digits")
	}
	if r, g, b, ok := parseHex("#46b3a5"); !ok || r != 0x46 || g != 0xb3 || b != 0xa5 {
		t.Errorf("parseHex = %d,%d,%d,%v", r, g, b, ok)
	}
}

// The artifact policy travels with the theme, because that is the channel every
// open surface already listens on for live config (tokens.go).
func TestThemeCarriesTheArtifactPolicy(t *testing.T) {
	cfg := config.Default()
	on := BuildTheme(cfg)
	if !on.ArtifactsEnabled || on.ArtifactMax != "640px" {
		t.Fatalf("default policy = %v %q", on.ArtifactsEnabled, on.ArtifactMax)
	}

	cfg.Artifact.Enabled = false
	cfg.Artifact.MaxHeightPx = 300
	off := BuildTheme(cfg)
	if off.ArtifactsEnabled || off.ArtifactMax != "300px" {
		t.Fatalf("configured policy = %v %q", off.ArtifactsEnabled, off.ArtifactMax)
	}

	// A Config that never went through Load (a test, a caller building tokens by
	// hand) must not emit "0px" and collapse the frame to nothing.
	if bare := BuildTheme(config.Config{}); bare.ArtifactMax != "640px" {
		t.Errorf("zero config = %q, want the default height", bare.ArtifactMax)
	}
}

// Both artifact knobs must be reachable from the settings surface, and its
// getter and setter must agree - a knob whose value cannot round-trip is a
// control that silently does nothing.
func TestArtifactKnobsRoundTrip(t *testing.T) {
	want := map[string]string{"artifact.enabled": "false", "artifact.max_height_px": "420"}
	cfg := config.Default()
	cfg.Artifact.Enabled = false
	cfg.Artifact.MaxHeightPx = 420

	found := 0
	for _, k := range allKnobs() {
		expected, ours := want[k.id()]
		if !ours {
			continue
		}
		found++
		if got := valueOf(cfg, k); got != expected {
			t.Errorf("%s reads back %q, want %q", k.id(), got, expected)
		}
		// And the setter must accept what the getter produced.
		back := config.Default()
		if err := setValue(&back, k, expected); err != nil {
			t.Errorf("%s: %v", k.id(), err)
		}
		if got := valueOf(back, k); got != expected {
			t.Errorf("%s did not round-trip: %q", k.id(), got)
		}
	}
	if found != len(want) {
		t.Errorf("found %d artifact knobs in the descriptor table, want %d", found, len(want))
	}
}
