package mcp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
)

func TestAskToItemChoiceVsText(t *testing.T) {
	c := askToItem(askIn{Title: "Deploy?", Options: []string{"Yes", "No"}, TimeoutS: 30, Default: "No"})
	if c.Kind != proto.KindChoice || len(c.Options) != 2 || c.Options[0].Label != "Yes" {
		t.Fatalf("options should map to a choice: %+v", c)
	}
	if c.TimeoutS != 30 || c.Default != "No" {
		t.Fatalf("timeout/default not carried: %+v", c)
	}
	txt := askToItem(askIn{Title: "Tag?"})
	if txt.Kind != proto.KindText {
		t.Fatalf("no options should map to free text, got %q", txt.Kind)
	}
}

// Every tool that BLOCKS has to tell the model how the question ended, not just
// whether it was answered (R-09): "he declined" and "nobody was there" call for
// opposite behaviour, and an agent that cannot tell them apart either re-asks a
// human who said no or gives up on a human who was in a meeting. The field is
// plumbed per tool, so this is what catches the sixth one added without it.
func TestEveryBlockingToolReportsHowItEnded(t *testing.T) {
	for _, out := range []any{askOut{}, confirmOut{}, formOut{}, secretOut{}, reviewOut{}} {
		typ := reflect.TypeOf(out)
		found := false
		for field := range typ.Fields() {
			if name, _, _ := strings.Cut(field.Tag.Get("json"), ","); name == "outcome" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s has no outcome field, so its caller cannot tell an expiry from a refusal", typ.Name())
		}
	}
	// veto is the exception and needs no field: `vetoed` already says which of its two
	// endings happened, and a window that lapsed IS the action proceeding.
	if reflect.TypeFor[vetoOut]().NumField() != 1 {
		t.Error("vetoOut grew a field; decide whether it now needs an outcome too")
	}
}

// show_artifact rides Show, so the only thing worth pinning is what it turns a
// tool call into: an artifact request, and a watch that only means something when
// there is a file behind it.
func TestArtifactRequestShape(t *testing.T) {
	s := &server{}
	req, err := s.artifactRequest(artifactIn{HTML: "<p>hi</p>", Title: "Panel", Watch: true})
	if err != nil {
		t.Fatalf("inline html: %v", err)
	}
	if !req.Artifact || req.Content != "<p>hi</p>" || req.Title != "Panel" {
		t.Fatalf("inline request: %+v", req)
	}
	if req.Watch {
		t.Error("inline html has nothing on disk to watch")
	}

	req, err = s.artifactRequest(artifactIn{Path: "app.html", Watch: true})
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if !req.Watch || !req.Artifact {
		t.Fatalf("a watched file should stay watched: %+v", req)
	}
	if !filepath.IsAbs(req.Path) {
		t.Errorf("path should be absolute for the daemon, got %q", req.Path)
	}

	if _, err := s.artifactRequest(artifactIn{}); err == nil {
		t.Error("an artifact with neither html nor a path should be refused")
	}
}

