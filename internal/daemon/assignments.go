package daemon

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/assign"
	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/store"
)

// The assignment scheduler (M12 / FR82). It owns WHEN, and nothing else: what a
// run actually does is a Runner, because starting a Claude child belongs to the
// surface that can show it, and a clock does not need a child process to be
// tested against.
//
// One tick, one pass. Every enabled assignment carries next_run_ms in its row
// rather than in memory, so a daemon restart does not reset every interval and
// the panel can say when the next one is without parsing the grammar.

// tickEvery is how often the scheduler looks. A minute is the floor on an
// interval (assign.parseEvery), so looking twice that often is enough to be
// punctual and rare enough to cost nothing on a laptop.
const tickEvery = 30 * time.Second

// Runner carries an assignment out. The daemon holds one; webui implements it by
// spawning the work as an ordinary session, which is what makes a run something
// Boris can open, read and take over rather than a black box that mailed him a
// result.
type Runner interface {
	// RunAssignment starts the work and blocks until it is done. The returned
	// summary is what the panel shows for the run; data is whatever the run
	// recorded for later analysis, or empty.
	RunAssignment(req RunRequest) (summary, data string, err error)
}

// RunRequest is one execution, fully resolved: the prompt already has its
// parameters in it, so a Runner never needs to know what a parameter is.
type RunRequest struct {
	AssignmentID string
	RunID        string
	Name         string
	Prompt       string
	Model        string
	Mode         string
	Dir          string
	Trigger      string
	// OnSession is called with the session id as soon as there is one, rather
	// than returned at the end: a run that takes twenty minutes should be
	// openable for nineteen of them, not only once it is over.
	OnSession func(sessionID string)
}

type scheduler struct {
	store  *store.Store
	log    *slog.Logger
	runner Runner
	// changed, when set, is called after anything the surface shows moves: a
	// run starting, finishing, or being recorded as skipped. The daemon wires it
	// to the Presenter's AssignmentsChanged; nil (the tests') means nobody is
	// watching.
	changed func()

	mu      sync.Mutex
	running map[string]bool // assignment id -> a run is in flight
	stop    chan struct{}
}

func newScheduler(st *store.Store, log *slog.Logger) *scheduler {
	return &scheduler{store: st, log: log, running: map[string]bool{}}
}

func (s *scheduler) notify() {
	if s.changed != nil {
		s.changed()
	}
}

// SetRunner wires the surface that can carry an assignment out. Nil is valid and
// means every due assignment is skipped with a reason rather than silently not
// happening - a headless daemon has a schedule and nothing to run it with.
func (d *Daemon) SetRunner(r Runner) {
	if d.sched == nil {
		return
	}
	d.sched.mu.Lock()
	d.sched.runner = r
	d.sched.mu.Unlock()
}

// StartAssignments begins the schedule. Separate from New because the runner is
// wired after the daemon exists, and a tick that fires before it would record a
// pile of failures on the way up.
func (d *Daemon) StartAssignments() {
	if d.sched != nil {
		d.sched.Start()
	}
}

// StopAssignments ends the tick loop. A run already in flight is left to
// finish - it is a child process with a conversation in it, and killing it on
// the way down would lose the work without recording that it happened.
func (d *Daemon) StopAssignments() {
	if d.sched != nil {
		d.sched.Stop()
	}
}

// Assignments is the whole read side, for the surface and the MCP tools.
func (d *Daemon) Assignments() ([]*assign.Assignment, error) {
	return d.st.ListAssignments()
}

// Assignment reads one; a missing id is (nil, nil).
func (d *Daemon) Assignment(id string) (*assign.Assignment, error) {
	return d.st.GetAssignment(id)
}

// assignmentsChanged tells the surface something moved. Every mutation below
// ends with it, whoever asked - the editor, an MCP tool, the scheduler - so an
// open surface follows all of them the same way.
func (d *Daemon) assignmentsChanged() {
	d.ui.AssignmentsChanged()
}

