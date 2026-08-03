package webui

import (
	"strconv"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
)

// The inline ask panel (FR49). An agent running inside agentbox's own session
// surface is already on screen, so a card popping over that window is worse than
// unnecessary: it covers the conversation the question is about. A session-tagged
// question is answered in the conversation instead, between the transcript and
// the composer.
//
// Everything with a vocabulary is decided here rather than in the surface, for
// the same reason the triage keymap and the toast treatment are: the panel, the
// card and the inbox have to agree about what a key means and what a control is
// called, and a decision in Go is a decision a test can hold. The keystroke path
// is literally the inbox's table (triageFor), so a digit cannot answer one thing
// in a conversation and another thing in the inbox.

// wireChoice is one control in the panel. The surface loops over these and knows
// nothing about kinds: it paints a label, a key cap and whichever verb Go put on
// it. That is why a notice's Dismiss arrives as an option too - the alternative
// is a switch on the kind in the frontend, which is exactly the drift this file
// exists to prevent.
type wireChoice struct {
	Label   string `json:"label"`
	Desc    string `json:"desc,omitempty"`
	Key     string `json:"key,omitempty"`    // the key cap shown on the control
	Answer  string `json:"answer,omitempty"` // the label to deliver (Bridge.Answer)
	Verb    string `json:"verb,omitempty"`   // "dismiss" instead of an answer
	Primary bool   `json:"primary,omitempty"`
}

// wireAction is a notify's caller-supplied button (FR32), by index so the
// surface never sees a command it could run.
type wireAction struct {
	Label string `json:"label"`
	Exec  string `json:"exec"` // shown on hover, anti-spoof (03-ui-ux.md)
	Index int    `json:"index"`
}

// wireAsk is the question a conversation answers in place.
type wireAsk struct {
	ID          string       `json:"id"`
	Kind        string       `json:"kind"`
	Level       string       `json:"level"`
	Glyph       string       `json:"glyph"`
	Lead        string       `json:"lead"` // what the panel calls itself
	Title       string       `json:"title"`
	BodyHTML    string       `json:"bodyHtml,omitempty"`
	Hue         string       `json:"hue"`
	Options     []wireChoice `json:"options"`
	Actions     []wireAction `json:"actions,omitempty"`
	Hint        string       `json:"hint"`
	ExpiresAtMS int64        `json:"expiresAtMs,omitempty"`
	Waiting     int          `json:"waiting,omitempty"`
}

// inlineSupported reports the kinds a conversation can answer in place: a
// question with buttons, and a notice. The rest keep their card - text and
// secret want a field and a submit, a form wants six of them, a diff wants the
// window, and a veto is a countdown with consequences.
func inlineSupported(k proto.Kind) bool {
	switch k {
	case proto.KindNotify, proto.KindChoice, proto.KindConfirm:
		return true
	}
	return false
}

// inlineRoutable decides whether an item is answered in a conversation instead
// of a card. Four things have to hold: the item names a session, that session is
// one the surface is showing, the kind is answerable in a panel, and the level is
// not urgent - urgent keeps the card and its escalation for the same reason it
// skips the toast (03-ui-ux.md): a notice that mattered that much must not be
// somewhere the user may not be looking.
//
// The app window being open is the condition the Gio build never had to ask
// about, because there a session died with its window. Here a session outlives
// the window, so a question routed into a conversation nobody can see would be an
// agent waiting forever.
func inlineRoutable(it *proto.Item, sessionShown, appOpen bool) bool {
	if it == nil || !appOpen || !sessionShown {
		return false
	}
	if it.Identity.Session == "" {
		return false
	}
	if it.EffectiveLevel() == proto.LevelUrgent {
		return false
	}
	return inlineSupported(it.Kind)
}

// encodeAsk turns the daemon's view into the panel. Every control it will paint,
// including which key answers what and which one is the default, is resolved
// here.
func (u *UI) encodeAsk(v daemon.View) *wireAsk {
	it := v.Item
	if it == nil {
		return nil
	}
	dark := u.themeMode() == "dark"
	a := &wireAsk{
		ID:          it.ID,
		Kind:        string(it.Kind),
		Level:       string(it.EffectiveLevel()),
		Glyph:       severityGlyph(it.EffectiveLevel()),
		Lead:        askLead(it.Kind),
		Title:       it.Title,
		BodyHTML:    RenderMarkdown(it.Body),
		Hue:         IdentityHue(it.Identity.Agent, it.Identity.Project, dark),
		Options:     askOptions(it),
		Hint:        triageHint(store.StoredItem{Item: *it, State: store.StatePending}),
		ExpiresAtMS: ms(v.ExpiresAt),
		Waiting:     v.Waiting,
	}
	if v.ActionsEnabled {
		for i, act := range it.Actions {
			a.Actions = append(a.Actions, wireAction{Label: act.Label, Exec: act.Exec, Index: i})
		}
	}
	return a
}

