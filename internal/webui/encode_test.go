package webui

import (
	"strings"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/session"
)

// The encoders are the whole contract between Go and the surfaces: everything a
// card or a conversation shows was decided here, and a Svelte file only paints
// what it is handed. They were the last untested part of internal/webui, which
// mattered because nothing renders a surface offscreen any more - a field that
// stopped being filled would be an empty region on a real screen and nothing
// else.
//
// What is worth pinning is the translation, not the plumbing: a time becoming
// zero rather than a 1970 countdown, an identity becoming a hue, a body becoming
// HTML, a tool result being cut before it reaches a window. The severity glyph
// and the sticky rule live in toast_test.go, which got there first.

func TestEncodeFillsTheCard(t *testing.T) {
	u := confUI()
	grace := time.Now().Add(3 * time.Second)
	expires := time.Now().Add(30 * time.Second)

	got := u.encode(daemon.View{
		Item: &proto.Item{
			ID: "a", Kind: proto.KindChoice, Level: proto.LevelWarning,
			Title: "Migrate?", Body: "This **rewrites** history.",
		},
		Waiting:        2,
		Graced:         true,
		GracedText:     "Answered: yes",
		GraceUntil:     grace,
		ExpiresAt:      expires,
		ActionsEnabled: true,
		Caller:         daemon.CallerLive,
	})

	if got.Item == nil || got.Item.ID != "a" {
		t.Fatalf("the item itself should travel: %+v", got.Item)
	}
	if !strings.Contains(got.BodyHTML, "<strong>rewrites</strong>") {
		t.Errorf("the body should arrive rendered, not as source: %q", got.BodyHTML)
	}
	if got.Waiting != 2 || !got.Graced || got.GracedText != "Answered: yes" {
		t.Errorf("view = %+v", got)
	}
	if got.GraceUntilMS != grace.UnixMilli() || got.ExpiresAtMS != expires.UnixMilli() {
		t.Errorf("deadlines = %d/%d, want %d/%d",
			got.GraceUntilMS, got.ExpiresAtMS, grace.UnixMilli(), expires.UnixMilli())
	}
	if !got.ActionsEnabled {
		t.Error("the FR32 kill switch has to reach the surface, or the buttons render anyway")
	}
}

// A zero time must encode as 0 rather than as an epoch millisecond: the surface
// treats 0 as "no deadline", and 1970 would count down from fifty-odd years ago.
func TestEncodeHasNoDeadlinesWhenThereAreNone(t *testing.T) {
	got := confUI().encode(daemon.View{Item: &proto.Item{ID: "a", Kind: proto.KindConfirm}})
	if got.GraceUntilMS != 0 || got.DismissAtMS != 0 || got.ExpiresAtMS != 0 {
		t.Errorf("deadlines = %+v, want all zero", got)
	}
}

// An empty view is what the surface gets when the queue drains, and it has to
// encode rather than panic: Present is called with no item on every resolve.
func TestEncodeEmptyView(t *testing.T) {
	got := confUI().encode(daemon.View{})
	if got.Item != nil || got.BodyHTML != "" || got.Glyph != "" {
		t.Errorf("view = %+v, want nothing to show", got)
	}
	if got.Caller != "none" {
		t.Errorf("caller = %q, want none", got.Caller)
	}
}

// The waiting dots are multi-agent legibility (03-ui-ux.md): one dot per queued
// item, in the agent's own stable hue, in queue order.
func TestEncodeWaitingHues(t *testing.T) {
	u := confUI()
	got := u.encode(daemon.View{
		Item: &proto.Item{ID: "a", Kind: proto.KindChoice},
		WaitingFrom: []proto.Identity{
			{Agent: "claude-code", Project: "agentbox"},
			{Agent: "nudge", Project: "ideas"},
			{Agent: "claude-code", Project: "agentbox"},
		},
	})

	if len(got.WaitingHues) != 3 {
		t.Fatalf("hues = %v, want one per queued item", got.WaitingHues)
	}
	if got.WaitingHues[0] != got.WaitingHues[2] {
		t.Error("the same identity must get the same hue, or the dots stop meaning anything")
	}
	if got.WaitingHues[0] == got.WaitingHues[1] {
		t.Error("two different agents should be told apart")
	}
	for _, h := range got.WaitingHues {
		if !strings.HasPrefix(h, "hsl(") {
			t.Errorf("hue = %q, want a colour the surface can use as-is", h)
		}
	}
	// The hue follows the theme, because a colour legible on dark is not on light.
	light := confUI()
	light.theme = Theme{Mode: "light"}
	if lit := light.encode(daemon.View{
		Item:        &proto.Item{ID: "a", Kind: proto.KindChoice},
		WaitingFrom: []proto.Identity{{Agent: "claude-code", Project: "agentbox"}},
	}); lit.WaitingHues[0] == got.WaitingHues[0] {
		t.Error("the identity hue should differ between dark and light")
	}
}

