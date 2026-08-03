package webui

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Images (M10 slice 4), and the one part of markdown rendering that is a security
// decision rather than a typographic one.
//
// Raw HTML never reaches a surface - goldmark drops it, because a card body can
// come from any agent - so `<img>` cannot be written by hand. But `![alt](src)`
// is ordinary markdown, and goldmark's own renderer turns it into a live
// `<img src>` pointing wherever the source said. That is a request the surface
// makes on an agent's behalf to a host the human never saw, with whatever the
// agent chose to put in the path, and it is why this file exists rather than
// letting the default renderer through.
//
// The rule, which is the same bargain ADR-0010 struck for artifacts - the markup
// is bound, the agent is not:
//
//   - A local file is read by Go and inlined as a data: URI. The surface receives
//     bytes, never a path, so it never learns how to open one.
//   - The path must be absolute (`~` counts), unless the render was handed a base
//     directory to read it against. Only a document that is a file on disk has
//     one - `agentbox show FILE` and the viewer's watch loop - and there
//     `![](out/chart.png)` means to the reader exactly what it meant to whoever
//     wrote the file. Prose arriving over the socket gets no base, because the
//     daemon's working directory is not the agent's and guessing would be wrong
//     more often than right.
//   - The bytes have to actually be a PNG, JPEG, GIF or WebP, by their own magic
//     numbers rather than by their extension.
//   - There is a ceiling, checked against the file's size before anything is
//     read.
//   - Everything else - http, https, protocol-relative, a scheme agentbox has never
//     heard of - is not fetched. It renders as a marked placeholder that says so
//     and keeps the alt text, because a reader should know something was meant to
//     be there.
//
// A data: URI written directly by an agent is allowed under the same ceiling and
// the same sniff, re-encoded from its decoded bytes so nothing rides along in the
// payload.
//
// index.html carries `img-src 'self' data: blob:` so this is enforced rather than
// merely intended: if a future edit here ever emitted a remote src, the surface
// still would not load it.

// maxImageBytes is the ceiling on one image. Big enough for any screenshot or
// plot an agent renders, small enough that a conversation holding several of them
// is still a conversation.
const maxImageBytes = 2 << 20

// imageCacheBytes bounds what inlining keeps in memory. It matters because
// encodeTurns re-renders every turn on every stream push (~16Hz): without a cache
// a screenshot in an early turn would be re-read and re-encoded sixteen times a
// second for the rest of the session.
const imageCacheBytes = 16 << 20

// imageTypes is the set agentbox will inline. SVG is deliberately absent: a vector
// picture from an agent is a ```chart or a ```mermaid fence, both of which agentbox
// draws itself, and neither of which needs a parser for somebody else's XML.
var imageTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// reScheme matches an RFC 3986 scheme. Anything with one is a URL, and the only
// URL agentbox resolves is file:.
var reScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)

// baseDirKey carries the directory a relative destination is read against. It
// travels in the parser context rather than on the renderer because one engine
// serves every surface: a card body and a document can render in the same
// millisecond and only one of them has a place of its own.
var baseDirKey = parser.NewContextKey()

// imageBaseTransformer rewrites a relative destination into the absolute path it
// means, for a render that was told where the document lives. It happens at parse
// time because a NodeRenderer is handed only the source bytes and the node - the
// parser context is the last place a per-render fact can still be read.
type imageBaseTransformer struct{}

func (imageBaseTransformer) Transform(doc *ast.Document, _ text.Reader, pc parser.Context) {
	base, _ := pc.Get(baseDirKey).(string)
	if base == "" {
		return
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		img, ok := n.(*ast.Image)
		if !ok {
			return ast.WalkContinue, nil
		}
		if abs := resolveImage(base, string(img.Destination)); abs != "" {
			img.Destination = []byte(abs)
		}
		return ast.WalkContinue, nil
	})
}

