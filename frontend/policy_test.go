package frontend

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The surfaces' half of the two bargains, checked against what actually ships.
//
// internal/webui has tests for everything Go decides. Nothing checked the other
// side: the Content-Security-Policies, the iframe sandbox, and the rule that a
// surface only ever injects HTML that Go produced. Those live in `.svelte` and
// `.js` files with no test runner behind them, and the built bundle they compile
// into is committed - so a policy could be edited out, or the bundle could go
// stale against its source, and `make check` would stay green.
//
// This asserts them where it matters most: in `dist`, the bytes `go:embed` puts in
// the binary. A source-only check would pass while the shipped bundle carried
// something else. Where the source is also checked it is so the failure names the
// file to fix.
//
// What this deliberately does NOT do is run any of it. Asserting that
// `buildDocument` behaves would need a JS test runner and a DOM (it uses
// DOMParser), which is a toolchain decision rather than a test - noted in
// docs/STATUS.md as the next step. Policy is a constant, and a constant can be
// checked as one.

// distFile reads one path out of the embedded bundle - the same bytes the webview
// is served, not the working tree.
func distFile(t *testing.T, name string) string {
	t.Helper()
	data, err := fs.ReadFile(Dist, "dist/"+name)
	if err != nil {
		t.Fatalf("read dist/%s: %v", name, err)
	}
	return string(data)
}

func distJS(t *testing.T) map[string]string {
	t.Helper()
	names, err := fs.Glob(Dist, "dist/assets/*.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 2 {
		t.Fatalf("dist/assets holds %d js chunks; the bundle looks unbuilt and every check below would pass vacuously", len(names))
	}
	out := make(map[string]string, len(names))
	for _, n := range names {
		data, err := fs.ReadFile(Dist, n)
		if err != nil {
			t.Fatal(err)
		}
		out[n] = string(data)
	}
	return out
}

var reCSPMeta = regexp.MustCompile(`(?i)<meta\s+http-equiv="Content-Security-Policy"\s+content="([^"]*)"`)

// The image policy is a second, independent check on what images.go guards: even if a
// future edit there emitted a remote src, the surface would not load it. That is
// only true while this meta tag is in the document that ships.
func TestShippedDocumentCarriesTheImagePolicy(t *testing.T) {
	shipped := distFile(t, "index.html")
	m := reCSPMeta.FindStringSubmatch(shipped)
	if m == nil {
		t.Fatalf("dist/index.html carries no Content-Security-Policy:\n%s", shipped)
	}
	policy := m[1]

	if !strings.Contains(policy, "img-src 'self' data: blob:") {
		t.Errorf("policy = %q, want img-src limited to self, data: and blob:", policy)
	}
	// img-src alone, on purpose: this restricts images and nothing else. A
	// default-src here would govern the bundle's own scripts and styles, which is
	// a different decision and not one this tag is making.
	if strings.Contains(policy, "default-src") {
		t.Errorf("policy = %q: a default-src here would govern the surface itself, not just images", policy)
	}
	for _, host := range []string{"http:", "https:", "*"} {
		if strings.Contains(policy, host) {
			t.Errorf("policy = %q names %q, so an image could come off the network", policy, host)
		}
	}
}

// The bundle is committed, so it can drift from its source: an edit to
// frontend/index.html that nobody rebuilt would leave the shipped policy at
// whatever it was before.
func TestTheShippedPolicyMatchesItsSource(t *testing.T) {
	src, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	from := reCSPMeta.FindStringSubmatch(string(src))
	if from == nil {
		t.Fatalf("frontend/index.html carries no Content-Security-Policy")
	}
	to := reCSPMeta.FindStringSubmatch(distFile(t, "index.html"))
	if to == nil {
		t.Fatal("dist/index.html carries no Content-Security-Policy")
	}
	if from[1] != to[1] {
		t.Errorf("the source policy is %q and the shipped one is %q; run `npm run build`", from[1], to[1])
	}
}

