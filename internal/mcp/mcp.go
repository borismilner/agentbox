// Package mcp serves agentbox over the Model Context Protocol (ADR-0004): a
// stdio server (`agentbox mcp`) whose tools proxy to the daemon socket, so it
// inherits auto-spawn. Tool descriptions are written for the model - they
// are the only documentation an MCP host shows it.
package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/agentbox/internal/client"
	"github.com/borismilner/agentbox/internal/proto"
)

type server struct {
	runtimeDir string
	id         proto.Identity

	// base is Serve's context, which lives as long as this child does. The
	// attach stream (sync.go) needs a context outliving any single tool call,
	// because presence is the whole session and not one question.
	base   context.Context
	attach attachState
}

// Serve runs the MCP stdio server until the client disconnects or ctx is
// cancelled. id is the default caller identity stamped on every item.
func Serve(ctx context.Context, runtimeDir, version string, id proto.Identity) error {
	s := &server{runtimeDir: runtimeDir, id: id, base: ctx}
	srv := sdk.NewServer(&sdk.Implementation{Name: "agentbox", Version: version}, nil)
	// FR83's discovery rider: news about company, appended to whatever tool
	// result it came back on (rider.go).
	srv.AddReceivingMiddleware(riderMiddleware)
	// The standards an agent can ask for mid-task (standards.go): resources and
	// a prompt, so a review kit does not have to be written from memory.
	addStandards(srv)
	// The roster family (FR83, sync.go). Registered unconditionally in slice 1:
	// the design's [sync] enabled flag exists to keep eight always-refusing
	// schemas out of a session's context, and with two harmless read-and-declare
	// tools there is nothing yet to gate.
	addSyncTools(srv, s)

	sdk.AddTool(srv, &sdk.Tool{
		Name:        "notify_user",
		Description: "Post a desktop notification to the human. Fire-and-forget: returns at once, never blocks. Use for progress and results, not for questions.",
	}, s.notify)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "ask_user",
		Description: "Ask the human a question and BLOCK until they answer. Give 2-9 options for a single choice, or omit options for free text. Only interrupt for a decision you cannot make safely yourself.",
	}, s.ask)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "confirm_action",
		Description: "Ask the human to confirm an action (yes/no) and BLOCK until they answer. Returns confirmed=true or false.",
	}, s.confirm)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "act_unless_stopped",
		Description: "Announce an action with a countdown and BLOCK. It proceeds automatically when the window elapses unless the human stops it; returns vetoed=true if they stopped it. Prefer this over confirm_action when proceeding is the expected outcome.",
	}, s.veto)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "ask_user_form",
		Description: "Ask the human to fill a small form (up to 6 typed fields: choice, text or bool) in one card, and BLOCK until they submit. Returns the field values.",
	}, s.form)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "request_secret",
		Description: "Ask the human for a secret (masked entry) and BLOCK. The value is written to a file (mode 0600) and is NEVER returned here - the result is the file path. Read the file when you need the value; never print or echo it.",
	}, s.requestSecret)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "show_document",
		Description: "Open a markdown document in AgentBox's reading window: rich rendering of headings, code, tables and alerts, far better than a terminal. Pass a file path or inline content. Non-blocking.",
	}, s.showDocument)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "show_artifact",
		Description: "Show an interactive HTML artifact in a window and RUN it: charts, calculators, prototypes, control panels, anything the human should use rather than read. " +
			"Write it as a self-contained HTML document, or as a React component module (export default) - React and Tailwind are already available, so import react and use utility classes; " +
			"do NOT link a CDN or any other package, the sandbox has no network at all. " +
			"Call window.agentbox.emit(name, data) anywhere in it and you receive that through await_artifact_event, so the human clicking in the artifact is how you decide what to do next. " +
			"Non-blocking: returns at once with the artifact_id to wait on. Pass a path instead of html to show a file, with watch=true to re-run it on every save.",
	}, s.showArtifact)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "await_artifact_event",
		Description: "BLOCK until the human does something in an artifact you showed - a click, a slider, a submit - and return what they did (the name and data it passed to agentbox.emit). " +
			"Pass the artifact_id show_artifact returned. This is how an artifact drives real work: they choose, you act, then show or update the artifact again. " +
			"Returns received=false with timed_out=true if the window elapses.",
	}, s.awaitArtifact)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "read_artifact_events",
		Description: "Take everything the human has done in an artifact since you last looked, without blocking. Repeated events of one name are coalesced to the newest, " +
			"so a slider dragged for a while is one final value rather than forty. Use this while you are working; use await_artifact_event when you have nothing to do but wait.",
	}, s.readArtifact)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "speak",
		Description: "Say one line out loud, if the human turned speech on. Non-blocking by default, and it creates no card, no notification and no inbox entry - " +
			"use it when something is worth hearing but not worth interrupting for, or to talk a human through a long job they are only half watching. " +
			"For anything that needs a card, put the same sentence in that tool's speak field instead so the chime and the voice arrive together. " +
			"Set wait when the next thing you do has to land after this line is heard - a sequence of lines, or a line that should finish before a card appears. " +
			"Write what you would say to somebody in the next room; it does nothing when speech is off.",
	}, s.speak)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "drive_desktop",
		Description: "Move the pointer, click, drag, scroll and type on the human's desktop, as if they did it themselves - real synthetic input, so any application accepts it. " +
			"Use it to do the thing rather than describe it: open a menu, fill a field, click the button in a window you just showed, demonstrate a workflow while they watch. " +
			"The script is one step per line: `window TITLE` (make the coordinates below relative to that window; prefix = to require an exact title), `screen`, " +
			"`move X Y`, `click [button|X Y [button]]`, `double`, `drag X1 Y1 X2 Y2`, `scroll N` (negative scrolls up), `type TEXT` (rest of the line, verbatim), " +
			"`key ctrl+alt+t` (also Escape, Return, Tab, End, arrows), `wait MS`, `speed N`, `wpm N`. " +
			"Coordinates: 400 from the near edge, -46 from the FAR edge (a card's buttons sit a fixed distance from its bottom), 60% across, center, ~ the pointer, ~+30 relative to it. " +
			"The whole script is validated before the first event, movements follow a human curve rather than a jump, and AgentBox logs the shape of every script but never the text it typed. " +
			"`window TITLE` is a target lock, not just a coordinate frame: it raises that window, follows it if it moves, and checks before EVERY click that the pointer is really over it and before every type/key that the keyboard is really in it. " +
			"A mismatch raises the window and tries once more, then fails the step naming what was there instead - so a window that closed, moved or was covered stops the script rather than sending your keystrokes into somebody else's document. " +
			"Name the window you mean, and the rest is checked for you; without one (`screen`) nothing is enforced, because there is nothing to compare against. " +
			"It is the human's own desktop, so drive it as sparingly as you would interrupt them.",
	}, s.drive)
	// The desktop handover (FR74). Three verbs rather than one tool with a mode,
	// because they differ in the only way that matters to a caller: the first
	// blocks on the human, the other two never do.
	sdk.AddTool(srv, &sdk.Tool{
		Name: "request_control",
		Description: "Ask for the human's desktop and BLOCK until they allow it. Use this BEFORE any run of drive_desktop longer than a step or two, and before driving anything else on their screen (a browser you automate, a window you photograph). " +
			"While you hold it, one always-on-top HANDS OFF strip stays on their screen saying who is driving and what they are doing - its presence is the whole signal, so it must live exactly as long as your run does. " +
			"Silence for window_s seconds counts as consent (they can allow it early or deny it outright). Returns granted=true, or denied=true, or held_by naming the agent that already has the desktop - one desktop cannot be shared, so on held_by wait and ask again rather than driving anyway. " +
			"Call set_activity as you move on, and release_control the moment you stop: a card or a spoken line will not do this job, because both are over before the driving starts.",
	}, s.controlRequest)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "set_activity",
		Description: "Say what you are doing RIGHT NOW, in one line, and keep it current as the work changes. Non-blocking, and it is cheap: call it whenever you move on to something else. " +
			"It writes your row on the human's Agents board always, so they can see every session is moving rather than stuck - and it additionally writes the HANDS OFF strip while you hold the desktop. " +
			"Re-sending an unchanged line deliberately does not reset its age, because repeating yourself is not progress. Announce first, so the line has a purpose to sit under.",
	}, s.controlActivity)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "release_control",
		Description: "Give the desktop back: ends the run and takes the hands-off strip off the screen, which is how the human learns they can touch things again. Non-blocking. " +
			"Call it as soon as you stop driving, including on the paths where the run failed - a strip left up claims hands-off for work that is over.",
	}, s.controlRelease)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "request_review",
		Description: "Show a unified diff for the human to approve or request changes, and BLOCK until they decide. Returns approved=true/false and any comment. Use before applying a patch the human should see.",
	}, s.review)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "report_progress",
		Description: "Report progress on a long-running task as a live bar (non-blocking, never steals focus). Call once to start (omit id, set title) and pass the returned id on every later call to update percent (0-100), or set indeterminate=true for a spinner when there is no known fraction. Call with done=true when finished - set error to report failure. Prefer this over repeated notify_user calls for incremental progress.",
	}, s.reportProgress)

	// The walkthrough family (FR58/FR59): durable step-by-step reviews the
	// human walks on the board and hands back in one turn.
	sdk.AddTool(srv, &sdk.Tool{
		Name: "create_walkthrough",
		Description: "Create a durable step-by-step code review (a walkthrough) and open it on the human's review board. " +
			"You supply a declarative spec: ordered steps, each with prose and citations {path, lines:[from,to]} into the repo at repo_root, pinned to a commit; " +
			"include the change's unified diff in the spec and AgentBox derives every added/removed marking from it - NEVER state diff status on a file-backed block, and NEVER put literal line numbers in prose (bind a phrase to a code region instead; the spec rejects both, with directions). " +
			"The human marks each step understood or unclear, writes notes and anchors comments to lines; everything persists in AgentBox's store, so the review outlives your session and theirs. " +
			"Non-blocking: returns the walkthrough id at once - call await_walkthrough with it to block until they submit. " +
			"BEFORE writing one, read the MCP resource agentbox://standards/walkthrough - it is the standard for how to structure the steps, " +
			"where the explanation goes versus the line annotations, and what coverage has to account for. Full spec reference: agentbox://manual/agent.",
	}, s.wtCreate)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "await_walkthrough",
		Description: "BLOCK until the human submits their review of a walkthrough, and return the whole handback in one turn: " +
			"unclear steps first (each with the note saying what is unclear), then every step's verdict, notes and line-anchored comments, and the not_reviewed list. " +
			"A submission made while nobody waited is also claimed here, exactly once. Returns timed_out=true if the window elapses, gone=true if the walkthrough was deleted.",
	}, s.wtAwait)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "read_walkthrough",
		Description: "Fetch a walkthrough's full stored state without blocking: the spec, the human's marks and comments so far, and the last submission payload if any. " +
			"Set ack=true to take a waiting submission exactly once - use that in a fresh session to pick up a review submitted after its agent was gone.",
	}, s.wtRead)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "list_walkthroughs",
		Description: "List stored walkthroughs, most recently touched first: state (open, submitted, delivered), how far the human got, and comment counts. find matches title, step content and cited paths.",
	}, s.wtList)
	sdk.AddTool(srv, &sdk.Tool{
		Name: "amend_walkthrough",
		Description: "Revise a stored walkthrough by step id without touching the human's marks. NOT available in this build yet: it refuses with directions (create a fresh walkthrough for revised content), " +
			"and a submitted, unread review is never overwritten either way - take the handback first.",
	}, s.wtAmend)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "delete_walkthrough",
		Description: "Delete a stored walkthrough permanently, marks and comments included. An agent awaiting it is released with gone=true. Prefer leaving finished reviews in the library - they are the human's record too.",
	}, s.wtDelete)

	// The assignment family (M12/FR82, assignments.go): the work agentbox gives an
	// agent on a schedule, which an agent also authors and edits.
	addAssignmentTools(srv, s)

	return srv.Run(ctx, &sdk.StdioTransport{})
}

