# 08 - Assignments (M12)

An **assignment** is a piece of work AgentBox gives to a Claude agent on its own, on a
schedule or on demand, with the whole of AgentBox available to it while it runs. It is
the inversion of everything AgentBox did before: until now an agent summoned AgentBox, and
here AgentBox summons the agent.

Decided with Boris 2026-08-01 (session 34). FR82. Depends on FR81 (the main
panel), because an assignment with no surface is a cron job with extra steps.

## Why

Boris: *"periodically check the usage of Claude and display a summary to the
user, making it a warning notification when getting critical usage and maybe even
collecting usage statistics for later analysis."*

That one example carries the whole shape. Something has to run without him
starting it, it has to reach him only when it matters, it has to escalate when
the number is bad, and what it learned has to still be there next month. AgentBox
already has every one of those pieces - a scheduler is the missing one.

## The three kinds of trigger

| Kind | Written as | Runs |
|---|---|---|
| ad-hoc | `schedule` empty | only when a human or an agent asks |
| periodic | `every 30m`, `every 4h`, `every 1d` | on an interval from the last run |
| scheduled | `daily 09:00`, `weekly mon 09:00` | at a wall-clock time |

**A missed slot is skipped, not caught up** (Boris, 2026-08-01). A usage check for
a window that has already passed is noise, and a laptop that was shut for the
weekend would otherwise wake up and fire three of them at once. The run is
recorded as `skipped` with the count, so the panel can say "3 runs missed while
off" and nothing is silently lost.

## Parameters: two ways, one source of truth

The prompt carries `{{placeholders}}`. What fills them is a **parameter spec**,
and it can be expressed two ways - Boris's call, 2026-08-01: *"built-in controls
and markdown is probably more uniform and will allow a more professional look,
[custom HTML] to be future-proof"*.

1. **Typed knobs (the default).** A JSON spec of `text | number | slider |
   toggle | enum | path | markdown`, rendered by AgentBox with the same
   descriptor-driven machinery the Settings surface uses. Validated, themed,
   editable by hand, and uniform across every assignment.
2. **A custom HTML panel (the escape hatch).** The agent writes a React/Tailwind
   panel that runs in AgentBox's existing artifact sandbox (ADR-0010, no network) and
   reports values back through `window.agentbox.emit`. For the assignment whose
   controls the knobs cannot express.

**The values live in the database either way.** That is the rule that keeps the
escape hatch from being a trapdoor: a custom panel that throws on load, or an
agent that writes a broken one, must never make an assignment uneditable. The
typed knobs stay available for every assignment, so there is always a way in.

Substitution is literal `{{key}}` -> value at run time. A placeholder with no
parameter behind it is a save-time warning, not a run-time surprise.

## The agent writes the assignment

Boris: *"upon creation, the AI agent itself should help with generating the
initial prompt and the configuration panel for it until the user is satisfied.
The AI agent should have full access to these so that it can help adjusting and
improving assignments as we go along."*

So the CRUD is an MCP surface before it is a UI: `list_assignments`,
`read_assignment`, `create_assignment`, `update_assignment`,
`delete_assignment`, `run_assignment`, `assignment_runs`. Any agent in any
project can read an assignment, propose a better prompt, and write it back.
"New assignment" in the panel opens an AgentBox session pointed at exactly those tools,
so the authoring conversation happens where the assignment will live.

## A run is a session

A run spawns a headless `claude` child through `internal/session` - the same
driver the Session surface already uses - with:

- the prompt, parameters substituted,
- `--model` from the assignment (added to the driver for this),
- AgentBox's own MCP server on `--mcp-config`, so the running agent can `notify_user`,
  `ask_user`, `show_document`, `report_progress` - the whole toolbox,
- a brief telling it that it is an assignment, how it is expected to report, and
  that a human may not be watching.

Because it is an ordinary session, it renders in the Session surface: a run is
something you can open, read and take over, not a black box that mailed you a
result.

## Storage (migration 0007)

`assignments` holds the definition (name, prompt, params spec, params, panel
html, model, mode, dir, schedule, enabled, timestamps, next run).
`assignment_runs` holds one row per run: when, what triggered it, the parameter
values actually used, the state, the summary, the error, the session id, and a
`data` blob the run may record for later analysis. That last column is what
"collecting usage statistics" means - the stats outlive the run that took them.

## How a run hands something back (built 2026-08-01)

The run's **last assistant message is the summary** - what the panel shows and
what `assignment_runs` returns. The brief every run is spawned with
(`internal/manual/assignment.md`) says so, and says the rest of what changes
when nobody typed the prompt: report in the reply, interrupt only when it
matters, do not end with "let me know if you would like me to continue".

To keep a measurement, a run ends with a fenced block tagged `agentbox-data`:

    ```agentbox-data
    {"usage_pct": 82, "window": "7d"}
    ```

AgentBox lifts it out into the run's `data` column and takes it out of the prose, so
the human reads a report and a month of runs reads back as a series. A block
with no closing fence is left in the summary rather than swallowing it - a
half-written block is a mistake worth seeing.

Two properties of a run being a session, both enforced:

- It does not take the human's selection. An assignment firing while the panel
  is open must not move them off the conversation they are typing into.
- Its child is stopped when it finishes and the transcript stays. A daily
  assignment that left one alive per run would have thirty `claude` processes
  idling by the end of the month.

An hour is the ceiling on a single run, after which it is killed and recorded as
overrun. Not a knob: the number exists so a wedged child cannot hold its
assignment's in-flight flag forever, which would mean the assignment silently
never runs again.

## Open

- The custom HTML panel stores, edits and round-trips, but the surface shows a
  note instead of running it. Running it needs a channel of its own: the
  artifact machinery routes `window.agentbox.emit` to whichever agent is awaiting the
  artifact, and a parameter panel's values have to reach `SetAssignmentParams`
  instead. The typed knobs stay the way in that always works.
- Whether a run should be able to answer its own cards (drive the desktop) or
  whether that stays a human decision per assignment.
- Concurrency: today one run per assignment at a time, and a second launch is
  refused rather than queued - a check that overran its own interval must not be
  joined by another on top of it.
