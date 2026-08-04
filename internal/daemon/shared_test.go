package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// posted is one signal the blackboard put on the hub, captured so a test can assert
// the wake happened without a whole daemon behind it.
type posted struct {
	topic string
	data  map[string]any
}

// newTestShared is the subsystem over a real store, for the reason the signal tests
// give: a fake store would let the CAS and the version drift apart, and the version
// is the whole contract. present is what the roster answers; the returned slice
// pointer collects the signals.
func newTestShared(t *testing.T, present func(string) bool) (*shared, *[]posted) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "agentbox.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	sh := newShared(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sh.SetStore(st)
	var got []posted
	if present == nil {
		present = func(string) bool { return true }
	}
	// Announced by default: the gate has its own test and every other one here is
	// about the values.
	sh.SetObservers(func(string) bool { return true }, present,
		func(topic string, _ proto.Identity, data any) {
			b, _ := json.Marshal(data)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			got = append(got, posted{topic: topic, data: m})
		})
	return sh, &got
}

func sharedSet(t *testing.T, sh *shared, key, value string, ifVersion *int64, own bool, agent, sess string) proto.SyncSharedResult {
	t.Helper()
	return sharedSetPID(t, sh, key, value, ifVersion, own, agent, sess, 0)
}

// sharedSetPID is the same write with an explicit owning process, for the tests
// about what happens when the roster cannot answer.
func sharedSetPID(t *testing.T, sh *shared, key, value string, ifVersion *int64, own bool, agent, sess string, pid int) proto.SyncSharedResult {
	t.Helper()
	res, rpcErr := sh.Handle(proto.SyncSharedParams{
		Identity: proto.Identity{Agent: agent, Key: sess}, Op: proto.SharedOpSet,
		Key: key, Value: json.RawMessage(value), IfVersion: ifVersion, Own: own, PID: pid,
	})
	if rpcErr != nil {
		t.Fatalf("set %s: %s", key, rpcErr.Message)
	}
	return res
}

func sharedGet(t *testing.T, sh *shared, key string) proto.SyncSharedResult {
	t.Helper()
	res, rpcErr := sh.Handle(proto.SyncSharedParams{
		Identity: proto.Identity{Agent: "claude", Key: "k1"}, Op: proto.SharedOpGet, Key: key})
	if rpcErr != nil {
		t.Fatalf("get %s: %s", key, rpcErr.Message)
	}
	return res
}

func vp(n int64) *int64 { return new(n) }

// The write gate is the announce door; the read is deliberately open. Visibility
// must not depend on good manners, but a claim the human cannot attribute to a
// purpose is a coordination he cannot follow when it stalls.
func TestSharedGatesWritesAndNotReads(t *testing.T) {
	sh, _ := newTestShared(t, nil)
	sh.SetObservers(func(string) bool { return false }, func(string) bool { return true }, nil)

	_, rpcErr := sh.Handle(proto.SyncSharedParams{
		Identity: proto.Identity{Agent: "claude", Key: "k1"}, Op: proto.SharedOpSet,
		Key: "claims/1", Value: json.RawMessage(`"x"`)})
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "announce first") {
		t.Fatalf("an unannounced set should be refused with a teaching error, got %+v", rpcErr)
	}
	_, rpcErr = sh.Handle(proto.SyncSharedParams{
		Identity: proto.Identity{Agent: "claude", Key: "k1"}, Op: proto.SharedOpDelete, Key: "claims/1"})
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "announce first") {
		t.Fatalf("an unannounced delete should be refused, got %+v", rpcErr)
	}
	if res, rpcErr := sh.Handle(proto.SyncSharedParams{
		Identity: proto.Identity{Agent: "claude", Key: "k1"}, Op: proto.SharedOpGet,
		Key: "claims/1"}); rpcErr != nil || !res.OK {
		t.Fatalf("a read must never be gated: res %+v, err %+v", res, rpcErr)
	}
}

// A missing key answers with version 0 rather than only "not found", because 0 is
// the if_version that claims it - saying so here is what saves the round trip.
func TestSharedGetOfAMissingKeyReportsVersionZero(t *testing.T) {
	sh, _ := newTestShared(t, nil)
	res := sharedGet(t, sh, "claims/nope")
	if res.Found || res.Value == nil || res.Value.Version != 0 {
		t.Fatalf("missing key: %+v", res)
	}
	if !strings.Contains(res.Note, "if_version 0") {
		t.Fatalf("the note should name the if_version that claims it, got %q", res.Note)
	}
}

