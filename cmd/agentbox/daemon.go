package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/borismilner/agentbox/internal/config"
	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/hand"
	"github.com/borismilner/agentbox/internal/hotkey"
	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/presence"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/server"
	"github.com/borismilner/agentbox/internal/sound"
	"github.com/borismilner/agentbox/internal/speech"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/tray"
	"github.com/borismilner/agentbox/internal/version"
	"github.com/borismilner/agentbox/internal/webui"
)

// The daemon satisfies the UI's store seams structurally; a signature drift
// should fail here, not at the SetBoardStore call site.
var _ webui.BoardStore = (*daemon.Daemon)(nil)

// lateResolver breaks the construction cycle: the UI needs a Resolver
// before the daemon (which is that resolver) exists.
type lateResolver struct {
	mu sync.Mutex
	d  *daemon.Daemon
}

func (l *lateResolver) get() *daemon.Daemon {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.d
}

func (l *lateResolver) set(d *daemon.Daemon) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.d = d
}

func (l *lateResolver) Answer(id, label string) {
	if d := l.get(); d != nil {
		d.Answer(id, label)
	}
}

func (l *lateResolver) Reply(id, text string) {
	if d := l.get(); d != nil {
		d.Reply(id, text)
	}
}

func (l *lateResolver) Dismiss(id string) {
	if d := l.get(); d != nil {
		d.Dismiss(id)
	}
}

func (l *lateResolver) Veto(id string) {
	if d := l.get(); d != nil {
		d.Veto(id)
	}
}

func (l *lateResolver) Secret(id, value string) {
	if d := l.get(); d != nil {
		d.Secret(id, value)
	}
}

func (l *lateResolver) Defer(id string) {
	if d := l.get(); d != nil {
		d.Defer(id)
	}
}

func (l *lateResolver) Undo(id string) {
	if d := l.get(); d != nil {
		d.Undo(id)
	}
}

func (l *lateResolver) AnswerForm(id string, values map[string]string) {
	if d := l.get(); d != nil {
		d.AnswerForm(id, values)
	}
}

func (l *lateResolver) RunAction(id string, index int) {
	if d := l.get(); d != nil {
		d.RunAction(id, index)
	}
}

func (l *lateResolver) Review(id string, approved bool, comment string) {
	if d := l.get(); d != nil {
		d.Review(id, approved, comment)
	}
}

func (l *lateResolver) ArtifactEvent(ev proto.ArtifactEvent) {
	if d := l.get(); d != nil {
		d.ArtifactEvent(ev)
	}
}

func daemonConfig(cfg config.Config) daemon.Config {
	return daemon.Config{
		ToastDuration:      time.Duration(cfg.Toast.DurationS) * time.Second,
		RetentionAge:       time.Duration(cfg.History.RetentionDays) * 24 * time.Hour,
		KeepLevel:          proto.Level(cfg.History.KeepLevel),
		UndoGrace:          time.Duration(cfg.Ask.UndoGraceS) * time.Second,
		VetoWindow:         time.Duration(cfg.Veto.DefaultWindowS) * time.Second,
		IdleAfter:          time.Duration(cfg.Presence.IdleAfterS) * time.Second,
		HoldWhenIdle:       cfg.Presence.HoldWhenIdle,
		FullscreenAutoDnd:  cfg.Presence.FullscreenAutoDnd,
		RespectDesktopDnd:  cfg.Presence.RespectDesktopDnd,
		EscalationInterval: time.Duration(cfg.Escalation.IntervalS) * time.Second,
		EscalationCount:    cfg.Escalation.Count,
		UrgentInterval:     time.Duration(cfg.Escalation.UrgentIntervalS) * time.Second,
		DndBlocksUrgent:    !cfg.Dnd.UrgentBreaksThrough,
		ActionsDisabled:    !cfg.Actions.Enabled,
		StartInDnd:         cfg.Dnd.StartInDnd,
		// Sync (FR83). WaitMax bounds a parked MCP call; the other two are the lock
		// table's clock. wait_warn_s = 0 means never warn and is passed through as
		// the zero it is: config.Default supplies the real default, so nothing
		// downstream has to guess whether a zero was chosen or omitted.
		SyncWaitMax:         time.Duration(cfg.Sync.WaitMaxS) * time.Second,
		SyncWaitWarn:        time.Duration(cfg.Sync.WaitWarnS) * time.Second,
		SyncHolderGoneGrace: time.Duration(cfg.Sync.HolderGoneGraceS) * time.Second,
		SignalKeep:          cfg.Sync.SignalKeep,
		SignalKeepDays:      cfg.Sync.SignalKeepDays,
		SharedMaxBytes:      cfg.Sync.SharedMaxBytes,
	}
}

