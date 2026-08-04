package webui

import "time"

// Canned data for `agentbox webui-demo`. It lives in the package because the
// wire types are unexported: the surfaces are the only consumer of that shape
// and exporting it just to build a fixture would widen the API for no reason.

// DemoSessions installs a conversation that exercises every part of the
// session surface - prose, thinking, tool chips, code, a table, and a second
// agent in the switcher - so the layout can be judged without a live agent.
func (u *UI) DemoSessions() {
	prose := func(md string) wireSeg { return wireSeg{Kind: "text", HTML: RenderMarkdown(md)} }
	// A user turn shows what was typed, so it carries the source too (sessions.go).
	said := func(text string) wireSeg {
		return wireSeg{Kind: "text", HTML: RenderMarkdown(text), Text: text}
	}
	think := func(md string) wireSeg { return wireSeg{Kind: "thinking", HTML: RenderMarkdown(md)} }
	tool := func(name, in string, done bool) wireSeg {
		return wireSeg{Kind: "tool", ToolName: name, ToolInput: in, HasResult: done}
	}

	conv := []wireTurn{
		{Role: "system", Segments: []wireSeg{prose("session started")}},
		{Role: "user", Segments: []wireSeg{said("Resume support is half-done. What's missing before I add range requests?")}},
		{
			Role: "assistant", Model: "claude-opus-5", CostUSD: 0.0412,
			Segments: []wireSeg{
				think("The state file only persists `seg[0]`, so multi-segment resume can't reconstruct offsets. Check whether the probe exists first."),
				tool("read", "internal/dl/segment.go", true),
				tool("read", "internal/dl/resume.go", true),
				tool("grep", `"Content-Range"`, true),
				prose("Resume works for **single-segment** downloads only - `resume.go` replays one offset as a `Range` header. Two things are missing:\n\n" +
					"| Missing piece | Where | Est. |\n|---|---|---|\n" +
					"| Per-segment offsets in the state file | `resume.go` | 40 min |\n" +
					"| Server range-support probe | `probe.go` | 30 min |\n\n" +
					"The gap is here:\n\n" +
					"```go\n// every segment tracks progress, but only seg[0] is persisted\nfunc (d *Download) resumeFrom(st *State) error {\n    for i, seg := range d.segments {\n        if i > 0 {\n            return errMultiSegmentResume // <- the gap\n        }\n        seg.offset = st.Offset\n    }\n    return nil\n}\n```\n\n" +
					"The probe decides the other two: if the server answers `Accept-Ranges: none` we should fall back to a single segment and skip the state file entirely."),
			},
		},
	}

	second := []wireTurn{
		{Role: "user", Segments: []wireSeg{said("Why does the hit-test miss the top edge?")}},
		{Role: "assistant", Segments: []wireSeg{
			tool("read", "internal/webui/chart.go", true),
			prose("The handle rect is built from the *content* origin, not the window origin, so it is short by the title bar height on the top edge only."),
		}},
	}

	u.sess.SetDemo([]wireSession{
		{
			ID: "s1", Title: "grabbit · resume", Project: "grabbit",
			Cwd: "~/me/projects/grabbit", Mode: "plan", State: "idle",
			Hue: IdentityHue("claude-code", "grabbit", true), Turns: len(conv),
			Selected: true, Model: "claude-opus-5", Conv: conv,
		},
		{
			ID: "s2", Title: "snapper · hit-test", Project: "snapper",
			Cwd: "~/me/projects/snapper", Mode: "full", State: "working",
			Hue: IdentityHue("claude-code", "snapper", true), Turns: len(second),
		},
	})
}

