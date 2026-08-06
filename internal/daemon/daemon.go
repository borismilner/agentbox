// Package daemon is the headless heart of agentbox: item lifecycle, display
// queue, blocking-call delivery. It knows nothing about any UI toolkit;
// the Presenter interface is its only view of the screen, which keeps the
// whole core testable without a display.
package daemon

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/sound"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/version"
)

// CallerState tracks the liveness of a blocking item's caller (FR45), shown
// as a dot by the identity pill.
type CallerState int

const (
	CallerNone     CallerState = iota // not a blocking item: no caller to track
	CallerLive                        // the caller is connected, waiting for the answer
	CallerGone                        // the caller's socket dropped; the answer reaches history only
	CallerAwaiting                    // restored after a restart (NFR7); no caller until it resubmits
)

// View is what the screen should show right now. A nil Item clears it.
// Graced means the item was answered and sits in the undo window (FR28):
// the UI shows a collapsed "answered" strip instead of the full card.
// DismissAt, when set, is the auto-dismiss deadline the toast counts down
// to.
type View struct {
	Item           *proto.Item
	Waiting        int
	WaitingFrom    []proto.Identity // identities of queued items, queue order
	Graced         bool
	GracedText     string
	GraceUntil     time.Time // when the undo window closes and the answer ships
	DismissAt      time.Time
	ExpiresAt      time.Time   // a timed question's expiry deadline; the footer counts down to it
	ActionsEnabled bool        // FR32: render the item's action buttons (false hides them, the kill switch)
	Caller         CallerState // FR45: caller-alive indicator for the displayed blocking item
}

// ProgressState is one live progress report (FR21). It renders as a thin bar
// outside the card queue, so reports are non-blocking - they run alongside
// questions and notifications. Percent is 0..100; Indeterminate shows a
// spinner when the fraction is unknown.
type ProgressState struct {
	ID            string
	Title         string
	Status        string
	Percent       int
	Indeterminate bool
	Identity      proto.Identity
}

// Presenter renders views. Present must not call back into the daemon
// synchronously; UI callbacks arrive on their own goroutines.
type Presenter interface {
	Present(v View)
	ShowApp(tab string) // the tabbed app window (M8); tab is "" or a tab id (inbox/stats/progress)
	Summon()
	ShowDocument(req proto.ShowRequest) // the markdown viewer (FR36-38)
	TogglePanel()                       // the drop-down session panel (M10)
	ShowPanel()
	HidePanel()
	PanelOpen() bool
	ShowProgress(reports []ProgressState) // the live progress bars (FR21); empty clears the window
	ShowBoard(id string)                  // the review board for a stored walkthrough (FR58)
	// AssignmentsChanged says something about an assignment or its runs is
	// different (M12): a save, a value written, a run starting or finishing -
	// whoever did it. The open surface refreshes on it instead of polling, which
	// is what lets an agent's update_assignment reach a panel somebody is
	// looking at. A poke, not a payload: the surface re-reads what it shows.
	AssignmentsChanged()
}

// Sounder is agentbox's audio channel: the earcon that classifies an event, and the
// spoken line an agent attached to it. The two live on one interface on purpose -
// every announcement chimes and then speaks, so a new Play site cannot be added
// that silently forgets the voice. Satisfied by the pairing in cmd/agentbox.
//
// Speak("") is a no-op, which is what makes the call sites read the same whether
// the agent wrote a line or not.
type Sounder interface {
	Play(c sound.Class)
	Speak(text string)
	// SpeakWait says a line and returns when it has been heard - the earcon
	// included, since the voice follows the chime. Only the speak method asks for
	// it: everything with a card behind it must not hold a card back for the length
	// of a sentence. ctx ends the wait early if the caller gives up.
	SpeakWait(ctx context.Context, text string)
	// StopSpeaking silences the voice now: what is queued is dropped and the line
	// in flight is cut off mid-word. This is a human pressing stop, not an agent
	// changing its mind, which is why it is harsher than anything Speak does.
	StopSpeaking()
	// ReadWait speaks one region of a read-aloud and returns when it has been
	// heard. It is SpeakWait without the sentence-length cap, because a passage a
	// human asked for is read whole and truncating it would drop the end of it.
	ReadWait(ctx context.Context, text string)
}

// Driver synthesises input on the desktop: the pointer moves, buttons click,
// keys are pressed, as if the person at the keyboard did it. Satisfied by
// internal/hand in cmd/agentbox.
//
// It is an interface for the same reason Sounder and Presenter are - this package
// stays free of X11 and testable with a fake - and it is nil on any machine that
// has no display to drive, which the handler reports rather than pretending.
type Driver interface {
	// Drive parses and runs a script, returning how many steps ran. park is the
	// pause latch (FR94): the driver asks it at every point where handing the
	// desktop back is safe, and abandons the rest of the script when it errors.
	// It is never nil - a daemon with no latch passes one that is never blocked -
	// so an implementation does not have to check.
	Drive(script string, speed float64, wpm int, park Park) (int, error)
}

// Park is the boundary check a running script parks on, satisfied by the control
// latch and by internal/hand's identical interface (the two never import each
// other; cmd/agentbox is where they meet).
type Park interface {
	Blocked() bool
	Wait() error
}

// pauseGate is the latch as one script sees it. It carries the caller's context
// so a parked script dies with the connection that asked for it rather than
// holding a slot for an agent that has gone.
type pauseGate struct {
	c   *control
	ctx context.Context
}

func (g pauseGate) Blocked() bool {
	paused, _ := g.c.Paused()
	return paused
}

func (g pauseGate) Wait() error { return g.c.gate(g.ctx, pauseWait) }

// Presence reports the desktop signals behind FR29. IdleFor backs FR44 (a
// toast that lapses while the user is idle is flagged "missed while away")
// and the hold-when-idle chime gate. FullscreenActive and DesktopDND back the
// auto-DND knobs: a focused fullscreen app, or the desktop's own
// do-not-disturb, each count as DND. A nil Presence, or one whose platform
// monitor is unavailable (Wayland, headless, no GNOME), reports all three
// false - the safe default, since over-marking is harmless but suppressing a
// real interruption is not.
type Presence interface {
	IdleFor(d time.Duration) bool
	FullscreenActive() bool
	DesktopDND() bool
}

// Config carries the daemon policy knobs; zero value gets defaults. All
// fields except RetentionAge/KeepLevel/StartInDnd apply live via SetPolicy.
type Config struct {
	ToastDuration time.Duration // auto-dismiss for info/success notify
	RetentionAge  time.Duration // history eviction age (FR10)
	KeepLevel     proto.Level   // this level and above is never evicted
	UndoGrace     time.Duration // FR28 window; 0 disables (answers deliver at once)
	VetoWindow    time.Duration // FR22 default countdown when a veto omits timeout_s
	IdleAfter     time.Duration // FR44/FR29 presence.idle_after_s; 0 disables the missed-while-away marker
	CallerGone    time.Duration // FR45 grace before a disconnected card auto-dismisses

	HoldWhenIdle      bool // FR29: hold chimes and pause escalation while idle, one summary chime on return
	FullscreenAutoDnd bool // FR29: a focused fullscreen app counts as DND
	RespectDesktopDnd bool // FR29: the desktop's own do-not-disturb counts as DND

	EscalationInterval time.Duration // FR9 earcon replay cadence
	EscalationCount    int           // then go silent, stay visible
	UrgentInterval     time.Duration // urgent insists harder
	DndBlocksUrgent    bool          // FR11 inverted so the zero value (urgent breaks through) is the default
	ActionsDisabled    bool          // FR32 kill switch, inverted so the zero value (action buttons on) is the default
	StartInDnd         bool

	// Sync (FR83). SyncWaitMax bounds a PARKED MCP CALL and nothing else: the
	// client aborts a tool call that has been silent for 1800s (measured), so a
	// wait that promised more would be a lie the transport tells. SyncWaitWarn is
	// the only coordination toast; a signal wait will never warn, because
	// listening is the intended steady state. SyncHolderGoneGrace is how long an
	// orphaned hold waits before its dead pid counts as proof the work is over.
	SyncWaitMax         time.Duration
	SyncWaitWarn        time.Duration // 0 disables
	SyncHolderGoneGrace time.Duration
	// Signal retention (FR83 slice 3), per topic and by age, whichever trims
	// first. Neither has an "off": a signal table that grew forever would be a leak
	// with no upper bound, and a cursor that fell off a trimmed edge is reported
	// rather than served silently, so finite retention costs honesty and not
	// correctness.
	SignalKeep     int
	SignalKeepDays int
	// SharedMaxBytes caps one shared value (FR83 slice 4). A knob where the signal
	// cap is a constant, because a value is state a workflow shapes while a signal is
	// an event agentbox shapes - but it is a cap either way: a blackboard with no
	// size limit is a memory leak with a JSON schema, and the idiom for anything
	// bigger is a file path.
	SharedMaxBytes int
}

func (c *Config) fill() {
	if c.ToastDuration == 0 {
		c.ToastDuration = 6 * time.Second
	}
	if c.RetentionAge == 0 {
		c.RetentionAge = 30 * 24 * time.Hour
	}
	if c.KeepLevel == "" {
		c.KeepLevel = proto.LevelWarning
	}
	if c.EscalationInterval == 0 {
		c.EscalationInterval = time.Minute
	}
	if c.EscalationCount == 0 {
		c.EscalationCount = 5
	}
	if c.UrgentInterval == 0 {
		c.UrgentInterval = 20 * time.Second
	}
	if c.VetoWindow == 0 {
		c.VetoWindow = 15 * time.Second
	}
	if c.CallerGone == 0 {
		c.CallerGone = 4 * time.Second
	}
	if c.SyncWaitMax == 0 {
		// Under the client's measured 1800s idle cap with room to spare, so a
		// parked wait ends as an honest timed_out the agent can re-arm rather than
		// as a transport error it cannot read (see keepalive.go).
		c.SyncWaitMax = 25 * time.Minute
	}
	// SyncWaitWarn is deliberately NOT defaulted here: 0 means "never warn", which
	// is what the config file's own 0 means, and config.Default supplies the real
	// default. Filling it in would make an explicit 0 impossible to express.
	if c.SyncHolderGoneGrace == 0 {
		c.SyncHolderGoneGrace = 5 * time.Second
	}
}

