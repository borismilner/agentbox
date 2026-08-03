package daemon

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

// The artifact channel (M10). An artifact is agent-authored HTML the human uses
// (internal/webui/artifact.go), sealed in a sandbox with exactly one way out:
// window.agentbox.emit. This is the far end of that channel. The iframe posts to the
// surface, the surface validates it and calls Bridge.ArtifactEvent, and here the
// event meets whichever agent is waiting for it.
//
// What the sandbox is for is worth restating, because it is easy to read as a
// restriction on the agent and it is the opposite. The artifact cannot reach the
// network or the surface, so nothing it contains can act on its own or report
// anywhere. Everything it wants done comes through here to an agent that has
// whatever tools and permissions it already had. A slider the human drags can
// end in a migration; it just cannot end in a request the human never saw.
//
// Two arrivals, because agents ask in two shapes and both were wanted:
//
//   - Blocking. AwaitArtifact parks an agent on the human the way ask does, with
//     the same timeout and the same event log behind it. An agent that shows a
//     form and waits for it is agentbox's own metaphor, in a new medium.
//   - Coalesced. A dragged slider emits many times a second, and an agent that
//     was busy must not come back to forty stale numbers. An event that arrives
//     with nobody waiting is buffered by (artifact, name) with the newest
//     winning, and ReadArtifact drains that buffer without blocking.
//
// None of this enters the item queue or the store. A slider drag is not an
// interruption, and counting it as one would put forty rows in the inbox for one
// gesture and tell the stats surface the human was interrupted forty times. The
// event log is the audit trail: one line per event, one per wait, one per timeout.

// artifactBufferMax bounds what an unattended artifact can accumulate. Coalescing
// already collapses a gesture to one entry per name, so this only bites on an
// artifact inventing new names in a loop, which is a runaway rather than a user.
const artifactBufferMax = 64

type artifactWaiter struct {
	artifactID string
	names      []string
	out        chan proto.ArtifactEvent
}

func (w *artifactWaiter) wants(ev proto.ArtifactEvent) bool {
	if w.artifactID != "" && w.artifactID != ev.ArtifactID {
		return false
	}
	return len(w.names) == 0 || slices.Contains(w.names, ev.Name)
}

type artifactHub struct {
	mu      sync.Mutex
	waiters []*artifactWaiter
	buffer  []proto.ArtifactEvent // oldest first, one per (artifact, name)
}

// ArtifactEvent takes one thing the human did in an artifact (webui.Resolver).
// A waiting agent gets it directly; otherwise it is buffered for the next look.
func (d *Daemon) ArtifactEvent(ev proto.ArtifactEvent) {
	if ev.Name == "" {
		return
	}
	if ev.AtMS == 0 {
		ev.AtMS = time.Now().UnixMilli()
	}

	d.art.mu.Lock()
	handed := false
	for i, w := range d.art.waiters {
		if !w.wants(ev) {
			continue
		}
		// One event is one action: it goes to one agent. A second waiter picking up
		// the same click would run the work twice.
		d.art.waiters = slices.Delete(d.art.waiters, i, i+1)
		w.out <- ev // buffered channel of one; the waiter may already have timed out
		handed = true
		break
	}
	if !handed {
		d.art.buffer = coalesce(d.art.buffer, ev)
	}
	d.art.mu.Unlock()

	d.log.Info("artifact.event", "component", "daemon", "artifact", ev.ArtifactID,
		"event", ev.Name, "bytes", len(ev.Data), "waiting_agent", handed)
}

// coalesce replaces an event of the same name from the same artifact rather than
// queueing behind it, and keeps its place in the order: the buffer stays a
// snapshot of what every control was last set to, in the order the controls were
// first touched.
func coalesce(buf []proto.ArtifactEvent, ev proto.ArtifactEvent) []proto.ArtifactEvent {
	for i := range buf {
		if buf[i].ArtifactID == ev.ArtifactID && buf[i].Name == ev.Name {
			buf[i] = ev
			return buf
		}
	}
	if len(buf) >= artifactBufferMax {
		buf = buf[1:]
	}
	return append(buf, ev)
}

