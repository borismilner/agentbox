package webui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/borismilner/agentbox/internal/config"
)

// The settings surface (M9 slice 4). Two things make it worth its own file.
//
// First, it is descriptor-driven: one table says what a knob is called, what
// control it deserves, what values it accepts and whether it takes effect at
// once. The surface renders the table; it does not know what a knob means. Add a
// config key, add a row here, and the panel grows a control.
//
// Second, Save is surgical. The user's config.toml is a file they also edit by
// hand, so agentbox writes only the keys whose value actually changed
// (config.Write edits the line in place and keeps comments), never a
// regenerated file and never a materialised default.

// knob kinds. Values cross the wire as strings whatever the kind, so there is
// one parse point (parseKnob) instead of a union type in JSON.
const (
	knobBool  = "bool"
	knobInt   = "int"
	knobFloat = "float"
	knobEnum  = "enum"
	knobText  = "text"
	knobColor = "color"
)

type knob struct {
	section, key string
	label        string
	hint         string
	kind         string
	min, max     float64
	step         float64
	unit         string
	enum         []string
	// suggest offers known values without forbidding others - a font the user
	// has installed is theirs to name, and no list here could be complete.
	suggest  []string
	swatches []swatch
	// restart marks a knob the daemon only reads at startup. Everything else
	// reloads within ~2s via config.Watch, and the theme applies at once.
	restart bool
}

func (k knob) id() string { return k.section + "." + k.key }

type swatch struct {
	Hex  string `json:"hex"`
	Name string `json:"name"`
}

type knobGroup struct {
	title   string
	caption string
	knobs   []knob
}

type knobSection struct {
	id      string
	title   string
	blurb   string
	preview bool // Appearance is the one section with a live preview
	groups  []knobGroup
}

// accents are the five that survived a pass over agentbox's surfaces: each one is
// legible as a focus ring on every ground, and none of them reads as a severity.
var accents = []swatch{
	{"#7c8cf8", "Periwinkle"},
	{"#46b3a5", "Teal"},
	{"#d99a5b", "Amber"},
	{"#b07bd4", "Orchid"},
	{"#8fa3b8", "Steel"},
}

