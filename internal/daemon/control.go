package daemon

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
)

// control is the desktop handover (FR74): one strip on screen for the length of a
// run, which asks for the desktop and then says what is being done with it.
//
// It exists because the announcements were events. `veto` said "I am about to
// take the mouse" once and closed; `notify` said "mouse is yours again" once.
// Between the two there was nothing on screen, so the only way to find out
// whether an agent was still driving was to touch something - which is exactly
// what breaks the run. Three drive sequences died that way in ten minutes before
// this was written.
//
// The strip's presence is the whole signal: on screen means the desktop is the
// agent's, gone means it is the human's. That is why there is no idle state and
// no "working but you may touch things" state - either would make presence
// ambiguous, and presence is the one thing this element cannot afford to blur.
//
// One run at a time, because the desktop is one resource and several agents reach
// this daemon. A second requester is refused with the holder's name rather than
// queued: see proto.ControlResult for why a queue would be worse than a refusal.
type control struct {
	log  *slog.Logger
	show func(*ControlState) // paint or move the strip; nil in tests
	hide func()              // take it off the screen

	mu  sync.Mutex
	run *controlRun
}

// controlRun is one live handover. denied is closed by the human pressing Deny,
// which is what releases the blocked request; grant is closed when the countdown
// elapses or the human allows it early.
type controlRun struct {
	id       string
	identity proto.Identity
	state    string
	reason   string
	activity string
	since    time.Time // when activity last changed, so a stuck run is visible
	deadline time.Time // when asking becomes driving

	denied  chan struct{}
	granted chan struct{}
	once    sync.Once // whichever of the two fires first wins
}

// ControlState is what the strip paints. Age is derived here rather than in the
// surface so a strip that reopens mid-run does not restart the clock.
type ControlState struct {
	ID       string         `json:"id"`
	State    string         `json:"state"`
	Reason   string         `json:"reason,omitempty"`
	Activity string         `json:"activity,omitempty"`
	Identity proto.Identity `json:"identity"`
	SinceMs  int64          `json:"since_ms"`            // ms since the activity last changed
	LeftMs   int64          `json:"left_ms,omitempty"`   // ms left on the countdown, while asking
	WindowMs int64          `json:"window_ms,omitempty"` // the countdown's full length
}

// Handover exposes the two answers the human can give, and nothing else, so the
// surface cannot reach the rest of the daemon on the way (webui.Handover).
func (d *Daemon) Handover() *control { return d.control }

// SetControlSurface wires the strip's window to the run state. Split from the
// constructor because the UI is built after the daemon.
func (d *Daemon) SetControlSurface(show func(*ControlState), hide func()) {
	d.control.SetSurface(show, hide)
}

func newControl(log *slog.Logger) *control {
	return &control{log: log}
}

// SetSurface wires the window. Kept separate from the constructor because the UI
// is built after the daemon, and because a headless build has no strip to paint -
// the state is still correct there, which is what the tests check.
func (c *control) SetSurface(show func(*ControlState), hide func()) {
	c.mu.Lock()
	c.show, c.hide = show, hide
	c.mu.Unlock()
}

// Request asks for the desktop and BLOCKS until the human grants it, denies it,
// or the countdown runs out. Silence is consent, which is the shape that fits "I
// am about to do the obvious thing" - the human only has to act to stop it.
//
// ctx is the caller's connection. If it dies while we are asking, the run dies
// with it: a strip that outlives its agent would claim hands-off for a process
// that is gone, and the human would wait on nothing.
func (c *control) Request(ctx context.Context, id proto.Identity, reason string, window time.Duration) proto.ControlResult {
	if window <= 0 {
		window = 20 * time.Second
	}
	now := time.Now()

	c.mu.Lock()
	if c.run != nil {
		held := c.run.identity.Agent
		heldReason := c.run.reason
		c.mu.Unlock()
		c.log.Info(logging.EvControl, "component", "daemon", "control", "refused",
			"agent", id.Agent, "held_by", held)
		return proto.ControlResult{OK: true, Live: true, HeldBy: held, Reason: heldReason}
	}
	run := &controlRun{
		id:       "k" + newID()[1:13],
		identity: id,
		state:    proto.ControlAsking,
		reason:   reason,
		since:    now,
		deadline: now.Add(window),
		denied:   make(chan struct{}),
		granted:  make(chan struct{}),
	}
	c.run = run
	show := c.show
	c.mu.Unlock()

	c.log.Info(logging.EvControl, "component", "daemon", "control", "asking",
		"agent", id.Agent, "reason", reason, "window_s", int(window.Seconds()))
	if show != nil {
		show(c.snapshot(run, window))
	}

	timer := time.NewTimer(window)
	defer timer.Stop()
	select {
	case <-run.denied:
		c.end(run, "denied")
		return proto.ControlResult{OK: true, Denied: true}
	case <-run.granted:
	case <-timer.C:
		run.once.Do(func() { close(run.granted) })
	case <-ctx.Done():
		// The agent went away mid-question. Nothing to hand over, and the strip
		// must not stay up asking on behalf of a process that is gone.
		c.end(run, "caller gone")
		return proto.ControlResult{OK: true}
	}

	c.mu.Lock()
	if c.run != run {
		c.mu.Unlock() // superseded while we waited
		return proto.ControlResult{OK: true}
	}
	run.state = proto.ControlDriving
	run.since = time.Now()
	show = c.show
	c.mu.Unlock()

	c.log.Info(logging.EvControl, "component", "daemon", "control", "driving", "agent", id.Agent)
	if show != nil {
		show(c.snapshot(run, 0))
	}
	return proto.ControlResult{OK: true, Live: true, Granted: true, State: proto.ControlDriving}
}

