package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
)

// The roster (FR83, slice 1). Boris: "Every agent using the platform must
// provide a short description of the purpose of the agent and the current thing
// the agent is doing and update these as they change so that I can monitor all
// works in the most convenient way possible."
//
// The daemon is the one meeting point every agent already has, so presence
// lives here. The shape rests on one mechanic: an attach is a single call that
// blocks for the session's whole life, and its context dying IS the session
// going away. That is FR45's per-call insight promoted to per-session, and it
// only works because Conn.Serve cancels a blocked handler when its peer hangs
// up - which it did not do until the fix that shipped alongside this.
//
// Three rules that are decisions, not implementation detail:
//
//   - Self-report colours a row; it never defines it. Purpose and activity are
//     the agent's words. The state is what the daemon observed, in a fixed
//     priority order, and no agent can set it.
//   - A row exists whether or not the model ever says anything. The child
//     registers what it knows for free, so a rude agent is dim rather than
//     invisible - the human's view must not depend on good manners.
//   - Absence is never asserted on partial data. Sessions older than this
//     feature have no attach, so any read that could be read as "you are alone"
//     carries partial instead (FR61's rule, applied to presence).

// The state vocabulary. Priority order top to bottom: the first one that is
// true wins, because a row has one chip and "asking you" outranks "working"
// every time.
const (
	StateAsking      = "asking"
	StateDriving     = "driving"
	StateBlocked     = "blocked"
	StateListening   = "listening"
	StateReporting   = "reporting"
	StateWorking     = "working"
	StateQuiet       = "quiet"
	StateUnannounced = "unannounced"
	// StateDetached is a row nothing is holding open: an announce or an activity
	// line that arrived from a hook or a CLI call on behalf of a session whose
	// own child has not attached. Real, worth showing, and not evidence that
	// anybody is still there.
	StateDetached = "detached"
)

// provisionalFor is how long a row with no attach behind it survives. The
// SessionStart hook in recipes.md announces before the agent's child has made
// its first tool call, so a provisional row is the normal case for a few seconds
// and it must not become permanent: nothing would ever clear it, and the board
// would fill with sessions that ended days ago.
const provisionalFor = 10 * time.Minute

// workingFor is how long an activity line still counts as "working". Past it
// the row reads quiet, with its age shown, which is the visible form of a
// session that has stopped saying anything. Not a knob: it is a reading-speed
// constant, and the age is on screen either way.
const workingFor = 90 * time.Second

// rosterEmitEvery throttles pushes to the surface. The roster changes on every
// activity line, and a webview does not need more than a few repaints a second
// to look live.
const rosterEmitEvery = 250 * time.Millisecond

// rosterTickEvery is how often the roster re-examines itself with nobody having
// called anything. It has to exist: every push is caused by a verb, so without a
// tick the two ways the board goes stale are both invisible. A dropped throttled
// push waits for unrelated traffic (measured at over a minute in the field), and
// a state that decayed with no traffic at all never decays on screen at all -
// the row keeps saying "working" while the CLI says "quiet". One second, because
// the cost is a snapshot of a handful of rows and the payoff is that the board
// stops lying within a second of the truth changing.
const rosterTickEvery = time.Second

type rosterRow struct {
	identity proto.Identity
	cwd      string
	pid      int
	area     string
	// areaPath is where the area lives, when the row's own cwd is evidence of it.
	// Computed once here rather than per snapshot: it stats the filesystem, and a
	// row's cwd does not move.
	areaPath string
	tags     []string

	purpose    string
	activity   string
	activityAt time.Time
	attachedAt time.Time
	announced  bool

	// attached says a live connection is holding this row open. False for a row
	// created by a hook or a CLI call on behalf of a session whose own child has
	// not attached yet: real, but not evidence anybody is still there.
	attached bool
	touched  time.Time

	// knownPeers is this session's cursor on its own area: the peers it has
	// already been told about, keyed by session key and remembering the name each
	// one was reported under. The rider reports the difference and moves the
	// cursor, so each arrival and departure is mentioned exactly once. Nil means
	// nothing has been reported to this session yet, which is not the same as an
	// empty set - an empty set says it has been told it is alone.
	//
	// The name is remembered rather than looked up because a departure is reported
	// after the row is gone: without this the line could only name the session key,
	// which reads as noise to the agent it is warning.
	knownPeers map[string]string
}

