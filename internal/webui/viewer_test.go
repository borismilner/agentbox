package webui

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/borismilner/agentbox/internal/proto"
)

func testViewer() *viewer {
	return newViewer(&UI{log: slog.New(slog.NewTextHandler(io.Discard, nil)), theme: Theme{Mode: "dark"}})
}

func TestDocTitle(t *testing.T) {
	cases := []struct {
		req  proto.ShowRequest
		want string
	}{
		{proto.ShowRequest{Title: "Release notes"}, "agentbox · Release notes"},
		{proto.ShowRequest{Title: "  Release notes  "}, "agentbox · Release notes"},
		// A title beats a path, and a bare filename beats an absolute path you
		// cannot read at a glance in a title bar.
		{proto.ShowRequest{Title: "Plan", Path: "/home/x/notes/plan.md"}, "agentbox · Plan"},
		{proto.ShowRequest{Path: "/home/x/notes/plan.md"}, "agentbox · plan.md"},
		{proto.ShowRequest{Content: "# inline"}, "agentbox · document"},
		{proto.ShowRequest{}, "agentbox · document"},
	}
	for _, c := range cases {
		if got := docTitle(c.req); got != c.want {
			t.Errorf("docTitle(%+v) = %q, want %q", c.req, got, c.want)
		}
	}
}

func TestViewerLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Title\n\nSome **prose** and `code`.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	v := testViewer()
	v.load(proto.ShowRequest{Path: path, Title: "Doc", Watch: true})
	got := v.snapshot()

	if got.Empty {
		t.Fatal("a loaded document is not empty")
	}
	if !strings.Contains(got.HTML, "<h1>Title</h1>") || !strings.Contains(got.HTML, "<strong>prose</strong>") {
		t.Errorf("html = %q, want the markdown rendered", got.HTML)
	}
	if got.Title != "agentbox · Doc" || got.Path != path || !got.Watch {
		t.Errorf("snapshot = %+v", got)
	}
	if got.RevMS == 0 {
		t.Error("a revision stamp is what lets a reload keep the scroll position")
	}
	if got.Error != "" {
		t.Errorf("error = %q, want none", got.Error)
	}
}

// The one surface with an honest base for a relative image path (images.go): a
// document read off disk resolves `![](plot.png)` against its own directory, and
// content with no file behind it still cannot.
func TestViewerResolvesImagesAgainstTheDocument(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plot.png"), pngBytes(t), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "# Report\n\n![a plot](plot.png)\n"
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	v := testViewer()
	v.load(proto.ShowRequest{Path: path})
	if got := v.snapshot().HTML; !strings.Contains(got, `src="data:image/png;base64,`) {
		t.Errorf("the image beside the document should be inlined:\n%s", got)
	}

	v.load(proto.ShowRequest{Content: body})
	if got := v.snapshot().HTML; !strings.Contains(got, `data-reason="relative"`) {
		t.Errorf("the same markdown with no file behind it has no base:\n%s", got)
	}
}

// Inline content is the `agentbox show -` path: no file, nothing to watch.
func TestViewerLoadInline(t *testing.T) {
	v := testViewer()
	v.load(proto.ShowRequest{Content: "plain body", Watch: true})
	got := v.snapshot()

	if !strings.Contains(got.HTML, "plain body") {
		t.Errorf("html = %q", got.HTML)
	}
	if got.Watch {
		t.Error("there is no file to watch, so the surface must not claim to be watching")
	}
}

// A file that cannot be read has to say so in the window: the reader is the only
// thing on screen, so an empty page would be the whole error message.
func TestViewerLoadMissingFile(t *testing.T) {
	v := testViewer()
	v.load(proto.ShowRequest{Path: filepath.Join(t.TempDir(), "gone.md")})
	got := v.snapshot()

	if got.Error == "" {
		t.Error("the failure should reach the surface")
	}
	if !strings.Contains(got.HTML, "Cannot open") {
		t.Errorf("html = %q, want an explanation in the document", got.HTML)
	}
}

// Content given alongside a path is the fallback when the path cannot be read
// (the client resolved it, the daemon may not see the same filesystem).
func TestViewerFallsBackToContent(t *testing.T) {
	v := testViewer()
	v.load(proto.ShowRequest{Path: filepath.Join(t.TempDir(), "gone.md"), Content: "# from stdin"})
	got := v.snapshot()

	if got.Error != "" {
		t.Errorf("error = %q: content was supplied, so nothing failed for the reader", got.Error)
	}
	if !strings.Contains(got.HTML, "from stdin") {
		t.Errorf("html = %q", got.HTML)
	}
}

func TestViewerEmptyUntilShown(t *testing.T) {
	got := testViewer().snapshot()
	if !got.Empty {
		t.Error("a viewer nobody has shown anything in is empty")
	}
	if got.Title == "" {
		t.Error("even the empty state needs a window title")
	}
}

// The watch loop is what makes --watch a live preview (FR37). It is driven here
// without a window by taking the window generation the loop guards on, so the
// reload path is exercised rather than assumed.
func TestViewerWatchReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live.md")
	if err := os.WriteFile(path, []byte("# one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	v := testViewer()
	v.load(proto.ShowRequest{Path: path, Watch: true})

	// Stand in for an open window: the loop retires when winGen moves or the
	// window goes, and there is no window in a test.
	v.mu.Lock()
	v.winGen = 1
	v.win = nil
	v.mu.Unlock()

	// With no window the loop must retire rather than poll a file forever.
	done := make(chan struct{})
	go func() { v.watch(1); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the watch loop should retire with its window")
	}

	// Now reload by hand, which is what the loop does on an mtime change, and
	// check the revision moves so the surface knows to keep its scroll.
	before := v.snapshot()
	time.Sleep(2 * time.Millisecond)
	if err := os.WriteFile(path, []byte("# two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v.load(proto.ShowRequest{Path: path, Watch: true})
	after := v.snapshot()

	if !strings.Contains(after.HTML, "two") {
		t.Errorf("html = %q, want the new content", after.HTML)
	}
	if after.RevMS < before.RevMS {
		t.Error("the revision stamp must move forward on a reload")
	}
}

// The loop's own reload branch, driven for real: an mtime that moves must put
// the new content in the snapshot without anyone calling load. The test stands a
// window up so the loop does not retire (nothing dereferences it - the loop only
// asks whether one is there, and emit tolerates a UI with no application).
func TestViewerWatchLoopReloadsOnMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live.md")
	if err := os.WriteFile(path, []byte("# one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	v := testViewer()
	v.load(proto.ShowRequest{Path: path, Watch: true})
	v.mu.Lock()
	v.win = &application.WebviewWindow{}
	v.winGen = 1
	v.mu.Unlock()

	go v.watch(1)
	defer func() {
		v.mu.Lock()
		v.win = nil
		v.mu.Unlock()
	}()

	// A whole second past the load, so the mtime is unambiguously newer even on a
	// filesystem with coarse timestamps.
	time.Sleep(time.Second)
	if err := os.WriteFile(path, []byte("# two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		if strings.Contains(v.snapshot().HTML, "two") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the watch loop never picked up the edit; html = %q", v.snapshot().HTML)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
