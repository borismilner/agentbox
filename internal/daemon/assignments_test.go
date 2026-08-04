package daemon

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/assign"
	"github.com/borismilner/agentbox/internal/store"
)

// fakeRunner stands in for the thing that spawns a Claude child. The scheduler
// owns WHEN, and that is what these tests are about; a child process would only
// make them slow and flaky without testing anything the driver's own suite does
// not already cover.
type fakeRunner struct {
	mu      sync.Mutex
	got     []RunRequest
	summary string
	data    string
	err     error
	release chan struct{} // when non-nil, a run blocks until it is closed
}

func (f *fakeRunner) RunAssignment(req RunRequest) (string, string, error) {
	f.mu.Lock()
	f.got = append(f.got, req)
	rel := f.release
	f.mu.Unlock()
	if rel != nil {
		<-rel
	}
	return f.summary, f.data, f.err
}

func (f *fakeRunner) requests() []RunRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]RunRequest(nil), f.got...)
}

func newSched(t *testing.T) (*scheduler, *store.Store, *fakeRunner) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "agentbox.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s := newScheduler(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := &fakeRunner{summary: "done"}
	s.runner = r
	return s, st, r
}

func fixture(t *testing.T, st *store.Store, mut func(*assign.Assignment)) *assign.Assignment {
	t.Helper()
	a := &assign.Assignment{
		ID:       store.NewAssignmentID(),
		Name:     "Usage watch",
		Prompt:   "Check usage for {{window}}.",
		Spec:     []assign.Param{{Key: "window", Type: assign.TypeEnum, Values: []string{"24h", "7d"}, Default: "7d"}},
		Params:   map[string]any{"window": "7d"},
		Schedule: "every 1h",
		Enabled:  true,
	}
	if mut != nil {
		mut(a)
	}
	if err := st.SaveAssignment(a); err != nil {
		t.Fatalf("save: %v", err)
	}
	return a
}

func waitFinished(t *testing.T, st *store.Store, id string, want int) []store.Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := st.RunsFor(id, 0)
		if err != nil {
			t.Fatalf("runs: %v", err)
		}
		done := 0
		for _, r := range runs {
			if r.State != store.RunRunning {
				done++
			}
		}
		if done >= want {
			return runs
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d finished runs", want)
	return nil
}

// The prompt reaches the runner with its parameters already in it: a Runner
// never has to know what a parameter is.
func TestLaunchRendersThePromptBeforeItLeaves(t *testing.T) {
	s, st, r := newSched(t)
	a := fixture(t, st, func(a *assign.Assignment) { a.Params = map[string]any{"window": "24h"} })

	if _, err := s.launch(a, "manual", nil); err != nil {
		t.Fatalf("launch: %v", err)
	}
	waitFinished(t, st, a.ID, 1)

	got := r.requests()
	if len(got) != 1 {
		t.Fatalf("runner saw %d requests", len(got))
	}
	if got[0].Prompt != "Check usage for 24h." {
		t.Errorf("prompt = %q, want the substituted one", got[0].Prompt)
	}
	if got[0].Trigger != "manual" || got[0].Name != "Usage watch" {
		t.Errorf("request lost context: %+v", got[0])
	}
}

// An assignment whose knobs were never declared still runs with its saved
// values (a custom panel can set values no spec describes). The save path
// keeps them for exactly this; the launch must not erase them at the door.
func TestLaunchKeepsParamsWhenNoSpecDeclaresThem(t *testing.T) {
	s, st, r := newSched(t)
	a := fixture(t, st, func(a *assign.Assignment) {
		a.Spec = nil
		a.Params = map[string]any{"window": "24h"}
	})

	if _, err := s.launch(a, "manual", nil); err != nil {
		t.Fatalf("launch: %v", err)
	}
	waitFinished(t, st, a.ID, 1)

	if got := r.requests()[0].Prompt; got != "Check usage for 24h." {
		t.Errorf("prompt = %q; a no-spec assignment lost its saved values", got)
	}
}

