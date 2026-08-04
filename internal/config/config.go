// Package config reads ~/.config/agentbox/config.toml (06-configuration.md).
// Zero config gives the full experience (NFR11): a missing file is the
// defaults, an invalid key is a warning, never a failure.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Sound struct {
		Enabled    bool    `toml:"enabled"`
		Volume     float64 `toml:"volume"`
		QuietHours string  `toml:"quiet_hours"`
	} `toml:"sound"`
	Escalation struct {
		IntervalS       int `toml:"interval_s"`
		Count           int `toml:"count"`
		UrgentIntervalS int `toml:"urgent_interval_s"`
	} `toml:"escalation"`
	Toast struct {
		DurationS int `toml:"duration_s"`
	} `toml:"toast"`
	Ask struct {
		AllowReply bool `toml:"allow_reply"`
		UndoGraceS int  `toml:"undo_grace_s"`
	} `toml:"ask"`
	Veto struct {
		DefaultWindowS int `toml:"default_window_s"`
	} `toml:"veto"`
	Presence struct {
		HoldWhenIdle      bool `toml:"hold_when_idle"`
		IdleAfterS        int  `toml:"idle_after_s"`
		FullscreenAutoDnd bool `toml:"fullscreen_auto_dnd"`
		RespectDesktopDnd bool `toml:"respect_desktop_dnd"`
	} `toml:"presence"`
	Actions struct {
		Enabled bool `toml:"enabled"`
	} `toml:"actions"`
	// Sync is multi-agent coordination (FR83). WaitMaxS bounds a PARKED MCP CALL
	// and nothing else: the client aborts a tool call it has heard nothing about
	// for 1800s, so the ceiling sits under that and a wait that hits it returns a
	// resumable timeout instead of a transport error. A CLI hold is bounded by
	// whatever runs the CLI, which is a different number entirely (120s from an
	// agent's shell, 600s with an explicit timeout) - the two must not be read as
	// one.
	Sync struct {
		WaitMaxS int `toml:"wait_max_s"`
		// WaitWarnS toasts when a LOCK wait exceeds it; 0 disables. A signal wait
		// never warns, because listening is the intended steady state and warning
		// on it would train the human to ignore the toast that matters.
		WaitWarnS        int `toml:"wait_warn_s"`
		HolderGoneGraceS int `toml:"holder_gone_grace_s"`
	} `toml:"sync"`
	// Panel is the drop-down session panel (M10). Hotkey is grabbed by the
	// daemon on X11, so it works with no desktop configuration; an empty string
	// turns the grab off and leaves `agentbox panel` as the only way in.
	Panel struct {
		Hotkey     string  `toml:"hotkey"`
		HeightFrac float64 `toml:"height_frac"`
		WidthFrac  float64 `toml:"width_frac"`
		SlideMS    int     `toml:"slide_ms"`   // 0 = no animation, just appear
		MeasurePx  int     `toml:"measure_px"` // reading column inside the panel
	} `toml:"panel"`
	// Window is every agentbox window's shape. These were constants until the sizes
	// turned out to be taste: a card that is right on a 1080p laptop is small on a
	// 4K panel, and the reading measure is the single biggest lever on how agentbox
	// reads. All of it applies live - the open windows resize, and the next card
	// opens at the new size (06-configuration.md).
	Window struct {
		CardWidth     int `toml:"card_width"`
		CardMaxHeight int `toml:"card_max_height"`

		ToastWidth     int `toml:"toast_width"`
		ToastMaxHeight int `toml:"toast_max_height"`
		ToastTopInset  int `toml:"toast_top_inset"`

		AppWidth  int `toml:"app_width"`
		AppHeight int `toml:"app_height"`

		ViewerWidth  int `toml:"viewer_width"`
		ViewerHeight int `toml:"viewer_height"`

		// The review board opens maximized; this is what un-maximizing
		// restores.
		BoardWidth  int `toml:"board_width"`
		BoardHeight int `toml:"board_height"`

		ProgressWidth     int `toml:"progress_width"`
		ProgressMaxHeight int `toml:"progress_max_height"`

		// MeasurePx is the reading column: prose caps here however wide the window
		// is, because a 1600px line of prose is unreadable.
		MeasurePx int `toml:"measure_px"`
	} `toml:"window"`
	// Artifact is agent-authored interactive HTML (M10). Enabled is a trust
	// switch, not a feature flag: with it off, an artifact stays source you can
	// read and nothing in it ever runs. MaxHeightPx bounds an inline artifact so
	// an agent cannot push the rest of a conversation off the screen.
	Artifact struct {
		Enabled     bool `toml:"enabled"`
		MaxHeightPx int  `toml:"max_height_px"`
	} `toml:"artifact"`
	// Speech reads an agent's spoken line out loud. It is off by default because
	// it needs a synthesiser and a voice, not because it is experimental: with
	// Command empty and Enabled true, agentbox looks for piper and a voice itself.
	// The engine contract is one line of text on stdin, raw s16le PCM on stdout.
	// Nothing is ever spoken that an agent did not write as a spoken line - agentbox
	// does not read titles aloud on its own.
	Speech struct {
		Enabled bool `toml:"enabled"`
		// Command is the engine argv. Empty means "find one". Set it to take
		// control of the voice and its tuning, e.g.
		//   command = ["piper", "--model", "/home/you/piper-voices/en_US-lessac-high.onnx",
		//              "--output-raw", "--length-scale", "1.1"]
		Command []string `toml:"command"`
		// Rate must match the voice. Zero reads it from the voice's own JSON when
		// the engine was detected; a voice played at the wrong rate is a chipmunk.
		Rate         int     `toml:"rate"`
		Channels     int     `toml:"channels"`
		Volume       float64 `toml:"volume"`
		MaxChars     int     `toml:"max_chars"`
		IdleTimeoutS int     `toml:"idle_timeout_s"`
		Prewarm      bool    `toml:"prewarm"`
	} `toml:"speech"`
	// Session is the Claude child agentbox spawns (FR49).
	Session struct {
		DefaultMode string `toml:"default_mode"` // plan | full, for a new session
		Binary      string `toml:"binary"`       // empty = `claude` on PATH
		Dir         string `toml:"dir"`          // empty = the daemon's cwd
		ShowCost    bool   `toml:"show_cost"`    // the per-reply dollar figure; off by default
	} `toml:"session"`
	// Theme is a token set, not a mode switch: the web UI resolves these into
	// CSS custom properties, so every key here applies live (no restart).
	// Ground and Accent are the only two colour knobs on purpose - hand-tuned
	// pairs plus one accent cannot produce an unreadable theme, twenty
	// individual surface colours can.
	Theme struct {
		Mode     string `toml:"mode"`     // auto | dark | light
		Ground   string `toml:"ground"`   // graphite | ink | slate
		Contrast string `toml:"contrast"` // normal | high; lifts the muted inks and edges
		Accent   string `toml:"accent"`   // hex; focus rings, links, primary action
		Density  string `toml:"density"`  // comfortable | compact
		Radius   int    `toml:"radius"`   // corner radius in px
		Motion   string `toml:"motion"`   // full | reduced | none
	} `toml:"theme"`
	Markdown struct {
		CodeTheme string `toml:"code_theme"` // auto | nord | gruvbox | github | onedark | dracula
	} `toml:"markdown"`
	Font struct {
		SizePt  float64 `toml:"size_pt"`
		Family  string  `toml:"family"`  // interface chrome; empty = system
		Reading string  `toml:"reading"` // agent prose; empty = the bundled serif
		Mono    string  `toml:"mono"`    // code, keycaps, numerals
	} `toml:"font"`
	Dnd struct {
		StartInDnd          bool `toml:"start_in_dnd"`
		UrgentBreaksThrough bool `toml:"urgent_breaks_through"`
	} `toml:"dnd"`
	History struct {
		RetentionDays int    `toml:"retention_days"`
		KeepLevel     string `toml:"keep_level"`
	} `toml:"history"`
	Log struct {
		Level       string `toml:"level"`
		RetentionMB int    `toml:"retention_mb"`
	} `toml:"log"`
}

