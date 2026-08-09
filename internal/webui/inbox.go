package webui

import (
	"fmt"
	"slices"
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
	// RecentBySession is the same read narrowed to one session key, for the Agents
	// board's opened row. Narrowed in SQL rather than by filtering RecentItems: a
	// quiet agent's last item can be a hundred items down, and a filter over the
	// recent hundred would show its row as having raised nothing.
	RecentBySession(key string, limit int) ([]store.StoredItem, error)
	// Promote puts a pending item back on screen. Like the Resolver's methods it
	// answers "" or a sentence saying why not (U-02): an inbox row can outlive the
	// item behind it, and a click that led nowhere used to look like one that worked.
	Promote(id string) string
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

// wireDetail is one item read back in full (FR73). The row above is a summary and
// truncates on purpose; this is the one payload that never does, because it exists
// for the case Boris hit - a card closed on its timer and took its body with it.
//
// It is asked for per opened row rather than shipped in the snapshot: a hundred
// rendered bodies in every push is exactly what Snippet was introduced to avoid.
type wireDetail struct {
	// Found says the id is gone rather than empty, so the surface can say which.
	Found bool `json:"found"`

	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Level string `json:"level"`
	Title string `json:"title"`
	// BodyHTML goes through the same renderer the card used, so a body reads the
	// same way after it closed as it did while it was on screen.
	BodyHTML string `json:"bodyHtml,omitempty"`

	Agent   string `json:"agent"`
	Project string `json:"project"`
	Session string `json:"session,omitempty"`
	Hue     string `json:"hue"`
	Muted   bool   `json:"muted"`

	Pending bool `json:"pending"`
	// Outcome and Tone are the row's chip, sent again here on purpose. The inbox
	// paints the detail under the row that owns it and reads the chip from there,
	// so these are currently unrendered - but this payload is the answer to "read
	// this item back", and one that leaves out how the item ended is only complete
	// while it is glued to a row that says so.
	Outcome string `json:"outcome"`
	Tone    string `json:"tone"`

	// Both timestamps, formatted here for the reason every other decision on this
	// surface is made here: the calendar day an item arrived is agentbox's answer,
	// not the webview's. The millis ride along so the same rel() the rows use can
	// put an age beside the clock time.
	CreatedAt  string `json:"createdAt"`
	CreatedMS  int64  `json:"createdMs"`
	ResolvedAt string `json:"resolvedAt,omitempty"`
	ResolvedMS int64  `json:"resolvedMs,omitempty"`
	// Took is how long the item stood before it ended - the number a reader who
	// missed it actually wants, and one neither timestamp states on its own.
	Took string `json:"took,omitempty"`

	// What was given back, in the item's own terms. At most one of the three is
	// set; a form's values arrive as pairs in the form's own field order, because
	// a map has none and the order the fields were asked in is the order they read.
	Answer string           `json:"answer,omitempty"`
	Reply  string           `json:"reply,omitempty"`
	Values []wireFieldValue `json:"values,omitempty"`

	// Options are the choices the card offered, and they are here for the same
	// reason the body is: they were on screen, they went with it, and an answer
	// read back with nothing to read it against is half a record. Which one was
	// taken is marked rather than left to a string comparison in the surface.
	Options []wireDetailOption `json:"options,omitempty"`

	// The two things FR73 had to leave out. They were written into this struct and
	// taken straight back out, because proto.Item had both fields and the items
	// table had neither column - a reader promising what the read behind it could
	// not deliver. Migration 0012 added them, so a resolved review's diff and the
	// line an agent had spoken are readable now. Items raised before that
	// migration still have neither, which is honest: the surface omits a block it
	// has nothing for rather than showing an empty one.
	Speak string `json:"speak,omitempty"`
	// Diff is the unified diff a review offered, sent raw. It is not run through
	// RenderMarkdown: a diff is not markdown, and the surface has a diff renderer
	// of its own that the card already used.
	Diff string `json:"diff,omitempty"`
}

// wireFieldValue is one answered form field: the label it was asked under, not
// its key.
type wireFieldValue struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// wireDetailOption is one choice the card offered, and whether it was the offered
// default or the one actually taken.
type wireDetailOption struct {
	Label   string `json:"label"`
	Desc    string `json:"desc,omitempty"`
	Default bool   `json:"default,omitempty"`
	Chosen  bool   `json:"chosen,omitempty"`
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

// detail reads one item back in full (FR73). The rows snapshot is the lookup, so
// an opened row normally costs no store read at all; a miss re-reads the source,
// because a surface can hold an id from a repaint the snapshot has since moved
// past and "gone" has to mean gone rather than "not in the last payload".
func (ib *inbox) detail(id string) wireDetail {
	src := ib.ui.source()

	ib.mu.Lock()
	it, found := findItem(ib.rows, id)
	ib.mu.Unlock()

	if !found && src != nil {
		// RecentItems(limit) is the whole of Source's read side, so the fallback is
		// that same read rather than a wider interface bought for one lookup.
		if items, err := src.RecentItems(100); err == nil {
			it, found = findItem(items, id)
		} else {
			ib.ui.log.Error("webui.inbox_detail_reload_failed", "component", "webui", "err", err.Error())
		}
	}
	if !found {
		return wireDetail{ID: id}
	}

	muted := src != nil && slices.Contains(src.MutedAgents(), it.Identity.Agent)
	return encodeDetail(it, muted, ib.ui.themeMode() == "dark")
}

func findItem(rows []store.StoredItem, id string) (store.StoredItem, bool) {
	for i := range rows {
		if rows[i].ID == id {
			return rows[i], true
		}
	}
	return store.StoredItem{}, false
}

func encodeDetail(it store.StoredItem, muted, dark bool) wireDetail {
	outcome, tone := outcomeOf(it)
	d := wireDetail{
		Found:     true,
		ID:        it.ID,
		Kind:      string(it.Kind),
		Level:     string(it.EffectiveLevel()),
		Title:     it.Title,
		BodyHTML:  RenderMarkdown(it.Body),
		Agent:     it.Identity.Agent,
		Project:   it.Identity.Project,
		Session:   it.Identity.Session,
		Hue:       IdentityHue(it.Identity.Agent, it.Identity.Project, dark),
		Muted:     muted,
		Pending:   it.State == store.StatePending,
		Outcome:   outcome,
		Tone:      tone,
		CreatedAt: clockText(it.CreatedAt),
		CreatedMS: ms(it.CreatedAt),
		Answer:    it.Answer,
		Reply:     it.Reply,
		Speak:     it.Speak,
		Diff:      it.Diff,
	}
	if !it.ResolvedAt.IsZero() {
		d.ResolvedAt = clockText(it.ResolvedAt)
		d.ResolvedMS = ms(it.ResolvedAt)
		d.Took = stood(it.ResolvedAt.Sub(it.CreatedAt))
	}
	d.Values = answeredValues(it)
	for _, o := range it.Options {
		d.Options = append(d.Options, wireDetailOption{
			Label:   o.Label,
			Desc:    o.Desc,
			Default: o.Label == it.Default,
			// Taken, not "clicked": an item that expired onto its default records
			// that default as its answer (daemon.go, StateExpired with an Outcome),
			// and the option that went back to the caller is the fact worth marking.
			// Which of the two happened is what the outcome chip beside it says.
			Chosen: it.Answer != "" && o.Label == it.Answer,
		})
	}
	return d
}

// answeredValues pairs a form's answer with the labels it was asked under. The
// map has no order and the fields do, so the fields decide: what is read back is
// what was on the card, top to bottom.
//
// A value under a key no field declares is still listed, sorted, rather than
// dropped - silently losing something the human typed is this FR's own defect in
// miniature.
func answeredValues(it store.StoredItem) []wireFieldValue {
	if len(it.Values) == 0 {
		return nil
	}
	out := make([]wireFieldValue, 0, len(it.Values))
	seen := make(map[string]bool, len(it.Values))
	for _, f := range it.Fields {
		v, ok := it.Values[f.Key]
		if !ok || seen[f.Key] {
			continue
		}
		seen[f.Key] = true
		label := f.Label
		if label == "" {
			label = f.Key
		}
		out = append(out, wireFieldValue{Label: label, Value: v})
	}
	extra := make([]string, 0, len(it.Values))
	for k := range it.Values {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	slices.Sort(extra)
	for _, k := range extra {
		out = append(out, wireFieldValue{Label: k, Value: it.Values[k]})
	}
	return out
}

// clockText is when something happened, to the minute. The exact second an item
// arrived has never mattered; the day and the time of day are what a reader who
// missed it is asking about.
func clockText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("Jan 2 15:04")
}

// stood is how long an item was up before it ended, coarse on purpose and in the
// units a reader thinks in. Under a second reads as nothing at all: an item
// dismissed the instant it arrived did not stand for any length of time.
func stood(d time.Duration) string {
	switch {
	case d < time.Second:
		return ""
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Round(time.Second).Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Round(time.Minute).Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
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
	it, found := findItem(ib.rows, id)
	ib.mu.Unlock()
	if !found || it.State != store.StatePending {
		return false
	}

	// A refusal from the answer path (U-02) reads the same way here as a key that
	// meant nothing: the keystroke did not land. The sentence goes to the log
	// rather than to the row, because the row's hint is written in the key's own
	// vocabulary and this method's answer is a bool by design (FR34) - the surface
	// is told the key did nothing, and the reason is where a session can find it.
	refuse := func(why string) bool {
		if why == "" {
			return true
		}
		ib.ui.log.Info("inbox.triage_refused", "component", "webui", "item_id", it.ID, "key", key, "reason", why)
		return false
	}

	cmd := triageFor(it, key)
	switch cmd.intent {
	case triageAnswer:
		if !refuse(ib.ui.res.Answer(it.ID, cmd.answer)) {
			return false
		}
	case triageVeto:
		if !refuse(ib.ui.res.Veto(it.ID)) {
			return false
		}
	case triageDismiss:
		if !refuse(ib.ui.res.Dismiss(it.ID)) {
			return false
		}
	case triagePromote:
		src := ib.ui.source()
		if src == nil {
			return false
		}
		if !refuse(src.Promote(it.ID)) {
			return false
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
	if it, ok := findItem(ib.rows, id); ok {
		return it.ClipboardText()
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