// FR45: the caller dot. The surface switches on a word, so the mapping is here
// and every state has one - an unknown state must read as "none" rather than as
// a live caller, because claiming a dead caller is alive is the wrong direction.
func TestCallerName(t *testing.T) {
	cases := map[daemon.CallerState]string{
		daemon.CallerLive:     "live",
		daemon.CallerGone:     "gone",
		daemon.CallerAwaiting: "awaiting",
		daemon.CallerNone:     "none",
	}
	for state, want := range cases {
		if got := callerName(state); got != want {
			t.Errorf("callerName(%v) = %q, want %q", state, got, want)
		}
	}
	if got := callerName(daemon.CallerState(99)); got != "none" {
		t.Errorf("an unknown state = %q, want none", got)
	}
}

func TestMSDropsTheZeroTime(t *testing.T) {
	if got := ms(time.Time{}); got != 0 {
		t.Errorf("ms(zero) = %d, want 0", got)
	}
	now := time.Now()
	if got := ms(now); got != now.UnixMilli() {
		t.Errorf("ms(now) = %d, want %d", got, now.UnixMilli())
	}
}

// cardHeight is an estimate - the surface measures itself - but a frameless
// window cannot be sloppy about it: empty space under a two-line question reads
// as a bug, and a card taller than the screen has nowhere to go. What is pinned
// is the direction (more content, more height) and the ceiling.
func TestCardHeightGrowsWithTheItem(t *testing.T) {
	u := confUI()
	base := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindNotify, Title: "done"}})

	cases := map[string]*proto.Item{
		"a body":             {Kind: proto.KindNotify, Title: "x", Body: "one\ntwo\nthree"},
		"three choices":      {Kind: proto.KindChoice, Title: "x", Options: []proto.Option{{Label: "a"}, {Label: "b"}, {Label: "c"}}},
		"a confirm":          {Kind: proto.KindConfirm, Title: "x"},
		"a text field":       {Kind: proto.KindText, Title: "x"},
		"a secret field":     {Kind: proto.KindSecret, Title: "x"},
		"a form":             {Kind: proto.KindForm, Title: "x", Fields: []proto.Field{{Label: "a"}, {Label: "b"}}},
		"a veto's countdown": {Kind: proto.KindVeto, Title: "x"},
		"a diff to review":   {Kind: proto.KindDiff, Title: "x", Diff: "@@ -1 +1 @@\n-a\n+b\n"},
	}
	for name, it := range cases {
		if h := u.cardHeight(daemon.View{Item: it}); h <= base {
			t.Errorf("%s: height %d, want more than the bare %d", name, h, base)
		}
	}

	// A fourth option wraps to a second row of buttons and needs the room.
	three := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindChoice,
		Options: []proto.Option{{Label: "a"}, {Label: "b"}, {Label: "c"}}}})
	four := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindChoice,
		Options: []proto.Option{{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"}}}})
	if four <= three {
		t.Errorf("four options (%d) should need more than three (%d)", four, three)
	}

	// A bigger patch needs a bigger pane, up to the point where the pane scrolls
	// instead - a 400-line patch and a 20-line one open the same window.
	small := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindDiff, Diff: "@@\n-a\n+b\n"}})
	medium := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindDiff, Diff: strings.Repeat("+line\n", 15)}})
	huge := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindDiff, Diff: strings.Repeat("+line\n", 400)}})
	twenty := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindDiff, Diff: strings.Repeat("+line\n", 20)}})
	if medium <= small {
		t.Errorf("a 15-line patch (%d) should need more room than a 3-line one (%d)", medium, small)
	}
	if huge != twenty {
		t.Errorf("400 lines asked for %d and 20 for %d; past the cap the pane scrolls", huge, twenty)
	}

	// Multiline text asks for a taller box than a single line.
	single := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindText}})
	multi := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindText, Multiline: true}})
	if multi <= single {
		t.Errorf("a multiline field (%d) should be taller than a single line (%d)", multi, single)
	}
}

