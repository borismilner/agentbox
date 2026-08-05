# Handoff - AgentBox: FR83's whole deferred queue is closed, and looking at the board paid for itself again

*Written by session 46, which fixed the identity colour, the project name, two board
dead ends and the form card - and found, by opening one row, a shell bug that had
been widening the window from any surface since the webview port.*

**Written:** 2026-08-05 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb              # expect clean, in sync with origin/main
git log --oneline -9        # newest: 0b268bf 650df81 cbc257b e68bfba 042c51d b365bd3 22f9965 faabd55
make deployed               # 0b268bf6b14c or newer, NOT "(dirty)"
agentbox pending            # expect "nothing pending"
agentbox sync agents        # your own row, put there by a hook
agentbox sync locks         # expect "no locks held"
make check                  # gofmt + vet + race, ~2 min
```

Then start **FR73**, which is the top of the queue and the last field request Boris
filed in his own words. The research is done and is below; nothing else is in flight.

### FR73 - a card body cannot be read back. Everything found for it

His complaint: *"I've missed what you said - so I tried to look in the inbox and
there doesn't seem to be a way to find it - this is a problem. I can't see the whole
thing you wanted to tell me."* The FR's own words: **a reader, not a schema change.**

What is actually there today, read this session:

- `wireItem` (internal/webui/inbox.go) carries `Snippet: snippet(it.Body, 140)` and
  nothing else of the body. A resolved row's only body is the `title=` tooltip, which
  truncates - that IS the defect.
- `StoredItem` already holds everything the FR asks for: `Body`, `Answer`, `Reply`,
  `Values`, `CreatedAt`, `ResolvedAt`, `State`, plus `Diff` on the item.
- `RenderMarkdown` (internal/webui) is the Go-side markdown renderer already used for
  `view.bodyHtml` and `ask.bodyHtml`.
- **Two allowlists must both learn any new injected field**: the `{@html}` allowlist in
  `frontend/policy_test.go` (TestEveryHTMLInjectionComesFromGo) and the sweep in
  `internal/webui/policy_test.go`. A new `det.bodyHtml` fails both until added.
- The bridge pattern is `frontend/src/lib/bridge.js` -> `Call.ByName(svc("ItemDetail"), id)`
  against a `func (b *Bridge) ItemDetail(id string) wireDetail` (copy `BreakLock` in
  internal/webui/agents.go).
- `inbox.rows` already caches the last snapshot of `store.StoredItem`, so a lookup by
  id needs no store read; `Source` only offers `RecentItems(limit)`, so re-read 100 and
  search again on a miss rather than widening the interface.
- Clicking a row today calls `bridge.promote(it.id)` **only when pending**. The FR says
  a row opens a detail view; a pending row's detail should carry "Show the card" so
  promoting stays one click, and the keyboard triage path (j/k + TRIAGE_KEYS on
  `chosen`, which only ever points at PENDING rows) must not change.

### After FR73

- **FR65** - open a citation in the editor.
- **FR74's fullscreen marker** - built session 34, never exercised, needs consent to
  drive.
- **FR84's other half**, and only with his word: a long body still pushes a form's
  fields down a scrolling card. He picked the compact shape over the one that fixed
  this (approach C in [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html),
  which is in the repo now precisely because that half is open). Do not build C on a
  session's initiative.
- **The row detail's three empty blocks.** `wireAgent.Timeline`, `.Signals`, `.Items`
  and `.Pending` are rendered by Agents.svelte and filled **only by demo.go** -
  `ShowRoster` never sets them, so opening an agent row shows the meta list and held
  locks and none of the history the design promises. The handoff before this one asked
  for signals "on expand rather than in the snapshot"; the honest shape is a bridge
  call per opened row. Note that "received" signals are **not recorded anywhere**: the
  store's `sync_signals` has `session_key` for the POSTER only, so received needs a
  bounded in-memory ring where a wait resolves (internal/daemon/signals.go), the way
  the activity ring would work.

## Where we are

FR83 is finished and so is everything Boris deferred behind it. This session closed
FR85, FR86 and FR84, trimmed two dead ends on the Agents board, and fixed a
shell-level layout bug that any surface could trigger. Eight commits, all pushed, all
deployed, and every UI claim was checked on the real screen rather than in a diff. The
open queue is his older one: FR73, FR65, FR74's marker.

## What this session changed, and what each cost

**FR85 was two divergences, not one.** Go hashes `agent + " " + project`; the frontend
hashed the two around a literal NUL, so four of five sampled identities wore different
colours. Taking Go's separator also made `tokens.js` text again - the NUL was why
`grep -rn identityHue frontend/src` came back empty and the second implementation was
invisible to every search for it. Pinning both sides against a table then showed the
divergence nobody had looked for: the frontend hashed **UTF-16 code units** where Go
hashes **UTF-8 bytes**, identical for every ASCII identity and a different colour for
the first project directory that is not one. Three tests now hold it: a fixed table
(Go), both implementations run over that table through node (skipped where there is no
node), and the shipped `dist` bundle checked for the NUL so a fix that was never
rebuilt fails `make check`. The cross-check was verified by breaking the separator on
purpose and watching it name both sides.

**FR86** replaced `filepath.Base(cwd)` in all six identity sites with
`daemon.DeriveProject`, which rides the walk `deriveArea` already did. An agent in
`frontend/src` reported project `src`; it reports `agentbox`. Watched live: the old
binary announced `sleep · src`, the new one `sleep · agentbox`.

**The board's two dead ends.** A blocked row said "blocked: lock X, held by Y" in the
chip AND "waiting on X for 20s, held by Y" underneath; the chip is trimmed on the
surface only, because `sync agents` has no second line. A shared value highlighted on
hover and did nothing on click; it opens now to the full value and its owner, and a row
with nothing more to show stops highlighting at all.

**Opening that row found the real bug, two files away.** The shell (`App.svelte`) is a
grid whose main column is `1fr`, and a `1fr` track's automatic minimum is its content's
min-content width - so **one unbreakable token in ANY surface widened the whole window**
and pushed the right edge of every row off screen. `min-width: 0` on the block did not
fix it and `word-break: break-all` did not fix it; `min-width: 0` on `main` did.
Latent for every surface since the webview port, found by a JSON value with no spaces.

**FR84 was answered by Boris, not decided for him.** A live artifact showed today's
card and three approaches; he picked the select with its chosen option spelled out
underneath, including the cost written on the mock (you read the option you picked, not
the ones you did not). The threshold is one number in two files with a test that fails
if they drift - FR85's lesson applied the same day.

**The usage assignment (`a0eff4b720959`) needed no retry and did need a fix.** Its ok
run predates the previous handoff's note, so that item was stale. But the run had found
two real defects nobody acted on: the prompt asked for a **`qq-data`** block, a tag
AgentBox has never parsed (rename fallout), and it read a 14-hour-stale
`cachedUsageUtilization` because it believed `claude -p /usage` could not answer
headless. It can. Both fixed in the prompt, and one run proves it: live figures,
`source: "claude -p /usage"`, and a captured data block.

## Traps this session paid for

- **`LockedHint` changes mid-session.** It was `no` at 13:30 and `yes` at 13:42, and a
  capture taken after that photographed the lock screen - an all-grey window with a
  clock where the board should have been. Check it immediately before looking, not once
  at the start.
- **Window ids are recycled.** One capture named a window id from a listing a second
  earlier and photographed an unrelated window of Boris's (deleted, and he was told).
  Verify the window NAME immediately before the shutter:
  `n=$(xdotool getwindowname $id); case "$n" in *"agentbox · app"*) import -window $id ...`
- **Window titles differ by surface**: `agentbox · app`, `agentbox · toast`,
  `agentbox · hands off`, and a **card is titled plainly `agentbox`** (use `window =agentbox`
  in `drive_desktop` for an exact match). A toast lives seconds, so poll for it in the
  same command that captures it, and give it ~0.8s to paint or you get a 300-byte png.
- **`import -window` on the app window returns one flat colour** while the screen is
  locked or the window has not painted. One distinct colour in the png is the tell.
- **`agentbox form --field` syntax is `type:key:opt1,opt2=default`** - the default goes
  LAST, after the options, and an option cannot contain a comma. Getting it wrong exits
  with usage and (with stderr silenced) looks exactly like a card that never appeared.
- **Clicking a row you already opened closes it and the layout moves**, so a second
  hover coordinate computed from the first screenshot lands on a different row. Sample
  the pixel AND read the png.
- **A `1fr` grid track's automatic minimum is min-content**, so no amount of wrapping
  inside a surface can stop it widening the window. Fix it at the track.

## Live state (volatile - verify on resume)

- **Deployed:** `0b268bf6b14c`, clean stamp, `make deployed` agrees with HEAD.
- **Git:** clean, `main` pushed to `origin` (GitLab, push-mirrors to GitHub). This
  session, oldest first: `faabd55` (FR85), `22f9965` (FR86), `b365bd3` (the two board
  dead ends), `042c51d` (docs for FR85/FR86), `e68bfba` (the value's wrap, which turned
  out not to be the cause), `cbc257b` (the shell's `1fr` track, which was), `650df81`
  (FR84), `0b268bf` (docs for FR84 + this session's history), plus this handoff.
- **Background jobs: none.** The `tools/sync-probe.py board` fixture and the two lock
  wraps have all exited. `pgrep -ax agentbox` should show one daemon plus one
  `agentbox mcp` per live session. **PRs:** none, ever - Boris pushes `main`.
- **Nothing pending, no locks held, blackboard empty** (the three demo shared values
  were deleted after the look).
- **Two other sessions were on the board** during this one: one in
  `~/work/assignments` ("fixing images#9879"), reachable through the same daemon. It
  was not touched.
- **The desktop was taken three times and released three times.** No `agentbox · app`
  window should be open; the deploy at the end closed it.
- **Usage:** session **23%** used, resets 2026-08-05 14:59 Asia/Jerusalem; week (all
  models) **3%**, resets 2026-08-12 04:59. Read it with
  `claude -p /usage 2>/dev/null | grep -E '^Current '` - it works headless, whatever
  the assignment used to believe.
- **In-flight edits: none.**

## Blocked on you (Boris)

Nothing - proceed autonomously. Two things you may want to weigh in on:

- **Your PostToolUse hook writes the raw Bash command as the activity line.** A
  heredoc commit message put five lines of commit prose on your own Agents board this
  session. It is your `~/.claude/settings.json`, so the wording is yours; truncating to
  the first line, or to the first 80 characters, would read better on the board.
- **FR84's other half** (a long body still pushes a form's fields below the fold) needs
  your word, because the approach that fixes it is the one you did not pick. The mock is
  in the repo: [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html).

## I can do solo (no input needed)

1. **FR73** - the inbox row opens a detail view. All the research is under "Do this
   next"; it is a reader, not a schema change.
2. **FR65** - open a citation in the editor.
3. **The row detail's three empty blocks** (timeline, signals, items), on expand rather
   than in the snapshot. Received signals need a ring; nothing records them today.
4. **FR74's fullscreen marker**, which needs consent to drive the desktop.

## Facts - verified vs assumed

- [verified] **The identity colour agrees across the two implementations, measured on
  screen.** A toast's pill (frontend hash) and the inbox row's dot (Go hash) for
  `agent · agentbox` both sampled `hsl(225 62% 68%)`. The frontend would have painted
  stop 30 for that identity before this build.
- [verified] **The cross-language test can fail.** Breaking the separator on purpose
  made it name both sides for all eight identities; it was then restored and re-run.
- [verified] **FR86 live against the running daemon**, from `frontend/src`: old binary
  `sleep · src`, new binary `sleep · agentbox`.
- [verified] **The blocked chip reads "blocked"** with the wait line under it carrying
  lock, age and holder - photographed with a real waiter behind a real lock.
- [verified] **A shared value opens, wraps inside its row, and the board stays the
  window's width** - photographed after the shell fix, having been photographed
  overflowing twice before it.
- [verified] **A short unowned shared row does not highlight under the pointer**:
  sampled `#161920` at the pointer against `#1c2028` on a hovered expandable row, with
  `xdotool getmouselocation` confirming where the pointer was.