// resolveImage joins a relative destination onto base, and returns "" for a
// destination that is not one: already absolute, a home path, a URL, a data:
// payload. It decides nothing about whether the result may be shown - the joined
// path goes through the same stat, sniff and ceiling as a path an agent wrote out
// in full, which is why a base grants no reach that an absolute path did not
// already have.
func resolveImage(base, dest string) string {
	dest = strings.TrimSpace(dest)
	switch {
	case dest == "", strings.HasPrefix(dest, "~"), strings.HasPrefix(dest, "//"):
		return ""
	case reScheme.MatchString(dest), filepath.IsAbs(dest):
		return ""
	}
	return filepath.Join(base, dest)
}

type imageRenderer struct{}

func (i *imageRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, i.render)
}

func (i *imageRenderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*ast.Image)
	dest := string(n.Destination)
	alt := altText(node, source)
	title := string(n.Title)

	uri, reason := inlineImage(dest)
	if reason != "" {
		w.WriteString(blockedImageHTML(alt, dest, reason))
		return ast.WalkSkipChildren, nil
	}

	// An <img> and nothing around it: goldmark puts an image inside the paragraph
	// it was written in, and a <figure> there would be invalid HTML that browsers
	// silently repair by splitting the paragraph.
	var b strings.Builder
	b.WriteString(`<img class="k-img" src="` + uri + `" alt="` + template(alt) + `"`)
	if title != "" {
		b.WriteString(` title="` + template(title) + `"`)
	}
	b.WriteString(` loading="lazy">`)
	w.WriteString(b.String())
	return ast.WalkSkipChildren, nil
}

// inlineImage turns a markdown destination into a data: URI, or reports why it
// will not. The reason is a stable token the surface styles and the tests assert
// on; the sentence a reader sees comes from blockedReasons.
func inlineImage(dest string) (uri, reason string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return "", "missing"
	}

	if strings.HasPrefix(strings.ToLower(dest), "data:") {
		return inlineDataURI(dest)
	}
	if strings.HasPrefix(dest, "//") {
		return "", "remote" // protocol-relative: whatever the surface is, over the network
	}

	path := dest
	if m := reScheme.FindString(dest); m != "" {
		if !strings.EqualFold(m, "file:") {
			return "", "remote"
		}
		u, err := url.Parse(dest)
		if err != nil || (u.Host != "" && u.Host != "localhost") {
			return "", "remote"
		}
		path = u.Path
	} else if unescaped, err := url.PathUnescape(path); err == nil {
		// `%20` for a space is how a path with a space in it survives markdown.
		path = unescaped
	}

	return inlineFile(path)
}

func inlineFile(path string) (uri, reason string) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "relative"
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if !filepath.IsAbs(path) {
		return "", "relative"
	}
	path = filepath.Clean(path)

	info, err := os.Stat(path)
	switch {
	case err != nil:
		return "", "missing"
	case !info.Mode().IsRegular():
		// A directory, a socket, a device. Reading /dev/zero would not end.
		return "", "not-an-image"
	case info.Size() > maxImageBytes:
		return "", "too-big"
	}

	if hit, ok := cachedImage(path, info); ok {
		return hit, ""
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", "unreadable"
	}
	if len(data) > maxImageBytes {
		return "", "too-big" // it grew between the stat and the read
	}
	mime := sniffImage(data)
	if mime == "" {
		return "", "not-an-image"
	}
	uri = dataURI(mime, data)
	storeImage(path, info, uri)
	return uri, ""
}

// inlineDataURI re-encodes a data: URI an agent wrote, which is how the ceiling
// and the sniff apply to it as well.
func inlineDataURI(dest string) (uri, reason string) {
	comma := strings.IndexByte(dest, ',')
	if comma < 0 {
		return "", "not-an-image"
	}
	meta, payload := dest[5:comma], dest[comma+1:]
	if !strings.Contains(strings.ToLower(meta), "base64") {
		return "", "not-an-image" // percent-encoded pixels are not a thing agents send
	}
	// Whitespace is how a long data: URI survives being written into prose.
	payload = strings.Join(strings.Fields(payload), "")
	if len(payload)/4*3 > maxImageBytes {
		return "", "too-big"
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", "not-an-image"
	}
	if len(data) > maxImageBytes {
		return "", "too-big"
	}
	mime := sniffImage(data)
	if mime == "" {
		return "", "not-an-image"
	}
	return dataURI(mime, data), ""
}

