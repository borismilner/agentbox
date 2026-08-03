package webui

import (
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/borismilner/agentbox/internal/config"
)

// Theme is the token set every surface reads. It is JSON because it crosses
// into the webview, and it is flat because the frontend writes each field
// straight onto a CSS custom property (frontend/src/lib/tokens.js).
//
// The point of the whole arrangement: a theme change is a variable write, not
// a widget rebuild, so `mode = "dark"` in config.toml applies within the two
// seconds config.Watch takes to notice - no restart, unlike the Gio build.
type Theme struct {
	Mode    string `json:"mode"` // dark | light, already resolved from auto
	Density string `json:"density"`
	Motion  string `json:"motion"`

	Ground   string `json:"ground"`
	Surface  string `json:"surface"`
	Surface2 string `json:"surface2"`
	Surface3 string `json:"surface3"`
	Edge     string `json:"edge"`
	EdgeSoft string `json:"edgeSoft"`
	Ink      string `json:"ink"`
	Ink2     string `json:"ink2"`
	Ink3     string `json:"ink3"`

	Info    string `json:"info"`
	Success string `json:"success"`
	Warning string `json:"warning"`
	Error   string `json:"error"`
	Urgent  string `json:"urgent"`
	Accent  string `json:"accent"`

	CodeKeyword string `json:"codeKeyword"`
	CodeString  string `json:"codeString"`
	CodeNumber  string `json:"codeNumber"`
	CodeFunc    string `json:"codeFunc"`
	CodeComment string `json:"codeComment"`
	CodeType    string `json:"codeType"`
	CodeConst   string `json:"codeConst"`
	CodeAttr    string `json:"codeAttr"`
	CodeEsc     string `json:"codeEsc"`
	CodeOp      string `json:"codeOp"`
	CodePunct   string `json:"codePunct"`

	FontUI   string `json:"fontUI"`
	FontRead string `json:"fontRead"`
	FontMono string `json:"fontMono"`
	Size     string `json:"size"`
	Radius   string `json:"radius"`
	// Measure is the reading column ([window] measure_px), and PanelMeasure the
	// wider one the drop-down panel uses. They are tokens rather than CSS
	// constants so a change to either arrives with the rest of the theme, live.
	Measure      string `json:"measure"`
	PanelMeasure string `json:"panelMeasure"`
	Pad          string `json:"pad"`
	Gap          string `json:"gap"`

	// The artifact policy ([artifact]) rides the theme push because that is the
	// channel every open surface already listens on for live config: turn
	// artifacts off and a conversation stops running them within the watcher's
	// poll, without a second event to wire up. ArtifactsEnabled is the trust
	// switch and is enforced where the iframe would be created, so "off" means
	// nothing runs rather than something runs invisibly (artifact.go).
	ArtifactsEnabled bool   `json:"artifactsEnabled"`
	ArtifactMax      string `json:"artifactMax"`
}

// grounds are hand-tuned dark/light pairs. Users pick a ground and an accent,
// not twenty individual surface colours: two knobs that cannot produce an
// unreadable theme beat twenty that can.
var grounds = map[string][2]Theme{
	"graphite": {
		{Ground: "#0f1116", Surface: "#161920", Surface2: "#1c2028", Surface3: "#22262f",
			Edge: "#272c36", EdgeSoft: "#1f242c", Ink: "#e5e8ee", Ink2: "#98a0ad", Ink3: "#69717e"},
		{Ground: "#edeff3", Surface: "#ffffff", Surface2: "#f5f6f9", Surface3: "#edf0f4",
			Edge: "#dce0e7", EdgeSoft: "#e7eaf0", Ink: "#171a20", Ink2: "#555d6a", Ink3: "#828a97"},
	},
	"ink": {
		{Ground: "#08090c", Surface: "#101217", Surface2: "#15181e", Surface3: "#1b1f26",
			Edge: "#22262e", EdgeSoft: "#181b21", Ink: "#e8eaef", Ink2: "#959ca8", Ink3: "#666d78"},
		{Ground: "#f6f6f7", Surface: "#ffffff", Surface2: "#f1f2f4", Surface3: "#e9ebee",
			Edge: "#d9dce1", EdgeSoft: "#e6e8ec", Ink: "#101216", Ink2: "#4e545e", Ink3: "#7c838e"},
	},
	"slate": {
		{Ground: "#111820", Surface: "#18212b", Surface2: "#1d2833", Surface3: "#233040",
			Edge: "#2a3846", EdgeSoft: "#1f2b37", Ink: "#e2e9f0", Ink2: "#94a3b2", Ink3: "#65737f"},
		{Ground: "#eaeef2", Surface: "#ffffff", Surface2: "#f2f5f8", Surface3: "#e8edf2",
			Edge: "#d5dde5", EdgeSoft: "#e3e9ef", Ink: "#141c24", Ink2: "#4f5d6a", Ink3: "#7b8895"},
	},
}

