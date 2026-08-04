package webui

import (
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
)

func TestSpecForRuntime(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		runtime  string
		react    bool
		tailwind bool
	}{
		{
			name:     "claude.ai react module",
			src:      "import React, { useState } from \"react\";\n\nexport default function App() {\n  return <div className=\"flex items-center gap-2 p-4\">hi</div>;\n}\n",
			runtime:  "react",
			react:    true,
			tailwind: true,
		},
		{
			name:     "component export without an import",
			src:      "export default function Panel() {\n  return <Chart data={[1,2]} />;\n}\n",
			runtime:  "react",
			react:    true,
			tailwind: true,
		},
		{
			name:    "plain document with its own css",
			src:     "<!doctype html><html><head><style>body{font:14px}</style></head><body><h1>Report</h1></body></html>",
			runtime: "html",
		},
		{
			name:    "fragment",
			src:     "<div id=\"app\"></div>\n<script>document.title = \"x\";</script>",
			runtime: "html",
		},
		{
			name:     "document that mentions tailwind",
			src:      "<!doctype html><html><head><script src=\"https://cdn.tailwindcss.com\"></script></head><body></body></html>",
			runtime:  "html",
			tailwind: true,
		},
		{
			name:     "document styled with utility classes",
			src:      "<div class=\"flex items-center gap-4 p-6\"><span class=\"text-sm\">hi</span></div>",
			runtime:  "html",
			tailwind: true,
		},
		{
			name:    "document with its own class names",
			src:     "<div class=\"report\"><span class=\"lead\">hi</span></div>",
			runtime: "html",
		},
		{
			name:    "react built through a cdn tag stays a document",
			src:     "<!doctype html><html><body><div id=\"root\"></div><script>ReactDOM.render(React.createElement(\"p\"), root)</script></body></html>",
			runtime: "html",
			react:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := specFor(tc.src)
			if got.Runtime != tc.runtime {
				t.Errorf("runtime = %q, want %q", got.Runtime, tc.runtime)
			}
			if got.React != tc.react {
				t.Errorf("react = %v, want %v", got.React, tc.react)
			}
			if got.Tailwind != tc.tailwind {
				t.Errorf("tailwind = %v, want %v", got.Tailwind, tc.tailwind)
			}
		})
	}
}

func TestIsArtifactFence(t *testing.T) {
	cases := []struct {
		lang, src string
		want      bool
	}{
		// The word is a request, whatever is in the block.
		{"artifact", "<p>hi</p>", true},
		{"artifact", "export default function A() { return <p/> }", true},
		// A document or something with behaviour runs.
		{"html", "<!doctype html><html><body>hi</body></html>", true},
		{"html", "<div id=\"app\"></div><script>go()</script>", true},
		// Markup an agent is explaining stays a code block, which is what it is.
		{"html", "<table>\n  <tr><td>a</td></tr>\n</table>", false},
		{"html", "<div class=\"card\">the wrapper</div>", false},
		// Nothing else runs, however React-shaped it looks.
		{"jsx", "export default function A() { return <p/> }", false},
		{"go", "func main() {}", false},
		{"", "plain", false},
	}
	for _, tc := range cases {
		if got := isArtifactFence(tc.lang, tc.src); got != tc.want {
			t.Errorf("isArtifactFence(%q, %.30q) = %v, want %v", tc.lang, tc.src, got, tc.want)
		}
	}
}

func TestArtifactBlockCarriesSourceAndSpec(t *testing.T) {
	src := "export default function App() {\n  return <b>x &amp; y</b>;\n}\n"
	html := artifactBlock(src, "Deploy plan", "a1", false)

	for _, want := range []string{
		`class="k-artifact"`,
		`data-runtime="react"`,
		`data-react="1"`,
		`data-tailwind="1"`,
		`data-view="preview"`,
		`class="k-artifact-stage"`,
		`data-artifact-view="code"`,
		"Deploy plan",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("block is missing %q", want)
		}
	}

	// The source travels as escaped text: what the sandbox runs is exactly what
	// the code tab shows, and neither can end an attribute or a tag early.
	if !strings.Contains(html, "&lt;b&gt;x &amp;amp; y&lt;/b&gt;") {
		t.Errorf("source is not escaped into the block:\n%s", html)
	}
	if strings.Contains(html, "<b>x") {
		t.Error("raw agent markup reached the surface outside the sandbox")
	}
}

