package webui

import (
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/session"
)

// mdRenders reads the miss counter, so a test asserts on the mechanism rather than
// on how fast this machine happens to be.
func mdRenders() int {
	mdCache.Lock()
	defer mdCache.Unlock()
	return mdCache.renders
}

// R-17's other half. encodeTurns runs on every push for the selected session, up to
// fifteen a second while a reply streams, and it used to send every segment of the
// whole conversation through goldmark each time. The streaming tail is the only part
// that changes.
func TestTranscriptRendersEachSegmentOnce(t *testing.T) {
	turns := []session.Turn{
		{Role: session.RoleUser, Segments: []session.Segment{
			{Kind: session.SegText, Text: "why does `make check` fail? " + strings.Repeat("x", 40)},
		}},
		{Role: session.RoleAssistant, Segments: []session.Segment{
			{Kind: session.SegThinking, Text: "the *race* detector is on, " + strings.Repeat("y", 40)},
			{Kind: session.SegText, Text: "Because of a **data race**, " + strings.Repeat("z", 40)},
			{Kind: session.SegToolUse, ToolName: "Bash", ToolInput: "go test ./... # " + strings.Repeat("q", 40)},
		}},
	}

	before := mdRenders()
	first := encodeTurns(turns)
	firstCost := mdRenders() - before
	if firstCost == 0 {
		t.Fatal("nothing was rendered at all, so this test is measuring the wrong thing")
	}

	// Fourteen more pushes of the same conversation, which is one second of
	// streaming with nothing new arriving in the part already on screen.
	for range 14 {
		encodeTurns(turns)
	}
	if extra := mdRenders() - before - firstCost; extra != 0 {
		t.Errorf("a second of pushes re-rendered %d segments; every one of them was unchanged", extra)
	}

	// And the output is the same, which is the thing the cache must not trade away.
	again := encodeTurns(turns)
	if segHTML(first) != segHTML(again) {
		t.Error("the cached render differs from the first one")
	}
}

// A streaming tail is a DIFFERENT string every push, so it must miss - the cache is
// content-addressed precisely so that a growing segment cannot serve a stale render.
func TestAGrowingSegmentIsNotServedStale(t *testing.T) {
	var last string
	for _, text := range []string{"Because", "Because of", "Because of a **race**"} {
		got := encodeTurns([]session.Turn{{Role: session.RoleAssistant, Segments: []session.Segment{
			{Kind: session.SegText, Text: text},
		}}})
		html := segHTML(got)
		if html == last {
			t.Errorf("%q rendered to the previous text's HTML", text)
		}
		if !strings.Contains(html, "Because") {
			t.Errorf("%q rendered as %q", text, html)
		}
		last = html
	}
}

// The budget is a byte ceiling with the whole map dropped when it is reached, so a
// conversation larger than the cache degrades to re-rendering rather than growing
// without limit.
func TestCacheStaysUnderItsBudget(t *testing.T) {
	big := strings.Repeat("prose that renders to rather more than it started as. ", 400)
	for i := range 500 {
		renderMarkdownCached(big + strings.Repeat("!", i))
	}

	mdCache.Lock()
	bytes, entries := mdCache.bytes, len(mdCache.at)
	mdCache.Unlock()
	if bytes > mdCacheBytes {
		t.Errorf("cache holds %d bytes, past its %d-byte budget", bytes, mdCacheBytes)
	}
	if entries == 0 {
		t.Error("the cache emptied itself and stayed empty")
	}
}
