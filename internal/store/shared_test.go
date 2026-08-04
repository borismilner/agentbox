package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func newSharedStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "agentbox.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func ver(n int64) *int64 { return new(n) }

func TestSharedSetVersionsFromOne(t *testing.T) {
	st := newSharedStore(t)
	// One is where versions start, which is what leaves zero free to mean "does not
	// exist" - the whole claim idiom rests on it.
	v, applied, err := st.SharedSet("progress:migration", `{"done":0}`, nil, "", "")
	if err != nil || !applied || v.Version != 1 {
		t.Fatalf("first write: version %d, applied %v, err %v; want version 1", v.Version, applied, err)
	}
	v, applied, err = st.SharedSet("progress:migration", `{"done":1}`, nil, "", "")
	if err != nil || !applied || v.Version != 2 {
		t.Fatalf("second write: version %d, applied %v, err %v; want version 2", v.Version, applied, err)
	}
	if string(v.Value) != `{"done":1}` {
		t.Fatalf("value came back as %q", v.Value)
	}
}

func TestSharedClaimFromEmptyHasExactlyOneWinner(t *testing.T) {
	st := newSharedStore(t)
	first, applied, err := st.SharedSet("claims/3", `"a"`, ver(0), "ka", "claude")
	if err != nil || !applied || first.Version != 1 {
		t.Fatalf("claim: %+v applied %v err %v", first, applied, err)
	}
	// The second claimer must lose AND be told what beat it, or its retry costs a
	// second call to find out.
	cur, applied, err := st.SharedSet("claims/3", `"b"`, ver(0), "kb", "codex")
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if applied {
		t.Fatal("two agents both claimed one key")
	}
	if cur.Version != 1 || cur.Owner != "ka" || cur.OwnerAgent != "claude" || string(cur.Value) != `"a"` {
		t.Fatalf("refusal did not carry the winner: %+v", cur)
	}
}