// A parameter panel is the same block with the routing mark: data-panel is what
// sends its emits to SetAssignmentParams instead of to a waiting agent, and the
// id is the assignment's, so the surface knows whose values arrived. An ordinary
// artifact must never carry the mark - it would silence its events.
func TestPanelBlockCarriesTheRoutingMark(t *testing.T) {
	src := "export default function Panel() { return <input />; }"

	panel := RenderPanel(src, "custom parameters", "a1b2c3")
	for _, want := range []string{`data-panel="1"`, `data-artifact-id="a1b2c3"`, `class="k-artifact-stage"`} {
		if !strings.Contains(panel, want) {
			t.Errorf("panel block is missing %q", want)
		}
	}
	if RenderPanel("  ", "t", "a1") != "" {
		t.Error("an assignment with no panel produced a block anyway")
	}
	if strings.Contains(RenderArtifact(src, "t", "a1"), "data-panel") {
		t.Error("an ordinary artifact carries the panel mark; its events would go nowhere")
	}
}

func TestArtifactRefusesSomethingHuge(t *testing.T) {
	src := "<div>" + strings.Repeat("x", artifactMaxBytes+1) + "</div>"
	html := artifactBlock(src, "", "", false)
	if !strings.Contains(html, `data-toobig="1"`) || !strings.Contains(html, `data-view="code"`) {
		t.Error("an oversized artifact should arrive as code, marked refused")
	}
	if strings.Contains(html, `class="k-artifact-stage"`) {
		t.Error("an oversized artifact should have no stage to run in")
	}
	if !strings.Contains(html, "larger than agentbox will run") {
		t.Error("a refusal should say why")
	}
}

func TestRenderArtifactIsJustTheBlock(t *testing.T) {
	if got := RenderArtifact("   ", "", ""); got != "" {
		t.Errorf("an empty artifact should render nothing, got %q", got)
	}
	// A fence sequence inside the source cannot end anything, because there is no
	// markdown pass around a standalone artifact.
	html := RenderArtifact("<p>```</p>", "Fences", "")
	if !strings.HasPrefix(html, `<div class="k-artifact"`) {
		t.Errorf("unexpected wrapper: %.60q", html)
	}
	if n := strings.Count(html, "k-artifact-src"); n != 1 {
		t.Errorf("the source should appear once, as text; found %d", n)
	}
	if !strings.Contains(html, "&lt;p&gt;```&lt;/p&gt;") {
		t.Errorf("the backticks ended something they should not have:\n%s", html)
	}
}

func TestArtifactFenceInMarkdown(t *testing.T) {
	html := RenderMarkdown("Here is one:\n\n```artifact\n<button onclick=\"go()\">Go</button>\n```\n")
	if !strings.Contains(html, `class="k-artifact"`) {
		t.Errorf("an artifact fence should hydrate:\n%s", html)
	}
	// And an ordinary html fence that is only markup stays a code block.
	code := RenderMarkdown("```html\n<td>cell</td>\n```\n")
	if strings.Contains(code, "k-artifact") {
		t.Errorf("a markup snippet should stay code:\n%s", code)
	}
	if !strings.Contains(code, "k-code") {
		t.Errorf("a markup snippet should still be a code block:\n%s", code)
	}
}

func TestViewerRunsAnArtifactAndReadsEverythingElse(t *testing.T) {
	v := &viewer{}
	v.load(proto.ShowRequest{Content: "<div class=\"flex p-2 gap-1\">hi</div>", Title: "Panel", Artifact: true})
	doc := v.snapshot()
	if !doc.Artifact {
		t.Error("an artifact request should mark the document as one")
	}
	if !strings.Contains(doc.HTML, "k-artifact") {
		t.Errorf("artifact request rendered as prose:\n%s", doc.HTML)
	}
	if doc.Title != "agentbox · Panel" {
		t.Errorf("title = %q", doc.Title)
	}

	v.load(proto.ShowRequest{Content: "# Heading", Title: "Doc"})
	if doc := v.snapshot(); doc.Artifact || !strings.Contains(doc.HTML, "<h1") {
		t.Errorf("a document should still be read as markdown: %+v", doc)
	}
}