// askLead is the panel's own label. A question is something the agent is blocked
// on; a notice is something it wanted the user to know. The panel should not read
// the same for both.
func askLead(k proto.Kind) string {
	if k == proto.KindNotify {
		return "Notice"
	}
	return "Waiting on your answer"
}

// askOptions is the control row. A choice gets its options on the number row, a
// confirm gets yes/no on y/n, and a notice gets the one control it has. The keys
// match triageFor, which is what actually acts on them.
func askOptions(it *proto.Item) []wireChoice {
	switch it.Kind {
	case proto.KindChoice:
		out := make([]wireChoice, 0, len(it.Options))
		for i, o := range it.Options {
			c := wireChoice{Label: o.Label, Desc: o.Desc, Answer: o.Label, Primary: o.Label == it.Default}
			if i < proto.MaxOptions {
				c.Key = strconv.Itoa(i + 1)
			}
			out = append(out, c)
		}
		return out
	case proto.KindConfirm:
		// The yes/no vocabulary stays in Go: the surface shows "Yes" and delivers
		// whatever answer this says, the same arrangement Bridge.Confirm has.
		return []wireChoice{
			{Label: "Yes", Key: "y", Answer: "yes", Primary: it.Default != "no"},
			{Label: "No", Key: "n", Answer: "no", Primary: it.Default == "no"},
		}
	default: // notify
		return []wireChoice{{Label: "Dismiss", Key: "d", Verb: "dismiss"}}
	}
}

// --- routing -----------------------------------------------------------------

// routeAsk decides whether the daemon's current item is answered in a
// conversation rather than a card, and remembers it if so. The daemon presents
// one item at a time, so there is at most one inline ask on screen and every call
// replaces whatever the last one left - including with nothing, which is how the
// panel disappears once the item resolves.
func (s *sessions) routeAsk(v daemon.View, appOpen bool) bool {
	s.mu.Lock()
	had := s.ask.Item != nil
	s.ask = daemon.View{}
	shown := false
	if v.Item != nil {
		shown = s.shownLocked(v.Item.Identity.Session)
	}
	took := inlineRoutable(v.Item, shown, appOpen)
	if took {
		s.ask = v
	}
	s.mu.Unlock()

	if took || had {
		s.touch()
	}
	return took
}

// pendingAsk is the question currently routed into a conversation, if any.
func (s *sessions) pendingAsk() (daemon.View, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ask, s.ask.Item != nil
}

// askKey applies one keystroke to the inline ask. The surface sends the key and
// Go decides what it means, through the inbox's table, so the panel cannot invent
// a shortcut the rest of agentbox does not have. Reports whether the key meant
// anything, so the surface can swallow it or let it through.
func (s *sessions) askKey(id, key string) bool {
	v, ok := s.pendingAsk()
	if !ok || v.Item.ID != id {
		return false
	}
	switch cmd := triageFor(store.StoredItem{Item: *v.Item, State: store.StatePending}, key); cmd.intent {
	case triageAnswer:
		s.ui.res.Answer(id, cmd.answer)
	case triageDismiss:
		s.ui.res.Dismiss(id)
	default:
		return false
	}
	return true
}

// attachAsk hangs the routed question on the session it belongs to. It rides the
// session payload rather than an event of its own because it is part of that
// conversation's state: one push, and a switcher row can mark itself waiting
// without a second subscription.
func (s *sessions) attachAsk(list []wireSession, ask daemon.View) []wireSession {
	if ask.Item == nil {
		return list
	}
	sid := ask.Item.Identity.Session
	enc := s.ui.encodeAsk(ask)
	for i := range list {
		if list[i].ID == sid {
			list[i].Ask = enc
		}
	}
	return list
}

// rerouteAsk re-presents the current item now that the conversation which was
// going to answer it has gone away - the app window closed. Present runs the
// routing rule again, sees no window, and gives the item its card.
func (u *UI) rerouteAsk() {
	if _, ok := u.sess.pendingAsk(); !ok {
		return
	}
	u.mu.Lock()
	v := u.cur
	u.mu.Unlock()
	u.Present(v)
}
