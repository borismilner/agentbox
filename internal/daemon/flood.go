package daemon

import (
	"fmt"
	"time"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// Flood control (FR30). An agent that puts more than cfg.FloodBurst items on
// screen inside cfg.FloodWindow stops getting a card each: everything past the
// budget collapses into ONE stack card carrying the burst, and the human reads a
// count instead of a wall.
//
// Nothing is dropped, which was the decision (Boris, 2026-08-06) between this and
// a rate limit that discards. Every collapsed item is stored and pending exactly
// as it would have been; the stack card is a different way of SHOWING items that
// all still exist. That matters most for a blocking call caught in a burst: its
// caller is still parked on it, and opening its row promotes the real item back
// onto the screen where it can be answered.
//
// The budget is per SESSION and not per agent name, though the setting reads
// "per agent": every Claude session on this machine calls itself `claude`, so a
// budget keyed on the name would be one budget shared by every session in the
// house - and the first looping agent would collapse its innocent neighbours.
// proto.Identity.Key names one session and is what the roster already trusts.

// floodState is one caller's budget and its open stack card.
type floodState struct {
	// recent is the arrival time of each item still inside the window, oldest
	// first. It is trimmed on every arrival, so it never grows past burst.
	recent []time.Time
	// stack is the ID of the stack card currently collecting this caller's
	// overflow, or "" when there is none open. A stack card that gets resolved
	// (dismissed, or read in the inbox) ends the collection: the next collapse
	// starts a fresh card rather than reviving one the human has finished with.
	stack string
}

// floodKey names the budget an item is charged to.
func floodKey(id proto.Identity) string {
	if id.Key != "" {
		return "key:" + id.Key
	}
	// Sessions older than sync (FR83) have no key. They fall back to the triple,
	// which is coarser - two keyless sessions in one repo do share a budget - but
	// it is the same identity the rest of agentbox falls back to, and a keyless
	// session is already a session the roster cannot tell apart.
	return "triple:" + id.Agent + "|" + id.Project + "|" + id.Session
}

// stackTitle is what the collapsed card says it is. The count is the headline
// because the count is the complaint.
//
// "notifications" only while they all are. A burst holding a question is not
// five notifications, and the title is the line a person decides from - it read
// "5 notifications" over a row saying "waiting on you" on the real desktop,
// which is the card contradicting itself in its own first sentence.
func stackTitle(agent string, entries []proto.StackEntry, within time.Duration) string {
	n := len(entries)
	noun := "notifications"
	if n == 1 {
		noun = "notification"
	}
	for _, e := range entries {
		if e.Blocking {
			noun = "items"
			if n == 1 {
				noun = "item"
			}
			break
		}
	}
	return fmt.Sprintf("%s: %d %s in %s", agent, n, noun, roundSecs(within))
}

// roundSecs renders a burst's span the way a person would say it.
func roundSecs(d time.Duration) string {
	if d < time.Second {
		return "under a second"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Round(time.Second).Seconds()))
	}
	return fmt.Sprintf("%dm", int(d.Round(time.Minute).Minutes()))
}

