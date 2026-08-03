# AgentBox for agents

AgentBox is a desktop interaction hub. Use it to reach the human at the
keyboard: post an event, ask a blocking question, confirm an action,
announce an act-unless-stopped countdown, or collect a small form. Answers
come back as the call's result. Drive it by running the `agentbox` CLI; every
blocking command prints its result on stdout and sets a stable exit code.

Identity defaults are filled for you: `--agent` (the parent process name),
`--project` (the working directory's basename), `--session` (empty). Pass
them explicitly when running several agents so each gets its own color.

## When to interrupt

Interrupt only for a decision you cannot make safely on your own: a
destructive or irreversible action, a genuinely ambiguous requirement, a
secret you do not have. For progress and results, use `notify` - it is
fire-and-forget and never blocks. A blocking question the user does not
need is the failure mode to avoid.

## Commands

- `notify` - fire-and-forget event. Non-blocking.
  `agentbox notify --level success --title "Tests passed" --body "412, 0 failures"`
  levels: info, success, warning, error, urgent (urgent escalates).
  add up to 3 action buttons that run a command on click:
  `--action "Open PR::gh pr view --web"` (repeatable).

- `ask` - one choice from 2-9 options. Blocks. Prints the chosen label.
  `agentbox ask --title "Deploy?" --option "Run now" --option "Skip" --timeout 300 --default "Skip"`
  `--option "Label::a short description"` adds a description.

- `input` - free text. Blocks. Prints the text. `--multiline` for long input.
  `agentbox input --title "Release tag?"`

- `confirm` - yes/no. Blocks. Exit 0 yes, 1 no.
  `agentbox confirm --title "Push to main?"`

- `veto` - act-unless-stopped. Announce an action with a countdown; it
  proceeds unless the user stops it. Blocks until the window elapses
  (proceed, exit 0) or the user stops it (vetoed, exit 1).
  `agentbox veto --in 15 --title "Pushing to main"`
  Prefer this over a blocking confirm when no answer is the common case.

- `secret` - masked entry for a credential. The daemon writes the value to
  `--to-file PATH` (mode 0600) and it never enters your transcript; pass
  `--stdout` to receive the value instead (it warns the user). Blocks.
  `agentbox secret --title "npm token" --to-file ./token`

- `form` - up to 6 typed fields in one card. Blocks. Prints JSON.
  `agentbox form --title "Release" --field choice:env:staging,prod --field text:tag --field bool:notify=yes`
  field spec: `type:key[:opt1,opt2][=default]`, type is choice|text|bool.

- `progress` - a live progress bar for a long task. Non-blocking; reads
  stdin. Each line is `NN` (percent), `NN status text`, or a bare status
  line; EOF finishes it (a completion toast follows). `--indeterminate`
  starts a spinner when there is no known fraction.
  `migrate.sh | agentbox progress --title "Migrating users"`

- `control` - ask for the desktop before driving it, and say what you are
  doing while you have it. `request` blocks; silence for `--window` seconds
  is consent. Exit 0 granted, 1 denied, 3 another agent holds it.
  `agentbox control request "clicking through the board" --window 20`
  `agentbox control activity "reading the takeaway aloud"` then `agentbox control release`
  One always-on-top strip stays up for the whole run: on screen means hands
  off, gone means the desktop is theirs. Release the moment you stop.

- `stats` - interruption history. `agentbox stats --since 7d` (or 24h, 30d, 0
  for all time); `--json` for the raw object.

Add `--json` to any blocking command for the full result object instead of
the bare answer.

## Exit codes (stable contract)

- 0 - answered / yes / proceeded
- 1 - no / vetoed
- 2 - usage error (bad flags); fix the command and retry
- 3 - unanswered: timed out with no default delivered
- 4 - transport or daemon error

## Result shapes (--json)

- ask / input: `{"id":"k..","answered":true,"answer":"Run now"}`
- reply hatch: `{"id":"k..","answered":true,"reply":"free text"}`
- timeout:     `{"id":"k..","answered":false,"default_applied":true,"answer":"Skip"}`
- confirm:     `answer` is `"yes"` or `"no"`
- veto:        `{"id":"k..","vetoed":true}` or `{"id":"k..","vetoed":false}`
- form:        `{"id":"k..","answered":true,"values":{"env":"prod","tag":"v1.4.0"}}`

Every choice and confirm card also accepts free text as an answer (the
reply hatch) unless you pass `--strict`. Handle a `reply` field as well as
`answer`. `agentbox schema` prints the full JSON Schema for these shapes.

## MCP

`agentbox mcp` is an MCP stdio server; register it so an MCP host (such as
Claude Code) can call these tools. Run `agentbox docs setup` for a paste-ready
`.mcp.json` and hook snippets.

- `notify_user` -> notify (non-blocking)
- `ask_user` -> ask; pass `options` for a single choice, omit them for free text
- `confirm_action` -> confirm; returns `confirmed` true/false
- `act_unless_stopped` -> veto; returns `vetoed` true/false
- `ask_user_form` -> form; returns the field `values`
- `request_secret` -> secret; returns a file `path`, never the value
- `request_review` -> diff review; pass `diff` (or a `path`); blocks and
  returns `approved` true/false plus any `comment`
- `show_document` -> open a markdown file or inline content in the reading
  window (non-blocking); far richer than dumping markdown into the terminal
- `speak` -> say one line out loud (non-blocking; `wait` returns after it is
  heard). No card, no inbox entry - an aside, not an interruption
- `drive_desktop` -> move the pointer, click and type on the human's desktop,
  as they would; validate-then-run script, one step per line. Announce first,
  drive sparingly
- `request_control` -> BLOCK until the human hands you their desktop, then a
  HANDS OFF strip stays on their screen for as long as you hold it. Do this
  before any run of drive_desktop worth the name, and before driving anything
  else on their screen. Returns `granted`, `denied`, or `held_by` (another
  agent has it - wait, do not drive)
- `set_activity` -> the line the strip shows, as the work moves on
  (non-blocking). `release_control` -> give the desktop back and take the strip
  down, the moment you stop driving. Presence is the signal, so a card or a
  spoken line cannot replace these
- `report_progress` -> a live progress bar (non-blocking). Omit `id` and set
  `title` to start; pass the returned `id` on every later call to update
  `percent` (or `indeterminate`); call `done` (with `error` to fail). Prefer
  this over repeated `notify_user` for incremental progress
- `show_artifact` -> run interactive HTML in a window: a chart to hover, a slider
  to drag, a panel to click. Non-blocking; returns an `artifact_id`
- `await_artifact_event` -> BLOCK until the human does something in an artifact,
  and return what they did (`name` and `data`)
- `read_artifact_events` -> take what they have done already, without blocking;
  repeats of one name are coalesced to the newest
- `create_walkthrough` -> a durable, step-by-step code review on the review
  board. Non-blocking; returns a `walkthrough_id`
- `await_walkthrough` -> BLOCK until the human submits that review; the whole
  handback arrives in one turn, unclear steps first
- `read_walkthrough` / `list_walkthroughs` / `delete_walkthrough` -> the stored
  review library; `read` with `ack:true` takes a waiting submission exactly once
- `amend_walkthrough` -> refuses in this build; create a fresh walkthrough for
  revised content
- `list_assignments` / `read_assignment` -> the work AgentBox runs on its own, and one
  of them whole with AgentBox's diagnosis of its prompt and knobs
- `create_assignment` / `update_assignment` -> write one, or improve one. An
  update changes only the fields you send, and `params` merges over the stored
  values - an edit must not blank what the human tuned
- `run_assignment` -> run one now (non-blocking, returns a `run_id`);
  `assignment_runs` -> how the last ones went. `delete_assignment` removes it
  with its history, so prefer `enabled:false` to pause

## Artifacts

`show_artifact` runs your HTML. Write a self-contained document, or a React
component module with an `export default` - React and Tailwind are already in the
page, so `import React from "react"`, JSX and utility classes all work. Two rules,
both absolute:

- **No CDN and no other package.** The artifact runs with no network at all, so a
  script tag or an import of anything but react/react-dom is a missing library.
- **`window.agentbox.emit(name, data)` is the only way out.** It cannot fetch, cannot
  reach AgentBox, cannot store anything. What the human does reaches you because you
  called `await_artifact_event` (or `read_artifact_events`), and then you act with
  your own tools.

That is the loop worth building: show a control panel, wait on it, do the work the
human asked for by clicking, then show the result. Use it when a question has more
than a few answers, or when the answer is a number, a shape or a selection rather
than one of nine options - a card is better for anything a sentence can ask.

## Walkthroughs

For a change worth walking rather than skimming, hand over a walkthrough
instead of a diff card: `create_walkthrough` takes a declarative spec -
ordered steps with prose, citations `{path, lines:[from,to]}` pinned to a
commit, and the change's unified diff as the manifest - and opens the review
board. The human marks each step understood or unclear, writes notes and
anchors comments to lines; everything persists across sessions.
`await_walkthrough` returns the whole review in one turn. Three spec rules,
all validated with directions: never state added/removed on a file-backed
block (the diff is the only carrier); never put literal line numbers in
prose (bind a phrase to a code region instead); end a diff-carrying review
with a check step. CLI:
`agentbox walkthrough create --spec review.json | await ID | read ID --ack | list | delete ID`.

**Read the standard before you write one.** How to structure the steps, where
the explanation goes versus the annotations, what coverage has to account for:
MCP resource `agentbox://standards/walkthrough`, or `agentbox docs walkthrough`. Four
things it will tell you that are easy to miss:

- **Paragraphs are explicit.** Prose is inline segments, so a bound phrase can
  sit mid-sentence. Set `"p": true` on the segment that starts each paragraph
  or the whole step renders as one wall.
- **Notes are the annotation channel.** `notes: [{at: [from,to], text}]` on a
  block renders as a numbered badge on the line and the text in the margin
  beside the code. That is where the "why" belongs - not in the prose above.
- **Binds are how prose points at code.** `{"t": "the guard", "bind": "guard"}`
  with `binds: {guard: {block: 0, lines: [77,79]}}` lights those lines when the
  reader is on the phrase. It is also the answer to the no-line-numbers rule.
- **The glossary keeps definitions out of the prose.** `glossary: [{term,
  short, body?, also?}]` on the spec. AgentBox marks the first occurrence of each
  term in each step and opens the entry only when the reader asks for it
  (click, or `g`). Define the domain acronyms and house terms this reader
  cannot guess - and spell them in the prose the way the entry spells them, or
  agentbox warns that nothing can reach them.

## Assignments

An assignment inverts the usual direction: AgentBox summons an agent, on a schedule
or on demand, with the whole toolbox available while it runs. A run is an
ordinary session, so the human can open it, read it and take it over.

You write them. Propose a prompt and its knobs with `create_assignment`,
`run_assignment` once so the human sees it work, read it back with
`assignment_runs`, and keep updating until they are satisfied.

`schedule` is empty (ad-hoc), `every 30m` / `every 4h` / `every 1d` (from the
last run), or `daily 09:00` / `weekly mon 09:00` (wall clock). A missed slot is
skipped and recorded, never caught up. One minute is the floor.

The prompt is a template: `{{key}}` is substituted from the parameters, and a
placeholder nothing fills is left verbatim rather than dropped. Declare a knob
for each in `spec` - `[{key, type, label?, help?, default?, min?, max?, unit?,
values?, body?}]`, type `text|number|slider|toggle|enum|path|markdown` - and AgentBox
renders the form. `panel_html` is the escape hatch (React/Tailwind in the
artifact sandbox, values out through `window.agentbox.emit`); declare a spec as well,
because a panel that fails to load must never make an assignment uneditable.

Write for an agent nobody is watching: say what to do, what is worth
interrupting for, and how to report.

The blocking tools wait for the human exactly like the CLI, and their
results carry the same fields as the JSON shapes above.