type roster struct {
	log *slog.Logger

	mu   sync.Mutex
	rows map[string]*rosterRow // by session key

	// seenUnattached records sessions the daemon has met through ordinary item
	// traffic but which hold no attach - a child older than this feature. Their
	// existence is why a read can only ever be partial.
	seenUnattached map[string]time.Time

	// observers answer the questions a self-report cannot: who is parked on a
	// human answer, who holds the desktop, and who holds or waits on a lock. All
	// three are already tracked elsewhere, so the roster asks rather than
	// duplicating.
	askingFn  func() map[string]bool
	drivingFn func() string
	lockRows  func() (map[string][]proto.SyncHold, map[string]proto.SyncWait)

	// onGone is told when a session's attach drops, after the row is removed.
	onGone func(key string)

	push    func([]proto.SyncAgent, bool)
	lastGen time.Time
	dirty   bool

	// lastState is the state of every row as the surface last saw it, and
	// lastPartial the notice that went with it. The tick compares against these
	// rather than pushing every second: an idle board should stay silent, and a
	// board whose truth moved should not.
	lastState   map[string]string
	lastPartial bool

	// pushMu serialises snapshot, record and push so the recorded states always
	// describe the payload the surface actually received. Held across the emit,
	// which is one-way into the webview and cannot call back in here.
	pushMu sync.Mutex

	stop chan struct{}
}

func newRoster(log *slog.Logger) *roster {
	return &roster{
		log:            log,
		rows:           map[string]*rosterRow{},
		seenUnattached: map[string]time.Time{},
	}
}

// SetObservers wires the two facts the roster cannot see for itself. Both are
// legitimately nil in a test.
func (r *roster) SetObservers(asking func() map[string]bool, driving func() string) {
	r.mu.Lock()
	r.askingFn, r.drivingFn = asking, driving
	r.mu.Unlock()
}

// SetPush wires the surface. Called with the whole roster whenever it changes,
// throttled.
func (r *roster) SetPush(push func([]proto.SyncAgent, bool)) {
	r.mu.Lock()
	r.push = push
	r.mu.Unlock()
}

// SetLocks wires the lock table's two reads into a row: what this session holds,
// and what it is parked on (FR83 slice 2). Nil in a roster with no locks, which
// is every test that predates them.
func (r *roster) SetLocks(rows func() (map[string][]proto.SyncHold, map[string]proto.SyncWait)) {
	r.mu.Lock()
	r.lockRows = rows
	r.mu.Unlock()
}

// SetOnGone wires what happens when a session's attach drops. The roster's own
// answer is to remove the row; the lock table's is more careful, because a dead
// child does not prove the work it started is over.
func (r *roster) SetOnGone(gone func(key string)) {
	r.mu.Lock()
	r.onGone = gone
	r.mu.Unlock()
}

// announced answers the lock verbs' gate: has this session said what it is for.
func (r *roster) announced(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[key]
	return row != nil && row.announced
}

// agentOf is one row as a caller outside this file sees it, so a lock refusal can
// name the holder in the same terms the human's board does. The state is derived
// the same way, which is why it is worth going through here rather than reading
// the map directly.
func (r *roster) agentOf(key string) (proto.SyncAgent, bool) {
	asking, driving := r.observed()

	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[key]
	if row == nil {
		return proto.SyncAgent{}, false
	}
	now := time.Now()
	a := proto.SyncAgent{
		Key: key, Agent: row.identity.Agent, Project: row.identity.Project,
		Session: row.identity.Session, Cwd: row.cwd, PID: row.pid,
		Area: row.area, AreaPath: row.areaPath,
		Purpose: row.purpose, Activity: row.activity,
		AgeMS: now.Sub(row.attachedAt).Milliseconds(),
	}
	if !row.activityAt.IsZero() {
		a.ActivitySinceMS = now.Sub(row.activityAt).Milliseconds()
	}
	// Deliberately without holds and waits: this is what a LOCK result embeds, and
	// a holder's own hold on the lock being asked about would be saying the same
	// thing twice in one answer.
	a.State, a.Detail = derivedState(row, key, asking, driving, nil, now)
	return a, true
}

// observed reads the two observer funcs and calls them outside r.mu. They reach
// other subsystems' locks, and the roster must never hold its own while doing
// that: the lock table calls back in here.
func (r *roster) observed() (map[string]bool, string) {
	r.mu.Lock()
	askingFn, drivingFn := r.askingFn, r.drivingFn
	r.mu.Unlock()

	asking := map[string]bool{}
	if askingFn != nil {
		asking = askingFn()
	}
	driving := ""
	if drivingFn != nil {
		driving = drivingFn()
	}
	return asking, driving
}