// settingsSpec is the whole surface. Appearance follows the design reviewed with
// the owner; the behaviour sections carry every knob the Gio settings tab
// edited, so the cutover to this UI loses nothing.
//
// ask.allow_reply is deliberately absent: the global flag is unwired, so a
// control for it would lie.
var settingsSpec = []knobSection{
	{
		id: "appearance", title: "Appearance", preview: true,
		blurb: "Ground and accent are the only colour knobs. Two knobs that cannot " +
			"produce an unreadable theme beat twenty that can - and severity and " +
			"identity hues are semantics, not decoration, so they are not here.",
		groups: []knobGroup{
			{title: "Theme", knobs: []knob{
				{section: "theme", key: "mode", label: "Mode", kind: knobEnum, enum: config.ThemeModes,
					hint: "auto follows the desktop"},
				{section: "theme", key: "ground", label: "Ground", kind: knobEnum, enum: config.ThemeGrounds},
				{section: "theme", key: "contrast", label: "Contrast", kind: knobEnum, enum: config.ThemeContrasts,
					hint: "high lifts the muted inks and the hairlines"},
			}},
			{title: "Accent", caption: "focus rings, links, primary action", knobs: []knob{
				{section: "theme", key: "accent", label: "Accent", kind: knobColor, swatches: accents},
			}},
			{title: "Density & shape", knobs: []knob{
				{section: "theme", key: "density", label: "Density", kind: knobEnum, enum: config.ThemeDensities},
				{section: "theme", key: "radius", label: "Corner radius", kind: knobInt,
					min: config.RadiusMin, max: config.RadiusMax, step: 1, unit: "px"},
			}},
			{title: "Type", knobs: []knob{
				{section: "font", key: "family", label: "Interface", kind: knobText,
					hint:    "empty = Cantarell, then the system",
					suggest: []string{"Cantarell", "Inter", "Noto Sans", "DejaVu Sans", "Ubuntu Sans"}},
				{section: "font", key: "reading", label: "Agent prose", kind: knobText,
					hint:    "empty = the bundled serif",
					suggest: []string{"Bitstream Charter", "Charter", "Noto Serif", "Source Serif 4", "Georgia"}},
				{section: "font", key: "mono", label: "Code", kind: knobText,
					suggest: []string{"JetBrains Mono", "Fira Code", "Source Code Pro", "DejaVu Sans Mono"}},
				{section: "font", key: "size_pt", label: "Base size", kind: knobFloat,
					min: config.FontSizeMin, max: config.FontSizeMax, step: 0.5, unit: "pt",
					hint: "every other size is relative to it"},
			}},
			{title: "Code & motion", knobs: []knob{
				{section: "markdown", key: "code_theme", label: "Code theme", kind: knobEnum, enum: config.CodeThemes,
					hint: "auto derives from the ground"},
				{section: "theme", key: "motion", label: "Motion", kind: knobEnum, enum: config.ThemeMotions},
			}},
		},
	},
	{
		id: "windows", title: "Windows & panel",
		blurb: "How big AgentBox's windows are, how wide it lets a line of prose run, and " +
			"the drop-down panel. All of it applies while you watch - the open windows " +
			"resize, so tune it against something real rather than from memory.",
		groups: []knobGroup{
			{title: "Reading", caption: "the measure caps a line of prose however wide the window is", knobs: []knob{
				{section: "window", key: "measure_px", label: "Reading measure", kind: knobInt,
					min: 320, max: 2400, step: 20, unit: "px",
					hint: "the app window and the reader"},
				{section: "panel", key: "measure_px", label: "Panel measure", kind: knobInt,
					min: 320, max: 3000, step: 20, unit: "px",
					hint: "the panel is wider, so its column can be too"},
			}},
			{title: "Drop-down panel", caption: "a hotkey rolls a session down from the top edge", knobs: []knob{
				{section: "panel", key: "hotkey", label: "Hotkey", kind: knobText,
					hint:    "grabbed by the daemon; empty = no grab, use `agentbox panel`",
					suggest: []string{"Ctrl+Alt+grave", "Super+grave", "Super+K", "Alt+F1"}},
				{section: "panel", key: "width_frac", label: "Width", kind: knobFloat,
					min: 0.3, max: 1, step: 0.02, unit: "× screen"},
				{section: "panel", key: "height_frac", label: "Height", kind: knobFloat,
					min: 0.2, max: 1, step: 0.02, unit: "× screen"},
				{section: "panel", key: "slide_ms", label: "Roll duration", kind: knobInt,
					min: 0, max: 1000, step: 10, unit: "ms", hint: "0 = no animation"},
			}},
			{title: "Cards and toasts", knobs: []knob{
				{section: "window", key: "card_width", label: "Card width", kind: knobInt,
					min: 240, max: 2000, step: 10, unit: "px"},
				{section: "window", key: "card_max_height", label: "Card max height", kind: knobInt,
					min: 200, max: 4000, step: 20, unit: "px", hint: "past this the body scrolls inside the card"},
				{section: "window", key: "toast_width", label: "Toast width", kind: knobInt,
					min: 240, max: 2000, step: 10, unit: "px"},
				{section: "window", key: "toast_max_height", label: "Toast max height", kind: knobInt,
					min: 60, max: 2000, step: 10, unit: "px"},
				{section: "window", key: "toast_top_inset", label: "Toast top inset", kind: knobInt,
					min: 0, max: 2000, step: 4, unit: "px", hint: "how far below the top edge a toast sits"},
			}},
			{title: "App, reader and progress", knobs: []knob{
				{section: "window", key: "app_width", label: "App width", kind: knobInt,
					min: 640, max: 6000, step: 20, unit: "px"},
				{section: "window", key: "app_height", label: "App height", kind: knobInt,
					min: 400, max: 4000, step: 20, unit: "px"},
				{section: "window", key: "viewer_width", label: "Reader width", kind: knobInt,
					min: 400, max: 6000, step: 20, unit: "px"},
				{section: "window", key: "viewer_height", label: "Reader height", kind: knobInt,
					min: 300, max: 4000, step: 20, unit: "px"},
				{section: "window", key: "progress_width", label: "Progress width", kind: knobInt,
					min: 240, max: 2000, step: 10, unit: "px"},
				{section: "window", key: "progress_max_height", label: "Progress max height", kind: knobInt,
					min: 80, max: 3000, step: 20, unit: "px"},
			}},
		},
	},
	{
		id: "sessions", title: "Sessions",
		blurb: "The Claude child AgentBox spawns for a session (FR49).",
		groups: []knobGroup{
			{title: "New sessions", knobs: []knob{
				{section: "session", key: "default_mode", label: "Permission mode", kind: knobEnum,
					enum: config.SessionModes,
					hint: "plan is read-only; the prompting modes are not offered yet"},
				{section: "session", key: "binary", label: "claude binary", kind: knobText,
					hint: "empty = `claude` on the daemon's PATH"},
				{section: "session", key: "dir", label: "Working directory", kind: knobText,
					hint: "empty = the daemon's own directory"},
				{section: "session", key: "show_cost", label: "Show the cost of each reply", kind: knobBool,
					hint: "off by default: interesting once, noise every turn"},
			}},
		},
	},
	{
		id: "sound", title: "Sound",
		blurb: "One chime per class, played through pw-play. Quiet hours silence the " +
			"sound, never the card.",
		groups: []knobGroup{
			{title: "Sound", knobs: []knob{
				{section: "sound", key: "enabled", label: "Earcons", kind: knobBool},
				{section: "sound", key: "volume", label: "Volume", kind: knobFloat,
					min: config.VolumeMin, max: config.VolumeMax, step: 0.05},
				{section: "sound", key: "quiet_hours", label: "Quiet hours", kind: knobText,
					hint: "HH:MM-HH:MM, empty for none"},
			}},
		},
	},
	{
		id: "interruptions", title: "Interruptions",
		blurb: "How hard AgentBox pushes for an answer, and how long it holds one back.",
		groups: []knobGroup{
			{title: "Escalation", caption: "replaying the earcon for an unanswered item", knobs: []knob{
				{section: "escalation", key: "interval_s", label: "Replay interval", kind: knobInt,
					min: 0, max: 600, step: 5, unit: "s", hint: "0 never replays"},
				{section: "escalation", key: "count", label: "Replay cap", kind: knobInt, min: 0, max: 20, step: 1},
				{section: "escalation", key: "urgent_interval_s", label: "Urgent interval", kind: knobInt,
					min: 0, max: 300, step: 5, unit: "s"},
			}},
			{title: "Timing", knobs: []knob{
				{section: "toast", key: "duration_s", label: "Toast auto-dismiss", kind: knobInt,
					min: 0, max: 60, step: 1, unit: "s"},
				{section: "ask", key: "undo_grace_s", label: "Undo grace", kind: knobInt,
					min: config.UndoGraceMin, max: config.UndoGraceMax, step: 1, unit: "s",
					hint: "how long an answer is held before it is sent"},
				{section: "veto", key: "default_window_s", label: "Default veto window", kind: knobInt,
					min: 1, max: 300, step: 1, unit: "s", hint: "when the caller names no window"},
			}},
			{title: "What an agent may run", caption: "two kill switches: a command behind a button, and interactive HTML in a window", knobs: []knob{
				{section: "actions", key: "enabled", label: "Action buttons", kind: knobBool,
					hint: "the kill switch for caller-supplied commands (FR32)"},
				{section: "artifact", key: "enabled", label: "Interactive artifacts", kind: knobBool,
					hint: "agent HTML runs sandboxed: no network, no reach into agentbox. Off leaves the source"},
				{section: "artifact", key: "max_height_px", label: "Artifact max height", kind: knobInt,
					min: 120, max: 4000, step: 20, unit: "px",
					hint: "how tall one may grow inside a conversation"},
			}},
		},
	},
	{
		id: "presence", title: "Presence & DND",
		blurb: "Nothing should chime into an empty room, and nothing should chime over " +
			"a presentation.",
		groups: []knobGroup{
			{title: "Presence", knobs: []knob{
				{section: "presence", key: "hold_when_idle", label: "Hold chimes while idle", kind: knobBool,
					hint: "the card still shows; one summary chime on return"},
				{section: "presence", key: "idle_after_s", label: "Idle after", kind: knobInt,
					min: 0, max: 1800, step: 10, unit: "s", hint: "0 disables the idle signal"},
				{section: "presence", key: "fullscreen_auto_dnd", label: "Fullscreen means DND", kind: knobBool},
				{section: "presence", key: "respect_desktop_dnd", label: "Follow the desktop's DND", kind: knobBool},
			}},
			{title: "Do not disturb", knobs: []knob{
				{section: "dnd", key: "urgent_breaks_through", label: "Urgent breaks through", kind: knobBool},
				{section: "dnd", key: "start_in_dnd", label: "Start in DND", kind: knobBool, restart: true},
			}},
		},
	},
	{
		id: "history", title: "History & logs",
		blurb: "What AgentBox keeps after the question is answered.",
		groups: []knobGroup{
			{title: "History", knobs: []knob{
				{section: "history", key: "retention_days", label: "Retention", kind: knobInt,
					min: 0, max: 365, step: 1, unit: "days", restart: true},
				{section: "history", key: "keep_level", label: "Keep level", kind: knobEnum,
					enum: config.KeepLevels, restart: true,
					hint: "this level and above is kept forever"},
			}},
			{title: "Log", knobs: []knob{
				{section: "log", key: "level", label: "Level", kind: knobEnum, enum: config.LogLevels, restart: true},
				{section: "log", key: "retention_mb", label: "Retention", kind: knobInt,
					min: 1, max: 1000, step: 10, unit: "MB", restart: true},
			}},
		},
	},
}

