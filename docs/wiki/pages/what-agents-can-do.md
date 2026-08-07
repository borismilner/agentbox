# Thirty-nine tools, and the eleven that wait for you

> **In short.** Thirty-nine tools reach you over MCP, and the distinction worth
> learning is which of them block. Eleven park the agent until you act; the rest
> return at once. Nearly all of them are a shell command too, so a hook or a cron
> job can ask you something the same way.
>
> **Read on if** you are picking a tool or writing a hook. **Skip to**
> [[Settings and defaults|settings]], or [[Words used here|glossary]].

Read the tables below by their last column first. A non-blocking tool is something
an agent tells you and then moves on from. A blocking one is a call that stops until
you answer it, which is the only kind that can change what the agent does next.

## What blocking costs the agent, and what it buys you

A parked call spends nothing while it waits. It is not polling and not burning
turns: the daemon holds the call and answers it when you do, which is why an agent
can afford to wait an hour for one word from you.

What keeps it alive is a tick. Every parked call gets a progress notification after
60 seconds and then every 60 seconds, because the client abandons a tool call that
has been silent for 1800 seconds. Without that tick, every blocking card was on a
thirty-minute fuse, and the answer you typed at minute 40 arrived at a caller that
had already given up.

Most blocking tools take a `timeout_s`, where `0` means wait as long as the caller
is patient. Four are different, and the differences are worth knowing:
`act_unless_stopped` defaults to a 15 second window, `request_control` to 20
seconds, and `confirm_action` and `request_review` have no timeout field at all, so
they wait until they are answered. A parked lock or signal wait is capped by the
daemon at 25 minutes, so it comes back as an honest timeout the agent can read
rather than as a transport error it cannot.

## The thirty-nine, group by group

### Asking, telling and taking back

| Tool | What it does for you | Blocks |
|---|---|---|
| `notify_user` | a message on your screen and in the inbox, nothing to answer | no |
| `retract` | takes back an item you have not dealt with yet | no |
| `ask_user` | a question with 2 to 9 numbered options, or a free-text field | yes |
| `confirm_action` | a yes or a no | yes |
| `act_unless_stopped` | says what it is about to do, with a countdown to stop it | yes |
| `ask_user_form` | up to six typed fields in one card, answered in one round trip | yes |
| `request_review` | a unified diff with Approve and Request changes | yes |
| `report_progress` | a live bar for a long job, in its own window, never in the queue | no |

### Documents, artifacts, and what you clicked in them

| Tool | What it does for you | Blocks |
|---|---|---|
| `show_document` | opens markdown in the reading window instead of a terminal | no |
| `show_artifact` | runs an interface the agent wrote, for you to operate | no |
| `await_artifact_event` | waits for you to click, drag or submit in that artifact | yes |
| `read_artifact_events` | takes what you have done since it last looked | no |

### One line spoken, one credential asked for

| Tool | What it does for you | Blocks |
|---|---|---|
| `speak` | says one sentence out loud, with no card and no inbox entry | only if it asks to wait |
| `request_secret` | masked entry, and the agent is handed a path rather than the value | yes |

### The pointer, the keyboard and the strip

| Tool | What it does for you | Blocks |
|---|---|---|
| `request_control` | asks for your desktop and puts the hands-off strip on screen | yes |
| `set_activity` | keeps the line saying what it is doing right now current | no |
| `drive_desktop` | moves the pointer, clicks and types as if you had | for the length of the script |
| `release_control` | gives the desktop back and takes the strip down | no |

### Peers, locks, signals and a blackboard

| Tool | What it does for you | Blocks |
|---|---|---|
| `announce` | says what a session is for, and learns who else is working here | no |
| `list_agents` | who else is running, with each one's purpose and activity | no |
| `acquire_lock` | parks until a named resource is its turn | yes |
| `try_lock` | takes the lock only if it is free right now, and never waits | no |
| `release_lock` | hands the lock to whoever is queued behind | no |
| `post_signal` | tells the other agents on this machine that something happened | no |
| `await_signal` | parks until a peer posts a matching signal | yes |
| `shared` | reads and writes a small shared blackboard, compare-and-swap | no |