// call sends one item to the daemon, auto-spawning it if needed. The dial
// has a short deadline; the call itself rides ctx, since a blocking question
// may take the human minutes (the item's own timeout_s bounds it).
func (s *server) call(ctx context.Context, method string, it *proto.Item) (proto.Result, error) {
	it.Identity = s.id
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := client.Dial(dialCtx, s.runtimeDir, nil)
	cancel()
	if err != nil {
		return proto.Result{}, fmt.Errorf("cannot reach agentbox daemon: %w", err)
	}
	defer conn.Close()
	var res proto.Result
	rider, err := conn.CallRidden(ctx, method, it, &res)
	if err != nil {
		return proto.Result{}, err
	}
	noteRider(ctx, rider)
	return res, nil
}

// errResult reports a transport/validation failure as a tool error the model
// can read, rather than a protocol-level error.
func errResult[T any](err error) (*sdk.CallToolResult, T, error) {
	var zero T
	return &sdk.CallToolResult{
		IsError: true,
		Content: []sdk.Content{&sdk.TextContent{Text: "agentbox: " + err.Error()}},
	}, zero, nil
}

type notifyAction struct {
	Label string `json:"label" jsonschema:"button text"`
	Exec  string `json:"exec" jsonschema:"shell command run when the user clicks; runs in your working directory; the user sees it verbatim on hover"`
}
type notifyIn struct {
	Title   string         `json:"title" jsonschema:"the headline shown to the user"`
	Body    string         `json:"body,omitempty" jsonschema:"optional markdown detail"`
	Level   string         `json:"level,omitempty" jsonschema:"info, success, warning, error or urgent; default info"`
	Actions []notifyAction `json:"actions,omitempty" jsonschema:"up to 3 buttons that run a local command on click, e.g. open a PR or tail a log"`
	Speak   string         `json:"speak,omitempty" jsonschema:"one short line read out loud when this appears, if the human turned speech on. Write what you would say to somebody in the next room - the point, not the title again. Omit it and AgentBox only chimes."`
}
type notifyOut struct {
	ID string `json:"id"`
}

