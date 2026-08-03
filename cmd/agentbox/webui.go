package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/config"
	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/walkthrough"
	"github.com/borismilner/agentbox/internal/webui"
)

// `agentbox webui-demo` drives the web surfaces from a fake daemon so the UI can
// be built and looked at without a socket, a store or a real agent. It is the
// web equivalent of `agentbox demo`, and it is what the port is developed
// against until every surface reaches parity.

type demoResolver struct {
	ui   *webui.UI
	next func()
}

func (d *demoResolver) log(what string, args ...any) {
	fmt.Printf("  ← %s %v\n", what, args)
	if d.next != nil {
		go func() { time.Sleep(900 * time.Millisecond); d.next() }()
	}
}

func (d *demoResolver) Answer(id, label string)                   { d.log("answer", id, label) }
func (d *demoResolver) Reply(id, text string)                     { d.log("reply", id, text) }
func (d *demoResolver) AnswerForm(id string, v map[string]string) { d.log("form", id, v) }
func (d *demoResolver) Dismiss(id string)                         { d.log("dismiss", id) }
func (d *demoResolver) Defer(id string)                           { d.log("defer", id) }
func (d *demoResolver) Undo(id string)                            { d.log("undo", id) }
func (d *demoResolver) Veto(id string)                            { d.log("veto", id) }
func (d *demoResolver) Secret(id, value string)                   { d.log("secret", id, "***") }
func (d *demoResolver) RunAction(id string, index int)            { d.log("action", id, index) }
func (d *demoResolver) Review(id string, ok bool, comment string) { d.log("review", id, ok, comment) }

// The artifact channel with no daemon behind it: the demo prints what an agent
// would have been handed, which is enough to judge that the channel works (M10).
func (d *demoResolver) ArtifactEvent(ev proto.ArtifactEvent) {
	fmt.Printf("  \u2190 artifact %s emit %s %s\n", ev.ArtifactID, ev.Name, string(ev.Data))
}

// demoSource stands in for the daemon behind the inbox and history surfaces, so
// they can be built and judged without a store. It satisfies webui.Source, which
// means the surfaces run the same code path they will in the daemon - there is
// no demo branch inside internal/webui.
type demoSource struct{}

func (demoSource) Promote(id string)     { fmt.Printf("  ← promote %s\n", id) }
func (demoSource) MutedAgents() []string { return []string{"codex"} }

