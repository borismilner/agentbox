package daemon

import (
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
)

// FR89. agentbox had four ways to CREATE an item and none to retire one, so a
// warning that turned out to be noise waited on the human's screen until he clicked
// it - and came back after every daemon restart, because pending items are restored
// by design. Boris asked about the same probe-generated toasts twice.

func postWarning(t *testing.T, d *Daemon, agent, key, title string) string {
	t.Helper()
	return callNotify(t, d, proto.Item{Kind: proto.KindNotify, Level: proto.LevelWarning,
		Title: title, Identity: proto.Identity{Agent: agent, Key: key}}).ID
}

func TestDismissRetiresOneNamedItem(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	id := postWarning(t, d, "claude", "k1", "Deadlock refused: probe:repo")

	res, rpcErr := d.DismissItems(proto.DismissParams{ID: id, Human: true})
	if rpcErr != nil {
		t.Fatalf("dismiss: %s", rpcErr.Message)
	}
	if res.Dismissed != 1 || len(res.IDs) != 1 || res.IDs[0] != id {
		t.Fatalf("want exactly that item retired, got %+v", res)
	}
	// And it is gone from the queue, so a restart cannot bring it back.
	pending, err := d.st.Pending()
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("the item is still pending: %+v", pending)
	}
}

func TestDismissAllIsTheHumansAlone(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	postWarning(t, d, "claude", "k1", "one")
	postWarning(t, d, "codex", "k2", "two")

	// An agent must not be able to empty his queue: one that could would be able to
	// hide a question it did not want answered.
	_, rpcErr := d.DismissItems(proto.DismissParams{All: true,
		Identity: proto.Identity{Agent: "claude", Key: "k1"}})
	if rpcErr == nil {
		t.Fatal("an agent was allowed to clear every pending item")
	}

	res, rpcErr := d.DismissItems(proto.DismissParams{All: true, Human: true})
	if rpcErr != nil {
		t.Fatalf("dismiss --all: %s", rpcErr.Message)
	}
	if res.Dismissed != 2 {
		t.Fatalf("the human's own sweep should take both, got %d", res.Dismissed)
	}
}

// An agent's retraction reaches its own items and stops there. Retiring another
// agent's item would be answering its question for it.
func TestRetractTouchesOnlyTheCallersOwnItems(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	mine := postWarning(t, d, "claude", "k1", "mine")
	theirs := postWarning(t, d, "codex", "k2", "theirs")

	_, rpcErr := d.DismissItems(proto.DismissParams{ID: theirs,
		Identity: proto.Identity{Agent: "claude", Key: "k1"}})
	if rpcErr == nil {
		t.Fatal("an agent retracted another agent's item")
	}

	res, rpcErr := d.DismissItems(proto.DismissParams{Mine: true,
		Identity: proto.Identity{Agent: "claude", Key: "k1"}})
	if rpcErr != nil {
		t.Fatalf("retract: %s", rpcErr.Message)
	}
	if res.Dismissed != 1 || res.IDs[0] != mine {
		t.Fatalf("a sweep should take this session's item and no other, got %+v", res)
	}
	pending, err := d.st.Pending()
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != theirs {
		t.Fatalf("the other agent's item should survive, got %+v", pending)
	}
}

func TestDismissNeedsAnIDOrAll(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	if _, rpcErr := d.DismissItems(proto.DismissParams{Human: true}); rpcErr == nil {
		t.Fatal("a bare dismiss with nothing to act on should teach, not sweep")
	}
}

func TestDismissAnUnknownIDSaysSo(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	_, rpcErr := d.DismissItems(proto.DismissParams{ID: "kdoesnotexist", Human: true})
	if rpcErr == nil || rpcErr.Code != proto.CodeItemNotFound {
		t.Fatalf("want an item-not-found refusal, got %v", rpcErr)
	}
}
