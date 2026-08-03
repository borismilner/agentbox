package webui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// The inbox surface (FR10) and the triage rules behind it (FR34/FR50).
//
// Source is deliberately a copy of internal/ui's interface rather than an
// import of it: the two UIs must not depend on each other, so deleting the Gio
// package at the cutover touches nothing here. *daemon.Daemon satisfies both.
type Source interface {
	RecentItems(limit int) ([]store.StoredItem, error)
	Promote(id string)
	MutedAgents() []string                      // FR47: agents to badge "(muted)"
	Stats(since time.Time) (proto.Stats, error) // FR35: the history surface
}

// SetSource wires the daemon in after it exists (the UI is built first, so the
// daemon can be handed it as its Presenter).
func (u *UI) SetSource(s Source) {
	u.mu.Lock()
	u.src = s
	u.mu.Unlock()
	u.inbox.noteChange()
}

func (u *UI) source() Source {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.src
}

// wireItem is one row. Everything the row displays is decided here rather than
// in the frontend, because every one of these decisions (what a veto's outcome
// is called, which keys act on a form, whether an agent is muted) is agentbox's
// vocabulary, not the surface's. The surface paints; it does not interpret.
type wireItem struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Level   string `json:"level"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Agent   string `json:"agent"`
	Project string `json:"project"`
	Session string `json:"session,omitempty"`
	Hue     string `json:"hue"`

	Pending  bool `json:"pending"`
	Blocking bool `json:"blocking"`
	Muted    bool `json:"muted"`
	Missed   bool `json:"missed,omitempty"` // FR44: auto-closed while away

	Outcome string `json:"outcome"`
	Tone    string `json:"tone"` // info | success | warning | error | muted
	Hint    string `json:"hint,omitempty"`

	CreatedMS int64 `json:"createdMs"`
}

// wireInbox is the whole surface in one payload: the rows plus the two numbers
// the header and footer show. Sending it whole means the surface never has to
// recompute a count the Go side already knows.
type wireInbox struct {
	Items   []wireItem `json:"items"`
	Pending int        `json:"pending"`
	Today   int        `json:"today"`
	Muted   []string   `json:"muted"`
}

type inbox struct {
	ui *UI

	mu   sync.Mutex
	rows []store.StoredItem // last snapshot, so a triage key can look an item up

	dirty   bool
	pushing bool
}

func newInbox(ui *UI) *inbox { return &inbox{ui: ui} }

// snapshot re-reads the source. The inbox is small (100 rows) and only refreshed
// on a queue change, so there is no cache to go stale.
func (ib *inbox) snapshot() wireInbox {
	src := ib.ui.source()
	if src == nil {
		return wireInbox{Items: []wireItem{}, Muted: []string{}}
	}
	items, err := src.RecentItems(100)
	if err != nil {
		ib.ui.log.Error("webui.inbox_reload_failed", "component", "webui", "err", err.Error())
		return wireInbox{Items: []wireItem{}, Muted: []string{}}
	}
	mutedList := src.MutedAgents()
	if mutedList == nil {
		mutedList = []string{}
	}
	muted := make(map[string]bool, len(mutedList))
	for _, a := range mutedList {
		muted[a] = true
	}

	ordered := pendingFirst(items)

	ib.mu.Lock()
	ib.rows = ordered
	ib.mu.Unlock()

	dark := ib.ui.themeMode() == "dark"
	out := wireInbox{Items: make([]wireItem, 0, len(ordered)), Muted: mutedList}
	for i := range ordered {
		it := ordered[i]
		if it.State == store.StatePending {
			out.Pending++
		}
		out.Items = append(out.Items, encodeItem(it, muted[it.Identity.Agent], dark))
	}
	out.Today = todayCount(ordered, time.Now())
	return out
}

func encodeItem(it store.StoredItem, muted, dark bool) wireItem {
	outcome, tone := outcomeOf(it)
	w := wireItem{
		ID:        it.ID,
		Kind:      string(it.Kind),
		Level:     string(it.EffectiveLevel()),
		Title:     it.Title,
		Snippet:   snippet(it.Body, 140),
		Agent:     it.Identity.Agent,
		Project:   it.Identity.Project,
		Session:   it.Identity.Session,
		Hue:       IdentityHue(it.Identity.Agent, it.Identity.Project, dark),
		Pending:   it.State == store.StatePending,
		Blocking:  it.Blocking(),
		Muted:     muted,
		Missed:    it.MissedWhileAway,
		Outcome:   outcome,
		Tone:      tone,
		CreatedMS: ms(it.CreatedAt),
	}
	if w.Pending {
		w.Hint = triageHint(it)
	}
	return w
}

// pendingFirst puts the unanswered items on top, each group keeping the store's
// order. Triage then selects within the leading run.
func pendingFirst(items []store.StoredItem) []store.StoredItem {
	pending := make([]store.StoredItem, 0, len(items))
	done := make([]store.StoredItem, 0, len(items))
	for _, it := range items {
		if it.State == store.StatePending {
			pending = append(pending, it)
			continue
		}
		done = append(done, it)
	}
	return append(pending, done...)
}

// outcomeOf is the row's right-hand chip: how the item ended, in the item's own
// vocabulary. A veto that expired *proceeded* - saying "expired" there would be
// technically true and completely misleading.
func outcomeOf(it store.StoredItem) (text, tone string) {
	if it.MissedWhileAway {
		return "missed while away", "warning"
	}
	if it.Kind == proto.KindVeto {
		switch it.State {
		case store.StatePending:
			return "deciding", "info"
		case store.StateAnswered:
			return "vetoed", "error"
		default:
			return "proceeded", "success"
		}
	}
	switch it.State {
	case store.StatePending:
		return "waiting", "info"
	case store.StateAnswered:
		switch {
		case it.Reply != "":
			return "replied", "success"
		case it.Answer != "":
			return it.Answer, "success"
		}
		return "answered", "success"
	case store.StateExpired:
		return "expired", "muted"
	case store.StateCancelled:
		return "cancelled", "muted"
	}
	return "", "muted"
}

// snippet is a one-line preview of the body: markdown syntax is noise at this
// size, so the row shows the prose flattened rather than rendered.
func snippet(body string, n int) string {
	s := strings.Join(strings.Fields(strings.NewReplacer("`", "", "*", "", "#", "", ">", "").Replace(body)), " ")
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