<sub>`acquire_lock` and `try_lock` differ in exactly one way, and it is the last
column: one queues, the other refuses and names who is holding it.</sub>

### Assignments, walkthroughs, and the one tool that always refuses

| Tool | What it does for you | Blocks |
|---|---|---|
| `list_assignments` | the work you set up for AgentBox to run on its own | no |
| `read_assignment` | one assignment whole, with its recent runs | no |
| `create_assignment` | adds one: a prompt, typed parameters, a schedule | no |
| `update_assignment` | changes only the fields it sends, leaving your knobs alone | no |
| `delete_assignment` | removes one with its whole run history | no |
| `run_assignment` | starts a run now and returns a run id, without waiting for it | no |
| `assignment_runs` | the run history, newest first, with what each one recorded | no |
| `create_walkthrough` | puts a step-by-step review on your board | no |
| `await_walkthrough` | waits for your whole review, handed back in one turn | yes |
| `read_walkthrough` | the stored state: your marks and comments so far | no |
| `list_walkthroughs` | stored reviews, most recently touched first | no |
| `amend_walkthrough` | **nothing. It is registered and always refuses** | no |
| `delete_walkthrough` | deletes one permanently, marks and comments included | no |

<sub>`amend_walkthrough` is on the list because it is in the tool list an agent
sees, and its own description sends the agent to create a fresh walkthrough
instead. Revising a review you have already started marking up is not built.</sub>

## The same daemon from a shell script

Nearly all of it is also a command, which is how a git hook, a Makefile or a cron
job reaches you with no agent in the middle. A blocking command prints the answer on
stdout and puts your decision in its exit status:

```sh
# .git/hooks/pre-push
if ! agentbox confirm --title "Push $(git rev-parse --abbrev-ref HEAD) to origin?" \
        --body "$(git log --oneline @{u}.. | head -5)" --timeout 120; then
    echo "not pushing" >&2
    exit 1
fi
```

| Code | What it means |
|---|---|
| `0` | answered, yes, or proceeded |
| `1` | no, or vetoed |
| `2` | the command was wrong |
| `3` | nobody answered before the timeout |
| `4` | AgentBox itself failed |

That is the whole contract, and it is why the hook above parses nothing. The
distinction between `1` and `3` is the one that earns its keep: "he said no" and "he
was not there" are different facts, and a script that treats them as one either
pushes when nobody was watching or refuses when somebody said yes.

Locks work the same way, wrapping a command instead of answering a question:

```sh
agentbox sync lock deploy:checkout-api --timeout 600 -- make deploy
```

That waits for its turn, then exits with `make deploy`'s own status. See
[[taking turns|taking-turns]] for what the queue behind it looks like, and
[[the install page|install]] for getting the tools in front of an agent in the
first place.

> [!NOTE]
> The `sync` verbs are the one exception to the table above, and it is deliberate:
> there, an `await` that times out exits `1` rather than `3`, because that verb's
> own contract is "0 done, 1 refused or not granted". A script that branches on `3`
> around a signal wait has a branch that never runs.

## What this page is not

It is the map, not the reference. Every tool carries a description written for the
agent that will call it, with its arguments, its defaults, and the anti-patterns
that got somebody into trouble. That ships inside the binary:

```sh
agentbox docs agent
```

<details>
<summary>The nine tools with no shell command</summary>

`try_lock`, `amend_walkthrough`, and all seven assignment tools
(`list_assignments`, `read_assignment`, `create_assignment`, `update_assignment`,
`delete_assignment`, `run_assignment`, `assignment_runs`).

Assignments are authored over MCP because an agent writes the prompt template and
the parameter spec, while your side of them is a surface in the app window rather
than a command. `try_lock` has no shell form because `agentbox sync lock` is the
blocking acquire: a script that cannot afford to wait gives it a short `--timeout`
and reads exit code `1`.

It goes the other way too. `agentbox walkthrough open`, `sync attach`, `sync break`
and every human-only verb (`status`, `inbox`, `dnd`, `mute`, `pending`, `stats`,
`summon`, `quit`) have no MCP tool, because they are things you do rather than
things an agent asks for.

</details>

**Next:** [[the defaults you might want to change|settings]], or
[[where it will not run|limits]].