// "Try it with the threshold at 95" without editing the assignment.
func TestOverridesApplyToTheRunAndNotTheDefinition(t *testing.T) {
	s, st, r := newSched(t)
	a := fixture(t, st, nil)

	if _, err := s.launch(a, "agent", map[string]any{"window": "30d"}); err != nil {
		t.Fatalf("launch: %v", err)
	}
	runs := waitFinished(t, st, a.ID, 1)

	if got := r.requests()[0].Prompt; got != "Check usage for 30d." {
		t.Errorf("prompt = %q, want the override", got)
	}
	if runs[0].Params["window"] != "30d" {
		t.Errorf("the run recorded %v, want what it actually used", runs[0].Params)
	}
	stored, _ := st.GetAssignment(a.ID)
	if stored.Params["window"] != "7d" {
		t.Errorf("an override edited the definition: %v", stored.Params)
	}
}

func TestAFailedRunKeepsItsReason(t *testing.T) {
	s, st, r := newSched(t)
	r.err = errors.New("claude exited 1")
	a := fixture(t, st, nil)

	if _, err := s.launch(a, "manual", nil); err != nil {
		t.Fatalf("launch: %v", err)
	}
	runs := waitFinished(t, st, a.ID, 1)
	if runs[0].State != store.RunFailed || runs[0].Error != "claude exited 1" {
		t.Fatalf("run = %+v, want failed with its reason", runs[0])
	}
}

