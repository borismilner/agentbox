package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/walkthrough"
)

func createWt(t *testing.T, d *Daemon, id string) {
	t.Helper()
	_, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughCreate, mustJSON(t, map[string]any{
		"id": id, "spec": wtSpec(), "no_show": true,
		"identity": proto.Identity{Agent: "claude-code"},
	}))
	if rpcErr != nil {
		t.Fatalf("create: %v", rpcErr)
	}
}

func wtState(t *testing.T, d *Daemon, id string) string {
	t.Helper()
	w, err := d.st.GetWalkthrough(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return w.State
}

func TestWalkthroughSubmitDeliversToWaiter(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	createWt(t, d, "w00000000000a")

	done := make(chan proto.WalkthroughAwaitResult, 1)
	go func() {
		done <- d.AwaitWalkthrough(context.Background(), proto.WalkthroughAwait{ID: "w00000000000a"})
	}()
	waitFor(t, "a waiter to park", func() bool {
		d.wt.mu.Lock()
		defer d.wt.mu.Unlock()
		return len(d.wt.waiters) == 1
	})

	delivered, atMS, err := d.BoardSubmit("w00000000000a")
	if err != nil || !delivered || atMS == 0 {
		t.Fatalf("submit with a waiter: delivered=%v at=%d err=%v", delivered, atMS, err)
	}
	select {
	case res := <-done:
		if !res.Submitted || res.TimedOut || res.Gone {
			t.Fatalf("await result: %+v", res)
		}
		var p map[string]any
		if err := json.Unmarshal(res.Payload, &p); err != nil {
			t.Fatalf("payload does not parse: %v", err)
		}
		if p["walkthrough_id"] != "w00000000000a" || p["version"] != float64(1) {
			t.Errorf("payload: %v %v", p["walkthrough_id"], p["version"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("submission never reached the waiting agent")
	}
	if s := wtState(t, d, "w00000000000a"); s != store.WtDelivered {
		t.Errorf("state after live delivery = %s", s)
	}
}

func TestWalkthroughSubmitParksWhenNobodyWaits(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	createWt(t, d, "w00000000000b")

	delivered, _, err := d.BoardSubmit("w00000000000b")
	if err != nil || delivered {
		t.Fatalf("submit with nobody waiting: delivered=%v err=%v", delivered, err)
	}
	if s := wtState(t, d, "w00000000000b"); s != store.WtSubmitted {
		t.Fatalf("state after parked submit = %s", s)
	}

	// The next session picks it up exactly once through read with ack.
	res, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughRead,
		mustJSON(t, map[string]any{"id": "w00000000000b", "ack": true}))
	if rpcErr != nil {
		t.Fatalf("read ack: %v", rpcErr)
	}
	if st := res.(*proto.WalkthroughState); len(st.Payload) == 0 || st.State != store.WtDelivered {
		t.Errorf("acked read: state=%s payload=%d bytes", st.State, len(st.Payload))
	}
}

func TestWalkthroughAwaitFindsBacklog(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	createWt(t, d, "w00000000000c")
	if _, _, err := d.BoardSubmit("w00000000000c"); err != nil {
		t.Fatal(err)
	}

	// Named and unnamed waits both claim a submission that predates them;
	// the second finds nothing left.
	res := d.AwaitWalkthrough(context.Background(), proto.WalkthroughAwait{ID: "w00000000000c"})
	if !res.Submitted || len(res.Payload) == 0 {
		t.Fatalf("named await on a parked submission: %+v", res)
	}
	if s := wtState(t, d, "w00000000000c"); s != store.WtDelivered {
		t.Errorf("state = %s", s)
	}
	res = d.AwaitWalkthrough(context.Background(), proto.WalkthroughAwait{TimeoutS: 1})
	if !res.TimedOut {
		t.Errorf("a delivered review must not be claimable twice: %+v", res)
	}
	d.wt.mu.Lock()
	if len(d.wt.waiters) != 0 {
		t.Error("a timed-out waiter must deregister")
	}
	d.wt.mu.Unlock()
}

func TestWalkthroughAwaitAnyFindsBacklog(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	createWt(t, d, "w00000000000d")
	if _, _, err := d.BoardSubmit("w00000000000d"); err != nil {
		t.Fatal(err)
	}
	res := d.AwaitWalkthrough(context.Background(), proto.WalkthroughAwait{})
	if !res.Submitted {
		t.Fatalf("unnamed await must find the parked submission: %+v", res)
	}
}

func TestWalkthroughDeleteReleasesWaiter(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	createWt(t, d, "w00000000000e")

	done := make(chan proto.WalkthroughAwaitResult, 1)
	go func() {
		done <- d.AwaitWalkthrough(context.Background(), proto.WalkthroughAwait{ID: "w00000000000e"})
	}()
	waitFor(t, "a waiter to park", func() bool {
		d.wt.mu.Lock()
		defer d.wt.mu.Unlock()
		return len(d.wt.waiters) == 1
	})
	if _, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughDelete,
		mustJSON(t, map[string]any{"id": "w00000000000e"})); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	select {
	case res := <-done:
		if !res.Gone || res.Submitted {
			t.Fatalf("await after delete: %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("delete never released the waiter")
	}
}

func TestWalkthroughSubmitGateRefusesHollowUnclear(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	createWt(t, d, "w00000000000f")
	if err := d.BoardVerdict("w00000000000f", "xkb", "unclear"); err != nil {
		t.Fatal(err)
	}

	_, _, err := d.BoardSubmit("w00000000000f")
	gate, ok := errors.AsType[*walkthrough.GateError](err)
	if !ok || gate.StepID != "xkb" {
		t.Fatalf("hollow unclear must refuse and name the step: %v", err)
	}
	if s := wtState(t, d, "w00000000000f"); s != store.WtOpen {
		t.Errorf("a refused submission must change nothing, state = %s", s)
	}

	if err := d.BoardNote("w00000000000f", "xkb", "which key releases the group?"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.BoardSubmit("w00000000000f"); err != nil {
		t.Fatalf("worded unclear must pass the gate: %v", err)
	}
	w, err := d.st.GetWalkthrough("w00000000000f")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Unclear []struct {
			StepID string `json:"step_id"`
		} `json:"unclear"`
	}
	if err := json.Unmarshal([]byte(w.Payload), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Unclear) != 1 || p.Unclear[0].StepID != "xkb" {
		t.Errorf("payload headline set: %+v", p.Unclear)
	}
}

func TestWalkthroughAmendGuards(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	createWt(t, d, "w000000000010")

	_, rpcErr := d.Handle(context.Background(), proto.MethodWalkthroughAmend,
		mustJSON(t, map[string]any{"id": "w000000000010", "expect_rev": 1}))
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "not in this build") {
		t.Errorf("amend on an open review: %v", rpcErr)
	}

	if _, _, err := d.BoardSubmit("w000000000010"); err != nil {
		t.Fatal(err)
	}
	_, rpcErr = d.Handle(context.Background(), proto.MethodWalkthroughAmend,
		mustJSON(t, map[string]any{"id": "w000000000010", "expect_rev": 1}))
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "unread") {
		t.Errorf("amend on a submitted review must guard the handback: %v", rpcErr)
	}
}

func TestWalkthroughAbandonedWaiterRequeues(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	createWt(t, d, "w000000000011")
	if _, _, err := d.BoardSubmit("w000000000011"); err != nil {
		t.Fatal(err)
	}

	// A handoff that raced a departing waiter: the delivery already
	// happened, but nobody will read it. Dropping the waiter must push the
	// review back to submitted so the next taker finds it.
	payload, ok := d.takeSubmitted("w000000000011", "test")
	if !ok {
		t.Fatal("takeSubmitted must win on a parked submission")
	}
	w := &wtWaiter{out: make(chan wtHandoff, 1)}
	w.out <- wtHandoff{id: "w000000000011", payload: payload}
	d.dropWtWaiter(w)
	if s := wtState(t, d, "w000000000011"); s != store.WtSubmitted {
		t.Errorf("abandoned handoff must requeue, state = %s", s)
	}
}