func (s *server) notify(ctx context.Context, _ *sdk.CallToolRequest, in notifyIn) (*sdk.CallToolResult, notifyOut, error) {
	it := &proto.Item{Kind: proto.KindNotify, Level: proto.Level(in.Level), Title: in.Title, Body: in.Body, Speak: in.Speak}
	for _, a := range in.Actions {
		it.Actions = append(it.Actions, proto.Action{Label: a.Label, Exec: a.Exec})
	}
	if len(it.Actions) > 0 {
		if wd, err := os.Getwd(); err == nil {
			it.Cwd = wd
		}
	}
	res, err := s.call(ctx, proto.MethodNotify, it)
	if err != nil {
		return errResult[notifyOut](err)
	}
	return &sdk.CallToolResult{}, notifyOut{ID: res.ID}, nil
}

type askIn struct {
	Title    string   `json:"title" jsonschema:"the question"`
	Body     string   `json:"body,omitempty" jsonschema:"optional markdown context"`
	Options  []string `json:"options,omitempty" jsonschema:"2-9 choices; omit for a free-text answer"`
	TimeoutS int      `json:"timeout_s,omitempty" jsonschema:"seconds before the default applies; 0 waits forever"`
	Default  string   `json:"default,omitempty" jsonschema:"answer applied on timeout"`
	Speak    string   `json:"speak,omitempty" jsonschema:"one short line read out loud when this appears, if the human turned speech on. Write what you would say to somebody in the next room - the point, not the title again. Omit it and AgentBox only chimes."`
}
type askOut struct {
	Answered       bool   `json:"answered"`
	Answer         string `json:"answer,omitempty"`
	Reply          string `json:"reply,omitempty" jsonschema:"set when the user typed free text instead of choosing an option"`
	DefaultApplied bool   `json:"default_applied,omitempty"`
}

func askToItem(in askIn) *proto.Item {
	it := &proto.Item{Title: in.Title, Body: in.Body, TimeoutS: in.TimeoutS, Default: in.Default, Speak: in.Speak}
	if len(in.Options) > 0 {
		it.Kind = proto.KindChoice
		for _, o := range in.Options {
			it.Options = append(it.Options, proto.Option{Label: o})
		}
	} else {
		it.Kind = proto.KindText
	}
	return it
}

func (s *server) ask(ctx context.Context, _ *sdk.CallToolRequest, in askIn) (*sdk.CallToolResult, askOut, error) {
	res, err := s.call(ctx, proto.MethodAsk, askToItem(in))
	if err != nil {
		return errResult[askOut](err)
	}
	return &sdk.CallToolResult{}, askOut{
		Answered: res.Answered, Answer: res.Answer, Reply: res.Reply, DefaultApplied: res.DefaultApplied,
	}, nil
}

type confirmIn struct {
	Title string `json:"title" jsonschema:"what to confirm, phrased as a yes/no question"`
	Body  string `json:"body,omitempty" jsonschema:"optional markdown context"`
	Speak string `json:"speak,omitempty" jsonschema:"one short line read out loud when this appears, if the human turned speech on. Write what you would say to somebody in the next room - the point, not the title again. Omit it and AgentBox only chimes."`
}
type confirmOut struct {
	Answered  bool   `json:"answered"`
	Confirmed bool   `json:"confirmed"`
	Reply     string `json:"reply,omitempty" jsonschema:"set when the user replied with free text instead of yes/no"`
}

func (s *server) confirm(ctx context.Context, _ *sdk.CallToolRequest, in confirmIn) (*sdk.CallToolResult, confirmOut, error) {
	res, err := s.call(ctx, proto.MethodAsk, &proto.Item{Kind: proto.KindConfirm, Title: in.Title, Body: in.Body, Speak: in.Speak})
	if err != nil {
		return errResult[confirmOut](err)
	}
	return &sdk.CallToolResult{}, confirmOut{
		Answered: res.Answered, Confirmed: res.Answered && res.Answer == "yes", Reply: res.Reply,
	}, nil
}

type vetoIn struct {
	Title   string `json:"title" jsonschema:"the action about to happen"`
	Body    string `json:"body,omitempty" jsonschema:"optional markdown context"`
	WindowS int    `json:"window_s,omitempty" jsonschema:"countdown seconds before it proceeds; default 15"`
	Speak   string `json:"speak,omitempty" jsonschema:"one short line read out loud when this appears, if the human turned speech on. Write what you would say to somebody in the next room - the point, not the title again. Omit it and AgentBox only chimes."`
}
type vetoOut struct {
	Vetoed bool `json:"vetoed"`
}

