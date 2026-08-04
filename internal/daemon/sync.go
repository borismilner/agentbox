package daemon

import (
	"context"
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

type rosterRow struct {
	identity proto.Identity
	cwd      string
	pid      int
	area     string
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
	// human answer, and who holds the desktop. Both are already tracked
	// elsewhere, so the roster asks rather than duplicating.
	askingFn  func() map[string]bool
	drivingFn func() string

	push    func([]proto.SyncAgent, bool)
	lastGen time.Time
	dirty   bool
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
	row.identity = p.Identity
	row.attached, row.touched = true, now
	row.cwd, row.pid = p.Cwd, p.PID
	if p.Area != "" {
		row.area = p.Area
	}
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
	r.mu.Unlock()
	r.log.Info(logging.EvSync, "component", "daemon", "sync", "detach", "key", key)
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
	}
	row.touched = now
	row.purpose = strings.TrimSpace(p.Purpose)
	row.announced = true
	if p.Activity != "" {
		row.activity, row.activityAt = p.Activity, now
	}
	if p.Area != "" {
		row.area = p.Area
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
	r.mu.Lock()
	asking := map[string]bool{}
	if r.askingFn != nil {
		asking = r.askingFn()
	}
	driving := ""
	if r.drivingFn != nil {
		driving = r.drivingFn()
	}
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
			Area: row.area, Tags: row.tags,
			Purpose: row.purpose, Activity: row.activity,
			AgeMS: now.Sub(row.attachedAt).Milliseconds(),
		}
		if !row.activityAt.IsZero() {
			a.ActivitySinceMS = now.Sub(row.activityAt).Milliseconds()
		}
		a.State, a.Detail = derivedState(row, key, asking, driving, now)
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
func derivedState(row *rosterRow, key string, asking map[string]bool, driving string, now time.Time) (string, string) {
	switch {
	case asking[key]:
		return StateAsking, ""
	case driving != "" && driving == key:
		return StateDriving, ""
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

	rows, partial := r.snapshot()
	push(rows, partial)
}

// Flush sends whatever the throttle held back. The daemon ticks this so a final
// activity line is never the one that got dropped.
func (r *roster) Flush() {
	r.mu.Lock()
	dirty, push := r.dirty, r.push
	if !dirty || push == nil {
		r.mu.Unlock()
		return
	}
	r.dirty = false
	r.lastGen = time.Now()
	r.mu.Unlock()

	rows, partial := r.snapshot()
	push(rows, partial)
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
	dir := strings.TrimSpace(cwd)
	if dir == "" {
		return ""
	}
	for {
		if _, err := os.Stat(dir + "/.git"); err == nil {
			return "repo:" + baseName(dir)
		}
		parent := parentDir(dir)
		if parent == dir {
			return "dir:" + baseName(cwd)
		}
		dir = parent
	}
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