// TestSpecToWalkthrough pins the create request's shape without a daemon:
// the id is minted tool-side (awaitable the moment create returns) and an
// empty spec is refused before it travels.
func TestSpecToWalkthrough(t *testing.T) {
	req, err := specToWalkthrough(map[string]any{"version": 1}, true, proto.Identity{Agent: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if !proto.ValidWalkthroughID(req.ID) {
		t.Errorf("minted id %q is not caller-shaped", req.ID)
	}
	if !req.NoShow || req.Identity.Agent != "a" || string(req.Spec) != `{"version":1}` {
		t.Errorf("request: %+v", req)
	}
	for _, empty := range []map[string]any{nil, {}} {
		if _, err := specToWalkthrough(empty, false, proto.Identity{}); err == nil {
			t.Errorf("spec %v should be refused tool-side", empty)
		}
	}
}

// TestManualListsEveryTool holds the docs to the code: every MCP tool
// registered in Serve must appear in both manuals - the full reference and
// the embedded quickstart. Tool names are read from the source so a new
// AddTool cannot ship undocumented.
// The three answers to a handover request mean different things to a caller and
// have to survive the trip out separately: granted is the desktop, denied is a
// no, held_by is somebody else driving. Collapsing any pair of them would have
// an agent drive a desktop it was not given.
func TestControlOutKeepsTheThreeAnswersApart(t *testing.T) {
	granted := out(proto.ControlResult{OK: true, Live: true, Granted: true, State: proto.ControlDriving})
	if !granted.Granted || granted.Denied || granted.HeldBy != "" || granted.State != proto.ControlDriving {
		t.Errorf("granted came out as %+v", granted)
	}
	denied := out(proto.ControlResult{OK: true, Denied: true})
	if denied.Granted || !denied.Denied || denied.Live {
		t.Errorf("denied came out as %+v", denied)
	}
	held := out(proto.ControlResult{OK: true, Live: true, HeldBy: "claude-code", Reason: "driving chrome"})
	if held.Granted || held.Denied || held.HeldBy != "claude-code" || held.Reason != "driving chrome" {
		t.Errorf("held came out as %+v", held)
	}
	act := out(proto.ControlResult{OK: true, Live: true, Granted: true, Activity: "scrolling the board"})
	if act.Activity != "scrolling the board" {
		t.Errorf("activity came out as %+v", act)
	}
}

// A blank reason and a blank activity are refused before the daemon is dialled:
// the reason is the only thing the human reads before handing over their screen,
// and an empty activity line would blank a strip that is meant to be narrating.
func TestControlRefusesEmptyTextWithoutADaemon(t *testing.T) {
	s := &server{runtimeDir: filepath.Join(t.TempDir(), "no-daemon")}
	res, _, err := s.controlRequest(t.Context(), nil, controlIn{Reason: "  "})
	if err != nil || res == nil || !res.IsError {
		t.Errorf("blank reason: res = %+v, err = %v; want a tool error", res, err)
	}
	res, _, err = s.controlActivity(t.Context(), nil, activityIn{Activity: ""})
	if err != nil || res == nil || !res.IsError {
		t.Errorf("blank activity: res = %+v, err = %v; want a tool error", res, err)
	}
}

func TestManualListsEveryTool(t *testing.T) {
	// Every source file in the package, not just mcp.go: a family of tools that
	// grew its own file (assignments.go) would otherwise be exempt from the one
	// check that keeps the manuals honest.
	srcs, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var src []byte
	for _, f := range srcs {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src = append(src, b...)
	}
	// Anchored on sdk.Tool so the resources and the prompt in standards.go,
	// which are documented elsewhere, are not mistaken for tools.
	names := regexp.MustCompile(`sdk\.Tool\{\s*Name:\s+"([a-z_]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(names) < 10 {
		t.Fatalf("found only %d registered tools in the package; the pattern has drifted", len(names))
	}
	for _, doc := range []string{
		filepath.Join("..", "..", "docs", "agent-manual.md"),
		filepath.Join("..", "manual", "agent.md"),
	} {
		text, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range names {
			if !strings.Contains(string(text), m[1]) {
				t.Errorf("%s does not mention the %s tool", doc, m[1])
			}
		}
	}
}

func TestFormToItem(t *testing.T) {
	it := formToItem(formIn{Title: "Release", Fields: []formFieldIn{
		{Key: "env", Type: "choice", Options: []string{"staging", "prod"}, Default: "staging"},
		{Key: "tag", Type: "text"},
	}})
	if it.Kind != proto.KindForm || len(it.Fields) != 2 {
		t.Fatalf("form mapping: %+v", it)
	}
	if it.Fields[0].Type != proto.FieldChoice || it.Fields[0].Options[1] != "prod" || it.Fields[0].Default != "staging" {
		t.Fatalf("choice field: %+v", it.Fields[0])
	}
	if it.Fields[1].Key != "tag" || it.Fields[1].Type != proto.FieldText {
		t.Fatalf("text field: %+v", it.Fields[1])
	}
}

// request_review reading a path is R-16's fourth door, in the child rather than
// the daemon: /dev/zero here kills the process that owns every one of this agent's
// tools. The refusal has to arrive before the daemon is dialled, which is what this
// test relies on - the server it uses has no daemon to reach.
func TestReviewRefusesAPathThatIsNotAFile(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("no /dev/zero")
	}
	s := &server{runtimeDir: filepath.Join(t.TempDir(), "no-daemon")}

	res, _, err := s.review(context.Background(), nil, reviewIn{Title: "R-16", Path: "/dev/zero"})
	if err != nil {
		t.Fatalf("a refusal belongs in the result, not in the transport: %v", err)
	}
	if !res.IsError {
		t.Fatal("/dev/zero was accepted as a diff")
	}
	if text := resultText(res); !strings.Contains(text, "not a regular file") {
		t.Errorf("result = %q, want the rule that refused it", text)
	}
}