func (s *server) veto(ctx context.Context, _ *sdk.CallToolRequest, in vetoIn) (*sdk.CallToolResult, vetoOut, error) {
	// timeout_s 0 lets the daemon fill its configured window ([veto]).
	it := &proto.Item{Kind: proto.KindVeto, Level: proto.LevelWarning, Title: in.Title, Body: in.Body, TimeoutS: in.WindowS, Speak: in.Speak}
	res, err := s.call(ctx, proto.MethodAsk, it)
	if err != nil {
		return errResult[vetoOut](err)
	}
	return &sdk.CallToolResult{}, vetoOut{Vetoed: res.Vetoed}, nil
}

type formFieldIn struct {
	Key     string   `json:"key" jsonschema:"unique field key, used in the result"`
	Label   string   `json:"label,omitempty" jsonschema:"shown to the user; defaults to key"`
	Type    string   `json:"type" jsonschema:"choice, text or bool"`
	Options []string `json:"options,omitempty" jsonschema:"2-9 choices, required for choice fields"`
	Default string   `json:"default,omitempty" jsonschema:"default value"`
}
type formIn struct {
	Title    string        `json:"title" jsonschema:"the form heading"`
	Body     string        `json:"body,omitempty" jsonschema:"optional markdown context"`
	Fields   []formFieldIn `json:"fields" jsonschema:"1-6 typed fields"`
	TimeoutS int           `json:"timeout_s,omitempty" jsonschema:"seconds before the form expires unanswered"`
	Speak    string        `json:"speak,omitempty" jsonschema:"one short line read out loud when this appears, if the human turned speech on. Write what you would say to somebody in the next room - the point, not the title again. Omit it and AgentBox only chimes."`
}
type formOut struct {
	Answered bool              `json:"answered"`
	Values   map[string]string `json:"values,omitempty"`
}

func formToItem(in formIn) *proto.Item {
	it := &proto.Item{Kind: proto.KindForm, Title: in.Title, Body: in.Body, TimeoutS: in.TimeoutS, Speak: in.Speak}
	for _, f := range in.Fields {
		it.Fields = append(it.Fields, proto.Field{
			Key: f.Key, Label: f.Label, Type: proto.FieldType(f.Type), Options: f.Options, Default: f.Default,
		})
	}
	return it
}

func (s *server) form(ctx context.Context, _ *sdk.CallToolRequest, in formIn) (*sdk.CallToolResult, formOut, error) {
	res, err := s.call(ctx, proto.MethodAsk, formToItem(in))
	if err != nil {
		return errResult[formOut](err)
	}
	return &sdk.CallToolResult{}, formOut{Answered: res.Answered, Values: res.Values}, nil
}

type secretIn struct {
	Title    string `json:"title" jsonschema:"what secret to request"`
	Body     string `json:"body,omitempty" jsonschema:"optional markdown context"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"seconds before the prompt expires unanswered"`
	Speak    string `json:"speak,omitempty" jsonschema:"one short line read out loud when this appears, if the human turned speech on. Write what you would say to somebody in the next room - the point, not the title again. Omit it and AgentBox only chimes."`
}
type secretOut struct {
	Provided bool   `json:"provided"`
	Path     string `json:"path,omitempty" jsonschema:"file holding the value (mode 0600); read it when needed, never echo it"`
}

func (s *server) requestSecret(ctx context.Context, _ *sdk.CallToolRequest, in secretIn) (*sdk.CallToolResult, secretOut, error) {
	// The MCP server picks the sink so the value never returns through the
	// transcript; the daemon writes it there at 0600.
	sink, err := newSecretPath(s.runtimeDir)
	if err != nil {
		return errResult[secretOut](err)
	}
	it := &proto.Item{Kind: proto.KindSecret, Title: in.Title, Body: in.Body, Sink: sink, TimeoutS: in.TimeoutS, Speak: in.Speak}
	res, err := s.call(ctx, proto.MethodAsk, it)
	if err != nil {
		return errResult[secretOut](err)
	}
	return &sdk.CallToolResult{}, secretOut{Provided: res.Answered, Path: res.SecretPath}, nil
}

func newSecretPath(runtimeDir string) (string, error) {
	dir := filepath.Join(runtimeDir, "secrets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return filepath.Join(dir, hex.EncodeToString(b[:])+".secret"), nil
}

type showIn struct {
	Path    string `json:"path,omitempty" jsonschema:"path to a markdown file; resolved relative to your working directory"`
	Content string `json:"content,omitempty" jsonschema:"inline markdown to render, when there is no file"`
	Title   string `json:"title,omitempty" jsonschema:"window title"`
}
type showOut struct {
	Shown bool `json:"shown"`
}

func (s *server) showDocument(ctx context.Context, _ *sdk.CallToolRequest, in showIn) (*sdk.CallToolResult, showOut, error) {
	if in.Path == "" && in.Content == "" {
		return errResult[showOut](fmt.Errorf("show_document needs a path or content"))
	}
	req := proto.ShowRequest{Content: in.Content, Title: in.Title}
	if in.Path != "" {
		abs, err := filepath.Abs(in.Path)
		if err != nil {
			return errResult[showOut](err)
		}
		req.Path = abs
	}
	return s.sendShow(ctx, req)
}

type artifactIn struct {
	HTML  string `json:"html,omitempty" jsonschema:"the artifact itself: a self-contained HTML document, or a React component module with an export default"`
	Path  string `json:"path,omitempty" jsonschema:"path to an .html or .jsx file instead of inline html; resolved relative to your working directory"`
	Title string `json:"title,omitempty" jsonschema:"window title"`
	Watch bool   `json:"watch,omitempty" jsonschema:"with path: re-run the artifact whenever the file changes on disk"`
}
type artifactOut struct {
	Shown      bool   `json:"shown"`
	ArtifactID string `json:"artifact_id" jsonschema:"pass this to await_artifact_event or read_artifact_events to hear what the human does in it"`
}

func (s *server) showArtifact(ctx context.Context, _ *sdk.CallToolRequest, in artifactIn) (*sdk.CallToolResult, artifactOut, error) {
	req, err := s.artifactRequest(in)
	if err != nil {
		return errResult[artifactOut](err)
	}
	if _, _, err := s.sendShow(ctx, req); err != nil {
		return errResult[artifactOut](err)
	}
	return &sdk.CallToolResult{}, artifactOut{Shown: true, ArtifactID: req.ArtifactID}, nil
}

// artifactRequest also mints the id the artifact's events will carry. The caller
// mints it rather than the daemon so the tool can return it in the same breath as
// showing the window: an agent that has to ask what it just showed is an agent
// that can miss the click that happened in between.
func (s *server) artifactRequest(in artifactIn) (proto.ShowRequest, error) {
	if in.Path == "" && in.HTML == "" {
		return proto.ShowRequest{}, fmt.Errorf("show_artifact needs html or a path")
	}
	id, err := proto.NewArtifactID()
	if err != nil {
		return proto.ShowRequest{}, err
	}
	req := proto.ShowRequest{Content: in.HTML, Title: in.Title, Artifact: true, Watch: in.Watch, ArtifactID: id}
	if in.Path == "" {
		req.Watch = false // inline html has nothing on disk to watch
		return req, nil
	}
	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return proto.ShowRequest{}, err
	}
	req.Path = abs
	return req, nil
}