// lockState is the holds and waits, read outside r.mu for the reason snapshot
// explains. Empty maps when there is no lock table, which keeps every row
// rendering correctly in a daemon built without one.
func (r *roster) lockState() (map[string][]proto.SyncHold, map[string]proto.SyncWait) {
	r.mu.Lock()
	rows := r.lockRows
	r.mu.Unlock()
	if rows == nil {
		return map[string][]proto.SyncHold{}, map[string]proto.SyncWait{}
	}
	return rows()
}

// betterIdentity keeps the most informative version of a session's identity
// across the several calls that describe it.
//
// One session is described by more than one caller: a SessionStart hook attaches
// on its behalf, and its own mcp child announces later. The hook's attach knows
// the least - under setsid its parent is init - so taking the newest wholesale
// stamped `systemd` over `claude` and left it there, which is what Boris's board
// showed. Newest wins on every field except a name, where a real one outranks a
// placeholder whichever order they arrive in.
func betterIdentity(have, incoming proto.Identity) proto.Identity {
	out := incoming
	if proto.PlaceholderAgent(out.Agent) && !proto.PlaceholderAgent(have.Agent) {
		out.Agent = have.Agent
	}
	if out.Project == "" {
		out.Project = have.Project
	}
	if out.Session == "" {
		out.Session = have.Session
	}
	return out
}

// Attach registers a session and blocks until its context ends, which is the
// session ending. The caller gets nothing back but the fact it was registered:
// the value of this call is entirely that it is still open.
func (r *roster) Attach(ctx context.Context, p proto.SyncAttachParams) (proto.SyncResult, *proto.RPCError) {
	key := strings.TrimSpace(p.Identity.Key)
	if key == "" {
		// Without a key there is nothing to key a row to, and agent-name
		// equality is the defect this feature exists to fix.
		return proto.SyncResult{}, &proto.RPCError{
			Code:    proto.CodeInvalidParams,
			Message: "attach needs identity.key: one session, one key. The mcp child mints it; a CLI caller passes --key to act on behalf of a session.",
		}
	}

	now := time.Now()
	r.mu.Lock()
	row := r.rows[key]
	if row == nil {
		row = &rosterRow{attachedAt: now}
		r.rows[key] = row
	}
	row.identity = betterIdentity(row.identity, p.Identity)
	row.attached, row.touched = true, now
	row.cwd, row.pid = p.Cwd, p.PID
	if p.Area != "" {
		row.area = p.Area
	}
	row.areaPath = areaPathOf(row.cwd, row.area)
	if len(p.Tags) > 0 {
		row.tags = p.Tags
	}
	// A redial after a daemon restart replays the announce, so an existing
	// purpose is kept rather than blanked by the reconnect.
	delete(r.seenUnattached, key)
	r.mu.Unlock()

	r.log.Info(logging.EvSync, "component", "daemon", "sync", "attach",
		"agent", p.Identity.Agent, "project", p.Identity.Project, "key", key, "pid", p.PID)
	r.changed()

	<-ctx.Done()

	r.mu.Lock()
	delete(r.rows, key)
	gone := r.onGone
	r.mu.Unlock()
	r.log.Info(logging.EvSync, "component", "daemon", "sync", "detach", "key", key)
	// Outside r.mu: this reaches the lock table, which reaches back here.
	if gone != nil {
		gone(key)
	}
	r.changed()

	return proto.SyncResult{OK: true}, nil
}

// Announce states the session's purpose and answers with its peers, so an agent
// learns it is not alone in its very first sync call - before it has touched
// anything.
func (r *roster) Announce(p proto.SyncAnnounceParams) (proto.SyncResult, *proto.RPCError) {
	key := strings.TrimSpace(p.Identity.Key)
	if key == "" {
		return proto.SyncResult{}, &proto.RPCError{
			Code:    proto.CodeInvalidParams,
			Message: "announce needs identity.key: one session, one key.",
		}
	}
	if strings.TrimSpace(p.Purpose) == "" {
		return proto.SyncResult{}, &proto.RPCError{
			Code:    proto.CodeInvalidParams,
			Message: "announce needs a purpose: one line saying what this session is for, in the human's terms.",
		}
	}

	now := time.Now()
	r.mu.Lock()
	row := r.rows[key]
	if row == nil {
		// Announcing without an attach is legal and useful: a CLI caller or a
		// hook can put a purpose on the board on a session's behalf. The row
		// exists from now on; it just has no connection proving liveness.
		row = &rosterRow{attachedAt: now}
		r.rows[key] = row
		row.identity = p.Identity
	} else if proto.PlaceholderAgent(row.identity.Agent) && !proto.PlaceholderAgent(p.Identity.Agent) {
		// The row exists because a hook announced or attached on this session's
		// behalf before its own child was up, so it is wearing whatever exec'd
		// the hook. This caller knows better. Only the name: the rest of an
		// existing row is not this call's business.
		row.identity.Agent = p.Identity.Agent
	}
	row.touched = now
	row.purpose = strings.TrimSpace(p.Purpose)
	row.announced = true
	if p.Activity != "" {
		row.activity, row.activityAt = p.Activity, now
	}
	if p.Area != "" {
		row.area = p.Area
		row.areaPath = areaPathOf(row.cwd, row.area)
	}
	if len(p.Tags) > 0 {
		row.tags = p.Tags
	}
	area := row.area
	r.mu.Unlock()

	r.log.Info(logging.EvSync, "component", "daemon", "sync", "announced",
		"agent", p.Identity.Agent, "key", key, "purpose", row.purpose, "area", area)
	r.changed()

	res := r.peersOf(key, area)
	res.OK = true
	return res, nil
}