// Clamp bounds shared by Load and the settings tab's steppers, so the UI and
// the loader enforce the same numbers (06-configuration.md). A value outside
// these falls back to the default with a warning.
const (
	VolumeMin, VolumeMax     = 0.0, 1.0
	FontSizeMin, FontSizeMax = 6.0, 36.0
	// A PCM sample rate a voice might plausibly emit. Telephone-band at the
	// bottom, studio at the top; piper's voices are 16k or 22.05k.
	SpeechRateMin, SpeechRateMax = 8000, 192000
	UndoGraceMin, UndoGraceMax   = 0, 5
)

// Default returns the documented defaults (06-configuration.md): an empty
// file behaves identically.
func Default() Config {
	var c Config
	c.Sound.Enabled = true
	c.Sound.Volume = 0.4
	c.Escalation.IntervalS = 60
	c.Escalation.Count = 5
	c.Escalation.UrgentIntervalS = 20
	c.Toast.DurationS = 6
	c.Ask.AllowReply = true
	c.Ask.UndoGraceS = 3
	c.Veto.DefaultWindowS = 15
	c.Presence.HoldWhenIdle = true
	c.Presence.IdleAfterS = 120
	c.Presence.FullscreenAutoDnd = true
	c.Presence.RespectDesktopDnd = true
	c.Actions.Enabled = true
	c.Sync.WaitMaxS = 1500
	c.Sync.WaitWarnS = 600
	c.Sync.HolderGoneGraceS = 5
	c.Panel.Hotkey = "Ctrl+Alt+grave"
	c.Panel.HeightFrac = 0.5 // half the monitor, never more: see the clamp below
	c.Panel.WidthFrac = 0.74
	// Off by default: see the note in internal/webui/panel.go. A resize per frame
	// cannot be made to look like Quake's console on this stack, and a roll that
	// draws attention to itself is worse than no roll at all. The knob stays.
	c.Panel.SlideMS = 0
	c.Panel.MeasurePx = 980
	c.Window.CardWidth = 470
	c.Window.CardMaxHeight = 760
	c.Window.ToastWidth = 430
	c.Window.ToastMaxHeight = 330
	c.Window.ToastTopInset = 48
	c.Window.AppWidth = 1180
	c.Window.AppHeight = 860
	c.Window.ViewerWidth = 900
	c.Window.ViewerHeight = 780
	c.Window.BoardWidth = 1360
	c.Window.BoardHeight = 900
	c.Window.ProgressWidth = 400
	c.Window.ProgressMaxHeight = 620
	c.Window.MeasurePx = 700
	c.Artifact.Enabled = true
	c.Artifact.MaxHeightPx = 640
	c.Speech.Channels = 1
	c.Speech.Volume = 0.7
	c.Speech.MaxChars = 240
	c.Speech.IdleTimeoutS = 600
	// Full, not plan. Plan was the cautious default while the session surface was
	// new; a console you open to get something done is not much use read-only, and
	// the human is looking straight at it.
	c.Session.DefaultMode = "full"
	c.Session.ShowCost = false // true puts the dollars for each reply beside it
	c.Theme.Mode = "auto"
	c.Theme.Ground = "graphite"
	c.Theme.Contrast = "normal"
	c.Theme.Density = "comfortable"
	c.Theme.Radius = 10
	c.Theme.Motion = "full"
	c.Markdown.CodeTheme = "auto"
	c.Font.SizePt = 12
	c.Dnd.UrgentBreaksThrough = true
	c.History.RetentionDays = 30
	c.History.KeepLevel = "warning"
	c.Log.Level = "info"
	c.Log.RetentionMB = 50
	return c
}