// todayCount counts items created on the same local calendar day as now - the
// footer's "N interruptions today" (FR35).
func todayCount(items []store.StoredItem, now time.Time) int {
	y, m, d := now.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	n := 0
	for _, it := range items {
		if !it.CreatedAt.Before(start) {
			n++
		}
	}
	return n
}

// --- triage (FR34/FR50) -----------------------------------------------------

type triageIntent int

const (
	triageNone triageIntent = iota
	triageAnswer
	triageVeto
	triageDismiss
	triagePromote
)

type triageCmd struct {
	intent triageIntent
	answer string // label delivered for triageAnswer
}

// triageFor maps a keypress on the selected pending row to an action, mirroring
// the on-card shortcuts so one keyboard vocabulary covers both. Items needing
// typed input are promoted to their full card instead of answered in place.
// Questions can also be dismissed for good (FR50) - except veto and diff, where
// walking away has consequences.
//
// The mapping lives in Go, not in the surface, so the card, the inbox and any
// future surface cannot drift apart, and so it stays table-testable.
func triageFor(it store.StoredItem, key string) triageCmd {
	if key == "d" || key == "Backspace" {
		switch it.Kind {
		case proto.KindVeto, proto.KindDiff:
			// s stops a veto, a/x answer a review; no silent walk-away here.
		default:
			return triageCmd{intent: triageDismiss}
		}
	}
	switch it.Kind {
	case proto.KindChoice:
		if n := digit(key); n >= 1 && n <= len(it.Options) {
			return triageCmd{intent: triageAnswer, answer: it.Options[n-1].Label}
		}
		if key == "Enter" && it.Default != "" {
			return triageCmd{intent: triageAnswer, answer: it.Default}
		}
	case proto.KindConfirm:
		switch key {
		case "y":
			return triageCmd{intent: triageAnswer, answer: "yes"}
		case "n":
			return triageCmd{intent: triageAnswer, answer: "no"}
		case "Enter":
			if it.Default != "" {
				return triageCmd{intent: triageAnswer, answer: it.Default}
			}
		}
	case proto.KindVeto:
		if key == "s" {
			return triageCmd{intent: triageVeto}
		}
	case proto.KindText, proto.KindSecret, proto.KindForm, proto.KindDiff:
		if key == "Enter" {
			return triageCmd{intent: triagePromote}
		}
	}
	return triageCmd{intent: triageNone}
}

