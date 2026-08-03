package daemon

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

func newTestAloud(snd Sounder) *aloud {
	return newAloud(snd, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestAloudReadsARegionAsOneUtterance(t *testing.T) {
	// The FR72 contract. The first version split a region into a passage per
	// paragraph so it could offer a position between them, and that split is what
	// broke the feature: an engine emits nothing until it has finished a line, so
	// the wait timed out before the audio existed and every control acted on a
	// position tens of seconds ahead of the sound. One region, one utterance.
	snd := &fakeSound{}
	a := newTestAloud(snd)
	text := "First paragraph here.\n\nSecond paragraph here.\n\nThird paragraph here."

	res := a.Command(proto.AloudStart, "intro", text)
	if !res.Playing || res.Region != "intro" {
		t.Fatalf("start reported playing=%v region=%q", res.Playing, res.Region)
	}
	waitFor(t, "the reading to finish", func() bool { return !a.state().Playing })

	spoken := snd.waitedLines()
	if len(spoken) != 1 {
		t.Fatalf("three paragraphs became %d utterances, wanted 1: %q", len(spoken), spoken)
	}
	want := "First paragraph here. Second paragraph here. Third paragraph here."
	if spoken[0] != want {
		t.Errorf("the region was altered\n got %q\nwant %q", spoken[0], want)
	}
}

func TestAloudReadsPastTheNotificationCap(t *testing.T) {
	// A region longer than the sentence cap must arrive whole: the reader uses the
	// reading path precisely so nothing is truncated.
	snd := &fakeSound{}
	a := newTestAloud(snd)
	long := strings.TrimSpace(strings.Repeat("The guard keeps the write additive. ", 20))

	a.Command(proto.AloudStart, "intro", long)
	waitFor(t, "the reading to finish", func() bool { return !a.state().Playing })

	spoken := snd.waitedLines()
	if len(spoken) != 1 {
		t.Fatalf("one region became %d utterances", len(spoken))
	}
	if want := strings.Join(strings.Fields(long), " "); spoken[0] != want {
		t.Errorf("the region was altered\n got %q\nwant %q", spoken[0], want)
	}
}

func TestAloudReportsWhichRegionIsPlaying(t *testing.T) {
	// The surface has a control per region, so it needs to know which one the voice
	// is on - painting them all as playing would be a lie about three of them.
	snd := &fakeSound{hold: make(chan struct{})}
	a := newTestAloud(snd)

	a.Command(proto.AloudStart, "lead:1", "The block below writes nothing until the chunk is whole.")
	waitFor(t, "playing", func() bool { return a.state().Playing })
	if got := a.state().Region; got != "lead:1" {
		t.Errorf("state reports region %q while reading lead:1", got)
	}

	// Starting another region replaces the first: one voice, one reading.
	a.Command(proto.AloudStart, "close", "That is the whole design.")
	if got := a.state().Region; got != "close" {
		t.Errorf("starting close left the region as %q", got)
	}
	if snd.stopCount() == 0 {
		t.Error("starting a second region did not silence the first")
	}
}

func TestAloudStopSilencesAndClearsTheRegion(t *testing.T) {
	snd := &fakeSound{hold: make(chan struct{})}
	a := newTestAloud(snd)
	a.Command(proto.AloudStart, "intro", "Passage one. Passage two.")
	waitFor(t, "playing", func() bool { return a.state().Playing })

	res := a.Command(proto.AloudStop, "", "")
	if res.Playing || res.Region != "" {
		t.Errorf("stop left playing=%v region=%q; both must be clear", res.Playing, res.Region)
	}
	if snd.stopCount() == 0 {
		t.Error("stop did not silence the voice")
	}

	// And nothing more may be said after a stop.
	before := len(snd.waitedLines())
	time.Sleep(50 * time.Millisecond)
	if after := len(snd.waitedLines()); after != before {
		t.Errorf("kept speaking after stop: %d lines became %d", before, after)
	}
}

func TestAloudStateOnlyReportsAndUnknownActionsDoTheSame(t *testing.T) {
	// The surface polls state to notice a reading that ended on its own, so state
	// must not touch anything. An action nobody recognises reports state rather
	// than failing: a control the human can see must not be able to error.
	snd := &fakeSound{hold: make(chan struct{})}
	a := newTestAloud(snd)
	a.Command(proto.AloudStart, "intro", "Passage one. Passage two.")
	waitFor(t, "playing", func() bool { return a.state().Playing })

	stops := snd.stopCount()
	if res := a.Command(proto.AloudState, "", ""); !res.OK || !res.Playing || res.Region != "intro" {
		t.Errorf("state reported ok=%v playing=%v region=%q", res.OK, res.Playing, res.Region)
	}
	if res := a.Command("sideways", "", ""); !res.OK {
		t.Error("an unknown action should report state, not fail")
	}
	if snd.stopCount() != stops {
		t.Error("asking for state stopped the reading")
	}
}

func TestAloudFinishedClearsTheRegionSoTheControlResets(t *testing.T) {
	// A reading that ends on its own has to stop claiming a region, or the surface
	// would paint a play button as playing for ever.
	snd := &fakeSound{}
	a := newTestAloud(snd)
	a.Command(proto.AloudStart, "close", "That is the whole design.")
	waitFor(t, "the reading to finish", func() bool { return !a.state().Playing })
	if got := a.state().Region; got != "" {
		t.Errorf("a finished reading still claims region %q", got)
	}
}

func TestAloudStartOnEmptyTextPlaysNothing(t *testing.T) {
	// The daemon rejects an empty start, but punctuation-only text reaches here
	// and cleans to nothing. It must not leave a control claiming to play.
	snd := &fakeSound{}
	a := newTestAloud(snd)
	res := a.Command(proto.AloudStart, "intro", " \n\t ")
	if res.Playing || res.Region != "" {
		t.Errorf("empty text produced playing=%v region=%q", res.Playing, res.Region)
	}
}
