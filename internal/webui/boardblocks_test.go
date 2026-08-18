package webui

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The render side of FR101. Everything here is about one promise: whatever a
// figure, a table or a callout says in the spec, what crosses the wire is
// something the surface can paint with no filesystem and no network.

// tinyPNG is a 1x1 PNG. Small enough to inline in a test, real enough to pass the
// sniff and the pixel budget every image on this surface goes through.
const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg=="

const wireSVG = `<svg viewBox="0 0 40 20"><rect x="1" y="1" width="10" height="10" fill="var(--k-surface-2)"/><text x="4" y="16">hop</text></svg>`

// bodySpec is a spec whose one ground step carries the blocks given, as JSON.
func bodySpec(t *testing.T, blocks []map[string]any) string {
	t.Helper()
	m := map[string]any{
		"version": 1, "title": "t", "repo_root": "/x", "pinned": "dd375a3cb2c7",
		"steps": []map[string]any{{
			"id": "s1", "kind": "ground", "title": "The shape",
			"prose": []map[string]any{{"t": "one request, three hops"}},
			"body":  blocks,
		}},
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func renderBody(t *testing.T, root string, blocks []map[string]any) []wireBlock {
	t.Helper()
	steps, _, _, err := renderSteps(bodySpec(t, blocks), "", root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return steps[0].Body
}

func TestFigureSVGCrossesAsRecomposedMarkup(t *testing.T) {
	body := renderBody(t, t.TempDir(), []map[string]any{
		{"lead": "The path, drawn.", "figure": map[string]any{"svg": wireSVG, "caption": "three hops"}},
	})
	if len(body) != 1 || body[0].Kind != "figure" || body[0].Figure == nil {
		t.Fatalf("body: %+v", body)
	}
	f := body[0].Figure
	if f.Err != "" {
		t.Fatalf("a plain diagram was refused at render time: %s", f.Err)
	}
	if !strings.Contains(f.SVG, `<svg viewBox="0 0 40 20">`) || !strings.Contains(f.SVG, "<text") {
		t.Errorf("svg did not survive: %s", f.SVG)
	}
	if f.Caption != "three hops" {
		t.Errorf("caption: %q", f.Caption)
	}
	// The lead belongs to the BLOCK, not to the code payload, which is what lets a
	// figure and a table have one at all.
	if body[0].Lead != "The path, drawn." {
		t.Errorf("lead: %q", body[0].Lead)
	}
}

// The surface never learns a path. A figure citing a file in the repo arrives as
// a data: URI, read here, jailed to the spec's root.
func TestFigureFileBecomesADataURI(t *testing.T) {
	root := t.TempDir()
	png, err := base64.StdEncoding.DecodeString(tinyPNG)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs", "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "img", "board.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	body := renderBody(t, root, []map[string]any{
		{"figure": map[string]any{"src": "docs/img/board.png", "alt": "the board"}},
	})
	f := body[0].Figure
	if f.Err != "" {
		t.Fatalf("the image was refused: %s", f.Err)
	}
	if !strings.HasPrefix(f.Src, "data:image/png;base64,") {
		t.Errorf("src should be a data URI, got %.40q", f.Src)
	}
	if strings.Contains(f.Src, "docs/img") {
		t.Errorf("the path reached the surface: %.60q", f.Src)
	}
}

// Two absences, both stated rather than blank: a file that is not there, and
// markup the sanitizer refuses. A stored spec can hold either, because a
// walkthrough outlives the tree it cites.
func TestFigureFailuresAreStatedNotBlank(t *testing.T) {
	root := t.TempDir()
	var missed []string
	steps, _, _, err := renderSteps(bodySpec(t, []map[string]any{
		{"figure": map[string]any{"src": "docs/gone.png"}},
		{"figure": map[string]any{"svg": `<svg viewBox="0 0 1 1"><script>x()</script></svg>`}},
	}), "", root, nil, func(step, path, reason string) { missed = append(missed, step+":"+path+":"+reason) })
	if err != nil {
		t.Fatal(err)
	}
	body := steps[0].Body
	if got := body[0].Figure.Err; got != "file not found" {
		t.Errorf("missing file reason = %q", got)
	}
	if got := body[1].Figure.Err; !strings.Contains(got, "<script> is not allowed") {
		t.Errorf("hostile markup reason = %q", got)
	}
	if body[0].Figure.Src != "" || body[1].Figure.SVG != "" {
		t.Error("a refused figure must carry no picture")
	}
	if len(missed) != 2 {
		t.Errorf("both failures should reach the log, got %v", missed)
	}
}

func TestTableAndCalloutCrossWhole(t *testing.T) {
	body := renderBody(t, t.TempDir(), []map[string]any{
		{"table": map[string]any{
			"head":    []string{"hop", "cost"},
			"rows":    [][]string{{"the daemon", "0.4 ms"}, {"the store", "11 ms"}},
			"align":   []string{"left", "right"},
			"caption": "measured on the loaded machine",
		}},
		{"callout": map[string]any{
			"tone":  "danger",
			"title": "The second hop is the one that fails",
			"prose": []map[string]any{{"t": "It retries three times, then gives up."}},
		}},
	})
	if body[0].Kind != "table" || body[0].Table == nil {
		t.Fatalf("table block: %+v", body[0])
	}
	tb := body[0].Table
	if len(tb.Rows) != 2 || tb.Rows[1][1] != "11 ms" || tb.Align[1] != "right" || tb.Caption == "" {
		t.Errorf("table: %+v", tb)
	}
	if body[1].Kind != "callout" || body[1].Callout == nil {
		t.Fatalf("callout block: %+v", body[1])
	}
	c := body[1].Callout
	if c.Tone != "danger" || c.Title == "" || len(c.Prose) != 1 || c.Prose[0].T == "" {
		t.Errorf("callout: %+v", c)
	}
}

// Glossary marking is per step and in reading order, and a callout is part of
// that order: a term first said inside one is marked there, and stays plain
// afterwards. Without this the definition is unreachable from the one place the
// author put the word.
func TestCalloutProseIsGlossaryMarked(t *testing.T) {
	m := map[string]any{
		"version": 1, "title": "t", "repo_root": "/x", "pinned": "dd375a3cb2c7",
		"glossary": []map[string]any{{"term": "keepalive", "short": "the ping that says a session is alive"}},
		"steps": []map[string]any{{
			"id": "s1", "kind": "ground", "title": "The shape",
			"prose": []map[string]any{{"t": "one request, three hops"}},
			"body": []map[string]any{
				{"callout": map[string]any{"tone": "warn", "prose": []map[string]any{{"t": "The keepalive dies first."}}}},
				{"callout": map[string]any{"tone": "note", "prose": []map[string]any{{"t": "The keepalive again, unmarked."}}}},
			},
		}},
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	steps, terms, _, err := renderSteps(string(raw), "", t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 1 {
		t.Fatalf("glossary: %+v", terms)
	}
	first := steps[0].Body[0].Callout.Prose[0].Runs
	marked := false
	for _, r := range first {
		if r.Key != "" {
			marked = true
		}
	}
	if !marked {
		t.Errorf("the first mention inside a callout was not marked: %+v", first)
	}
	for _, r := range steps[0].Body[1].Callout.Prose[0].Runs {
		if r.Key != "" {
			t.Errorf("the second mention was marked again: %+v", r)
		}
	}
}

// "all of this is new" is a claim about cited code. A step with no citations must
// never make it, or a walkthrough of three diagrams announces itself as an
// all-new change.
func TestAllNewIsAboutCodeOnly(t *testing.T) {
	steps, _, _, err := renderSteps(bodySpec(t, []map[string]any{
		{"figure": map[string]any{"svg": wireSVG}},
	}), "", t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if steps[0].AllNew {
		t.Error("a step whose body is one figure claimed to be all new")
	}
}

// boardFigureSVG renders one accepted figure whose text carries markup an author
// might have pasted in. The sweep in policy_test.go audits the result: the words
// arrive escaped, and nothing in the output fetches.
func boardFigureSVG(t *testing.T) string {
	t.Helper()
	body := renderBody(t, t.TempDir(), []map[string]any{
		{"figure": map[string]any{
			"svg": `<svg viewBox="0 0 60 20"><text x="2" y="14">` +
				`&lt;img src="https://evil.example/a.gif"&gt; and url(https://evil.example/b.css)` +
				`</text></svg>`,
		}},
	})
	f := body[0].Figure
	if f.Err != "" {
		t.Fatalf("the fixture figure was refused, so the sweep would pass vacuously: %s", f.Err)
	}
	return f.SVG
}