// SaveAssignment writes a definition and re-places its next run, so a schedule
// edited in the panel takes effect without waiting for whatever the old one had
// queued.
func (d *Daemon) SaveAssignment(a *assign.Assignment) error {
	if err := d.st.SaveAssignment(a); err != nil {
		return err
	}
	if d.sched != nil && a.Enabled {
		d.sched.arm(a, time.Now())
	} else if d.sched != nil {
		d.sched.setSchedule(a, a.LastRunMS, 0)
	}
	d.assignmentsChanged()
	return nil
}

// DeleteAssignment removes one with its history.
func (d *Daemon) DeleteAssignment(id string) (bool, error) {
	gone, err := d.st.DeleteAssignment(id)
	if gone {
		d.assignmentsChanged()
	}
	return gone, err
}

// SetAssignmentEnabled pauses or resumes one, re-arming on the way back.
func (d *Daemon) SetAssignmentEnabled(id string, on bool) error {
	if err := d.st.SetAssignmentEnabled(id, on); err != nil {
		return err
	}
	defer d.assignmentsChanged()
	a, err := d.st.GetAssignment(id)
	if err != nil || a == nil || d.sched == nil {
		return err
	}
	if on {
		d.sched.arm(a, time.Now())
	} else {
		d.sched.setSchedule(a, a.LastRunMS, 0)
	}
	return nil
}

// SetAssignmentParams saves just the knob values, which is what the panel does.
func (d *Daemon) SetAssignmentParams(id string, params map[string]any) error {
	if err := d.st.SetAssignmentParams(id, params); err != nil {
		return err
	}
	d.assignmentsChanged()
	return nil
}

// RunAssignmentNow starts one outside its schedule. overrides apply to this run
// only.
func (d *Daemon) RunAssignmentNow(id, trigger string, overrides map[string]any) (string, error) {
	a, err := d.st.GetAssignment(id)
	if err != nil {
		return "", err
	}
	if a == nil {
		return "", fmt.Errorf("no assignment %q", id)
	}
	if d.sched == nil {
		return "", errors.New("this daemon has no scheduler")
	}
	return d.sched.launch(a, trigger, overrides)
}

// AssignmentRuns is the history of one assignment, newest first.
func (d *Daemon) AssignmentRuns(id string, limit int) ([]store.Run, error) {
	return d.st.RunsFor(id, limit)
}

// AssignmentRunning reports whether a run is in flight.
func (d *Daemon) AssignmentRunning(id string) bool {
	return d.sched != nil && d.sched.Running(id)
}

// StartScheduler begins the tick loop and reaps runs orphaned by the last
// shutdown. Safe to call once; the daemon's own teardown stops it.
func (s *scheduler) Start() {
	if n, err := s.store.ReapRunningRuns("the daemon stopped while this was running"); err != nil {
		s.log.Error(logging.EvStoreError, "component", "assignments", "op", "reap", "err", err.Error())
	} else if n > 0 {
		s.log.Info(logging.EvAssignmentRun, "component", "assignments", "reaped", n)
	}
	// Arm anything that has no next run yet: a schedule added while the daemon
	// was down, or an assignment created before the scheduler existed.
	s.armAll()

	s.mu.Lock()
	if s.stop != nil {
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	s.stop = stop
	s.mu.Unlock()

	go func() {
		t := time.NewTicker(tickEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				now := time.Now()
				safely(s.log, "assignments.tick", func() { s.tick(now) })()
			}
		}
	}()
}

func (s *scheduler) Stop() {
	s.mu.Lock()
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
	s.mu.Unlock()
}

// armAll gives every enabled, scheduled assignment a next_run_ms if it has none.
func (s *scheduler) armAll() {
	list, err := s.store.ListAssignments()
	if err != nil {
		s.log.Error(logging.EvStoreError, "component", "assignments", "op", "list", "err", err.Error())
		return
	}
	now := time.Now()
	for _, a := range list {
		if !a.Enabled || a.NextRunMS != 0 {
			continue
		}
		s.arm(a, now)
	}
}