// agentbox works offline, which is a promise about the surface as well as the
// artifact: everything the shipped document loads has to come out of the binary.
func TestTheShippedDocumentLoadsNothingRemote(t *testing.T) {
	shipped := distFile(t, "index.html")
	refs := regexp.MustCompile(`(?i)\b(?:src|href)\s*=\s*"([^"]*)"`).FindAllStringSubmatch(shipped, -1)
	if len(refs) == 0 {
		t.Fatal("dist/index.html references no assets at all; the bundle looks broken")
	}
	for _, m := range refs {
		if !strings.HasPrefix(m[1], "/assets/") {
			t.Errorf("dist/index.html loads %q, which is not a bundled asset", m[1])
		}
	}
}

// The artifact bargain (ADR-0010): agent markup runs, but it runs with no network
// of any kind and no reach into the surface around it. Both halves are constants,
// and both have to survive into the bundle.
func TestTheArtifactSandboxShipsClosed(t *testing.T) {
	chunks := distJS(t)

	// Every directive that closes a way out. connect-src is fetch, XHR and
	// websockets; frame-src and child-src are a nested frame that would have its
	// own; form-action is a POST; base-uri would let a relative URL be re-pointed.
	required := []string{
		"default-src 'none'",
		"connect-src 'none'",
		"form-action 'none'",
		"frame-src 'none'",
		"child-src 'none'",
		"base-uri 'none'",
		"img-src data: blob:",
		"font-src data:",
	}
	var carrier string
	for name, js := range chunks {
		if strings.Contains(js, "default-src 'none'") {
			carrier = name
			for _, directive := range required {
				if !strings.Contains(js, directive) {
					t.Errorf("%s carries the artifact policy but not %q", name, directive)
				}
			}
		}
	}
	if carrier == "" {
		t.Fatal("no shipped chunk carries the artifact CSP; an artifact would run under no policy at all")
	}

	// The sandbox. allow-scripts without allow-same-origin is what gives the frame
	// an opaque origin; adding allow-same-origin would hand agent code the surface's
	// own origin, and the two together are documented as equivalent to no sandbox.
	if !strings.Contains(chunks[carrier], "allow-scripts") {
		t.Errorf("%s does not set allow-scripts", carrier)
	}
	for name, js := range chunks {
		if strings.Contains(js, "allow-same-origin") {
			t.Errorf("%s ships allow-same-origin, which cancels the sandbox", name)
		}
	}
	if css, err := fs.Glob(Dist, "dist/assets/*.css"); err == nil {
		for _, n := range css {
			data, _ := fs.ReadFile(Dist, n)
			if strings.Contains(string(data), "allow-same-origin") {
				t.Errorf("%s mentions allow-same-origin", n)
			}
		}
	}
}

// The source, so a failure above names the file to fix rather than a hashed chunk.
func TestTheArtifactPolicySourceIsClosed(t *testing.T) {
	src, err := os.ReadFile("src/lib/artifact-runtime.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)
	for _, want := range []string{
		`SANDBOX = "allow-scripts"`,
		`"default-src 'none'"`,
		`"connect-src 'none'"`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("artifact-runtime.js no longer declares %s", want)
		}
	}
	// In a string literal, not in prose: the file's header comment says the words
	// "no allow-same-origin", and explaining the rule is not breaking it.
	for _, m := range reStringLit.FindAllStringSubmatch(js, -1) {
		lit := m[1] + m[2] + m[3] // whichever quote it was
		if strings.Contains(lit, "allow-same-origin") {
			t.Errorf("artifact-runtime.js declares allow-same-origin in %q, which cancels the sandbox", lit)
		}
	}
}

// reStringLit is every quoted string in a JS source, which is where a policy value
// can actually live. It is deliberately naive about escapes: a sandbox token has
// none, and a false positive here would only ever be a stricter test.
var reStringLit = regexp.MustCompile("\"([^\"\\n]*)\"|'([^'\\n]*)'|`([^`]*)`")