// collapseLocked charges an arriving item to its caller's budget and reports
// whether it was collapsed. When it returns true the item must NOT be enqueued:
// it is already recorded in the stack card returned, which is either newly made
// (and must be enqueued in its place) or already on screen (and fresh is nil).
//
// Called under d.mu, before enqueueLocked, and only for items that have already
// been written to the store - the entry it records points at a real row.
func (d *Daemon) collapseLocked(it *proto.Item, now time.Time) (collapsed bool, fresh *proto.Item) {
	if d.cfg.FloodBurst <= 0 || d.cfg.FloodWindow <= 0 {
		return false, nil
	}
	// A stack card is never itself charged to the budget: it IS the budget's
	// answer, and charging it would let flood control collapse its own output.
	if it.Kind == proto.KindStack {
		return false, nil
	}
	if d.flood == nil {
		d.flood = map[string]*floodState{}
	}
	key := floodKey(it.Identity)
	f := d.flood[key]
	if f == nil {
		f = &floodState{}
		d.flood[key] = f
	}

	cutoff := now.Add(-d.cfg.FloodWindow)
	kept := f.recent[:0]
	for _, t := range f.recent {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	f.recent = kept

	// A stack card that is still on screen keeps collecting, whatever the bucket
	// says. Without this the budget refills underneath an open stack and a
	// sustained loop gets a fresh card every window - measured on the real
	// desktop at three cards per ten seconds, which is eighteen a minute and not
	// anybody's idea of "calm survives buggy callers". The collapse ends when the
	// human ends it: dismissing the stack (or answering through it) takes the
	// card out of the queue, and the next item after that starts a clean budget.
	open := d.liveStackLocked(f.stack)
	if open == nil && len(f.recent) < d.cfg.FloodBurst {
		f.recent = append(f.recent, now)
		return false, nil
	}

	entry := proto.StackEntry{
		ID:       it.ID,
		Kind:     it.Kind,
		Level:    it.EffectiveLevel(),
		Title:    it.Title,
		Blocking: it.Blocking(),
		AtMS:     now.UnixMilli(),
	}

	if open != nil {
		open.Stack = append(open.Stack, entry)
		open.Title = stackTitle(it.Identity.Agent, open.Stack, stackSpan(open.Stack, now))
		// The card wears the worst thing inside it. A burst that started as three
		// info toasts and has since collapsed an error must not still read as the
		// mildest of its contents - the human is being asked to judge the whole
		// stack from one line.
		open.Level = worstLevel(open.Stack)
		d.persistStack(open)
		// An urgent item must still break through. Collapsed into a stack card that
		// is sitting in the queue it would not: the card rises to urgent and then
		// waits behind whatever is on screen, so flood control would have turned
		// the one level that is allowed to interrupt into the one that cannot.
		// Same rule as an ordinary urgent arrival (enqueueLocked), applied to the
		// card that now speaks for it.
		if open.Level == proto.LevelUrgent {
			d.raiseLocked(open)
		}
		d.log.Info(logging.EvItemCollapsed, "component", "daemon", "item_id", it.ID,
			"stack_id", open.ID, "agent", it.Identity.Agent, "collapsed", len(open.Stack))
		return true, nil
	}

	stack := &proto.Item{
		ID:    newID(),
		Kind:  proto.KindStack,
		Level: worstLevel([]proto.StackEntry{entry}),
		Title: stackTitle(it.Identity.Agent, []proto.StackEntry{entry}, 0),
		// The warning FR30 asks for, said on the card that carries the burst rather
		// than on a second card beside it. Two cards for one flood would be the
		// noise this feature exists to stop.
		Body:     fmt.Sprintf("Over its rate limit (%d in %s), so its items are being collapsed here instead of arriving one at a time. Nothing has been dropped.", d.cfg.FloodBurst, roundSecs(d.cfg.FloodWindow)),
		Stack:    []proto.StackEntry{entry},
		Identity: it.Identity,
	}
	if err := d.st.CreateItem(stack); err != nil {
		// The store refused the card that was to hold the burst. Collapsing into
		// nothing would silently eat the item, so the honest fallback is to let it
		// through as its own card: noisy beats lost.
		d.log.Error("store.stack_create_failed", "component", "daemon", "item_id", it.ID, "err", err.Error())
		return false, nil
	}
	f.stack = stack.ID
	d.log.Info(logging.EvFlooded, "component", "daemon", "stack_id", stack.ID,
		"agent", it.Identity.Agent, "project", it.Identity.Project,
		"burst", d.cfg.FloodBurst, "window_s", int(d.cfg.FloodWindow.Seconds()))
	d.log.Info(logging.EvItemCollapsed, "component", "daemon", "item_id", it.ID,
		"stack_id", stack.ID, "agent", it.Identity.Agent, "collapsed", 1)
	return true, stack
}

// raiseLocked puts an already-queued stack card on screen because something
// urgent has just been collapsed into it. It is enqueueLocked's urgent rule
// applied after the fact: the card is only ever preempting a card that is not
// itself urgent, and during a recording it takes the front of the queue instead
// of the screen, which is the one thing recording mode exists to guarantee.
func (d *Daemon) raiseLocked(stack *proto.Item) {
	if d.current == stack {
		return
	}
	for i, q := range d.queue {
		if q == stack {
			d.queue = append(d.queue[:i], d.queue[i+1:]...)
			d.enqueueLocked(stack)
			return
		}
	}
}

// markStackedLocked quiets the row of an item that has just been resolved, in
// whichever live stack card is carrying it. Called from resolve, so every way an
// item can end - answered through its own row, dismissed, expired, cancelled -
// reaches the list showing it.
//
// The row is marked and not removed. A list that reflows under the pointer is
// how the wrong thing gets clicked, and the burst is also a record: "you were
// sent fourteen things" stays true after you have dealt with four of them.
func (d *Daemon) markStackedLocked(id string) *proto.Item {
	all := d.queue
	if d.current != nil {
		all = append([]*proto.Item{d.current}, d.queue...)
	}
	for _, it := range all {
		if it.Kind != proto.KindStack {
			continue
		}
		for i := range it.Stack {
			if it.Stack[i].ID == id && !it.Stack[i].Done {
				it.Stack[i].Done = true
				return it
			}
		}
	}
	return nil
}

// worstLevel is the level a stack card wears: the highest of the entries in it,
// floored at warning. Warning is the floor because the collapse itself is the
// warning FR30 asks for - a stack of info toasts is still agentbox saying an
// agent is over its limit, and saying that in the info voice would bury it.
func worstLevel(entries []proto.StackEntry) proto.Level {
	worst := proto.LevelWarning
	for _, e := range entries {
		if e.Level.Rank() > worst.Rank() {
			worst = e.Level
		}
	}
	return worst
}

// stackSpan is how long the burst in a stack card has been running.
func stackSpan(entries []proto.StackEntry, now time.Time) time.Duration {
	if len(entries) == 0 {
		return 0
	}
	return now.Sub(time.UnixMilli(entries[0].AtMS))
}

// liveStackLocked finds a stack card that is still on screen or still queued.
// A card that has left both is finished with, whatever the store says about it.
func (d *Daemon) liveStackLocked(id string) *proto.Item {
	if id == "" {
		return nil
	}
	if d.current != nil && d.current.ID == id {
		return d.current
	}
	for _, q := range d.queue {
		if q.ID == id {
			return q
		}
	}
	return nil
}

// persistStack writes a stack card's grown entry list back, so a restart
// re-presents the burst it actually collected rather than its first entry.
func (d *Daemon) persistStack(it *proto.Item) {
	if err := d.st.UpdateStack(it.ID, it.Title, it.Stack); err != nil {
		d.log.Error("store.stack_update_failed", "component", "daemon", "item_id", it.ID, "err", err.Error())
	}
}

// OpenStacked promotes one collapsed item out of a stack card and onto the
// screen, which is how a blocking call caught in a burst gets answered (FR30).
// The stack card keeps the row: the human is looking at a list and a row that
// vanished under the pointer is how the wrong thing gets clicked. The row is
// marked instead, by the item resolving in the normal way.
func (d *Daemon) OpenStacked(stackID, itemID string) string {
	d.mu.Lock()
	stack := d.liveStackLocked(stackID)
	if stack == nil || stack.Kind != proto.KindStack {
		d.mu.Unlock()
		d.log.Warn("stack.open_rejected", "component", "daemon", "stack_id", stackID, "item_id", itemID, "reason", "no live stack card")
		return "that stack card is no longer on screen."
	}
	held := false
	for _, e := range stack.Stack {
		if e.ID == itemID {
			held = true
			break
		}
	}
	if !held {
		d.mu.Unlock()
		d.log.Warn("stack.open_rejected", "component", "daemon", "stack_id", stackID, "item_id", itemID, "reason", "not in this stack")
		return "that row is not one of the items in this stack."
	}
	// Already on screen or already queued: nothing to promote, and promoting a
	// second copy would put two cards on one item. Not a refusal - what the row
	// asked for has already happened.
	if (d.current != nil && d.current.ID == itemID) || d.liveStackLocked(itemID) != nil {
		d.mu.Unlock()
		return ""
	}
	d.mu.Unlock()

	stored, err := d.st.Item(itemID)
	if err != nil || stored == nil {
		d.log.Warn("stack.open_rejected", "component", "daemon", "stack_id", stackID, "item_id", itemID, "reason", "item is gone from the store")
		return "that item is gone: the store has no record of it."
	}
	if stored.State != store.StatePending {
		// Answered from the inbox, expired, or dismissed while the stack sat open.
		// Re-presenting it would ask a question that already has an answer.
		d.log.Info("stack.open_skipped", "component", "daemon", "item_id", itemID, "state", stored.State)
		return "that item was already " + stored.State + ", so there is no card to open."
	}

	it := stored.Item
	d.mu.Lock()
	// Order matters and it is the opposite of the obvious one. The stack card
	// steps aside FIRST, and only then does the promoted item go to the front -
	// so the queue is [item, stack, ...] and advance puts the item on screen with
	// the list waiting right behind it. Written the other way round (item first,
	// then the stack pushed in front of it) the stack card simply retook the
	// screen and the promoted item sat behind it reading "1 waiting", which is
	// what the number key appeared to do on the real desktop while every unit
	// test passed - they all had the stack in the queue rather than on screen.
	if d.current != nil && d.current.ID == stackID {
		d.stopTimerLocked(d.current.ID)
		d.queue = append([]*proto.Item{d.current}, d.queue...)
		d.current = nil
	}
	d.queue = append([]*proto.Item{&it}, d.queue...)
	d.advanceLocked()
	view := d.viewLocked()
	d.mu.Unlock()
	d.log.Info("stack.opened", "component", "daemon", "stack_id", stackID, "item_id", itemID)
	d.ui.Present(view)
	return ""
}

// sweepStack is what dismissing a stack card means for the burst under it: the
// notifications go with it (the human has seen the count and said enough), and
// every blocking row stays pending, because an agent parked on a question is not
// answered by somebody closing a summary. Those keep their place in the inbox,
// which is where FR30 said they would be resolved.
//
// It takes the ID rather than the item, and every door that retires an item
// calls it, because there are three: the card's own Esc (Resolver.Dismiss), the
// inbox row, and `agentbox dismiss` / an agent retracting (DismissItems, FR89).
// It was written on the first of those alone, and the CLI then dismissed a stack
// card and left five invisible notifications pending behind it - found by
// clearing the queue after a live test, not by reading the diff.
func (d *Daemon) sweepStack(id string) {
	d.mu.Lock()
	var ids []string
	if it := d.liveStackLocked(id); it != nil && it.Kind == proto.KindStack {
		for _, e := range it.Stack {
			if !e.Blocking {
				ids = append(ids, e.ID)
			}
		}
	}
	d.mu.Unlock()
	for _, cid := range ids {
		d.resolve(cid, store.StateDismissed, store.Outcome{})
	}
}