// Activity writes the caller's current line. Non-blocking, coalesced by being a
// plain last-write-wins field: an eager agent costs nothing.
func (r *roster) Activity(p proto.SyncActivityParams) (proto.SyncResult, *proto.RPCError) {
	key := strings.TrimSpace(p.Identity.Key)
	line := strings.TrimSpace(p.Activity)
	if key == "" || line == "" {
		return proto.SyncResult{OK: true}, nil
	}
	now := time.Now()

	r.mu.Lock()
	row := r.rows[key]
	if row == nil {
		r.mu.Unlock()
		// Not an error: a session may report activity before it attaches, and
		// refusing would make the ticker less truthful, not more.
		return proto.SyncResult{OK: true,
			Note: "no roster row for this session yet; call announce so the human can see what you are for"}, nil
	}
	// An unchanged line is not progress and must not reset the age, or a stalled
	// agent looks busy forever. Same rule the control strip already applies.
	row.touched = now
	if line != row.activity {
		row.activity, row.activityAt = line, now
	}
	r.mu.Unlock()

	r.changed()
	return proto.SyncResult{OK: true}, nil
}

// riderFor is the discovery rider: what this session has not yet been told about
// its own area, in one line, and nothing when there is nothing new.
//
// It is the mechanism that makes discovery work for an agent that is mid-task.
// announce answers "who is here" at the start of a session and list_agents
// answers it on demand, but both need the agent to think of asking, and the
// collision this feature exists to prevent happens to an agent that is already
// deep in a file. So the news comes back attached to whatever it calls next.
//
// Each arrival and departure is reported exactly once: the row carries the set of
// peers it has been told about, and this moves that cursor. A session that has
// never been told anything gets the peers already present, because from its point
// of view they are all news.
func (r *roster) riderFor(key string) string {
	if key == "" {
		return ""
	}
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	row := r.rows[key]
	if row == nil || row.area == "" {
		return ""
	}
	current := map[string]string{}
	for k, other := range r.rows {
		if k != key && other.area == row.area {
			name := other.identity.Agent
			if name == "" {
				name = k
			}
			current[k] = name
		}
	}
	known := row.knownPeers
	row.knownPeers = current

	var joined, left []string
	for k := range current {
		if _, told := known[k]; !told {
			joined = append(joined, k)
		}
	}
	for k := range known {
		if _, still := current[k]; !still {
			left = append(left, k)
		}
	}
	if known == nil {
		// Nothing has been said to this session yet, so everybody present is news
		// and nobody has left. Silence when it really is alone: "you are alone" is
		// a claim, and the roster only makes it where it can be sure (see partial).
		left = nil
		if len(joined) == 0 {
			return ""
		}
	}
	if len(joined) == 0 && len(left) == 0 {
		return ""
	}
	// Stable wording for a stable event, so two identical situations read the same.
	sort.Strings(joined)
	sort.Strings(left)

	asking := map[string]bool{}
	if r.askingFn != nil {
		asking = r.askingFn()
	}
	driving := ""
	if r.drivingFn != nil {
		driving = r.drivingFn()
	}
	describe := func(k string) string {
		peer := r.rows[k]
		if peer == nil {
			return k
		}
		name := peer.identity.Agent
		if name == "" {
			name = k
		}
		what := "no purpose given"
		if peer.purpose != "" {
			what = `"` + peer.purpose + `"`
		}
		// Without the lock state: this runs under r.mu, and reaching the lock
		// table from here would invert the two subsystems' lock order (see
		// snapshot). A peer's lock is one call away on the board; what the rider
		// owes the agent is that the peer EXISTS.
		state, _ := derivedState(peer, k, asking, driving, nil, now)
		return fmt.Sprintf("%s %s (%s)", name, what, state)
	}

	var parts []string
	if len(joined) > 0 {
		who := make([]string, 0, len(joined))
		for _, k := range joined {
			who = append(who, describe(k))
		}
		verb := "is also in"
		if known != nil {
			verb = "joined"
		}
		parts = append(parts, fmt.Sprintf("%d %s %s %s: %s",
			len(joined), plural(len(joined), "agent"), verb, row.area, strings.Join(who, ", ")))
	}
	if len(left) > 0 {
		gone := make([]string, 0, len(left))
		for _, k := range left {
			// The row is already gone, so this is the name it was reported under.
			name := known[k]
			if name == "" {
				name = k
			}
			gone = append(gone, name)
		}
		parts = append(parts, fmt.Sprintf("%d %s left %s (%s)",
			len(left), plural(len(left), "agent"), row.area, strings.Join(gone, ", ")))
	}
	line := "sync: " + strings.Join(parts, "; ") + "."
	if len(joined) > 0 {
		line += " Coordinate before you edit a shared file: split the work or wait."
	}
	return line
}

