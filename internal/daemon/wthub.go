package daemon

// The walkthrough submission channel (FR58). An agent that created a review
// usually has nothing to do until the human hands it back, so it parks here
// the way an artifact wait parks (artifacts.go) - but where the artifact
// buffer is the only memory that channel has, a walkthrough's durable state
// IS the buffer: a submission with nobody waiting persists as `submitted`
// and the store's guarded update (DeliverWalkthrough) keeps every handoff
// exactly-once, whether it happens live through this hub or later through
// walkthrough_read with ack.

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

type wtWaiter struct {
	id  string // "" waits on any walkthrough
	out chan wtHandoff
}

// wtHandoff is one resolved wait: a submission's payload, or word that the
// awaited walkthrough was deleted.
type wtHandoff struct {
	id      string
	payload json.RawMessage
	gone    bool
}

type wtHub struct {
	mu      sync.Mutex
	waiters []*wtWaiter
}

// offerSubmission hands a fresh submission to the first matching waiter, if
// any, flipping the store to delivered on the waiter's behalf. It returns
// whether a waiter took it - the board's receipt says "delivered" or
// "saved" from this answer.
func (d *Daemon) offerSubmission(id string, payload string) bool {
	d.wt.mu.Lock()
	defer d.wt.mu.Unlock()
	for i, w := range d.wt.waiters {
		if w.id != "" && w.id != id {
			continue
		}
		if _, err := d.st.DeliverWalkthrough(id, "await"); err != nil {
			// Somebody else (a racing read with ack) already took it; the
			// waiter stays parked for the next submission.
			return false
		}
		d.wt.waiters = slices.Delete(d.wt.waiters, i, i+1)
		w.out <- wtHandoff{id: id, payload: json.RawMessage(payload)} // buffered; the waiter may have left
		d.log.Info("walkthrough.delivered", "component", "daemon", "wt_id", id, "via", "await")
		return true
	}
	return false
}

// releaseGone frees every waiter parked on exactly this walkthrough after a
// delete. An any-walkthrough waiter keeps waiting: its review is whichever
// one arrives next.
func (d *Daemon) releaseGone(id string) {
	d.wt.mu.Lock()
	defer d.wt.mu.Unlock()
	kept := d.wt.waiters[:0]
	for _, w := range d.wt.waiters {
		if w.id == id {
			w.out <- wtHandoff{id: id, gone: true}
			continue
		}
		kept = append(kept, w)
	}
	d.wt.waiters = kept
}

// AwaitWalkthrough blocks until the human submits the review, the window
// elapses, or the walkthrough is deleted. Registration comes BEFORE the
// persisted-state check: a submission landing between the two finds the
// waiter instead of falling into the gap.
func (d *Daemon) AwaitWalkthrough(ctx context.Context, req proto.WalkthroughAwait) proto.WalkthroughAwaitResult {
	w := &wtWaiter{id: req.ID, out: make(chan wtHandoff, 1)}
	d.wt.mu.Lock()
	d.wt.waiters = append(d.wt.waiters, w)
	d.wt.mu.Unlock()

	agent := req.Identity.Agent
	if agent == "" {
		agent = "await"
	}
	if payload, ok := d.takeSubmitted(req.ID, agent); ok {
		d.dropWtWaiter(w)
		return proto.WalkthroughAwaitResult{Submitted: true, Payload: payload}
	}

	d.log.Info("walkthrough.await", "component", "daemon", "wt_id", req.ID,
		"agent", req.Identity.Agent, "timeout_s", req.TimeoutS)

	var timeout <-chan time.Time
	if req.TimeoutS > 0 {
		t := time.NewTimer(time.Duration(req.TimeoutS) * time.Second)
		defer t.Stop()
		timeout = t.C
	}

	select {
	case h := <-w.out:
		if h.gone {
			d.log.Info("walkthrough.await_gone", "component", "daemon", "wt_id", h.id)
			return proto.WalkthroughAwaitResult{Gone: true}
		}
		return proto.WalkthroughAwaitResult{Submitted: true, Payload: h.payload}
	case <-timeout:
		d.dropWtWaiter(w)
		d.log.Info("walkthrough.await_timeout", "component", "daemon", "wt_id", req.ID)
		return proto.WalkthroughAwaitResult{TimedOut: true}
	case <-ctx.Done():
		d.dropWtWaiter(w)
		return proto.WalkthroughAwaitResult{}
	}
}

// takeSubmitted claims a submission that predates the wait: the named
// walkthrough if it sits submitted, or the most recent submitted one when
// the wait is unnamed. A lost claim race is not an error - the review went
// to exactly one other caller, and this one keeps waiting.
func (d *Daemon) takeSubmitted(id, agent string) (json.RawMessage, bool) {
	candidates := []string{}
	if id != "" {
		w, err := d.st.GetWalkthrough(id)
		if err != nil || w.State != store.WtSubmitted {
			return nil, false
		}
		candidates = append(candidates, id)
	} else {
		rows, err := d.st.ListWalkthroughs("", store.WtSubmitted, 0)
		if err != nil {
			return nil, false
		}
		for _, r := range rows {
			candidates = append(candidates, r.ID)
		}
	}
	for _, c := range candidates {
		payload, err := d.st.DeliverWalkthrough(c, agent)
		if err == nil {
			d.log.Info("walkthrough.delivered", "component", "daemon", "wt_id", c, "via", "await_backlog")
			return json.RawMessage(payload), true
		}
	}
	return nil, false
}

// dropWtWaiter deregisters a wait that ended without a handoff. When the
// handoff won the race instead, the review was already marked delivered for
// a reader that is gone - the payload is pushed back to submitted so the
// next await or ack finds it, which the transition trail records honestly.
func (d *Daemon) dropWtWaiter(w *wtWaiter) {
	d.wt.mu.Lock()
	if i := slices.Index(d.wt.waiters, w); i >= 0 {
		d.wt.waiters = slices.Delete(d.wt.waiters, i, i+1)
		d.wt.mu.Unlock()
		return
	}
	d.wt.mu.Unlock()
	select {
	case h := <-w.out:
		if h.gone {
			return
		}
		if err := d.st.SubmitWalkthrough(h.id, string(h.payload)); err != nil && !errors.Is(err, store.ErrWalkthroughNotFound) {
			d.log.Warn("walkthrough.requeue_failed", "component", "daemon", "wt_id", h.id, "err", err.Error())
		}
	default:
	}
}
