package daemon

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

func newTestRoster() *roster {
	return newRoster(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func id(agent, project, key string) proto.Identity {
	return proto.Identity{Agent: agent, Project: project, Key: key}
}

// attached runs an attach in the background and returns a cancel that plays the
// part of the session going away.
func attached(t *testing.T, r *roster, i proto.Identity, cwd, area string) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Attach(ctx, proto.SyncAttachParams{Identity: i, Cwd: cwd, Area: area, PID: os.Getpid()})
	}()
	// Wait for the ATTACH, not merely for a row: a row may already exist because
	// a hook announced on this session's behalf, and returning on that would let
	// the test read the roster before the connection was registered.
	deadline := time.Now().Add(2 * time.Second)
	for {
		r.mu.Lock()
		row := r.rows[i.Key]
		live := row != nil && row.attached
		r.mu.Unlock()
		if live {
			return func() { cancel(); <-done }
		}
		if time.Now().After(deadline) {
			t.Fatalf("attach never registered a live row for %s", i.Key)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestAttachIsThePresenceAndDetachTakesTheRowWithIt(t *testing.T) {
	// The whole design rests on this: the call being open IS the agent existing,
	// so the row must appear on attach and be gone the moment the context ends.
	// A roster that kept a dead agent's row would be worse than no roster - the
	// human would coordinate around somebody who left.
	r := newTestRoster()
	stop := attached(t, r, id("claude", "agentbox", "k1"), "/tmp/agentbox", "repo:agentbox")

	rows, _ := r.snapshot()
	if len(rows) != 1 || rows[0].Key != "k1" {
		t.Fatalf("attach did not register one row: %+v", rows)
	}
	if rows[0].State != StateUnannounced {
		t.Errorf("a session that never announced reads %q, want %q - it must be visible but dim",
			rows[0].State, StateUnannounced)
	}

	stop()
	if rows, _ := r.snapshot(); len(rows) != 0 {
		t.Errorf("the row outlived its session: %+v", rows)
	}
}

func TestAnnounceAnswersWithTheAgentsAlreadyInYourArea(t *testing.T) {
	// Boris: agents "should easily find existing or new joining agents that are
	// working on the same area". The first call an agent makes has to answer that,
	// or it learns it has company only after the damage.
	r := newTestRoster()
	defer attached(t, r, id("claude", "agentbox", "first"), "/repo", "repo:agentbox")()
	if _, err := r.Announce(proto.SyncAnnounceParams{
		Identity: id("claude", "agentbox", "first"), Purpose: "shipping the roster",
	}); err != nil {
		t.Fatalf("first announce: %v", err)
	}

	defer attached(t, r, id("claude", "agentbox", "second"), "/repo", "repo:agentbox")()
	res, err := r.Announce(proto.SyncAnnounceParams{
		Identity: id("claude", "agentbox", "second"), Purpose: "fixing the inbox reader",
	})
	if err != nil {
		t.Fatalf("second announce: %v", err)
	}
	if res.Peers != 1 || len(res.Agents) != 1 {
		t.Fatalf("the joining agent was told it had %d peers: %+v", res.Peers, res.Agents)
	}
	if got := res.Agents[0]; got.Key != "first" || got.Purpose != "shipping the roster" {
		t.Errorf("the peer came back as %+v, without the purpose that makes it useful", got)
	}
}

func TestAnotherAreaIsNotYourCompany(t *testing.T) {
	// Grouping is the whole value: two agents in one repo can collide, two in
	// different repos cannot, and telling an agent about the second kind trains
	// it to ignore the first.
	r := newTestRoster()
	defer attached(t, r, id("claude", "grabbit", "elsewhere"), "/other", "repo:grabbit")()
	res, err := r.Announce(proto.SyncAnnounceParams{
		Identity: id("claude", "agentbox", "mine"), Purpose: "shipping the roster", Area: "repo:agentbox",
	})
	if err != nil {
		t.Fatalf("announce: %v", err)
	}
	if res.Peers != 0 {
		t.Errorf("an agent in another repo was reported as company: %+v", res.Agents)
	}
}

func TestRepeatingAnActivityLineDoesNotHideAStall(t *testing.T) {
	// Same rule the control strip already applies, for the same reason: an agent
	// that keeps re-sending one line has not made progress, and a row whose age
	// resets on every repeat reads as busy forever.
	r := newTestRoster()
	defer attached(t, r, id("claude", "agentbox", "k1"), "/repo", "repo:agentbox")()
	r.Announce(proto.SyncAnnounceParams{Identity: id("claude", "agentbox", "k1"), Purpose: "p"})
	r.Activity(proto.SyncActivityParams{Identity: id("claude", "agentbox", "k1"), Activity: "make check"})

	time.Sleep(40 * time.Millisecond)
	r.Activity(proto.SyncActivityParams{Identity: id("claude", "agentbox", "k1"), Activity: "make check"})
	rows, _ := r.snapshot()
	if rows[0].ActivitySinceMS < 30 {
		t.Errorf("an unchanged line reset its age to %dms, so a stalled agent looks busy", rows[0].ActivitySinceMS)
	}

	r.Activity(proto.SyncActivityParams{Identity: id("claude", "agentbox", "k1"), Activity: "make deploy"})
	rows, _ = r.snapshot()
	if rows[0].ActivitySinceMS > 30 {
		t.Errorf("a genuinely new line came back %dms old", rows[0].ActivitySinceMS)
	}
	if rows[0].State != StateWorking {
		t.Errorf("a fresh activity line reads %q, want working", rows[0].State)
	}
}

func TestAskingOutranksWorking(t *testing.T) {
	// The priority order is not cosmetic. An agent parked on a question the human
	// has not answered is the one row worth walking over to, and "working" would
	// bury it - the daemon knows which it is, so the agent never gets to choose.
	r := newTestRoster()
	r.SetObservers(func() map[string]bool { return map[string]bool{"k1": true} }, func() string { return "" })
	defer attached(t, r, id("claude", "agentbox", "k1"), "/repo", "repo:agentbox")()
	r.Announce(proto.SyncAnnounceParams{Identity: id("claude", "agentbox", "k1"), Purpose: "p"})
	r.Activity(proto.SyncActivityParams{Identity: id("claude", "agentbox", "k1"), Activity: "editing a file"})

	rows, _ := r.snapshot()
	if rows[0].State != StateAsking {
		t.Errorf("a session with a pending question reads %q, want %q", rows[0].State, StateAsking)
	}
}

func TestDrivingIsReadFromTheDaemonNotFromTheAgent(t *testing.T) {
	r := newTestRoster()
	r.SetObservers(func() map[string]bool { return nil }, func() string { return "k1" })
	defer attached(t, r, id("claude", "agentbox", "k1"), "/repo", "repo:agentbox")()
	r.Announce(proto.SyncAnnounceParams{Identity: id("claude", "agentbox", "k1"), Purpose: "p"})

	rows, _ := r.snapshot()
	if rows[0].State != StateDriving {
		t.Errorf("the desktop holder reads %q, want %q", rows[0].State, StateDriving)
	}
}

func TestAnUnattachedSessionMakesEveryReadPartial(t *testing.T) {
	// FR61's rule applied to presence: "you are alone" must be true when it is
	// said, or not said at all. A session whose mcp child predates sync has no
	// attach, so a roster that quietly omitted it would let a second agent
	// conclude it had the repo to itself.
	r := newTestRoster()
	defer attached(t, r, id("claude", "agentbox", "mine"), "/repo", "repo:agentbox")()

	if _, partial := r.snapshot(); partial {
		t.Fatal("partial before any unattached session was seen")
	}
	r.SeenUnattached(id("claude", "agentbox", ""))

	res, err := r.Announce(proto.SyncAnnounceParams{
		Identity: id("claude", "agentbox", "mine"), Purpose: "shipping the roster",
	})
	if err != nil {
		t.Fatalf("announce: %v", err)
	}
	if !res.Partial {
		t.Error("a roster that cannot see everybody reported itself as complete")
	}
	if res.Peers != 0 {
		t.Errorf("peers=%d, so partial is doing the work of saying 'somebody may be here'", res.Peers)
	}
}

func TestAnAttachedSessionIsNotAlsoCountedAsUnattached(t *testing.T) {
	// The attached session's own items flow through the same path that records
	// unattached sessions, so without this every roster would call itself partial
	// forever and the honesty signal would mean nothing.
	r := newTestRoster()
	defer attached(t, r, id("claude", "agentbox", "k1"), "/repo", "repo:agentbox")()
	r.SeenUnattached(id("claude", "agentbox", "k1"))
	if _, partial := r.snapshot(); partial {
		t.Error("an attached session's own item traffic marked the roster partial")
	}
}

func TestAnnounceRefusesWithoutAKeyAndSaysWhy(t *testing.T) {
	// Self-teaching refusals are the house rule: an agent that gets this wrong
	// must learn the contract from the error, not from the manual.
	r := newTestRoster()
	_, err := r.Announce(proto.SyncAnnounceParams{Identity: id("claude", "agentbox", ""), Purpose: "p"})
	if err == nil {
		t.Fatal("announce accepted a session with no key, which is the collision this feature fixes")
	}
	if _, err := r.Announce(proto.SyncAnnounceParams{Identity: id("claude", "agentbox", "k1")}); err == nil {
		t.Error("announce accepted an empty purpose, which is the one thing it exists to collect")
	}
}

func TestAnnounceSurvivesADaemonRestartByReplay(t *testing.T) {
	// A reconnecting child replays its announce. The row must come back with the
	// purpose intact rather than being demoted to "no purpose given" until the
	// model happens to speak again.
	r := newTestRoster()
	stop := attached(t, r, id("claude", "agentbox", "k1"), "/repo", "repo:agentbox")
	r.Announce(proto.SyncAnnounceParams{Identity: id("claude", "agentbox", "k1"), Purpose: "shipping the roster"})
	stop()

	// The child redials and replays.
	defer attached(t, r, id("claude", "agentbox", "k1"), "/repo", "repo:agentbox")()
	r.Announce(proto.SyncAnnounceParams{Identity: id("claude", "agentbox", "k1"), Purpose: "shipping the roster"})
	rows, _ := r.snapshot()
	if len(rows) != 1 || rows[0].Purpose != "shipping the roster" {
		t.Errorf("the roster did not heal: %+v", rows)
	}
}

func TestAHookAnnouncedRowDoesNotClaimTheSessionIsLive(t *testing.T) {
	// The SessionStart hook in recipes.md announces before the agent's own child
	// has attached, so a row with nothing holding it open is the normal case for
	// a few seconds. It must say what the session is for without claiming
	// anybody is still there - otherwise the board shows a working agent for a
	// session that never started.
	r := newTestRoster()
	if _, err := r.Announce(proto.SyncAnnounceParams{
		Identity: id("claude", "agentbox", "hooked"), Purpose: "agentbox session (purpose not yet stated)",
	}); err != nil {
		t.Fatalf("announce: %v", err)
	}
	rows, _ := r.snapshot()
	if len(rows) != 1 {
		t.Fatalf("a hook announce produced %d rows", len(rows))
	}
	if rows[0].State != StateDetached {
		t.Errorf("a row with no attach reads %q, want %q", rows[0].State, StateDetached)
	}

	// And the real attach takes it over rather than making a second row.
	defer attached(t, r, id("claude", "agentbox", "hooked"), "/repo", "repo:agentbox")()
	rows, _ = r.snapshot()
	if len(rows) != 1 {
		t.Fatalf("the child's attach made a second row: %+v", rows)
	}
	if rows[0].Purpose != "agentbox session (purpose not yet stated)" {
		t.Errorf("the attach dropped the purpose the hook had already posted: %+v", rows[0])
	}
	if rows[0].State == StateDetached {
		t.Error("the row still reads detached with a live attach holding it open")
	}
}

func TestAProvisionalRowIsReapedRatherThanLivingForever(t *testing.T) {
	// A row with a live attach is removed when the attach ends. A row without one
	// has no such event, so if nothing reaped it the board would fill with
	// sessions that ended days ago - and every session start adds one, because
	// the hook recipe announces on every session start.
	r := newTestRoster()
	r.Announce(proto.SyncAnnounceParams{
		Identity: id("claude", "agentbox", "stale"), Purpose: "long gone",
	})
	r.mu.Lock()
	r.rows["stale"].touched = time.Now().Add(-2 * provisionalFor)
	r.mu.Unlock()

	if rows, _ := r.snapshot(); len(rows) != 0 {
		t.Errorf("a provisional row nothing has touched for %s is still on the board: %+v",
			2*provisionalFor, rows)
	}
}

func TestAnAttachedRowIsNeverReapedForBeingQuiet(t *testing.T) {
	// The reaper must only ever take provisional rows. An agent thinking hard for
	// twenty minutes is still there, and dropping its row would be the roster
	// lying in the other direction.
	r := newTestRoster()
	defer attached(t, r, id("claude", "agentbox", "thinker"), "/repo", "repo:agentbox")()
	r.Announce(proto.SyncAnnounceParams{Identity: id("claude", "agentbox", "thinker"), Purpose: "thinking"})
	r.mu.Lock()
	r.rows["thinker"].touched = time.Now().Add(-10 * provisionalFor)
	r.mu.Unlock()

	rows, _ := r.snapshot()
	if len(rows) != 1 {
		t.Fatalf("a live attached session was reaped for being quiet: %+v", rows)
	}
	if rows[0].State != StateQuiet {
		t.Errorf("a long-silent attached session reads %q, want %q", rows[0].State, StateQuiet)
	}
}

func TestDeriveAreaPutsOneRepoInOneArea(t *testing.T) {
	// The collision that actually happened was three agents in ONE checkout, so
	// the derived area has to be the repo and nothing finer. A subdirectory deep
	// inside the tree must land in the same area as its root.
	root := t.TempDir()
	if err := os.Mkdir(root+"/.git", 0o755); err != nil {
		t.Fatal(err)
	}
	deep := root + "/internal/daemon"
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	want := "repo:" + baseName(root)
	if got := DeriveArea(root); got != want {
		t.Errorf("the repo root derived %q, want %q", got, want)
	}
	if got := DeriveArea(deep); got != want {
		t.Errorf("a subdirectory derived %q, want %q - two agents in one repo would not see each other", got, want)
	}

	// Outside a repo there is still an answer, because a row with no area cannot
	// be grouped and would read as nowhere.
	plain := t.TempDir()
	if got := DeriveArea(plain); got != "dir:"+baseName(plain) {
		t.Errorf("a non-repo directory derived %q", got)
	}
	if got := DeriveArea(""); got != "" {
		t.Errorf("an empty cwd derived %q, want empty", got)
	}
}

func TestListIsUngatedSoVisibilityNeverDependsOnManners(t *testing.T) {
	// The design's rule: the sync verbs may refuse an unannounced session, but
	// the reads never do. A human and an agent must not be able to see different
	// rosters, and a rude agent's row is exactly the one worth seeing.
	r := newTestRoster()
	defer attached(t, r, id("claude", "agentbox", "rude"), "/repo", "repo:agentbox")()
	res, err := r.List(proto.SyncListParams{})
	if err != nil {
		t.Fatalf("list refused a caller with no key at all: %v", err)
	}
	if len(res.Agents) != 1 {
		t.Fatalf("list returned %d rows", len(res.Agents))
	}
	if res.Agents[0].State != StateUnannounced {
		t.Errorf("the never-announced row reads %q", res.Agents[0].State)
	}
}

func TestTheSurfaceIsPushedWhenTheRosterChanges(t *testing.T) {
	// Boris watches this board rather than a terminal, so a change nobody pushes
	// is a change he does not see.
	r := newTestRoster()
	got := make(chan int, 8)
	r.SetPush(func(rows []proto.SyncAgent, _ bool) { got <- len(rows) })

	stop := attached(t, r, id("claude", "agentbox", "k1"), "/repo", "repo:agentbox")
	defer stop()
	select {
	case n := <-got:
		if n != 1 {
			t.Errorf("the surface was pushed %d rows on attach", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("attaching an agent pushed nothing at the surface")
	}
}