// Activity updates the line the strip shows, and resets its age. Non-blocking:
// an agent narrating its work must never be held up by the narration.
//
// Only the agent holding the run may write to it, so a second agent's stray
// update cannot put words in the driver's mouth.
func (c *control) Activity(id proto.Identity, activity string) proto.ControlResult {
	c.mu.Lock()
	run := c.run
	if run == nil {
		c.mu.Unlock()
		return proto.ControlResult{OK: true}
	}
	if run.identity.Agent != id.Agent {
		held := run.identity.Agent
		c.mu.Unlock()
		return proto.ControlResult{OK: true, Live: true, HeldBy: held}
	}
	if activity != "" && activity != run.activity {
		run.activity = activity
		run.since = time.Now()
	}
	show := c.show
	state, act := run.state, run.activity
	c.mu.Unlock()

	if show != nil {
		show(c.snapshot(run, 0))
	}
	return proto.ControlResult{OK: true, Live: true, Granted: true, State: state, Activity: act}
}

// Release ends the run and takes the strip off the screen, which is how the human
// learns the desktop is his again.
func (c *control) Release(id proto.Identity) proto.ControlResult {
	c.mu.Lock()
	run := c.run
	if run == nil {
		c.mu.Unlock()
		return proto.ControlResult{OK: true}
	}
	if run.identity.Agent != id.Agent {
		held := run.identity.Agent
		c.mu.Unlock()
		return proto.ControlResult{OK: true, Live: true, HeldBy: held}
	}
	c.mu.Unlock()
	c.end(run, "released")
	return proto.ControlResult{OK: true}
}

// State reports without changing anything, for a surface that just opened and for
// a caller checking whether it still holds the desktop.
func (c *control) State() proto.ControlResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.run == nil {
		return proto.ControlResult{OK: true}
	}
	return proto.ControlResult{
		OK: true, Live: true, State: c.run.state,
		Activity: c.run.activity, HeldBy: c.run.identity.Agent, Reason: c.run.reason,
	}
}

// Deny is the human pressing the button on the strip. It releases the blocked
// request, which is what makes the strip the place the answer happens rather than
// a card beside it.
func (c *control) Deny(id string) {
	c.mu.Lock()
	run := c.run
	c.mu.Unlock()
	if run == nil || (id != "" && run.id != id) {
		return
	}
	run.once.Do(func() { close(run.denied) })
}

// Allow is the human granting it early rather than waiting out the countdown.
func (c *control) Allow(id string) {
	c.mu.Lock()
	run := c.run
	c.mu.Unlock()
	if run == nil || (id != "" && run.id != id) {
		return
	}
	run.once.Do(func() { close(run.granted) })
}

// Snapshot is what the surface asks for when its window opens mid-run.
func (c *control) Snapshot() *ControlState {
	c.mu.Lock()
	run := c.run
	c.mu.Unlock()
	if run == nil {
		return nil
	}
	return c.snapshot(run, 0)
}

// end clears the run and hides the strip, once, whoever got here first.
func (c *control) end(run *controlRun, why string) {
	c.mu.Lock()
	if c.run != run {
		c.mu.Unlock()
		return
	}
	c.run = nil
	hide := c.hide
	c.mu.Unlock()

	c.log.Info(logging.EvControl, "component", "daemon", "control", "ended",
		"agent", run.identity.Agent, "why", why)
	if hide != nil {
		hide()
	}
}

// snapshot reads the run under the lock the caller may or may not hold, so it
// takes its own copy of everything the surface needs. window is only passed while
// asking, where the countdown's full length is part of the paint.
func (c *control) snapshot(run *controlRun, window time.Duration) *ControlState {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := &ControlState{
		ID:       run.id,
		State:    run.state,
		Reason:   run.reason,
		Activity: run.activity,
		Identity: run.identity,
		SinceMs:  time.Since(run.since).Milliseconds(),
	}
	if run.state == proto.ControlAsking {
		st.LeftMs = max(time.Until(run.deadline).Milliseconds(), 0)
		st.WindowMs = window.Milliseconds()
		if st.WindowMs == 0 {
			st.WindowMs = st.LeftMs
		}
	}
	return st
}