// progressEntry is a live progress report plus daemon bookkeeping (FR21).
// held ties the report to the connection that created it (the CLI pipe): when
// that connection drops before Done, the report is reaped. updated stamps the
// last touch so the stale reaper can drop an orphan whose caller died.
type progressEntry struct {
	st      ProgressState
	updated time.Time
	held    bool
}

// graced is an answer waiting out its undo window.
type graced struct {
	id    string
	out   store.Outcome
	text  string
	until time.Time
	timer *time.Timer
}

type Daemon struct {
	cfg Config
	log *slog.Logger
	st  *store.Store
	snd Sounder
	ui  Presenter
	prs Presence // FR44 idle signal; nil means never idle
	drv Driver   // synthetic input; nil means this daemon cannot drive the desktop
	// sched owns WHEN an assignment runs (M12/FR82). It is nil in a daemon built
	// without a store, which is every test that does not care about assignments.
	sched *scheduler

	// OnQuit, when set, is invoked by the agentbox.v1.quit method.
	// OnDndChange, when set, hears every DND flip (tray checkbox sync).
	// OnMuteChange, when set, hears the muted-agent set after every change
	// (tray badge sync, FR47).
	OnQuit       func()
	OnDndChange  func(on bool)
	OnMuteChange func(agents []string)

	mu        sync.Mutex
	current   *proto.Item
	queue     []*proto.Item
	waiters   map[string]chan proto.Result
	timers    map[string]*time.Timer
	expiries  map[string]time.Time // timed questions' expiry deadlines, for the View countdown
	graced    *graced
	dnd       bool
	muted     map[string]bool // FR47: agents silenced in memory, cleared on restart
	gone      map[string]bool // FR45: blocking items whose caller socket has dropped
	dismissAt time.Time       // auto-dismiss deadline of the current toast
	escTimer  *time.Timer     // escalation for the current item (FR9)
	escCount  int

	// FR29 presence state, refreshed from the monitor (never queried under
	// d.mu). idle gates chimes/escalation; autoDnd (a focused fullscreen app
	// or the desktop's own DND) gates new-item display like manual DND;
	// chimeHeld records that a chime was suppressed and is owed as one
	// summary chime once the user returns / the suppression lifts.
	idle      bool
	autoDnd   bool
	chimeHeld bool

	// FR21 live progress reports, keyed by id and held in creation order for a
	// stable display. progUpdated stamps the last touch so the reaper can drop
	// a report whose caller died without sending Done.
	progress  map[string]*progressEntry
	progOrder []string

	// The artifact channel (M10, artifacts.go). Its own lock: an artifact event is
	// not an item, so it must not queue behind whatever d.mu is doing.
	art artifactHub

	// The walkthrough submission channel (FR58, wthub.go): agents parked on
	// a review the human has not finished yet. Same isolation as art.
	wt wtHub

	// The read-aloud transport (aloud.go). Its own lock for the same reason:
	// somebody pressing pause must not wait on whatever d.mu is doing.
	aloud *aloud
	// control is the desktop handover strip (FR74). Its own lock, like aloud:
	// a blocked request must not hold the daemon while the human decides.
	control *control

	// roster is who else is here (FR83). Its own lock for the same reason as
	// the others, and more sharply: an attach is a call parked for the whole
	// life of a session, so it must never be holding d.mu while it waits.
	roster *roster
	// locks is who is taking turns over what (FR83 slice 2). Same isolation, and
	// the same reason again: an acquire is parked for as long as the caller is
	// patient, and holding d.mu across that would stop the daemon.
	locks *locks
	// signals is how agents wake and message each other (FR83 slice 3). Same
	// isolation once more, and it is the most parked of the three: await is the
	// design's one sanctioned way for an agent to spend a turn doing nothing.
	signals *signals
	// shared is the blackboard (FR83 slice 4). The only one of the four that never
	// parks: get, set and delete are all one statement against the store, so it needs
	// no isolation from d.mu for latency - it has its own mutex for the same reason
	// the others do, which is that its observers point back into the roster.
	shared *shared

	closing atomic.Bool // FR45: set at shutdown so a teardown disconnect is not shown as "caller gone"
}

// SetPolicy applies the live-reloadable knobs (config file watch).
func (d *Daemon) SetPolicy(cfg Config) {
	cfg.fill()
	d.mu.Lock()
	d.cfg.ToastDuration = cfg.ToastDuration
	d.cfg.UndoGrace = cfg.UndoGrace
	d.cfg.VetoWindow = cfg.VetoWindow
	d.cfg.EscalationInterval = cfg.EscalationInterval
	d.cfg.EscalationCount = cfg.EscalationCount
	d.cfg.UrgentInterval = cfg.UrgentInterval
	d.cfg.DndBlocksUrgent = cfg.DndBlocksUrgent
	d.cfg.ActionsDisabled = cfg.ActionsDisabled
	d.cfg.IdleAfter = cfg.IdleAfter
	d.cfg.HoldWhenIdle = cfg.HoldWhenIdle
	d.cfg.FullscreenAutoDnd = cfg.FullscreenAutoDnd
	d.cfg.RespectDesktopDnd = cfg.RespectDesktopDnd
	d.mu.Unlock()
}

// SetPresence wires the idle signal (FR44). Called once at startup with the
// platform monitor; nil-safe, so tests and headless runs simply never mark
// items missed-while-away.
func (d *Daemon) SetPresence(p Presence) {
	d.mu.Lock()
	d.prs = p
	d.mu.Unlock()
}

// driveShape summarises a script as the sequence of step names in it
// ("window,move,click,type"), which is enough to see what an agent did to the
// desktop and impossible to recover a typed secret from.
func driveShape(script string) string {
	var ops []string
	for line := range strings.SplitSeq(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		op, _, _ := strings.Cut(line, " ")
		ops = append(ops, strings.ToLower(op))
		if len(ops) == 40 {
			ops = append(ops, "...")
			break
		}
	}
	return strings.Join(ops, ",")
}

// SetDriver wires synthetic input. Called once at startup on a machine with a
// display; nil-safe, so a headless daemon refuses to drive rather than crashing.
func (d *Daemon) SetDriver(drv Driver) {
	d.mu.Lock()
	d.drv = drv
	d.mu.Unlock()
}

// computePresence asks the monitor for the FR29 signals enabled by config and
// folds them into (idle, autoDnd). It never holds d.mu across a monitor call -
// IdleFor/FullscreenActive/DesktopDND can do an X11 round trip or a
// subprocess, which must not block the daemon lock.
func (d *Daemon) computePresence() (idle, autoDnd bool) {
	d.mu.Lock()
	prs := d.prs
	hold := d.cfg.HoldWhenIdle
	idleAfter := d.cfg.IdleAfter
	fsOn := d.cfg.FullscreenAutoDnd
	ddOn := d.cfg.RespectDesktopDnd
	d.mu.Unlock()
	if prs == nil {
		return false, false
	}
	if hold && idleAfter > 0 {
		idle = prs.IdleFor(idleAfter)
	}
	if fsOn && prs.FullscreenActive() {
		autoDnd = true
	}
	if ddOn && prs.DesktopDND() {
		autoDnd = true
	}
	return idle, autoDnd
}

// DndState is the honest answer to "will anything reach me right now": the
// switch, whether a rule is holding things anyway, and why.
//
// It exists because `agentbox dnd status` used to print "off" while every card was
// being held by FR29's auto-DND - a focused fullscreen app, or the desktop's own
// do-not-disturb - and a status line that says "off" while nothing appears on
// screen is indistinguishable from a broken install. The reason is the whole
// value: it names the rule, so the human can turn that rule off instead of
// wondering.
func (d *Daemon) DndState() (on, auto bool, reason string) {
	d.mu.Lock()
	on = d.dnd
	prs := d.prs
	fsOn, ddOn := d.cfg.FullscreenAutoDnd, d.cfg.RespectDesktopDnd
	d.mu.Unlock()

	var why []string
	if on {
		why = append(why, "do-not-disturb is switched on")
	}
	if prs != nil {
		if fsOn && prs.FullscreenActive() {
			auto = true
			why = append(why, "a fullscreen window is focused ([presence] fullscreen_auto_dnd)")
		}
		if ddOn && prs.DesktopDND() {
			auto = true
			why = append(why, "the desktop is in do-not-disturb ([presence] respect_desktop_dnd)")
		}
	}
	return on, auto, strings.Join(why, "; ")
}

// PresencePoll refreshes the cached idle/autoDnd state and acts on the
// suppression-lifted edge (FR29): when the user returns from idle or exits
// fullscreen / desktop DND, any item held silent reveals and a single summary
// chime plays - "one summary chime and the oldest pending card", never a
// metronome in an empty room. Called on a ticker while there is pending work;
// a quiet daemon skips the monitor entirely. handleSubmit refreshes the same
// state on each arrival, so a card that pops mid-presentation is still gated.
func (d *Daemon) PresencePoll() {
	d.mu.Lock()
	busy := d.current != nil || len(d.queue) > 0 || d.chimeHeld
	d.mu.Unlock()
	if !busy {
		return
	}
	idle, autoDnd := d.computePresence()

	d.mu.Lock()
	wasSuppressed := d.idle || d.autoDnd
	d.idle, d.autoDnd = idle, autoDnd
	nowSuppressed := idle || autoDnd
	if wasSuppressed == nowSuppressed {
		d.mu.Unlock()
		return
	}
	var summon *proto.Item
	if !nowSuppressed {
		if d.current == nil {
			d.advanceLocked() // a held item can take the screen again
		}
		if d.chimeHeld && d.current != nil {
			summon = d.current // the one summary chime owed for the return
		}
		d.chimeHeld = false // owed chime is now spent, or there was nothing to show
	}
	view := d.viewLocked()
	d.mu.Unlock()

	if summon != nil {
		d.log.Info("presence.returned", "component", "daemon", "item_id", summon.ID, "waiting", view.Waiting)
	}
	d.ui.Present(view)
	if summon != nil {
		// First announcement for this one: it was held silent while you were idle.
		d.snd.Play(sound.ClassFor(summon))
		d.snd.Speak(summon.Speak)
	}
}