func TestCardHeightIsBounded(t *testing.T) {
	u := confUI()
	_, maxH := u.cardGeom()

	// A body stops adding height at the 12-line cap and scrolls inside the card,
	// so 400 lines ask for exactly what 12 do.
	line := "a line of body text that goes on and on and on\n"
	long := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindText, Body: strings.Repeat(line, 400)}})
	twelve := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindText, Body: strings.Repeat(line, 12)}})
	if long != twelve {
		t.Errorf("400 lines asked for %d and 12 for %d; the cap is what keeps a paste inside the card", long, twelve)
	}
	if long > maxH {
		t.Errorf("height = %d, want <= the configured %d", long, maxH)
	}

	// A form with a field per line of a questionnaire is the other way to overflow.
	fields := make([]proto.Field, 60)
	if h := u.cardHeight(daemon.View{Item: &proto.Item{Kind: proto.KindForm, Fields: fields}}); h > maxH {
		t.Errorf("a 60-field form = %d, want <= %d", h, maxH)
	}

	// No item at all is the empty card, which has a fixed size and no estimate.
	if h := u.cardHeight(daemon.View{}); h != cardH {
		t.Errorf("an empty view = %d, want the default %d", h, cardH)
	}
}

func TestCardHeight2TakesTheWidthFromConfig(t *testing.T) {
	u := confUI()
	wantW, _ := u.cardGeom()
	v := daemon.View{Item: &proto.Item{Kind: proto.KindConfirm, Title: "x"}}
	w, h := u.cardHeight2(v)
	if w != wantW {
		t.Errorf("width = %d, want the configured %d", w, wantW)
	}
	if h != u.cardHeight(v) {
		t.Errorf("height = %d, want cardHeight's %d", h, u.cardHeight(v))
	}
}

