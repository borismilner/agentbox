package webui

import (
	"encoding/json"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

// Artifacts (M10): agent-authored interactive HTML that agentbox runs.
//
// The bargain is stated twice, here and in the surface that assembles the
// document (frontend/src/lib/artifact-runtime.js). Agent markup runs, but it runs
// inside `sandbox="allow-scripts"` with no `allow-same-origin` - an opaque origin,
// no reach into the surface around it - under a Content-Security-Policy with no
// network of any kind. React and Tailwind are injected from agentbox's own bundle
// rather than a CDN, so an artifact written for claude.ai runs offline, and
// nothing in it can phone home with what you typed into it.
//
// Go's half is the part worth testing: which fences become an artifact, what
// runtime a source wants, and the ceiling on how much of one agentbox will carry
// into a window. What it emits is inert - the source, its highlighted twin and an
// empty stage - and stays inert until a surface hydrates it. The trust switch
// ([artifact] enabled) is enforced at that hydration, because that is where the
// iframe would otherwise be created; with it off an artifact is source you read.
const artifactMaxBytes = 96 << 10

// artifactSpec is what a source needs in order to run: which shape it is, and
// which of the two bundled libraries have to be in the document with it.
type artifactSpec struct {
	Runtime  string // "react" for a module agentbox mounts, "html" for a document or fragment
	React    bool
	Tailwind bool
	TooBig   bool
}

var (
	// A module that imports react, or exports a component, is a React artifact -
	// the claude.ai shape, and the one agentbox has to mount itself.
	reReactImport = regexp.MustCompile(`(?m)^\s*import\b[^;\n]*from\s*['"]react`)
	reExportDflt  = regexp.MustCompile(`(?m)^\s*export\s+default\b`)
	reHTMLDoc     = regexp.MustCompile(`(?i)<!doctype\s+html|<html[\s>]|<head[\s>]|<body[\s>]`)
	reReactGlobal = regexp.MustCompile(`\bReact(DOM)?\s*\.`)
	reBabelScript = regexp.MustCompile(`(?i)type\s*=\s*["'](text/babel|text/jsx)["']`)
	reReactCDN    = regexp.MustCompile(`(?i)src\s*=\s*["'][^"']*\breact(-dom)?[^"']*\.js`)
	reTailwindRef = regexp.MustCompile(`(?i)tailwind`)
	reClassAttr   = regexp.MustCompile(`class(?:Name)?\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	reScriptTag   = regexp.MustCompile(`(?i)<script[\s>]`)
)

// specFor decides how a source wants to be run. It errs toward "html": a plain
// document is the safer reading of an ambiguous source, because agentbox mounts a
// React artifact for it and mounting something that is not a component shows an
// empty stage, while a document that happens to mention React still runs.
func specFor(src string) artifactSpec {
	spec := artifactSpec{Runtime: "html", TooBig: len(src) > artifactMaxBytes}
	switch {
	case reHTMLDoc.MatchString(src):
		// A whole document is a document, even one that builds its UI with React.
	case reReactImport.MatchString(src), reExportDflt.MatchString(src):
		// A module: it imports react, or it hands out a component for agentbox to
		// mount. Nothing that is meant to be a page says `export default`.
		spec.Runtime = "react"
	}

	spec.React = spec.Runtime == "react" ||
		reReactGlobal.MatchString(src) || reBabelScript.MatchString(src) || reReactCDN.MatchString(src)

	// Tailwind is assumed for a React artifact: the claude.ai convention is
	// utility classes and no stylesheet, and an artifact written there arrives
	// styled by classes alone. For a document, ship it only when the document
	// looks like it wants it - Tailwind's preflight resets margins and heading
	// sizes, so applying it to markup that brought its own CSS would restyle
	// somebody else's design.
	spec.Tailwind = spec.Runtime == "react" || reTailwindRef.MatchString(src) || tailwindish(src)
	return spec
}

// utilityWords are Tailwind class names that stand alone.
var utilityWords = map[string]bool{
	"flex": true, "grid": true, "block": true, "inline": true, "inline-flex": true,
	"inline-block": true, "hidden": true, "container": true, "relative": true,
	"absolute": true, "fixed": true, "sticky": true, "truncate": true, "italic": true,
	"underline": true, "uppercase": true, "antialiased": true, "sr-only": true,
}

// utilityStems are the ones that carry a value: p-4, text-sm, bg-slate-900.
var utilityStems = map[string]bool{
	"p": true, "px": true, "py": true, "pt": true, "pb": true, "pl": true, "pr": true,
	"m": true, "mx": true, "my": true, "mt": true, "mb": true, "ml": true, "mr": true,
	"w": true, "h": true, "gap": true, "text": true, "font": true, "bg": true,
	"border": true, "rounded": true, "shadow": true, "ring": true, "items": true,
	"justify": true, "self": true, "opacity": true, "z": true, "overflow": true,
	"leading": true, "tracking": true, "inset": true, "top": true, "left": true,
	"right": true, "bottom": true, "transition": true, "duration": true, "cursor": true,
	"divide": true, "space": true, "col": true, "row": true, "min": true, "max": true,
	"place": true, "order": true, "basis": true, "flex": true, "grid": true,
}

// tailwindish reports whether the class attributes in a source read as utility
// classes. Three is the threshold: one or two names could be anybody's CSS,
// while three in the same document is a convention.
func tailwindish(src string) bool {
	seen := 0
	for _, m := range reClassAttr.FindAllStringSubmatch(src, -1) {
		value := m[1]
		if value == "" {
			value = m[2]
		}
		for tok := range strings.FieldsSeq(value) {
			// Strip variants (hover:, md:, dark:group-hover:) down to the utility.
			if i := strings.LastIndex(tok, ":"); i >= 0 {
				tok = tok[i+1:]
			}
			tok = strings.TrimPrefix(tok, "-") // -mt-2
			if utilityWords[tok] {
				seen++
				continue
			}
			if i := strings.Index(tok, "-"); i > 0 && utilityStems[tok[:i]] {
				seen++
			}
		}
		if seen >= 3 {
			return true
		}
	}
	return false
}

// isArtifactFence decides whether a fence runs or stays code. ```artifact always
// runs: the word is a request. ```html is the ambiguous one - agents write it
// both to show an app and to talk about markup - so it runs only when the block
// is a document or brings behaviour with it. A table of markup an agent is
// explaining stays a code block, which is what it is.
func isArtifactFence(lang, src string) bool {
	switch lang {
	case "artifact":
		return true
	case "html":
		return reHTMLDoc.MatchString(src) || reScriptTag.MatchString(src)
	}
	return false
}

// artifactBlock is the inert markup for one artifact. The source travels as the
// text of a hidden <pre> for the same reason a mermaid diagram does: it is full
// of quotes and newlines, and an attribute would need a second escaping rule to
// get wrong. That <pre> is also the honest record - what the sandbox runs is
// exactly the text you can read in the code tab.
//
// id is what an event from this artifact is stamped with, so an agent can wait on
// this artifact rather than on anything that happens to emit (M10 slice 3). A
// caller that will wait mints its own; a fence gets one derived from its source,
// which keeps it stable across the re-renders a streaming turn causes.
func artifactBlock(src, title, id string) string {
	spec := specFor(src)
	var b strings.Builder

	if id == "" {
		id = artifactFenceID(src)
	}
	b.WriteString(`<div class="k-artifact" data-artifact-id="` + template(id) + `" data-runtime="` + spec.Runtime + `"`)
	if spec.React {
		b.WriteString(` data-react="1"`)
	}
	if spec.Tailwind {
		b.WriteString(` data-tailwind="1"`)
	}
	view := "preview"
	if spec.TooBig {
		b.WriteString(` data-toobig="1"`)
		view = "code"
	}
	b.WriteString(` data-view="` + view + `">`)

	b.WriteString(`<div class="k-artifact-bar">`)
	b.WriteString(`<span class="k-artifact-badge">interactive</span>`)
	if t := strings.TrimSpace(title); t != "" {
		b.WriteString(`<span class="k-artifact-title">` + template(t) + `</span>`)
	}
	b.WriteString(`<span class="k-artifact-runtime">` + runtimeLabel(spec) + `</span>`)
	b.WriteString(`<span class="k-artifact-note"></span>`)
	b.WriteString(`<span class="k-artifact-tabs">`)
	b.WriteString(`<button type="button" class="k-artifact-tab" data-artifact-view="preview">preview</button>`)
	b.WriteString(`<button type="button" class="k-artifact-tab" data-artifact-view="code">code</button>`)
	b.WriteString(`</span>`)
	b.WriteString(`<button type="button" class="k-artifact-run" data-artifact-run title="run it again">reload</button>`)
	b.WriteString(`</div>`)

	b.WriteString(`<pre class="k-artifact-src" hidden>` + template(src) + `</pre>`)
	b.WriteString(`<div class="k-artifact-code">` + codeBlockHTML(src, codeLang(spec)) + `</div>`)
	if spec.TooBig {
		b.WriteString(`<p class="k-artifact-refused">This artifact is larger than agentbox will run (` +
			kb(len(src)) + ` of ` + kb(artifactMaxBytes) + `). Its source is above.</p>`)
	} else {
		b.WriteString(`<div class="k-artifact-stage"></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// RenderArtifact is the whole document for an artifact shown on its own
// (show_artifact, agentbox show --artifact): one block, with no markdown pass around
// it, so a fence sequence inside the source cannot end a fence that is not there.
func RenderArtifact(src, title, id string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	return artifactBlock(src, title, id)
}

// artifactFenceID names an artifact nobody minted an id for. It is derived from
// the source rather than counted or randomised, because a conversation re-renders
// its whole HTML on every streamed token: a counter would give one artifact a
// different name every frame, and an event would carry whichever name the last
// repaint happened to leave behind.
func artifactFenceID(src string) string {
	h := fnv.New64a()
	h.Write([]byte(src))
	return "f" + strconv.FormatUint(h.Sum64(), 16)
}

// What a message out of a sandbox is allowed to be. The name is a word an agent
// matches on, so it stays short and plain; the payload is the agent's own
// vocabulary, so agentbox only insists that it is real JSON and that one gesture
// cannot hand it a megabyte.
const (
	artifactNameMax = 64
	artifactDataMax = 16 << 10
)

// artifactEvent validates one emit and turns it into the event the daemon carries.
// The sender is agent-authored code in an opaque origin, so nothing here is taken
// on trust: a name that is not a plain token, or data that is not JSON, is not a
// message agentbox passes on.
func artifactEvent(id, name, dataJSON string) (proto.ArtifactEvent, bool) {
	id, name = strings.TrimSpace(id), strings.TrimSpace(name)
	if name == "" || len(name) > artifactNameMax || !plainToken(name) {
		return proto.ArtifactEvent{}, false
	}
	if id != "" && (len(id) > artifactNameMax || !plainToken(id)) {
		return proto.ArtifactEvent{}, false
	}

	ev := proto.ArtifactEvent{ArtifactID: id, Name: name, AtMS: time.Now().UnixMilli()}
	data := strings.TrimSpace(dataJSON)
	switch {
	case data == "" || data == "null":
		// An event with no payload is a button: the name is the whole message.
	case len(data) > artifactDataMax:
		return proto.ArtifactEvent{}, false
	case !json.Valid([]byte(data)):
		return proto.ArtifactEvent{}, false
	default:
		ev.Data = json.RawMessage(data)
	}
	return ev, true
}

// plainToken keeps a name to what an agent can write in a tool call without
// quoting it: letters, digits, and the few separators event names actually use.
func plainToken(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == ':':
		default:
			return false
		}
	}
	return true
}

func runtimeLabel(spec artifactSpec) string {
	parts := []string{spec.Runtime}
	if spec.Runtime != "react" && spec.React {
		parts = append(parts, "react")
	}
	if spec.Tailwind {
		parts = append(parts, "tailwind")
	}
	return strings.Join(parts, " + ")
}

// codeLang is what the code tab highlights as. A React artifact is JSX, which
// chroma knows; a document is HTML.
func codeLang(spec artifactSpec) string {
	if spec.Runtime == "react" {
		return "jsx"
	}
	return "html"
}

func kb(n int) string {
	return strconv.Itoa((n+1023)/1024) + " kB"
}