// The heart of slice 4: a claim whose session is gone reads as orphaned, so a table
// is drainable after a death instead of stuck on a chunk nobody is working on. The
// owner's AGENT name is what makes the report nameable, and it is only nameable
// because it was recorded at write time - the roster row is long gone.
func TestSharedReportsAClaimWhoseOwnerIsGone(t *testing.T) {
	live := map[string]bool{"alive": true}
	sh, _ := newTestShared(t, func(key string) bool { return live[key] })

	sharedSet(t, sh, "claims/1", `"working"`, vp(0), true, "codex", "dead")
	sharedSet(t, sh, "claims/2", `"working"`, vp(0), true, "claude", "alive")

	gone := sharedGet(t, sh, "claims/1")
	if gone.Value == nil || !gone.Value.OwnerGone {
		t.Fatalf("a claim by a session that is not on the roster should read as orphaned: %+v", gone.Value)
	}
	if gone.Value.OwnerAgent != "codex" {
		t.Fatalf("the orphan should still name its agent, got %q", gone.Value.OwnerAgent)
	}
	if !strings.Contains(gone.Note, "no longer running") || !strings.Contains(gone.Note, "if_version 1") {
		t.Fatalf("the note should say it is abandoned and how to take it over, got %q", gone.Note)
	}
	if held := sharedGet(t, sh, "claims/2"); held.Value == nil || held.Value.OwnerGone {
		t.Fatalf("a claim by a live session must not read as orphaned: %+v", held.Value)
	}

	// And the family read counts them, which is how a drainer notices without
	// inspecting every row.
	table := sharedGet(t, sh, "claims/*")
	if len(table.Values) != 2 {
		t.Fatalf("prefix read returned %d values, want 2", len(table.Values))
	}
	if countGone(table.Values) != 1 || !strings.Contains(table.Note, "owner_gone") {
		t.Fatalf("the family note should point at the orphan, got %q", table.Note)
	}
}

// A losing claimer must be told what beat it AND whether that thing is still alive,
// because the two cases call for opposite next moves: move on, or take it over.
func TestSharedRefusalTellsTheLoserWhatToDoNext(t *testing.T) {
	live := map[string]bool{"alive": true}
	sh, _ := newTestShared(t, func(key string) bool { return live[key] })

	sharedSet(t, sh, "claims/1", `"first"`, vp(0), true, "claude", "alive")
	lost := sharedSet(t, sh, "claims/1", `"second"`, vp(0), true, "codex", "other")
	if lost.Applied || !lost.Stale {
		t.Fatalf("the second claim should be stale, got %+v", lost)
	}
	if !strings.Contains(lost.Note, "still running") {
		t.Fatalf("losing to a live claimer should say move on, got %q", lost.Note)
	}

	sharedSet(t, sh, "claims/2", `"first"`, vp(0), true, "codex", "dead")
	lost = sharedSet(t, sh, "claims/2", `"second"`, vp(0), true, "claude", "alive")
	if lost.Applied || !lost.Stale {
		t.Fatalf("claiming a held key should be stale, got %+v", lost)
	}
	if !strings.Contains(lost.Note, "abandoned") || !strings.Contains(lost.Note, "if_version 1") {
		t.Fatalf("losing to a DEAD claimer should say take it over, got %q", lost.Note)
	}
}

// Every write posts shared:<key> through the one hub, and a delete posts too: a
// waiter that only heard about writes would park forever on a key whose work
// finished and whose row was removed.
func TestSharedPostsASignalForEveryChange(t *testing.T) {
	sh, got := newTestShared(t, nil)

	sharedSet(t, sh, "claims/3", `"mine"`, vp(0), true, "claude", "k1")
	if len(*got) != 1 {
		t.Fatalf("a set should post one signal, got %d", len(*got))
	}
	first := (*got)[0]
	if first.topic != "shared:claims/3" {
		t.Fatalf("topic came out as %q", first.topic)
	}
	if first.data["version"] != float64(1) || first.data["owner"] != "k1" {
		t.Fatalf("the signal should carry the key, version and owner, got %+v", first.data)
	}
	// Never the value: the value cap and the signal cap are the same number, so a
	// maximum-size value plus an envelope would be over the limit and the wake would
	// be dropped rather than delivered small.
	if _, carried := first.data["value"]; carried {
		t.Fatalf("the signal is a doorbell and must not carry the value: %+v", first.data)
	}

	res, rpcErr := sh.Handle(proto.SyncSharedParams{
		Identity: proto.Identity{Agent: "claude", Key: "k1"}, Op: proto.SharedOpDelete, Key: "claims/3"})
	if rpcErr != nil || !res.Applied {
		t.Fatalf("delete: res %+v, err %+v", res, rpcErr)
	}
	if len(*got) != 2 || (*got)[1].data["deleted"] != true {
		t.Fatalf("a delete should post a signal saying so, got %+v", *got)
	}

	// A refused write must NOT wake anybody: nothing changed, and a wake with nothing
	// behind it teaches a waiter to distrust the doorbell.
	sharedSet(t, sh, "claims/4", `"a"`, vp(0), false, "claude", "k1")
	before := len(*got)
	sharedSet(t, sh, "claims/4", `"b"`, vp(0), false, "codex", "k2")
	if len(*got) != before {
		t.Fatalf("a stale write posted a signal: %+v", (*got)[before:])
	}
}

