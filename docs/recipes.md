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

## Keeping the Agents board honest with no tokens (FR83)

The roster is only as truthful as the agents on it, and a model that forgets to
`set_activity` leaves a stale row that looks like a hung session. Hooks close
that gap: they are shell, so they cost no tokens and they run whether the model
remembers anything or not.

`AGENTBOX_SESSION_KEY` is what ties a hook to the session it belongs to. Set it
once, in the same environment the agent runs in, and every `agentbox sync` call
below acts on behalf of that session:

```sh
export AGENTBOX_SESSION_KEY="$(head -c8 /dev/urandom | od -An -tx1 | tr -d ' \n')"
export AGENTBOX_AGENT=claude   # optional, and worth it: see below
```

`AGENTBOX_AGENT` is who the row says is calling. Without it the name is worked out
from the process tree, which is usually right and is occasionally embarrassing: a
hook runs as claude -> sh -> agentbox, and anything wrapped in `setsid` has been
reparented to init, so the tree says nothing. The walk skips shells and wrappers
and falls back to `agent` rather than naming one of them, and a row wearing that
placeholder is renamed the moment the session's own child announces. Setting the
variable skips all of that guessing.

Then in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command",
        "command": "agentbox sync announce \"$(basename \"$PWD\") session (purpose not yet stated)\""}]}
    ],
    "PostToolUse": [
      {"matcher": "Edit|Write|NotebookEdit",
       "hooks": [{"type": "command",
        "command": "agentbox sync activity \"editing $(jq -r '.tool_input.file_path // \"a file\"')\""}]},
      {"matcher": "Bash",
       "hooks": [{"type": "command",
        "command": "agentbox sync activity \"$(jq -r '.tool_input.command // \"running a command\"' | cut -c1-70)\""}]}
    ]
  }
}
```

What that buys, in order of how much it matters:

- A row exists from the first moment of the session, with a placeholder purpose.
  The agent's own `announce` replaces it with something meaningful; if the model
  never bothers, the human still sees which directory the session is in rather
  than a nameless dim row.
- The activity line stays current through every edit and every command, so
  "working" and "quiet" mean what they say. A model that never calls
  `set_activity` still produces a truthful ticker.
- None of it costs a token, because none of it goes through the model.

The placeholder purpose is deliberately worded as unfinished. A hook cannot know
why a session exists, and a confident-sounding guess would be worse than an
obvious placeholder: the human would read it as the agent's own answer.

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