// DemoAgents installs a canned roster for `agentbox webui-demo agents` (FR83,
// gate 2). It is the mock the working rule asks for: the surface exists to be
// judged before the daemon can fill it.
//
// The roster is built to put the whole state vocabulary on screen at once,
// because the chips are what the human learns to read at a glance and a mock
// that shows four of the nine leaves five unjudged. It also carries the three
// cases the design argues about rather than only the happy ones: a wait that
// names its holder, a session that never announced, and an orphaned hold whose
// recorded process is still alive.
func (u *UI) DemoAgents() {
	dark := u.themeMode() == "dark"
	hue := func(agent, project string) string { return IdentityHue(agent, project, dark) }
	age := func(d time.Duration) int64 { return d.Milliseconds() }

	const (
		boxArea = "repo:agentbox"
		grbArea = "repo:grabbit"
		dspArea = "repo:dispatch"
		snpArea = "repo:snapper"
	)
	boxCwd := "~/me/projects/agentbox"

	// The two agents sharing this repo are the case the feature was requested
	// for: on 2026-08-04 three of them shared this checkout and coordinated
	// through a file in /tmp.
	deployer := wireAgent{
		Key: "9f2a4c81", Agent: "claude", Project: "agentbox", Hue: hue("claude", "agentbox"),
		Cwd: boxCwd, PID: 41207, Area: boxArea, AreaLabel: "agentbox",
		Tags:     []string{"subsystem:daemon"},
		Purpose:  "shipping FR83 slice 1: the roster and discovery",
		Activity: "make deploy",
		State:    agentWorking, ActivitySinceMS: age(3 * time.Second), AgeMS: age(21 * time.Minute),
		Holds: []wireHold{
			{Name: "deploy:agentbox", SinceMS: age(2*time.Minute + 12*time.Second),
				Note: "deploying 5943fff", Waiters: 2, PID: 41207, PIDLive: true},
		},
		Timeline: []wireTick{
			{Line: "make deploy", SinceMS: age(3 * time.Second)},
			{Line: "make check: 412 tests green", SinceMS: age(1*time.Minute + 40*time.Second)},
			{Line: "editing internal/daemon/sync.go", SinceMS: age(6 * time.Minute)},
			{Line: "reading docs/09-sync.md", SinceMS: age(19 * time.Minute)},
		},
		Signals: []wireSignal{
			{Topic: "lock:deploy:agentbox", Dir: "posted", SinceMS: age(2*time.Minute + 12*time.Second), Data: `{"reason":"acquired"}`},
			{Topic: "tests:green", Dir: "posted", SinceMS: age(1*time.Minute + 38*time.Second)},
		},
		Items: []wireItemRef{
			{Title: "Deploy from a dirty tree?", Kind: "confirm", State: "answered", SinceMS: age(2*time.Minute + 30*time.Second)},
		},
	}

	checker := wireAgent{
		Key: "3c81dd04", Agent: "claude", Project: "agentbox", Hue: hue("claude", "agentbox"),
		Cwd: boxCwd, PID: 41333, Area: boxArea, AreaLabel: "agentbox",
		Tags:     []string{"subsystem:webui"},
		Purpose:  "FR73: making a closed card's body readable again",
		Activity: "editing frontend/src/surfaces/Inbox.svelte",
		State:    agentWorking, ActivitySinceMS: age(11 * time.Second), AgeMS: age(8 * time.Minute),
		Holds: []wireHold{
			{Name: "repo:agentbox", SinceMS: age(40 * time.Second), Note: "editing the inbox surface", PID: 41333, PIDLive: true},
		},
		Timeline: []wireTick{
			{Line: "editing frontend/src/surfaces/Inbox.svelte", SinceMS: age(11 * time.Second)},
			{Line: "reading internal/webui/inbox.go", SinceMS: age(2 * time.Minute)},
		},
		Signals: []wireSignal{
			{Topic: "agents:repo:agentbox", Dir: "received", SinceMS: age(8 * time.Minute), Data: `{"joined":"9f2a4c81"}`},
		},
	}

	// The blocked row. It names the holder and its place in the queue, because
	// "blocked" with nobody named is the state the human already has today.
	releaser := wireAgent{
		Key: "b0417e9a", Agent: "claude", Project: "agentbox", Hue: hue("claude", "agentbox"),
		Cwd: boxCwd, PID: 41402, Area: boxArea, AreaLabel: "agentbox",
		Purpose: "cutting the 0.9 tag once the tree is deployed",
		State:   agentBlocked, Detail: "deploy:agentbox", AgeMS: age(4 * time.Minute),
		// No activity line on purpose: the wait line below carries the whole
		// story, and saying "waiting for the deploy lock" twice on one row is how
		// a dense surface starts reading as noise.
		Wait: &wireWait{
			Lock: "deploy:agentbox", HolderKey: "9f2a4c81", Holder: "claude · agentbox",
			SinceMS: age(1*time.Minute + 4*time.Second), Place: 1, Queue: 2,
		},
		Timeline: []wireTick{
			{Line: "waiting for the deploy lock", SinceMS: age(1*time.Minute + 4*time.Second)},
			{Line: "wrote the release notes", SinceMS: age(3 * time.Minute)},
		},
	}

	// The dim row: present, registered by its child, never announced. It is here
	// rather than invisible, which is the point of registering what the child
	// knows without asking the model for anything.
	rude := wireAgent{
		Key: "77c1e5b2", Agent: "claude", Project: "agentbox", Hue: hue("claude", "agentbox"),
		Cwd: boxCwd, PID: 41880, Area: boxArea, AreaLabel: "agentbox",
		State: agentUnannounced, AgeMS: age(6 * time.Minute),
	}

	asking := wireAgent{
		Key: "5ab90c37", Agent: "claude", Project: "grabbit", Hue: hue("claude", "grabbit"),
		Cwd: "~/me/projects/grabbit", PID: 39115, Area: grbArea, AreaLabel: "grabbit",
		Purpose: "adding range requests to the downloader",
		State:   agentAsking, AgeMS: age(34 * time.Minute),
		Activity: "asked about the state file's shape", ActivitySinceMS: age(38 * time.Second),
		Pending: "Persist per-segment offsets in the state file?",
		Timeline: []wireTick{
			{Line: "asked about the state file's shape", SinceMS: age(38 * time.Second)},
			{Line: "editing internal/dl/resume.go", SinceMS: age(4 * time.Minute)},
		},
		Items: []wireItemRef{
			{Title: "Persist per-segment offsets in the state file?", Kind: "choice", State: "pending", SinceMS: age(38 * time.Second)},
		},
	}

	// Parked on a signal, which is a steady state and never warns: listening is
	// what success looks like for an agent whose turn has not come.
	listener := wireAgent{
		Key: "c4d20f6e", Agent: "claude", Project: "grabbit", Hue: hue("claude", "grabbit"),
		Cwd: "~/me/projects/grabbit", PID: 39240, Area: grbArea, AreaLabel: "grabbit",
		Tags:    []string{"role:release"},
		Purpose: "release captain: publish the moment the tests go green",
		State:   agentListening, Detail: "tests:green", AgeMS: age(12 * time.Minute),
		Activity: "parked on tests:green", ActivitySinceMS: age(4*time.Minute + 20*time.Second),
		Signals: []wireSignal{
			{Topic: "tests:green", Dir: "received", SinceMS: age(9 * time.Minute), Data: `{"suite":"unit","ok":true}`},
		},
	}

	quiet := wireAgent{
		Key: "e18b7a55", Agent: "codex", Project: "grabbit", Hue: hue("codex", "grabbit"),
		Cwd: "~/me/projects/grabbit", PID: 38004, Area: grbArea, AreaLabel: "grabbit",
		Purpose:  "reviewing the resume path for off-by-one offsets",
		Activity: "reading internal/dl/resume.go",
		State:    agentQuiet, ActivitySinceMS: age(14*time.Minute + 30*time.Second), AgeMS: age(52 * time.Minute),
	}

	driving := wireAgent{
		Key: "a7e34b18", Agent: "claude", Project: "dispatch", Hue: hue("claude", "dispatch"),
		Cwd: "~/me/projects/dispatch", PID: 37220, Area: dspArea, AreaLabel: "dispatch",
		Purpose:  "checking the fullscreen marker on the real desktop",
		Activity: "moving the pointer to the tray icon",
		State:    agentDriving, ActivitySinceMS: age(2 * time.Second), AgeMS: age(3 * time.Minute),
		Holds: []wireHold{
			{Name: "desktop", SinceMS: age(1*time.Minute + 50*time.Second), Note: "the control run", PID: 37220, PIDLive: true},
		},
		Timeline: []wireTick{
			{Line: "moving the pointer to the tray icon", SinceMS: age(2 * time.Second)},
			{Line: "opened a fullscreen window", SinceMS: age(1 * time.Minute)},
		},
	}

	reporting := wireAgent{
		Key: "d90c6f42", Agent: "claude", Project: "dispatch", Hue: hue("claude", "dispatch"),
		Cwd: "~/me/projects/dispatch", PID: 37455, Area: dspArea, AreaLabel: "dispatch",
		Purpose:  "migrating the events table",
		Activity: "812 of 1204 rows",
		State:    agentReporting, Detail: "64%", ActivitySinceMS: age(1 * time.Second), AgeMS: age(6 * time.Minute),
	}

	// The pre-sync session: no attach, so no row of its own kind - this one is
	// derived from item traffic alone, and its existence is why the reads carry
	// partial. Absence is never asserted on partial data.
	detached := wireAgent{
		Key: "", Agent: "claude", Project: "snapper", Hue: hue("claude", "snapper"),
		Cwd: "~/me/projects/snapper", Area: snpArea, AreaLabel: "snapper",
		State: agentDetached, AgeMS: age(3 * time.Minute),
		Items: []wireItemRef{
			{Title: "Rebased onto main", Kind: "notify", State: "expired", SinceMS: age(3 * time.Minute)},
		},
	}

	u.ShowAgents(wireRoster{
		Agents: []wireAgent{
			deployer, checker, releaser, rude,
			asking, listener, quiet,
			driving, reporting,
			detached,
		},
		// The orphan: the session that held the VM died, and the lock did NOT go
		// with it. Its recorded process is still alive, so nobody gets the VM
		// until that pid is gone or the human breaks it - the failure a
		// five-second grace would have caused, made visible instead.
		Orphans: []wireHold{{
			Name: "vm:boris-vm", SinceMS: age(8*time.Minute + 30*time.Second),
			Note: "running the 128-vCPU build", Waiters: 1,
			Orphaned: true, PID: 40122, PIDLive: true, Holder: "claude · agentbox",
		}},
		Partial: true,
	})
}