// --- the wire -------------------------------------------------------------

type wireKnob struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Hint    string   `json:"hint,omitempty"`
	Kind    string   `json:"kind"`
	Value   string   `json:"value"`
	Default string   `json:"default"`
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	Step    float64  `json:"step,omitempty"`
	Unit    string   `json:"unit,omitempty"`
	Enum    []string `json:"enum,omitempty"`
	Suggest []string `json:"suggest,omitempty"`
	Swatch  []swatch `json:"swatches,omitempty"`
	Restart bool     `json:"restart,omitempty"`
}

type wireKnobGroup struct {
	Title   string     `json:"title"`
	Caption string     `json:"caption,omitempty"`
	Knobs   []wireKnob `json:"knobs"`
}

type wireSection struct {
	ID      string          `json:"id"`
	Title   string          `json:"title"`
	Blurb   string          `json:"blurb,omitempty"`
	Preview bool            `json:"preview,omitempty"`
	Groups  []wireKnobGroup `json:"groups"`
}

type wireSettings struct {
	Path     string        `json:"path"`
	Sections []wireSection `json:"sections"`
	Warnings []string      `json:"warnings,omitempty"`
	Err      string        `json:"err,omitempty"`
}

// wireSaved is what the surface shows after Save: exactly which lines went into
// the file, so the claim "only what you changed" is checkable rather than
// promised.
type wireSaved struct {
	Written []string `json:"written"`
	Note    string   `json:"note"`
	Err     string   `json:"err,omitempty"`
	Restart bool     `json:"restart,omitempty"`
}