func TestSharedRefusesTheShapesThatCannotMeanAnything(t *testing.T) {
	sh, _ := newTestShared(t, nil)
	id := proto.Identity{Agent: "claude", Key: "k1"}

	// A wildcard in a stored key would be unreachable by an exact get and matched by
	// every prefix get by accident - the rule post_signal enforces on topics.
	if _, rpcErr := sh.Handle(proto.SyncSharedParams{Identity: id, Op: proto.SharedOpSet,
		Key: "claims/*", Value: json.RawMessage(`"x"`)}); rpcErr == nil ||
		!strings.Contains(rpcErr.Message, `cannot contain "*"`) {
		t.Fatalf("a key with a wildcard should be refused, got %+v", rpcErr)
	}
	// "Delete it only if it does not exist" is not a thing to mean, and reading it as
	// unconditional would delete a key the caller meant to protect.
	if _, rpcErr := sh.Handle(proto.SyncSharedParams{Identity: id, Op: proto.SharedOpDelete,
		Key: "claims/1", IfVersion: vp(0)}); rpcErr == nil ||
		!strings.Contains(rpcErr.Message, "cannot be a delete") {
		t.Fatalf("delete with if_version 0 should be refused, got %+v", rpcErr)
	}
	// A set with no value: the way to remove a key is delete, and a null value would
	// leave it claimed.
	if _, rpcErr := sh.Handle(proto.SyncSharedParams{Identity: id, Op: proto.SharedOpSet,
		Key: "claims/1"}); rpcErr == nil || !strings.Contains(rpcErr.Message, "needs a value") {
		t.Fatalf("a set with no value should be refused, got %+v", rpcErr)
	}
	if _, rpcErr := sh.Handle(proto.SyncSharedParams{Identity: id, Op: "increment",
		Key: "claims/1"}); rpcErr == nil || !strings.Contains(rpcErr.Message, "get, set, delete") {
		t.Fatalf("an unknown op should name the three, got %+v", rpcErr)
	}
	if _, rpcErr := sh.Handle(proto.SyncSharedParams{Identity: id, Op: proto.SharedOpGet,
		Key: "  "}); rpcErr == nil || !strings.Contains(rpcErr.Message, "needs a key") {
		t.Fatalf("a blank key should be refused, got %+v", rpcErr)
	}
}

func TestSharedCapsTheValue(t *testing.T) {
	sh, _ := newTestShared(t, nil)
	sh.SetMaxBytes(64)
	big := `"` + strings.Repeat("x", 128) + `"`
	_, rpcErr := sh.Handle(proto.SyncSharedParams{
		Identity: proto.Identity{Agent: "claude", Key: "k1"}, Op: proto.SharedOpSet,
		Key: "big", Value: json.RawMessage(big)})
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "over the 64-byte cap") {
		t.Fatalf("an oversize value should be refused naming the cap, got %+v", rpcErr)
	}
	if !strings.Contains(rpcErr.Message, "file") {
		t.Fatalf("the refusal should point at the idiom for anything bigger, got %q", rpcErr.Message)
	}
}

// A daemon with no database refuses rather than keeping a map. Durability is the
// feature here more than anywhere: the acceptance case is literally "restart the
// daemon mid-drain and the claims survive", and an in-memory fallback would pass
// every test and lose a claim table on the first restart.
func TestSharedRefusesWithoutAStore(t *testing.T) {
	sh := newShared(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, rpcErr := sh.Handle(proto.SyncSharedParams{
		Identity: proto.Identity{Agent: "claude", Key: "k1"}, Op: proto.SharedOpGet, Key: "k"})
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "no database") {
		t.Fatalf("want a refusal naming the missing database, got %+v", rpcErr)
	}
}