// Path returns the config file location.
func Path() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "agentbox", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "agentbox", "config.toml")
}

// Load reads path over the defaults. The error is never fatal to a daemon:
// a missing file means defaults, a broken file means defaults plus a
// warning. Unknown keys come back as warnings (logged, then ignored).
func Load(path string) (Config, []string, error) {
	c := Default()
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil, nil
		}
		return Default(), nil, fmt.Errorf("config unreadable, using defaults: %w", err)
	}
	var warnings []string
	for _, k := range md.Undecoded() {
		warnings = append(warnings, "unknown config key: "+k.String())
	}
	if c.Sound.Volume < VolumeMin || c.Sound.Volume > VolumeMax {
		warnings = append(warnings, fmt.Sprintf("sound.volume %v out of [%v,%v], using default", c.Sound.Volume, VolumeMin, VolumeMax))
		c.Sound.Volume = Default().Sound.Volume
	}
	// speech.rate is the one knob whose zero is a choice rather than an omission:
	// it means "take the rate from the voice", which is what a detected engine
	// does. So it cannot go in the clamp table, whose zero means "unset".
	if c.Speech.Rate != 0 && (c.Speech.Rate < SpeechRateMin || c.Speech.Rate > SpeechRateMax) {
		warnings = append(warnings, fmt.Sprintf("speech.rate %d out of [%d,%d], reading it from the voice instead",
			c.Speech.Rate, SpeechRateMin, SpeechRateMax))
		c.Speech.Rate = 0
	}
	if c.Speech.Volume < VolumeMin || c.Speech.Volume > VolumeMax {
		warnings = append(warnings, fmt.Sprintf("speech.volume %v out of [%v,%v], using default", c.Speech.Volume, VolumeMin, VolumeMax))
		c.Speech.Volume = Default().Speech.Volume
	}
	if c.Font.SizePt < FontSizeMin || c.Font.SizePt > FontSizeMax {
		warnings = append(warnings, fmt.Sprintf("font.size_pt %v out of [%v,%v], using default", c.Font.SizePt, FontSizeMin, FontSizeMax))
		c.Font.SizePt = Default().Font.SizePt
	}
	// The undo strip must stay short (user requirement: 3-5 s tops); a
	// long window holds every answer hostage.
	if c.Ask.UndoGraceS < UndoGraceMin || c.Ask.UndoGraceS > UndoGraceMax {
		warnings = append(warnings, fmt.Sprintf("ask.undo_grace_s %v out of [%v,%v], using default", c.Ask.UndoGraceS, UndoGraceMin, UndoGraceMax))
		c.Ask.UndoGraceS = Default().Ask.UndoGraceS
	}
	// The panel is sized as a fraction of the screen; outside these bounds it is
	// either a sliver or indistinguishable from a full-screen window, and both
	// read as broken rather than configured.
	//
	// Height stops at half the screen. A drop-down console that covers more than
	// that is not a console any more, it is a window that arrived without being
	// asked - and the thing you were reading underneath is the reason the panel
	// rolls down rather than opening.
	for _, f := range []struct {
		name string
		val  *float64
		def  float64
		lo   float64
		hi   float64
	}{
		{"panel.height_frac", &c.Panel.HeightFrac, Default().Panel.HeightFrac, 0.2, 0.5},
		{"panel.width_frac", &c.Panel.WidthFrac, Default().Panel.WidthFrac, 0.3, 1},
	} {
		if *f.val < f.lo || *f.val > f.hi {
			warnings = append(warnings, fmt.Sprintf("%s %v out of [%v,%v], using default", f.name, *f.val, f.lo, f.hi))
			*f.val = f.def
		}
	}
	// Every pixel knob is clamped rather than trusted: a zero would produce an
	// invisible window and a 40000 would produce one nobody can reach, and both
	// arrive by typo. The bounds are wide on purpose - taste is allowed, nonsense
	// is not.
	def0 := Default()
	for _, k := range []struct {
		name     string
		val      *int
		def      int
		lo, hi   int
		zeroFree bool // 0 is a meaningful value (an animation that does not animate)
	}{
		{name: "window.card_width", val: &c.Window.CardWidth, def: def0.Window.CardWidth, lo: 240, hi: 2000},
		{name: "window.card_max_height", val: &c.Window.CardMaxHeight, def: def0.Window.CardMaxHeight, lo: 200, hi: 4000},
		{name: "window.toast_width", val: &c.Window.ToastWidth, def: def0.Window.ToastWidth, lo: 240, hi: 2000},
		{name: "window.toast_max_height", val: &c.Window.ToastMaxHeight, def: def0.Window.ToastMaxHeight, lo: 60, hi: 2000},
		{name: "window.toast_top_inset", val: &c.Window.ToastTopInset, def: def0.Window.ToastTopInset, lo: 0, hi: 2000, zeroFree: true},
		{name: "window.app_width", val: &c.Window.AppWidth, def: def0.Window.AppWidth, lo: 640, hi: 6000},
		{name: "window.app_height", val: &c.Window.AppHeight, def: def0.Window.AppHeight, lo: 400, hi: 4000},
		{name: "window.viewer_width", val: &c.Window.ViewerWidth, def: def0.Window.ViewerWidth, lo: 400, hi: 6000},
		{name: "window.viewer_height", val: &c.Window.ViewerHeight, def: def0.Window.ViewerHeight, lo: 300, hi: 4000},
		{name: "window.board_width", val: &c.Window.BoardWidth, def: def0.Window.BoardWidth, lo: 700, hi: 6000},
		{name: "window.board_height", val: &c.Window.BoardHeight, def: def0.Window.BoardHeight, lo: 500, hi: 4000},
		{name: "window.progress_width", val: &c.Window.ProgressWidth, def: def0.Window.ProgressWidth, lo: 240, hi: 2000},
		{name: "window.progress_max_height", val: &c.Window.ProgressMaxHeight, def: def0.Window.ProgressMaxHeight, lo: 80, hi: 3000},
		{name: "window.measure_px", val: &c.Window.MeasurePx, def: def0.Window.MeasurePx, lo: 320, hi: 2400},
		{name: "panel.measure_px", val: &c.Panel.MeasurePx, def: def0.Panel.MeasurePx, lo: 320, hi: 3000},
		{name: "panel.slide_ms", val: &c.Panel.SlideMS, def: def0.Panel.SlideMS, lo: 0, hi: 1000, zeroFree: true},
		{name: "artifact.max_height_px", val: &c.Artifact.MaxHeightPx, def: def0.Artifact.MaxHeightPx, lo: 120, hi: 4000},
		{name: "speech.channels", val: &c.Speech.Channels, def: def0.Speech.Channels, lo: 1, hi: 2},
		{name: "speech.max_chars", val: &c.Speech.MaxChars, def: def0.Speech.MaxChars, lo: 20, hi: 2000},
		{name: "speech.idle_timeout_s", val: &c.Speech.IdleTimeoutS, def: def0.Speech.IdleTimeoutS, lo: 5, hi: 86400},
	} {
		if *k.val == 0 && !k.zeroFree {
			*k.val = k.def // an unset key in a partial file, not a choice
			continue
		}
		if *k.val < k.lo || *k.val > k.hi {
			warnings = append(warnings, fmt.Sprintf("%s %d out of [%d,%d], using default", k.name, *k.val, k.lo, k.hi))
			*k.val = k.def
		}
	}
	if c.Session.DefaultMode == "" {
		c.Session.DefaultMode = def0.Session.DefaultMode
	}
	if !slices.Contains(SessionModes, c.Session.DefaultMode) {
		warnings = append(warnings, fmt.Sprintf("session.default_mode %q is not one of %s, using %q",
			c.Session.DefaultMode, strings.Join(SessionModes, "|"), def0.Session.DefaultMode))
		c.Session.DefaultMode = def0.Session.DefaultMode
	}
	if _, _, err := parseWindow(c.Sound.QuietHours); err != nil {
		warnings = append(warnings, err.Error())
		c.Sound.QuietHours = ""
	}
	// The token knobs are enums, and a typo in one of them would otherwise
	// resolve to whatever the theme builder falls back to - visible, but with no
	// explanation. Say so and use the default.
	def := Default()
	for _, e := range []struct {
		name  string
		val   *string
		def   string
		valid []string
	}{
		{"theme.mode", &c.Theme.Mode, def.Theme.Mode, ThemeModes},
		{"theme.ground", &c.Theme.Ground, def.Theme.Ground, ThemeGrounds},
		{"theme.contrast", &c.Theme.Contrast, def.Theme.Contrast, ThemeContrasts},
		{"theme.density", &c.Theme.Density, def.Theme.Density, ThemeDensities},
		{"theme.motion", &c.Theme.Motion, def.Theme.Motion, ThemeMotions},
		{"markdown.code_theme", &c.Markdown.CodeTheme, def.Markdown.CodeTheme, CodeThemes},
	} {
		if !slices.Contains(e.valid, *e.val) {
			warnings = append(warnings, fmt.Sprintf("%s %q is not one of %s, using %q",
				e.name, *e.val, strings.Join(e.valid, "|"), e.def))
			*e.val = e.def
		}
	}
	return c, warnings, nil
}