func TestArtifactRequestThatCannotBeReadFallsBackToProse(t *testing.T) {
	v := &viewer{}
	v.load(proto.ShowRequest{Path: "/nonexistent/agentbox-artifact.html", Artifact: true})
	doc := v.snapshot()
	if doc.Artifact {
		t.Error("a failure is a sentence, not a program: it must not be marked as an artifact")
	}
	if !strings.Contains(doc.HTML, "Cannot open") {
		t.Errorf("the failure should be readable:\n%s", doc.HTML)
	}
}

func TestArtifactTitleFallback(t *testing.T) {
	if got := docTitle(proto.ShowRequest{Artifact: true}); got != "agentbox · artifact" {
		t.Errorf("docTitle = %q", got)
	}
}

// The way out of the sandbox, from the Go side. The sender is agent-authored code
// in an opaque origin, so this is the boundary where nothing is taken on trust.
func TestArtifactEventValidation(t *testing.T) {
	ok := []struct {
		name, id, data string
	}{
		{"run", "a1", `{"rows":500}`},
		{"cell:click", "a1", `[1,2,3]`},
		{"batch-2", "", `42`},
		{"submit", "a1", ""},     // a button: the name is the whole message
		{"submit", "a1", "null"}, // and null is the same thing
	}
	for _, tc := range ok {
		ev, valid := artifactEvent(tc.id, tc.name, tc.data)
		if !valid {
			t.Errorf("artifactEvent(%q, %q, %q) was rejected", tc.id, tc.name, tc.data)
			continue
		}
		if ev.Name != tc.name || ev.ArtifactID != tc.id {
			t.Errorf("event = %+v", ev)
		}
		if ev.AtMS == 0 {
			t.Error("an event should be stamped where it arrives")
		}
		if tc.data == "" || tc.data == "null" {
			if len(ev.Data) != 0 {
				t.Errorf("no payload should mean no data, got %s", ev.Data)
			}
		} else if string(ev.Data) != tc.data {
			t.Errorf("data = %s, want %s", ev.Data, tc.data)
		}
	}

	bad := []struct {
		why, id, name, data string
	}{
		{"no name", "a1", "", `{}`},
		{"blank name", "a1", "   ", `{}`},
		{"a name with spaces", "a1", "run it", `{}`},
		{"a name with markup", "a1", "<script>", `{}`},
		{"a name too long", "a1", strings.Repeat("n", artifactNameMax+1), `{}`},
		{"an id that is not a token", "a 1", "run", `{}`},
		{"data that is not json", "a1", "run", `{oops}`},
		{"data too big", "a1", "run", `"` + strings.Repeat("x", artifactDataMax) + `"`},
	}
	for _, tc := range bad {
		if _, valid := artifactEvent(tc.id, tc.name, tc.data); valid {
			t.Errorf("%s should have been rejected", tc.why)
		}
	}
}

// A conversation re-renders its whole HTML on every streamed token, so the id an
// artifact's events carry has to come from the artifact rather than from a counter.
func TestArtifactFenceIDIsStableForTheSameSource(t *testing.T) {
	src := "<button onclick=\"agentbox.emit('go')\">go</button>"
	first := artifactBlock(src, "", "", false)
	if first != artifactBlock(src, "", "", false) {
		t.Error("the same fence rendered twice produced two different blocks")
	}
	if artifactFenceID(src) == artifactFenceID(src+" ") {
		t.Error("two different artifacts share an id")
	}
	if !strings.Contains(first, `data-artifact-id="`+artifactFenceID(src)+`"`) {
		t.Errorf("the block does not carry its id:\n%s", first)
	}
	// A minted id wins: the caller that will wait on it chose the name.
	if !strings.Contains(artifactBlock(src, "", "a9f", false), `data-artifact-id="a9f"`) {
		t.Error("a minted id should be used as given")
	}
}
