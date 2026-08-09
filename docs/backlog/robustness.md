# Backlog - what would make AgentBox dependable unattended

Written 2026-08-07 against commit `69230d4`, by reading `internal/daemon` in
full, `internal/store`, `internal/server`, `internal/client`, `internal/mcp`,
`internal/webui`, `internal/hand`, `internal/hotkey`, `internal/session`,
`internal/proto/rpc.go` and the whole test tree. `docs/wiki/FACTS.md` was the
starting point for what is true; where a doc and the code disagreed, the code
won.

The bar is the product's own claim: a message from an agent to a human is never
lost. Every item below is measured against that, so the ordering is by what the
user loses and not by how hard it is to hit. An item that cannot be answered is
a different product from a window that looks wrong, and the bands say which is
which.

Three of the five worst were reproduced with throwaway tests against the real
daemon rather than argued from a reading. Those say so, and the recipe is in the
**Test** field.

Numbering is `R-nn`, sequential across bands, and does not collide with `F-nn`
in `features.md` or the `FR` series.

**The bands**

| Band | What defines it |
|---|---|
| [A](#band-a) | A question or an answer is destroyed, and neither side is told |
| [B](#band-b) | The hub stops serving, so nothing reaches anybody until a human notices |
| [C](#band-c) | The desktop layer fails while the daemon records a delivery |
| [D](#band-d) | Agent-authored content shapes a surface it should only fill |
| [E](#band-e) | The coordination primitives mislead instead of blocking |
| [F](#band-f) | We would not find out: the tests and the log |

---

## Band A

*A question or an answer is destroyed, and neither side is told. This is the
band that decides whether the product is what it says it is.*

### R-01. A question that flood control collapsed cannot be answered once its summary is dismissed

> **Fixed** in `1d00fd2` (2026-08-07), with the reproduction kept as a test.
> `Promote` now falls back to the store, which is the fix this entry proposed.
> Deployed 2026-08-07. Left here in full because the reasoning is what stops it
> coming back.

**How it fails.** Flood control is on by default: three items from one session
inside ten seconds (`internal/config/config.go:269-270`). The fourth is
collapsed, which means it is written to the store as pending and then
deliberately not put on the queue (`internal/daemon/daemon.go:1232-1241`, the
`case collapsed:` arm sets `shown = nil` and never calls `enqueueLocked`). The
human then dismisses the stack card. `sweepStack` retires the collapsed
notifications and spares every blocking row on purpose
(`internal/daemon/flood.go:370-384`), so the question is still pending with its
caller still parked. It is now in no queue, is not `d.current`, and has no stack
card to be opened from. The inbox lists it, because the inbox reads the store
(`internal/webui/inbox.go:190`), and the row's hint for a text, form, secret or
diff item is "enter open", which routes to `Promote`
(`internal/webui/inbox.go:604`). `Promote` looks for the item in `d.current` and
`d.queue`, finds neither, and returns without a log line or a repaint
(`internal/daemon/daemon.go:2089-2099`). Pressing enter does nothing, twice,
forever. A choice or confirm item escapes this because its triage keys answer
the store directly; the four kinds that need a card to be answered do not.
**Reproduced**: a `request_review` collapsed behind three notifies, stack
dismissed, `Promote` a no-op, the screen showing an unrelated toast.
**Consequence.** An item is lost in the strongest sense: it exists, the agent is
parked on it, the human can see the row, and there is no sequence of keys or
clicks that answers it. `request_review` and `ask_user_form` have no default
timeout, so the agent waits until its transport gives up.
**Where.** `internal/daemon/daemon.go:2089-2099`, `internal/daemon/flood.go:370-384`,
`internal/webui/inbox.go:598-606`.
**Fix.** `Promote` should fall back to the store: if the id is pending and not in
memory, read it, put it at the head of the queue and advance, which is what
`OpenStacked` already does at `flood.go:323-351`. The tempting wrong fix is to
enqueue collapsed items after all, which undoes FR30, or to have `sweepStack`
retire blocking rows too, which answers a question on the human's behalf.
**Test that would have caught it.** `TestAQuestionCaughtInABurstStaysAnswerable`
(`internal/daemon/flood_test.go:329`) stops one step short: it answers through
the stack card while the card is on screen. The missing test dismisses the stack
first and then answers the survivor through the inbox route, for each of the
four kinds whose triage is "open". `TestDismissingAStackKeepsTheQuestionsAndClearsTheNotices`
(`flood_test.go:442`) asserts the item is "still pending for inbox triage" and
never tries the triage.
**Size.** hours.
**Confidence.** Reproduced with a probe test.

### R-02. A store write that fails takes the human's answer with it, and the card claims it was delivered

> **Fixed** in `1d00fd2` (2026-08-07), with the reproduction kept as a test.
> `resolve` no longer returns on a store error while telling the card it shipped.
> Deployed 2026-08-07. Left here in full for the same reason as R-01.

**How it fails.** Every answer goes through `resolve`, which treats the store
transition as the arbiter. If `st.Resolve` returns anything other than
`ErrNotFound` it logs `store.resolve_failed` and returns false
(`internal/daemon/daemon.go:1770-1775`). Nothing after that runs: the waiter is
not delivered to, the queue does not advance, and no `Present` is issued. The
undo grace makes it worse rather than better. `finalizeGrace` clears
`d.graced` before calling `resolve` (`daemon.go:2041-2050`), so when the write
fails the grace record is gone, the last view painted is still the collapsed
"Answered: Yes" strip, and `Undo` now returns early because there is nothing
graced (`daemon.go:2054-2058`). The item is pending, the agent is parked, and
the screen says the answer shipped. Triggers: a full disk, a database locked
past the 5000 ms busy timeout, a store closed under a racing shutdown.
**Reproduced**: answer, close the store, then observe the waiter still
registered, the grace record cleared, and `Graced=true` still the last painted
view.
**Consequence.** The answer is destroyed. The human has every reason to believe
otherwise, so he will not give it again, and the agent waits.
**Where.** `internal/daemon/daemon.go:1770-1775`, `:2041-2050`.
**Fix.** A failed resolve has to be surfaced, not only logged: put the item back
on screen and raise a daemon-authored error card saying the answer could not be
recorded. The wrong fix is retrying the write silently, which turns a visible
failure into an invisible one and can deliver twice.
**Test that would have caught it.** A fake store whose `Resolve` returns an error
(`internal/daemon/shared_test.go:385` already has the pattern for a two-method
fake), asserting that the caller is unblocked or the card comes back, and that
the last view is not the answered strip.
**Size.** a day.
**Confidence.** Reproduced with a probe test.

### R-03. An answer undone after a deadline has passed strands the caller and blinds the caller-gone detector

**How it fails.** `ask` with `timeout_s: 60`. At t=59 the human answers and the
undo grace opens. At t=60 the arrival-anchored timer fires, `resolve(expired)`
bounces off the grace by design (`daemon.go:1764-1767`), and the handler falls
through to `return <-wait, nil` (`daemon.go:1319-1324`), a bare channel receive
with no deadline and no `ctx` branch. Then the human presses undo. The grace is
cancelled, the item is pending again, and the timer has already been consumed
and is never re-armed. Two things follow. The `timeout_s` the agent asked for no
longer exists, so a call that promised to proceed on its default parks
indefinitely. And because the handler is no longer in the `select`, a caller
whose socket drops is never noticed: `callerGone` is never called, `d.gone` stays
empty, and `callerStateLocked` keeps reporting `CallerLive`
(`daemon.go:1698-1710`). **Reproduced**: after the undo the caller heard nothing
past its own deadline, and cancelling its context left the card showing a live
caller.
**Consequence.** The human is shown a live-caller dot for an agent that has gone
and spends a decision on a question nobody will read. The agent that asked for a
bounded wait gets an unbounded one.
**Where.** `internal/daemon/daemon.go:1307-1324` (both bare receives), `:2054-2066`.
**Fix.** Replace both `return <-wait, nil` with a select over `wait` and
`ctx.Done()`, and re-arm the expiry when `Undo` puts an item back. The tempting
wrong fix is to forbid undo after the deadline, which takes away the one control
that is live while the strip is showing.
**Test that would have caught it.** Answer at 90 % of a one-second timeout, undo
inside the grace, then assert two things: the caller still gets a result by the
original deadline, and cancelling the caller's context flips the card to
`CallerGone`. `TestTimeoutCannotStealGracedAnswer` (`daemon_test.go:858`) covers
the half of this that works.
**Size.** hours.
**Confidence.** Reproduced with a probe test.

### R-04. An item over 4 MB kills the connection with no reply, and takes every other call on it

**How it fails.** `Item.Validate` has no length check on `Title`, `Body` or
`Diff` (`internal/proto/types.go:251-352`). The only bound in the path is the
wire line: `sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)`
(`internal/proto/rpc.go:997`). A `request_review` carrying a large diff, a
`show_document` with large inline content, or a big walkthrough spec makes the
daemon's scanner return `ErrTooLong`, `Serve` returns, and the connection closes
with no response written. The client's read loop is capped the same way
(`rpc.go:951`), so an oversized *response* does the same in reverse and closes
every pending call on that connection at once (`rpc.go:964-974`). The
walkthrough path is the one input that was sized against this cap deliberately
(`internal/walkthrough/spec.go:22-46`); the item path was not.
**Consequence.** The message is lost, and the agent is told
`connection closed during agentbox.v1.ask: EOF`, which names neither size nor the
field at fault. Nothing is stored, so there is no history row either.
**Where.** `internal/proto/rpc.go:951`, `:997`, `:1047-1049`;
`internal/proto/types.go:251-352`.
**Fix.** Cap `Body`, `Title` and `Diff` in `Validate` with a refusal that names
the field and the limit, sized under the wire cap the way
`internal/walkthrough/spec.go` does. Separately, `Serve` should answer
`ErrTooLong` with an RPC error before it drops the connection. The wrong fix is
raising the scanner limit, which moves the cliff and leaves the same silence at
the new edge.
**Test that would have caught it.** A table over `Validate` asserting a refusal
at each cap, plus a round trip over a real `net.Pipe` with a 5 MB body asserting
the caller gets a readable error rather than EOF. `internal/proto/proto_test.go`
already has the pipe harness.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-05. The card surface never pulls its state, so a slow mount leaves a blank window over a live question

**How it fails.** Every other surface pulls on mount and says so:
`Bridge.Ready` marks the prompt surfaces ready and its own comment records that
the card's first view is pushed rather than pulled
(`internal/webui/bridge.go:31-41`). `armCard` waits at most two seconds on the
ready channel, logs `webui.card_ready_timeout`, and then emits anyway
(`internal/webui/webui.go:531-540`). `u.emit` is a fire-and-forget Wails event
(`webui.go:569-574`) with nothing to buffer it, and the card component only
subscribes to `agentbox:view`, so a payload that arrives before the bundle mounts
is gone. Nothing re-sends until the next queue change. The window is created
fresh whenever the queue has emptied, because `closeCardNow` drops the window and
resets the ready channel (`webui.go:550-556`), so every isolated item pays the
full mount cost of a new webview. Two seconds is a guess about WebKit process
startup on a loaded machine, and it is not measured anywhere.
**Consequence.** The human hears the earcon, sees an empty card, and the daemon
has already logged `item.displayed` and armed escalation. For a blocking item the
agent waits on a question that is on screen only in the sense that a window
exists.
**Where.** `internal/webui/webui.go:531-540`, `:569-574`, `:550-556`;
`internal/webui/bridge.go:31-41`.
**Fix.** Give the card and toast surfaces a `Bridge.View()` they call on mount,
and keep the push as the fast path. That is the pattern the other six surfaces
already use. The wrong fix is a longer timeout.
**Test that would have caught it.** Nothing existing could: no test renders a
Svelte component or exercises a webview (see R-40). The check that would catch it
is the pull path in Go plus one live exercise, driving the card window from a
cold daemon start under load and reading the card back with
`tools/uidrive/uidrive.py shot`.
**Size.** hours for the pull, a day with the live check.
**Confidence.** Confirmed by reading the code; the mount latency that triggers it
is inferred and worth measuring.

### R-06. Nothing checks that a window ever mapped, and the auto-dismiss clock starts before it could have

**How it fails.** `setCurrentLocked` logs `item.displayed` and, for an info or
success notify, arms `time.AfterFunc(d.cfg.ToastDuration, toastExpired)` while
still holding the daemon lock and before `Present` has reached the UI at all
(`internal/daemon/daemon.go:1496-1512`). `toastExpired` resolves the item to
dismissed (`:1717-1727`). Nothing in `internal/webui` reads `MapState`,
subscribes to `MapNotify`, or reports back that a surface appeared: the only
window-attribute reads in the tree are in `internal/hand`. So the six-second
dismissal is a promise about the clock, not about the screen, and a window that
failed to map is retired on schedule as though it had been seen.
**Consequence.** A notification is lost silently, with a history row saying
dismissed. FR44's missed-while-away marker only fires when the human was idle, so
a window that never appeared while he was at the desk is recorded as read.
**Where.** `internal/daemon/daemon.go:1496-1512`, `:1717-1727`.
**Fix.** Have the UI acknowledge a mapped surface (it already knows the xid at
`internal/webui/webui.go:498-505`) and start the dismissal clock from that
acknowledgement, with a bounded fallback that raises a warning rather than
retiring the item. The wrong fix is to lengthen the toast duration.
**Test that would have caught it.** A fake presenter whose `Present` reports "the
window did not appear" and an assertion that the item is not resolved to
dismissed. The fake UI at `internal/daemon/daemon_test.go:23` has no notion of a
window at all, which is why this was invisible.
**Size.** a day.
**Confidence.** Confirmed by reading the code.

### R-07. Shutdown does not drain, so a question in flight is cancelled or stranded depending on a race

**How it fails.** `shutdown` runs `BeginShutdown()`, `cancel()`, `lst.Close()`,
`st.Close()`, `os.Exit(0)` with nothing waiting for the per-connection goroutines
(`cmd/agentbox/daemon.go:485-506`). `Conn.Serve` does have a `wg.Wait()` and
cancels before waiting on purpose (`internal/proto/rpc.go:980-994`), but nothing
in the shutdown path waits for `Serve` to return. So a parked `ask` handler is in
a race with `os.Exit`. If it wins, it takes the `closing` branch, resolves the
item to **cancelled** and returns `CodeShuttingDown`
(`internal/daemon/daemon.go:1325-1332`), which means the item will not be
re-presented, contradicting the comment two lines above it and the wiki's "pending
is the one state a daemon restart reads back off disk". If it loses, the item
stays pending, comes back after the restart with `CallerAwaiting`, and the agent
sees a transport EOF instead of a shutdown error. Either outcome is defensible;
which one happens is timing. `make deploy` runs this path on every deploy
(`Makefile:261`, `:277-286`).
**Consequence.** In the cancelled case a question the human was mid-way through
answering disappears with no trace on screen. In the stranded case he is
re-presented a question whose caller is dead. Neither is a lost answer, but the
interaction is destroyed and the behaviour is not reproducible.
**Where.** `cmd/agentbox/daemon.go:485-506`, `internal/daemon/daemon.go:1325-1332`.
**Fix.** Pick one semantic and drain to it. Leaving items pending is the better
one, because it matches the documented promise: drop the cancel, keep
`CodeShuttingDown`, and give shutdown a bounded wait on the server's goroutines
before closing the store. Separately, `make deploy` should refuse or warn when a
blocking item is pending, the way it already warns on a dirty tree.
**Test that would have caught it.** Park an ask, call `BeginShutdown`, cancel the
server context, and assert the item is still pending and the caller got
`CodeShuttingDown`. No test covers the shutdown branch at all.
**Size.** a day.
**Confidence.** Confirmed by reading the code.

### R-08. An answer inside the undo grace does not survive a restart

**How it fails.** The undo grace holds the outcome in memory only: `graced`
carries the outcome and a timer, and the store is not touched until
`finalizeGrace` runs (`internal/daemon/daemon.go:2014-2050`). The default window
is three seconds (`internal/config/config.go:259`). A daemon that stops inside
it, which is exactly what a deploy does, loses the outcome. The item is still
pending, so it comes back after the restart, but the caller does not, so the
human answers a second time into a dead socket.
**Consequence.** The answer is lost and the human is asked again. Recoverable if
the agent is still running and re-asks, which it has no way to know it should.
**Where.** `internal/daemon/daemon.go:2014-2050`.
**Fix.** Two options, and the cheap one is enough: on shutdown, finalize any
graced answer immediately rather than dropping it (the human has already decided,
and the undo window is a courtesy). The heavier option is persisting the graced
outcome, which buys little because the caller is gone regardless.
**Test that would have caught it.** Answer into a grace, call the shutdown path,
reopen the daemon on the same store, and assert the answer was recorded.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-09. A result cannot say whether the question expired, was dismissed, or died with the daemon

**How it fails.** `proto.Result` carries `Answered`, `Answer`, `Reply`, `Values`,
`DefaultApplied`, `Vetoed`, `Secret`, `SecretPath` and `Approved`
(`internal/proto/types.go:206-219`). There is no field for how a question ended.
So `answered: false` is returned for a lapsed window, a human pressing Esc, a
cancel, a caller-gone auto-dismissal (`daemon.go:1336` returns a bare
`Result{ID}`) and a shutdown alike. Every other blocking family in the product
does carry the distinction: `await_signal`, `acquire_lock`,
`await_artifact_event` and `await_walkthrough` all return `timed_out`.
**Consequence.** An agent working unattended cannot act on the difference between
"he saw it and declined" and "nobody was there". The correct behaviour diverges
sharply between those two, and today the model has to guess. This is the field
that would let an agent retry safely.
**Where.** `internal/proto/types.go:206-219`, `internal/daemon/daemon.go:1307-1336`,
`internal/mcp/mcp.go:314-319`, `:339-341`.
**Fix.** Add an `outcome` string (`answered`, `expired`, `dismissed`, `cancelled`,
`caller_gone`, `shutting_down`), set it at the four return sites in
`handleSubmit` and in `resolve`'s delivery, and surface it in the MCP tool
results. The store already records the state, so this is plumbing rather than new
truth. The wrong fix is inferring it from `DefaultApplied`, which is empty
whenever the item had no default.
**Test that would have caught it.** One table test per ending asserting the
outcome string, over the same daemon harness that already tests each ending
separately.
**Size.** a day.
**Confidence.** Confirmed by reading the code.

### R-10. A keepalive that stops working is invisible, and it is the only thing holding the ask family up

**How it fails.** Claude Code abandons a tool call after 1800 s of silence, so
every blocking tool depends on the progress ticker in
`internal/mcp/keepalive.go:42-67`. The sync family is built not to need it: a
lock or signal park is capped at 25 minutes so it ends as an honest `timed_out`
under the transport's tolerance (`internal/daemon/locks.go:256-281`,
`internal/daemon/daemon.go:254-258`). The ask family has no such ceiling: with
`timeout_s: 0` no timer is created at all (`daemon.go:1290-1300`), and the same
is true of `await_artifact_event` (`artifacts.go:136-141`) and
`await_walkthrough` (`wthub.go:105-110`). So the ticker is load-bearing, and its
one failure signal is thrown away: `_ = ss.NotifyProgress(...)`
(`keepalive.go:84-91`) discards the error, keeps ticking, logs nothing and tells
the daemon nothing. There are two more silent disables: no `progressToken` on the
request means no ticker at all, and a session that is not a `*sdk.ServerSession`
means the same (`keepalive.go:54-58`).
**Consequence.** The wait silently reverts to a 30-minute fuse. The human answers
at minute 40, the daemon records the answer, and there is nobody on the other
end. Both sides believe they did their part.
**Where.** `internal/mcp/keepalive.go:54-58`, `:84-91`.
**Fix.** Treat a `NotifyProgress` failure as the client having gone: stop the
ticker, cancel the tool call's context so the daemon runs `callerGone`, and log
it. Then close the asymmetry by giving the ask family the same ceiling the sync
family has, returning the R-09 outcome `expired` rather than parking forever.
**Test that would have caught it.** A fake session whose `NotifyProgress` returns
an error, asserting the handler context is cancelled. `keepalive_test.go:128`
tests the no-token case as intended behaviour and never as a risk.
**Size.** a day.
**Confidence.** Confirmed by reading the code.

### R-11. One corrupt item row stops the daemon from starting at all

**How it fails.** `Store.query` fails the whole read if any single row has an
unreadable JSON blob in `options`, `fields`, `actions` or `form_values`
(`internal/store/store.go:383-406`). `Pending()` uses it, `daemon.New` returns
that error (`internal/daemon/daemon.go:639-642`), and `cmd/agentbox/daemon.go:383-386`
logs `daemon.init_failed` and exits. Every subsequent auto-spawn repeats it. The
stack blob has the right treatment already and says why: a pre-migration `""` is
skipped rather than read as corruption (`store.go:392-399`); the other four
columns do not get that care.
**Consequence.** Total outage from one bad row. Every agent gets "daemon did not
come up", nothing appears on screen, and the only evidence is one line in a log
file, because an auto-spawned daemon has its stderr sent to nowhere
(`internal/client/client.go:30`).
**Where.** `internal/store/store.go:383-406`, `internal/daemon/daemon.go:639-642`.
**Fix.** Skip and log a row that will not decode rather than failing the read, and
raise one daemon-authored warning card naming the count. A corrupt row is one lost
item; refusing to start is all of them.
**Test that would have caught it.** Write a row with `options = '{'` directly and
assert `Pending()` returns the other rows. `store_test.go` covers the schema and
the lifecycle and never a malformed blob.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-12. The drop-down panel reports itself open when it never mapped, and that is a routing input

**How it fails.** The panel window is created hidden
(`internal/webui/panel.go:278`) and the only `Show()` is inside `slide`, after two
early returns for `x == nil || win == nil` and `xid == 0`
(`panel.go:353-359`). The deferred `p.open = down` runs on both of those paths
(`panel.go:346-351`), so the state machine records an open panel that was never
mapped, and `PanelOpen()` keeps saying so. `PanelOpen()` is then one of the two
inputs to ask routing: `routeAsk` is called with `u.AppOpen() || u.PanelOpen()`
(`internal/webui/webui.go:402`), and a routed question gets no card at all
(`:402-405`). The gate narrows the blast radius: routing also needs the item to
carry a session the session surface is showing (`inline.go:92-103`), so this
reaches questions from AgentBox-spawned sessions and assignment runs rather than
from external agents.
**Consequence.** An assignment running unattended asks a question that appears on
no surface. The earcon still plays and the switcher row still gets its mark, so it
is discoverable by sound, but there is nothing on screen to answer.
**Where.** `internal/webui/panel.go:346-359`, `internal/webui/webui.go:402-405`.
**Fix.** Do not set `open` on a path that did not map, and log the reason, the way
`showCard` warns with `webui.card_unprepared` (`webui.go:518-523`). Every sibling
surface has that fallback and the panel is the one that does not.
**Test that would have caught it.** `slide` with a nil X handle, asserting
`PanelOpen()` stays false. `panel_test.go` covers only the easing arithmetic.
**Size.** hours.
**Confidence.** Confirmed by reading the code; the trigger needs the X11 dial to
have failed, which is R-19.

### R-13. The one message with no retry: the rider's news is consumed before it is sent

**How it fails.** FR83's discovery rider tells a session that a peer joined its
area, riding on the next successful reply. The cursor moves at compute time:
`riderFor` overwrites `row.knownPeers` (`internal/daemon/sync.go:592-612`) inside
`Serve`, immediately before `c.send(resp)` (`internal/proto/rpc.go:1036-1044`).
If that send fails, or the client's unmarshal of the result fails afterwards
(`rpc.go:941-943`), the line is gone for good, because each arrival is reported
exactly once and nothing persists it.
**Consequence.** A session never learns a peer joined its tree. That is the
warning the whole two-agents-in-one-checkout rule rests on, and it is the one
message in the product with neither a retry nor a store behind it.
**Where.** `internal/daemon/sync.go:592-612`, `internal/proto/rpc.go:1036-1044`.
**Fix.** Move the cursor only after a successful send, or keep the pending news on
the roster row until the next call acknowledges it. The rest of the product's
hand-offs are claim-once against the store for exactly this reason
(`internal/daemon/wthub.go:46-64`).
**Test that would have caught it.** A conn whose `send` fails, asserting the news
is still owed on the next call. `internal/mcp/sync.go` has no test file at all.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-14. A tool documented as non-blocking can park an agent forever

> **Fixed on 2026-08-09.** `nonBlockingCap` (10s) plus `s.fast` / `s.fastErr` and
> bounded twins of the three call helpers (`callFast`, `callIntoFast`,
> `syncCallFast`). Applied to every tool whose description promises it returns at
> once: notify_user, retract, announce, set_activity, list_agents, try_lock,
> release_lock, post_signal, shared, show_document, report_progress,
> release_control, and the whole walkthrough and assignment CRUD family.
>
> **It is opt-in, and that is the design decision worth keeping.** Marking a
> blocking tool fast by mistake would cap how long a human is allowed to think
> and would read to the agent as the daemon dropping their answer - much worse
> than the defect. Forgetting to mark a non-blocking tool leaves it exactly as it
> is today. So the safe direction is the one that needs an edit, and the four
> genuine waits (`ask` and its relatives, `await_signal`, `acquire_lock`,
> `await_artifact_event`, `await_walkthrough`, `request_control`) keep the
> caller's own context. `control()` branches on the action rather than the
> handler, because that file already explains which of the three waits.
>
> `fastErr` refuses to blame the daemon when it was the CALLER who gave up: a
> parent that is already done gets its own error back unchanged. The two are
> indistinguishable to a model otherwise, and only one of them is AgentBox's
> fault.
>
> `internal/mcp/deadline_test.go`: a daemon that accepts every connection and
> then says nothing, which is what a wedged one looks like from outside. Six
> tools asserted bounded, ask_user asserted NOT bounded, and the whole file
> checked by neutering `s.fast` to a plain `WithCancel` - after which it hangs to
> the 120s test timeout, which is the defect exactly. The existing MCP tests
> point at a runtime dir with no daemon at all, so every one of them exercised
> the dial deadline and none could ever have caught this.

**How it fails.** Only the dial is bounded, at five seconds
(`internal/client/client.go:39-64`). Once a connection exists, every tool passes
the raw tool context to the call and no deadline is applied: `notify_user`,
`show_document`, `report_progress`, `retract`, `announce` and `set_activity`
included (`internal/mcp/mcp.go:211-227`, `:609-620`, `:625-639`, `:832-846`,
`internal/mcp/sync.go:149-160`). A daemon that accepts connections but is wedged
on the UI thread (R-19, R-21) or on a store write parks the caller indefinitely
while the keepalive reports it as healthy. `notify_user`'s own description
promises it returns at once and never blocks.
**Consequence.** An agent's turn is spent waiting on a fire-and-forget
notification. The item may well have been created, so nothing is lost, but the
work stops and the reason is invisible.
**Where.** `internal/mcp/mcp.go:211-227`, `internal/mcp/sync.go:149-160`.
**Fix.** Give the non-blocking tools a call deadline of a few seconds and return a
readable failure. The blocking family must keep the caller's own context.
**Test that would have caught it.** A fake daemon that accepts and never answers,
asserting `notify_user` returns an error inside its budget. The MCP tests
deliberately point at a runtime dir with no daemon
(`internal/mcp/mcp_test.go:112`), so no test dials a daemon that behaves badly.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-15. An abandoned tool call keeps its card alive and swallows the answer

**How it fails.** The MCP SDK cancels the handler context on
`notifications/cancelled`, and that path is clean: context cancel, connection
close, `callerGone`, card marked disconnected. If a client abandons a request
without sending that notification, which is the open question about what
Claude Code does on `/clear`, the MCP child stays alive with its socket open, the
card keeps `CallerLive`, the human answers, the daemon records `StateAnswered`,
and the response is written to a request id nobody is tracking.
**Consequence.** The human spends a decision and it goes nowhere, with no sign on
either side: the card resolves normally and the log records an answered item.
**Where.** `internal/mcp/mcp.go:334-342`, `internal/mcp/keepalive.go:87`.
**Fix.** First find out: park an `ask_user`, `/clear` the session, and watch
whether the card flips to disconnected. If it does not, the keepalive's discarded
error is the available signal (R-10) and a failed notify should cancel the call.
**Test that would have caught it.** No unit test can settle it, because the
behaviour belongs to the host. It needs one live exercise, and it is the clearest
case in this document for an MCP-host integration check.
**Size.** hours to find out, a day to fix.
**Confidence.** Inferred, needs a repro.

---

## Band B

*The hub stops serving. Pending items survive in the store, but every parked
caller is dropped and nothing reaches the human until somebody notices.*

### R-16. One `show_document` call at a non-regular path reads until the OOM killer arrives

**How it fails.** `viewer.load` does a bare `os.ReadFile(req.Path)` with no stat
first, no size check and no `Mode().IsRegular()` check
(`internal/webui/viewer.go:95`); the stat happens afterwards, only for the
modification time. The watch loop repeats the same read (`:173`). `show_document`
sends only the path and never stats it (`internal/mcp/mcp.go:470-482`). So
`show_document{path: "/dev/zero"}` grows a buffer until the kernel kills the
daemon, and a fifo blocks the goroutine forever instead. The guard for exactly
this exists a hundred lines away in a sibling file, with the comment "Reading
/dev/zero would not end" (`internal/webui/images.go:218-223`).
**Consequence.** The hub dies. Every parked agent gets a transport error, the
in-memory undo grace and flood state are lost, and systemd brings the daemon back
two seconds later with no card explaining what happened. One ordinary tool call,
available to any agent, no human involved.
**Where.** `internal/webui/viewer.go:95`, `:173`.
**Fix.** Stat first: regular files only, under a cap, with the same distinguished
reasons `images.go` already returns so the viewer can say why. Apply it to the
board's file reader too (R-30).
**Test that would have caught it.** `show_document` against a fifo and against a
device node, asserting a refusal rather than a read. `images_test.go:253` has the
directory case for the sibling path.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-17. A code fence in a card body costs seconds of CPU and tens of megabytes of HTML

**How it fails.** Past ten lines, `codeBlockHTML` switches to chroma's
line-numbered formatter (`internal/webui/mdhtml.go:232-261`), which emits a table
cell and a span per line. Measured against this repo's option set: 1.1 MB of Go
in one fence tokenises in 4 ms and formats in 2.4 s, producing 17.75 MB of HTML,
a 16x amplification. A fence with no language adds `lexers.Analyse`
(`mdhtml.go:237`), a further 777 ms for the same input. With no cap on body size
(R-04), a 4 MB unlabelled fence is roughly twelve seconds of CPU and about 60 MB
of HTML, marshalled and copied to the GTK main thread once per open window. The
worst multiplier is the session transcript, which re-renders the whole selected
conversation every 60 ms with no cache (`internal/webui/sessions.go:602-625`,
`:690-740`).
**Consequence.** Not a lost item: `Present` runs outside the daemon lock on
purpose, so other agents keep being served. What the user loses is the machine,
for as long as it takes, and a window that may not survive the payload.
**Where.** `internal/webui/mdhtml.go:232-261`, `internal/webui/sessions.go:602-625`.
**Fix.** Bound the input (R-04) and stop the amplification: fall back to plain
escaped text past a line count or byte size, which is what the 400-line diff cap
already does for the other big renderer. Cache the rendered transcript per turn so
streaming re-renders only the tail.
**Test that would have caught it.** A benchmark with a byte ceiling on the output:
assert that rendering a 1 MB fence produces under some multiple of its input and
returns inside a deadline. No test asserts a ceiling on any render.
**Size.** a day.
**Confidence.** Confirmed by measurement.

### R-18. A 439 kB PNG decodes to 1.5 GB inside the webview

**How it fails.** Images are gated on magic number and on a 2 MB byte ceiling
(`internal/webui/images.go:65`, `:224`, `:236`) and nothing anywhere calls
`image.DecodeConfig`, so pixel dimensions are never read. A 20000 by 20000
grayscale PNG is 439,201 bytes on disk, sniffs as `image/png`, becomes a 586 kB
data URI, and decodes to about 1.49 GB of RGBA. 20000 is under cairo's per-axis
limit, so the decode is attempted. One `![](/tmp/bomb.png)` in any card body.
**Consequence.** The WebProcess for that surface is killed, so the card does not
appear while the daemon records it displayed (R-06). The daemon itself survives.
**Where.** `internal/webui/images.go:60-90`, `:218-240`.
**Fix.** `image.DecodeConfig` on the sniffed bytes and refuse past a pixel budget,
returning the existing `too-big` reason so the placeholder path already handles
it.
**Test that would have caught it.** Generate a large-dimension small-byte PNG in
the test and assert the refusal. `images_test.go` covers formats, magic numbers
and the byte ceiling, and never a decoded size.
**Size.** hours.
**Confidence.** Confirmed by measurement.

### R-19. Nothing drains the webui's X connection, so accumulated errors can wedge the GTK thread permanently

**How it fails.** `dialX11` creates the connection and nothing in
`internal/webui` ever calls `WaitForEvent` or `PollForEvent`
(`internal/webui/x11.go:97-115`). Every unchecked request in that file pushes any
`BadWindow` or `BadValue` into xgb's event channel, which is buffered at 5000
(`xgb.go:43`) and blocks on send when full, at which point `readResponses` stops.
The file then flushes by round-tripping `GetInputFocus(...).Reply()` in a dozen
places (`x11.go:231`, `:265`, `:313`, `:342`, `:365`, `:428`, `:439`, `:485`,
`:500`, `:512`, `:523`, `:535`), and those run inside `onMain` closures, on the
GTK main thread. A full buffer turns the next flush into a permanent block.
**Consequence.** The UI freezes with the daemon still answering RPC: no further
window appears, every pending item is stranded on screen, `Present` blocks because
`invoke` is `application.InvokeSync` (`internal/webui/webui.go:218`), and so every
agent calling `notify_user` parks too (R-14). The store stays correct and the
human sees a frozen desktop hub.
**Where.** `internal/webui/x11.go:97-115` and the twelve flush sites.
**Fix.** Drain the connection in a goroutine that logs and discards X errors, and
give the flush a checked cookie so a failure is visible. The wrong fix is
enlarging the buffer.
**Test that would have caught it.** None in this design: it needs a real X server.
What replaces it is a liveness signal, a UI-thread heartbeat the daemon can log
and `agentbox status` can report (R-40), so a frozen hub is diagnosable rather
than mysterious.
**Size.** a day.
**Confidence.** Mechanism confirmed by reading; whether 5000 errors accumulate in
a real session is inferred.

### R-20. `SetSize` and `Hide` on a destroyed window are a use-after-free that segfaults the daemon

**How it fails.** Wails guards `NativeWindow`, `Size`, `Focus`, `Maximise` and
`Minimise` with an `isDestroyed()` check and does not guard `SetSize`
(`webview_window.go:412-452`) or `Hide` (`:498-504`), while
`linuxWebviewWindow.close()` calls `gtk_window_destroy` and leaves the pointer
dangling without nilling `impl`. AgentBox captures a window and then dispatches to
the main thread in several places, so any close in between is a call on freed
memory: `Bridge.Fit` reads `u.prompt` and calls `w.SetSize` inside a later
`InvokeSync` (`internal/webui/bridge.go:84`, `:99-106`), which races a human
answering the card it is resizing; the panel calls `win.SetSize` on every roll
frame from a window captured once (`internal/webui/panel.go:341`, `:371`, `:386`);
progress does the same shape (`progress.go:179-191`).
**Consequence.** The process dies, and no recover can catch a cgo segfault. Pending
items come back on restart; parked callers do not.
**Where.** `internal/webui/bridge.go:99-106`, `internal/webui/panel.go:371`,
`:386`, `internal/webui/progress.go:179-191`.
**Fix.** Re-read the window under the lock inside the dispatched closure and skip
if it is gone, which is the fix already applied to the reuse-or-replace race for
the same reason (`webui.go:451-460`). A local `isDestroyed` helper over
`NativeWindow() == nil` gives the same guard Wails omits.
**Test that would have caught it.** A unit test of the read-inside-the-closure
ordering with a fake dispatcher, which `loop_test.go:136-192` already does for the
card path and nobody did for `Fit`, the panel or progress.
**Size.** a day.
**Confidence.** Confirmed by reading the Wails source alongside the call sites.

### R-21. Panic-capable webui goroutines with no recover, each of them fatal to every parked agent

**How it fails.** The daemon wraps its own timers in `safely` and explains why: an
unrecovered panic on a goroutine takes the process down and with it every parked
caller (`internal/daemon/daemon.go:704-728`). `internal/webui` has only two
recovers, at `viewer.go:151` and `bridge.go:328`. Uncovered goroutines that run
agent-authored content through goldmark and chroma: the session transcript push
(`sessions.go:612-625`), the inbox push (`inbox.go:638-650`), and four
`go u.rerouteAsk()` sites that reach `Present` and a markdown render
(`app.go:79`, `:161`, `sessions.go:539`, `panel.go:301`). The hotkey reader
launches its action as `go fn()` outside its own recover
(`internal/hotkey/hotkey.go:237` against the recover at `:214-218`), so a panic in
the pause handler kills the daemon.
**Consequence.** One panic in a renderer on pathological input drops every parked
agent and every unread card. systemd restarts the daemon, and the pending items
come back, but the answers in flight do not.
**Where.** `internal/webui/sessions.go:612-625`, `internal/webui/inbox.go:638-650`,
`internal/hotkey/hotkey.go:237`.
**Fix.** The daemon already has the right shape. Export a `safely` equivalent for
the webui and wrap each of these goroutines, so a bad render costs one surface
rather than the process.
**Test that would have caught it.** A renderer stub that panics, asserting the
daemon survives, which is exactly what
`TestAPanicOnATimersGoroutineDoesNotTakeTheDaemonDown`
(`daemon_test.go:378`) does for the daemon's own timers and nobody did for the UI.
**Size.** hours.
**Confidence.** Paths confirmed by reading; that goldmark or chroma panics on some
input is inferred.

### R-22. An action button runs a command with no timeout and buffers its whole output

**How it fails.** `execAction` runs `sh -c` with `CombinedOutput()` and no
context and no deadline (`internal/daemon/daemon.go:2159-2181`). A command that
never exits parks its goroutine and its process for the daemon's lifetime with no
failure card, and one that writes without bound is buffered entirely in the
daemon's memory. Nothing kills it at shutdown. The action string is agent-authored
but needs a human click, which is a documented decision and not the defect here.
**Consequence.** A wedged or noisy action leaks a process, and a loud one can take
the daemon out by memory, dropping every parked agent.
**Where.** `internal/daemon/daemon.go:2159-2181`.
**Fix.** `exec.CommandContext` with a timeout, `Setpgid` so the group can be
killed, and a bounded output reader. The failure card already exists for the
non-zero case and should be raised on the timeout too.
**Test that would have caught it.** An action of `sleep 60` asserting the failure
card arrives inside the timeout, alongside the four `RunAction` tests that already
exist (`daemon_test.go:1311-1360`).
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-23. Rollback across a migration leaves a daemon that cannot start

**How it fails.** The store refuses to open a database written by a newer binary,
deliberately and correctly (`internal/store/store.go:161-164`, ADR-0005). `make
rollback` installs the previous binary and restarts (`Makefile:305-308`). If the
build being rolled back added a migration, the older binary now meets a schema it
refuses, exits, and `make deployed` reports that no daemon answered. There is no
downgrade path and no warning before the rollback.
**Consequence.** The recovery command produces a total outage, at the moment
somebody is already recovering from something.
**Where.** `internal/store/store.go:161-164`, `Makefile:305-308`.
**Fix.** Have `make rollback` compare the store's schema version against what the
previous binary knows and refuse with the reason, naming the manual step
(restore a store backup, or roll forward instead). Cheap, and it turns a mystery
into a sentence.
**Test that would have caught it.** A store at version N+1 opened by a binary that
knows N, asserting the error, which `store_test.go:179` already covers at the
store level. What is missing is the check in the deploy path.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-24. A crash loop ends with systemd giving up and nothing on screen saying so

**How it fails.** The unit is `Restart=on-failure`, `RestartSec=2`, with no
start-limit override (`packaging/agentbox.service:29-30`), so the defaults
`StartLimitBurst=5` and `StartLimitIntervalSec=10s` apply, verified live. Five
failures two seconds apart land on that limit, the unit enters `failed`, and
systemd stops trying. The auto-spawn partly rescues it, because the next MCP call
spawns an unmanaged daemon, which then leaves the unit inactive while a daemon
serves. Meanwhile the fatal path writes to stderr, which goes to the journal and
not to the JSONL, so the event log shows five `daemon.start` lines with no
`daemon.stop` and no reason (live counts already show 266 stops against 256
starts, itself unexplained).
**Consequence.** The desktop can end up with no hub and no indication, or with a
daemon systemd does not manage. Pending items survive either way.
**Where.** `packaging/agentbox.service:29-30`, `internal/logging/logging.go:65-82`.
**Fix.** Widen the interval so a real crash loop is visible without being fatal,
and write the fatal path to the JSONL before exiting so the reason is in the file
an engineer will read. Report the unit state in `agentbox status`.
**Test that would have caught it.** Not a unit test. The check is a documented
recovery drill: kill the daemon five times in ten seconds and confirm what the
board, the log and `agentbox status` say afterwards.
**Size.** hours.
**Confidence.** Confirmed by reading, with the systemd defaults verified live.

### R-25. Writing to a session child blocks the UI goroutine

**How it fails.** `Send` writes to the child's stdin with no deadline and no size
check, and its comment says it is safe to call from the UI goroutine
(`internal/session/driver.go:249-272`). A child that has stopped reading stdin, or
a prompt larger than the 64 kB pipe buffer, blocks that write and therefore the
frame goroutine. Nearby, `addUserPrompt` runs before the write
(`driver.go:265-271`), so a prompt that never reached the child is recorded in the
transcript as though it had.
**Consequence.** The app window freezes while an agent is mid-answer, and the
conversation shows a turn that was never sent.
**Where.** `internal/session/driver.go:249-272`.
**Fix.** Write from a goroutine with a deadline, report the failure into the
conversation, and record the user turn only once the write succeeded.
**Test that would have caught it.** A child that never reads stdin, asserting
`Send` returns an error rather than blocking. `driver_test.go` covers argument
building, spawn and read, and none of the lifecycle.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-26. No bound on anything an agent puts in a card, in three shapes that each break a surface

**How it fails.** Beyond R-04's wire cap there is no content bound in the item
path. Three concrete shapes, all under 4 MB so all accepted. A diff of 200,000
`diff --git` headers is about 5 MB: `DIFF_CAP` bounds rendered lines and
deliberately not structure (`frontend/src/lib/diff.js:19`, `:134-139`), so the
card builds 200,000 rail buttons and 200,000 sections
(`frontend/src/surfaces/Card.svelte:457`, `:470`) while reporting nothing wrong. A
chart with 500,000 points is about 3.5 MB of JSON and renders one circle with a
nested title per value (`internal/webui/chart.go:218-249`), roughly 55 MB of SVG;
x labels are thinned and the pie legend is bounded, which makes the missing point
cap look deliberate and almost certainly is not. A 4 MB markdown table is about
170,000 rows, all of which goldmark renders.
**Consequence.** The surface is unusable or its WebProcess dies, and the item is
recorded as displayed (R-06). Form fields, choice options and action buttons are
capped and tested; these three are not.
**Where.** `internal/webui/chart.go:58-139`, `frontend/src/lib/diff.js:19`,
`internal/proto/types.go:251-352`.
**Fix.** Cap the point count, the file count and the row count where the other
three caps live, in `Validate` and in the renderers, with a visible "truncated at
N" rather than silence.
**Test that would have caught it.** The caps that exist all have tests, which is
why the absent ones are invisible. One table test per bound.
**Size.** a day.
**Confidence.** Confirmed by reading, with the sizes from arithmetic.

---

## Band C

*The desktop layer fails while the daemon records a delivery. The item is safe;
what breaks is the human ever seeing it, or the controls he needs to take the
machine back.*

### R-27. One failed atom intern silently disables every placement and hint in the process

**How it fails.** `dialX11` interns fourteen atoms and returns nil if any of them
fails, closing the connection (`internal/webui/x11.go:106-112`). With `u.x == nil`
the whole prepare-and-settle block in `showCard` is skipped
(`internal/webui/webui.go:488-523`) and the bare `win.Show()` at `:524` runs
*without* the `webui.card_unprepared` warning, because that warning is inside the
skipped block. Cards then map with no above-hint, no skip-taskbar and no centring,
which the code itself names as the shape of the session-25 ghost, and there is not
one log line.
**Consequence.** Cards appear behind other windows, in the taskbar, wherever the
compositor likes. Nothing is lost, and nothing is diagnosable either. It also
enables R-12.
**Where.** `internal/webui/x11.go:106-112`, `internal/webui/webui.go:488-524`.
**Fix.** Log which atom failed, at warn, and raise the degraded state where
`agentbox status` can report it. Interning is not all-or-nothing: an atom used by
one feature should disable that feature.
**Test that would have caught it.** Not reachable without an X server. The
substitute is the diagnostic: a startup line naming the display state, which is
the same gap as R-40.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-28. All three hotkey grabs die silently and permanently on an X read error

**How it fails.** The reader blocks on `WaitForEvent`
(`internal/hotkey/hotkey.go:220`); on a connection read error xgb returns
`(nil, nil)` and the `case nil: return` at `:241-243` exits the goroutine with no
log line and no reconnect. All three grabs are opened once at startup and never
re-opened (`cmd/agentbox/daemon.go:442-481`). Related: a failed `Rebind` returns
after having already grabbed some of the `{0, caps, num, caps|num}` cross-product
and does not ungrab them (`hotkey.go:179-196` against the cleanup `Open` does at
`:143`), so the combination stays claimed desktop-wide while `h.codes` holds the
old codes, which means it is swallowed for every application and triggers nothing.
`Rebind` runs on every config reload (`cmd/agentbox/daemon.go:557-559`).
**Consequence.** The pause key is the human's way to take his desktop back while
an agent is typing. Losing it silently is the worst of the three, and the failure
is indistinguishable from a key that was never bound.
**Where.** `internal/hotkey/hotkey.go:220`, `:241-243`, `:179-196`, `:280-289`.
**Fix.** Log the reader's exit at warn and reconnect with backoff. Ungrab the
partial cross-product on a failed rebind. The hotkey layer already translates
`BadAccess` into a sentence about another application holding the key
(`hotkey.go:188`), which is the standard the rest of it should meet.
**Test that would have caught it.** The reader exit path can be tested with a
fake connection that returns a read error, asserting a log line and a retry.
`hotkey_test.go` covers `Parse` and the modifier-less refusal and never reaches X.
**Size.** a day.
**Confidence.** Confirmed by reading the code.

### R-29. Nothing reacts to a monitor being unplugged or the resolution changing

**How it fails.** `randr.SelectInput` and `ScreenChangeNotify` appear nowhere in
the tree. The active monitor is re-resolved per placement, which is deliberate and
right (`internal/webui/x11.go:166-174`), but an already-mapped window is never
re-placed, and the root rectangle is captured once at dial time
(`x11.go:103-104`), which is the fallback `pickMon` uses when RandR is absent
(`monitor.go:62`). Separately, `relayout` re-reads the active monitor on every
`put` and `drop` (`topstack.go:120`), so a toast resize drags the hands-off strip
onto whichever monitor the pointer is on, mid-run, and a surface taller than half
the monitor gets a negative y with no clamp (`topstack.go:125-127`).
**Consequence.** A pending question centred on a screen that is gone, and a
hands-off strip that moves during a driven run, which is the one window whose
position is a safety claim.
**Where.** `internal/webui/x11.go:103-104`, `internal/webui/topstack.go:120-127`.
**Fix.** Subscribe to `ScreenChangeNotify`, refresh the root rectangle, and
re-place mapped windows. Pin the strip's monitor for the length of a handover
rather than re-asking on every unrelated layout.
**Test that would have caught it.** The geometry is already extracted into pure
functions over rectangles, which is how `monitor_test.go` tests placement without a
display. `topstack.go` has no test at all, and the negative-y clamp is exactly the
kind of arithmetic that harness catches.
**Size.** a day for the RandR subscription, hours for the two clamps.
**Confidence.** Confirmed by absence in the tree.

### R-30. The review board's file jail is lexical, so a symlink reads any file into a review

**How it fails.** `fileCache.lines` joins the requested path onto the repo root
and jails it by lexical prefix with no `EvalSymlinks`, then reads it with no size
cap and no regular-file check (`internal/webui/boardrender.go:451-468`). A symlink
inside the repo root pointing at `~/.ssh/id_ed25519` passes the prefix test, and
its contents are highlighted into the board's wire model. A symlink to `/dev/zero`
is R-16 again.
**Consequence.** A file outside the review's subject is rendered into a surface the
human reads as "the code this walkthrough cites". The walkthrough spec is
agent-authored, so the path is chosen by the agent.
**Where.** `internal/webui/boardrender.go:451-468`.
**Fix.** `filepath.EvalSymlinks` both sides before the prefix test, plus the stat
guard from R-16. Images need no jail because they have nothing to escape from; the
board does.
**Test that would have caught it.** A symlink out of a temp repo root, asserting a
refusal. `boardrender_test.go:128` tests honest read errors and not escapes.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-31. Two agents can drive the physical desktop at the same time

**How it fails.** `MethodDrive` takes no lock, and the pause gate reports only the
human's pause (`internal/daemon/daemon.go:888-928`, `:152-155`). Each call opens
its own connection (`cmd/agentbox/daemon.go:284`), so two concurrent
`drive_desktop` calls interleave motion, clicks and keystrokes on one pointer and
one keyboard, and run the per-stroke XKB group lock against each other.
`request_control` exists and is the documented etiquette, but nothing enforces it
at the driving verb.
**Consequence.** Keystrokes land in the wrong window. The target lock in
`internal/hand/target.go` is the strongest code in this audit and it protects one
script against a moving desktop, not against a second script.
**Where.** `internal/daemon/daemon.go:888-928`.
**Fix.** Serialise `drive` on the handover: refuse when another session holds it,
and take it implicitly for the length of a script when nobody does. The mechanism
exists.
**Test that would have caught it.** Two concurrent `MethodDrive` calls with a fake
driver that records interleaving, asserting the second is refused or queued.
**Size.** a day.
**Confidence.** Confirmed by reading the code.

### R-32. Panel geometry and interaction edges

**How it fails.** Three small ones in one place. The half-the-monitor cap is
applied before the minimum-height floor, so the floor wins and on a 600 px screen
the panel takes 60 % of it, contradicting the comment above the code
(`internal/webui/panel.go:210-223`). The window's `MinWidth` is the constant
rather than the clamped width (`panel.go:283` against `:218-220`), so on a narrow
monitor the compositor keeps the panel wider than the screen. `Hide` has no
`animating` guard and reads `p.open`, which `slide` only sets at the end, so Esc
during a roll-down is swallowed (`panel.go:164-183` against `:346-351`); `Toggle`
guards this and `Hide` does not. `Hide` also reads `p.w` outside the mutex
(`panel.go:175`).
**Consequence.** A window that looks wrong and a keypress that does nothing. No
item is at risk.
**Where.** `internal/webui/panel.go:210-223`, `:283`, `:164-183`.
**Fix.** Clamp after flooring, pass the computed width as `MinWidth`, give `Hide`
the same `animating` guard `Toggle` has, and read under the lock.
**Test that would have caught it.** `sizeOn` on a 600 px monitor, which the
existing arithmetic tests (`loop_test.go:194-215`, `monitor_test.go:109`) cover only
where the cap wins.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

---

## Band D

*Agent-authored content shapes a surface it should only fill. Injection is
genuinely closed; what is open is markup that wears AgentBox's own chrome, and
resource use with no ceiling.*

### R-33. Heading attributes let agent text wear system chrome and cover the buttons

**How it fails.** `parser.WithAttribute()` is enabled
(`internal/webui/mdhtml.go:89`). In goldmark 1.8.2 that affects headings only, and
its global attribute filter admits `class`, `id`, `style`, `title`, `role`,
`tabindex` and any `data-*`. Values are escaped, so there is no attribute
break-out, but the attributes themselves are honoured. Verified by running it:

```
# hello {style="position:fixed;inset:0;background:#000;z-index:99999" class="k-alert" data-tone="error"}
```

renders as an `h1` carrying all three. One heading in any notify body, ask body,
agent turn or document buys a full-viewport overlay inside the card, panel, viewer
or conversation, swallowing pointer events over Approve and Reject, and lets agent
text wear AgentBox's own alert classes so it reads as system chrome. It does not
buy event handlers, and it cannot spawn an artifact.
**Consequence.** The human is shown something that looks like AgentBox saying it,
over the controls he would use to disagree. That is a trust boundary rather than a
lost item, and it is the only markup hole found.
**Where.** `internal/webui/mdhtml.go:89`.
**Fix.** Drop `WithAttribute`, or filter to `id` alone if heading anchors are
wanted. The policy sweep should gain a heading-attribute case either way.
**Test that would have caught it.** `hostileMarkdown` in
`internal/webui/policy_test.go:109-150` is the right harness and has no
heading-attribute case; its `style=` audit looks only for `url()`, so a fixed
overlay passes today.
**Size.** hours.
**Confidence.** Confirmed by running goldmark with this repo's option set.

### R-34. Middle-clicking a rendered link bypasses the scheme allowlist

**How it fails.** Dangerous schemes are closed twice: goldmark blanks
`javascript:` and `vbscript:` hrefs, and `openURL` allowlists http, https and
mailto before handing anything to `xdg-open`
(`internal/webui/bridge.go:314-330`). But `data:image/svg+xml` survives goldmark
deliberately, and the surface's interceptor listens for `click` only
(`frontend/src/lib/markdown.svelte.js:255-274`). A middle click fires `auxclick`,
which is not intercepted, and there is no Wails navigation policy hook anywhere in
the tree. The webview's default action for that link is then unguarded, which is
the exact failure the interceptor's own header says it exists to prevent.
**Consequence.** A surface navigates somewhere AgentBox did not sanction. Bounded
by the CSP on that surface, so this is a hole in a fence rather than an open door.
**Where.** `frontend/src/lib/markdown.svelte.js:255-274`.
**Fix.** Intercept `auxclick` alongside `click`, and add a Wails navigation policy
as the backstop so the guarantee does not rest on enumerating event names.
**Test that would have caught it.** No test covers hostile URL schemes at all;
`policy_test.go:243` tests one well-formed https link and exempts links from the
fetch audit. A middle click needs the live exercise.
**Size.** hours.
**Confidence.** Reading confirmed the missing handler; the resulting navigation is
inferred and needs one live middle-click.

### R-35. An artifact has no watchdog of any kind

**How it fails.** The sandbox is tight and closes every escape that was checked:
`allow-scripts` only, opaque origin, `connect-src 'none'`, no modals, no popups, no
top navigation, `allow=""` for permissions
(`frontend/src/lib/artifact-runtime.js:31-45`,
`frontend/src/lib/artifact.svelte.js:324-326`). What is absent is any bound on
resource use. `while(true){}` or an allocation loop freezes the surface the frame
shares a WebProcess with, which is the viewer window or the app window when the
artifact is a fence in a conversation. Nothing rate-limits `postMessage`, and
every `height` message forces a layout (`artifact.svelte.js:275-286`). Nothing
rate-limits `emit`, and each accepted event writes an Info log line
(`internal/daemon/artifacts.go:93`). A parameter panel's emits become one SQL
UPDATE each with no throttle and no total size cap on the merged object
(`internal/store/assignment.go:202-209`).
**Consequence.** One window stops responding, or the log fills, or the assignment
row grows 16 kB at a time. The daemon's main thread is not blocked, because
`evaluate_javascript` is asynchronous.
**Where.** `frontend/src/lib/artifact.svelte.js:156-190`, `:275-286`,
`internal/daemon/artifacts.go:93`.
**Fix.** Rate-limit `emit` and `height` per artifact, log the flood once rather
than per event, and cap the merged params object. A CPU watchdog is harder and
worth less: the surface is recoverable by closing the window.
**Test that would have caught it.** `tools/artifact-probe.html` probes every escape
and nothing about exhaustion, and `frontend/policy_test.go:26-30` says outright
that it refuses to execute any of it. The missing thing is a JS runner: the same
one R-38 asks for would let the probe assert a spin is contained and a flood is
rate-limited.
**Size.** a day.
**Confidence.** Confirmed by reading; the shared-WebProcess detail is inferred from
WebKitGTK's process model.

### R-36. An artifact too big to run is fully highlighted first

**How it fails.** `artifactMaxBytes` is 96 kB and an artifact over it is marked
`TooBig` so nothing runs (`internal/webui/artifact.go:30`, `:60`, `:204-209`). But
`codeBlockHTML(src, ...)` runs unconditionally before that branch (`:203`, and the
same in `RenderPanel` at `:228`), so a 4 MB artifact source is chroma-highlighted
into the payload anyway, at R-17's cost.
**Consequence.** The refusal costs more than the acceptance would have.
**Where.** `internal/webui/artifact.go:203`, `:228`.
**Fix.** Check the size before highlighting.
**Test that would have caught it.** `artifact_test.go:158` asserts the refusal;
what is missing is an assertion about the size of what the refusal produces.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

---

## Band E

*The coordination primitives mislead instead of blocking. The orphan rule
(a dead holder's lock does not free itself) is correct and deliberate and is not
in this band.*

### R-37. A lock recorded against a reparented MCP child can never age out

**How it fails.** The orphan rule frees a hold when the recorded pid is gone,
which is the right design. The pid comes from `os.Getppid()` read fresh at each
call site (`internal/mcp/sync.go:85`, `:329`, `:344`, `:562`). If the agent process
dies while its MCP child lives, the child is reparented and reports the reaper,
which is pid 1. A hold recorded against pid 1 can never look orphaned, because pid
1 never dies, so it stays "held by live work" until a human breaks it from the
board.
**Consequence.** A permanently held lock on the deploy, the repo or the VM,
blocking every other agent, with a refusal that names a holder who is gone. Every
waiter queues behind it correctly and forever.
**Where.** `internal/mcp/sync.go:85`, `:329`, `:344`, `:562`,
`internal/daemon/locks.go` (the orphan probe).
**Fix.** Capture the agent pid once, at MCP startup, alongside the session key,
and refuse to record pid 1 as an owner. A hold whose owner is pid 1 should be
treated as having no probeable owner, which the code already handles: pid 0 frees
after the grace.
**Test that would have caught it.** A hold recorded with pid 1, asserting it
orphans after the grace rather than never. `locks_test.go:389` already backdates
holds for the orphan tests.
**Size.** hours.
**Confidence.** Confirmed by reading the code.

### R-38. A failed session-key derivation splits one session into two identities, silently

**How it fails.** The key comes from `AGENTBOX_SESSION_KEY`, then
`AGENTBOX_SESSION_ID`, then a walk up `/proc` to the nearest ancestor whose `comm`
is not a placeholder, keyed as `proc-<pid>-<starttime>`
(`cmd/agentbox/main.go:227-238`, `:318-353`). If the walk finds nothing, which
happens with a setsid-cut tree, inside a container, or when every ancestor up to
pid 1 is a placeholder, `sessionKey()` mints a random key while
`inheritedSessionKey()` returns empty (`main.go:231-237` against `:263-268`). The
child then holds locks and shared claims under a key no hook can name, hooks that
act on the session are refused, and the board can show the session twice. Nothing
warns. The mirror-image failure is a collision: the placeholder list skips shells,
terminals, `setsid`, `make` and `tmux`, so if the host's `comm` is ever one of
those the walk lands on a shared ancestor and two sessions derive one key. That is
not cosmetic, because `retract` with no id sweeps `Mine: true`
(`internal/mcp/mcp.go:294`) and mine is `it.Identity.Key == key`
(`internal/daemon/daemon.go:1903`), so one session would retract the other's
pending questions.
**Consequence.** In the split case, coordination silently stops working for that
session. In the collision case, one agent destroys another's questions, which is
Band A damage reached through this door.
**Where.** `cmd/agentbox/main.go:231-237`, `:263-268`, `:335-353`,
`internal/proto/types.go:122-134`.
**Fix.** Make a failed derivation loud: log it, and mark the roster row so the
board can say the session cannot be coordinated with. For the collision, include
the MCP child's own pid in the key when the walk lands on a placeholder ancestor,
so two siblings cannot collide.
**Test that would have caught it.** The walk has a seam (`agentProcessFrom(from)`)
and no test. A table over synthetic `/proc` trees: a normal chain, an all-placeholder
chain, a chain whose parent has exited, and two siblings under one shell.
**Size.** a day.
**Confidence.** Confirmed by reading the code; whether the collision is reachable
depends on the host's `comm`, which is why the fix is cheap insurance rather than
urgent.

### R-39. Session children are not in a process group, and `Stop` never escalates

**How it fails.** The spawned `claude` gets only `Dir` and `Env`, with no
`SysProcAttr` and so no `Setpgid` (`internal/session/driver.go:166-168`). `Kill`
therefore signals that one process, and its own MCP child and tool processes
survive as orphans; closing a session tab is exactly this path
(`internal/webui/sessions.go:535`). `Stop` closes stdin and returns with no
escalation and no deadline (`driver.go:274-300`), and the assignment path uses
`Stop` (`internal/webui/assignments.go:145`), which is the path whose own comment
worries about thirty idle `claude` processes by the end of the month. Two more
processes are launched and never waited on: the review board's editor
(`internal/webui/board.go:403`) and `xdg-open` (`internal/webui/bridge.go:332`),
one zombie each per use, and the auto-spawned daemon is released but not reaped
(`internal/client/client.go:34`).
**Consequence.** Leaked processes that hold locks, CPU and tokens, and a machine
that gets slower over a long day of unattended runs. Under systemd the daemon's
own cgroup catches the children on shutdown; a daemon killed outside systemd does
not.
**Where.** `internal/session/driver.go:166-168`, `:274-300`,
`internal/webui/board.go:403`, `internal/webui/bridge.go:332`.
**Fix.** `Setpgid` on the child and signal the group; give `Stop` a deadline that
escalates to `Kill`; reap the fire-and-forget launches in a goroutine, which is the
shape `internal/sound/sound.go:174-190` already uses.
**Test that would have caught it.** A stub child that spawns a grandchild and
ignores stdin, asserting both are gone after `Stop`. Nothing covers `Kill` or
`Stop` semantics today.
**Size.** a day.
**Confidence.** Confirmed by reading the code.

---

## Band F

*We would not find out. Everything above is discoverable in principle; this band
is about why none of it was discovered, and what the log leaves an engineer
guessing on a machine nobody can reach.*

### R-40. No test renders any surface, so every display defect in this document was found by reading

> **Started** on 2026-08-07, not finished. The harness this entry asks for exists:
> vitest and jsdom, `frontend/vitest.config.js`, and `frontend/test/card.test.js`
> mounting `Card.svelte` and driving it from the keyboard. That is fix (1) of the
> three below. Fixes (2), the hostile payload, and (3), executing `buildDocument`,
> are still open, and sixteen of the thirty-two Svelte files still have no test that
> so much as imports them. The count in the paragraph below is therefore no longer
> literally true and the rest of the reasoning is.

**How it fails.** 32 Svelte files and 13,158 lines are never executed by anything:
no vite, no jsdom, no svelte compilation in any test. `frontend/package.json` has
no test script; the one JS test is `frontend/src/lib/diff.test.js` over the pure
`parseDiff`, run by node's own `--test`. The Go side tests the HTML it *produces*
thoroughly, with substring and regex assertions and no golden files, and
`frontend/policy_test.go` audits the shipped bundle as text. `buildDocument`, the
artifact sandbox assembler, is never executed, and its own header says so.
Consequences visible in this document: R-05, R-06, R-12, R-26 and R-35 all live in
the gap.
**Consequence.** Any change to a surface is verified by looking at it once, by
hand, on one machine. STATUS records this as a toolchain decision waiting to be
made rather than an oversight, which is correct, and the decision has been waiting
while thirteen milestones of surface were built on top of it.
**Where.** `frontend/package.json`, `frontend/policy_test.go:26-30`,
`Makefile:169`.
**Fix, concretely.** vitest plus jsdom, and three tests first, in this order.
(1) Mount `Card.svelte` with a view payload and assert the question, its options
and their keys are in the DOM: that is the test that turns R-05 from invisible into
red. (2) Mount it with a hostile payload (the heading overlay from R-33, a
200,000-file diff from R-26) and assert nothing escapes its container and the
render returns inside a deadline. (3) Execute `buildDocument` and assert the CSP
and sandbox attributes on the assembled document, which today are checked only as
string constants.
**Test that would have caught it.** This is the entry.
**Size.** a week for the harness plus the first three tests.
**Confidence.** Confirmed by grepping the whole test tree.

### R-41. No test carries a real request from a tool call to a store row

**How it fails.** The MCP tests deliberately point at a runtime directory with no
daemon (`internal/mcp/mcp_test.go:112`), so they stop at request construction. The
daemon tests start at `d.Handle(ctx, method, json)` in process
(`internal/daemon/daemon_test.go:282`). A real socket is exercised, but with a
three-line `echoHandler` on the far end (`internal/client/client_test.go:21`); no
test ever passes a `*Daemon` to `Serve`. So nothing proves a JSON frame written to
the socket reaches `Handle` with the right method and params, and
`internal/mcp/sync.go` has no test file at all, which is where R-13, R-37 and R-38
live.
**Consequence.** The seam between the two halves of the product is unverified,
including every path in Band A that depends on what a caller actually receives.
**Where.** `internal/mcp/mcp_test.go:112`, `internal/client/client_test.go:21`.
**Fix.** One integration test: real store on a temp dir, real daemon, real
listener, an MCP server pointed at it, then `notify_user` and assert the row. The
awkwardness the tests avoided is that `client.Dial(..., nil)` would exec the test
binary with the argument `daemon`, which is solved by passing a `SpawnFunc` that
starts the daemon in process.
**Test that would have caught it.** This is the entry.
**Size.** a day.
**Confidence.** Confirmed by reading the test tree.

### R-42. No test closes a real socket, and there is no clock

**How it fails.** Every caller-disconnect test simulates the hangup with a
`context.CancelFunc` (eight in `daemon_test.go`, plus the subsystem equivalents).
The one test of the real thing is a layer down and unwired
(`internal/proto/hangup_test.go:20` over `net.Pipe`), and STATUS lists confirming
FR45 with a killed caller on a real session as still outstanding. Separately there
is no clock abstraction anywhere: production code calls `time.Now` and
`time.AfterFunc` directly, and tests compensate with millisecond config values,
reaching into unexported fields to backdate, and 75 real sleeps, of which 30 are
unconditional barriers totalling 4.4 s. The tightest is a 900 ms sleep against a
1 s timeout (`daemon_test.go:867`), which is the margin that would have to shrink
to catch R-03.
**Consequence.** The timing edges are where Band A lives, and they are tested with
real time on a loaded machine. `internal/daemon` alone is 45 s of a 48 s suite.
**Where.** `internal/daemon/daemon_test.go:867`, `internal/store/store_test.go:363`.
**Fix.** Introduce a clock interface with a fake, starting with the item lifecycle
(toast expiry, escalation, grace, expiry, caller-gone), which is the subsystem where
five of the top fifteen findings are. There is already an unused seam for this at
`internal/session/conversation.go:43`. Then one real-socket test: start a daemon on
a temp socket, connect, park an ask, close the connection, assert `CallerGone`.
**Test that would have caught it.** R-03 and R-07 both need it.
**Size.** a week for the clock, a day for the socket test.
**Confidence.** Confirmed by reading the test tree.

### R-43. No test migrates a store written by an older binary

**How it fails.** `store_test.go:33` applies all thirteen migrations to an empty
database, `:168` proves reopening is idempotent, and `:179` fabricates version 99
to test `ErrSchemaTooNew`. Nothing seeds a store at version 5 and upgrades it, and
no fixture database is checked in. The partial-failure path that leaves a comment
about the schema being left at version N (`internal/store/store.go:172`) is
untested, as are the naming and sequence checks (`:124`, `:128`, `:139`).
**Consequence.** The one operation that touches every user's data on every upgrade
is verified only on the empty case. R-11 and R-23 both live here.
**Where.** `internal/store/store_test.go:33`, `internal/store/store.go:165-183`.
**Fix.** Check in one small fixture per historical schema version, or generate them
by applying a prefix of the migration list, then assert an upgrade preserves rows.
Both are cheap; the generated form does not rot.
**Test that would have caught it.** This is the entry.
**Size.** a day.
**Confidence.** Confirmed by reading the test tree.

### R-44. The log cannot tell you whether a window appeared, and its `level` key is ambiguous

**How it fails.** Two defects in the log itself, both verified against the live
8,364-line file. `item.created` passes the item's severity as `"level"`
(`internal/daemon/daemon.go:1204`), which slog emits beside its own `level` key:
428 lines carry two, exactly the count of `item.created`, and every JSON parser
returns the item severity for those lines, so filtering by log level is
unreliable. And `store.migrated` is logged unconditionally on every start
(`cmd/agentbox/daemon.go:365`), 256 times against 256 `daemon.start` lines, all at
schema version 1, so grepping for a migration finds one per boot.
Then the incident that matters most is the one the log cannot answer. For "an item
was created and no window appeared" you can prove the item reached the store, that
it was placed at the head of the queue, and that none of the four gates held it,
because each has its own line carrying `item_id`
(`daemon.go:1270`, `:1274`, `:1277`, `:1282`). You cannot tell whether a window
ever existed: `item.displayed` is logged under the daemon lock before any UI call
(`:1500`), no line in `internal/webui` carries an `item_id` at all, the one
success line is Debug (`bridge.go:37`), and the two failure lines that would fire
(`webui.go:522`, `:536`) carry no item id, so even when they appear you cannot say
which item they belong to. A successful notification produces three visible lines
in total: created, displayed, dismissed.
**Consequence.** The product's central failure mode is the one its log is silent
about. Remote diagnosis stops at "the daemon thinks it showed it".
**Where.** `internal/daemon/daemon.go:1204`, `:1500`, `internal/webui/webui.go:522`,
`:536`, `internal/webui/bridge.go:37`.
**Fix.** Rename the colliding key to `severity`. Thread `item_id` through the webui
lines and promote one success line to Info. Log the surface's ready handshake with
the item it was for, which the R-05 fix makes natural.
**Test that would have caught it.** A log assertion test is cheap and nobody has
one: capture the handler's output for one notify flow and assert the sequence of
event names and that every line carrying an item has the same id.
**Size.** hours.
**Confidence.** Confirmed by reading, with the counts verified against the live log.

### R-45. Three more things the log does not say, each of which turns a diagnosis into a guess

**How it fails.** (1) The most important store write in the product logs nothing on
failure: `CreateItem`'s error is returned to the caller and never logged
(`internal/daemon/daemon.go:1200`), and the same shape appears at fifteen other
sites. The live log confirms the asymmetry: four `store.resolve_failed` lines
exist and zero for item creation. (2) `item.dismissed` has no actor. An agent's
retract, a human clicking Dismiss, `agentbox dismiss --all` and a toast expiring
all emit the identical line, and `DismissItems` computes the actor per candidate
and throws it away (`daemon.go:1892-1896`). (3) `[log] level` offers debug, info,
warn and error in the settings surface and only debug is honoured
(`cmd/agentbox/daemon.go:325`), so choosing warn silently behaves as info. Beyond
the log: there is no UI-thread liveness signal anywhere, no counters, and
`agentbox status` returns exactly two fields, pending and version
(`internal/daemon/daemon.go:772`), so nothing distinguishes a healthy daemon from
one wedged on the main thread (R-19).
**Consequence.** "He says he answered it" and "the window is frozen" are both
answerable in principle and unanswerable in practice.
**Where.** `internal/daemon/daemon.go:1200`, `:1892-1896`, `:772`,
`cmd/agentbox/daemon.go:325`.
**Fix.** Log the create failure. Carry the actor on a dismissal. Honour the level.
Add a UI heartbeat that `agentbox status` reports alongside pending and version,
which is the single most useful addition for a machine you cannot reach.
**Test that would have caught it.** The same log-assertion harness as R-44 for the
first three. The heartbeat needs the live check.
**Size.** hours for the log, a day for the heartbeat.
**Confidence.** Confirmed by reading, with the counts verified against the live log.

---

## Checked and found correct

Stated so nobody spends a day filing against them.

- **An orphaned lock does not free itself.** A hold whose session dies becomes
  orphaned with the pid it recorded and is freed only when that pid is gone or a
  human breaks it. The reasoning is in `internal/daemon/locks.go:19-45`: a dropped
  attach proves the MCP child died and says nothing about the `make deploy` it
  started. Correct as written. R-37 is a defect in the pid, not in the rule.
- **A pending item is never evicted.** `Prune` filters on `state != 'pending'`
  regardless of age (`internal/store/store.go:420-458`), so the question in the
  brief about a pending blocking item whose row is evicted has no answer: it cannot
  happen.
- **A signal cursor past the retention edge is reported, not silently served.**
  `gapAt` answers from what retention recorded taking, per topic, and the batch
  says so on the wire (`internal/daemon/signals.go:412-429`).
- **Register-before-check in all four hand-off hubs.** A waiter is in the map
  before the read that decides whether to park, then reads again
  (`signals.go:343-366`, `artifacts.go:120-131`, `wthub.go:87-100`).
- **A grant to a waiter that gave up is released again**, rather than left held by
  a call that moved on (`internal/daemon/locks.go:792-817`).
- **A walkthrough submission with nobody waiting persists**, and delivery is
  claim-once against the store with a push-back on a lost race
  (`internal/daemon/wthub.go:46-64`, `:164-182`).
- **Answer delivery is exactly-once**, arbitrated by
  `UPDATE ... WHERE state = 'pending'` and a buffered channel deleted in the same
  critical section (`internal/store/store.go:242-275`,
  `internal/daemon/daemon.go:1785-1799`).
- **Two auto-spawns cannot make two daemons**, and only the flock holder may
  unlink a stale socket. Lock, then remove, then bind
  (`internal/server/server.go:75-101`); the loser exits 0.
- **The socket is bound before the store and the UI initialise**
  (`cmd/agentbox/daemon.go:341` against `:358` and `:595`), so a client dialing
  during startup queues in the backlog instead of failing.
- **Cancel-before-wait in `Conn.Serve`** is the whole of FR45 working, with the
  reasoning in the code and a regression test
  (`internal/proto/rpc.go:980-994`, `internal/proto/hangup_test.go:19`).
- **Raw HTML never reaches a surface**, and every agent string written into an
  attribute goes through the escaper. The policy sweep runs one hostile document
  through all eleven producers of surface HTML and self-tests its own fixture
  (`internal/webui/policy_test.go:154`, `:219`).
- **The artifact sandbox closes every escape probed by
  `tools/artifact-probe.html`**: opaque origin, no network, no modals, no popups,
  no top navigation, no camera or microphone. R-35 is about exhaustion, which the
  probe does not test.
- **An image may name a local file and nothing else**, is typed by magic number,
  excludes SVG, and refuses a non-regular path with the `/dev/zero` case named in
  the comment (`internal/webui/images.go:218-223`). R-16 is that the viewer never
  got the same guard.
- **The `randr` gate** exists because calling into an uninitialised extension
  panics; removing it would be a daemon-killer (`internal/webui/x11.go:78`).
- **Non-US layouts** are handled by a per-stroke group lock, with the measurement
  and the residual race documented (`internal/hand/xkb.go`), and the hotkey grab
  deliberately covers every keycode producing the keysym across all groups.
- **`internal/hand/target.go`** is the strongest code in this audit: geometry
  re-read from the server in root coordinates and the pointer and focus chains
  checked against the locked window before every click and keystroke.
- **A stuck assignment run is reconciled on start**, and missed runs are surfaced
  rather than dropped (`internal/daemon/assignments.go:212`, `:345`).
- **XTEST absence** is reported as an RPC error rather than a crash
  (`internal/daemon/daemon.go:902-906`).
- **No `ExecStop` in the unit**, with the long comment explaining that
  `agentbox quit` there destroyed the healthy daemon it was meant to manage.