// New restores pending items from the store (NFR7), prunes old history
// (FR10) and returns a daemon ready to handle calls.
func New(cfg Config, log *slog.Logger, st *store.Store, snd Sounder, ui Presenter) (*Daemon, error) {
	cfg.fill()
	d := &Daemon{
		cfg:      cfg,
		log:      log,
		st:       st,
		snd:      snd,
		ui:       ui,
		dnd:      cfg.StartInDnd,
		waiters:  make(map[string]chan proto.Result),
		timers:   make(map[string]*time.Timer),
		expiries: make(map[string]time.Time),
		muted:    make(map[string]bool),
		gone:     make(map[string]bool),
		progress: make(map[string]*progressEntry),
	}
	d.aloud = newAloud(snd, log)
	d.control = newControl(log)
	d.roster = newRoster(log)
	// The two facts a self-report cannot supply. Both live behind their own
	// locks, and the roster only ever reads them, so this cannot deadlock
	// against either subsystem.
	d.roster.SetObservers(d.askingKeys, d.drivingKey)
	d.locks = newLocks(log)
	d.locks.SetPolicy(cfg.SyncWaitWarn, cfg.SyncHolderGoneGrace)
	// Locks read the roster (is this session announced, and who is it) and the
	// waiters map (is the holder parked on a question to the human), and write
	// back only two things: a repaint, and a warning toast. The roster in turn
	// asks the table for holds and waits when it renders a row. Each direction
	// takes only its own lock, which is what keeps the pair from deadlocking.
	d.locks.SetObservers(d.roster.announced, d.roster.agentOf, d.askingKeys, d.lockWarn, d.roster.changed)
	// The forgotten-pause card (FR94). It goes through the item path rather than
	// the strip, because the strip is already saying this to a screen he is not
	// looking at - the card is for the one who walked away, and it chimes.
	d.control.SetNag(func(title, body string, actions []proto.Action) {
		d.surfaceNotify(proto.LevelWarning, proto.Identity{Agent: "agentbox"}, title, body, actions...)
	})
	// An attach dropping is a session dying, and its locks must not silently
	// free the resources its work may still be using (locks.go, the orphan rule).
	// The signal hub is told too, so a machine that has run all day is not holding
	// the topics of sessions that ended hours ago - wired below, once it exists.
	d.roster.SetOnGone(d.locks.SessionGone)
	d.roster.SetLocks(d.locks.rows)
	// Signals (FR83 slice 3). The store is the whole point - a signal is delivered
	// whether or not anybody was waiting - so the subsystem is built with it rather
	// than falling back to a hub that would lose a hand-off on the first restart.
	d.signals = newSignals(log)
	d.signals.SetStore(st)
	d.signals.SetRetention(cfg.SignalKeep, cfg.SignalKeepDays)
	d.signals.SetObservers(d.roster.announced, d.roster.changed)
	// What a session is parked on, so a listening row says so instead of looking
	// hung. Read-only from the roster's side, like every other observer.
	d.roster.SetListens(d.signals.listens)
	// Now that both exist, a departure reaches them both. Chained here rather than
	// as a second setter: "what happens when a session goes" is one list, and two
	// registration points is how one of them gets forgotten.
	sessionGone := d.locks.SessionGone
	d.roster.SetOnGone(func(key string) {
		sessionGone(key)
		d.signals.forgetReceived(key)
	})
	// The built-in topics, both of them the same mechanism: a join, an announce or
	// a departure is itself a signal, so an agent that is genuinely idle can park on
	// its area instead of polling the roster. A lock changing hands is the other
	// one - see the note on postLockSignal.
	d.roster.SetPost(d.postSignal)
	d.locks.SetPost(d.postSignal)
	// Shared values (FR83 slice 4), the last primitive. It reads the roster for two
	// different questions - has this session announced (the write gate) and does this
	// session still exist (whether a claim is orphaned) - and writes back only through
	// the signal hub, which is the design's one wake mechanism.
	d.shared = newShared(log)
	d.shared.SetStore(st)
	d.shared.SetMaxBytes(cfg.SharedMaxBytes)
	d.shared.SetObservers(d.roster.announced, d.roster.present, d.postSignal, d.roster.changed)
	// The scheduler is built here but not STARTED here: it wants a Runner, and
	// the surface that can carry an assignment out is wired after the daemon
	// exists (SetRunner, then StartAssignments). A daemon that never gets one
	// still records every due run as failed-with-a-reason rather than quietly
	// not running it.
	d.sched = newScheduler(st, log)
	d.sched.changed = d.assignmentsChanged
	if n, err := st.Prune(cfg.RetentionAge, cfg.KeepLevel); err != nil {
		log.Error("store.prune_failed", "component", "daemon", "err", err.Error())
	} else if n > 0 {
		log.Info("store.pruned", "component", "daemon", "evicted", n)
	}
	pending, err := st.Pending()
	if err != nil {
		return nil, err
	}
	for i := range pending {
		it := pending[i].Item
		d.queue = append(d.queue, &it)
		d.log.Info("item.restored", "component", "daemon", "item_id", it.ID, "kind", string(it.Kind))
	}
	d.mu.Lock()
	d.advanceLocked()
	view := d.viewLocked()
	d.mu.Unlock()
	d.ui.Present(view)
	return d, nil
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not short-read on Linux; if it ever does, a
		// time-based suffix keeps IDs unique rather than colliding on zeros.
		binary.BigEndian.PutUint64(b[:], uint64(time.Now().UnixNano()))
	}
	return "k" + hex.EncodeToString(b[:])
}