// codePalette is one syntax colour set. An empty field falls back in
// BuildTheme - comment to the ground's muted ink, typ/cons/attr/esc to their
// nearest sibling role, op and punct to the ground's ink2/ink3 - which is what
// makes a five-colour palette still render the full role set, and the default
// theme feel part of the ground rather than pasted onto it.
type codePalette struct {
	keyword, str, number, fn, comment string
	// The richer roles (2026-07-28): types and classes, language constants and
	// builtins, tags/attributes/decorators, escapes and interpolation inside
	// strings, operators, punctuation.
	typ, cons, attr, esc, op, punct string
}

// codeThemes are dark/light pairs, chosen so a user who reads code in one
// editor palette all day can have agentbox agree with it. `auto` is agentbox's own,
// derived from the ground. Values follow each upstream theme's own role
// assignments where they exist; light pairs for themes with no official light
// variant (dracula) are hand-darkened to hold AA contrast on white.
var codeThemes = map[string][2]codePalette{
	"auto": {
		{keyword: "#93a4e8", str: "#8fbf9f", number: "#d9a441", fn: "#7fb3d5",
			typ: "#63b8ab", cons: "#b995dd", attr: "#c99578", esc: "#e3b968"},
		{keyword: "#4453a8", str: "#35704a", number: "#8a6212", fn: "#2c6a94",
			typ: "#22766c", cons: "#6d4a9e", attr: "#935a36", esc: "#a85f22"},
	},
	"nord": {
		{keyword: "#81a1c1", str: "#a3be8c", number: "#b48ead", fn: "#88c0d0", comment: "#616e88",
			typ: "#8fbcbb", cons: "#81a1c1", attr: "#d08770", esc: "#ebcb8b", op: "#81a1c1"},
		{keyword: "#5e81ac", str: "#4f7a3f", number: "#8a5f7a", fn: "#3b7f96", comment: "#7b879b",
			typ: "#3f8079", cons: "#5e81ac", attr: "#a35d3d", esc: "#8a6d1c", op: "#5e81ac"},
	},
	"gruvbox": {
		{keyword: "#fb4934", str: "#b8bb26", number: "#d3869b", fn: "#8ec07c", comment: "#928374",
			typ: "#fabd2f", cons: "#d3869b", attr: "#fe8019", esc: "#fe8019"},
		{keyword: "#9d0006", str: "#79740e", number: "#8f3f71", fn: "#427b58", comment: "#928374",
			typ: "#b57614", cons: "#8f3f71", attr: "#af3a03", esc: "#af3a03"},
	},
	"github": {
		{keyword: "#ff7b72", str: "#a5d6ff", number: "#79c0ff", fn: "#d2a8ff", comment: "#8b949e",
			typ: "#7ee787", cons: "#79c0ff", attr: "#7ee787", esc: "#79c0ff"},
		{keyword: "#cf222e", str: "#0a3069", number: "#0550ae", fn: "#8250df", comment: "#6e7781",
			typ: "#116329", cons: "#0550ae", attr: "#116329", esc: "#0550ae"},
	},
	"onedark": {
		{keyword: "#c678dd", str: "#98c379", number: "#d19a66", fn: "#61afef", comment: "#7f848e",
			typ: "#e5c07b", cons: "#d19a66", attr: "#e06c75", esc: "#56b6c2"},
		{keyword: "#a626a4", str: "#50a14f", number: "#986801", fn: "#4078f2", comment: "#8a8c93",
			typ: "#c18401", cons: "#986801", attr: "#e45649", esc: "#0184bc"},
	},
	"dracula": {
		{keyword: "#ff79c6", str: "#f1fa8c", number: "#bd93f9", fn: "#50fa7b", comment: "#6272a4",
			typ: "#8be9fd", cons: "#bd93f9", attr: "#50fa7b", esc: "#ff79c6", op: "#ff79c6"},
		{keyword: "#b0367e", str: "#7d7106", number: "#7c4dcc", fn: "#1f9e4c", comment: "#6272a4",
			typ: "#0e7f96", cons: "#7c4dcc", attr: "#1f9e4c", esc: "#b0367e", op: "#b0367e"},
	},
}