type awaitIn struct {
	ArtifactID string   `json:"artifact_id,omitempty" jsonschema:"the id show_artifact returned; omit to hear any artifact"`
	Names      []string `json:"names,omitempty" jsonschema:"only these event names; omit for any"`
	TimeoutS   int      `json:"timeout_s,omitempty" jsonschema:"seconds to wait before giving up; 0 waits as long as you do"`
}

// readIn is awaitIn without the timeout: a read does not wait, and offering a
// knob that does nothing is a way to be misunderstood.
type readIn struct {
	ArtifactID string   `json:"artifact_id,omitempty" jsonschema:"the id show_artifact returned; omit to take from any artifact"`
	Names      []string `json:"names,omitempty" jsonschema:"only these event names; omit for any"`
}
type eventOut struct {
	ArtifactID string `json:"artifact_id,omitempty"`
	Name       string `json:"name" jsonschema:"the first argument the artifact passed to agentbox.emit"`
	Data       any    `json:"data,omitempty" jsonschema:"the second argument, as the artifact sent it"`
	AtMS       int64  `json:"at_ms,omitempty"`
}
type awaitOut struct {
	Received bool      `json:"received"`
	Event    *eventOut `json:"event,omitempty"`
	TimedOut bool      `json:"timed_out,omitempty"`
}
type readOut struct {
	Events []eventOut `json:"events"`
}

func (s *server) awaitArtifact(ctx context.Context, _ *sdk.CallToolRequest, in awaitIn) (*sdk.CallToolResult, awaitOut, error) {
	var res proto.ArtifactWaitResult
	// No dial deadline on the call itself: waiting on a human is the point, and the
	// item's own timeout (or the caller's context) is what bounds it.
	if err := s.callInto(ctx, proto.MethodArtifactWait, proto.ArtifactWait{
		ArtifactID: in.ArtifactID, Names: in.Names, TimeoutS: in.TimeoutS,
	}, &res); err != nil {
		return errResult[awaitOut](err)
	}
	out := awaitOut{Received: res.Received, TimedOut: res.TimedOut}
	if res.Event != nil {
		ev := eventOut{ArtifactID: res.Event.ArtifactID, Name: res.Event.Name, AtMS: res.Event.AtMS}
		ev.Data = decodeEventData(res.Event.Data)
		out.Event = &ev
	}
	return &sdk.CallToolResult{}, out, nil
}

func (s *server) readArtifact(ctx context.Context, _ *sdk.CallToolRequest, in readIn) (*sdk.CallToolResult, readOut, error) {
	var res proto.ArtifactReadResult
	if err := s.callInto(ctx, proto.MethodArtifactRead, proto.ArtifactWait{
		ArtifactID: in.ArtifactID, Names: in.Names,
	}, &res); err != nil {
		return errResult[readOut](err)
	}
	out := readOut{Events: make([]eventOut, 0, len(res.Events))}
	for _, ev := range res.Events {
		out.Events = append(out.Events, eventOut{
			ArtifactID: ev.ArtifactID, Name: ev.Name, AtMS: ev.AtMS, Data: decodeEventData(ev.Data),
		})
	}
	return &sdk.CallToolResult{}, out, nil
}

// decodeEventData hands the artifact's payload on as itself rather than as a
// string of JSON: the model reads {"rows": 500}, not "{\"rows\": 500}". Anything
// that will not decode travels as the text it was, which is more use than nothing.
func decodeEventData(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}

// callInto is the plain request/response path for methods that are not items:
// dial, call, decode into out. The dial keeps a short deadline; the call itself
// rides ctx, because an artifact wait may take as long as the human does.
func (s *server) callInto(ctx context.Context, method string, params, out any) error {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := client.Dial(dialCtx, s.runtimeDir, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("cannot reach agentbox daemon: %w", err)
	}
	defer conn.Close()
	rider, err := conn.CallRidden(ctx, method, params, out)
	noteRider(ctx, rider)
	return err
}

// sendShow opens the reading window on a request. Show is one-way: it returns as
// soon as the daemon has the request, because a document nobody has read yet is
// not a question the agent is owed an answer to.
func (s *server) sendShow(ctx context.Context, req proto.ShowRequest) (*sdk.CallToolResult, showOut, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := client.Dial(dialCtx, s.runtimeDir, nil)
	cancel()
	if err != nil {
		return errResult[showOut](fmt.Errorf("cannot reach agentbox daemon: %w", err))
	}
	defer conn.Close()
	rider, err := conn.CallRidden(ctx, proto.MethodShow, &req, nil)
	noteRider(ctx, rider)
	if err != nil {
		return errResult[showOut](err)
	}
	return &sdk.CallToolResult{}, showOut{Shown: true}, nil
}

// speakWaitCap bounds a waiting speak call. The daemon has a ceiling of its own on
// any single line; this one is a little longer, so the daemon's answer is what a
// caller normally sees rather than a timeout on this side.
const speakWaitCap = 3 * time.Minute

type speakIn struct {
	Text string `json:"text" jsonschema:"the line to say out loud, a sentence or two at most"`
	Wait bool   `json:"wait,omitempty" jsonschema:"block until the line has finished playing instead of returning as soon as it is queued. Use it to narrate a sequence, where each line must be heard before the next begins. Leave it off for an ordinary aside - a spoken line should not cost you the seconds it takes to read."`
}
type speakOut struct {
	Spoken bool `json:"spoken" jsonschema:"the line reached the daemon; it is still silent if the human has speech off"`
	Waited bool `json:"waited,omitempty" jsonschema:"the call returned after the audio finished rather than after it was queued"`
}