func TestSharedCASNeedsTheVersionItAsksFor(t *testing.T) {
	st := newSharedStore(t)
	if _, _, err := st.SharedSet("k", `1`, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, applied, err := st.SharedSet("k", `2`, ver(1), "", ""); err != nil || !applied {
		t.Fatalf("write at the current version: applied %v, err %v", applied, err)
	}
	// Version 1 is now stale, and a stale write must not land.
	cur, applied, err := st.SharedSet("k", `3`, ver(1), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("a write against a stale version landed")
	}
	if cur.Version != 2 || string(cur.Value) != `2` {
		t.Fatalf("refusal should carry the current row, got %+v", cur)
	}
}

// A version for a key that does not exist has to be refused rather than created:
// an upsert here would hand version 1 to a caller who asked about version 3, which
// is a lost update dressed as a success.
func TestSharedCASOnAMissingKeyIsRefusedNotCreated(t *testing.T) {
	st := newSharedStore(t)
	cur, applied, err := st.SharedSet("nothing/here", `1`, ver(3), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("a CAS against a missing key created it")
	}
	// Version 0 is the answer, and it is also the if_version that would claim it.
	if cur.Version != 0 || cur.Key != "nothing/here" {
		t.Fatalf("refusal should report version 0 for a missing key, got %+v", cur)
	}
	if _, found, err := st.SharedGet("nothing/here"); err != nil || found {
		t.Fatalf("the key should not exist: found %v, err %v", found, err)
	}
}

func TestSharedDeleteRespectsTheVersion(t *testing.T) {
	st := newSharedStore(t)
	if _, _, err := st.SharedSet("claims/1", `"mine"`, ver(0), "ka", "claude"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.SharedSet("claims/1", `"theirs"`, ver(1), "kb", "codex"); err != nil {
		t.Fatal(err)
	}
	// Deleting at a version somebody has written past must fail: the work being
	// declared finished may now be another agent's.
	if _, applied, err := st.SharedDelete("claims/1", ver(1)); err != nil || applied {
		t.Fatalf("stale delete: applied %v, err %v; want refused", applied, err)
	}
	if _, applied, err := st.SharedDelete("claims/1", ver(2)); err != nil || !applied {
		t.Fatalf("delete at the current version: applied %v, err %v", applied, err)
	}
	if _, found, err := st.SharedGet("claims/1"); err != nil || found {
		t.Fatalf("key survived its delete: found %v, err %v", found, err)
	}
	// And a delete of nothing is refused rather than reported as a success, so a
	// finisher learns somebody removed its claim.
	if _, applied, err := st.SharedDelete("claims/1", nil); err != nil || applied {
		t.Fatalf("deleting a missing key: applied %v, err %v; want refused", applied, err)
	}
}

func TestSharedListReadsAFamilyAndSaysWhenCapped(t *testing.T) {
	st := newSharedStore(t)
	for i := range 5 {
		if _, _, err := st.SharedSet(fmt.Sprintf("claims/%d", i), `"x"`, ver(0), "ka", "claude"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := st.SharedSet("progress:other", `1`, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	got, more, err := st.SharedList("claims/", 0)
	if err != nil || more || len(got) != 5 {
		t.Fatalf("prefix read: %d keys, more %v, err %v; want 5", len(got), more, err)
	}
	// Key order, so a claim table reads the same way twice.
	if got[0].Key != "claims/0" || got[4].Key != "claims/4" {
		t.Fatalf("not in key order: %s .. %s", got[0].Key, got[4].Key)
	}
	got, more, err = st.SharedList("claims/", 2)
	if err != nil || !more || len(got) != 2 {
		t.Fatalf("capped read: %d keys, more %v, err %v; want 2 and more", len(got), more, err)
	}
}

// A key holding a LIKE metacharacter must not become a wildcard nobody wrote. The
// same escaping the topic predicate needed, and it is worth its own test because
// the failure is silent: an unescaped _ matches any character, so one claim read
// would quietly return a neighbour's.
func TestSharedListEscapesLikeMetacharacters(t *testing.T) {
	st := newSharedStore(t)
	for _, k := range []string{"a_b/1", "axb/1", "a%c/1", "azc/1"} {
		if _, _, err := st.SharedSet(k, `"x"`, nil, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	got, _, err := st.SharedList("a_b/", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "a_b/1" {
		t.Fatalf("_ was treated as a wildcard: got %+v", got)
	}
	got, _, err = st.SharedList("a%c/", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "a%c/1" {
		t.Fatalf("%% was treated as a wildcard: got %+v", got)
	}
}

// The cap refuses a NEW key and never blocks an update, because refusing an update
// would strand a claim its owner is trying to finish - and evicting anything would
// hand a chunk to two agents, which is why this cap fails loudly instead.
func TestSharedCapRefusesNewKeysAndAllowsUpdates(t *testing.T) {
	st := newSharedStore(t)
	for i := range SharedKeyMax {
		if _, _, err := st.SharedSet(fmt.Sprintf("k/%d", i), `1`, nil, "", ""); err != nil {
			t.Fatalf("filling at %d: %v", i, err)
		}
	}
	if _, _, err := st.SharedSet("k/new", `1`, nil, "", ""); !errors.Is(err, ErrSharedFull) {
		t.Fatalf("a new key at the cap: err %v; want ErrSharedFull", err)
	}
	if _, _, err := st.SharedSet("k/new", `1`, ver(0), "", ""); !errors.Is(err, ErrSharedFull) {
		t.Fatalf("a new claim at the cap: err %v; want ErrSharedFull", err)
	}
	if _, applied, err := st.SharedSet("k/7", `2`, nil, "", ""); err != nil || !applied {
		t.Fatalf("updating an existing key at the cap: applied %v, err %v", applied, err)
	}
	// And deleting one makes room again, which is what the refusal tells the caller
	// to do.
	if _, applied, err := st.SharedDelete("k/7", nil); err != nil || !applied {
		t.Fatal(err)
	}
	if _, applied, err := st.SharedSet("k/new", `1`, nil, "", ""); err != nil || !applied {
		t.Fatalf("after making room: applied %v, err %v", applied, err)
	}
}

// The acceptance property at its own layer, and the reason every CAS here is ONE
// statement: with -race and real concurrency, each key must have exactly one winner.
// A read-then-write version of this would pass single-threaded and lose an update
// here.
func TestSharedClaimsAreAtomicUnderConcurrency(t *testing.T) {
	st := newSharedStore(t)
	const keys, workers = 10, 8

	var mu sync.Mutex
	wins := map[string]int{}
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for k := range keys {
				key := fmt.Sprintf("claims/%d", k)
				_, applied, err := st.SharedSet(key, fmt.Sprintf(`"w%d"`, w), ver(0),
					fmt.Sprintf("k%d", w), "claude")
				if err != nil {
					t.Errorf("worker %d on %s: %v", w, key, err)
					return
				}
				if applied {
					mu.Lock()
					wins[key]++
					mu.Unlock()
				}
			}
		}(w)
	}
	wg.Wait()

	if len(wins) != keys {
		t.Fatalf("%d of %d keys were claimed; every key should have a winner", len(wins), keys)
	}
	for key, n := range wins {
		if n != 1 {
			t.Errorf("%s was claimed %d times; a claim must have exactly one winner", key, n)
		}
	}
}