func (demoSource) RecentItems(limit int) ([]store.StoredItem, error) {
	now := time.Now()
	at := func(d time.Duration) time.Time { return now.Add(-d) }
	claude := proto.Identity{Agent: "claude-code", Project: "grabbit"}
	codex := proto.Identity{Agent: "codex", Project: "snapper"}
	third := proto.Identity{Agent: "claude-code", Project: "dispatch"}

	items := []store.StoredItem{
		{
			Item: proto.Item{
				ID: "d1", Kind: proto.KindChoice, Level: proto.LevelWarning, Identity: claude,
				Title:   "Run DB migration on staging?",
				Body:    "Adds a **non-null** column to `events`; the backfill holds a write lock for ~30s.",
				Options: []proto.Option{{Label: "Run now"}, {Label: "Dry run"}, {Label: "Skip"}},
				Default: "Dry run",
			},
			State: store.StatePending, CreatedAt: at(40 * time.Second),
		},
		{
			Item: proto.Item{
				ID: "d2", Kind: proto.KindConfirm, Level: proto.LevelError, Identity: codex,
				Title: "Force-push to main?", Body: "`origin/main` is 3 commits ahead.",
			},
			State: store.StatePending, CreatedAt: at(4 * time.Minute),
		},
		{
			Item: proto.Item{
				ID: "d3", Kind: proto.KindVeto, Level: proto.LevelUrgent, Identity: codex,
				Title: "Deleting 1,204 orphaned blobs", TimeoutS: 15,
			},
			State: store.StatePending, CreatedAt: at(11 * time.Minute),
		},
		{
			Item: proto.Item{
				ID: "d4", Kind: proto.KindText, Level: proto.LevelInfo, Identity: third,
				Title: "What should the release be called?",
			},
			State: store.StatePending, CreatedAt: at(26 * time.Minute),
		},
		{
			Item: proto.Item{
				ID: "d5", Kind: proto.KindChoice, Level: proto.LevelInfo, Identity: claude,
				Title: "Which fixture should the parser test use?",
			},
			State: store.StateAnswered, Answer: "the truncated one", CreatedAt: at(52 * time.Minute),
			ResolvedAt: at(51 * time.Minute),
		},
		{
			Item: proto.Item{
				ID: "d6", Kind: proto.KindNotify, Level: proto.LevelSuccess, Identity: claude,
				Title: "Build passed", Body: "All 412 tests green in 38s.",
			},
			State: store.StateExpired, MissedWhileAway: true, CreatedAt: at(80 * time.Minute),
		},
		{
			Item: proto.Item{
				ID: "d7", Kind: proto.KindVeto, Level: proto.LevelWarning, Identity: codex,
				Title: "Pruning the download cache", TimeoutS: 15,
			},
			State: store.StateExpired, CreatedAt: at(3 * time.Hour),
		},
		{
			Item: proto.Item{
				ID: "d8", Kind: proto.KindText, Level: proto.LevelInfo, Identity: third,
				Title: "Describe the regression in one line",
			},
			State: store.StateAnswered, Reply: "veto card lingered until a hover",
			CreatedAt: at(5 * time.Hour), ResolvedAt: at(5 * time.Hour),
		},
		{
			Item: proto.Item{
				ID: "d9", Kind: proto.KindConfirm, Level: proto.LevelError, Identity: codex,
				Title: "Drop the staging database?",
			},
			State: store.StateCancelled, CreatedAt: at(27 * time.Hour),
		},
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// demoBoard stands in for the daemon behind the review board: a canned
// walkthrough over this repo's own files, marks and comments held in memory,
// and a submit that prints what an agent would have been handed. It
// satisfies webui.BoardStore, so the board runs the exact code path it runs
// in the daemon - including the hollow-unclear gate.
type demoBoard struct {
	mu       sync.Mutex
	w        store.Walkthrough
	marks    map[string]*store.Mark
	comments []store.Comment
	n        int
}

func newDemoBoard() *demoBoard {
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}
	return &demoBoard{
		w: store.Walkthrough{
			ID: "wdem000000000", Title: "the exit-code contract", RepoRoot: root,
			PinnedSHA: "0000000demo", State: store.WtOpen, SpecRev: 1,
			CountedSteps: 2, Spec: demoBoardSpec(),
		},
		marks: map[string]*store.Mark{},
	}
}

// demoBoardSpec cites this repo (run `agentbox webui-demo board` from the repo
// root); the snippet step carries its own content so at least one code step
// renders whole from any directory.
func demoBoardSpec() string {
	return `{
	  "version": 1, "title": "the exit-code contract", "repo_root": ".", "pinned": "0000000demo",
	  "steps": [
	    {"id": "ground", "kind": "ground", "title": "What this demo shows",
	     "prose": [{"t": "Two code steps and a check. Mark them, write notes, select code to comment, then press s - the submit modal is the point of this demo."}]},
	    {"id": "codes", "kind": "code", "title": "Exit codes are a contract",
	     "purpose": "Serves: FR41 - scripts branch on these numbers. Decided by: the CLI's first release.",
	     "prose": [{"t": "Five numbers, "}, {"t": "stable forever", "bind": "codes"}, {"t": ", because agents write scripts against them the day they learn them."}],
	     "code": [{"path": "cmd/agentbox/main.go", "lines": [30, 37],
	       "notes": [{"at": [32, 36], "text": "0 through 4: answered, refused, misused, unanswered, broken."}]}],
	     "binds": {"codes": {"lines": [31, 37]}},
	     "checks": [{"q": "A blocking ask times out. Which code?", "a": "3 - unanswered/timeout. 1 is a human saying no; 4 is agentbox itself failing."}]},
	    {"id": "snippet", "kind": "code", "title": "A snippet block",
	     "purpose": "Serves: content that lives in no file still renders with all three channels.",
	     "prose": [{"t": "Snippets carry their own add and del vocabulary - there is no manifest to derive from."}],
	     "code": [{"snippet": {"lang": "go", "text": "func demo() int {\n\treturn exitOK\n}", "added": [2],
	       "del": [{"after": 1, "old": 41, "lines": ["\treturn 0 // what it said before the constant existed"]}]},
	       "label": "the snippet", "notes": [{"at": [2, 2], "text": "The added flag and this note travel as separate channels."}]}]},
	    {"id": "gate", "kind": "check", "title": "The gate",
	     "purpose": "Serves: finishing is an observation, not a feeling.",
	     "prose": [{"t": "Mark a step unclear with no note, then submit: the modal jumps back instead of shipping it hollow."}],
	     "cmds": [{"cmd": "agentbox walkthrough list", "expect": "the library lists this review", "recorded": "2026-07-29"}]}
	  ]
	}`
}

func (b *demoBoard) BoardData(string) (store.Walkthrough, []store.Mark, []store.Comment, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	marks := make([]store.Mark, 0, len(b.marks))
	for _, m := range b.marks {
		marks = append(marks, *m)
	}
	return b.w, marks, append([]store.Comment(nil), b.comments...), nil
}

func (b *demoBoard) mark(stepID string) *store.Mark {
	m := b.marks[stepID]
	if m == nil {
		m = &store.Mark{StepID: stepID}
		b.marks[stepID] = m
	}
	return m
}

func (b *demoBoard) BoardVerdict(_, stepID, verdict string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mark(stepID).Verdict = verdict
	fmt.Printf("  ← verdict %s %s\n", stepID, verdict)
	return nil
}

func (b *demoBoard) BoardNote(_, stepID, note string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mark(stepID).Note = note
	return nil
}

func (b *demoBoard) BoardReveal(_, stepID string, revealed []int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mark(stepID).Revealed = revealed
	return nil
}

func (b *demoBoard) BoardPos(_ string, pos int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.w.Pos = pos
	return nil
}

func (b *demoBoard) BoardCommentAdd(_, stepID, path, side string, from, to int, exact, body string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n++
	id := fmt.Sprintf("cdem%09d", b.n)
	b.comments = append(b.comments, store.Comment{
		ID: id, StepID: stepID, Path: path, Side: side,
		FromLine: from, ToLine: to, Exact: exact, Body: body,
	})
	fmt.Printf("  ← comment %s %s:%d-%d %q\n", stepID, path, from, to, body)
	return id, nil
}

func (b *demoBoard) BoardCommentEdit(_, commentID, body string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.comments {
		if b.comments[i].ID == commentID {
			b.comments[i].Body = body
		}
	}
	return nil
}

func (b *demoBoard) BoardCommentDelete(_, commentID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	kept := b.comments[:0]
	for _, c := range b.comments {
		if c.ID != commentID {
			kept = append(kept, c)
		}
	}
	b.comments = kept
	return nil
}

// BoardSubmit mirrors the daemon's gate, then prints the tally an agent
// would receive - the demo's stand-in for delivery.
// The demo has exactly one review and no store to delete it from, so the
// library shows that one row and refuses to remove it.
func (b *demoBoard) BoardLibrary() ([]proto.WalkthroughSummary, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return []proto.WalkthroughSummary{{
		ID: b.w.ID, Title: b.w.Title, RepoRoot: b.w.RepoRoot, Pinned: b.w.PinnedSHA,
		State: b.w.State, CountedSteps: b.w.CountedSteps, UpdatedAtMS: time.Now().UnixMilli(),
	}}, nil
}

func (b *demoBoard) BoardDelete(string) (bool, error) {
	fmt.Println("  ← delete: the demo has nothing to delete from")
	return false, nil
}

func (b *demoBoard) BoardSubmit(string) (bool, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	understood, unclear := 0, 0
	for _, m := range b.marks {
		switch m.Verdict {
		case "unclear":
			if strings.TrimSpace(m.Note) == "" {
				return false, 0, &walkthrough.GateError{StepID: m.StepID, Title: m.StepID}
			}
			unclear++
		case "understood":
			understood++
		}
	}
	fmt.Printf("  ← submit: %d understood · %d unclear · %d comments\n", understood, unclear, len(b.comments))
	return true, time.Now().UnixMilli(), nil
}

func (demoSource) Stats(since time.Time) (proto.Stats, error) {
	day := func(d int) string { return time.Now().AddDate(0, 0, -d).Format("2006-01-02") }
	return proto.Stats{
		SinceMS: since.UnixMilli(), Total: 63, Questions: 41, Answered: 34, MedianAnswerMS: 14_200,
		ByAgent: []proto.AgentStat{
			{Agent: "claude-code", Total: 38, Questions: 25, Answered: 22, MedianAnswerMS: 11_800},
			{Agent: "codex", Total: 19, Questions: 13, Answered: 10, MedianAnswerMS: 22_400},
			{Agent: "nudge", Total: 6, Questions: 3, Answered: 2, MedianAnswerMS: 96_000},
		},
		ByDay: []proto.DayCount{
			{Day: day(6), Count: 4}, {Day: day(5), Count: 11}, {Day: day(4), Count: 7},
			{Day: day(3), Count: 16}, {Day: day(2), Count: 9}, {Day: day(1), Count: 12},
			{Day: day(0), Count: 4},
		},
	}, nil
}

func runWebUIDemo(args []string) {
	cfg, _, _ := config.Load(config.Path())
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	res := &demoResolver{}
	u := webui.New(res, log, cfg)
	res.ui = u

	// `agentbox webui-demo panel` rolls the drop-down panel down with a canned
	// conversation in it, so the animation and the layout can be judged without a
	// `claude` child (M10). The hotkey belongs to the daemon; here the toggle is
	// this process opening it once.
	if len(args) > 0 && args[0] == "panel" {
		u.DemoSessions()
		u.SetSource(demoSource{})
		go func() {
			time.Sleep(400 * time.Millisecond)
			u.ShowPanel()
		}()
		if err := u.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "webui:", err)
			os.Exit(exitError)
		}
		return
	}

	// `agentbox webui-demo app` opens the application window with canned sessions,
	// inbox rows and history instead of cycling cards.
	if len(args) > 0 && args[0] == "app" {
		u.DemoSessions()
		u.SetSource(demoSource{})
		// `agentbox webui-demo app inbox` lands on a surface directly, so a single
		// one can be looked at without clicking through the rail.
		tab := "session"
		if len(args) > 1 {
			tab = args[1]
		}
		go func() {
			time.Sleep(300 * time.Millisecond)
			u.ShowApp(tab)
		}()
		if err := u.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "webui:", err)
			os.Exit(exitError)
		}
		return
	}

	// `agentbox webui-demo ask` opens the session surface and presents session-tagged
	// items through the real Present path, so the routing rule decides where they
	// land exactly as it will in the daemon - the panel is not a canned picture.
	if len(args) > 0 && args[0] == "ask" {
		u.DemoSessions()
		u.SetSource(demoSource{})
		items := demoAsks()
		at := 0
		show := func() {
			if at >= len(items) {
				fmt.Println("demo done")
				return
			}
			it := items[at]
			at++
			fmt.Printf("→ %s: %s\n", it.Kind, it.Title)
			u.Present(daemon.View{
				Item:           &it,
				Waiting:        len(items) - at,
				ExpiresAt:      time.Now().Add(time.Duration(max(it.TimeoutS, 300)) * time.Second),
				ActionsEnabled: true,
				Caller:         daemon.CallerLive,
			})
		}
		res.next = func() {
			u.Present(daemon.View{}) // resolve: the panel clears, then the next one arrives
			time.Sleep(400 * time.Millisecond)
			show()
		}
		go func() {
			time.Sleep(300 * time.Millisecond)
			u.ShowApp("session")
			time.Sleep(900 * time.Millisecond)
			show()
		}()
		if err := u.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "webui:", err)
			os.Exit(exitError)
		}
		return
	}

	// `agentbox webui-demo board` opens the review board on a canned walkthrough
	// over this repo's own files (run it from the repo root). Marks live in
	// memory; submit prints the tally an agent would receive.
	if len(args) > 0 && args[0] == "board" {
		b := newDemoBoard()
		u.SetBoardStore(b)
		go func() {
			time.Sleep(300 * time.Millisecond)
			u.ShowBoard(b.w.ID)
		}()
		if err := u.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "webui:", err)
			os.Exit(exitError)
		}
		return
	}

	// The surfaces that are not driven by the queue get their own demo paths:
	// `webui-demo viewer [FILE]` opens the reader on a watched file, and
	// `webui-demo progress` drives a set of fake reports.
	if len(args) > 0 && args[0] == "viewer" {
		path := "docs/sample.md"
		if len(args) > 1 {
			path = args[1]
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		go func() {
			time.Sleep(300 * time.Millisecond)
			u.ShowDocument(proto.ShowRequest{Path: abs, Watch: true})
		}()
		if err := u.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "webui:", err)
			os.Exit(exitError)
		}
		return
	}
	// `agentbox webui-demo artifact` runs the canned artifact below through the real
	// show path, so the sandbox, the injected React and Tailwind, the code/preview
	// toggle and the interaction channel can all be judged without an agent (M10).
	if len(args) > 0 && args[0] == "artifact" {
		src, title := demoArtifact(), "Deploy plan"
		if len(args) > 1 {
			data, err := os.ReadFile(args[1])
			if err != nil {
				fmt.Fprintln(os.Stderr, "webui-demo artifact:", err)
				os.Exit(exitError)
			}
			src, title = string(data), filepath.Base(args[1])
		}
		go func() {
			time.Sleep(300 * time.Millisecond)
			u.ShowDocument(proto.ShowRequest{Content: src, Title: title, Artifact: true})
		}()
		if err := u.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "webui:", err)
			os.Exit(exitError)
		}
		return
	}
	if len(args) > 0 && args[0] == "progress" {
		go demoProgress(u)
		if err := u.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "webui:", err)
			os.Exit(exitError)
		}
		return
	}

	items := demoItems()
	if len(args) > 0 {
		items = filterDemo(items, args[0])
	}
	at := 0
	show := func() {
		if at >= len(items) {
			fmt.Println("demo done")
			u.Quit()
			return
		}
		it := items[at]
		at++
		fmt.Printf("→ %s: %s\n", it.Kind, it.Title)
		v := daemon.View{
			Item:           &it,
			Waiting:        len(items) - at,
			WaitingFrom:    waitingIdentities(items[at:]),
			ExpiresAt:      time.Now().Add(time.Duration(max(it.TimeoutS, 300)) * time.Second),
			ActionsEnabled: true,
			Caller:         daemon.CallerLive,
		}
		// Mirror the daemon's toast rule (setCurrentLocked): an info or success
		// notice closes itself, a warning or error waits. Without this the demo
		// would show every toast as sticky and the countdown would never be seen.
		if it.Kind == proto.KindNotify {
			switch it.EffectiveLevel() {
			case proto.LevelInfo, proto.LevelSuccess:
				v.DismissAt = time.Now().Add(6 * time.Second)
			}
		}
		u.Present(v)
	}
	res.next = func() {
		u.Present(daemon.View{}) // clear, then the next item gets a fresh window
		time.Sleep(180 * time.Millisecond)
		show()
	}

	go func() {
		time.Sleep(400 * time.Millisecond)
		show()
	}()

	if err := u.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "webui:", err)
		os.Exit(exitError)
	}
}

