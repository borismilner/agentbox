package webui

import (
	"strings"
	"sync"

	"github.com/borismilner/agentbox/internal/proto"
)

// The Agents surface (FR83): every live agent session, what it is for, what it
// is doing right now, what it holds and what it waits on.
//
// MOCK, gate 2 of FR83. There is no roster in the daemon yet. This file exists
// so the surface can be looked at and clicked over canned data before any
// daemon code is written, which is the working rule at the top of
// docs/07-field-requests.md. What survives into slice 1 is the wire shape and
// the surface; what gets replaced is where the rows come from.
//
// Two shape decisions worth keeping, because they are the ones the daemon will
// have to honour:
//
//   - Ages travel as an age, not as a timestamp. Every `since_ms` is how old
//     the thing was when the payload left the daemon, and the surface anchors
//     it to the moment the payload arrived - the arrangement the control strip
//     already proved (control.go, Control.svelte). A wall-clock jump then
//     rebases everything on the next push instead of reading every agent as
//     quiet after a suspend.
//   - The state is one string the daemon decides, never a set of booleans the
//     surface has to prioritise. The chip vocabulary is the design's table, in
//     the design's priority order, and self-report never sets it: an agent can
//     write its purpose and its activity, and nothing else on the row.
const (
	agentAsking      = "asking"      // a blocking item of theirs is pending
	agentDriving     = "driving"     // they hold the control run
	agentBlocked     = "blocked"     // parked in acquire_lock
	agentListening   = "listening"   // parked in await_signal
	agentReporting   = "reporting"   // they own a live progress report
	agentWorking     = "working"     // activity line fresher than the threshold
	agentQuiet       = "quiet"       // present, nothing reported lately
	agentUnannounced = "unannounced" // present, never announced: the dim row
	agentDetached    = "detached"    // pre-sync child, derived from item traffic
)

// wireAgent is one roster row.
type wireAgent struct {
	// Key is the session key, which is the only identity sync trusts. It is on
	// the wire because a wait names its holder by key and the surface has to be
	// able to find that row.
	Key     string `json:"key"`
	Agent   string `json:"agent"`
	Project string `json:"project"`
	Session string `json:"session"` // the name, when agentbox spawned the session
	Hue     string `json:"hue"`
	Cwd     string `json:"cwd"`
	PID     int    `json:"pid"`

	Area      string   `json:"area"`       // the group key: derived, never declared
	AreaLabel string   `json:"area_label"` // what to show as the heading
	Tags      []string `json:"tags"`       // declared kind:scope refinements

	Purpose  string `json:"purpose"`  // the announce line, empty if never announced
	Activity string `json:"activity"` // the self-reported current line

	State  string `json:"state"`
	Detail string `json:"detail"` // the chip's second half: a topic, a percentage

	ActivitySinceMS int64 `json:"activity_since_ms"`
	AgeMS           int64 `json:"age_ms"` // since the attach

	Holds []wireHold `json:"holds"`
	Wait  *wireWait  `json:"wait"`

	// The detail view. A row is a glance; this is what opening one is for.
	Timeline []wireTick    `json:"timeline"`
	Signals  []wireSignal  `json:"signals"`
	Items    []wireItemRef `json:"items"`
	Pending  string        `json:"pending"` // the question they are parked on, if any
}

// wireHold is a held lock. It doubles as an orphan: a hold whose session is
// gone keeps the row so the resource is visibly still taken, which is the whole
// point of orphaning rather than releasing.
type wireHold struct {
	Name    string `json:"name"`
	SinceMS int64  `json:"since_ms"`
	Note    string `json:"note"`
	Waiters int    `json:"waiters"`

	Orphaned bool   `json:"orphaned"`
	PID      int    `json:"pid"`      // the pid recorded at acquire time
	PIDLive  bool   `json:"pid_live"` // and whether it is still there
	Holder   string `json:"holder"`   // last-known holder, for an orphan with no row
}

// wireWait is what a blocked agent is blocked on. It names the holder rather
// than the lock alone: "blocked" with nobody named is the state the human
// already has today.
type wireWait struct {
	Lock      string `json:"lock"`
	HolderKey string `json:"holder_key"`
	Holder    string `json:"holder"`
	SinceMS   int64  `json:"since_ms"`
	Place     int    `json:"place"` // position in the FIFO queue, 1-based
	Queue     int    `json:"queue"` // how many are waiting in total
}

type wireTick struct {
	Line    string `json:"line"`
	SinceMS int64  `json:"since_ms"`
}

type wireSignal struct {
	Topic   string `json:"topic"`
	Dir     string `json:"dir"` // posted or received
	SinceMS int64  `json:"since_ms"`
	Data    string `json:"data"`
}

type wireItemRef struct {
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	State   string `json:"state"`
	SinceMS int64  `json:"since_ms"`
}

// wireRoster is the whole surface in one payload.
type wireRoster struct {
	Agents  []wireAgent `json:"agents"`
	Orphans []wireHold  `json:"orphans"` // orphaned holds whose row is already gone

	// Partial says at least one session is known only from item traffic, so the
	// roster cannot claim to be everybody. "You are alone" must be true when
	// said or not said at all, which is FR61's rule applied to presence.
	Partial bool `json:"partial"`
}