// Handle is the proto.Handler for the socket server. A panic in any handler
// is recovered here and turned into a logged CodeInternal error, so one bad
// request can never take the daemon down or leave its caller hanging.
func (d *Daemon) Handle(ctx context.Context, method string, params json.RawMessage) (result any, rpcErr *proto.RPCError) {
	d.log.Debug(logging.EvIPCCall, "component", "daemon", "method", method)
	defer func() {
		if r := recover(); r != nil {
			d.log.Error(logging.EvPanic, "component", "daemon", "method", method, "panic", fmt.Sprint(r))
			result, rpcErr = nil, &proto.RPCError{Code: proto.CodeInternal, Message: "internal error handling " + method}
		}
	}()
	switch method {
	case proto.MethodNotify:
		return d.handleSubmit(ctx, params, false)
	case proto.MethodAsk:
		return d.handleSubmit(ctx, params, true)
	case proto.MethodCancel:
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `cancel wants {"id": "<item id>"}`}
		}
		if !d.resolve(p.ID, store.StateCancelled, store.Outcome{}) {
			return nil, &proto.RPCError{Code: proto.CodeItemNotFound, Message: "no pending item " + p.ID}
		}
		return map[string]bool{"ok": true}, nil
	case proto.MethodDismiss:
		var p proto.DismissParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &proto.RPCError{Code: proto.CodeInvalidParams,
					Message: `dismiss wants {"id": "<item id>"} or {"all": true}`}
			}
		}
		return d.DismissItems(p)
	case proto.MethodList:
		items, err := d.st.Pending()
		if err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
		}
		return map[string]any{"pending": items}, nil
	case proto.MethodStatus:
		d.mu.Lock()
		pending := len(d.queue)
		if d.current != nil {
			pending++
		}
		d.mu.Unlock()
		// The version of the build that is actually SERVING, which is not the same
		// question as what the client on the other end of the socket happens to be.
		// `make deploy` replaces the binary and restarts, and the only way to know
		// the restart took is to ask the daemon what it is.
		return map[string]any{"pending": pending, "version": version.Get()}, nil
	case proto.MethodInbox:
		d.ui.ShowApp("inbox")
		return map[string]bool{"ok": true}, nil
	case proto.MethodApp:
		var p struct {
			Tab string `json:"tab"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `app wants {"tab": "inbox|stats|progress"} or {}`}
			}
		}
		d.ui.ShowApp(p.Tab)
		return map[string]bool{"ok": true}, nil
	case proto.MethodSpeak:
		// A line with no item behind it: something worth hearing that is not worth
		// a card. It goes through the same voice, so quiet hours and the [speech]
		// switch apply, but it never touches the queue, the store or the inbox -
		// saying something out loud is not an interruption to be triaged.
		var req proto.SpeakRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "speak wants a {text} object"}
		}
		if strings.TrimSpace(req.Text) == "" {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "speak wants a non-empty text"}
		}
		if req.Wait {
			// The caller wants the answer when the line has been heard. Blocking here
			// holds up nothing else: every connection is served on its own goroutine,
			// and the queue behind the voice is where order is kept.
			d.snd.SpeakWait(ctx, req.Text)
			d.log.Info("speech.requested", "component", "daemon", "chars", len(req.Text), "waited", true)
			return proto.SpeakResult{OK: true, Waited: true}, nil
		}
		d.snd.Speak(req.Text)
		d.log.Info("speech.requested", "component", "daemon", "chars", len(req.Text))
		return proto.SpeakResult{OK: true}, nil
	case proto.MethodAloud:
		// The human asking to hear one region of what is on screen. Unlike speak
		// the text is not capped - a passage somebody asked for is read whole - and
		// unlike the first version of this it is not split either, because one
		// utterance is the only shape that keeps the voice intact (FR72).
		var req proto.AloudRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `aloud wants {"action": "start|stop|state", "region": "...", "text": "..."}`}
		}
		if req.Action == proto.AloudStart && strings.TrimSpace(req.Text) == "" {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "aloud start wants a non-empty text"}
		}
		return d.aloud.Command(req.Action, req.Region, req.Text), nil
	case proto.MethodControl:
		// An agent asking for the desktop, then saying what it is doing with it
		// (FR74). request BLOCKS while the human decides, which is why ctx matters
		// here: the connection dying is the run dying, so the strip can never claim
		// hands-off for an agent that is gone.
		var req proto.ControlRequestParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `control wants {"action": "request|activity|release|state", ...}`}
		}
		id := req.Identity
		switch req.Action {
		case proto.ControlRequest:
			if strings.TrimSpace(req.Reason) == "" {
				return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "control request wants a reason: it is what the human reads before allowing it"}
			}
			return d.control.Request(ctx, id, req.Reason, time.Duration(req.WindowS)*time.Second), nil
		case proto.ControlActivity:
			// One tool, two readers (FR83). set_activity already meant "say what
			// you are doing", so it stays one tool and now writes the roster
			// ALWAYS and the hands-off strip additionally, when this caller
			// happens to hold the desktop. An agent that is not driving used to
			// get silence from this verb; now it gets a live line on the board.
			d.roster.Activity(proto.SyncActivityParams{Identity: id, Activity: req.Activity})
			return d.control.Activity(id, req.Activity), nil
		case proto.ControlRelease:
			return d.control.Release(id), nil
		case proto.ControlPause:
			// No identity check, and that is the point (FR94): this verb is the
			// human's, and it reaches the daemon from his strip, his hotkey and his
			// shell. The socket is his own, so anything that can call this is
			// already him.
			return d.control.Pause(req.Reason), nil
		case proto.ControlResume:
			return d.control.Resume(req.Reason), nil
		default:
			return d.control.State(), nil
		}
	case proto.MethodDrive:
		// The agent acting on the desktop instead of asking about it. It goes
		// through the daemon rather than straight to X so there is one place that
		// knows it happened: the log below is the answer to "what moved my mouse?".
		var req proto.DriveRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `drive wants a {"script": "..."} object`}
		}
		if strings.TrimSpace(req.Script) == "" {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "drive wants a non-empty script"}
		}
		d.mu.Lock()
		drv := d.drv
		d.mu.Unlock()
		if drv == nil {
			return nil, &proto.RPCError{Code: proto.CodeInternal,
				Message: "this daemon has no display to drive (X11 with XTEST is required)"}
		}
		// The shape of the script, never its text: a `type` step can carry a
		// password, and a log that would leak one is a log nobody can keep.
		shape := driveShape(req.Script)
		gate := pauseGate{c: d.control, ctx: ctx}
		// Before the first step as well as between them (FR94): a script that
		// arrived while the desktop was already latched must not get one free
		// click in before it notices.
		if gate.Blocked() {
			d.log.Info(logging.EvControl, "component", "daemon", "control", "parked",
				"shape", shape)
			if err := gate.Wait(); err != nil {
				return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: err.Error()}
			}
		}
		n, err := drv.Drive(req.Script, req.Speed, req.WPM, gate)
		if err != nil {
			d.log.Warn("input.drive_failed", "component", "daemon", "shape", shape, "err", err.Error())
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: err.Error()}
		}
		d.log.Info("input.driven", "component", "daemon", "steps", n, "shape", shape)
		return proto.DriveResult{Steps: n}, nil
	case proto.MethodSummon:
		d.ui.Summon()
		return map[string]bool{"ok": true}, nil
	case proto.MethodPanel:
		var p struct {
			Action string `json:"action"` // toggle (default) | show | hide | state
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "panel wants {action?: toggle|show|hide|state}"}
			}
		}
		switch p.Action {
		case "show":
			d.ui.ShowPanel()
		case "hide":
			d.ui.HidePanel()
		case "state":
			// no change; the reply says where it is
		case "", "toggle":
			d.ui.TogglePanel()
		default:
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "panel action must be toggle, show, hide or state"}
		}
		return map[string]bool{"open": d.ui.PanelOpen()}, nil
	case proto.MethodShow:
		var req proto.ShowRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "show wants a {path|content, title?, watch?} object"}
		}
		if req.Path == "" && req.Content == "" {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "show needs a path or content"}
		}
		d.ui.ShowDocument(req)
		return map[string]bool{"ok": true}, nil
	case proto.MethodStats:
		var p struct {
			SinceMS int64 `json:"since_ms"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `stats wants {"since_ms": <epoch ms>} or {}`}
			}
		}
		st, err := d.st.Stats(time.UnixMilli(p.SinceMS))
		if err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
		}
		return st, nil
	case proto.MethodQuit:
		if d.OnQuit == nil {
			return nil, &proto.RPCError{Code: proto.CodeInternal, Message: "quit not wired"}
		}
		go d.OnQuit()
		return map[string]bool{"ok": true}, nil
	case proto.MethodDnd:
		var p struct {
			Set *bool `json:"set"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `dnd wants {"set": true|false} or {}`}
		}
		if p.Set != nil {
			d.DndSet(*p.Set)
		}
		on, auto, reason := d.DndState()
		return map[string]any{"enabled": on, "auto_held": auto, "reason": reason}, nil
	case proto.MethodMute:
		var p struct {
			Agent  string `json:"agent"`
			Unmute bool   `json:"unmute"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `mute wants {"agent": "<name>", "unmute"?: bool} or {} to list`}
		}
		if p.Agent != "" {
			if p.Unmute {
				d.Unmute(p.Agent)
			} else {
				d.Mute(p.Agent)
			}
		}
		return map[string]any{"muted": d.MutedAgents()}, nil
	// Sync (FR83, sync.go). attach is the odd one: it BLOCKS for the session's
	// whole life, because the call being open IS the presence. ctx is therefore
	// load-bearing exactly as it is for control - the connection dying is the
	// agent dying, and the roster must not keep a row for somebody who is gone.
	case proto.MethodSyncAttach:
		var p proto.SyncAttachParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `sync_attach wants {"identity": {...}, "cwd": "...", "pid": N}`}
		}
		if p.Area == "" {
			p.Area = DeriveArea(p.Cwd)
		}
		return d.roster.Attach(ctx, p)
	case proto.MethodSyncAnnounce:
		var p proto.SyncAnnounceParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `sync_announce wants {"identity": {...}, "purpose": "..."}`}
		}
		// Derived here exactly as it is for the attach, and for the same reason: the
		// area is a fact about where the caller stands, not something a model should
		// have to state. An announce that arrives before its session's attach - which
		// is every hook-driven announce - would otherwise create a row with no area,
		// and an area-filtered read cannot see one of those (FR90).
		if p.Area == "" {
			p.Area = DeriveArea(p.Cwd)
		}
		return d.roster.Announce(p)
	case proto.MethodSyncActivity:
		var p proto.SyncActivityParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `sync_activity wants {"identity": {...}, "activity": "..."}`}
		}
		return d.roster.Activity(p)
	case proto.MethodSyncList:
		var p proto.SyncListParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: `sync_list wants {"area": "...", "project": "..."} or {}`}
			}
		}
		return d.roster.List(p)
	// Locks (FR83 slice 2, locks.go). acquire parks in a FIFO queue, so ctx is
	// load-bearing the same way it is for a card: the caller going away must take
	// its place in the queue with it.
	case proto.MethodSyncLock:
		p, rpcErr := lockParams(params, "sync_lock")
		if rpcErr != nil {
			return nil, rpcErr
		}
		return d.locks.Acquire(ctx, p, d.cfg.SyncWaitMax)
	case proto.MethodSyncTryLock:
		p, rpcErr := lockParams(params, "sync_try_lock")
		if rpcErr != nil {
			return nil, rpcErr
		}
		return d.locks.Try(p)
	case proto.MethodSyncUnlock:
		p, rpcErr := lockParams(params, "sync_unlock")
		if rpcErr != nil {
			return nil, rpcErr
		}
		return d.locks.Release(p)
	case proto.MethodSyncBreakLock:
		// The human's verb, not an agent's: it arrives from the Agents surface.
		p, rpcErr := lockParams(params, "sync_break_lock")
		if rpcErr != nil {
			return nil, rpcErr
		}
		return d.locks.Break(p.Name), nil
	case proto.MethodSyncLocks:
		return proto.SyncLocksResult{OK: true, Locks: d.locks.Snapshot()}, nil
	// Signals (FR83 slice 3, signals.go). await parks like acquire does, so ctx is
	// load-bearing again: a caller that goes away must take its registration with
	// it, or the hub fans out to a listener that will never read.
	case proto.MethodSyncPost:
		p, rpcErr := signalParams[proto.SyncPostParams](params, "sync_post",
			`{"identity": {...}, "topic": "kind:scope", "data"?: {...}}`)
		if rpcErr != nil {
			return nil, rpcErr
		}
		return d.signals.Post(p)
	case proto.MethodSyncAwait:
		p, rpcErr := signalParams[proto.SyncAwaitParams](params, "sync_await",
			`{"identity": {...}, "topics": ["kind:scope", "done:*"], "after_seq"?: N, "timeout_s"?: N}`)
		if rpcErr != nil {
			return nil, rpcErr
		}
		return d.signals.Await(ctx, p, d.cfg.SyncWaitMax)
	// Shared values (FR83 slice 4, shared.go). One method for all three operations,
	// and no ctx: none of them blocks, so there is nothing for a caller going away to
	// cancel. A write that landed has landed - that is the difference between a
	// blackboard and a hand-off.
	case proto.MethodSyncShared:
		p, rpcErr := sharedParams(params)
		if rpcErr != nil {
			return nil, rpcErr
		}
		return d.shared.Handle(p)
	case proto.MethodProgress:
		var u proto.ProgressUpdate
		if err := json.Unmarshal(params, &u); err != nil {
			return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "progress wants a {id?, title?, status?, percent?, indeterminate?, done?, error?} object: " + err.Error()}
		}
		return d.Progress(ctx, u)
	// The artifact channel (M10, artifacts.go). Wait blocks like ask does; read
	// takes whatever arrived while the agent was busy.
	case proto.MethodArtifactWait:
		req, rpcErr := artifactWaitParams(params, "artifact_wait")
		if rpcErr != nil {
			return nil, rpcErr
		}
		return d.AwaitArtifact(ctx, req), nil
	case proto.MethodArtifactRead:
		req, rpcErr := artifactWaitParams(params, "artifact_read")
		if rpcErr != nil {
			return nil, rpcErr
		}
		return d.ReadArtifact(req), nil
	case proto.MethodWalkthroughCreate:
		return d.walkthroughCreate(params)
	case proto.MethodWalkthroughAmend:
		return d.walkthroughAmend(params)
	case proto.MethodWalkthroughRepair:
		return d.walkthroughRepair(params)
	case proto.MethodWalkthroughAwait:
		var req proto.WalkthroughAwait
		if len(params) > 0 {
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, &proto.RPCError{Code: proto.CodeInvalidParams,
					Message: `walkthrough_await wants {"id"?, "timeout_s"?, "identity"?} or {}`}
			}
		}
		return d.AwaitWalkthrough(ctx, req), nil
	case proto.MethodWalkthroughRead:
		return d.walkthroughRead(params)
	case proto.MethodWalkthroughList:
		return d.walkthroughList(params)
	case proto.MethodWalkthroughDelete:
		return d.walkthroughDelete(params)
	case proto.MethodWalkthroughOpen:
		return d.walkthroughOpen(params)
	// The assignment CRUD (M12/FR82, assignmentsrpc.go): an MCP surface before
	// it is a UI, because the agent authors assignments with the human.
	case proto.MethodAssignmentList:
		return d.assignmentList()
	case proto.MethodAssignmentRead:
		return d.assignmentRead(params)
	case proto.MethodAssignmentSave:
		return d.assignmentSave(params)
	case proto.MethodAssignmentDelete:
		return d.assignmentDelete(params)
	case proto.MethodAssignmentRun:
		return d.assignmentRun(params)
	case proto.MethodAssignmentRuns:
		return d.assignmentRuns(params)
	default:
		return nil, &proto.RPCError{Code: proto.CodeMethodNotFound,
			Message: method + " is not an agentbox.v1 method; see agentbox.v1.notify/ask/cancel/list/status"}
	}
}