// The fan-out the design describes, end to end at this layer: three workers, ten
// keys, every key claimed exactly once and the whole table drained.
func TestSharedDrainsAClaimTableWithoutDoubles(t *testing.T) {
	sh, _ := newTestShared(t, nil)
	const chunks = 10
	claimed := map[string]string{}

	for _, worker := range []string{"w1", "w2", "w3"} {
		for c := range chunks {
			key := fmt.Sprintf("claims/%d", c)
			res := sharedSet(t, sh, key, `"`+worker+`"`, vp(0), true, "claude", worker)
			if res.Applied {
				if had, seen := claimed[key]; seen {
					t.Fatalf("%s was claimed by %s and then by %s", key, had, worker)
				}
				claimed[key] = worker
			}
		}
	}
	if len(claimed) != chunks {
		t.Fatalf("%d of %d chunks were claimed", len(claimed), chunks)
	}
	// Every winner finishes and removes its key, which is what empties the table.
	for key := range claimed {
		res, rpcErr := sh.Handle(proto.SyncSharedParams{
			Identity: proto.Identity{Agent: "claude", Key: claimed[key]},
			Op:       proto.SharedOpDelete, Key: key})
		if rpcErr != nil || !res.Applied {
			t.Fatalf("finishing %s: res %+v, err %+v", key, res, rpcErr)
		}
	}
	if left := sharedGet(t, sh, "claims/*"); len(left.Values) != 0 {
		t.Fatalf("the table did not drain: %+v", left.Values)
	}
}

// The two-step ownership check, and the case that made it necessary: a daemon
// restart empties the roster, so for the second it takes every child to redial,
// asking the roster alone reports LIVE work as abandoned - an invitation to take
// over a chunk somebody is writing, which is the failure this whole feature exists
// to prevent. The pid is the only fact about an owner that outlives the daemon.
func TestSharedOwnershipFallsBackToTheProcess(t *testing.T) {
	// Nobody is on the roster: exactly the state right after a restart.
	sh, _ := newTestShared(t, func(string) bool { return false })
	sh.alive = func(pid int) bool { return pid == 4242 }

	sharedSetPID(t, sh, "claims/live", `"working"`, vp(0), true, "claude", "gone-from-roster", 4242)
	sharedSetPID(t, sh, "claims/dead", `"working"`, vp(0), true, "codex", "really-dead", 5150)
	// No pid recorded, which is the CLI's honest zero: the roster is all there is.
	sharedSetPID(t, sh, "claims/nopid", `"working"`, vp(0), true, "aider", "cli-written", 0)

	live := sharedGet(t, sh, "claims/live")
	if live.Value.OwnerGone {
		t.Fatal("a claim whose process is still running read as abandoned; this is the restart window that made the pid necessary")
	}
	if dead := sharedGet(t, sh, "claims/dead"); !dead.Value.OwnerGone {
		t.Fatal("a claim whose process is gone should read as abandoned")
	}
	if nopid := sharedGet(t, sh, "claims/nopid"); !nopid.Value.OwnerGone {
		t.Fatal("with no pid and no roster row there is nothing left to say the owner lives")
	}

	// And the roster still wins when it CAN answer: a session that is attached is
	// doing the work whatever a recorded pid looks like.
	sh.SetObservers(func(string) bool { return true }, func(string) bool { return true }, nil)
	sh.alive = func(int) bool { return false }
	if again := sharedGet(t, sh, "claims/dead"); again.Value.OwnerGone {
		t.Fatal("an attached session's claim must not read as abandoned because of a stale pid")
	}
}

// The pid has to survive the round trip through the store, or the fallback above is
// reading a zero and calling every restart-window claim dead.
func TestSharedRecordsTheOwningProcess(t *testing.T) {
	sh, _ := newTestShared(t, nil)
	res := sharedSetPID(t, sh, "claims/1", `"x"`, vp(0), true, "claude", "k1", 4242)
	if res.Value.OwnerPID != 4242 {
		t.Fatalf("the write did not record the owning process: %+v", res.Value)
	}
	if got := sharedGet(t, sh, "claims/1"); got.Value.OwnerPID != 4242 {
		t.Fatalf("the pid did not survive a read: %+v", got.Value)
	}
	// An unowned write records none: a counter has no owner and must not inherit the
	// caller's process.
	res = sharedSetPID(t, sh, "progress:x", `1`, nil, false, "claude", "k1", 4242)
	if res.Value.OwnerPID != 0 || res.Value.Owner != "" {
		t.Fatalf("an unowned write should record no owner at all: %+v", res.Value)
	}
}