// List answers the roster read, filtered. It is deliberately ungated: the human
// and the agents must never see different rosters, and visibility cannot depend
// on the caller having announced.
func (r *roster) List(p proto.SyncListParams) (proto.SyncResult, *proto.RPCError) {
	rows, partial := r.snapshot()
	out := make([]proto.SyncAgent, 0, len(rows))
	for _, a := range rows {
		if p.Area != "" && a.Area != p.Area {
			continue
		}
		if p.Project != "" && a.Project != p.Project {
			continue
		}
		out = append(out, a)
	}
	res := proto.SyncResult{OK: true, Agents: out, Partial: partial}
	// Peers still means "in MY area, not me", whatever the filter asked for.
	if key := strings.TrimSpace(p.Identity.Key); key != "" {
		r.mu.Lock()
		area := ""
		if row := r.rows[key]; row != nil {
			area = row.area
		}
		r.mu.Unlock()
		res.Peers = r.peersOf(key, area).Peers
	}
	return res, nil
}

// SeenUnattached records a session met through ordinary item traffic. Every
// item already carries an identity, so this costs nothing and it is what keeps
// the roster from claiming to be everybody.
func (r *roster) SeenUnattached(id proto.Identity) {
	key := strings.TrimSpace(id.Key)
	r.mu.Lock()
	defer r.mu.Unlock()
	if key != "" {
		if _, attached := r.rows[key]; attached {
			return
		}
		r.seenUnattached[key] = time.Now()
		return
	}
	// No key at all is the pre-sync case: an older child, or a CLI call. Keyed
	// by what identity it does have, which is enough to know somebody is there.
	r.seenUnattached[id.Agent+"\x1f"+id.Project] = time.Now()
}

// peersOf counts and lists the rows sharing an area, excluding the caller.
func (r *roster) peersOf(key, area string) proto.SyncResult {
	rows, partial := r.snapshot()
	out := make([]proto.SyncAgent, 0, len(rows))
	for _, a := range rows {
		if a.Key == key {
			continue
		}
		if area != "" && a.Area != area {
			continue
		}
		out = append(out, a)
	}
	return proto.SyncResult{Agents: out, Peers: len(out), Partial: partial}
}