func (d *Daemon) handleSubmit(ctx context.Context, params json.RawMessage, blocking bool) (any, *proto.RPCError) {
	var it proto.Item
	if err := json.Unmarshal(params, &it); err != nil {
		return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "params must be an item object: " + err.Error()}
	}
	if blocking && it.Kind == proto.KindNotify {
		return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "ask cannot carry kind notify; use agentbox.v1.notify"}
	}
	if !blocking && it.Kind != proto.KindNotify {
		return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: "notify only carries kind notify; use agentbox.v1.ask for questions"}
	}
	it.ID = newID()
	if it.Kind == proto.KindVeto && it.TimeoutS <= 0 {
		it.TimeoutS = int(d.cfg.VetoWindow.Seconds()) // zero-config window (FR22, [veto] default_window_s)
	}
	if err := it.Validate(); err != nil {
		return nil, &proto.RPCError{Code: proto.CodeInvalidParams, Message: err.Error()}
	}
	// Every item carries an identity, so this costs nothing and it is what keeps
	// the roster from lying (FR83). A session whose child predates sync has no
	// attach and no row; noticing it here is how a read learns to say partial
	// instead of implying the caller is alone.
	d.roster.SeenUnattached(it.Identity)
	if err := d.st.CreateItem(&it); err != nil {
		return nil, &proto.RPCError{Code: proto.CodeInternal, Message: err.Error()}
	}
	d.log.Info(logging.EvItemCreated, "component", "daemon", "item_id", it.ID,
		"kind", string(it.Kind), "level", string(it.EffectiveLevel()),
		"agent", it.Identity.Agent, "project", it.Identity.Project, "title", it.Title)

	// FR29: read the presence signals before placing the item, so a card that
	// arrives mid-presentation (fullscreen / desktop DND) is gated and a chime
	// that arrives while idle is held. computePresence never touches d.mu
	// during its monitor calls.
	idle, autoDnd := d.computePresence()

	var wait chan proto.Result
	d.mu.Lock()
	d.idle, d.autoDnd = idle, autoDnd
	if blocking {
		wait = make(chan proto.Result, 1)
		d.waiters[it.ID] = wait
		if it.TimeoutS > 0 {
			// Arrival-anchored deadline (a question expires even while
			// queued), recorded before the item can be presented so the
			// very first View already counts down the truth.
			d.expiries[it.ID] = time.Now().Add(time.Duration(it.TimeoutS) * time.Second)
		}
	}
	d.enqueueLocked(&it)
	visible := d.current != nil && d.current.ID == it.ID
	muted := d.muted[it.Identity.Agent]
	// A shown card whose chime is held by idle, or a card held silent by
	// (auto-)DND, owes one summary chime when the user returns.
	silentIdle := visible && d.cfg.HoldWhenIdle && idle
	suppressed := !visible && !muted && !d.breaksDndLocked(&it)
	manualDnd := d.dnd
	if silentIdle || suppressed {
		d.chimeHeld = true
	}
	view := d.viewLocked()
	d.mu.Unlock()
	d.ui.Present(view)
	switch {
	case visible && !silentIdle:
		d.snd.Play(sound.ClassFor(&it))
		d.snd.Speak(it.Speak)
	case silentIdle:
		d.log.Info("item.held_idle", "component", "daemon", "item_id", it.ID)
	case muted:
		d.log.Info("item.held_muted", "component", "daemon", "item_id", it.ID, "agent", it.Identity.Agent)
	case suppressed:
		// Which kind of do-not-disturb held it. Without this the log said "held"
		// while `agentbox dnd status` said "off", and the two together read as a bug
		// rather than as the auto-DND rules doing their job.
		d.log.Info("item.held_dnd", "component", "daemon", "item_id", it.ID,
			"manual", manualDnd, "auto", autoDnd)
	}

	if !blocking {
		return proto.Result{ID: it.ID}, nil
	}

	var timeout <-chan time.Time
	if it.TimeoutS > 0 {
		defer func() {
			d.mu.Lock()
			delete(d.expiries, it.ID)
			d.mu.Unlock()
		}()
		t := time.NewTimer(time.Duration(it.TimeoutS) * time.Second)
		defer t.Stop()
		timeout = t.C
	}
	select {
	case res := <-wait:
		if it.Kind == proto.KindSecret {
			return d.finishSecret(&it, res)
		}
		return res, nil
	case <-timeout:
		if it.Kind == proto.KindVeto {
			// The window elapsed unstopped: the action proceeds (FR22).
			if d.resolve(it.ID, store.StateExpired, store.Outcome{}) {
				return proto.Result{ID: it.ID, Vetoed: false}, nil
			}
			return <-wait, nil
		}
		dflt := it.Default
		if it.Kind == proto.KindForm {
			dflt = "" // forms have per-field defaults, no whole-form default
		}
		if d.resolve(it.ID, store.StateExpired, store.Outcome{Answer: dflt}) {
			return proto.Result{ID: it.ID, Answered: false, Answer: dflt, DefaultApplied: dflt != ""}, nil
		}
		// The user answered inside the undo grace; the real result is on
		// its way once the window closes.
		return <-wait, nil
	case <-ctx.Done():
		if d.closing.Load() {
			// Daemon teardown, not a caller drop: cancel quietly. Pending items
			// re-present on the next start (NFR7).
			d.resolve(it.ID, store.StateCancelled, store.Outcome{})
			return nil, &proto.RPCError{Code: proto.CodeShuttingDown, Message: "daemon shutting down"}
		}
		// The caller's socket dropped while the question was still open (FR45):
		// mark the card disconnected, let it auto-dismiss, and stop waiting on
		// a peer that is gone. Any answer now reaches history only (FR6).
		d.callerGone(it.ID)
		return proto.Result{ID: it.ID}, nil
	}
}

// breaksDnd reports whether an item may surface during do-not-disturb.
func (d *Daemon) breaksDndLocked(it *proto.Item) bool {
	// FR29: a focused fullscreen app or the desktop's own DND counts as DND,
	// alongside the manual toggle. The break-through rule is shared, so urgent
	// still pierces unless dnd_blocks_urgent is set (06-configuration.md).
	dnd := d.dnd || d.autoDnd
	return !dnd || (!d.cfg.DndBlocksUrgent && it.EffectiveLevel() == proto.LevelUrgent)
}

// displayableLocked reports whether an item may take the screen right now.
// A muted agent's items never surface (FR47): they queue silently and the
// caller can answer from the inbox, exactly like a DND-held item.
func (d *Daemon) displayableLocked(it *proto.Item) bool {
	return d.breaksDndLocked(it) && !d.muted[it.Identity.Agent]
}

// enqueueLocked places the item: urgent preempts the current card, which
// returns to the head of the queue (FR8). During DND, items below the
// break-through bar queue silently (FR11).
func (d *Daemon) enqueueLocked(it *proto.Item) {
	if it.EffectiveLevel() == proto.LevelUrgent && d.displayableLocked(it) &&
		d.current != nil && d.current.EffectiveLevel() != proto.LevelUrgent {
		d.queue = append([]*proto.Item{d.current}, d.queue...)
		d.stopTimerLocked(d.current.ID)
		d.setCurrentLocked(it)
		return
	}
	d.queue = append(d.queue, it)
	if d.current == nil {
		d.advanceLocked()
	}
}

func (d *Daemon) advanceLocked() {
	d.cancelEscalationLocked()
	d.dismissAt = time.Time{}
	for i, it := range d.queue {
		if d.displayableLocked(it) {
			d.queue = append(d.queue[:i], d.queue[i+1:]...)
			d.setCurrentLocked(it)
			return
		}
	}
	d.current = nil
}