// AwaitArtifact blocks until a matching event arrives, one is already buffered, or
// the window elapses. The caller's context bounds it too: an agent that goes away
// takes its wait with it.
func (d *Daemon) AwaitArtifact(ctx context.Context, req proto.ArtifactWait) proto.ArtifactWaitResult {
	w := &artifactWaiter{artifactID: req.ArtifactID, names: req.Names, out: make(chan proto.ArtifactEvent, 1)}

	d.art.mu.Lock()
	for i, ev := range d.art.buffer {
		if !w.wants(ev) {
			continue
		}
		d.art.buffer = slices.Delete(d.art.buffer, i, i+1)
		d.art.mu.Unlock()
		d.log.Debug("artifact.await_buffered", "component", "daemon", "artifact", ev.ArtifactID, "event", ev.Name)
		return proto.ArtifactWaitResult{Received: true, Event: &ev}
	}
	d.art.waiters = append(d.art.waiters, w)
	d.art.mu.Unlock()

	d.log.Info("artifact.await", "component", "daemon", "artifact", req.ArtifactID,
		"events", req.Names, "timeout_s", req.TimeoutS)

	var timeout <-chan time.Time
	if req.TimeoutS > 0 {
		t := time.NewTimer(time.Duration(req.TimeoutS) * time.Second)
		defer t.Stop()
		timeout = t.C
	}

	select {
	case ev := <-w.out:
		return proto.ArtifactWaitResult{Received: true, Event: &ev}
	case <-timeout:
		d.dropWaiter(w)
		d.log.Info("artifact.await_timeout", "component", "daemon", "artifact", req.ArtifactID)
		return proto.ArtifactWaitResult{TimedOut: true}
	case <-ctx.Done():
		d.dropWaiter(w)
		return proto.ArtifactWaitResult{}
	}
}

// dropWaiter deregisters a wait that ended without an event, and puts back an
// event that arrived in the same instant: the publisher hands to one waiter and
// forgets it, so a race here would lose the human's click rather than deliver it
// late.
func (d *Daemon) dropWaiter(w *artifactWaiter) {
	d.art.mu.Lock()
	if i := slices.Index(d.art.waiters, w); i >= 0 {
		d.art.waiters = slices.Delete(d.art.waiters, i, i+1)
		d.art.mu.Unlock()
		return
	}
	select {
	case ev := <-w.out:
		d.art.buffer = coalesce(d.art.buffer, ev)
	default:
	}
	d.art.mu.Unlock()
}

// artifactWaitParams parses what both artifact methods take. An empty body is
// legal and means "any artifact, any event": an agent that showed one thing should
// not have to name it.
func artifactWaitParams(params []byte, method string) (proto.ArtifactWait, *proto.RPCError) {
	var req proto.ArtifactWait
	if len(params) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return req, &proto.RPCError{
			Code:    proto.CodeInvalidParams,
			Message: method + " wants {artifact_id?, names?, timeout_s?} or {}: " + err.Error(),
		}
	}
	return req, nil
}

// ReadArtifact drains the buffer without blocking: everything the human has done
// that no agent picked up, newest value per control.
func (d *Daemon) ReadArtifact(req proto.ArtifactWait) proto.ArtifactReadResult {
	probe := &artifactWaiter{artifactID: req.ArtifactID, names: req.Names}

	d.art.mu.Lock()
	defer d.art.mu.Unlock()

	out := make([]proto.ArtifactEvent, 0, len(d.art.buffer))
	kept := d.art.buffer[:0]
	for _, ev := range d.art.buffer {
		if probe.wants(ev) {
			out = append(out, ev)
			continue
		}
		kept = append(kept, ev)
	}
	d.art.buffer = kept
	return proto.ArtifactReadResult{Events: out}
}