// snapshot renders every row, with its state derived. The derivation is the
// whole reason the surface can be trusted: it reads the daemon's own facts
// first and the agent's self-report last.
func (r *roster) snapshot() ([]proto.SyncAgent, bool) {
	// Every observer is read BEFORE r.mu, and that is a rule rather than a
	// preference: the lock table asks the roster who a holder is while holding its
	// own mutex, so a roster that held r.mu while asking the lock table would give
	// the two subsystems opposite lock orders and deadlock the daemon.
	asking, driving := r.observed()
	holds, waits := r.lockState()

	r.mu.Lock()
	now := time.Now()
	// Reap provisional rows nothing has touched. A row with a live attach is
	// removed by the attach ending; a row without one has no such event, so its
	// only exit is this.
	for key, row := range r.rows {
		if !row.attached && now.Sub(row.touched) > provisionalFor {
			delete(r.rows, key)
		}
	}
	out := make([]proto.SyncAgent, 0, len(r.rows))
	for key, row := range r.rows {
		a := proto.SyncAgent{
			Key: key, Agent: row.identity.Agent, Project: row.identity.Project,
			Session: row.identity.Session, Cwd: row.cwd, PID: row.pid,
			Area: row.area, AreaPath: row.areaPath, Tags: row.tags,
			Purpose: row.purpose, Activity: row.activity,
			AgeMS: now.Sub(row.attachedAt).Milliseconds(),
		}
		if !row.activityAt.IsZero() {
			a.ActivitySinceMS = now.Sub(row.activityAt).Milliseconds()
		}
		a.Holds = holds[key]
		var wait *proto.SyncWait
		if w, ok := waits[key]; ok {
			// The holder's NAME comes from here rather than from the lock table: the
			// roster is the authority on identity (it is what keeps a hook's attach
			// from stamping a row `systemd`), and the lock only knows what the caller
			// that took it happened to send.
			if holder := r.rows[w.HolderKey]; holder != nil && holder.identity.Agent != "" {
				w.HolderAgent = holder.identity.Agent
			}
			wait = &w
			a.Waiting = wait
		}
		a.State, a.Detail = derivedState(row, key, asking, driving, wait, now)
		out = append(out, a)
	}
	partial := len(r.seenUnattached) > 0
	r.mu.Unlock()

	// Stable order so the surface does not reshuffle on every repaint: by area,
	// then by how long the session has been here.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Area != out[j].Area {
			return out[i].Area < out[j].Area
		}
		return out[i].AgeMS > out[j].AgeMS
	})
	return out, partial
}

// derivedState is the priority order, in one place. Facts the daemon can check
// come first; the agent's own activity line only decides between working and
// quiet, which is the most it should ever decide.
//
// A lock wait outranks working for the same reason asking does: the agent is
// stopped, and a row that says "working" while its call is parked behind another
// agent hides the one thing the human could act on. It names the holder, because
// "blocked" without a holder is a puzzle rather than a state.
func derivedState(row *rosterRow, key string, asking map[string]bool, driving string, wait *proto.SyncWait, now time.Time) (string, string) {
	switch {
	case asking[key]:
		return StateAsking, ""
	case driving != "" && driving == key:
		return StateDriving, ""
	case wait != nil:
		detail := "lock " + wait.Name
		if who := wait.HolderAgent; who != "" {
			detail += ", held by " + who
		}
		if wait.Ahead > 0 {
			detail += fmt.Sprintf(", %d ahead of you", wait.Ahead)
		}
		return StateBlocked, detail
	case !row.announced:
		return StateUnannounced, ""
	case !row.attached:
		// Announced by a hook or a CLI call, with nothing holding it open. It
		// says what the session is for and does not claim the session is live.
		return StateDetached, ""
	case row.activity == "":
		return StateQuiet, ""
	case now.Sub(row.activityAt) <= workingFor:
		return StateWorking, ""
	default:
		return StateQuiet, ""
	}
}

// changed pushes the roster at the surface, throttled. Called from every verb,
// so it must be cheap and must never hold the lock across the push.
//
// A push dropped here is not lost: it leaves dirty set, and the tick sends it.
func (r *roster) changed() {
	r.mu.Lock()
	push := r.push
	if push == nil {
		r.mu.Unlock()
		return
	}
	// Throttle by dropping intermediate pushes rather than queueing them: the
	// roster is a snapshot, so the newest one is the only one worth sending.
	if time.Since(r.lastGen) < rosterEmitEvery {
		r.dirty = true
		r.mu.Unlock()
		return
	}
	r.lastGen = time.Now()
	r.dirty = false
	r.mu.Unlock()

	r.emit(push)
}

// emit takes the snapshot, records what it says, and pushes it, all under
// pushMu. Recording inside the same critical section is the point: the tick
// decides whether to push by comparing against what the surface last received,
// so a record that described some other payload would make it skip a repaint the
// board needs.
func (r *roster) emit(push func([]proto.SyncAgent, bool)) {
	r.pushMu.Lock()
	defer r.pushMu.Unlock()

	rows, partial := r.snapshot()

	r.mu.Lock()
	states := make(map[string]string, len(rows))
	for _, a := range rows {
		states[a.Key] = a.State
	}
	r.lastState, r.lastPartial = states, partial
	r.mu.Unlock()

	push(rows, partial)
}