type agents struct {
	ui *UI

	mu   sync.Mutex
	seen wireRoster
}

func newAgents(u *UI) *agents { return &agents{ui: u} }

func (a *agents) snapshot() wireRoster {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.seen
}

func (a *agents) set(r wireRoster) {
	a.mu.Lock()
	a.seen = r
	a.mu.Unlock()
	a.ui.emit("agentbox:agents", r)
}

// ShowAgents pushes a roster at the surface. The daemon's sync subsystem calls
// this on every roster change, coalesced on its side; the surface renders
// whatever arrives.
func (u *UI) ShowAgents(r wireRoster) { u.agents.set(r) }

// ShowRoster is the daemon's own door: it takes the roster in the daemon's
// vocabulary and renders it in the surface's. It satisfies the push signature
// the sync subsystem expects, so wiring is one line at construction.
//
// The conversion lives here rather than in the daemon because the wire types are
// this package's and unexported - the same reason the demo fixture lives here.
// The hue is computed with the daemon's function, not the frontend's, which is
// deliberate: the two disagree (FR85), and this surface joins the majority
// rather than widening the split.
func (u *UI) ShowRoster(agents []proto.SyncAgent, partial bool) {
	dark := u.themeMode() == "dark"
	rows := make([]wireAgent, 0, len(agents))
	for _, a := range agents {
		rows = append(rows, wireAgent{
			Key: a.Key, Agent: a.Agent, Project: a.Project, Session: a.Session,
			Hue: IdentityHue(a.Agent, a.Project, dark),
			Cwd: a.Cwd, PID: a.PID,
			Area: a.Area, AreaLabel: areaLabel(a.Area), Tags: a.Tags,
			Purpose: a.Purpose, Activity: a.Activity,
			State: a.State, Detail: a.Detail,
			ActivitySinceMS: a.ActivitySinceMS, AgeMS: a.AgeMS,
		})
	}
	u.agents.set(wireRoster{Agents: rows, Partial: partial})
}

// areaLabel is the heading a group of agents gets. An area key is machine-shaped
// (`repo:agentbox`); the heading is what the human already calls the thing.
func areaLabel(area string) string {
	if _, name, ok := strings.Cut(area, ":"); ok && name != "" {
		return name
	}
	return area
}

// Roster is the daemon's sync subsystem as this surface needs it: the one thing
// the human can do to an agent from here. Its own keyhole, like Handover and
// Voice, so the surface cannot reach anything else on the way.
//
// Nil is valid and means the mock: the break then edits the canned roster in
// place so the confirm and the row's disappearance can be judged, and says so
// in the log.
type Roster interface {
	BreakLock(name string) error
}

// SetRoster wires it.
func (u *UI) SetRoster(r Roster) {
	u.mu.Lock()
	u.roster = r
	u.mu.Unlock()
}

func (u *UI) rosterSrc() Roster {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.roster
}

// Agents is the surface asking for the roster to paint on mount, so a window
// opened between two pushes does not start blank.
func (b *Bridge) Agents() wireRoster { return b.ui.agents.snapshot() }

// BreakLock is the human taking a lock away from whoever holds it. It answers
// with "" or a sentence to show, the shape the assignment editor uses.
//
// The copy on the button says what this does and does not do, and it is not
// decoration: breaking reassigns the lock, it does not stop the ex-holder. A
// human who thinks otherwise has been handed the exact failure the orphan rule
// exists to prevent.
func (b *Bridge) BreakLock(name string) string {
	if r := b.ui.rosterSrc(); r != nil {
		if err := r.BreakLock(name); err != nil {
			return err.Error()
		}
		return ""
	}
	b.ui.log.Info("webui.agents_break_mock", "component", "webui", "lock", name)
	b.ui.agents.mockBreak(name)
	return ""
}

// mockBreak is the gate-2 affordance and nothing more: with no daemon behind
// the surface, breaking a lock has to change something or the confirm cannot be
// judged. Slice 1 deletes this along with the canned roster.
func (a *agents) mockBreak(name string) {
	a.mu.Lock()
	r := a.seen
	kept := make([]wireHold, 0, len(r.Orphans))
	for _, h := range r.Orphans {
		if h.Name != name {
			kept = append(kept, h)
		}
	}
	r.Orphans = kept

	rows := make([]wireAgent, 0, len(r.Agents))
	for _, ag := range r.Agents {
		held := make([]wireHold, 0, len(ag.Holds))
		for _, h := range ag.Holds {
			if h.Name != name {
				held = append(held, h)
			}
		}
		ag.Holds = held
		// The waiter wins the moment the hold goes, and its row has to say so
		// rather than sitting on a wait for a lock that no longer has a holder.
		if ag.Wait != nil && ag.Wait.Lock == name {
			ag.Wait = nil
			ag.State, ag.Detail = agentWorking, ""
			ag.Activity, ag.ActivitySinceMS = "granted "+name+" after the break", 0
			ag.Holds = append(ag.Holds, wireHold{Name: name, Note: "granted by a break"})
		}
		rows = append(rows, ag)
	}
	r.Agents = rows
	a.seen = r
	a.mu.Unlock()

	a.ui.emit("agentbox:agents", r)
}