// mixHex blends two #rrggbb colours; amount is how far to travel from a to b.
// Used for the high-contrast lift, which is a nudge along an existing pair
// rather than a second hand-tuned palette to maintain.
func mixHex(a, b string, amount float64) string {
	ar, ag, ab, ok1 := parseHex(a)
	br, bg, bb, ok2 := parseHex(b)
	if !ok1 || !ok2 {
		return a
	}
	lerp := func(x, y int) int { return int(float64(x) + (float64(y)-float64(x))*amount + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", lerp(ar, br), lerp(ag, bg), lerp(ab, bb))
}

func parseHex(s string) (r, g, b int, ok bool) {
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0, false
	}
	var v int64
	for i := 1; i < 7; i++ {
		d := hexDigit(s[i])
		if d < 0 {
			return 0, 0, 0, false
		}
		v = v<<4 | int64(d)
	}
	return int(v >> 16), int(v>>8) & 0xff, int(v) & 0xff, true
}

func hexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// BuildTheme resolves config into tokens. Unknown values fall back rather
// than fail: a typo in config.toml must never leave the user without a UI.
func BuildTheme(c config.Config) Theme {
	dark := true
	switch c.Theme.Mode {
	case "light":
		dark = false
	case "dark":
		dark = true
	default:
		dark = systemPrefersDark()
	}

	pair, ok := grounds[c.Theme.Ground]
	if !ok {
		pair = grounds["graphite"]
	}
	t := pair[0]
	if !dark {
		t = pair[1]
	}

	t.Mode = "dark"
	if !dark {
		t.Mode = "light"
	}
	t.Density = or(c.Theme.Density, "comfortable")
	t.Motion = or(c.Theme.Motion, "full")

	// High contrast pulls the recessive tokens toward the ink rather than
	// swapping in a second palette: the muted greys and the hairlines are the
	// only things that go too quiet on a dim screen, and one ground stays one
	// ground.
	if c.Theme.Contrast == "high" {
		t.Ink2 = mixHex(t.Ink2, t.Ink, 0.45)
		t.Ink3 = mixHex(t.Ink3, t.Ink, 0.35)
		t.Edge = mixHex(t.Edge, t.Ink, 0.22)
	}

	if dark {
		t.Info, t.Success, t.Warning, t.Error = "#4fa3e3", "#4fb286", "#d9a441", "#e05c5c"
	} else {
		t.Info, t.Success, t.Warning, t.Error = "#2c7fbf", "#2e8b62", "#a9761b", "#c4444b"
	}
	t.Urgent = t.Error

	code := codeThemes[c.Markdown.CodeTheme]
	if code == [2]codePalette{} {
		code = codeThemes["auto"]
	}
	cp := code[0]
	if !dark {
		cp = code[1]
	}
	t.CodeKeyword, t.CodeString, t.CodeNumber, t.CodeFunc = cp.keyword, cp.str, cp.number, cp.fn
	t.CodeComment = or(cp.comment, t.Ink3)
	// The richer roles fall back to their nearest sibling, so a palette that
	// names only the classic five still colours every role sensibly.
	t.CodeType = or(cp.typ, cp.fn)
	t.CodeConst = or(cp.cons, cp.number)
	t.CodeAttr = or(cp.attr, cp.keyword)
	t.CodeEsc = or(cp.esc, cp.number)
	t.CodeOp = or(cp.op, t.Ink2)
	t.CodePunct = or(cp.punct, t.Ink3)

	t.Accent = or(c.Theme.Accent, map[bool]string{true: "#7c8cf8", false: "#5566e0"}[dark])

	t.FontUI = or(c.Font.Family, `"Cantarell", "Ubuntu Sans", system-ui, sans-serif`)
	t.FontRead = or(c.Font.Reading, `"Bitstream Charter", Charter, Georgia, serif`)
	t.FontMono = or(c.Font.Mono, `"JetBrains Mono", "Fira Code", ui-monospace, monospace`)

	size := c.Font.SizePt
	if size < config.FontSizeMin || size > config.FontSizeMax {
		size = 12
	}
	// Precedence: env beats config for quick experiments; config is the durable
	// setting (06-configuration.md). The factor is relative to 12pt, which is
	// what AGENTBOX_FONT_SCALE meant before the web UI.
	if s := os.Getenv("AGENTBOX_FONT_SCALE"); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 0.5 && f <= 3 {
			size = 12 * f
		}
	}
	t.Size = fmt.Sprintf("%.2fpx", size*96.0/72.0)

	// A zero here means a Config that never went through Load (a test, a caller
	// building tokens by hand); fall back rather than emit "0px" and collapse every
	// column to nothing.
	t.Measure = fmt.Sprintf("%dpx", orInt(c.Window.MeasurePx, config.Default().Window.MeasurePx))
	t.PanelMeasure = fmt.Sprintf("%dpx", orInt(c.Panel.MeasurePx, config.Default().Panel.MeasurePx))
	t.ArtifactsEnabled = c.Artifact.Enabled
	t.ArtifactMax = fmt.Sprintf("%dpx", orInt(c.Artifact.MaxHeightPx, config.Default().Artifact.MaxHeightPx))

	radius := c.Theme.Radius
	if radius < 0 || radius > 24 {
		radius = 10
	}
	t.Radius = fmt.Sprintf("%dpx", radius)

	scale := size * 96.0 / 72.0
	if t.Density == "compact" {
		t.Pad = fmt.Sprintf("%.1fpx", scale*0.95)
		t.Gap = fmt.Sprintf("%.1fpx", scale*1.0)
	} else {
		t.Pad = fmt.Sprintf("%.1fpx", scale*1.28)
		t.Gap = fmt.Sprintf("%.1fpx", scale*1.45)
	}
	return t
}

func orInt(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func or(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// systemPrefersDark asks GNOME. Anything unreadable or missing means dark,
// which is the safer default for a tool that pops windows at night.
func systemPrefersDark() bool {
	if v := os.Getenv("AGENTBOX_THEME"); v != "" {
		return v != "light"
	}
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err != nil {
		return true
	}
	return !strings.Contains(string(out), "prefer-light")
}

// IdentityHue mirrors frontend/src/lib/tokens.js exactly, so an agent is the
// same colour in a card, in a session turn and in the tray. Reds are excluded
// so no agent can dress itself up as an error.
func IdentityHue(agent, project string, dark bool) string {
	h := fnv.New32a()
	fmt.Fprintf(h, "%s %s", agent, project)
	stops := []int{30, 55, 80, 105, 135, 160, 185, 205, 225, 250, 275, 300}
	hue := stops[int(h.Sum32())%len(stops)]
	if dark {
		return fmt.Sprintf("hsl(%d 62%% 68%%)", hue)
	}
	return fmt.Sprintf("hsl(%d 58%% 42%%)", hue)
}