// One run per assignment. The second refusal is not a queue: a check that
// overran its own interval must not be joined by another one on top of it.
func TestASecondLaunchIsRefusedWhileOneIsInFlight(t *testing.T) {
	s, st, r := newSched(t)
	r.release = make(chan struct{})
	a := fixture(t, st, nil)

	if _, err := s.launch(a, "manual", nil); err != nil {
		t.Fatalf("first launch: %v", err)
	}
	if !s.Running(a.ID) {
		t.Fatal("a run in flight is not reported as running")
	}
	if _, err := s.launch(a, "manual", nil); err == nil {
		t.Error("a second concurrent run was allowed")
	}
	close(r.release)
	waitFinished(t, st, a.ID, 1)

	deadline := time.Now().Add(time.Second)
	for s.Running(a.ID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s.Running(a.ID) {
		t.Fatal("the in-flight flag outlived the run, so the assignment can never run again")
	}
}

// A daemon with a schedule and nothing to carry it out records the fact rather
// than letting the run quietly not happen.
func TestNoRunnerFailsTheRunOutLoud(t *testing.T) {
	s, st, _ := newSched(t)
	s.runner = nil
	a := fixture(t, st, nil)

	if _, err := s.launch(a, "manual", nil); err == nil {
		t.Error("launch reported success with no runner")
	}
	runs, _ := st.RunsFor(a.ID, 0)
	if len(runs) != 1 || runs[0].State != store.RunFailed || runs[0].Error == "" {
		t.Fatalf("runs = %+v, want one failed run with a reason", runs)
	}
}

// The rule Boris set: a laptop shut for the weekend does not wake up and fire
// every slot it missed. It fires ONE, and records how many it did not.
func TestATickSkipsMissedSlotsAndRunsOnce(t *testing.T) {
	s, st, r := newSched(t)
	now := time.Now()
	a := fixture(t, st, func(a *assign.Assignment) {
		a.NextRunMS = now.Add(-3*time.Hour - 10*time.Minute).UnixMilli()
	})

	s.tick(now)
	runs := waitFinished(t, st, a.ID, 2)

	var ran, skipped int
	var skipSummary string
	for _, run := range runs {
		switch run.State {
		case store.RunSkipped:
			skipped++
			skipSummary = run.Summary
		case store.RunOK:
			ran++
		}
	}
	if ran != 1 {
		t.Errorf("%d runs actually fired, want exactly 1", ran)
	}
	if skipped != 1 {
		t.Fatalf("%d skip records, want 1", skipped)
	}
	if skipSummary == "" {
		t.Error("the skip says nothing, so nothing in the panel can report it")
	}
	if len(r.requests()) != 1 {
		t.Errorf("the runner was asked %d times", len(r.requests()))
	}

	// And the next slot is armed forward, not left in the past.
	after, _ := st.GetAssignment(a.ID)
	if after.NextRunMS <= now.UnixMilli() {
		t.Errorf("next run is still in the past (%d); it would fire again every tick", after.NextRunMS)
	}
}

// A schedule that came due exactly once records no skip - "1 missed" would be
// the run that is happening right now.
func TestAPunctualTickRecordsNoSkip(t *testing.T) {
	s, st, _ := newSched(t)
	now := time.Now()
	a := fixture(t, st, func(a *assign.Assignment) {
		a.NextRunMS = now.Add(-time.Second).UnixMilli()
	})

	s.tick(now)
	runs := waitFinished(t, st, a.ID, 1)
	for _, r := range runs {
		if r.State == store.RunSkipped {
			t.Fatalf("a punctual run reported a missed slot: %q", r.Summary)
		}
	}
}

// A typo in the schedule must not become a run every thirty seconds. It disarms,
// and the panel shows the string back so the mistake is visible.
func TestAnUnparseableScheduleDisarmsRatherThanGuessing(t *testing.T) {
	s, st, r := newSched(t)
	a := fixture(t, st, func(a *assign.Assignment) {
		a.Schedule = "every fortnight"
		a.NextRunMS = time.Now().Add(-time.Hour).UnixMilli()
	})

	s.tick(time.Now())

	after, _ := st.GetAssignment(a.ID)
	if after.NextRunMS != 0 {
		t.Errorf("next run = %d, want disarmed", after.NextRunMS)
	}
	if after.Schedule != "every fortnight" {
		t.Error("the bad schedule was rewritten, so the mistake became invisible")
	}
	if n := len(r.requests()); n != 0 {
		t.Errorf("an unparseable schedule ran %d times", n)
	}
}

func TestArmSkipsAdHocAndPlacesTheRest(t *testing.T) {
	s, st, _ := newSched(t)
	adhoc := fixture(t, st, func(a *assign.Assignment) { a.Schedule = "" })
	timed := fixture(t, st, func(a *assign.Assignment) { a.Schedule = "every 30m" })

	s.armAll()

	if got, _ := st.GetAssignment(adhoc.ID); got.NextRunMS != 0 {
		t.Errorf("an ad-hoc assignment was armed for %d", got.NextRunMS)
	}
	got, _ := st.GetAssignment(timed.ID)
	if got.NextRunMS == 0 {
		t.Fatal("a periodic assignment was left unarmed and would never run")
	}
	if want := time.Now().Add(30 * time.Minute).UnixMilli(); got.NextRunMS < want-5000 || got.NextRunMS > want+5000 {
		t.Errorf("armed for %d, want about %d", got.NextRunMS, want)
	}
}

// Creating something is not the same gesture as running it.
func TestArmingDoesNotFireImmediately(t *testing.T) {
	s, st, r := newSched(t)
	a := fixture(t, st, func(a *assign.Assignment) { a.NextRunMS = 0 })

	s.armAll()
	s.tick(time.Now())

	if n := len(r.requests()); n != 0 {
		t.Fatalf("creating an assignment ran it %d times", n)
	}
	if got, _ := st.GetAssignment(a.ID); got.NextRunMS <= time.Now().UnixMilli() {
		t.Error("armed in the past")
	}
}

func TestDisabledAssignmentsAreNeverDue(t *testing.T) {
	s, st, r := newSched(t)
	fixture(t, st, func(a *assign.Assignment) {
		a.Enabled = false
		a.NextRunMS = time.Now().Add(-time.Hour).UnixMilli()
	})

	s.tick(time.Now())
	if n := len(r.requests()); n != 0 {
		t.Fatalf("a disabled assignment ran %d times", n)
	}
}