- [verified] **FR84 on screen, both halves.** A three-field form opened at 470x437 with
  every control and Submit above the fold and each spelled line reading its option in
  full; Tab-then-Down onto a longer option grew the window 470x274 -> 470x309 and
  wrapped the line to three.
- [verified] `make check` passes (gofmt, vet, race) with the new tests, and the
  identity tests fail red when dist is stale.
- [verified] **The usage assignment's fix, by running it**: live figures, `source:
  "claude -p /usage"`, week 2%, and a captured `agentbox-data` block.
- [verified] No em-dashes, curly quotes or filler vocabulary in what this session
  wrote, checked over `git diff` rather than by eye.
- [assumed] **That the identity cross-check runs on a machine with no node.** The skip
  path was written, not exercised.
- [assumed] **That `provisionalFor` retires a hook-only row after ten minutes.** Read
  in the code, never waited out. Carried from session 45.
- [assumed] **The 200-key prefix cap** on the surface and in a tool result; tested at
  the store layer only.
- [assumed] **`@me` in a shared key**, and **a real client's `await_signal` parked
  longer than 20s** (the 1500s ceiling and the keep-alive ticker have never been
  exercised together). Carried from session 45.
- [assumed] **The `holder parked on ask_user` and 600s long-wait lock warnings** -
  unit-tested, never seen live. Carried from session 42.
- [assumed] That `webui-demo agents` still renders; that path has not been opened since
  session 44 gained three shared values in its fixture.

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 45's whole "What slice 5 changed under everything" section (the derived session key, `inheritedSessionKey` returning empty, the load-bearing start time, the `\|\| true` on hooks) | Nothing changed in it this session and it is not on the path of the work ahead. It lives in 09-sync.md's "Identity: the session key" and in session 45 of [docs/history.md](docs/history.md) |
| Session 45's "changes that live OUTSIDE this repo" section, with the settings-revert `jq` recipe | Still true, still needed only if the hooks must be reverted: [docs/recipes.md](docs/recipes.md) is the canonical copy of what was installed, and session 45 in history.md carries the revert recipe |
| Session 45's lock-order paragraph (read every other subsystem's state before taking your own mutex) | The comment on `roster.snapshot` in internal/daemon/sync.go, which is where a session touching it will be |
| Session 45's "looking at the surface" command block | Rewritten as this session's traps, which found three more failure modes in it (lock state changing mid-session, recycled ids, per-surface window titles) |
| The old queue lines for FR85/FR86/FR84 and the "Claude usage check retry" | All four are closed; their records are in [docs/07-field-requests.md](docs/07-field-requests.md) and the assignment's own run history |
| Session 45's "photograph a hook-announced row" solo item | It had already been done in session 45 and its own facts section said so. Deleted as a contradiction, not moved |

## Map

1. [docs/07-field-requests.md](docs/07-field-requests.md) - FR numbers. **FR73 and FR65
   are the open ones**; FR84/FR85/FR86 closed 2026-08-05 with what each fix found.
2. [docs/STATUS.md](docs/STATUS.md) - current state, what works, known gaps.
3. [docs/09-sync.md](docs/09-sync.md) - FR83, all five slices, the chip vocabulary (it
   now says what the surface shows and what the CLI shows, which differ on purpose).
4. [docs/history.md](docs/history.md) - session by session; this session is "Forty-sixth".
5. [docs/mocks/fr84-form-shapes.html](docs/mocks/fr84-form-shapes.html) - the four card
   shapes Boris chose from; approach C is the unbuilt half of FR84.
6. [docs/agent-manual.md](docs/agent-manual.md) - the tool reference.
   `internal/manual/agent.md` is the embedded copy; `TestManualListsEveryTool` fails if
   a tool ships without them.
7. [CLAUDE.md](CLAUDE.md) - traps that have cost sessions; read before touching the
   build or the daemon.
8. `tools/sync-probe.py` - `rider`, `locks`, `signals`, `shared`, `board` scenarios.
   `board` leaves orphaned holds behind on purpose; its own sessions exit early, so the
   blocked row it promises is better made by hand with two `agentbox sync lock` wraps.