func (d *Daemon) setCurrentLocked(it *proto.Item) {
	d.cancelEscalationLocked()
	d.dismissAt = time.Time{}
	d.current = it
	d.log.Info(logging.EvItemDisplayed, "component", "daemon", "item_id", it.ID, "waiting", len(d.queue))
	if it.Kind == proto.KindNotify {
		lvl := it.EffectiveLevel()
		if lvl == proto.LevelInfo || lvl == proto.LevelSuccess {
			id := it.ID
			d.dismissAt = time.Now().Add(d.cfg.ToastDuration)
			d.timers[id] = time.AfterFunc(d.cfg.ToastDuration, func() {
				d.toastExpired(id)
			})
			return
		}
	}
	if it.Kind == proto.KindVeto {
		// The on-card countdown deadline (FR22). The authoritative proceed
		// timer lives in handleSubmit so a veto still fires while the user
		// is away or in DND; this only drives the visible "proceeding in Ns".
		// Veto does not escalate - it resolves itself.
		d.dismissAt = time.Now().Add(time.Duration(it.TimeoutS) * time.Second)
		return
	}
	d.scheduleEscalationLocked(it)
}

// Escalation (FR9): replay the earcon for the displayed unanswered item at
// the configured cadence, up to the cap.
func (d *Daemon) scheduleEscalationLocked(it *proto.Item) {
	if !it.Blocking() && it.EffectiveLevel() != proto.LevelUrgent {
		return
	}
	interval := d.cfg.EscalationInterval
	if it.EffectiveLevel() == proto.LevelUrgent {
		interval = d.cfg.UrgentInterval
	}
	id := it.ID
	d.escCount = 0
	d.escTimer = time.AfterFunc(interval, func() { d.escalate(id, interval) })
}

func (d *Daemon) escalate(id string, interval time.Duration) {
	idle, autoDnd := d.computePresence()
	d.mu.Lock()
	d.idle, d.autoDnd = idle, autoDnd
	if d.current == nil || d.current.ID != id || d.graced != nil || d.dnd || d.autoDnd {
		// DND, fullscreen, or the desktop's own DND silences escalation (FR29),
		// like the manual toggle.
		d.mu.Unlock()
		return
	}
	if d.cfg.HoldWhenIdle && d.idle {
		// FR29: pause while idle without spending the escalation budget;
		// reschedule so the cadence picks up the moment the user is back.
		d.escTimer = time.AfterFunc(interval, func() { d.escalate(id, interval) })
		d.mu.Unlock()
		return
	}
	d.escCount++
	count := d.escCount
	it := *d.current
	if count < d.cfg.EscalationCount {
		d.escTimer = time.AfterFunc(interval, func() { d.escalate(id, interval) })
	}
	d.mu.Unlock()
	d.log.Info("item.escalated", "component", "daemon", "item_id", id, "count", count)
	d.snd.Play(sound.ClassFor(&it))
	// Escalation says it again, which is the point of escalation.
	d.snd.Speak(it.Speak)
}

func (d *Daemon) cancelEscalationLocked() {
	if d.escTimer != nil {
		d.escTimer.Stop()
		d.escTimer = nil
	}
	d.escCount = 0
}

// IsDnd reports the current do-not-disturb state.
func (d *Daemon) IsDnd() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dnd
}

// DndSet flips do-not-disturb (FR11). Turning it off reveals whatever
// queued silently.
func (d *Daemon) DndSet(on bool) {
	d.mu.Lock()
	if d.dnd == on {
		d.mu.Unlock()
		return
	}
	d.dnd = on
	var revealed *proto.Item
	if !on && d.current == nil {
		d.advanceLocked()
		revealed = d.current
	}
	view := d.viewLocked()
	d.mu.Unlock()
	d.log.Info("dnd.changed", "component", "daemon", "enabled", on)
	d.ui.Present(view)
	if revealed != nil {
		// Revealed for the first time now, so it announces itself now.
		d.snd.Play(sound.ClassFor(revealed))
		d.snd.Speak(revealed.Speak)
	}
	if d.OnDndChange != nil {
		d.OnDndChange(on)
	}
}

// Mute silences an agent in memory (FR47): its items queue straight to the
// inbox, no card and no sound, until unmuted or the daemon restarts. This is
// the instant reaction the config `mute` (FR17, ~2 s reload) is too slow for.
func (d *Daemon) Mute(agent string) {
	if agent == "" {
		return
	}
	d.mu.Lock()
	if d.muted[agent] {
		d.mu.Unlock()
		return
	}
	d.muted[agent] = true
	d.mu.Unlock()
	d.log.Info("agent.muted", "component", "daemon", "agent", agent)
	d.notifyMuteChange()
}

// Unmute lifts a runtime mute (FR47). Whatever queued silently from that
// agent surfaces now if the screen is free, with its earcon, like the reveal
// when DND turns off.
func (d *Daemon) Unmute(agent string) {
	d.mu.Lock()
	if !d.muted[agent] {
		d.mu.Unlock()
		return
	}
	delete(d.muted, agent)
	var revealed *proto.Item
	if d.current == nil {
		d.advanceLocked()
		revealed = d.current
	}
	view := d.viewLocked()
	d.mu.Unlock()
	d.log.Info("agent.unmuted", "component", "daemon", "agent", agent)
	d.ui.Present(view)
	if revealed != nil {
		// Revealed for the first time now, so it announces itself now.
		d.snd.Play(sound.ClassFor(revealed))
		d.snd.Speak(revealed.Speak)
	}
	d.notifyMuteChange()
}

// MutedAgents lists the runtime-muted agents, sorted, for the CLI and the
// inbox/tray badges (FR47).
func (d *Daemon) MutedAgents() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.muted))
	for a := range d.muted {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

func (d *Daemon) notifyMuteChange() {
	if d.OnMuteChange != nil {
		d.OnMuteChange(d.MutedAgents())
	}
}

func (d *Daemon) stopTimerLocked(id string) {
	if t, ok := d.timers[id]; ok {
		t.Stop()
		delete(d.timers, id)
	}
}

func (d *Daemon) viewLocked() View {
	v := View{Item: d.current, Waiting: len(d.queue), DismissAt: d.dismissAt, ActionsEnabled: !d.cfg.ActionsDisabled}
	if d.current != nil {
		v.ExpiresAt = d.expiries[d.current.ID]
	}
	for _, q := range d.queue {
		v.WaitingFrom = append(v.WaitingFrom, q.Identity)
	}
	v.Caller = d.callerStateLocked(d.current)
	return v
}

// callerStateLocked derives the caller-alive indicator (FR45) from existing
// state: a blocking item is live while its waiter channel is registered,
// gone once its socket drops, and awaiting-reconnect when it was restored
// from the store after a restart (no waiter, never marked gone).
func (d *Daemon) callerStateLocked(it *proto.Item) CallerState {
	if it == nil || !it.Blocking() {
		return CallerNone
	}
	switch {
	case d.gone[it.ID]:
		return CallerGone
	case d.waiters[it.ID] != nil:
		return CallerLive
	default:
		return CallerAwaiting
	}
}

// toastExpired auto-dismisses a self-closing info/success toast. If the user
// was idle when it lapsed (FR44), it is flagged "missed while away" so the
// return-from-idle inbox review separates toasts that flashed unseen from
// ones actually read. The idle check is read off the lock (it round-trips to
// the display server); the flag is a history marker, not a fresh interruption.
func (d *Daemon) toastExpired(id string) {
	d.mu.Lock()
	prs := d.prs
	idleAfter := d.cfg.IdleAfter
	d.mu.Unlock()
	missed := idleAfter > 0 && prs != nil && prs.IdleFor(idleAfter)
	if missed {
		d.log.Info("item.missed_while_away", "component", "daemon", "item_id", id)
	}
	d.resolve(id, store.StateDismissed, store.Outcome{MissedAway: missed})
}

// callerGone marks a blocking item whose caller socket dropped (FR45). The
// card turns to "caller disconnected" and auto-dismisses after a short grace
// so no thought is wasted answering a dead question; until then the user can
// still dismiss it with any key. Only an item with a live waiter qualifies -
// a restored item (awaiting reconnect) never had a connection to lose.
func (d *Daemon) callerGone(id string) {
	d.mu.Lock()
	if d.waiters[id] == nil || d.gone[id] {
		d.mu.Unlock()
		return
	}
	d.gone[id] = true
	if d.current != nil && d.current.ID == id {
		d.cancelEscalationLocked() // a gone card must not keep chiming
	}
	d.stopTimerLocked(id)
	window := d.cfg.CallerGone
	d.timers[id] = time.AfterFunc(window, func() { d.resolve(id, store.StateCancelled, store.Outcome{}) })
	view := d.viewLocked()
	d.mu.Unlock()
	d.log.Info("item.caller_gone", "component", "daemon", "item_id", id, "dismiss_ms", window.Milliseconds())
	d.ui.Present(view)
}

// BeginShutdown records that the daemon is tearing down, so a per-connection
// context cancel is read as shutdown rather than a caller drop (FR45). Call
// it before cancelling the server context.
func (d *Daemon) BeginShutdown() { d.closing.Store(true) }

// resolve finishes an item exactly once: the store transition is the
// arbiter, delivery and display advance follow. Reports whether this call
// won the resolution. While an item sits in its undo grace, only the grace
// finalizer may resolve it; expiry and cancel bounce off (FR28).
func (d *Daemon) resolve(id, toState string, out store.Outcome) bool {
	d.mu.Lock()
	if d.graced != nil && d.graced.id == id && toState != store.StateAnswered {
		d.mu.Unlock()
		return false
	}
	d.mu.Unlock()

	if err := d.st.Resolve(id, toState, out); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			d.log.Error("store.resolve_failed", "component", "daemon", "item_id", id, "err", err.Error())
		}
		return false
	}
	event := map[string]string{
		store.StateAnswered:  logging.EvItemAnswered,
		store.StateExpired:   logging.EvItemExpired,
		store.StateCancelled: logging.EvItemCancelled,
		store.StateDismissed: logging.EvItemDismissed,
	}[toState]
	d.log.Info(event, "component", "daemon", "item_id", id,
		"answer", out.Answer, "reply", out.Reply, "form", len(out.Values) > 0)

	d.mu.Lock()
	d.stopTimerLocked(id)
	delete(d.gone, id)
	if w, ok := d.waiters[id]; ok {
		delete(d.waiters, id)
		w <- proto.Result{
			ID: id, Answered: toState == store.StateAnswered,
			Answer: out.Answer, Reply: out.Reply, Values: out.Values,
			Vetoed: out.Vetoed, Secret: out.Secret, Approved: out.Approved,
		}
	}
	if d.current != nil && d.current.ID == id {
		d.advanceLocked()
	} else {
		for i, q := range d.queue {
			if q.ID == id {
				d.queue = append(d.queue[:i], d.queue[i+1:]...)
				break
			}
		}
	}
	view := d.viewLocked()
	d.mu.Unlock()
	d.ui.Present(view)
	return true
}