// runTray runs the system tray, recovering from a panic so a tray fault never
// takes the daemon down - the tray is progressive enhancement, the daemon and
// its cards must outlive it.
func runTray(log *slog.Logger, hooks tray.Hooks) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(logging.EvPanic, "component", "tray", "panic", fmt.Sprint(r))
		}
	}()
	tray.Run(hooks)
}

// audio is agentbox's audio channel: the earcons and the voice behind one interface
// (daemon.Sounder), so a notification chimes and then speaks without the daemon
// knowing there are two subprocesses behind it. Pairing them here rather than in
// the daemon is what keeps internal/sound about embedded wav files and
// internal/speech about a synthesiser.
type audio struct {
	*sound.Player
	say *speech.Speaker
}

func (a audio) Speak(text string) { a.say.Speak(text) }

// SpeakWait is Speak that comes back when the line has been heard. The earcon is
// inside the wait rather than beside it: speech queues behind the chime (see
// Before), so a caller timing anything against the answer gets the whole sound.
func (a audio) SpeakWait(ctx context.Context, text string) {
	select {
	case <-a.say.SpeakWait(text):
	case <-ctx.Done():
	}
}

// StopSpeaking makes pause and stop immediate: without it the transport could
// only act between utterances, which on a paragraph means seconds of lag.
func (a audio) StopSpeaking() { a.say.Stop() }

// ReadWait is SpeakWait for one passage of a reading, without the sentence-length
// cap. The cap is a notification policy; applying it here would quietly drop the
// end of every paragraph.
func (a audio) ReadWait(ctx context.Context, text string) {
	select {
	case <-a.say.ReadWait(text):
	case <-ctx.Done():
	}
}

// driver satisfies daemon.Driver: an agent asking agentbox to move the pointer and
// press keys. A connection is opened per script rather than held open, because a
// script is a self-contained sequence and a long-lived XTEST connection would be
// one more thing to keep alive across a suspend for no gain.
type driver struct{}

func (driver) Drive(script string, speed float64, wpm int, park daemon.Park) (int, error) {
	steps, err := hand.ParseScript(script)
	if err != nil {
		return 0, err
	}
	h, err := hand.Open(0)
	if err != nil {
		return 0, err
	}
	defer h.Close()
	h.SetSpeed(speed)
	h.SetWPM(wpm)
	// daemon.Park and hand.Park are the same two methods declared in two packages
	// that must not import each other; this is the one line where they meet.
	if park != nil {
		h.SetPark(park)
	}
	// The count comes from Run rather than from len(steps): a script the human
	// paused part way through ran fewer than it was given, and saying otherwise
	// would tell the agent work happened that did not.
	return h.Run(steps)
}

func applySound(snd *sound.Player, cfg config.Config) {
	snd.Configure(cfg.Sound.Enabled, cfg.Sound.Volume, cfg.InQuietHours)
}

// applySpeech mirrors applySound: both are called at startup and again on every
// config reload, so the voice can be turned on, retuned or silenced live.
func applySpeech(say *speech.Speaker, cfg config.Config) {
	say.Configure(speech.Options{
		Enabled:  cfg.Speech.Enabled,
		Argv:     cfg.Speech.Command,
		Rate:     cfg.Speech.Rate,
		Channels: cfg.Speech.Channels,
		Volume:   cfg.Speech.Volume,
		MaxChars: cfg.Speech.MaxChars,
		Idle:     time.Duration(cfg.Speech.IdleTimeoutS) * time.Second,
		Prewarm:  cfg.Speech.Prewarm,
	}, cfg.InQuietHours)
}