// demoProgress drives the FR21 window without a daemon: two determinate tasks at
// different speeds plus an indeterminate one, then an empty set so the window's
// self-closing path gets exercised too.
func demoProgress(u *webui.UI) {
	claude := proto.Identity{Agent: "claude-code", Project: "grabbit"}
	codex := proto.Identity{Agent: "codex", Project: "snapper"}

	time.Sleep(300 * time.Millisecond)
	for step := 0; step <= 100; step += 4 {
		reports := []daemon.ProgressState{
			{ID: "p1", Title: "Migrating events table", Status: fmt.Sprintf("%d of 1204 rows", step*12), Percent: step, Identity: claude},
		}
		if step > 12 {
			reports = append(reports, daemon.ProgressState{
				ID: "p2", Title: "Fetching release assets", Percent: step * 2 / 3,
				Status: "linux-amd64.tar.gz", Identity: codex,
			})
		}
		if step > 30 {
			reports = append(reports, daemon.ProgressState{
				ID: "p3", Title: "Indexing", Indeterminate: true, Status: "reading the store", Identity: claude,
			})
		}
		u.ShowProgress(reports)
		time.Sleep(320 * time.Millisecond)
	}
	u.ShowProgress(nil)
	fmt.Println("demo done")
}

func waitingIdentities(rest []proto.Item) []proto.Identity {
	out := make([]proto.Identity, 0, len(rest))
	for _, it := range rest {
		out = append(out, it.Identity)
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

func filterDemo(items []proto.Item, kind string) []proto.Item {
	out := items[:0]
	for _, it := range items {
		if string(it.Kind) == kind {
			out = append(out, it)
		}
	}
	return out
}

// demoAsks are tagged with the demo conversation's session id, which is what
// makes them land in the panel rather than in a card. The last one is deliberately
// a kind the panel does not take, so the fallback to a card can be seen.
func demoAsks() []proto.Item {
	claude := proto.Identity{Agent: "claude-code", Project: "grabbit", Session: "s1"}

	return []proto.Item{
		{
			ID: "ask-choice", Kind: proto.KindChoice, Level: proto.LevelWarning,
			Title: "Persist per-segment offsets in the state file?",
			Body: "Multi-segment resume needs them, but the file's shape changes and " +
				"older state files stop loading. I can migrate them on read instead.",
			Options: []proto.Option{
				{Label: "Change the shape"},
				{Label: "Migrate on read", Desc: "slower, keeps old files"},
				{Label: "Leave it"},
			},
			Default: "Migrate on read", TimeoutS: 300, Identity: claude,
		},
		{
			ID: "ask-confirm", Kind: proto.KindConfirm, Level: proto.LevelError,
			Title:    "Delete the old state files?",
			Body:     "14 files under `~/.local/state/grabbit`, none newer than the schema change.",
			Identity: claude,
		},
		{
			ID: "ask-notify", Kind: proto.KindNotify, Level: proto.LevelSuccess,
			Title:    "Range-request probe landed",
			Body:     "`probe.go` is in, with tests for `Accept-Ranges: none` and a truncated response.",
			Actions:  []proto.Action{{Label: "Show the diff", Exec: "git diff --stat"}},
			Identity: claude,
		},
		{
			ID: "ask-text", Kind: proto.KindText, Level: proto.LevelInfo,
			Title:    "What should the fallback be called?",
			Body:     "A text answer needs a field, so this one gets its card even though it is tagged.",
			Identity: claude,
		},
	}
}

// demoArtifact is a React artifact in the claude.ai shape: an import from react,
// JSX, Tailwind classes, no stylesheet, no CDN tag, and a component as the
// default export. If this runs, an artifact written for claude.ai runs. It also
// calls agentbox.emit, which is the whole point of the thing - a control the human
// moves and the agent hears about (M10).
func demoArtifact() string {
	return `import React, { useState } from "react";

export default function DeployPlan() {
  const [batch, setBatch] = useState(500);
  const [dryRun, setDryRun] = useState(true);
  const [note, setNote] = useState("");

  const send = (name, data) => {
    window.agentbox?.emit(name, data);
    setNote(name + " sent at " + new Date().toLocaleTimeString());
  };

  return (
    <div className="p-5 font-sans">
      <h1 className="text-lg font-semibold mb-1">Migrate events table</h1>
      <p className="text-sm opacity-70 mb-5">
        1,204 rows. Pick a batch size and I will run it in that many at a time.
      </p>

      <label className="block text-sm mb-2">
        Batch size: <span className="font-mono">{batch}</span>
      </label>
      <input
        type="range" min="50" max="2000" step="50" value={batch}
        onChange={(e) => setBatch(Number(e.target.value))}
        onMouseUp={() => send("batch", { rows: batch })}
        className="w-full mb-5"
      />

      <label className="flex items-center gap-2 text-sm mb-5">
        <input type="checkbox" checked={dryRun} onChange={(e) => setDryRun(e.target.checked)} />
        Dry run first
      </label>

      <div className="flex gap-2">
        <button
          onClick={() => send("run", { rows: batch, dryRun })}
          className="px-3 py-1.5 rounded-md text-sm font-medium"
          style={{ background: "var(--k-accent)", color: "var(--k-ground)" }}>
          {dryRun ? "Dry run" : "Run it"}
        </button>
        <button
          onClick={() => send("cancel", {})}
          className="px-3 py-1.5 rounded-md text-sm border"
          style={{ borderColor: "var(--k-edge)" }}>
          Cancel
        </button>
      </div>

      {note && <p className="mt-4 text-xs font-mono opacity-60">{note}</p>}
    </div>
  );
}
`
}

func demoItems() []proto.Item {
	claude := proto.Identity{Agent: "claude-code", Project: "grabbit"}
	other := proto.Identity{Agent: "codex", Project: "snapper"}
	third := proto.Identity{Agent: "claude-code", Project: "dispatch"}

	return []proto.Item{
		{
			ID: "demo-choice", Kind: proto.KindChoice, Level: proto.LevelWarning,
			Title: "Run DB migration on staging?",
			Body: "The diff adds a **non-null** column to `events`. Backfill takes about " +
				"4 minutes and holds a write lock for the last 30 seconds of it.",
			Options: []proto.Option{
				{Label: "Run now"},
				{Label: "Dry run", Desc: "no writes, prints the plan"},
				{Label: "Skip"},
			},
			Default: "Dry run", TimeoutS: 300, Identity: claude,
		},
		{
			ID: "demo-confirm", Kind: proto.KindConfirm, Level: proto.LevelError,
			Title:    "Force-push to main?",
			Body:     "`origin/main` is 3 commits ahead. This rewrites them.",
			Identity: other,
		},
		{
			ID: "demo-text", Kind: proto.KindText, Level: proto.LevelInfo,
			Title:    "What should the release be called?",
			Body:     "Used as the tag and the GitHub release title.",
			Identity: third, Multiline: false,
		},
		{
			ID: "demo-notify", Kind: proto.KindNotify, Level: proto.LevelSuccess,
			Title: "Build passed",
			Body:  "All 412 tests green in 38s.\n\n```go\nok  \tgithub.com/boris-milner/grabbit/internal/dl\t0.412s\n```",
			Actions: []proto.Action{
				{Label: "Open report", Exec: "xdg-open ./coverage.html"},
				{Label: "Tag release", Exec: "git tag -a"},
			},
			Identity: claude,
		},
		{
			ID: "demo-notify-warn", Kind: proto.KindNotify, Level: proto.LevelWarning,
			Title: "Coverage dropped below the gate",
			Body: "`internal/store` fell to **71.4%** (gate is 75%). The uncovered lines are all in " +
				"the migration path, which the suite skips on a fresh database - so this is either a " +
				"real gap or a fixture that needs to start from an older schema.",
			Identity: other,
		},
		{
			ID: "demo-notify-info", Kind: proto.KindNotify, Level: proto.LevelInfo,
			Title: "Rebased onto main", Identity: third,
		},
		{
			ID: "demo-veto", Kind: proto.KindVeto, Level: proto.LevelUrgent,
			Title: "Deleting 1,204 orphaned blobs", TimeoutS: 15, Identity: other,
		},
		{
			ID: "demo-form", Kind: proto.KindForm, Level: proto.LevelInfo,
			Title: "Deploy settings",
			Fields: []proto.Field{
				{Key: "env", Label: "Environment", Type: "choice", Options: []string{"staging", "prod"}, Default: "staging"},
				{Key: "tag", Label: "Tag", Type: "text", Default: "v0.4.1"},
				{Key: "notify", Label: "Notify channel", Type: "bool"},
			},
			Identity: claude,
		},
	}
}
