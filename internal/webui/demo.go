package webui

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