func runDaemon() {
	cfg, cfgWarns, cfgErr := config.Load(config.Path())

	level := slog.LevelInfo
	if cfg.Log.Level == "debug" || os.Getenv("AGENTBOX_LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	log, logCloser, err := logging.Open(stateDir(), level, cfg.Log.RetentionMB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox daemon: %v\n", err)
		os.Exit(exitError)
	}
	if cfgErr != nil {
		log.Error("config.broken", "component", "daemon", "err", cfgErr.Error())
	}
	for _, w := range cfgWarns {
		log.Warn("config.invalid_key", "component", "daemon", "warning", w)
	}

	rd := runtimeDir()
	lst, err := server.Listen(rd, log)
	if errors.Is(err, server.ErrAlreadyRunning) {
		fmt.Println("agentbox daemon already running")
		os.Exit(exitOK)
	}
	if err != nil {
		log.Error(logging.EvDaemonStart, "component", "daemon", "err", err.Error())
		fmt.Fprintf(os.Stderr, "agentbox daemon: %v\n", err)
		os.Exit(exitError)
	}

	vi := version.Get()
	log.Info(logging.EvDaemonStart, "component", "daemon",
		"revision", vi.Revision, "dirty", vi.Dirty, "build_time", vi.BuildTime,
		"go", vi.GoVersion, "runtime_dir", rd, "state_dir", stateDir(),
		"session_type", os.Getenv("XDG_SESSION_TYPE"), "desktop", os.Getenv("XDG_CURRENT_DESKTOP"))

	st, err := store.Open(filepath.Join(stateDir(), "agentbox.db"))
	if err != nil {
		log.Error("store.open_failed", "component", "daemon", "err", err.Error())
		fmt.Fprintf(os.Stderr, "agentbox daemon: %v\n", err)
		os.Exit(exitError)
	}
	if v, err := st.SchemaVersion(); err == nil {
		log.Info(logging.EvStoreMigrated, "component", "daemon", "schema_version", v)
	}

	snd := sound.New(log, filepath.Join(rd, "sounds"), exec.LookPath)
	applySound(snd, cfg)
	say := speech.New(log, exec.LookPath)
	applySpeech(say, cfg)
	// Speech waits for the earcon rather than talking over it: the chime still
	// carries the level, and the sentence arrives behind it.
	say.Before(snd.Wait)

	// The UI resolves its own appearance from the config (theme mode, including
	// `auto`, and the font knobs); AGENTBOX_THEME and AGENTBOX_FONT_SCALE override it
	// there, not here (internal/webui/tokens.go).
	res := &lateResolver{}
	u := webui.New(res, log, cfg)

	d, err := daemon.New(daemonConfig(cfg), log, st, audio{Player: snd, say: say}, u)
	if err != nil {
		log.Error("daemon.init_failed", "component", "daemon", "err", err.Error())
		os.Exit(exitError)
	}
	res.set(d)
	u.SetSource(d)
	u.SetBoardStore(d)
	u.SetVoice(d)
	// The handover strip, both directions: the daemon paints it, and the strip's
	// Deny and Allow answer the agent blocked on the request (FR74).
	u.SetHandover(d.Handover())
	d.SetControlSurface(u.ShowControl, u.HideControl)
	// The Agents surface (FR83): the roster pushes at it on every change,
	// throttled on the daemon's side. One line, because the conversion into the
	// surface's own vocabulary lives with the surface.
	d.SetRosterSurface(u.ShowRoster)
	// The lock table, which the rows alone cannot show (a hold from a CLI caller
	// has no row) and which carries the human's one verb here: break a lock.
	u.SetRoster(rosterBridge{d})
	// And the pull, so a window opened between two pushes does not start blank.
	u.ShowRoster(d.RosterSnapshot())
	// The tick, started after the surface is wired. Without it the board is only
	// ever as fresh as the last verb somebody happened to call: a state that
	// decays on time alone never decays on screen, and a throttled push waits for
	// unrelated traffic.
	d.StartRoster()
	// The lock table's clock (FR83 slice 2): an orphaned hold whose process has
	// died, a ttl that has run out, a wait long enough to be contention. None of
	// those is caused by a verb, so nothing else would notice them.
	d.StartLocks()
	// Retention's clock (FR83 slice 3). Signals are trimmed by count on every post,
	// but nothing a caller does makes a signal a week old, so age needs its own
	// tick for the same reason the two above do.
	d.StartSignals()
	// And the rider, which is how an agent already deep in a file hears that it
	// has company: it rides back on whatever call it makes next (FR83).
	lst.SetRider(d.SyncRider)
	d.SetPresence(presence.New()) // FR29/FR44 presence signals; no-op without X11
	d.SetDriver(driver{})         // synthetic input (agentbox drive / drive_desktop); refuses itself without X11
	// Assignments (M12/FR82): the daemon owns the schedule, the webui carries a
	// run out as an ordinary session. Wired here and started here, in that
	// order - a tick that fired before the runner existed would record a pile of
	// failures on the way up.
	d.SetRunner(u)
	u.SetAssignmentStore(d)
	d.StartAssignments()

	// M10: the drop-down panel's hotkey, grabbed by the daemon so the panel works
	// with no desktop configuration. Every failure here is survivable and none of
	// them is fatal: `agentbox panel` still works, and the reason is logged and
	// printed once rather than swallowed - "my hotkey does nothing" is otherwise
	// unanswerable. An empty [panel] hotkey turns the grab off deliberately.
	var hk *hotkey.Hotkey
	if spec := strings.TrimSpace(cfg.Panel.Hotkey); spec != "" {
		h, err := hotkey.Open(spec, log, u.TogglePanel)
		if err != nil {
			log.Warn("hotkey.unavailable", "component", "daemon", "hotkey", spec, "err", err.Error())
			fmt.Fprintf(os.Stderr, "agentbox: %v (use `agentbox panel` instead, or set [panel] hotkey)\n", err)
		} else {
			hk = h
			log.Info("hotkey.grabbed", "component", "daemon", "hotkey", spec)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	var shutdown func()
	shutdown = func() {
		log.Info(logging.EvDaemonStop, "component", "daemon")
		say.Close() // let the voice finish its sentence, then drop the model
		if hk != nil {
			hk.Close() // release the key grab before the process goes
		}
		d.StopAssignments() // no new runs; one in flight is left to finish
		d.StopRoster()      // stop repainting a board that is about to go
		// Before st.Close() below, and that ordering is the reason this call exists:
		// retention's tick writes to the store, so a trim in flight when the database
		// closed would be a use-after-close on the way out.
		d.StopSignals()
		u.ShutdownApp()   // close button only hides to tray; quit ends the sessions
		d.BeginShutdown() // a disconnect now is teardown, not a caller drop (FR45)
		cancel()
		lst.Close()
		st.Close()
		logCloser.Close()
		os.Exit(exitOK)
	}
	d.OnQuit = shutdown
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		shutdown()
	}()

	// FR29 presence poll: while items are pending, watch for the user
	// returning from idle or leaving fullscreen / desktop DND, so held cards
	// reveal and the one summary chime fires without waiting on a new call.
	// A quiet daemon makes no monitor calls (PresencePoll early-returns).
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error(logging.EvPanic, "component", "daemon", "goroutine", "presence_poll", "panic", fmt.Sprint(r))
			}
		}()
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				d.PresencePoll()
				d.ReapStaleProgress() // FR21: drop progress reports whose caller died without Done
			}
		}
	}()

	// 400ms rather than the old 2s: the point of a live config is that you can tune
	// agentbox while looking at it, and a two-second lag turns "nudge the measure" into
	// "wait, did that apply?". A stat() every 400ms costs nothing measurable.
	stopWatch := config.Watch(config.Path(), 400*time.Millisecond, func(c config.Config, warns []string) {
		for _, w := range warns {
			log.Warn("config.invalid_key", "component", "daemon", "warning", w)
		}
		d.SetPolicy(daemonConfig(c))
		applySound(snd, c)
		applySpeech(say, c)
		// Everything the UI is made of - tokens, window sizes, the reading measure,
		// the panel's shape - lands here, live. This is what makes tuning agentbox a
		// conversation with the config file rather than a restart.
		u.SetConfig(c)

		// The panel's hotkey too: rebind an existing grab, take one if the config
		// had none, or drop it if the key was cleared.
		spec := strings.TrimSpace(c.Panel.Hotkey)
		switch {
		case hk != nil && spec == "":
			hk.Close()
			hk = nil
			log.Info("hotkey.released", "component", "daemon")
		case hk != nil && spec != hk.Spec():
			if err := hk.Rebind(spec); err != nil {
				log.Warn("hotkey.rebind_failed", "component", "daemon", "hotkey", spec, "err", err.Error())
			}
		case hk == nil && spec != "":
			if h, err := hotkey.Open(spec, log, u.TogglePanel); err != nil {
				log.Warn("hotkey.unavailable", "component", "daemon", "hotkey", spec, "err", err.Error())
			} else {
				hk = h
				log.Info("hotkey.grabbed", "component", "daemon", "hotkey", spec)
			}
		}

		log.Info("config.reloaded", "component", "daemon")
	})
	defer stopWatch()

	if os.Getenv("AGENTBOX_NO_TRAY") == "" {
		u.OnView = func(v daemon.View) {
			n := v.Waiting
			if v.Item != nil {
				n++
			}
			tray.SetPending(n)
		}
		d.OnDndChange = tray.SetDnd
		d.OnMuteChange = tray.SetMuted
		u.OnAppChange = tray.SetAppOpen
		go runTray(log, tray.Hooks{
			ToggleApp: u.ToggleApp,
			AppOpen:   u.AppOpen,
			ToggleDnd: func() bool {
				d.DndSet(!d.IsDnd())
				return d.IsDnd()
			},
			DndState: d.IsDnd,
			Quit:     shutdown,
		})
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error(logging.EvPanic, "component", "server", "panic", fmt.Sprint(r))
				os.Exit(exitError)
			}
		}()
		if err := lst.Serve(ctx, d.Handle); err != nil {
			log.Error("server.serve_failed", "component", "server", "err", err.Error())
		}
	}()

	// The UI owns the main thread from here; windows come and go per card.
	if err := u.Run(); err != nil {
		log.Error("ui.run_failed", "component", "daemon", "err", err.Error())
		fmt.Fprintf(os.Stderr, "agentbox daemon: %v\n", err)
		os.Exit(exitError)
	}
}
