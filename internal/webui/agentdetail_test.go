package webui

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// fakeRoster is the daemon's half of an opened row.
type fakeRoster struct {
	detail proto.SyncAgentDetail
	asked  []string
}

func (f *fakeRoster) BreakLock(string) error       { return nil }
func (f *fakeRoster) Locks() []proto.SyncLockState { return nil }
func (f *fakeRoster) Shared() []proto.SharedValue  { return nil }
func (f *fakeRoster) AgentDetail(key string) proto.SyncAgentDetail {
	f.asked = append(f.asked, key)
	return f.detail
}

func detailUI(t *testing.T, r Roster, src Source) *UI {
	t.Helper()
	u := &UI{log: slog.New(slog.NewTextHandler(io.Discard, nil)), theme: Theme{Mode: "dark"}}
	u.inbox = newInbox(u)
	u.agents = newAgents(u)
	u.roster = r
	u.src = src
	return u
}

func TestAgentDetailJoinsTheDaemonAndTheStore(t *testing.T) {
	// Three owners meet in this call and none of them knows about the others.
	r := &fakeRoster{detail: proto.SyncAgentDetail{
		Key: "sk-1", Found: true,
		Timeline: []proto.SyncTick{{Line: "reading the spec", SinceMS: 60_000}},
		Signals: []proto.SyncSignalTick{
			{Topic: "tests:green", Dir: "received", SinceMS: 30_000, Data: `{"n":1}`},
			{Topic: "deploy:start", Dir: "posted", SinceMS: 10_000},
		},
	}}
	src := &fakeSource{items: []store.StoredItem{
		{Item: proto.Item{ID: "k1", Kind: proto.KindChoice, Title: "which branch?",
			Identity: proto.Identity{Agent: "claude", Key: "sk-1"}},
			State: store.StatePending, CreatedAt: time.Now().Add(-5 * time.Minute)},
	}}
	b := &Bridge{ui: detailUI(t, r, src)}

	got := b.AgentDetail("sk-1")
	if !got.Found {
		t.Fatal("a live session came back not found")
	}
	if len(got.Timeline) != 1 || got.Timeline[0].Line != "reading the spec" {
		t.Fatalf("timeline = %+v", got.Timeline)
	}
	if len(got.Signals) != 2 || got.Signals[0].Dir != "received" || got.Signals[1].Dir != "posted" {
		t.Fatalf("signals = %+v", got.Signals)
	}
	if len(got.Items) != 1 || got.Items[0].Title != "which branch?" {
		t.Fatalf("items = %+v", got.Items)
	}
	// The pending one is what the human is being waited on for, and naming it is
	// the whole reason the block leads with a sentence rather than a list.
	if got.Pending != "which branch?" {
		t.Fatalf("pending = %q", got.Pending)
	}
	if len(r.asked) != 1 || r.asked[0] != "sk-1" {
		t.Fatalf("the roster was asked %v", r.asked)
	}
}

func TestAgentDetailSeesOnlyItsOwnSessionsItems(t *testing.T) {
	// The point of matching on the session key: two Claude sessions in one repo
	// are the identical agent/project/session triple.
	r := &fakeRoster{detail: proto.SyncAgentDetail{Key: "sk-1", Found: true}}
	src := &fakeSource{items: []store.StoredItem{
		{Item: proto.Item{ID: "k1", Title: "mine", Identity: proto.Identity{Agent: "claude", Project: "agentbox", Key: "sk-1"}},
			State: store.StateAnswered, CreatedAt: time.Now()},
		{Item: proto.Item{ID: "k2", Title: "the neighbour's", Identity: proto.Identity{Agent: "claude", Project: "agentbox", Key: "sk-2"}},
			State: store.StateAnswered, CreatedAt: time.Now()},
	}}
	b := &Bridge{ui: detailUI(t, r, src)}

	got := b.AgentDetail("sk-1")
	if len(got.Items) != 1 || got.Items[0].Title != "mine" {
		t.Fatalf("items = %+v, want only this session's", got.Items)
	}
	if got.Pending != "" {
		t.Fatalf("pending = %q with nothing pending", got.Pending)
	}
}

func TestAgentDetailOfAGoneSession(t *testing.T) {
	// Not found and empty are different answers, and the surface says different
	// things for them: one is "it left", the other "it has done nothing yet".
	r := &fakeRoster{detail: proto.SyncAgentDetail{Key: "sk-gone"}}
	b := &Bridge{ui: detailUI(t, r, &fakeSource{})}
	if got := b.AgentDetail("sk-gone"); got.Found {
		t.Fatal("a gone session came back found")
	}
}

func TestAgentDetailFallsBackToTheDemoFixture(t *testing.T) {
	// `agentbox webui-demo agents` has no daemon behind it, and its rows carry
	// these fields inline. One code path in the surface either way.
	u := detailUI(t, nil, nil)
	u.agents.set(wireRoster{Agents: []wireAgent{{
		Key:      "demo-1",
		Timeline: []wireTick{{Line: "canned", SinceMS: 1000}},
		Pending:  "a canned question",
	}}})
	b := &Bridge{ui: u}

	got := b.AgentDetail("demo-1")
	if !got.Found || len(got.Timeline) != 1 || got.Pending != "a canned question" {
		t.Fatalf("fixture detail = %+v", got)
	}
	if missing := b.AgentDetail("nobody"); missing.Found {
		t.Fatal("a key the fixture does not have came back found")
	}
}