// Answer and the methods below are the UI's side of the contract. User
// answers pass through the undo grace; dismissals do not.

func (d *Daemon) Answer(id, label string) {
	d.maybeGrace(id, store.Outcome{Answer: label}, "Answered: "+label)
}

func (d *Daemon) Reply(id, text string) {
	d.maybeGrace(id, store.Outcome{Reply: text}, "Reply sent")
}

func (d *Daemon) AnswerForm(id string, values map[string]string) {
	d.maybeGrace(id, store.Outcome{Values: values}, "Form submitted")
}

// Review delivers a diff-review verdict (FR33): approve or request changes,
// with an optional comment. Like any answer it passes through the undo
// grace; the comment rides in Reply, the verdict in Answer and Approved.
func (d *Daemon) Review(id string, approved bool, comment string) {
	word, text := "rejected", "Changes requested"
	if approved {
		word, text = "approved", "Approved"
	}
	d.maybeGrace(id, store.Outcome{Approved: approved, Answer: word, Reply: comment}, text)
}

func (d *Daemon) Dismiss(id string) { d.resolve(id, store.StateDismissed, store.Outcome{}) }

// DismissItems retires pending items from a door that is not the mouse (FR89).
//
// It exists because agentbox had four ways to CREATE an item and none to retire one:
// a warning stays pending until it is clicked, pending items survive a daemon
// restart by design, and so a toast an agent posted and immediately knew to be
// noise - an acceptance probe's refused deadlock, say - sat on the human's screen
// forever, coming back after every restart. Boris asked about the same four toasts
// twice before this was built.
//
// Who may retire what is the whole of the safety model, and it is not symmetrical:
//
//   - Naming an ID retires that one, and an AGENT may only name its own. Retiring
//     another agent's question would be answering for it.
//   - `all` is the human's alone. An agent that could empty his queue could hide a
//     question it did not want answered.
//   - An agent sweeping without an ID gets its own items and nothing else, which is
//     what "take back what I posted" means.
//
// A dismissal is an ordinary resolution, so the caller of a blocking item learns
// its item ended the same way it learns about a cancel, and history records it.
func (d *Daemon) DismissItems(p proto.DismissParams) (proto.DismissResult, *proto.RPCError) {
	if p.ID == "" && !p.All && !p.Mine {
		return proto.DismissResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: `dismiss wants an item id, or --all to clear every pending item. "agentbox list" shows what is pending.`}
	}
	if p.All && !p.Human {
		return proto.DismissResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
			Message: "clearing every pending item is the human's own call, not an agent's: an agent that could empty his queue could hide a question it did not want answered. Retract what you posted instead."}
	}

	key := strings.TrimSpace(p.Identity.Key)
	// Snapshot under the lock, resolve outside it: resolve takes d.mu itself, and it
	// also delivers to a parked caller.
	type candidate struct {
		id    string
		byMe  bool
		agent string
	}
	var found []candidate
	d.mu.Lock()
	consider := func(it *proto.Item) {
		if it == nil {
			return
		}
		mine := key != "" && it.Identity.Key == key
		found = append(found, candidate{id: it.ID, byMe: mine, agent: it.Identity.Agent})
	}
	consider(d.current)
	for _, it := range d.queue {
		consider(it)
	}
	d.mu.Unlock()

	out := proto.DismissResult{OK: true}
	for _, c := range found {
		switch {
		case p.ID != "":
			if c.id != p.ID {
				continue
			}
			if !p.Human && !c.byMe {
				return proto.DismissResult{}, &proto.RPCError{Code: proto.CodeInvalidParams,
					Message: "that item was posted by " + nameOr(c.agent, "another agent") +
						", so it is not yours to retract. Retract only what you posted; the human can dismiss anything."}
			}
		case p.All:
			// Everything, and only the human gets here.
		default:
			// A sweep by an agent: its own and nothing else.
			if !c.byMe {
				continue
			}
		}
		if d.resolve(c.id, store.StateDismissed, store.Outcome{}) {
			out.Dismissed++
			out.IDs = append(out.IDs, c.id)
		}
	}
	if p.ID != "" && out.Dismissed == 0 {
		return proto.DismissResult{}, &proto.RPCError{Code: proto.CodeItemNotFound,
			Message: "no pending item " + p.ID}
	}
	if out.Dismissed > 0 {
		d.log.Info("item.dismissed", "component", "daemon", "count", out.Dismissed,
			"by", nameOr(p.Identity.Agent, "human"), "all", p.All)
	}
	if out.Dismissed == 0 {
		out.Note = "nothing pending to dismiss."
	}
	return out, nil
}

// Veto stops an act-unless-stopped item (FR22); the caller learns it was
// vetoed. No undo grace: the countdown already gave the deliberation
// window, and a stop must take effect at once.
func (d *Daemon) Veto(id string) { d.resolve(id, store.StateAnswered, store.Outcome{Vetoed: true}) }

// Secret delivers a masked value (FR23). No undo grace: the value must not
// linger on screen, and the answered strip has nothing safe to show. The
// value rides the in-memory Outcome only; finishSecret routes it to disk or
// stdout, and it is never logged or stored.
func (d *Daemon) Secret(id, value string) {
	d.resolve(id, store.StateAnswered, store.Outcome{Secret: value})
}

// finishSecret routes a delivered secret value (FR23): to its 0600 file when
// a sink is set, and onto the socket only when the caller opted into stdout.
// The value never crosses the socket otherwise, and never enters the log.
func (d *Daemon) finishSecret(it *proto.Item, res proto.Result) (any, *proto.RPCError) {
	val := res.Secret
	res.Secret = ""
	if !res.Answered {
		return res, nil
	}
	if it.Sink != "" {
		if err := writeSecretFile(it.Sink, val); err != nil {
			d.log.Error("secret.write_failed", "component", "daemon", "item_id", it.ID, "err", err.Error())
			return nil, &proto.RPCError{Code: proto.CodeInternal, Message: "write secret file: " + err.Error()}
		}
		res.SecretPath = it.Sink
		d.log.Info("secret.written", "component", "daemon", "item_id", it.ID, "path", it.Sink)
	}
	if it.Stdout {
		res.Secret = val
	}
	return res, nil
}

// writeSecretFile writes value to path at mode 0600, tightening permissions
// even if the file already existed.
func writeSecretFile(path, value string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.WriteString(value); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// maybeGrace holds a user answer for the undo window before delivery
// (FR28). Only the displayed card can be graced; anything else delivers
// directly. A second answer while graced is dropped: the strip is showing
// and undo is the only live control.
func (d *Daemon) maybeGrace(id string, out store.Outcome, text string) {
	d.mu.Lock()
	if d.graced != nil {
		d.mu.Unlock()
		return
	}
	if d.cfg.UndoGrace <= 0 || d.current == nil || d.current.ID != id {
		d.mu.Unlock()
		d.resolve(id, store.StateAnswered, out)
		return
	}
	grace := d.cfg.UndoGrace
	g := &graced{id: id, out: out, text: text, until: time.Now().Add(grace)}
	g.timer = time.AfterFunc(grace, func() { d.finalizeGrace(g) })
	d.graced = g
	view := d.viewLocked()
	view.Graced = true
	view.GracedText = text
	view.GraceUntil = g.until
	d.mu.Unlock()
	// The strip's whole lifecycle must be reconstructable from the log
	// (NFR13): started here, then item.answered at delivery or
	// item.undone.
	d.log.Info("item.grace_started", "component", "daemon", "item_id", id, "grace_ms", grace.Milliseconds())
	d.ui.Present(view)
}

func (d *Daemon) finalizeGrace(g *graced) {
	d.mu.Lock()
	if d.graced != g {
		d.mu.Unlock()
		return
	}
	d.graced = nil
	d.mu.Unlock()
	d.resolve(g.id, store.StateAnswered, g.out)
}

// Undo retracts a graced answer; the card returns untouched and the caller
// never learns it happened (FR28).
func (d *Daemon) Undo(id string) {
	d.mu.Lock()
	if d.graced == nil || d.graced.id != id {
		d.mu.Unlock()
		return
	}
	d.graced.timer.Stop()
	d.graced = nil
	view := d.viewLocked()
	d.mu.Unlock()
	d.log.Info("item.undone", "component", "daemon", "item_id", id)
	d.ui.Present(view)
}

// RecentItems feeds the inbox window (FR10).
func (d *Daemon) RecentItems(limit int) ([]store.StoredItem, error) {
	return d.st.Recent(limit)
}

// Stats backs the app's history tab (FR35); the store query is the same one
// the agentbox.v1.stats RPC serves.
func (d *Daemon) Stats(since time.Time) (proto.Stats, error) {
	return d.st.Stats(since)
}

// Promote pulls a queued item to the front of the display (inbox click).
// Promoting the current item just re-presents it.
func (d *Daemon) Promote(id string) {
	d.mu.Lock()
	if d.current != nil && d.current.ID == id {
		view := d.viewLocked()
		d.mu.Unlock()
		d.ui.Present(view)
		return
	}
	idx := -1
	for i, q := range d.queue {
		if q.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		d.mu.Unlock()
		return
	}
	it := d.queue[idx]
	d.queue = append(d.queue[:idx], d.queue[idx+1:]...)
	if d.current != nil {
		d.stopTimerLocked(d.current.ID)
		d.queue = append([]*proto.Item{d.current}, d.queue...)
	}
	d.setCurrentLocked(it)
	view := d.viewLocked()
	d.mu.Unlock()
	d.ui.Present(view)
}

// Defer sends the current card to the back of the queue; the caller keeps
// waiting (Esc, 03-ui-ux.md).
func (d *Daemon) Defer(id string) {
	d.mu.Lock()
	if d.current == nil || d.current.ID != id || len(d.queue) == 0 {
		d.mu.Unlock()
		return
	}
	it := d.current
	d.stopTimerLocked(id)
	d.queue = append(d.queue, it)
	d.advanceLocked()
	view := d.viewLocked()
	d.mu.Unlock()
	d.ui.Present(view)
}

// actionOutputCap bounds how much exec output a failure toast quotes; the
// full output is in the log regardless.
const actionOutputCap = 400

// RunAction runs the action button at index on the displayed notify item
// (FR32). Only the on-screen item's buttons are live, so a queued card cannot
// be triggered blind. The exec runs off the lock and off the UI goroutine so
// a slow command never freezes the card; the click is fire-and-forget.
func (d *Daemon) RunAction(id string, index int) {
	d.mu.Lock()
	if d.cfg.ActionsDisabled {
		d.mu.Unlock()
		d.log.Warn("action.rejected", "component", "daemon", "item_id", id, "reason", "actions disabled")
		return
	}
	it := d.current
	if it == nil || it.ID != id || index < 0 || index >= len(it.Actions) {
		d.mu.Unlock()
		return
	}
	act := it.Actions[index]
	cwd := it.Cwd
	agent := it.Identity.Agent
	d.mu.Unlock()
	go d.execAction(id, agent, cwd, act)
}

// execAction runs one action command via sh -c, in the item's cwd, as the
// current user (no privilege boundary, 02-architecture.md). Output lands in
// the log; a non-zero exit or spawn failure raises an error card.
func (d *Daemon) execAction(itemID, agent, cwd string, act proto.Action) {
	d.log.Info("action.started", "component", "daemon", "item_id", itemID,
		"label", act.Label, "command", act.Exec, "cwd", cwd)
	cmd := exec.Command("sh", "-c", act.Exec)
	cmd.Dir = cwd // empty means the daemon's own working directory
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		d.log.Error("action.failed", "component", "daemon", "item_id", itemID,
			"label", act.Label, "command", act.Exec, "err", err.Error(), "output", trimmed)
		body := fmt.Sprintf("`%s` exited with an error.", act.Exec)
		if trimmed != "" {
			if len(trimmed) > actionOutputCap {
				trimmed = trimmed[:actionOutputCap] + "…"
			}
			body += "\n\n" + trimmed
		}
		d.surfaceError(agent, "Action failed: "+act.Label, body)
		return
	}
	d.log.Info("action.finished", "component", "daemon", "item_id", itemID,
		"label", act.Label, "output", trimmed)
}

