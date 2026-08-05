package webui

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/frontend"
	"github.com/borismilner/agentbox/internal/daemon"
)

// The identity colour, which is one hash with two implementations - Go here and
// `identityHue` in frontend/src/lib/tokens.js - and which had been quietly
// computing two different answers (FR85). What follows pins it from three sides,
// because each of the three would have missed the bug on its own:
//
//   - a fixed table, so neither side can be "corrected" by moving both;
//   - both implementations run over that table, so they cannot drift apart;
//   - the shipped bundle, so a fix that never reached `dist` fails here rather
//     than on Boris's screen.

// identityHueSamples is the table. Two rows earn their place: `agent` with no
// project is what stats.go asks for, and the last two are non-ASCII, which is
// exactly where a hash over UTF-16 code units and a hash over UTF-8 bytes come
// apart while every ASCII identity keeps agreeing.
var identityHueSamples = []struct {
	agent, project, dark, light string
}{
	{"claude-code", "agentbox", "hsl(205 62% 68%)", "hsl(205 58% 42%)"},
	{"claude", "agentbox", "hsl(105 62% 68%)", "hsl(105 58% 42%)"},
	{"codex", "grabbit", "hsl(250 62% 68%)", "hsl(250 58% 42%)"},
	{"aider", "src", "hsl(30 62% 68%)", "hsl(30 58% 42%)"},
	{"agent", "", "hsl(185 62% 68%)", "hsl(185 58% 42%)"},
	{"", "", "hsl(205 62% 68%)", "hsl(205 58% 42%)"},
	{"claude", "café", "hsl(160 62% 68%)", "hsl(160 58% 42%)"},
	{"claude", "מסמכים", "hsl(300 62% 68%)", "hsl(300 58% 42%)"},
}

func TestIdentityHueIsPinned(t *testing.T) {
	for _, s := range identityHueSamples {
		if got := IdentityHue(s.agent, s.project, true); got != s.dark {
			t.Errorf("IdentityHue(%q, %q, dark) = %s, want %s", s.agent, s.project, got, s.dark)
		}
		if got := IdentityHue(s.agent, s.project, false); got != s.light {
			t.Errorf("IdentityHue(%q, %q, light) = %s, want %s", s.agent, s.project, got, s.light)
		}
	}
}

// hueShim runs the frontend's identityHue without a browser: the source with its
// `export` keywords stripped, evaluated in a function whose only DOM is the one
// dataset flag the light branch reads. It is not a bundler and does not want to
// be - if this ever fails to parse, tokens.js has gained an import and the
// function needs lifting out of it.
const hueShim = `
const fs = require("fs");
const src = fs.readFileSync(process.argv[1], "utf8").replace(/\bexport\s+/g, "");
globalThis.document = { documentElement: { dataset: { mode: "dark" } } };
const identityHue = new Function(src + "\nreturn identityHue;")();
const out = [];
for (const [agent, project] of JSON.parse(process.argv[2])) {
  document.documentElement.dataset.mode = "dark";
  const dark = identityHue(agent, project);
  document.documentElement.dataset.mode = "light";
  out.push([dark, identityHue(agent, project)]);
}
process.stdout.write(JSON.stringify(out));
`

func TestTheTwoIdentityHuesAgree(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		// The table above still holds the Go side to its values, so a machine with
		// no node loses the cross-check and nothing else. Boris's has node; so does
		// any machine that can run `make build`.
		t.Skip("no node on this machine: the frontend half of the hash cannot be run")
	}
	pairs := make([][2]string, 0, len(identityHueSamples))
	for _, s := range identityHueSamples {
		pairs = append(pairs, [2]string{s.agent, s.project})
	}
	table, err := json.Marshal(pairs)
	if err != nil {
		t.Fatalf("marshal table: %v", err)
	}
	src := filepath.Join("..", "..", "frontend", "src", "lib", "tokens.js")
	out, err := exec.Command(node, "-e", hueShim, src, string(table)).CombinedOutput()
	if err != nil {
		t.Fatalf("run the frontend hue: %v\n%s", err, out)
	}
	var got [][2]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("read the frontend hue: %v\n%s", err, out)
	}
	if len(got) != len(identityHueSamples) {
		t.Fatalf("got %d hues for %d identities", len(got), len(identityHueSamples))
	}
	for i, s := range identityHueSamples {
		if got[i][0] != s.dark || got[i][1] != s.light {
			t.Errorf("%q/%q: frontend says %s / %s, Go says %s / %s",
				s.agent, s.project, got[i][0], got[i][1], s.dark, s.light)
		}
	}
}

// A NUL in the separator is how FR85 stayed hidden for as long as it did: it made
// tokens.js read as binary, so grep skipped the second implementation without
// printing anything a session would take for a warning. This asserts against the
// bytes go:embed ships rather than the source, so a fix that was never rebuilt into
// `dist` (the trap in CLAUDE.md) fails here.
func TestTheShippedBundleCarriesNoNULSeparator(t *testing.T) {
	err := fs.WalkDir(frontend.Dist, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".js") {
			return err
		}
		data, err := fs.ReadFile(frontend.Dist, path)
		if err != nil {
			return err
		}
		if i := strings.IndexByte(string(data), 0); i >= 0 {
			t.Errorf("%s carries a NUL at byte %d: run `make build` if tokens.js is already fixed", path, i)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk dist: %v", err)
	}
}

// FR86: one repo is one project, so one agent in a repo is one colour wherever in
// it that agent stands. This is the pair the board actually showed - `aider · src`
// beside a peer at the root, two rows for two directories of one checkout.
func TestOneRepoIsOneIdentityColour(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "frontend", "src", "lib")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}
	atRoot, inDeep := daemon.DeriveProject(root), daemon.DeriveProject(deep)
	if atRoot != filepath.Base(root) {
		t.Errorf("project at the root = %q, want %q", atRoot, filepath.Base(root))
	}
	if inDeep != atRoot {
		t.Errorf("project in %s = %q, want %q - a subdirectory is not a project", deep, inDeep, atRoot)
	}
	if a, b := IdentityHue("aider", atRoot, true), IdentityHue("aider", inDeep, true); a != b {
		t.Errorf("one agent, two colours: %s at the root, %s in a subdirectory", a, b)
	}
	// Outside a repo the directory is the only honest name, and no cwd has none.
	plain := t.TempDir()
	if got := daemon.DeriveProject(plain); got != filepath.Base(plain) {
		t.Errorf("DeriveProject(%q) = %q, want %q", plain, got, filepath.Base(plain))
	}
	if got := daemon.DeriveProject(""); got != "" {
		t.Errorf("DeriveProject(\"\") = %q, want empty", got)
	}
}

// FR84's threshold is one number in two files: the surface draws the extra line,
// webui.go leaves room for it in the window the card opens at. Same failure mode
// as the identity hue above, so the same answer - a test rather than a comment
// asking the next session nicely.
func TestTheSpellThresholdIsOneNumber(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "surfaces", "Card.svelte"))
	if err != nil {
		t.Fatalf("read Card.svelte: %v", err)
	}
	m := regexp.MustCompile(`SPELL_AT\s*=\s*(\d+)`).FindSubmatch(src)
	if m == nil {
		t.Fatal("Card.svelte no longer declares SPELL_AT; the two sides can now drift silently")
	}
	want, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("SPELL_AT is not a number: %q", m[1])
	}
	if want != spelledAt {
		t.Errorf("Card.svelte says SPELL_AT = %d, webui.go says spelledAt = %d", want, spelledAt)
	}
}