// settings reads the config file fresh each time the surface opens. The daemon's
// in-memory config is not the baseline: the file is, because the file is what
// Save edits and what the user may have changed by hand since.
func (u *UI) settings() wireSettings {
	path := config.Path()
	cfg, warnings, err := config.Load(path)
	out := wireSettings{Path: path, Warnings: warnings}
	if err != nil {
		out.Err = err.Error()
	}
	def := config.Default()
	for _, sec := range settingsSpec {
		ws := wireSection{ID: sec.id, Title: sec.title, Blurb: sec.blurb, Preview: sec.preview}
		for _, g := range sec.groups {
			wg := wireKnobGroup{Title: g.title, Caption: g.caption}
			for _, k := range g.knobs {
				wg.Knobs = append(wg.Knobs, wireKnob{
					ID: k.id(), Label: k.label, Hint: k.hint, Kind: k.kind,
					Value: valueOf(cfg, k), Default: valueOf(def, k),
					Min: k.min, Max: k.max, Step: k.step, Unit: k.unit,
					Enum: k.enum, Suggest: k.suggest, Swatch: k.swatches,
					Restart: k.restart,
				})
			}
			ws.Groups = append(ws.Groups, wg)
		}
		out.Sections = append(out.Sections, ws)
	}
	return out
}