// The trust model in one line: a surface renders HTML, and the only HTML it may
// render is HTML that Go produced. Every {@html} in the tree is therefore an
// allowlist entry, and a new one has to be argued for here before it ships -
// `{@html item.title}` would put an agent's own markup on screen, and Svelte
// would not stop it.
func TestEveryHTMLInjectionComesFromGo(t *testing.T) {
	// The five fields, each rendered by internal/webui and covered by the sweep in
	// internal/webui/policy_test.go:
	//   seg.html       encodeTurns      (an agent's turn: prose and thinking)
	//   seg.toolHtml   encodeSegments   (a tool call's argument, highlighted)
	//   view.bodyHtml  encode           (a card body, and the same field in a toast)
	//   ask.bodyHtml   encodeAsk        (a question in the inline panel)
	//   det.bodyHtml   encodeDetail     (an inbox row read back in full, FR73)
	//   doc.html       the viewer       (a document, or an artifact block)
	//   l.html/dl.html boardrender      (one code line's chroma spans on the
	//                                    review board; dl is a removed line)
	allowed := map[string]bool{
		"seg.html":      true,
		"seg.toolHtml":  true,
		"view.bodyHtml": true,
		// Assignments.svelte: an assignment's custom parameter panel, rendered by
		// RenderPanel (internal/webui/artifact.go), and a markdown knob's prose,
		// rendered by RenderMarkdown (paramsFor). Both are in the sweep.
		"open.panelBlock": true,
		"k.bodyHtml":      true,
		"ask.bodyHtml":    true,
		"det.bodyHtml":    true,
		"doc.html":        true,
		"l.html":          true,
		"dl.html":         true,
	}

	files, err := svelteAndJS()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 10 {
		t.Fatalf("found %d surface sources; the walk is not finding them", len(files))
	}

	re := regexp.MustCompile(`\{@html\s+([^}]+)\}`)
	found := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(data), -1) {
			expr := strings.TrimSpace(m[1])
			found++
			if !allowed[expr] {
				t.Errorf("%s injects {@html %s}, which is not a field Go rendered. "+
					"If it is one, add it to the allowlist here and to the sweep in "+
					"internal/webui/policy_test.go; if it is agent text, print it instead.", path, expr)
			}
		}
	}
	if found == 0 {
		t.Fatal("no {@html} sites found at all; this test would never fail")
	}
}

// Raw agent text must reach a surface as text. This is the counterpart to the
// allowlist: the fields that are NOT rendered HTML must never be injected.
func TestAgentTextIsNeverInjected(t *testing.T) {
	files, err := svelteAndJS()
	if err != nil {
		t.Fatal(err)
	}
	// Fields that carry an agent's own words verbatim (proto.Item and the wire
	// types around it). innerHTML on any of them would run whatever they contain.
	danger := []string{"innerHTML", "outerHTML", "insertAdjacentHTML"}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			for _, d := range danger {
				if !strings.Contains(line, d) {
					continue
				}
				// markdown.svelte.js rewrites blocks Go already produced (mermaid,
				// KaTeX, copy buttons) and artifact-runtime.js assembles the sandbox
				// document; both are reviewed and both are covered above.
				if strings.Contains(path, "markdown.svelte.js") ||
					strings.Contains(path, "artifact-runtime.js") ||
					strings.Contains(path, "artifact.svelte.js") {
					continue
				}
				t.Errorf("%s uses %s: %s", path, d, strings.TrimSpace(line))
			}
		}
	}
}

func svelteAndJS() ([]string, error) {
	var out []string
	err := filepath.WalkDir("src", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// The generated React and Tailwind runtimes are somebody else's minified
			// library, injected as text into the sandbox; they are not surface code.
			if d.Name() == "generated" {
				return fs.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".svelte", ".js":
			out = append(out, path)
		}
		return nil
	})
	return out, err
}