// arm computes and stores the next due time. An unparseable schedule disarms the
// assignment rather than guessing at it: a typo must not become a run every
// tick, and the panel shows the same string back so the mistake is visible.
func (s *scheduler) arm(a *assign.Assignment, now time.Time) {
	sc, err := assign.ParseSchedule(a.Schedule)
	if err != nil {
		s.log.Warn(logging.EvAssignmentRun, "component", "assignments", "id", a.ID,
			"schedule", a.Schedule, "err", err.Error())
		s.setSchedule(a, a.LastRunMS, 0)
		return
	}
	if sc.AdHoc() {
		s.setSchedule(a, a.LastRunMS, 0)
		return
	}
	var last time.Time
	if a.LastRunMS > 0 {
		last = time.UnixMilli(a.LastRunMS)
	}
	next := sc.Next(now, last)
	if next.IsZero() {
		s.setSchedule(a, a.LastRunMS, 0)
		return
	}
	s.setSchedule(a, a.LastRunMS, next.UnixMilli())
}

// setSchedule writes the scheduler's two columns AND the copy in hand. Keeping
// them together is not tidiness: launch stamps the last run from the same struct
// a moment later, and when only the row was updated it wrote the stale next-run
// straight back over the one arm had just placed - so a due assignment stayed
// due and fired on every single tick.
func (s *scheduler) setSchedule(a *assign.Assignment, lastMS, nextMS int64) {
	a.LastRunMS, a.NextRunMS = lastMS, nextMS
	if err := s.store.SetAssignmentSchedule(a.ID, lastMS, nextMS); err != nil {
		s.log.Error(logging.EvStoreError, "component", "assignments", "op", "arm", "err", err.Error())
	}
}

// tick runs everything that is due. Missed slots are counted and recorded, never
// caught up (Boris, 2026-08-01): a laptop shut for the weekend must not wake and
// fire three usage checks at once, and a check for a window that has already
// passed is noise. What it leaves behind is a `skipped` row, so the panel can
// say "3 runs missed while off" instead of the runs quietly never happening.
func (s *scheduler) tick(now time.Time) {
	due, err := s.store.DueAssignments(now.UnixMilli())
	if err != nil {
		s.log.Error(logging.EvStoreError, "component", "assignments", "op", "due", "err", err.Error())
		return
	}
	for _, a := range due {
		sc, err := assign.ParseSchedule(a.Schedule)
		if err != nil {
			s.arm(a, now) // disarms and logs
			continue
		}
		if missed := sc.Missed(time.UnixMilli(a.NextRunMS), now); missed > 1 {
			s.recordSkipped(a, missed-1)
		}
		s.arm(a, now)
		if _, err := s.launch(a, "schedule", nil); err != nil {
			// Logged rather than retried: the two ways launch refuses are "already
			// running" (the previous run overran its own interval, and starting a
			// second is worse than skipping one) and "no runner", which the next
			// tick cannot fix either.
			s.log.Warn(logging.EvAssignmentRun, "component", "assignments", "id", a.ID,
				"state", "not started", "err", err.Error())
		}
	}
}