// previewTheme resolves the tokens a set of pending values would produce,
// without writing anything. The preview is the real resolver rather than a
// palette table copied into JavaScript, so what you see cannot drift from what
// Save gives you.
func (u *UI) previewTheme(vals map[string]string) Theme {
	cfg, _, err := config.Load(config.Path())
	if err != nil {
		cfg = config.Default()
	}
	for _, k := range allKnobs() {
		v, ok := vals[k.id()]
		if !ok {
			continue
		}
		if err := setValue(&cfg, k, v); err != nil {
			continue // a half-typed hex is not worth refusing a repaint over
		}
	}
	return BuildTheme(cfg)
}

// saveSettings writes the changed keys and nothing else. A rejected value stops
// its own key, never the rest: it would be perverse for a typo in quiet_hours to
// swallow a theme change made in the same visit.
func (u *UI) saveSettings(vals map[string]string) wireSaved {
	path := config.Path()
	base, _, err := config.Load(path)
	if err != nil {
		return wireSaved{Err: "config unreadable: " + err.Error()}
	}

	var changes []config.Change
	var written, refused []string
	restart := false

	for _, k := range allKnobs() {
		v, ok := vals[k.id()]
		if !ok {
			continue
		}
		lit, norm, err := parseKnob(k, v)
		if err != nil {
			refused = append(refused, err.Error())
			continue
		}
		if norm == valueOf(base, k) {
			continue
		}
		changes = append(changes, config.Change{Section: k.section, Key: k.key, Literal: lit})
		written = append(written, fmt.Sprintf("%s.%s = %s", k.section, k.key, lit))
		restart = restart || k.restart
	}

	out := wireSaved{Written: written, Restart: restart}
	if len(changes) == 0 {
		out.Note = "Nothing to write - every value is already what the file says."
		if len(refused) > 0 {
			out.Note = ""
			out.Err = strings.Join(refused, " · ")
		}
		return out
	}
	if err := config.Write(path, changes); err != nil {
		u.log.Error("webui.settings_save_failed", "component", "webui", "err", err.Error())
		return wireSaved{Err: "write failed: " + err.Error()}
	}
	u.log.Info("webui.settings_saved", "component", "webui", "keys", len(changes))

	// Re-theme now rather than waiting on config.Watch. The watcher will fire
	// too and land on the same tokens; this is so the click feels like it did
	// something.
	if cfg, _, err := config.Load(path); err == nil {
		u.SetTheme(cfg)
	}

	out.Note = fmt.Sprintf("Wrote %s to %s.", plural(len(changes), "key", "keys"), path)
	if restart {
		out.Note += " One of them applies on the next daemon start."
	}
	if len(refused) > 0 {
		out.Err = strings.Join(refused, " · ")
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func allKnobs() []knob {
	var out []knob
	for _, sec := range settingsSpec {
		for _, g := range sec.groups {
			out = append(out, g.knobs...)
		}
	}
	return out
}

// --- values ---------------------------------------------------------------

// valueOf and setValue are the two places that know a knob id maps to a struct
// field. Written out rather than reflected: the table is short, the compiler
// checks it, and a mistyped tag cannot silently stop writing a key.
func valueOf(c config.Config, k knob) string {
	switch k.id() {
	case "sound.enabled":
		return boolStr(c.Sound.Enabled)
	case "sound.volume":
		return floatStr(c.Sound.Volume)
	case "sound.quiet_hours":
		return c.Sound.QuietHours
	case "escalation.interval_s":
		return strconv.Itoa(c.Escalation.IntervalS)
	case "escalation.count":
		return strconv.Itoa(c.Escalation.Count)
	case "escalation.urgent_interval_s":
		return strconv.Itoa(c.Escalation.UrgentIntervalS)
	case "toast.duration_s":
		return strconv.Itoa(c.Toast.DurationS)
	case "ask.undo_grace_s":
		return strconv.Itoa(c.Ask.UndoGraceS)
	case "veto.default_window_s":
		return strconv.Itoa(c.Veto.DefaultWindowS)
	case "presence.hold_when_idle":
		return boolStr(c.Presence.HoldWhenIdle)
	case "presence.idle_after_s":
		return strconv.Itoa(c.Presence.IdleAfterS)
	case "presence.fullscreen_auto_dnd":
		return boolStr(c.Presence.FullscreenAutoDnd)
	case "presence.respect_desktop_dnd":
		return boolStr(c.Presence.RespectDesktopDnd)
	case "actions.enabled":
		return boolStr(c.Actions.Enabled)
	case "artifact.enabled":
		return boolStr(c.Artifact.Enabled)
	case "artifact.max_height_px":
		return strconv.Itoa(c.Artifact.MaxHeightPx)
	case "theme.mode":
		return c.Theme.Mode
	case "theme.ground":
		return c.Theme.Ground
	case "theme.contrast":
		return c.Theme.Contrast
	case "theme.accent":
		return c.Theme.Accent
	case "theme.density":
		return c.Theme.Density
	case "theme.radius":
		return strconv.Itoa(c.Theme.Radius)
	case "theme.motion":
		return c.Theme.Motion
	case "markdown.code_theme":
		return c.Markdown.CodeTheme
	case "window.measure_px":
		return strconv.Itoa(c.Window.MeasurePx)
	case "window.card_width":
		return strconv.Itoa(c.Window.CardWidth)
	case "window.card_max_height":
		return strconv.Itoa(c.Window.CardMaxHeight)
	case "window.toast_width":
		return strconv.Itoa(c.Window.ToastWidth)
	case "window.toast_max_height":
		return strconv.Itoa(c.Window.ToastMaxHeight)
	case "window.toast_top_inset":
		return strconv.Itoa(c.Window.ToastTopInset)
	case "window.app_width":
		return strconv.Itoa(c.Window.AppWidth)
	case "window.app_height":
		return strconv.Itoa(c.Window.AppHeight)
	case "window.viewer_width":
		return strconv.Itoa(c.Window.ViewerWidth)
	case "window.viewer_height":
		return strconv.Itoa(c.Window.ViewerHeight)
	case "window.progress_width":
		return strconv.Itoa(c.Window.ProgressWidth)
	case "window.progress_max_height":
		return strconv.Itoa(c.Window.ProgressMaxHeight)
	case "panel.hotkey":
		return c.Panel.Hotkey
	case "panel.width_frac":
		return floatStr(c.Panel.WidthFrac)
	case "panel.height_frac":
		return floatStr(c.Panel.HeightFrac)
	case "panel.slide_ms":
		return strconv.Itoa(c.Panel.SlideMS)
	case "panel.measure_px":
		return strconv.Itoa(c.Panel.MeasurePx)
	case "session.default_mode":
		return c.Session.DefaultMode
	case "session.binary":
		return c.Session.Binary
	case "session.dir":
		return c.Session.Dir
	case "session.show_cost":
		return boolStr(c.Session.ShowCost)
	case "font.size_pt":
		return floatStr(c.Font.SizePt)
	case "font.family":
		return c.Font.Family
	case "font.reading":
		return c.Font.Reading
	case "font.mono":
		return c.Font.Mono
	case "dnd.urgent_breaks_through":
		return boolStr(c.Dnd.UrgentBreaksThrough)
	case "dnd.start_in_dnd":
		return boolStr(c.Dnd.StartInDnd)
	case "history.retention_days":
		return strconv.Itoa(c.History.RetentionDays)
	case "history.keep_level":
		return c.History.KeepLevel
	case "log.level":
		return c.Log.Level
	case "log.retention_mb":
		return strconv.Itoa(c.Log.RetentionMB)
	}
	return ""
}

func setValue(c *config.Config, k knob, v string) error {
	_, norm, err := parseKnob(k, v)
	if err != nil {
		return err
	}
	b := norm == "true"
	i, _ := strconv.Atoi(norm)
	f, _ := strconv.ParseFloat(norm, 64)

	switch k.id() {
	case "sound.enabled":
		c.Sound.Enabled = b
	case "sound.volume":
		c.Sound.Volume = f
	case "sound.quiet_hours":
		c.Sound.QuietHours = norm
	case "escalation.interval_s":
		c.Escalation.IntervalS = i
	case "escalation.count":
		c.Escalation.Count = i
	case "escalation.urgent_interval_s":
		c.Escalation.UrgentIntervalS = i
	case "toast.duration_s":
		c.Toast.DurationS = i
	case "ask.undo_grace_s":
		c.Ask.UndoGraceS = i
	case "veto.default_window_s":
		c.Veto.DefaultWindowS = i
	case "presence.hold_when_idle":
		c.Presence.HoldWhenIdle = b
	case "presence.idle_after_s":
		c.Presence.IdleAfterS = i
	case "presence.fullscreen_auto_dnd":
		c.Presence.FullscreenAutoDnd = b
	case "presence.respect_desktop_dnd":
		c.Presence.RespectDesktopDnd = b
	case "actions.enabled":
		c.Actions.Enabled = b
	case "artifact.enabled":
		c.Artifact.Enabled = b
	case "artifact.max_height_px":
		c.Artifact.MaxHeightPx = i
	case "theme.mode":
		c.Theme.Mode = norm
	case "theme.ground":
		c.Theme.Ground = norm
	case "theme.contrast":
		c.Theme.Contrast = norm
	case "theme.accent":
		c.Theme.Accent = norm
	case "theme.density":
		c.Theme.Density = norm
	case "theme.radius":
		c.Theme.Radius = i
	case "theme.motion":
		c.Theme.Motion = norm
	case "markdown.code_theme":
		c.Markdown.CodeTheme = norm
	case "window.measure_px":
		c.Window.MeasurePx = i
	case "window.card_width":
		c.Window.CardWidth = i
	case "window.card_max_height":
		c.Window.CardMaxHeight = i
	case "window.toast_width":
		c.Window.ToastWidth = i
	case "window.toast_max_height":
		c.Window.ToastMaxHeight = i
	case "window.toast_top_inset":
		c.Window.ToastTopInset = i
	case "window.app_width":
		c.Window.AppWidth = i
	case "window.app_height":
		c.Window.AppHeight = i
	case "window.viewer_width":
		c.Window.ViewerWidth = i
	case "window.viewer_height":
		c.Window.ViewerHeight = i
	case "window.progress_width":
		c.Window.ProgressWidth = i
	case "window.progress_max_height":
		c.Window.ProgressMaxHeight = i
	case "panel.hotkey":
		c.Panel.Hotkey = norm
	case "panel.width_frac":
		c.Panel.WidthFrac = f
	case "panel.height_frac":
		c.Panel.HeightFrac = f
	case "panel.slide_ms":
		c.Panel.SlideMS = i
	case "panel.measure_px":
		c.Panel.MeasurePx = i
	case "session.default_mode":
		c.Session.DefaultMode = norm
	case "session.binary":
		c.Session.Binary = norm
	case "session.dir":
		c.Session.Dir = norm
	case "session.show_cost":
		c.Session.ShowCost = b
	case "font.size_pt":
		c.Font.SizePt = f
	case "font.family":
		c.Font.Family = norm
	case "font.reading":
		c.Font.Reading = norm
	case "font.mono":
		c.Font.Mono = norm
	case "dnd.urgent_breaks_through":
		c.Dnd.UrgentBreaksThrough = b
	case "dnd.start_in_dnd":
		c.Dnd.StartInDnd = b
	case "history.retention_days":
		c.History.RetentionDays = i
	case "history.keep_level":
		c.History.KeepLevel = norm
	case "log.level":
		c.Log.Level = norm
	case "log.retention_mb":
		c.Log.RetentionMB = i
	default:
		return fmt.Errorf("%s: unknown knob", k.id())
	}
	return nil
}

// parseKnob validates a submitted value and returns both the TOML literal to
// write and the normalised string to compare against the file. Out-of-range
// numbers clamp (a slider that refuses its own extreme is a worse experience
// than one that stops), but a value that cannot mean anything - a bad enum, a
// malformed hex, an unparseable quiet-hours window - is refused with a reason.
func parseKnob(k knob, v string) (literal, norm string, err error) {
	v = strings.TrimSpace(v)
	switch k.kind {
	case knobBool:
		b := v == "true"
		return config.BoolLiteral(b), boolStr(b), nil

	case knobInt:
		n, e := strconv.Atoi(v)
		if e != nil {
			return "", "", fmt.Errorf("%s: %q is not a whole number", k.label, v)
		}
		n = int(clamp(float64(n), k.min, k.max))
		return config.IntLiteral(n), strconv.Itoa(n), nil

	case knobFloat:
		f, e := strconv.ParseFloat(v, 64)
		if e != nil {
			return "", "", fmt.Errorf("%s: %q is not a number", k.label, v)
		}
		f = clamp(f, k.min, k.max)
		return config.FloatLiteral(f), floatStr(f), nil

	case knobEnum:
		if !slices.Contains(k.enum, v) {
			return "", "", fmt.Errorf("%s: %q is not one of %s", k.label, v, strings.Join(k.enum, ", "))
		}
		return config.StringLiteral(v), v, nil

	case knobColor:
		if v != "" {
			if _, _, _, ok := parseHex(strings.ToLower(v)); !ok {
				return "", "", fmt.Errorf("%s: %q is not a #rrggbb colour", k.label, v)
			}
			v = strings.ToLower(v)
		}
		return config.StringLiteral(v), v, nil

	case knobText:
		if k.id() == "sound.quiet_hours" && !config.ValidWindow(v) {
			return "", "", fmt.Errorf("Quiet hours: %q is not HH:MM-HH:MM", v)
		}
		return config.StringLiteral(v), v, nil
	}
	return "", "", fmt.Errorf("%s: unsupported control", k.label)
}

func clamp(v, lo, hi float64) float64 {
	if hi == 0 && lo == 0 {
		return v
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// floatStr keeps a float readable and stable: 0.4 rather than 0.400000, and 12
// rather than 12.0, so the string comparison against the file does not see a
// change where there is none.
func floatStr(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