func digit(key string) int {
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		return int(key[0] - '0')
	}
	return 0
}

// triageHint is the affordance under the selected row: the keys that act on it,
// in the item's own terms.
func triageHint(it store.StoredItem) string {
	var keys []string
	switch it.Kind {
	case proto.KindChoice:
		for i, o := range it.Options {
			if i >= proto.MaxOptions {
				break
			}
			keys = append(keys, fmt.Sprintf("%d %s", i+1, strings.ToLower(o.Label)))
		}
		if it.Default != "" {
			keys = append(keys, "enter "+strings.ToLower(it.Default))
		}
	case proto.KindConfirm:
		keys = append(keys, "y yes", "n no")
		if it.Default != "" {
			keys = append(keys, "enter "+strings.ToLower(it.Default))
		}
	case proto.KindVeto:
		keys = append(keys, "s stop")
	case proto.KindText, proto.KindSecret, proto.KindForm:
		keys = append(keys, "enter open")
	case proto.KindDiff:
		keys = append(keys, "enter review")
	case proto.KindNotify:
		keys = append(keys, "d dismiss")
	}
	switch it.Kind {
	case proto.KindVeto, proto.KindDiff, proto.KindNotify:
	default:
		keys = append(keys, "d dismiss")
	}
	keys = append(keys, "c copy")
	return strings.Join(keys, " · ")
}

// act carries out a triage command. Resolution re-presents, which pushes a
// fresh snapshot, so the row updates without the surface asking.
func (ib *inbox) act(id, key string) bool {
	ib.mu.Lock()
	var it store.StoredItem
	found := false
	for _, row := range ib.rows {
		if row.ID == id {
			it, found = row, true
			break
		}
	}
	ib.mu.Unlock()
	if !found || it.State != store.StatePending {
		return false
	}

	cmd := triageFor(it, key)
	switch cmd.intent {
	case triageAnswer:
		ib.ui.res.Answer(it.ID, cmd.answer)
	case triageVeto:
		ib.ui.res.Veto(it.ID)
	case triageDismiss:
		ib.ui.res.Dismiss(it.ID)
	case triagePromote:
		if src := ib.ui.source(); src != nil {
			src.Promote(it.ID)
		}
	default:
		return false
	}
	ib.noteChange()
	return true
}

// clipText is the row's paste form (FR43). The surface hands over an id, never
// text, so the clipboard can only ever hold something agentbox actually holds.
func (ib *inbox) clipText(id string) string {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	for i := range ib.rows {
		if ib.rows[i].ID == id {
			return ib.rows[i].ClipboardText()
		}
	}
	return ""
}

// noteChange coalesces pushes the same way the session surface does: a burst of
// queue changes (an answer resolving three waiters) is one repaint, not three.
func (ib *inbox) noteChange() {
	ib.mu.Lock()
	ib.dirty = true
	if ib.pushing {
		ib.mu.Unlock()
		return
	}
	ib.pushing = true
	ib.mu.Unlock()

	go func() {
		for {
			time.Sleep(80 * time.Millisecond)
			ib.mu.Lock()
			if !ib.dirty {
				ib.pushing = false
				ib.mu.Unlock()
				return
			}
			ib.dirty = false
			ib.mu.Unlock()
			ib.ui.emit("agentbox:inbox", ib.snapshot())
		}
	}()
}