func (s *server) speak(ctx context.Context, _ *sdk.CallToolRequest, in speakIn) (*sdk.CallToolResult, speakOut, error) {
	if strings.TrimSpace(in.Text) == "" {
		return errResult[speakOut](fmt.Errorf("speak needs text"))
	}
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := client.Dial(dialCtx, s.runtimeDir, nil)
	cancel()
	if err != nil {
		return errResult[speakOut](fmt.Errorf("cannot reach agentbox daemon: %w", err))
	}
	defer conn.Close()
	callCtx := ctx
	if in.Wait {
		var cancelCall context.CancelFunc
		callCtx, cancelCall = context.WithTimeout(ctx, speakWaitCap)
		defer cancelCall()
	}
	req := proto.SpeakRequest{Text: in.Text, Wait: in.Wait}
	rider, err := conn.CallRidden(callCtx, proto.MethodSpeak, &req, nil)
	noteRider(ctx, rider)
	if err != nil {
		return errResult[speakOut](err)
	}
	return &sdk.CallToolResult{}, speakOut{Spoken: true, Waited: in.Wait}, nil
}

type driveIn struct {
	Script string  `json:"script" jsonschema:"the steps to run, one per line: window/screen, move, click, double, drag, scroll, type, key, wait, speed, wpm"`
	Speed  float64 `json:"speed,omitempty" jsonschema:"movement speed multiplier; 1 is a hand's pace, 2 is twice as fast"`
	WPM    int     `json:"wpm,omitempty" jsonschema:"typing speed in words per minute (default 300)"`
}
type driveOut struct {
	Steps int `json:"steps" jsonschema:"how many steps ran"`
}

func (s *server) drive(ctx context.Context, _ *sdk.CallToolRequest, in driveIn) (*sdk.CallToolResult, driveOut, error) {
	if strings.TrimSpace(in.Script) == "" {
		return errResult[driveOut](fmt.Errorf("drive_desktop needs a script, for example: window =agentbox\\nclick 25%% -46"))
	}
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := client.Dial(dialCtx, s.runtimeDir, nil)
	cancel()
	if err != nil {
		return errResult[driveOut](fmt.Errorf("cannot reach agentbox daemon: %w", err))
	}
	defer conn.Close()
	req := proto.DriveRequest{Script: in.Script, Speed: in.Speed, WPM: in.WPM}
	var res proto.DriveResult
	// A script can legitimately run for a while (movements, typing, waits), so
	// this call does not get the short dial timeout the others use.
	rider, err := conn.CallRidden(ctx, proto.MethodDrive, &req, &res)
	noteRider(ctx, rider)
	if err != nil {
		return errResult[driveOut](err)
	}
	return &sdk.CallToolResult{}, driveOut{Steps: res.Steps}, nil
}

// The handover's three verbs (FR74). They share one wire method and one shape
// of call, so the differences worth writing down are which of them blocks and
// what the human sees when it returns.
type controlIn struct {
	Reason  string `json:"reason" jsonschema:"what you are about to do on their desktop, in their terms - it is what they read before allowing it, so say the work and not the mechanics"`
	WindowS int    `json:"window_s,omitempty" jsonschema:"seconds before silence counts as consent (default 20). Give a long run a longer window: the countdown is their chance to say no before you start, not a delay to get past"`
}
type activityIn struct {
	Activity string `json:"activity" jsonschema:"one short line saying what is happening right now, in the human's terms - it replaces the previous line on the strip and resets its age"`
}
type releaseIn struct{}

type controlOut struct {
	Granted  bool   `json:"granted,omitempty" jsonschema:"the desktop is yours until you release it"`
	Denied   bool   `json:"denied,omitempty" jsonschema:"they said no; do not drive anything"`
	Live     bool   `json:"live" jsonschema:"whether a run is on screen at all"`
	State    string `json:"state,omitempty" jsonschema:"asking or driving"`
	Activity string `json:"activity,omitempty" jsonschema:"the line the strip is showing"`
	HeldBy   string `json:"held_by,omitempty" jsonschema:"the other agent holding the desktop, when it is not you"`
	Reason   string `json:"reason,omitempty" jsonschema:"what that agent said it was doing"`
}

func out(res proto.ControlResult) controlOut {
	return controlOut{
		Granted: res.Granted, Denied: res.Denied, Live: res.Live,
		State: res.State, Activity: res.Activity, HeldBy: res.HeldBy, Reason: res.Reason,
	}
}

// control makes one handover call. request rides ctx like drive_desktop does,
// rather than the short dial budget: it is waiting out a countdown and then a
// human, and cutting it off would abandon the very strip it just put up.
func (s *server) control(ctx context.Context, req proto.ControlRequestParams) (controlOut, error) {
	req.Identity = s.id
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := client.Dial(dialCtx, s.runtimeDir, nil)
	cancel()
	if err != nil {
		return controlOut{}, fmt.Errorf("cannot reach agentbox daemon: %w", err)
	}
	defer conn.Close()
	var res proto.ControlResult
	rider, err := conn.CallRidden(ctx, proto.MethodControl, &req, &res)
	noteRider(ctx, rider)
	if err != nil {
		return controlOut{}, err
	}
	return out(res), nil
}

func (s *server) controlRequest(ctx context.Context, _ *sdk.CallToolRequest, in controlIn) (*sdk.CallToolResult, controlOut, error) {
	if strings.TrimSpace(in.Reason) == "" {
		return errResult[controlOut](fmt.Errorf("request_control needs a reason: it is what the human reads before handing over their desktop"))
	}
	res, err := s.control(ctx, proto.ControlRequestParams{
		Action: proto.ControlRequest, Reason: in.Reason, WindowS: in.WindowS,
	})
	if err != nil {
		return errResult[controlOut](err)
	}
	return &sdk.CallToolResult{}, res, nil
}

func (s *server) controlActivity(ctx context.Context, _ *sdk.CallToolRequest, in activityIn) (*sdk.CallToolResult, controlOut, error) {
	if strings.TrimSpace(in.Activity) == "" {
		return errResult[controlOut](fmt.Errorf("set_activity needs a line saying what is happening now"))
	}
	res, err := s.control(ctx, proto.ControlRequestParams{Action: proto.ControlActivity, Activity: in.Activity})
	if err != nil {
		return errResult[controlOut](err)
	}
	return &sdk.CallToolResult{}, res, nil
}