// SessionModes are the permission modes a new session can start in (FR49). The
// two prompting modes are absent on purpose: agentbox does not handle the
// stream-json permission protocol yet, so they would stall.
var SessionModes = []string{"plan", "full"}

// The valid values for the enum knobs, exported so the settings surface offers
// exactly what Load will accept.
var (
	ThemeModes     = []string{"auto", "dark", "light"}
	ThemeGrounds   = []string{"graphite", "ink", "slate"}
	ThemeContrasts = []string{"normal", "high"}
	ThemeDensities = []string{"comfortable", "compact"}
	ThemeMotions   = []string{"full", "reduced", "none"}
	CodeThemes     = []string{"auto", "nord", "gruvbox", "github", "onedark", "dracula"}
	KeepLevels     = []string{"info", "success", "warning", "error", "urgent"}
	LogLevels      = []string{"debug", "info", "warn", "error"}
)

// RadiusMin/Max bound the corner-radius knob, matching what the theme builder
// accepts.
const (
	RadiusMin, RadiusMax = 0, 24
)

func parseWindow(s string) (from, to int, err error) {
	if s == "" {
		return 0, 0, nil
	}
	parse := func(hm string) (int, error) {
		var h, m int
		if _, err := fmt.Sscanf(hm, "%d:%d", &h, &m); err != nil || h > 23 || m > 59 || h < 0 || m < 0 {
			return 0, fmt.Errorf("bad time %q", hm)
		}
		return h*60 + m, nil
	}
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0, fmt.Errorf("sound.quiet_hours %q: want \"HH:MM-HH:MM\"", s)
	}
	if from, err = parse(a); err != nil {
		return 0, 0, fmt.Errorf("sound.quiet_hours %q: %v", s, err)
	}
	if to, err = parse(b); err != nil {
		return 0, 0, fmt.Errorf("sound.quiet_hours %q: %v", s, err)
	}
	return from, to, nil
}

