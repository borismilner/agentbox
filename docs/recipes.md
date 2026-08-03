# Recipes: wiring AgentBox into Claude Code

`agentbox docs setup` prints these ready to paste, pointed at your installed
binary. The snippets below use the bare `agentbox` name; use an absolute path
if it is not on PATH.

## MCP server (richest integration)

Register the server so the model can call AgentBox's tools directly. Project
scope, `.mcp.json` at the repo root:

```json
{"mcpServers": {"agentbox": {"command": "agentbox", "args": ["mcp"]}}}
```

The model then has `notify_user`, `ask_user`, `confirm_action`,
`act_unless_stopped`, `ask_user_form` and `request_secret`. The tool
descriptions teach it when to reach for each; `request_secret` returns a
file path, never the value.

## Hooks (no MCP needed)

Add to `~/.claude/settings.json`. A Stop hook chimes when the agent
finishes; a Notification hook surfaces Claude's own prompts.

```json
{
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command",
        "command": "agentbox notify --level success --title \"Agent finished\""}]}
    ],
    "Notification": [
      {"hooks": [{"type": "command",
        "command": "agentbox notify --level info --title \"Claude needs you\""}]}
    ]
  }
}
```

## Teach the agent to drive AgentBox itself

Drop the quickstart into your project instructions so the agent reaches for
AgentBox without being told the flags:

```sh
agentbox docs agent >> CLAUDE.md
```

That page lists every command, its result shape and exit code, and when to
interrupt. `agentbox schema` gives the machine-readable wire contract.

## Shell one-liners

```sh
# ask before a risky step
agentbox confirm --title "Run destructive migration?" || exit 1

# proceed unless stopped within 15 s
agentbox veto --in 15 --title "Deploying to prod"

# collect a token without it landing in the transcript
agentbox secret --title "npm token" --to-file ./.npmrc-token

# notify with action buttons (FR32): up to 3, "Label::command", run on click
agentbox notify --level success --title "PR opened" \
  --action "Open PR::gh pr view 482 --web" \
  --action "View CI::gh run watch"

# render a markdown file in the reading window, live-reload while editing
agentbox show CHANGELOG.md --watch
git show HEAD --stat | agentbox show -    # pipe anything markdown-ish

# review a diff before applying it: exit 0 approved, 1 changes requested
git diff | agentbox review --title "Apply working-tree changes?" && git commit -am wip
```

Action buttons run their command locally in the directory the `notify` was
called from. The user sees the exact command on hover; turn the feature off
globally with `[actions] enabled = false`. Over MCP, `notify_user` takes the
same buttons through its `actions` argument.