// sniffImage reads the type out of the bytes. The extension is not evidence: it
// is the part of the destination an agent is most likely to have got wrong, and
// the part an attacker would choose.
func sniffImage(data []byte) string {
	mime := http.DetectContentType(data)
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	mime = strings.TrimSpace(strings.ToLower(mime))
	if !imageTypes[mime] {
		return ""
	}
	return mime
}

func dataURI(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// --- the cache --------------------------------------------------------------

type imageEntry struct {
	mod  time.Time
	size int64
	uri  string
}

var images = struct {
	sync.Mutex
	byPath map[string]imageEntry
	order  []string
	bytes  int
}{byPath: map[string]imageEntry{}}

// cachedImage returns a previous encoding of this file, but only while the file
// on disk is still the one that was encoded - so `agentbox show --watch` on a
// document whose image was just regenerated shows the new picture.
func cachedImage(path string, info os.FileInfo) (string, bool) {
	images.Lock()
	defer images.Unlock()
	e, ok := images.byPath[path]
	if !ok || e.size != info.Size() || !e.mod.Equal(info.ModTime()) {
		return "", false
	}
	return e.uri, true
}

func storeImage(path string, info os.FileInfo, uri string) {
	images.Lock()
	defer images.Unlock()
	if old, ok := images.byPath[path]; ok {
		images.bytes -= len(old.uri)
	} else {
		images.order = append(images.order, path)
	}
	images.byPath[path] = imageEntry{mod: info.ModTime(), size: info.Size(), uri: uri}
	images.bytes += len(uri)

	for images.bytes > imageCacheBytes && len(images.order) > 1 {
		oldest := images.order[0]
		images.order = images.order[1:]
		if e, ok := images.byPath[oldest]; ok {
			images.bytes -= len(e.uri)
			delete(images.byPath, oldest)
		}
	}
}

// --- the placeholder --------------------------------------------------------

// blockedReasons is what a reader is told. Each one says what to do about it,
// because the person reading is usually the person who can fix it.
var blockedReasons = map[string]string{
	"remote":       "remote image not loaded",
	"relative":     "needs an absolute path",
	"missing":      "file not found",
	"too-big":      "over the size limit",
	"not-an-image": "not a PNG, JPEG, GIF or WebP",
	"unreadable":   "could not be read",
}

func blockedImageHTML(alt, dest, reason string) string {
	why, ok := blockedReasons[reason]
	if !ok {
		why = "not shown"
	}
	if reason == "too-big" {
		why = fmt.Sprintf("over %d MB", maxImageBytes>>20)
	}
	label := strings.TrimSpace(alt)
	if label == "" {
		label = "image"
	}
	// The destination goes in the title attribute and nowhere else: it is what
	// somebody debugging needs, and putting it in the flow of a sentence would let
	// an agent write prose into a paragraph by way of a broken image.
	return `<span class="k-img-blocked" data-reason="` + template(reason) + `" title="` + template(dest) + `">` +
		imageOffIcon() +
		`<span class="k-img-alt">` + template(label) + `</span>` +
		`<span class="k-img-why">` + template(why) + `</span></span>`
}

func imageOffIcon() string {
	return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" ` +
		`stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">` +
		`<path d="M20 16.5V5a1 1 0 0 0-1-1H7.5"/><path d="M4 7.5V19a1 1 0 0 0 1 1h11.5"/>` +
		`<path d="M4 17l4.5-4.5 3 3"/><path d="M3 3l18 18"/></svg>`
}

// altText collects the alt text out of an image's children. goldmark keeps it as
// inline nodes because it is markdown in its own right; a reader wants the words.
func altText(node ast.Node, source []byte) string {
	var b strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch t := child.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(source))
		case *ast.String:
			b.Write(t.Value)
		default:
			b.WriteString(altText(child, source))
		}
	}
	return b.String()
}