func (s *scheduler) recordSkipped(a *assign.Assignment, n int) {
	r := &store.Run{
		AssignmentID: a.ID, Trigger: "schedule", State: store.RunSkipped,
		Params:  a.Params,
		Summary: fmt.Sprintf("%d scheduled %s missed while agentbox was not running", n, plural(n, "run")),
	}
	if err := s.store.StartRun(r); err != nil {
		s.log.Error(logging.EvStoreError, "component", "assignments", "op", "skip", "err", err.Error())
		return
	}
	_ = s.store.FinishRun(r.ID, store.RunSkipped, r.Summary, "", "")
	s.log.Info(logging.EvAssignmentRun, "component", "assignments", "id", a.ID,
		"state", store.RunSkipped, "missed", n)
	s.notify()
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// Launch starts a run now. trigger is "manual" (a person pressed Run) or "agent"
// (an MCP call); overrides replace stored parameter values for this run only,
// which is what makes "try it with the threshold at 95" possible without editing
// the assignment.
func (s *scheduler) launch(a *assign.Assignment, trigger string, overrides map[string]any) (string, error) {
	s.mu.Lock()
	runner := s.runner
	if s.running[a.ID] {
		s.mu.Unlock()
		return "", errors.New("this assignment is already running")
	}
	s.running[a.ID] = true
	s.mu.Unlock()

	// mergeParams, not assign.Merge: an assignment with no declared spec still
	// substitutes {{key}} from its saved values - the save path keeps them for
	// exactly this moment, and assign.Merge would erase every one at the door.
	// Overrides ride over the merge untouched, so a run_assignment override is
	// never second-guessed by the spec.
	params := mergeParams(a.Spec, a.Params, nil)
	maps.Copy(params, overrides)
	prompt, missing := assign.Render(a.Prompt, params)

	r := &store.Run{AssignmentID: a.ID, Trigger: trigger, Params: params}
	if err := s.store.StartRun(r); err != nil {
		s.done(a.ID)
		return "", err
	}
	// An interval counts from the last run, so stamping it re-places the next
	// one: pressing Run at 14:00 on an "every 4h" assignment means 18:00, not a
	// run four hours after whenever the schedule last happened to land.
	s.setSchedule(a, time.Now().UnixMilli(), a.NextRunMS)
	s.arm(a, time.Now())
	if len(missing) > 0 {
		s.log.Warn(logging.EvAssignmentRun, "component", "assignments", "id", a.ID,
			"unfilled", strings.Join(missing, ","))
	}

	if runner == nil {
		msg := "no runner: agentbox has a schedule for this but nothing to carry it out"
		_ = s.store.FinishRun(r.ID, store.RunFailed, "", msg, "")
		s.done(a.ID)
		s.notify()
		return r.ID, errors.New(msg)
	}

	s.log.Info(logging.EvAssignmentRun, "component", "assignments", "id", a.ID,
		"run", r.ID, "trigger", trigger, "state", "started")
	s.notify() // the run row exists and the running flag is up: the surface can show both

	go func() {
		// done before notify: a poke that lands while the running flag is still
		// up would show the surface a finished run with a greyed Run button.
		defer func() { s.done(a.ID); s.notify() }()
		summary, data, err := runner.RunAssignment(RunRequest{
			AssignmentID: a.ID, RunID: r.ID, Name: a.Name, Prompt: prompt,
			Model: a.Model, Mode: a.Mode, Dir: a.Dir, Trigger: trigger,
			OnSession: func(sessionID string) {
				if err := s.store.SetRunSession(r.ID, sessionID); err != nil {
					s.log.Error(logging.EvStoreError, "component", "assignments", "op", "session", "err", err.Error())
				}
			},
		})
		state, errMsg := store.RunOK, ""
		if err != nil {
			state, errMsg = store.RunFailed, err.Error()
		}
		if err := s.store.FinishRun(r.ID, state, summary, errMsg, data); err != nil {
			s.log.Error(logging.EvStoreError, "component", "assignments", "op", "finish", "err", err.Error())
		}
		s.log.Info(logging.EvAssignmentRun, "component", "assignments", "id", a.ID,
			"run", r.ID, "state", state)
	}()
	return r.ID, nil
}

func (s *scheduler) done(assignmentID string) {
	s.mu.Lock()
	delete(s.running, assignmentID)
	s.mu.Unlock()
}

// Running reports whether an assignment has a run in flight, which is what the
// panel greys its Run button on.
func (s *scheduler) Running(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[id]
}
