package webui

import "sync"

// The rendered-markdown cache (R-17's other half).
//
// The session transcript re-renders the WHOLE selected conversation on every push,
// and a push happens up to every 60ms while a reply streams (`sessions.touch`). So
// every segment of a long conversation went through goldmark fifteen times a second
// while only the last one was changing, and the cost grows with the conversation
// rather than with what arrived.
//
// Content-addressed rather than keyed on a position, which is what makes it safe:
// rendering is pure, so the same text always gives the same HTML, a streaming tail
// simply misses until it stops growing, an edited turn cannot serve a stale render,
// and scrolling back through history is free. There is nothing to invalidate.
//
// Bounded by bytes, and when it is full the whole map goes rather than the oldest
// entry. That is what the image cache does, for the same reason: an LRU here would
// be more code than the work it saves, and the pathological case (a conversation
// bigger than the budget) degrades to what the code did before this existed.
const mdCacheBytes = 8 << 20

var mdCache = struct {
	sync.Mutex
	at    map[string]string
	bytes int
	// renders counts the misses, which is what a test can assert on. A timing
	// assertion would measure the machine; this measures the mechanism.
	renders int
}{at: map[string]string{}}

// cachedRender returns the rendered form of one segment, rendering it only the
// first time that exact text is seen. key must include whatever else the render
// depends on (a lexer name, for instance) - two renders that share a key must be
// the same render.
func cachedRender(key string, render func() string) string {
	mdCache.Lock()
	if html, ok := mdCache.at[key]; ok {
		mdCache.Unlock()
		return html
	}
	mdCache.Unlock()

	// Outside the lock. Holding it across goldmark would serialise every surface
	// behind the slowest render, and losing a race here costs one duplicate render.
	html := render()

	mdCache.Lock()
	defer mdCache.Unlock()
	mdCache.renders++
	if mdCache.bytes+len(key)+len(html) > mdCacheBytes {
		mdCache.at = make(map[string]string, len(mdCache.at))
		mdCache.bytes = 0
	}
	mdCache.at[key] = html
	mdCache.bytes += len(key) + len(html)
	return html
}

// renderMarkdownCached is RenderMarkdown for content that is re-rendered on a
// cadence rather than once. The prefix keeps prose apart from a one-line
// highlight of the same string.
func renderMarkdownCached(src string) string {
	if src == "" {
		return ""
	}
	return cachedRender("md\x00"+src, func() string { return RenderMarkdown(src) })
}

// highlightInlineCached is HighlightInline under the same cache. A tool's argument
// is short, and there are hundreds of them in a long conversation.
func highlightInlineCached(src, lang string) string {
	if src == "" {
		return ""
	}
	return cachedRender("hl\x00"+lang+"\x00"+src, func() string { return HighlightInline(src, lang) })
}