// ValidWindow reports whether s is an acceptable quiet_hours value (empty, or
// "HH:MM-HH:MM"). The settings tab uses it to refuse an invalid entry before
// writing, matching what Load would otherwise warn about and discard.
func ValidWindow(s string) bool {
	_, _, err := parseWindow(s)
	return err == nil
}

// InQuietHours reports whether now falls inside the configured window;
// windows may cross midnight ("22:30-08:00").
func (c Config) InQuietHours(now time.Time) bool {
	from, to, err := parseWindow(c.Sound.QuietHours)
	if err != nil || c.Sound.QuietHours == "" {
		return false
	}
	cur := now.Hour()*60 + now.Minute()
	if from <= to {
		return cur >= from && cur < to
	}
	return cur >= from || cur < to
}

// Watch polls the file and calls onChange with the freshly loaded config
// whenever its mtime moves. Polling keeps it dependency-free; 2 s lag on a
// config edit is imperceptible.
func Watch(path string, every time.Duration, onChange func(Config, []string)) (stop func()) {
	done := make(chan struct{})
	var lastMod time.Time
	if st, err := os.Stat(path); err == nil {
		lastMod = st.ModTime()
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				st, err := os.Stat(path)
				if err != nil || st.ModTime().Equal(lastMod) {
					continue
				}
				lastMod = st.ModTime()
				c, warns, err := Load(path)
				if err != nil {
					warns = append(warns, err.Error())
				}
				onChange(c, warns)
			}
		}
	}()
	return func() { close(done) }
}