func (s *server) controlRelease(ctx context.Context, _ *sdk.CallToolRequest, _ releaseIn) (*sdk.CallToolResult, controlOut, error) {
	res, err := s.control(ctx, proto.ControlRequestParams{Action: proto.ControlRelease})
	if err != nil {
		return errResult[controlOut](err)
	}
	return &sdk.CallToolResult{}, res, nil
}

type reviewIn struct {
	Title string `json:"title" jsonschema:"what is being reviewed"`
	Diff  string `json:"diff,omitempty" jsonschema:"the unified diff text"`
	Path  string `json:"path,omitempty" jsonschema:"path to a diff file, read relative to your working directory; use instead of diff"`
	Body  string `json:"body,omitempty" jsonschema:"optional markdown context shown above the diff"`
	Speak string `json:"speak,omitempty" jsonschema:"one short line read out loud when this appears, if the human turned speech on. Write what you would say to somebody in the next room - the point, not the title again. Omit it and AgentBox only chimes."`
}
type reviewOut struct {
	Answered bool   `json:"answered"`
	Approved bool   `json:"approved"`
	Comment  string `json:"comment,omitempty"`
}

type progressIn struct {
	ID            string `json:"id,omitempty" jsonschema:"the report id returned by the first call; omit to start a new report"`
	Title         string `json:"title,omitempty" jsonschema:"the task name; set it on the first call"`
	Status        string `json:"status,omitempty" jsonschema:"a short status line shown under the bar"`
	Percent       int    `json:"percent,omitempty" jsonschema:"0-100 completion; omit and set indeterminate when unknown"`
	Indeterminate bool   `json:"indeterminate,omitempty" jsonschema:"true for a spinner when there is no known fraction"`
	Done          bool   `json:"done,omitempty" jsonschema:"true when the task is finished; emits a completion toast"`
	Error         string `json:"error,omitempty" jsonschema:"set together with done to report failure instead of success"`
}
type progressOut struct {
	ID string `json:"id" jsonschema:"pass this back as id on every later call for the same task"`
}

func (s *server) reportProgress(ctx context.Context, _ *sdk.CallToolRequest, in progressIn) (*sdk.CallToolResult, progressOut, error) {
	u := &proto.ProgressUpdate{
		ID: in.ID, Title: in.Title, Status: in.Status,
		Percent: in.Percent, Indeterminate: in.Indeterminate,
		Done: in.Done, Error: in.Error, Identity: s.id,
	}
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := client.Dial(dialCtx, s.runtimeDir, nil)
	cancel()
	if err != nil {
		return errResult[progressOut](fmt.Errorf("cannot reach agentbox daemon: %w", err))
	}
	defer conn.Close()
	var res proto.ProgressResult
	rider, err := conn.CallRidden(ctx, proto.MethodProgress, u, &res)
	noteRider(ctx, rider)
	if err != nil {
		return errResult[progressOut](err)
	}
	return &sdk.CallToolResult{}, progressOut{ID: res.ID}, nil
}

// --- the walkthrough family (FR58/FR59) --------------------------------

// Spec is map[string]any, not json.RawMessage. The SDK derives the tool's input
// schema from these types, and a []byte alias derives to an array of integers
// 0-255 - so the schema said "send me sixty thousand numbers" while the
// description said "send an object", and a client that validates arguments
// before sending could not call this tool at all (FR64). An object type derives
// to an object; the handler re-marshals it, which costs one pass over a document
// that is about to be parsed and validated anyway.
type wtCreateIn struct {
	Spec   map[string]any `json:"spec" jsonschema:"the version-1 walkthrough spec object: {version:1, title, repo_root (absolute), pinned (commit sha the citations are true against), base?, diff? (the change's unified diff - the only carrier of added/removed knowledge), out_of_scope?, glossary? ([{term, short, body?, also?}] - definitions the board marks in prose and opens on demand, so they stay out of the reading flow), steps:[{id, kind: ground|code|none|check, title, purpose, prose (segments; p:true starts a paragraph), code:[{path, lines:[from,to], notes:[{at:[from,to], text}]}], binds?, checks?, cmds?}]}. Read agentbox://standards/walkthrough first; full field reference in agentbox://manual/agent"`
	NoShow bool           `json:"no_show,omitempty" jsonschema:"store without opening the board window"`
}
type wtCreateOut struct {
	WalkthroughID string   `json:"walkthrough_id" jsonschema:"pass to await_walkthrough to block until the human submits, and to read/amend/delete"`
	Rev           int      `json:"rev"`
	Warnings      []string `json:"warnings,omitempty" jsonschema:"non-blocking teaching notes about the spec; fix them in your next review"`
}

// specToWalkthrough shapes the create request, minting the id tool-side so
// the caller can await the review the moment create returns and a retried
// create stays idempotent.
func specToWalkthrough(spec map[string]any, noShow bool, id proto.Identity) (proto.WalkthroughCreate, error) {
	if len(spec) == 0 {
		return proto.WalkthroughCreate{}, fmt.Errorf("create_walkthrough needs a spec object")
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return proto.WalkthroughCreate{}, fmt.Errorf("spec does not re-encode: %w", err)
	}
	wid, err := proto.NewWalkthroughID()
	if err != nil {
		return proto.WalkthroughCreate{}, err
	}
	return proto.WalkthroughCreate{ID: wid, Spec: raw, NoShow: noShow, Identity: id}, nil
}

func (s *server) wtCreate(ctx context.Context, _ *sdk.CallToolRequest, in wtCreateIn) (*sdk.CallToolResult, wtCreateOut, error) {
	req, err := specToWalkthrough(in.Spec, in.NoShow, s.id)
	if err != nil {
		return errResult[wtCreateOut](err)
	}
	var res proto.WalkthroughCreateResult
	if err := s.callInto(ctx, proto.MethodWalkthroughCreate, req, &res); err != nil {
		return errResult[wtCreateOut](err)
	}
	return &sdk.CallToolResult{}, wtCreateOut{WalkthroughID: res.ID, Rev: res.Rev, Warnings: res.Warnings}, nil
}