// surfaceError injects a daemon-originated error card (FR32 action failure).
// It is attributed to the original agent so the identity pill stays
// consistent, persisted like any notify item, and routed through the normal
// queue so it respects whatever is currently on screen.
func (d *Daemon) surfaceError(agent, title, body string) {
	d.surfaceNotify(proto.LevelError, proto.Identity{Agent: agent}, title, body)
}

// surfaceNotify enqueues a daemon-originated notify toast (an action error, a
// progress completion) through the normal display path so it chimes, shows and
// lands in history.
func (d *Daemon) surfaceNotify(level proto.Level, id proto.Identity, title, body string, actions ...proto.Action) {
	it := &proto.Item{
		ID: newID(), Kind: proto.KindNotify, Level: level,
		Title: title, Body: body, Identity: id, Actions: actions,
	}
	if err := d.st.CreateItem(it); err != nil {
		d.log.Error("store.create_failed", "component", "daemon", "item_id", it.ID, "err", err.Error())
	}
	d.mu.Lock()
	d.enqueueLocked(it)
	visible := d.current != nil && d.current.ID == it.ID
	view := d.viewLocked()
	d.mu.Unlock()
	d.ui.Present(view)
	if visible {
		d.snd.Play(sound.ClassFor(it))
		d.snd.Speak(it.Speak)
	}
}

// staleProgressAfter reaps a progress report whose caller went silent without
// sending Done (e.g. an MCP agent that crashed); generous so a genuinely slow
// task is not cut off. The CLI path is reaped promptly on disconnect instead
// (Hold + reapProgressConn).
const staleProgressAfter = 15 * time.Minute

// Progress creates, updates or finalizes a live progress report (FR21). It is
// non-blocking and never enters the card queue. ctx is the calling
// connection's context: when Hold is set (the CLI pipe), the report is reaped
// if that connection drops before Done. Returns the report ID.
func (d *Daemon) Progress(ctx context.Context, u proto.ProgressUpdate) (proto.ProgressResult, *proto.RPCError) {
	d.mu.Lock()
	id := u.ID
	created := false
	if id == "" {
		id = newID()
		d.progress[id] = &progressEntry{st: ProgressState{ID: id, Identity: u.Identity}, held: u.Hold}
		d.progOrder = append(d.progOrder, id)
		created = true
	}
	e := d.progress[id]
	if e == nil {
		d.mu.Unlock()
		d.log.Warn("progress.unknown", "component", "daemon", "progress_id", id)
		return proto.ProgressResult{ID: id}, &proto.RPCError{Code: proto.CodeItemNotFound, Message: "no such progress report: " + id}
	}
	e.updated = time.Now()
	if u.Title != "" {
		e.st.Title = u.Title
	}
	if u.Status != "" {
		e.st.Status = u.Status
	}
	e.st.Indeterminate = u.Indeterminate
	if !u.Indeterminate {
		p := u.Percent
		if p < 0 {
			p = 0
		} else if p > 100 {
			p = 100
		}
		e.st.Percent = p
	}
	if u.Identity.Agent != "" {
		e.st.Identity = u.Identity
	}
	finished := e.st
	if u.Done {
		d.removeProgressLocked(id)
	}
	reports := d.progressListLocked()
	d.mu.Unlock()

	if created {
		d.log.Info("progress.started", "component", "daemon", "progress_id", id,
			"agent", u.Identity.Agent, "title", u.Title, "held", u.Hold)
		if u.Hold && ctx != nil {
			go d.reapProgressConn(ctx, id)
		}
	}
	d.ui.ShowProgress(reports)
	if u.Done {
		d.log.Info("progress.done", "component", "daemon", "progress_id", id,
			"percent", finished.Percent, "error", u.Error)
		d.completeProgress(finished, u.Error)
	}
	return proto.ProgressResult{ID: id}, nil
}

// removeProgressLocked drops a report from the map and the order slice.
func (d *Daemon) removeProgressLocked(id string) {
	delete(d.progress, id)
	for i, x := range d.progOrder {
		if x == id {
			d.progOrder = append(d.progOrder[:i], d.progOrder[i+1:]...)
			break
		}
	}
}

func (d *Daemon) progressListLocked() []ProgressState {
	out := make([]ProgressState, 0, len(d.progOrder))
	for _, id := range d.progOrder {
		if e := d.progress[id]; e != nil {
			out = append(out, e.st)
		}
	}
	return out
}

// completeProgress turns a finished report into one completion toast: success,
// or error when errMsg is set (FR21). Empty agent defaults to "agentbox" so the
// toast still has an identity in history.
func (d *Daemon) completeProgress(st ProgressState, errMsg string) {
	if st.Identity.Agent == "" {
		st.Identity.Agent = "agentbox"
	}
	title := st.Title
	if title == "" {
		title = "Task"
	}
	if errMsg != "" {
		d.surfaceNotify(proto.LevelError, st.Identity, title+" failed", errMsg)
		return
	}
	d.surfaceNotify(proto.LevelSuccess, st.Identity, title+" complete", st.Status)
}

// reapProgressConn drops a held report when its creating connection drops
// before Done (FR21): the CLI pipe died mid-task, so the bar must not linger.
// A clean Done removes the report first, so this then finds nothing and a
// shutdown is told apart by the closing flag (no spurious "interrupted" toast).
func (d *Daemon) reapProgressConn(ctx context.Context, id string) {
	<-ctx.Done()
	if d.closing.Load() {
		return
	}
	d.mu.Lock()
	e := d.progress[id]
	if e == nil {
		d.mu.Unlock()
		return // already finished cleanly
	}
	st := e.st
	d.removeProgressLocked(id)
	reports := d.progressListLocked()
	d.mu.Unlock()
	d.log.Warn("progress.caller_gone", "component", "daemon", "progress_id", id, "title", st.Title)
	d.ui.ShowProgress(reports)
	title := st.Title
	if title == "" {
		title = "Task"
	}
	if st.Identity.Agent == "" {
		st.Identity.Agent = "agentbox"
	}
	d.surfaceNotify(proto.LevelWarning, st.Identity, title+" interrupted", "the reporting process exited before finishing")
}

// ReapStaleProgress drops reports untouched for staleProgressAfter - the
// backstop for a caller (e.g. an MCP agent) that died without Done and had no
// held connection to detect. Called from the daemon's poll ticker.
func (d *Daemon) ReapStaleProgress() {
	cutoff := time.Now().Add(-staleProgressAfter)
	d.mu.Lock()
	var stale []string
	for id, e := range d.progress {
		if !e.held && e.updated.Before(cutoff) {
			stale = append(stale, id)
		}
	}
	if len(stale) == 0 {
		d.mu.Unlock()
		return
	}
	for _, id := range stale {
		d.removeProgressLocked(id)
	}
	reports := d.progressListLocked()
	d.mu.Unlock()
	for _, id := range stale {
		d.log.Warn("progress.stale_reaped", "component", "daemon", "progress_id", id)
	}
	d.ui.ShowProgress(reports)
}

// RecentBySession backs the Agents board's opened row: the items ONE session
// raised, matched on the session key rather than on the agent/project/session
// triple every Claude session in a repo shares.
func (d *Daemon) RecentBySession(key string, limit int) ([]store.StoredItem, error) {
	return d.st.RecentBySession(key, limit)
}
