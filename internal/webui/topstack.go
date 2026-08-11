package webui

import (
	"sync"
)

// topStack owns the top-centre column (FR75).
//
// Before this, every surface that wanted the top-centre inset computed it for
// itself with x.place(..., top: true, inset) and none of them knew the others
// existed. The first live run of the FR74 strip proved what that costs - the
// geometry, read back with a toast up:
//
//	agentbox · toast       430x78 at 745,48
//	agentbox · hands off   620x62 at 650,48
//
// Same edge, same inset, overlapping rectangles. The stacking order was right, so
// the toast was simply invisible underneath the strip. Boris: "Other notifications
// should know about the hands off and in general of other notifications to not
// collide on the screen - if a second one comes it should be below the previous
// one."
//
// So position is no longer a per-surface decision. Everything that wants the top
// of the screen claims a slot here, and the column lays them out downward from the
// inset in claim order. A surface leaving compacts the ones below it back up,
// because a gap where a toast used to be reads as a window that failed to draw.
type topStack struct {
	ui *UI

	mu    sync.Mutex
	slots []topSlot
}

// topSlot is one claim on the column. w and h are the window's current size,
// because the layout is a sum of heights and a resized toast moves everything
// under it.
type topSlot struct {
	key  string // stable per window: "control", "toast:<id>"
	xid  winID
	w, h int
	// first pins a slot to the top of the column whatever the claim order. Only
	// the hands-off strip sets it: FR74 requires that nothing of agentbox's own covers
	// it, and being second in a column is a quieter way of being covered.
	first bool
}

// topGap is the space between two surfaces in the column. Enough that two dark
// cards on a dark desktop read as two, and not so much that a third is off the
// useful part of the screen.
const topGap = 8

func newTopStack(ui *UI) *topStack { return &topStack{ui: ui} }

// put claims or updates a slot and re-lays the column out. Idempotent per key, so
// the resize path can call it on every measurement without growing the column.
func (t *topStack) put(key string, xid winID, w, h int, first bool) {
	t.mu.Lock()
	found := false
	for i := range t.slots {
		if t.slots[i].key == key {
			t.slots[i].xid, t.slots[i].w, t.slots[i].h, t.slots[i].first = xid, w, h, first
			found = true
			break
		}
	}
	if !found {
		t.slots = append(t.slots, topSlot{key: key, xid: xid, w: w, h: h, first: first})
	}
	t.mu.Unlock()
	t.relayout()
}

// drop releases a slot. Safe to call for a key that never claimed one, which is
// what lets every close path call it without checking.
func (t *topStack) drop(key string) {
	t.mu.Lock()
	kept := t.slots[:0]
	for _, s := range t.slots {
		if s.key != key {
			kept = append(kept, s)
		}
	}
	gone := len(kept) != len(t.slots)
	t.slots = kept
	t.mu.Unlock()
	if gone {
		t.relayout()
	}
}

// relayout moves every claimant to where the column says it belongs: centred
// horizontally on the monitor the person is at, and stacked downward from the top
// inset in order.
//
// The column has a bottom. Past it a surface would be laid out over the middle of
// the screen, where cards live, so the overflow is pinned to the last position
// that fits and logged - a silent cap here would look like a window that failed to
// appear, and this whole file exists because an invisible window is expensive.
func (t *topStack) relayout() {
	if t.ui.x == nil {
		return
	}
	t.mu.Lock()
	order := make([]topSlot, 0, len(t.slots))
	for _, s := range t.slots {
		if s.first {
			order = append(order, s)
		}
	}
	for _, s := range t.slots {
		if !s.first {
			order = append(order, s)
		}
	}
	t.mu.Unlock()

	inset := t.ui.toastTopInset()
	m := t.ui.x.activeMon()
	limit := m.y + m.h/2 // never past the middle: that belongs to cards

	y := m.y + inset
	for _, s := range order {
		py := y
		if py+s.h > limit && y > m.y+inset {
			py = limit - s.h
			t.ui.log.Info("webui.top_stack_overflow", "component", "webui",
				"key", s.key, "slots", len(order))
		}
		px := m.x + (m.w-s.w)/2
		t.ui.x.moveTo(s.xid, px, py)
		y = py + s.h + topGap
	}
	t.ui.x.flush()
}