// tick is the roster looking at itself with nobody having called anything. It
// pushes when the board would otherwise be wrong, and stays silent when it would
// not, so an idle board costs one snapshot a second and no repaints.
//
// Two failures need it, and they look the same on screen. A throttled push left
// dirty has no other way out: the next verb might be a minute away, and in the
// field one was. And a row whose state decayed on time alone - "working" past
// workingFor with nothing to report since - is never recomputed by anything else,
// because every other push is caused by a change. That one is the worse of the
// two: a session that died mid-task keeps its "working" chip forever, which is
// precisely the hung session this surface exists to expose.
func (r *roster) tick() {
	r.mu.Lock()
	push := r.push
	if push == nil {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	r.pushMu.Lock()
	rows, partial := r.snapshot()

	r.mu.Lock()
	stale := r.dirty || partial != r.lastPartial || len(rows) != len(r.lastState)
	if !stale {
		for _, a := range rows {
			if was, ok := r.lastState[a.Key]; !ok || was != a.State {
				stale = true
				break
			}
		}
	}
	if !stale {
		r.mu.Unlock()
		r.pushMu.Unlock()
		return
	}
	states := make(map[string]string, len(rows))
	for _, a := range rows {
		states[a.Key] = a.State
	}
	r.lastState, r.lastPartial = states, partial
	r.dirty = false
	r.lastGen = time.Now()
	r.mu.Unlock()
	r.pushMu.Unlock()

	push(rows, partial)
}

// Start begins the tick. Idempotent, and safe to leave unstarted: a roster with
// no tick still answers every read correctly, it only lets the surface go stale,
// which is why the tests that care start it themselves.
func (r *roster) Start() {
	r.mu.Lock()
	if r.stop != nil {
		r.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	r.stop = stop
	r.mu.Unlock()

	go func() {
		t := time.NewTicker(rosterTickEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				r.tick()
			}
		}
	}()
}

// Stop ends the tick. The roster keeps every row: sessions outlive a shutdown of
// this loop, and a restarted tick picks them up as they are.
func (r *roster) Stop() {
	r.mu.Lock()
	if r.stop != nil {
		close(r.stop)
		r.stop = nil
	}
	r.mu.Unlock()
}

// askingKeys is the set of session keys with a blocking item of their own still
// pending. It is what makes "asking you" a fact rather than a claim: the daemon
// already holds every waiter, so the roster asks it instead of trusting an agent
// to say it is stuck on a question.
func (d *Daemon) askingKeys() map[string]bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]bool, len(d.waiters))
	mark := func(it *proto.Item) {
		if it == nil {
			return
		}
		if _, waiting := d.waiters[it.ID]; !waiting {
			return
		}
		if k := it.Identity.Key; k != "" {
			out[k] = true
		}
	}
	mark(d.current)
	for _, it := range d.queue {
		mark(it)
	}
	return out
}

// drivingKey is the session key holding the desktop, or empty. Read through the
// control subsystem's own lock, never d.mu.
func (d *Daemon) drivingKey() string { return d.control.holderKey() }

// SetRosterSurface wires the Agents surface to the roster, the same shape
// SetControlSurface has. The subsystem itself stays unexported: the app needs to
// connect it, not to reach into it.
func (d *Daemon) SetRosterSurface(push func([]proto.SyncAgent, bool)) {
	d.roster.SetPush(push)
}

// lockWarn is the lock table's one voice at the human: a deadlock refused, a
// wait long enough to be contention rather than progress, or a holder parked on
// a question only he can answer. A warning toast, through the normal display
// path, so it chimes and lands in history like anything else.
//
// Everything else about coordination stays quiet on purpose (vision principle 1):
// two agents taking turns correctly is not news.
func (d *Daemon) lockWarn(title, body string) {
	d.surfaceNotify(proto.LevelWarning, proto.Identity{Agent: "agentbox"}, title, body)
}

// StartLocks and StopLocks run the lock table's clock, which is what notices an
// orphan whose process has died and a wait worth warning about.
func (d *Daemon) StartLocks() { d.locks.Start() }
func (d *Daemon) StopLocks()  { d.locks.Stop() }

// BreakLock is the human's, from the Agents surface. It reassigns the lock and
// tells the ex-holder; it does not stop the ex-holder's work, and the copy beside
// the button has to say so.
func (d *Daemon) BreakLock(name string) proto.SyncLockResult { return d.locks.Break(name) }

// LockSnapshot is every lock the daemon knows, for the surface's pull on mount.
func (d *Daemon) LockSnapshot() []proto.SyncLockState { return d.locks.Snapshot() }