func TestCountLinesWrapsAndCaps(t *testing.T) {
	cases := []struct {
		name string
		body string
		per  int
		want int
	}{
		{"nothing", "", 10, 0},
		{"one short line", "hello", 10, 1},
		{"a hard break", "one\ntwo", 10, 2},
		{"a trailing break opens a line", "one\n", 10, 2},
		{"wrapping at the measure", strings.Repeat("x", 25), 10, 3},
		// The cap is what keeps a 5000-line paste from asking for a window taller
		// than the display; the body scrolls inside the card instead.
		{"the cap", strings.Repeat("x\n", 500), 10, 12},
	}
	for _, c := range cases {
		if got := countLines(c.body, c.per); got != c.want {
			t.Errorf("%s: countLines = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestRowsForButtons(t *testing.T) {
	// Three buttons to a row, so 4 needs two rows and 7 needs three.
	cases := map[int]int{0: 1, 1: 1, 3: 1, 4: 2, 6: 2, 7: 3, 9: 3}
	for n, want := range cases {
		if got := rowsFor(n); got != want {
			t.Errorf("rowsFor(%d) = %d, want %d", n, got, want)
		}
	}
}

// --- the conversation --------------------------------------------------------

// encodeTurns runs on every stream push (~16Hz) for the selected session, so it
// is both the hottest encoder and the one whose output is a whole screen.
func TestEncodeTurns(t *testing.T) {
	turns := []session.Turn{
		{Role: session.RoleUser, Segments: []session.Segment{
			{Kind: session.SegText, Text: "why does `make check` fail?"},
		}},
		{Role: session.RoleAssistant, Model: "claude-opus-5", CostUSD: 0.0123, Segments: []session.Segment{
			{Kind: session.SegThinking, Text: "the *race* detector is on"},
			{Kind: session.SegText, Text: "Because of a **data race**."},
			{Kind: session.SegToolUse, ToolName: "Bash", ToolInput: "go test ./...", Result: "ok", HasResult: true},
			{Kind: session.SegToolResult, Result: "exit 1", HasResult: true, IsError: true},
		}},
	}

	got := encodeTurns(turns)
	if len(got) != 2 {
		t.Fatalf("turns = %d, want 2", len(got))
	}

	user := got[0]
	if user.Role != "user" {
		t.Errorf("role = %q, want user", user.Role)
	}
	if len(user.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(user.Segments))
	}
	// A user's prompt shows as typed, so the source travels beside the HTML. The
	// two are not interchangeable: stripping tags leaves the entities behind.
	if user.Segments[0].Text != "why does `make check` fail?" {
		t.Errorf("the user's own words = %q", user.Segments[0].Text)
	}
	if !strings.Contains(user.Segments[0].HTML, "<code>make check</code>") {
		t.Errorf("html = %q, want the markdown rendered too", user.Segments[0].HTML)
	}

	a := got[1]
	if a.Model != "claude-opus-5" || a.CostUSD != 0.0123 {
		t.Errorf("turn = %+v, want the model and cost carried", a)
	}
	if len(a.Segments) != 4 {
		t.Fatalf("segments = %d, want 4", len(a.Segments))
	}
	kinds := []string{"thinking", "text", "tool", "result"}
	for i, want := range kinds {
		if a.Segments[i].Kind != want {
			t.Errorf("segment %d kind = %q, want %q", i, a.Segments[i].Kind, want)
		}
	}
	if !strings.Contains(a.Segments[0].HTML, "<em>race</em>") {
		t.Errorf("thinking renders as markdown too: %q", a.Segments[0].HTML)
	}
	// Only a user turn carries its source; an agent's prose is read as rendered.
	if a.Segments[1].Text != "" {
		t.Errorf("an assistant segment should not carry source: %q", a.Segments[1].Text)
	}
	tool := a.Segments[2]
	if tool.ToolName != "Bash" || tool.ToolInput != "go test ./..." || !tool.HasResult {
		t.Errorf("tool segment = %+v", tool)
	}
	if tool.HTML != "" {
		t.Errorf("a tool call is a chip, not prose: %q", tool.HTML)
	}
	if !a.Segments[3].IsError {
		t.Error("an error result has to be marked, or it reads as an ordinary one")
	}
}

// A turn with no segments is what the stream opens an assistant reply with, and
// the surface skips it (no stray header) - but it must survive the encoding.
func TestEncodeTurnsEmpty(t *testing.T) {
	if got := encodeTurns(nil); got == nil || len(got) != 0 {
		t.Errorf("encodeTurns(nil) = %#v, want an empty list and not null", got)
	}
	got := encodeTurns([]session.Turn{{Role: session.RoleAssistant}})
	if len(got) != 1 || len(got[0].Segments) != 0 {
		t.Errorf("turns = %+v, want one turn with no segments", got)
	}
}

// A system turn is the init banner or an error notice, and its text is the error.
func TestEncodeTurnsSystemError(t *testing.T) {
	got := encodeTurns([]session.Turn{{
		Role: session.RoleSystem, Err: "claude: exit status 1",
		Segments: []session.Segment{{Kind: session.SegText, Text: "claude: exit status 1"}},
	}})
	if got[0].Role != "system" || got[0].Err != "claude: exit status 1" {
		t.Errorf("turn = %+v", got[0])
	}
}

// A tool that prints a megabyte is cut before it reaches a window: the whole
// conversation is re-encoded on every push, and nobody reads a megabyte in a chip.
func TestEncodeTurnsTrimsAToolResult(t *testing.T) {
	huge := strings.Repeat("x", 12000)
	got := encodeTurns([]session.Turn{{Role: session.RoleAssistant, Segments: []session.Segment{
		{Kind: session.SegToolUse, ToolName: "Bash", Result: huge, HasResult: true},
	}}})

	res := got[0].Segments[0].Result
	if len(res) >= len(huge) {
		t.Fatalf("result kept %d bytes of %d", len(res), len(huge))
	}
	if !strings.HasSuffix(res, "truncated") {
		t.Errorf("a cut result should say so: %q", res[max(0, len(res)-40):])
	}
	short := "ok\n"
	if got := trim(short, 4000); got != short {
		t.Errorf("trim left a short result alone as %q", got)
	}
}

// countDiffLines is what sizes a review card. It counts lines and nothing else:
// the diff pane is monospaced and scrolls sideways, so a long line is one line.
func TestCountDiffLines(t *testing.T) {
	for in, want := range map[string]int{
		"":                        0,
		"   \n\n":                 0,
		"one line":                1,
		"one line\n":              1,
		"a\nb\nc":                 3,
		"a\nb\nc\n":               3,
		strings.Repeat("+x\n", 9): 9,
	} {
		if got := countDiffLines(in); got != want {
			t.Errorf("countDiffLines(%q) = %d, want %d", in, got, want)
		}
	}
	long := strings.Repeat("x", 500)
	if got := countDiffLines(long); got != 1 {
		t.Errorf("a 500-character line counted as %d lines, want 1", got)
	}
}

// Claude Code sends one assistant message per tool call, so an answer that ran
// five commands before writing a word arrived as six turns - and the surface drew
// its identity pill and its clock on every one of them. Six pills for one reply is
// what Boris saw. A run of consecutive assistant messages is ONE turn to read.
func TestConsecutiveAssistantMessagesAreOneTurn(t *testing.T) {
	at := time.Date(2026, 7, 25, 17, 53, 0, 0, time.Local)
	turns := []session.Turn{
		{Role: session.RoleUser, At: at, Segments: []session.Segment{{Kind: session.SegText, Text: "go"}}},
		{Role: session.RoleAssistant, At: at, Model: "opus",
			Segments: []session.Segment{{Kind: session.SegToolUse, ToolName: "Bash", ToolInput: "ls"}}},
		{Role: session.RoleAssistant, At: at.Add(30 * time.Second),
			Segments: []session.Segment{{Kind: session.SegToolUse, ToolName: "Write", ToolInput: "a.md"}}},
		{Role: session.RoleAssistant, At: at.Add(35 * time.Second), CostUSD: 0.02,
			Think:    35 * time.Second,
			Segments: []session.Segment{{Kind: session.SegText, Text: "done"}}},
		{Role: session.RoleUser, At: at.Add(time.Minute),
			Segments: []session.Segment{{Kind: session.SegText, Text: "again"}}},
	}

	got := encodeTurns(turns)
	if len(got) != 3 {
		roles := make([]string, len(got))
		for i, g := range got {
			roles[i] = g.Role
		}
		t.Fatalf("encoded %d turns (%v), want 3: prompt, one agent turn, prompt", len(got), roles)
	}
	agent := got[1]
	if len(agent.Segments) != 3 {
		t.Errorf("the agent turn has %d segments, want all 3 of the run", len(agent.Segments))
	}
	if agent.At != "17:53" {
		t.Errorf("at = %q, want the moment the reply started (17:53)", agent.At)
	}
	if agent.Think != "35s" {
		t.Errorf("think = %q, want the 35s from the message that produced text", agent.Think)
	}
	if agent.Model != "opus" {
		t.Errorf("model = %q, want the run's model", agent.Model)
	}
	if agent.CostUSD != 0.02 {
		t.Errorf("cost = %v, want the run's 0.02", agent.CostUSD)
	}

	// A user turn between two agent turns still separates them.
	split := encodeTurns([]session.Turn{
		{Role: session.RoleAssistant, Segments: []session.Segment{{Kind: session.SegText, Text: "one"}}},
		{Role: session.RoleUser, Segments: []session.Segment{{Kind: session.SegText, Text: "and"}}},
		{Role: session.RoleAssistant, Segments: []session.Segment{{Kind: session.SegText, Text: "two"}}},
	})
	if len(split) != 3 {
		t.Errorf("a prompt between two replies merged them: %d turns", len(split))
	}
}