type wtAwaitIn struct {
	WalkthroughID string `json:"walkthrough_id,omitempty" jsonschema:"the id create_walkthrough returned; omit to take the next submission from any walkthrough"`
	TimeoutS      int    `json:"timeout_s,omitempty" jsonschema:"seconds to wait before giving up; 0 waits as long as you do"`
}
type wtAwaitOut struct {
	Submitted bool `json:"submitted"`
	Review    any  `json:"review,omitempty" jsonschema:"the whole handback: tally, unclear steps first (answer these), every step's verdict/note/comments, not_reviewed"`
	TimedOut  bool `json:"timed_out,omitempty"`
	Gone      bool `json:"gone,omitempty" jsonschema:"the walkthrough was deleted while you waited"`
}

func (s *server) wtAwait(ctx context.Context, _ *sdk.CallToolRequest, in wtAwaitIn) (*sdk.CallToolResult, wtAwaitOut, error) {
	var res proto.WalkthroughAwaitResult
	if err := s.callInto(ctx, proto.MethodWalkthroughAwait, proto.WalkthroughAwait{
		ID: in.WalkthroughID, TimeoutS: in.TimeoutS, Identity: s.id,
	}, &res); err != nil {
		return errResult[wtAwaitOut](err)
	}
	return &sdk.CallToolResult{}, wtAwaitOut{
		Submitted: res.Submitted, Review: decodeEventData(res.Payload),
		TimedOut: res.TimedOut, Gone: res.Gone,
	}, nil
}

type wtReadIn struct {
	WalkthroughID string `json:"walkthrough_id"`
	Ack           bool   `json:"ack,omitempty" jsonschema:"take a waiting submission, exactly once; false just looks"`
}
type wtReadOut struct {
	Walkthrough any `json:"walkthrough" jsonschema:"the stored review: spec, state, the human's marks and comments, and the last submission payload if any"`
}

func (s *server) wtRead(ctx context.Context, _ *sdk.CallToolRequest, in wtReadIn) (*sdk.CallToolResult, wtReadOut, error) {
	if in.WalkthroughID == "" {
		return errResult[wtReadOut](fmt.Errorf("read_walkthrough needs a walkthrough_id; list_walkthroughs finds one"))
	}
	var res json.RawMessage
	if err := s.callInto(ctx, proto.MethodWalkthroughRead, proto.WalkthroughRead{
		ID: in.WalkthroughID, Ack: in.Ack,
	}, &res); err != nil {
		return errResult[wtReadOut](err)
	}
	return &sdk.CallToolResult{}, wtReadOut{Walkthrough: decodeEventData(res)}, nil
}

type wtListIn struct {
	Find  string `json:"find,omitempty" jsonschema:"match title, step content or cited paths"`
	State string `json:"state,omitempty" jsonschema:"open, submitted or delivered; omit for all"`
	Limit int    `json:"limit,omitempty" jsonschema:"rows to return; default 20"`
}
type wtListOut struct {
	Walkthroughs []proto.WalkthroughSummary `json:"walkthroughs"`
}

func (s *server) wtList(ctx context.Context, _ *sdk.CallToolRequest, in wtListIn) (*sdk.CallToolResult, wtListOut, error) {
	var res proto.WalkthroughListResult
	if err := s.callInto(ctx, proto.MethodWalkthroughList, proto.WalkthroughList{
		Query: in.Find, State: in.State, Limit: in.Limit,
	}, &res); err != nil {
		return errResult[wtListOut](err)
	}
	return &sdk.CallToolResult{}, wtListOut{Walkthroughs: res.Walkthroughs}, nil
}

type wtAmendIn struct {
	WalkthroughID string          `json:"walkthrough_id"`
	ExpectRev     int             `json:"expect_rev" jsonschema:"the spec revision you believe is current; a mismatch refuses rather than clobbers"`
	Ops           json.RawMessage `json:"ops,omitempty" jsonschema:"amendment operations by step id"`
}
type wtAmendOut struct {
	Rev int `json:"rev"`
}

func (s *server) wtAmend(ctx context.Context, _ *sdk.CallToolRequest, in wtAmendIn) (*sdk.CallToolResult, wtAmendOut, error) {
	var res proto.WalkthroughAmendResult
	err := s.callInto(ctx, proto.MethodWalkthroughAmend, proto.WalkthroughAmend{
		ID: in.WalkthroughID, ExpectRev: in.ExpectRev, Identity: s.id,
	}, &res)
	if err != nil {
		return errResult[wtAmendOut](err)
	}
	return &sdk.CallToolResult{}, wtAmendOut{Rev: res.Rev}, nil
}

type wtDeleteIn struct {
	WalkthroughID string `json:"walkthrough_id"`
}
type wtDeleteOut struct {
	Deleted bool `json:"deleted"`
}

func (s *server) wtDelete(ctx context.Context, _ *sdk.CallToolRequest, in wtDeleteIn) (*sdk.CallToolResult, wtDeleteOut, error) {
	if in.WalkthroughID == "" {
		return errResult[wtDeleteOut](fmt.Errorf("delete_walkthrough needs a walkthrough_id"))
	}
	var res proto.WalkthroughDeleteResult
	if err := s.callInto(ctx, proto.MethodWalkthroughDelete, proto.WalkthroughDelete{
		ID: in.WalkthroughID,
	}, &res); err != nil {
		return errResult[wtDeleteOut](err)
	}
	return &sdk.CallToolResult{}, wtDeleteOut{Deleted: res.Deleted}, nil
}

func (s *server) review(ctx context.Context, _ *sdk.CallToolRequest, in reviewIn) (*sdk.CallToolResult, reviewOut, error) {
	diff := in.Diff
	if diff == "" && in.Path != "" {
		data, err := os.ReadFile(in.Path)
		if err != nil {
			return errResult[reviewOut](err)
		}
		diff = string(data)
	}
	if diff == "" {
		return errResult[reviewOut](fmt.Errorf("request_review needs a diff or path"))
	}
	res, err := s.call(ctx, proto.MethodAsk, &proto.Item{Kind: proto.KindDiff, Title: in.Title, Body: in.Body, Diff: diff, Speak: in.Speak})
	if err != nil {
		return errResult[reviewOut](err)
	}
	return &sdk.CallToolResult{}, reviewOut{Answered: res.Answered, Approved: res.Approved, Comment: res.Reply}, nil
}