// SyncRider is what rides back on a response envelope (FR83). It answers for any
// method, because the point is that news reaches an agent through whatever it
// happened to be doing.
//
// Two families are excluded, and both for the same reason: they already answer
// the question. announce and list_agents return the roster in their own result,
// so a rider would say it twice; the cursor still moves, so the next call does
// not repeat what they just showed. attach is excluded because it returns once,
// at the end of the session.
func (d *Daemon) SyncRider(method string, params json.RawMessage) string {
	id := identityOf(params)
	if id.Via != proto.ViaMCP {
		// A shell cannot show the line, and spending the news on it would lose it:
		// a session's hooks call the CLI with that session's own key several times
		// a minute, so this is the difference between the rider working and the
		// rider being eaten before the model ever sees it.
		return ""
	}
	key := strings.TrimSpace(id.Key)
	// A lost lock rides the same envelope, and it goes first: "the human broke
	// your lock" outranks news about company, and it is owed to this session
	// whatever it called. Taken even for the methods that carry their own roster,
	// because nothing else in the system will ever mention it again.
	//
	// A daemon assembled by hand (the roster's own tests) has no lock table, and
	// the rider is still expected to work: presence never depends on the rest of
	// the daemon being there.
	var lines []string
	if d.locks != nil {
		lines = d.locks.TakeNotices(key)
	}
	switch method {
	case proto.MethodSyncAttach:
		// The attach returns once, at the end of a session. Anything put on it
		// would arrive as the agent stops existing, so a notice is kept for the
		// next real call instead of being spent here.
		d.locks.keepNotices(key, lines)
		return ""
	case proto.MethodSyncAnnounce, proto.MethodSyncList:
		// Move the cursor and say nothing about peers: whatever is here was in that
		// result. A broken lock was not, so it still rides.
		d.roster.riderFor(key)
		return strings.Join(lines, " ")
	}
	if peers := d.roster.riderFor(key); peers != "" {
		lines = append(lines, peers)
	}
	return strings.Join(lines, " ")
}

// identityOf reads the caller identity out of any params object. Every method's
// params carry it in the same place, so this does not need to know which method it
// is looking at - and a params blob without one simply has no session to tell
// anything to.
func identityOf(params json.RawMessage) proto.Identity {
	if len(params) == 0 {
		return proto.Identity{}
	}
	var p struct {
		Identity proto.Identity `json:"identity"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return proto.Identity{}
	}
	return p.Identity
}

// StartRoster begins the roster's own tick, which is what keeps the Agents
// surface honest between calls. Wire the surface first: a tick with nowhere to
// push is a no-op.
func (d *Daemon) StartRoster() { d.roster.Start() }

// StopRoster ends it.
func (d *Daemon) StopRoster() { d.roster.Stop() }

// RosterSnapshot is the roster as a caller outside the package sees it, for the
// surface's pull on mount.
func (d *Daemon) RosterSnapshot() ([]proto.SyncAgent, bool) { return d.roster.snapshot() }

// DeriveArea is the area an agent belongs to when it declares nothing (FR83's
// triage: derived first, declared second). One repo is one area, worktrees
// included, because the collision that actually happened was three agents in
// one checkout.
//
// The git top-level is the honest key. It is read from the filesystem rather
// than by running git, so this cannot block on a slow repo or a missing binary.
func DeriveArea(cwd string) string {
	area, _ := deriveArea(cwd)
	return area
}

// deriveArea also answers WHERE the area is, which is a different question from
// where the agent is: two agents in one repo stand in different subdirectories,
// and the area they share is the root above both.
func deriveArea(cwd string) (area, dir string) {
	dir = strings.TrimSpace(cwd)
	if dir == "" {
		return "", ""
	}
	for {
		if _, err := os.Stat(dir + "/.git"); err == nil {
			return "repo:" + baseName(dir), dir
		}
		parent := parentDir(dir)
		if parent == dir {
			return "dir:" + baseName(cwd), strings.TrimSpace(cwd)
		}
		dir = parent
	}
}

// areaPathOf is the path a surface may caption an area group with, and empty when
// there is no honest answer. It is empty whenever the row's area is not the one
// its cwd would derive: an agent that declares `repo:laptop-setup` while standing
// in the agentbox tree belongs to that area, but the agentbox path is not where
// that area lives, and a header saying otherwise states a falsehood.
func areaPathOf(cwd, area string) string {
	if cwd == "" || area == "" {
		return ""
	}
	derived, dir := deriveArea(cwd)
	if derived != area {
		return ""
	}
	return dir
}

func baseName(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func parentDir(p string) string {
	p = strings.TrimRight(p, "/")
	i := strings.LastIndex(p, "/")
	switch {
	case i > 0:
		return p[:i]
	case i == 0:
		return "/"
	default:
		return p
	}
}
