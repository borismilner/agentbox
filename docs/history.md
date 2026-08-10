# Session history

The dated, session-by-session record that used to accumulate at the top of
[STATUS.md](STATUS.md). STATUS.md now carries only the current state; this
file carries how it got there, newest first. Nothing here is needed to build
or run AgentBox - it is the record of decisions, defects and verifications, kept
because each cost something to learn.

The project has worn earlier names; prose here uses the current name
throughout, including in entries dated before a rename.

## Sixtieth session (2026-08-10): the wiki's pictures, all of them, on a desktop

Boris asked for the wiki's examples to be created rather than captured, and for
the pages to be fixed and pushed. Twelve of the fifteen frames are now drawings
and both wiki hosts have them, along with `README.md`, which had been showing
screenshots taken before the rename.

**Four defects in the harness came first, and three are one shape: something
stood in for waiting and was not waiting.**

The ground `draw.py` computed never reached the page. It sized the viewport for a
24px margin and the page padded 24px unconditionally, so a frame that asked for
none got one anyway - the agents board sat in a black band its own spec says to
crop away, and the app window shrank to its content instead of filling the frame.
Both numbers now come from one place.

The measuring pass inherited a definite height. `app.css` sets `height: 100%` on
`html`, `body` and `#root`, which is right for a window and wrong for a probe, so
a surface that is `min-height: 100%` filled the 2000px measuring viewport and
handed back 2000 as the height it needed. The progress window did exactly that.
The subtler version of the same failure had been shipping since the harness was
built: the toast was measured out of a DOM dump taken before its last resize
landed, and under-reported by 22px.

The capture waited on the wrong clock. `--virtual-time-budget` fast-forwards a
page's timers rather than its CPU, so it expires in a few real milliseconds and
photographs whatever happens to be finished. Every surface survived that. The
artifact did not: half a megabyte of inline React in a sandboxed iframe came out
an empty stage in one run and a working canary console in the next, from one
fixture. An hour went into believing it was the content - the stat row I had just
added, then the opacity syntax, then the grid - before the two-runs-differ shape
of it pointed at timing rather than at markup. `tools/wiki/shoot.mjs` now drives
the browser over the DevTools protocol and waits for the page to say it is ready,
including a per-frame selector for the ones with an iframe in them.

And then the animations had to be settled explicitly, because with a real clock a
card is caught mid-drop: two draws of one fixture produced two different files.
Things that end are finished, things that loop are frozen 40% in - at zero the
progress window's indeterminate sweep sits off the left of its own track, which
reads as a bar that is not working.

**Two things the request did not anticipate.**

Go has to render the documents. The viewer, the artifact and the console show
*rendered* markdown, and in the product that HTML is `mdhtml.go`'s and
`artifact.go`'s. A fixture that hand-wrote it would be drawing a renderer nobody
ships, so `tools/wiki/drawhtml` calls `RenderMarkdown` and `RenderArtifact` and
`draw.py` runs it before every drawing.

The frames need a desktop, and not only for the three whose subject is where they
sit. Boris looked at the published secret card and called it ugly beside the
toast, which was right and had a mechanical reason under it: a card is a
transparent frameless window sized to its own content, so the CSS shadow it
carries is clipped by its own edge. What draws a shadow under a window on a real
screen is the desktop under it, and a frame cropped flush has neither. Every
frame now sits on the same drawn desktop, in an iframe sized to exactly the
window Go opens - which is also what lets `height: 100vh` on the hands-off strip
mean the window rather than the picture.

**What the drawing found, which is the return FR99 hoped for.** Four claims in
the docs that the code contradicts. `DESIGN.md` specified an inbox outcome column
reading `unanswered`, a word `outcomeOf` cannot produce. It specified the
artifact's preview/code toggle as "the trust half of the shot", and `app.css:960`
hides that toggle in exactly the shape `agentbox show --artifact` opens - so
`documents-and-artifacts.md`, which claimed the toggle sits above *every*
artifact directly over that command, was wrong in its own strongest paragraph.
It asked the console frame for a turn's 24-hour timestamp, which a 600px console
with an ask panel up cannot show. And an artifact that fails to run says nothing
at all: bar, badge, runtime label, empty stage, and an empty error slot. The last
two are filed as U-17 and U-18; the rest are corrected in place.

**What is still photographed, on purpose.** Three frames, and each for the same
reason: the whole argument of the frame is that it is real. The install doctor is
a terminal transcript. The history table's point is that the rows are this
machine's unpruned own. The review board's spec asks for a real change from this
repo. Drawing any of them would replace evidence with fiction, which is the line
`DRAW.md` now draws.

## Fifty-ninth session (2026-08-09): the owed look at a screen, and four band-A fixes

Session 58 left one thing owed - nobody had looked at U-01 or U-02 on a desktop -
and it was owed for a reason worth reading: the drive was interrupted a keystroke
in because Boris could not tell a scratch card from his own.

**The owed check, done.** Esc on the only card paints the amber line in full and
the window grows 199 to 241 to fit it; the `x` puts it away and the window shrinks
back, which is the other direction of the remeasure and the one that had to say so
itself; answering clears the notice and hands `staging` to the caller. `Defer`'s
empty-queue refusal made the whole trip from a Go sentence to a line a human can
read, so one of the thirteen refusal reasons has now been seen rather than
asserted.

**It cost the desktop its only signal, and that is a trap now written down.** The
HANDS OFF strip is a window the daemon draws, so `make stop` takes it down with
everything else: control is granted, and the one thing telling Boris it is held
disappears at the moment the driving starts. He asked, reasonably, why nothing was
happening. Same shape as the deploy lock that cannot live inside the daemon the
deploy stops. CLAUDE.md carries it, along with the tell - `set_activity` answering
`{"live":false}` means the strip is already gone.

**U-16, found by looking rather than reading.** The card advertises `Esc defer`,
`⇧Esc dismiss` and a number per option, and none of it reaches the card unless the
card holds focus - which it often does not, on purpose, because vision principle 3
says AgentBox never grabs focus and the panel and the progress bar both paid for
that rule. The first card raised by a fresh daemon took focus and its whole keymap
worked. A second, raised seconds after the first closed, did not: `xdotool` named
the terminal, `1` did nothing twice, and the same `1` answered immediately after
one click. It is U-01's symptom by a route U-01's wrapper cannot see, because no
bridge call is ever made to fail. It wants one more repro before its fix is
chosen: "always" and "sometimes" argue for different answers.

Two things follow from it. `ux.md` said "nothing here was found by looking at a
running window", and that sentence is now false and amended. And R-40's harness
would not have caught this either - the whole mechanism (who owns the X input
focus) is outside every file that audit read.

**FR99: the wiki's frames are drawn now.** `tools/wiki/draw.py` plus
`frontend/draw/` serve the shipped surfaces with the daemon door aliased to a
fixture and photograph them headless. No desktop, no daemon, no XTEST. Two frames
are drawn - the card, and the agents board that the photographer could not get
because `sync lock` mints a session key per lock and turned four agents into seven
rows.

Three things came out of building it that the request did not anticipate. The
height is the product's own: every frameless surface already hands Go its measured
height through `bridge.fit()`, so the picture is exactly as tall as the window a
real run would open, rather than as tall as a viewport somebody guessed. A redraw
is byte-identical, because the clock is frozen before the bundle loads - so a diff
in `docs/wiki/img/` means the product changed, which is what makes these
reviewable in a way a photograph never was. And the drawing is more faithful to
its own specification than the photograph it replaced: DESIGN.md asks for no
desktop context and `expires in 1:57`, and the photograph had a code editor behind
it and read `1:40`.

Drawing S2 then caught DESIGN.md being wrong about the product - it said the
shared-values block sits below the agent rows, and `Agents.svelte:292` has always
put it above. The photograph had shown the real order and been accepted, so the
sentence survived a sitting it contradicted. The doc is corrected; whether above
is the RIGHT order is left open, because that is a design decision and not a
caption fix.

**Six band-A robustness items, all deployed.** Band A is down from fifteen to seven.

- **R-14**: every tool that promises to return at once was bounded only at the
  DIAL. Once a connection existed, `notify_user` - whose description says it never
  blocks - would wait forever on a daemon that accepts and then wedges, with the
  keepalive reporting it healthy throughout. Now capped at 10s through bounded
  twins of the three call helpers. Opt-in, deliberately: marking a blocking tool
  fast by mistake would cap how long a human may think, which is worse than the
  defect, so the safe direction is the one that needs an edit.
- **R-03**: the expiry can fire and do nothing, because the answer is inside its
  undo grace. The handler then fell into a bare receive with no deadline and no
  ctx branch - so a later undo left the item pending with the timer already spent
  (the agent's `timeout_s` silently unbounded), and the handler could no longer see
  its own caller drop (the card showing a live caller for an agent that had gone).
  The select is a loop now, and a bounced expiry is re-armed for a full window.
- **R-04**: nothing capped what an agent could put in an item, so the only bound
  was the 4 MB wire line - and passing it produced no refusal at all, just a
  dropped connection and `EOF`, with nothing stored. Caps in `Validate` that name
  the field, the limit and the actual size; `Serve` answers `ErrTooLong` before the
  connection goes; and the client keeps that sentence rather than the EOF that
  follows it, without which the refusal is sent and read by nobody.
- **R-05**: the card and toast were the only surfaces with no pull. `Bridge.View()`
  now exists and both call it on mount, after `ready()` so the push stays the fast
  path. A push that arrived first wins, or a form somebody has begun filling in
  would reset.
- **R-11**: one unreadable JSON blob failed the WHOLE store read, so `Pending()`
  failed, `daemon.New` returned the error, the process exited, and every
  auto-spawn repeated it. Total outage from one bad row, with the only evidence a
  line in a log file nobody was reading. The row is skipped now and said out loud
  in a warning card, because a silently dropped item is the same silence one item
  smaller.
- **R-08**: the undo grace holds its outcome in memory for three seconds, which a
  deploy fits inside comfortably, and a daemon stopping in there lost the answer -
  the item came back pending and the human was asked again, into a caller that had
  gone. `BeginShutdown` ships it now.

Every one of the six was checked by neutering the fix and watching the tests fail
- which is session 58's lesson repeated, and it paid twice: R-03's countdown test
and R-14's whole file hang to the test timeout when neutered, which IS the defect
rather than a proxy for it.

**R-05 was then seen on a real card**, in the live queue on the deployed build,
which is the second desktop trip of the session and a much cheaper one: raising
one card into Boris's own queue and answering it needs no daemon swap at all. The
lesson from the first trip is that the swap was the expensive part, and most
checks do not need it.

**An unplanned second use for the drawn frames.** They are a regression check on
the surfaces. Redrawing the card after R-05 gave a byte-identical PNG - a real
browser mounting the real component through the changed path - which answered "did
I break the card" before anybody took the desktop. It is chrome rather than
WebKitGTK and no daemon is involved, so it is a first answer and not the last one.

**One thing the tooling taught.** A commit message containing "harness" is
refused by a guard hook, and the refusal blocks the WHOLE Bash call - so an edit
bundled into the same command never ran either. Worth knowing before diagnosing
why a file looks unchanged.

## Fifty-eighth session (2026-08-09): the answer path learns to say no

Tier 1 of the merged backlog, and the two items it named: U-02 then U-01, which are
one defect from the two ends. Both are deployed (`a3e570de160d`).

**U-02 first, because U-01 is worthless without it.** Every method on the answer
path was declared to return nothing. `func (b *Bridge) Answer(id, label string)` -
no value, nowhere for a refusal to go, even where the daemon had already decided on
one and written it to the log. `Resolver`, `Source.Promote` and every `Bridge`
method on that path now answer `""` or a sentence for the surface to show, which is
the convention the assignment editor and `BreakLock` already used. The refusals were
written one per reason rather than one for all of them, and the tests assert they
are readable rather than matching the words: a sentence, lowercase, ending in a
full stop, twenty characters or more. `Triage` and `AskKey` keep their bool (FR34) -
a refusal makes them report the key as having done nothing, which is what the
surface already knows how to paint, with the sentence in the log. `lateResolver`,
which swallowed every call made before the daemon existed, says so now: that was
the worst version of the defect, since a window opened during startup ate the
keystroke and looked exactly like one that worked.

**Then U-01, which is the same failure from the webview.** The card made 26 calls
and awaited, caught and handled none of them; across the card, the toast and the
inline ask panel it was 38 calls and zero error handling of any kind. One wrapper in
`bridge.js` rather than a `.catch` at each site, because 26 sites is 26 chances to
forget one and the tempting fix - `.catch(() => {})` per call - converts a silent
failure into a silent failure that passes review. The wrapper takes both ways a
keystroke fails (a rejection, and U-02's sentence) and never rethrows.
`lib/trouble.svelte.js` holds the last one, one line per window; the card, the
toast, the ask panel and the inbox detail show it, the card re-measures so it is not
clipped off a frameless window, and the card and toast gained the board's two
`window` listeners.

**The check worth copying.** The 15 vitest tests all passed on the first run, which
is not evidence. Neutering `note()` to a no-op and re-running was: 13 failed and the
2 that passed were the two negative controls. A test that cannot fail is a comment.

**What is owed.** The live pass. The build was put on a throwaway daemon
(`XDG_STATE_HOME` in /tmp, config copied), a choice card was raised and the drive
was interrupted one keystroke in - Boris saw a card he had not asked for and asked
whether the session was stuck. Nothing was wrong and his own store was never
touched, but the lesson is the interruption itself: a card from a scratch daemon is
indistinguishable from a real one, and holding the desktop does not make an
unexplained card on it any less alarming. Say what is about to appear, not only that
something will.

**One behaviour change he will meet daily.** Esc on the only card now says "nothing
else is waiting, so there is nothing to move this behind" instead of doing nothing.
The card's own header promises `Esc defer`, so the silence was the defect; whether
the notice earns its place every single time is a judgement only use will settle.

**FR99, from the same sitting.** The wiki's pictures should be drawn rather than
photographed. The photo pipeline cost eight harness defects on its first run and
still has four shots stuck on things that are about the desktop and not the product.
Drawn frames buy exact content and a reviewable diff, and cost the one thing a
photograph has: it is evidence. The rule that keeps them honest is in the entry.

## Fifty-seventh session (2026-08-07): the deploy that was owed, and the first surface a test has ever mounted

Session 56 left a deploy owed and three things unbuilt. All four are done, and one
of them turned out to be wrong in the handoff that described it.

**The deploy.** The running daemon was `69230d4f7e32`, thirteen commits behind, so
neither fix in `1d00fd2` was live on the machine Boris is actually reached through.
`make deploy` put `74b350326153` on it, confirmed by asking the daemon rather than
by looking at the file. `check` is a prerequisite of `deploy-locked`, so the deploy
exiting 0 is also the end-to-end pass that session 56 could only mark `[assumed]`.

**The UX audit** was the deliverable session 56 owed and its agent died before
writing. `docs/backlog/ux.md`: fifteen items in six bands, same shape as
`robustness.md`. Band A is the finding. The three surfaces where a human actually
answers an agent - the card, the toast and the inline ask panel - hold 38 bridge
calls between them and no error handling of any kind: no catch, no try, no await.
The board handles it and installs an unhandledrejection listener, which is what
makes the other three an omission rather than a house style. Under that, the
answer-path methods in Go return no value at all, so the daemon cannot report a
refusal even when it refuses. That is R-01's other half and the reason it stayed
invisible; `Triage` is the one method that can say no, and the inbox discards its
answer.

Two smaller ones worth naming. `daemonUp` in `App.svelte` is declared, never
assigned, and read by the one dot that would tell you the hub had died; the status
strip's "daemon up" has no condition at all. And `theme.motion = "reduced"` is
honoured by four components out of the thirteen that animate, because the global
kill-switch in `app.css` is scoped to `"none"` - so the obvious middle choice in a
three-item control does almost nothing.

**The carried note that was wrong.** Session 56's handoff, STATUS and the dead
agent's last words disagreed about the card's height. The handoff said it was
estimated from raw text length. It is not: `Card.svelte:143-177` measures the
laid-out card with a ResizeObserver. The dead agent was right to flag it. The real
defect is narrower - the observer can only see the card grow, because min-height
pins it from below, so shrinking runs off a hand-written list of triggers that
omits two controls that shrink it. Filed as U-06, and the claim is corrected in
STATUS where it had been sitting.

**The merged backlog.** `docs/backlog/README.md` puts all seventy-six items in one
order and states the rule. The question the three files could not answer separately
was how a band-A robustness item ranks against F-01: F-01 makes an agent cleverer
about whether to interrupt, band A is about interruptions that cannot be answered
once made, and a better decision to ask is worth nothing when the asking is what
breaks. Writing it caught R-01 and R-02 still listed as open a day after they were
fixed and deployed, which would have made the merged list open by recommending two
finished pieces of work. Both now carry a fixed marker.

**The install shot.** `install.md` has asked for terminal output since it was
written. It is taken, published, and verified serving on both hosts. It is not a
photograph of a terminal and `SHOTS.md` says so at length: no Xvfb and no terminal
emulator on this machine, so both commands were run for real and their verbatim
output typeset. The spec asked for one row honestly reading "missing (only needed
for showcase screenshots)" so the frame would read as a real machine. That row is
ImageMagick's `import`, which is present here, so the requirement was dropped rather
than faked.

**The first test to mount a surface.** Thirty-two Svelte files, thirteen thousand
lines, and nothing had ever executed one - which is why every item in the UX audit
was found by reading. vitest and jsdom are in, and `Card.svelte` has ten tests that
drive it from the keyboard. What they ask is not "is it in the right place", which
jsdom cannot answer, but "does pressing 1 answer the question", which it can and
which nothing was asking. Escape gets four of them, because "no matter how many
times I press Esc, it pops back up" was a real complaint with a real fix and no test.

It earned itself on day one. U-06 is pinned with `test.fails`, and the first version
of that test was wrong in a way worth keeping: it counted the card's mount-time
measurement as a response to the change under test, and so reported the defect as
already fixed. Draining the microtask queue is what separates the two. The finding
held, but only because the number was checked rather than trusted - which is the
argument for the whole tier.

**And then one tier-1 item, because the rig made it cheap.** U-03: the inbox awaits
`Triage` now and says so when Go declines the key, in the place the row already
states what its keys do. Eight more tests, and the inbox is the second surface
anything has mounted. Writing it produced the session's neatest lesson - awaiting an
uncaught promise there would have reproduced U-01 inside the fix for U-03, so a
rejected call is treated as a refusal, which is also the honest reading: whatever
went wrong, the keystroke did not answer anything.

It was committed and held back, because a surface change is done after somebody has
exercised the real webui. Boris then said "actually I lied - lets see it", so it was
exercised: the deployed daemon stopped, the working-copy build put up on throwaway
state (`XDG_STATE_HOME` in /tmp, his config copied for the theme so his own queue was
never the thing being experimented on), two questions raised, and the inbox driven
over XTEST with `tools/uidrive/uidrive.py`. `y` on a choice row gave the amber
refusal, `j` retired it, `1` answered and handed `staging` back to the caller, `n`
refused again. Then `make deploy`, and the daemon is `5485842cc01a`, equal to HEAD.

One thing that pass taught, which no unit test would have: the screenshot taken two
seconds after the answering keystroke still showed both rows waiting. Nothing was
wrong - the daemon coalesces queue pushes - but it is a reminder that on this surface
the pixels lag the store, and a shot is not a reading.

**And then the screenshot sitting, which Boris handed over the desktop for.** The
script had never been run. It did not survive contact with one: eight defects, every
one in the harness rather than the product, and every one invisible to a test that
did not involve a screen.

The first is the one worth remembering. `agentbox sync attach` holds presence open
for as long as its process lives and never returns - its own source says "the whole
point is to stay" - and `stage_roster` ran it in the foreground and waited for an
exit code. The sitting hung on line four of phase one with the machine's daemon
already down, and produced no output for five minutes. Boris noticed before any
timeout did.

Then, in order: `walkthrough create`'s exit code was discarded, so a schema refusal
presented as a 45-second window timeout and said nothing about why; behind that,
`repo_root` was relative where the daemon demands absolute, and there was no
`pinned` SHA at all. Code steps need a `tldr`, which is not paperwork here because
the board opens in brief - for S4 the tldr IS the photograph. The board also opens
on the ground step, which has no code, no highlighted range and no note, so the
first review-board frame was a summary box over an empty half-screen.

The last three were interference between shots, which is the class no unit test was
ever going to find. The three progress feeders hold their pipes open, so that window
sat always-on-top in the corner of the agents board, the inbox and the artifact -
and closing them with a signal made the daemon report an interrupted task, whose
error toast then sat over the next two shots instead. Flood control collapsed S1's
card into a stack, because staging fires a roster, four history items, a toast and a
card from the same sessions inside a few seconds. And S9 is a warning notify, which
by design waits to be read, so it held the queue and the card behind it never became
current. S1 passed alone and failed in every full run, which looks exactly like a
flaky window and was nothing of the kind.

Two things came out of it beyond the fixes. A test that asserted `repo_root == "."`
had enshrined the bug rather than checking the contract, which is what a test looks
like when it copies the code instead of the requirement. And the script's own
verification can only tell a rendered surface from wallpaper - it cannot see a card
sitting over the subject, and three frames it passed had one. Every frame was read
by eye before anything was published.

Ten of the twelve are live on both wikis and verified serving. Held back and said so
rather than shipped quietly: the reader is scrolled to the wrong section, and the
progress and history frames are good but no page asks for them yet.

## Fifty-sixth session (2026-08-07): a public wiki, and twenty claims the code contradicted

Boris asked for a wiki: features and rationale, engaging rather than dull, nothing
that only interests a maintainer, and every page opening with a summary for
somebody in a hurry. Mid-session he added the instruction that made the rest of it
work, which was to read the code and not only the documents, because the documents
might be stale.

They were, in twenty places. The count is the story of this session.

**What went live.** Eighteen pages plus a sidebar at
https://gitlab.com/fu-bar/agentbox/-/wikis/home, mirrored to
https://github.com/borismilner/agentbox/wiki. Source in `docs/wiki/pages/`,
published by `tools/wiki/publish.sh`, because nothing syncs two wiki repos on its
own: GitLab's repository mirroring has never covered wikis (gitlab#37049) and
GitHub has no mirroring at all. Three things differ between the hosts and each one
publishes nothing visible if you get it wrong, so the script renames on the way
out: branch `main` against `master`, page `home` against `Home`, sidebar
`_sidebar.md` against `_Sidebar.md`. The GitHub wiki repo does not exist until a
human saves one page in the browser, and there is no API for it; Boris did that.

**`tools/wiki/lint.py` is the gate.** Half of GitLab's markdown is invisible or
broken on GitHub and it looks correct on the side you wrote it on: `[[_TOC_]]`
becomes a red broken link, footnotes vanish, front matter renders as content. It
also checks that every wiki link resolves, that every page carries its hurry
block, and that no page carries the character-level fingerprints of
machine-written prose.

**Verified by looking, not by pushing.** A headless Chrome against both hosts:
the mermaid sequence diagram renders on each, the screenshot loads at 470px on
each, `<kbd>`, tables and alerts render, no wiki link is broken. The first pass
reported mermaid missing on GitLab and that was a wrong selector, which is the
reason the rule is pixels over diffs.

One belief corrected for whoever reads the research: **GitHub does rewrite
relative image paths per page depth**, emitting `wiki/img/card.png` from the front
page and `img/card.png` from a subpage, both landing on the same file. So images
work relatively on both hosts and neither wiki depends on the other host.

**The twenty stale claims.** `docs/wiki/FACTS.md` is now the audited fact base with
a line of source behind every number, and it is what to quote from. What was wrong:
nine config keys documented as working that no code reads, including three whole
sections; `panel.height_frac = 0.62` documented as the default and outside its own
clamp, so copying the documented file gets you a warning and a reset;
`session.default_mode` documented as plan and shipped as full; `--tab stats` and
`--tab progress` promised in the manual, in `agentbox help`, in the flag help and
in the daemon's own error message, four places and three different lists, all
wrong; the whole `control` family, `webui-demo`, twelve of fourteen `sync` verbs
and `walkthrough repair` missing from help; six sync tools where there are eight,
which broke the same sentence's total of 39; a Progress tab that does not exist,
claimed two lines from the sentence saying it does not; `sync status`, never
implemented; fourteen tools in the showcase where there are thirty-nine; `Ctrl+L`
and `Ctrl+I` in the card keymap and bound nowhere; three inbox filter controls
where there is one search box; earcons documented as all under 400 ms, measured at
90, 160, 220, 260, 340 and 430, the urgent one being the exception and the right
one; a toast stacking collector and a while-you-were-away surface, both specified
and never built; and on the safety side, "an answer is a state transition, not
content" (the log holds the chosen option and the typed reply), the socket
documented as mode 0700 when the directory is and the socket takes your umask, and
"the agent never gets the value", which is true over MCP and not from a shell,
where `--stdout` exists and the card says so in the warning colour.

**One claim deliberately left standing as unverified.** The
artifact-restart-while-streaming rough edge may have been closed by the stable
fence id (`internal/webui/artifact.go:240`) and the `data-live` hydration guard
(`frontend/src/lib/artifact.svelte.js:383`), and may not, because neither helps if
a re-render replaces the node rather than patching it. Nobody has watched it since.
Both docs now say that with the line references. A doc quietly declaring something
fixed on an inference is how this list got to twenty in the first place.

**STATUS lost 88 lines.** It carried fourteen sessions of narrative and then a
paragraph explaining that the session narrative lives in this file. Both were true.
The narrative is gone rather than shortened.

**The backlogs.** `docs/backlog/features.md` (eleven extensions, five bets) and
`docs/backlog/robustness.md` (45 items in six consequence-ordered bands, plus
nineteen things verified correct so nobody refiles them). Three findings were
reproduced rather than reasoned about, and two of them are the failure this product
exists to prevent:

- **A flood-collapsed question that no keystroke can answer.** Flood control is on
  by default at three items in ten seconds. The fourth is stored pending and not
  queued. `Daemon.Promote` returns silently when an item is neither current nor in
  the in-memory queue (`internal/daemon/daemon.go:2096`), and Promote is the
  inbox's only route for text, secret, form and diff. So a question with a caller
  parked on it survives the summary card being dismissed and cannot be answered
  from anywhere. Verified independently at the no-op.
- **A failed store write eats the answer while the card says it shipped.** `resolve`
  returns false and nothing else runs, and because `finalizeGrace` clears the grace
  record first, the last painted view stays the answered strip with undo dead.

The test that would turn a third of the display findings from invisible into red is
the one that does not exist: nothing in the repo renders any of 32 Svelte files.

**Also this session**, outside the repo: the research behind the wiki's prose rules
lives at `~/me/study/guides/writing/ai-tells/`, measured against four corpora
rather than asserted, and it inverts three pieces of standard advice. Vocabulary
separates human from machine prose by seven to seventy-five times while
sentence-length variance separates by 1.2, models use fewer passives than people,
and the list-versus-prose ratio does not discriminate at all. A `scrub-ai-tells`
skill and a `/scrub-ai` command were built from it.

## Fifty-fifth session (2026-08-06): a fuzz target and a look, both of which found something else

Session 54 handed over an empty "Blocked on you" and a five-item queue to pick
from. This session took the two cheapest and both turned out to be guarding a
defect nobody had guessed at.

**`config.SplitArgv` got the fifth fuzz target, and it failed in seven seconds.**
`FuzzArgvSurvivesTheBox` drives the whole path a command knob makes - split the
typed line, render it back into the box with `JoinArgv` and into the file with
`ArgvLiteral`, read both again - and asserts both round trips. Two defects:

- **A tab or a newline inside a quoted argument came back as the letter t or n.**
  `JoinArgv` borrowed `StringLiteral`, which renders TOML, while `SplitArgv` read
  a backslash the shell way (`\X` means X). The two languages agree on `\"` and
  `\\` and part company on exactly the two escapes that carry whitespace, so the
  disagreement was invisible until an argument held both a space (forcing quotes)
  and a tab. `JoinArgv` now has its own quoting whose only contract is that
  `SplitArgv` reverses it, and `SplitArgv` decodes `\n` `\t` `\r`.
- **A control character was written into the file raw, which TOML rejects.** The
  blast radius is the part worth remembering: `Load` does not lose one setting, it
  abandons the whole file and every unrelated knob falls back to its default. One
  pasted DEL in one text box would have silently reset Boris's config. Escaping is
  `\b \f \r` plus `\uXXXX` for the rest.

Both are pinned by name as well as by corpus hash, and both were checked against
the old `write.go` to confirm they fail without the fix.

**The inline ask panel does not need FR84's fold - it needed something else.**
Session 54 judged the fold out of scope there and asked for a look at a real long
body. The judgement holds, and for a reason worth writing down: the card folds
because a fixed-height window puts its fields out of reach, while the panel's
body is bounded at 260px with the controls under it, so they never move. The
demo fixture had no long-bodied ask, which is exactly why nobody had seen this;
it has one now (`webui-demo ask`, second item).

What the look found instead was worse than the thing it was checking for.
`.askwrap` had no `min-height: 0`, and a flex item's automatic minimum is its
content, so the panel kept its full height whatever the window did. The
transcript collapsed to nothing and the overflow came off the bottom, which is
the composer. At 1000x520 the reply box was clipped to a sliver with no Send
button and no hint line: the human could read the question and had nowhere to
type the answer. The panel yields now - its body is the only part that shrinks,
the composer never does - and it was seen at 1000x520 before and after, scrolled
inside its own body, and re-checked at 1180x860 to confirm nothing moved where
there was already room.

**The first fix for it was wrong, and only the screen said so.** Letting the
panel shrink is not the same as telling it where to stop. At 22pt in a 900x560
window the body reached zero and the controls kept going past the panel's own
border, over the composer and the status bar - worse than the clipping they
replaced. The obvious correction, dropping `min-height: 0` and trusting the
automatic minimum, does not work either: the panel's min-content includes the
body's text, so nothing shrinks and the composer goes straight back off the
window. Both were built and looked at before the shape that holds: the body
absorbs the squeeze first, and past that the panel scrolls as a unit, so nothing
ever paints outside it. Three build-and-look cycles, which is what this surface
seems to cost.

**Reaching the drop-down panel from a demo for the first time** (`webui-demo ask
panel`, added for it) showed a question there opening as a card - 470x258,
landing over the panel's lower half. `inlineRoutable` required the app window and
the panel did not count, which is the exact thing FR49 exists to prevent, in the
host nobody had checked. Raised as a question rather than fixed unasked, since it
changes where Boris's questions appear; he said to do what was best, so the gate
became "a host that renders conversations is on screen". The mechanic worth
keeping: the panel's reroute has to run after `slide()`, because that is what
clears the open flag - asked any earlier, the rule still sees a host and the
question stays in a panel that is no longer on screen.

**The showcase was settled the same way.** Delete the video, keep what is
reusable: `record.sh`, `take.sh`, `verify.sh`, `docs/recording.md` and
`docs/youtube.md` are gone, and `deck.py`, `perform.py`, `console.jsx`, `tour.md`
and `docs/showcase.md` stay because each is worth having in front of a room with
no camera anywhere. `perform.py` lost its recorder marks and its
recorded-monitor check. `make deck` turned out to be broken in the state this
machine was actually in - a venv holding a python and no pip made it skip the
venv build and then die on the pip line - so it gates on importing pptx now.

The lesson is the same one session 54 wrote down, arriving from the other
direction: neither defect was in the thing being worked on. One came from a
property test asking a question nobody had asked in prose, the other from putting
a real window at a size nobody had tried.

## Fifty-fourth session (2026-08-06): FR30 built, and four things only the screen said

The session opened on an empty queue with everything of substance blocked on one
answer, so the first move was to ask for all of them at once. Boris cleared the
whole list in two exchanges: collapse a flood into one stack card at 3 cards in
10s per agent (configurable); close FR84's other half with approach C; build the
argv settings control; drop the showcase video for good; and go ahead with the
daemon-down look at `webui-demo agents`. Nothing was left blocked.

**`webui-demo agents`, the last unseen surface, is seen and is fine.** Every
area, every state badge, the orphan lock with its Break lock confirm, the
abandoned shared claim, the wait chain, and an opened row's four blocks off the
canned fixture. The findings that cost time were about the harness rather than
the app, and both are now in [STATUS.md](STATUS.md): `import -window` captures
window-relative coordinates while `xdotool` clicks in screen coordinates, and a
click only reaches the page once the window has focus. Clicking at an unoffset
coordinate silently changed surface, which is the demo-mode version of the trap
CLAUDE.md already warns about on the real board.

**FR30 flood control is built** (`internal/daemon/flood.go`, kind `stack`,
migration 0013). Past its budget a session stops getting a card each and
everything over collapses into one warning-level stack card listing the burst,
newest first, nothing dropped. A question caught in a flood keeps its own item,
says "waiting on you" in the list, and opens as a real card by click or number
key. The budget is keyed on the SESSION rather than the agent name, because
every Claude session here calls itself `claude` and a name-keyed bucket would
let the first looping agent collapse its innocent neighbours. The one departure
from the written requirement is recorded in
[01-requirements.md](01-requirements.md): the warning FR30 asks for is the stack
card itself, not a second toast beside it.

**Three of the four defects this session found were invisible to the tests, and
all three came from running it.** The window refilled underneath an open stack
card, so a sustained loop collected a fresh budget every window - eighteen cards
a minute on the shipped defaults, which is not "calm survives buggy callers".
`agentbox dismiss <stack>` cleared the summary and left every notification under
it pending, because the sweep had been written on the card's own Esc and there
are three doors. And the number key promoted the right item and then put the
stack card back in front of it, so pressing 1 read as nothing happening except
"1 waiting" in the footer - the test that missed it had the stack in the queue
rather than on screen, which is not where a human is when they press the key.
The fourth was on the same screen: an answered row still read "waiting on you"
under a footer still counting one, because the entries were a snapshot taken at
collapse time that nothing revisited.

**FR84's other half is closed with approach C.** Past 240 characters of body the
controls come first and the reasoning folds behind one line ("?" or a click). A
diff is never folded - its body is the thing being judged, which is the cost the
mock named for this shape. Fixing it uncovered a defect older than the feature:
`.card` is `min-height: 100%`, so measuring the shell measured the WINDOW, and
once a card had grown it could never shrink again. It had stayed invisible
because nothing on a card changed height mid-life before. Two fixes, because the
first was only half: the measurement now takes min-height off for the read, and
anything that can make the card shorter asks for a re-measure, since under the
window's height there is nothing left for the observer to observe. Measured on
the deployed build: 230px folded, 384 open, 230 folded again.

**The two argv settings have a control.** `[editor] command` and `[speech]
command` are edited as the line you would type, quotes holding a path together,
and stored as arrays. Neither needs a restart, which is worth saying because the
carried note assumed both would. Exercised on the real Settings surface against
Boris's own config: his `speech.command` renders as itself and the form stays
clean, which is the round-trip check that mattered - a knob that normalised
differently would mark the form dirty on sight. Typing into the editor box
flipped the footer to "1 key to write · editor.command", and Revert put it back
without writing to his file.

**The showcase is dropped for good.** No re-record, no re-upload. `docs/showcase.md`,
`docs/recording.md` and `docs/youtube.md` carry a frozen banner instead of being
deleted; `tools/showcase/` is untouched, because deleting it is Boris's call and
he has not made it.

## Fifty-third session (2026-08-06): two clean fuzz targets, and the hook that made a whole surface unreadable

A leftover-queue session. Nothing was in flight and no field request was open, so
it spent itself on the two solo items session 52 left and on the one thing that
had been carried, unfixed, through six handoffs.

**Two fuzz targets, both clean.** `walkthrough.Parse` (9.0M executions) and
`BuildPayload` (3.1M). After session 52's target found a real overflow inside
thirty seconds, clean is the duller answer and still the right one to record. What
makes them worth having is not the panic check but the invariants, which are the
promises callers read off these two without ever testing: that an accepted spec's
citations are sliceable, that glossary marking **partitions** the author's text
rather than rewriting it (concatenate the runs, get the input back), that the
tallies match the lists under them, that no remark is dropped or doubled between
its step and `orphaned_comments`, that absence travels as `[]` and never `null`,
and that the payload marshals at all - a handback `json.Marshal` refuses loses the
whole review at the instant of submitting it.

**A cap that reads like a cap and is not one.** `Parse` checks the whole payload
against `MaxSpecBytes+MaxDiffBytes` and then the diff against `MaxDiffBytes`, and
nothing anywhere checks the spec's own half - so a spec carrying no diff may be
3 MB, three times what `MaxSpecBytes = 1 << 20` reads as. The worst case that
could be built (2.8 MB, 48 glossary terms of near-miss spellings) costs 940 ms in
`Parse`, nearly all of it the glossary scan, which is O(prose x spellings).
Documented rather than patched: the obvious check, `len(raw) - len(s.Diff)`,
counts a heavily-escaped diff against the spec's half and would start refusing
honest specs at the extreme.

### The Agents row detail, watched at last - and two answers nobody can reach

`Recent items` paints exactly as designed: twelve rows at the `agentDetailItems`
cap, newest first, each with kind, state and age. That was the point of the
sitting. The other two carried assumptions turned out to be the wrong shape of
question - they are not unseen, they are **unreachable**:

- *"This session has left the board"* needs `found: false`, but `Agents.svelte`
  closes the detail the instant the roster stops listing the row. The daemon's
  answer survives only in the sub-second race between the surface's roster copy
  and the daemon's map, or when the bridge call itself throws.
- *"Nothing behind it yet"* needs a row with no timeline, no signals and no items -
  and **every row on the board got there by announcing**, which posts the signal
  that fills the block. Watched on the hook-only row, which showed its meta and
  its one announce, correctly.

Both are right defensive code. Neither is worth chasing again, which is the actual
value of having looked.

### The thing six handoffs called a wording preference

The same screen showed why the blocks under the activity line had never been the
problem. Boris's PostToolUse hook wrote the raw Bash command through
`cut -c1-70` - and **`cut` truncates every line and drops none**, so a heredoc
arrived whole. One opened row rendered a Go test file as a single wrapping
activity line and a commit message as another; `Recent items` sat two screens
below the fold and `Signals` was off-screen.

That is not a wording problem, and calling it one for six handoffs is why it
survived. He authorised the fix in one line:

    jq -r '.tool_input.command' | tr '\n' ' ' | tr -s ' ' | sed -E 's/^(.{80}).+$/\1…/'

Collapse first, then truncate, and mark the cut. Watched on the board with the
last old-format entry still in the activity ring directly above the new ones -
three wrapped lines against one, and `Signals` back in the first screenful. The
ellipsis is what says a line was cut, which `cut` never did.

**The lesson worth keeping:** a note that gets carried across handoffs unchanged
is a note nobody has tested. Six sessions described this line; the first one to
open the surface it lands on found both a different cause and a different severity
than the description had.

## Fifty-second session (2026-08-06): the column goes quiet with the sign, and FR95 closes

The one answer session 51 left: while the hands-off sign is demoted for a
recording, cards queue instead of appearing and drain the moment it goes loud.
It was the biggest of the four because it was the first to touch the presentation
path, and the gate itself turned out to be the small part.

**The gate.** `displayableLocked` gained one term and lost nothing:
`!d.quiet && d.announceableLocked(it)`, where the new `announceableLocked` is the
old body (DND plus mute). The split is the feature: recording mode is NOT
do-not-disturb, because DND holds the chime and this keeps it - what he asked for
is a quiet picture, not a quiet machine. Control owns the mode and the daemon
owns the column, so the flip arrives by callback (`SetQuietSink`), which the fuse
needs anyway: it flips the mode from a timer inside control with no RPC in sight.

**Four things the one-line answer did not contain**, each found by asking what a
real take would look like rather than by reading the requirement again:

- **The chime is for the arrival that WOULD have taken the screen.** A loud
  desktop chimes once for a burst of five and queues four behind the first;
  chiming for each held card makes recording mode noisier than not recording.
  `wouldShowLocked` answers that counterfactual - urgent always yes, otherwise
  only if nothing announceable is ahead of it.
- **Urgent waits, and it must not lose its place.** It cannot preempt, because
  there is nothing on screen to preempt and putting a card there is what the mode
  exists to stop. So `enqueueLocked` inserts it at the FRONT instead, after any
  urgent already waiting. Without that, going loud shows the oldest build
  notification while a question an agent is parked on drains fourth.
- **The spoken line is held; the earcon is not.** `speak` is the loudest thing
  AgentBox does and it lands in the take, so it is said when the card reaches the
  screen. The mode is called quiet - that is the argument.
- **The progress window goes too.** A bar is not a card, but it is AgentBox on
  screen, and a long task reporting through a whole take is the one window
  guaranteed to be in the frame. One gate in `progressListLocked` covers all
  three call sites; the report keeps running and comes back where it got to.

**A card already on screen comes down with the strip.** He arms this a second
before he hits record, and whatever is up at that second is in the first frame
otherwise. It goes back to the head of the queue exactly the way an urgent
arrival displaces one - unless it is inside its undo grace, which resolves itself
in seconds and must not have its answer yanked out from under it.

**The race worth the paragraph, because it has no symptom.** Two flips (a hotkey
against the fuse) can each release control's lock and then be scheduled in the
other order, so the daemon latches `quiet=true` while the strip says loud: every
card held, nothing on screen to suggest it, no way back but a restart. The sink is
now serialized by its own mutex and reads the mode at its turn instead of carrying
the value its caller wrote, so whichever flip goes last through the gate reads the
truth that outlived the race. It is deliberately not `c.mu`: the sink reaches into
`d.mu`, and control's lock must never sit underneath that. The test that guards it
runs sixty concurrent flips and asserts the two sides agree; mutating the delivery
back to the pass-the-value shape makes it fail under `-race`.

**Watched on the deployed build, not read off the diff** (`66174032f1ae`): with
the sign demoted, three notifies left `wmctrl -l` with no AgentBox window at all
and `agentbox control state` read `quiet: the sign is demoted for 30m0s more,
3 cards waiting`. Going loud put the URGENT one on screen - "2 waiting" in its
footer - which is the ordering fix visible in one screenshot. A live `agentbox
progress` window vanished on `control quiet`, and the completion toast it raised
while demoted drained on `control loud`.

**What is deliberately not held:** a walkthrough, a document or an artifact
window. Those are one deliberate window somebody asked for rather than an
interruption arriving on its own, and queueing them would park an agent behind a
thirty-minute fuse for something the human is standing there waiting to see.

### Then Boris asked for robustness, and four things came out of it

*"Adversarially look for opportunities to dramatically improve robustness and
avoid different kinds of bugs and unexpected behavior."* What that turned into,
in the order the reasoning went:

**A panic on a timer's goroutine took the whole daemon down.** Every handler has
had a recover since the beginning and every timer has had none: a toast expiring,
an escalation replaying, an undo grace finalizing, the recording fuse, the pause
nag and four subsystem tickers all run where there is no Handle above them.
systemd puts the daemon back two seconds later, which does nothing for the agents
already parked on questions. One guard now wraps the callback - **not the loop**,
because a recover around a ticker goroutine would swallow the panic and end the
reaper with it, trading a loud death for a silent one. The test arms the fake
surface to blow up on the toast's expiry; without the guard it does not report a
failure, it takes the package's whole run down with it.

**The coverage rule got its validator**, which was the last open finding from the
standard's trial in session 49 and had been carried by three handoffs. Rule 49
asks for one traversal covering every changed line, and nothing checked it - the
capture log counted citations and never compared them against the diff. Now every
hunk is compared against every citation at create AND on every read (recomputed,
never stored: the spec is the only copy of the truth). Four decisions inside it:
overlap rather than containment, because a step citing the twenty lines around a
three-line change has stood on it; a pure-deletion hunk's span collapses to the
seam it left, or every review that removed code would read as incomplete; a
deleted FILE is not counted either way, because there is nothing left to cite and
counting it would teach an `out_of_scope` entry for every deletion; and no diff
means uncomputed rather than clean. Watched live on the deployed build against
this repo's own diff: *"4 of 5 hunks are not cited by any step"*, each named by
path and line.

**The frontend got a test runner**, on node's own `--test` and no framework - a
dist committed so a machine without npm can build should not start needing npm to
be tested, and the target says so and moves on when node is absent. `parseDiff`
is the first module in it: shared by the review card and the inbox detail, whose
whole promise is that the two agree, and nothing checked that either.

**And the fuzz target found a real one on its first run.** `FuzzParse` over
`internal/change`: a hunk header with twenty digits overflowed the hand-rolled
atoi and came back as a NEGATIVE line number, which then flows into every span
and slice built from the geometry - and the blast radius had grown that same
morning, because the new coverage arithmetic runs on every walkthrough READ as
well as every create. Clamped at 2^31, failing input committed as the corpus.
The frontend parser is told the same lie and reads it differently (enormous, not
negative, so it swallows the following files into one body); fixing that means
changing the render path, which is not worth doing blind, so STATUS carries it.

**And then the screen came back, so the two things that needed one were
finished.** The `quiet_hotkey` knob renders under HANDS OFF as "Recording key",
value populated and hint intact. Recording mode was watched with a run LIVE for
the first time: the strip demoted to `1920x4+0+0` (amber sampled at x=20, 600,
1200 and 1900), two cards held with `2 cards waiting` on the state line, and the
URGENT one on screen first when it went loud.

**The last piece was the one that needed eyes:** both diff parsers were ending a
hunk on the header's word alone, so a count larger than the body swallowed every
file after it. In the render that loses files silently; in the coverage
arithmetic it is worse, because the hunks it ate are hunks nobody is told are
uncovered. Both now end a hunk when a body line cannot be a body line (' ', '+',
'-', '\', or empty - an empty line stays context, because that is a stripped
trailing space and far more common than a lying header). Watched in a real
review card: a header claiming nine thousand lines, and `after-the-liar.go`
still rendering below it with its own count.

## Fifty-first session (2026-08-06): FR95 mocked, settled and mostly built, and three of its four measurements lied first

FR95 is "get the hands-off strip out of a screen recording". Its shape was already
settled in the entry - demoted to FR74's four pixels rather than hidden, and the
demoted marker gives up being top-most so a window can cover it - so this session
measured the mechanic, mocked it, had Boris settle four questions at the mock, and
built three of the four answers.

**The measurement came before the mock, and that was the right order.** The whole
shape rests on one thing: dropping the notification type is what makes the sign
coverable, and if GNOME's own top bar then drew over the four pixels, a demoted
marker would be no sign at all - the one wrong answer FR74 names. It does not: a
frameless 4px window at `+0+0` is visible over the top bar with no window type and
no ABOVE. So the mock could promise it rather than hedge.

**Three of the four measurements were wrong the first time**, each in a way that
produced a plausible answer rather than an error. All three are now in "Mechanics
discovered":

- **Mutter decorates a bare X11 window.** The first probe measured its own 30px
  title bar and reported the top bar covering the marker.
- **`import -window root` cannot see a fullscreen window at all** (Mutter
  unredirects it for direct scanout), so the same probe read "covered" and "not
  covered" depending only on the capture tool. `gnome-screenshot` sees the
  composited output.
- **A pre-map `_NET_WM_STATE_FULLSCREEN` is silently ignored**, so the "fullscreen"
  test window came up at `y=102` and covered nothing near the top edge. Twice:
  once with a raw window, once with Chrome's `--start-fullscreen`, which does
  nothing in `--app` mode. `--kiosk` and `wmctrl -b add,fullscreen` are the routes
  that work.

And one confound that is AgentBox's own: **while an agent holds the desktop, a
fullscreen window makes AgentBox open ITS marker at `+0+0`**, so any measurement
of the top edge mid-run is measuring AgentBox rather than the thing under test.

**The mock was driven headless before it went on his screen**, in Chrome over the
DevTools protocol: 35 assertions over every state, including "no option title is
squeezed into a column", which is what caught the two layout defects. The second
was the interesting one - **a class collision**. The recording-frame panel was
`.rec` and the badge modifier is `.tag.rec`, so the panel's `width: 320px` landed
on every RECOMMENDED badge on the page. Fully styled, wrong on screen, and
invisible to every behavioural check.

**Boris took all four recommendations**: a hotkey plus the same verb in the shell,
a mode that dies with the daemon and expires after 30 minutes, colour carrying the
pause on four pixels, and cards queueing while the sign is demoted. The fourth
question was the mock's own find: FR94's three-minute nag lands in the same
top-centre column the strip was in, so demoting the strip alone still leaves a card
in the shot.

**Built and deployed:** the mode in the daemon (`quiet`/`loud`, the fuse,
independent of FR94's latch), `Ctrl+Alt+Q` as the third grab beside the panel's and
the pause key, `agentbox control quiet|loud`, the demoted marker as a second
window treatment, and green on four pixels while paused. **Not built:** the cards
half, which is the biggest of the four because it touches the presentation path.

**The defect only the screen could find, again.** `x11.plain` was written to
decline the notification type and `_NET_WM_STATE_ABOVE`, and then `unlisted()` -
the next line - added ABOVE straight back as a post-map client message, the one
route Mutter honours. Everything read correctly and a kiosk-fullscreen window
still had four amber pixels sitting on top of it. That is three sessions in a row
where a hands-off surface passed every test and lied on screen.

**And one bug that was not a bug.** `wmctrl -lG` reports doubled coordinates on
this desktop: the strip at `+650+48` is listed as `1300 96`. It was read as a
placement regression, chased, and disproved by `xwininfo` and the pixels, which
agree with each other. The commit written against it was reworded to say what was
actually observed rather than what it looked like.

## Fiftieth session (2026-08-06): FR94 shipped, and the hotkey it shipped with was dead on arrival

Resumed from session 49's handoff. Everything in it verified clean on arrival -
tree in sync, deployed `35d1cf4d0e6e`, nothing pending, no locks, and none of the
three ghost rows it warned about still on the board. Queue item 1 was FR94, so
that is what this session is.

### The mock did its job, twice over

Built [mocks/fr94-pause-resume.html](mocks/fr94-pause-resume.html) before writing
any code, per the working rule: a stage with a fake desktop and a live strip, the
agent's side of the wire as a scrolling log, three candidate shapes for the paused
sign, and the entry's three open questions as options with their costs. Boris
drove it himself and pressed Settle it.

**He went against the recommendation on two of the four**, which is the argument
for mocking rather than speccing:

- The **inverted strip**, not the collapsed pill that was recommended. One shape
  to learn, and he accepted the cost (it keeps the top of the screen).
- **Finish the current step**, not park at the next atomic boundary.

The second needed one narrowing before it could be written down, and he took it:
finish the step *except* `type`, which parks between characters. `move`, `click`
and `drag` are all sub-100ms and stopping a drag half way is what leaves a button
held down, but a `type` step is a whole sentence at a set wpm - finishing it is
seconds of typing into whatever he just switched to, which is the opposite of
"suddenly need the keyboard".

The other two went as recommended: **no auto-resume ever** (it would take the
desktop back while he is still using it, the exact collision the feature
prevents), and a **desktop-wide latch** rather than a per-run one.

> **A mock is a shared desktop.** Driving the mock to check it worked while Boris
> was clicking in it produced twenty minutes of confusion: the page scrolled
> between my screenshot and my click, selections changed on their own, and one
> stray click emitted a decision he had not made. It was him, and he said so.
> CLAUDE.md already warns never to click a row at a coordinate read off an
> earlier screenshot; the missing half is that **the moment a mock is on his
> screen it is his**, and the agent's job is to get out of it.

### What shipped

Four commits, each exercised on the real desktop rather than read:

- **The latch** (`internal/daemon/control.go`). Desktop-wide, not per-run:
  `Pause`/`Resume`/`Paused`/`gate`, a `resumed` channel replaced on each pause so
  one resume wakes every parked caller, and a 10-minute waiting budget whose
  expiry says the run is still the agent's. `Request` waits on it too, so a
  paused desktop is never handed to the next agent when the parked run releases.
  A release under a live latch keeps the strip up and flips it to
  nobody-waiting - a strip that vanished there would say "agents may drive
  again" at the exact moment they may not.
- **The park** (`internal/hand`). A two-method `Park` interface rather than one
  blocking call, because `Blocked()` runs between every keystroke and has to be
  free on the common path. `Run` parks between steps and now returns how many
  actually ran; `Type` parks between strokes and **releases Shift first**, since
  a latch held for minutes with a modifier down is the stuck keyboard parking is
  supposed to avoid.
- **The strip's paused form** (`Control.svelte`). Green, `PAUSED - YOURS`, the
  frozen activity line in italic, a filled Resume button. At two minutes it turns
  amber and reads `AGENT WAITING`.
- **`Ctrl+Alt+Escape`**, and the `grab` helper in `cmd/agentbox/daemon.go` that
  now carries both hotkeys through open, rebind-on-reload and release.

### The hotkey was dead on arrival, and the log said it was fine

`Super+Escape` was the first default. It reads better than what shipped, the
daemon logged `hotkey.grabbed for=pause hotkey=Super+Escape`, and pressing it did
nothing at all.

**GNOME Shell takes every `Super` combination before a core X11 passive grab can
see it, and it does so silently.** `XGrabKey` returns success - no `BadAccess`,
so nothing anywhere reports a problem - and the key is simply dead forever.

Isolating it took firing a combination known to work through the identical path:
`agentbox drive "key ctrl+alt+grave"` toggled the panel, which proved XTEST
presses do trigger passive grabs and that the fault was specific to Super. A
throwaway probe then grabbed each candidate and pressed it at itself:

```
Super+F9             swallowed before the grab
Super+P              swallowed before the grab
Ctrl+Alt+Escape      FIRES
Ctrl+Alt+P           FIRES
Ctrl+Alt+space       FIRES
Ctrl+Alt+comma       FIRES
Ctrl+Shift+Escape    FIRES
```

Both the finding and the two techniques are in "Mechanics discovered"
(07-field-requests.md), including that a probe importing `internal/...` has to
live under `internal/` itself - a `main.go` in the scratchpad cannot compile
against it at all.

### What was verified on the real desktop

Not read off the diff:

- **The parked drive.** A `drive_desktop` sent through a fresh `agentbox mcp`
  child held for **21.45s** with the pointer frozen at `1228,85`, then ran both
  its steps on resume and landed on `900,640` exactly, returning `{"steps": 2}`.
- **The hotkey**, after the fix: one press paused a live run, the next resumed
  it.
- **All three strip states**, captured: amber `HANDS OFF` with a Pause button,
  green `PAUSED - YOURS` with the italic frozen line at `0s`, and amber
  `AGENT WAITING` at `2m 12s`.
- **The second agent.** A `request_control` from another identity blocked for 23
  seconds across the pause and was granted the instant of the resume.

### The defect only the screen could find

At `2m 50s` with the run released, the strip read **`AGENT WAITING` over the line
"nothing is driving; agents are held off"** - the escalation firing with nobody
to escalate about. The warm state's entire content is "somebody is blocked on
you", so an idle latch must never reach it. One line (`warm` now requires
`!idle`), and it is the second time in three sessions that a surface passed every
test and lied on screen.

### Also this session

**FR95, filed by Boris mid-build**: the strip should be hidable while he records
the screen. Settled in shape the same day - **demoted, not hidden** (it drops to
FR74's 4px marker), and the demoted marker **gives up being top-most**, so a
window over the top edge covers it. That relaxation also removes the one thing
FR74 could never make work: Mutter will not put a fullscreen window above a
notification window. It still needs a mock.

## Forty-ninth session (2026-08-06): the standard was tested on a real change, and it failed in five places

Resumed from session 48's handoff with nothing in flight. Three things it had
left unwatched were watched, and the experiment Boris asked for was run.

**The TL;DR control, on screen at last.** `1cd941e` shipped on a reading of the
diff. The review it was written for (`wfd51b30be854`) had been deleted from the
board a minute before this session started, so the disabled case needed a new
subject: a two-step probe review of only `ground` and `none` steps, which are
the two kinds the spec allows no `tldr` on. Confirmed - the TL;DR half renders
dim, `t` does nothing, a click does nothing, the legend drops the key, and the
hover title says *"no step in this review has a TL;DR - it was written before
they existed, or its author left them out"*. On a review that HAS them the
control is live and the board opens in TL;DR, and a step without one says *"No
TL;DR was written for this step, so it is shown in full"*.

**The Agents board's three detail blocks paint.** The largest untested thing
session 48 shipped. An opened row shows the meta list, an Activity block of the
lines the session has moved past, and a Signals block with direction arrows and
clipped payloads. The detail is fetched once per open, so a signal posted while
a row is open appears only after closing and re-opening it - which is the
documented design and is now observed rather than assumed.

> **The trap that cost twenty minutes:** the row toggles. Clicking it twice
> across two attempts opens and closes it, and a screenshot taken after the
> second click shows a surface that looks broken. The row's `.row.open`
> background is the same colour as `:hover`, so the state is invisible while the
> pointer is on it. Move the mouse away before judging: the row that stays lit
> is open.

**The walkthrough-standard experiment.** A review of `e511a01` (the Agents
detail work) was authored WITHOUT reading the standard, then audited against it
rule by rule, rewritten, and created: `w5bd381d0590a`, 12 steps, 4 domains, 30
citations, 0 missed, no warnings. Fourteen divergences, of which the ones worth
recording are these.

- **Prose carried the explanation and the margins were empty.** The standard's
  own preamble to the channels section names this as the most common way a good
  review reads badly. The first draft had 6 notes; the rewrite has 30.
- **No reading order, no handoffs, no `close` on any step.** Three rules
  (finish with a reading order; end by handing off; the takeaway goes under the
  code) that a competent author does not invent unprompted.
- **A command that matched nothing.** `go test ./internal/daemon/ -run
  AgentDetail` returns *"no tests to run"* and exits 0 - the daemon's tests are
  named for the rings, not the call. The rule that says to record only what you
  actually ran is what caught it.
- **No traversal.** Seven changed files no thematic step stood on, including
  both test files and every line of wiring.
- **`p: true` missing on seven paragraph-starting segments**, each of which
  would have rendered as a wall fused across the seam.

**What the standard got wrong, and what was fixed.** This is the point of the
exercise, and it found five things (`84e19ef`, `35d1cf4`):

1. **Rules 13-16 did not exist.** The "order the code" section was renumbered to
   29-32 when domains and the TL;DR were inserted, and the old numbers were
   never reused: 55 rules numbered up to 59, with a hole an auditing agent has
   to stop and account for. Renumbered contiguously, 1-56.
2. **The standard never named the fields it demands.** Rule 52 asks for "the
   command, the result to expect, and the date that expectation last held" and
   does not say they are `cmd`, `expect` and `recorded`. The first spec written
   against the standard was refused for an unknown field.
3. **`tldr` and `domains` appeared in no field reference an agent reaches.**
   The in-binary manual listed four easy-to-miss fields and named neither; the
   `create_walkthrough` schema description omitted both. An agent that read the
   standard and went looking for the shape found nothing.
4. **"Two to four domains. Six is the cap"** in one rule, with no way to tell
   which was the instruction.
5. **"Say what you did not verify next to the gate" is impossible as written.**
   A check step has no code blocks, and a step with no code blocks is refused a
   `close`. It goes in prose, and the standard now says so - along with the fact
   that ending with a gate AND giving doubts their own step means two check
   steps in a row, which reads as a duplicate until somebody says it is not.

Two more things the standard still does not cover, left as findings rather than
fixed: **nothing verifies rule 49's completeness claim** (`walkthrough.captured`
counts citations, and nothing compares the cited set against the diff's changed
set - the rule that most needs a validator has none), and **nothing says what
changes when the reader is the code's own author**, which is the normal case
here. A paragraph now says the rules assume the other reader.

**FR74's fullscreen marker, watched at last - and it is half right.** Built in
session 34, never seen. With the desktop held and a `gnome-terminal` fullscreen
and focused, an `agentbox · hands off marker` window maps at `+0+0`, `1920x4`,
amber (`srgb(189,144,60)`) at every column sampled across the screen, and it is
gone within a beat of leaving fullscreen. **The strip does not step aside**, so
a film would get the 620x62 card as well as the line. Not the arithmetic:
`planMark` returns `step: true` on one monitor and `beat` calls `x.lower(strip)`
on the transition. The strip is a NOTIFICATION type window carrying
`_NET_WM_STATE_ABOVE`, and Mutter layers notifications above a fullscreen window
whatever the X stacking order says - `xwininfo -root -children` shows the probe
above both agentbox windows while the strip is still the thing on screen. The
comment on `fullscreenActive` states the premise that fails: Mutter promotes a
focused fullscreen window above the ABOVE layer, not above the notification
layer. Left unfixed on purpose and put to Boris, because the failure is in the
safe direction (the guarantee is over-kept, not broken) and the fix - hide the
strip rather than lower it - risks taking the keyboard back on the remap, which
would be worse than a card over a film. **Boris: "leave it, it's the safe
direction."** What he asked for instead is FR94: a way to pause a hands-off run
and resume it, for the moments he suddenly needs the mouse. A run is binary
today, so the only thing he can do is reach for it anyway - which is the
collision FR74 exists to prevent.

**The `xdg-open` fallback, exercised.** Never seen before because this machine
has `goland` on PATH. Run against four real PATHs rather than a stubbed one:
`goland` (config absent, JetBrains first), `subl` (a lower-priority known
launcher, when only `/usr/bin` is visible), `xdg-open` with the line dropped
from the argv, and the honest *"no editor found; set editor.command"* when even
that is absent. Then the fallback's own launch was run the way `editor.Start`
runs it, through `systemd-run --user --scope --collect`: this desktop hands a
`.go` file to Zed (`dev.zed.Zed.desktop`), which opened at `1:1`. Worth knowing
that `zed` IS in the known table, so on a machine where its binary is on PATH
the line survives; it is only the desktop-handler route that loses it.

## Forty-eighth session (2026-08-06): a citation opens in the editor, and then the review learns two ways to be read

Resumed from session 47's handoff with nothing in flight and FR65 at the head of
the queue: the review board offered copy-the-path and copy-the-anchor and no way
to go there, so a reader left the board, pasted, and counted lines - on a file
cited across eight steps, the motion made most.

**What shipped.** `internal/editor` resolves an argv template and launches it;
`Bridge.BoardOpenInEditor` and an arrow beside copy in every block header.
The surface names a review and a repo-relative path, never a file - the root
comes from the stored walkthrough and `underRoot` refuses anything resolving
outside it, the treatment `OpenURL` already had.

**The trap that would have shipped invisibly.** `agentbox.service` is
`KillMode=control-group`, and a JetBrains Toolbox launcher *execs* the IDE rather
than forking it, so the IDE **is** the daemon's child and lands in the service's
cgroup: the editor a reader opened would die on the next `make deploy`. Only on a
cold start - with the IDE already up the launcher is a client that hands over and
exits - so it is exactly the case testing skips. The launch goes through
`systemd-run --user --scope --collect`, and it was proved by leaving a window open
and running a real deploy under it.

**Three more things the real screen said, none of them visible in the diff.**
Detection with no config at all picked `goland` and the caret landed on `141:2`,
the cited line. Opening a project GoLand did not already have raises a
This Window / New Window / Attach modal first, and the file then lands in a
background tab behind the restored session's own - the warm case (`378:2`,
routed to the open window, tab switched, window raised) is the one worth
promising. And a template naming a program that is not there puts
`editor "nosucheditor-fr65-probe" is not on PATH` in words beside the block,
which needed Boris's own config borrowed for a minute and restored by checksum.

**A defect found by using the new control, older than the control.** The board's
shortcuts bailed for INPUT and TEXTAREA and nothing else, so Enter on a focused
button ran the button AND the board's binding: the file opened and the board
jumped to the next unread step underneath the reader. Watched from step 2 landing
back on step 1, fixed, and watched again staying put while the log recorded the
open the same Enter had triggered. True of every button on the board since the
shortcuts were added.

**Deliberately not built.** No `[editor]` control in the settings tab: the value
is an argv array and the descriptor table has no kind for one, which is why
`speech.command` has none either. `$EDITOR` is not consulted, against FR51's own
proposal - it is usually a terminal editor and the daemon has no terminal to give
it, so honouring it would be a click that silently does nothing.

### Then the fix list, and three new asks

Boris: "Fix everything needs fixing." What that turned out to mean:

**The store was losing three things.** Migration 0012 added `session_key`,
`speak` and `diff` to items. The first is the only identity that names ONE
session - the agent/project/session triple is shared by every Claude session in a
repo - and without it the Agents board could only match a row to its neighbour's
items. The other two were written into FR73's read-back and taken straight out
again when the insert turned out not to name them; they are readable now, and the
unified-diff parser moved to `lib/diff.js` so the card that asks for a review and
the detail that reads it back use the same one.

**The Agents board's three detail blocks were rendered by demo.go and by nothing
else.** They are real now, assembled per opened row rather than pushed - the
roster goes out once a second while anything moves. Three owners meet in the call
and none learns about the others: the roster gained a bounded ring of the
activity lines a session has moved past, the signal hub gained a ring of what a
session HEARD (the store cannot know - a signal is fanned out by meaning and one
row is read by every listener), and the store answers what it posted and raised.
An empty history is now a sentence, because three missing blocks read as a
surface that failed to load.

**FR93, mid-session, from two urgent cards on his screen.** Esc deferred a
notification, escalation raised it again every 20 seconds, and ⇧Esc was the only
way out and was written down nowhere. Esc dismisses a notify now.

**FR91 and FR92 came as asks while the rest was in flight**, and both are in
07-field-requests.md in full. The two things worth repeating here:

The TL;DR is not the lossy version - "not necessarily less exhaustive, but
optimally structured for a person with a very short attention span that must
still get a mastery level of the most important aspects". That one sentence
killed the free-text field and the total character cap, and left a shape:
`bottom` plus up to six standalone `points`, capped per point.

And the CSS lesson that cost the most time in the whole session. A segmented
control rendered as two bare words in the header while being fully styled in the
stylesheet. Moving it into its own component fixed it, and every `var()` in that
component now carries a fallback: **a var() that resolves to nothing takes its
whole declaration with it, and a control that has silently lost its background
still reads as working to everything except the screen.** Three build-deploy-look
cycles went into believing the diff over the pixels.

## Forty-seventh session (2026-08-05): the last of Boris's own field requests, and the store could not keep two of its promises

Resumed from session 46's handoff with instructions to skip the queue triage and
go straight to FR73 - the one where he missed a card, went to the inbox to find
what it said, and could not. Its research was all in the handoff, and every line
of it held: `wireItem` carried a 140-character `Snippet` and nothing else of the
body, `StoredItem` already held the rest, `inbox.rows` was the lookup, and the two
`{@html}` allowlists both had to learn a new field before it would ship. Nothing
in the live state had drifted.

**What shipped.** Clicking any row opens a detail under it: the body through
`RenderMarkdown`, the same renderer the card used; both timestamps to the minute
plus how long the item stood; what it offered, with the default and the taken
option marked separately; a form's answers in the order its fields were asked
rather than its map's; the identity, its hue, the session key and the id. Nothing
is clamped or ellipsised. `Bridge.ItemDetail(id)` is a call per opened row, not a
field on the snapshot - a hundred rendered bodies in every push is what the
snippet exists to avoid.

**The FR said "everything needed is already stored", and it was nearly right.**
`speak` and `diff` were written into the wire type, and then taken back out: the
store's items table is `id, kind, level, title, body, options, fields, actions,
cwd, timeout_s, dflt, agent, project, session, state, created_at`, and neither is
a column. `RecentItems` reads that table, so both fields would have been empty in
every real read - a reader promising two things the read behind it cannot deliver.
Found by checking the insert statement rather than the struct: `proto.Item` has
`Speak` and `Diff`, so the code compiled, the tests passed, and only the schema
said otherwise. A resolved review's diff is still unrecoverable, and that is now a
named gap in STATUS instead of a field that looks implemented.

**Two things the change itself broke, both found before shipping.** A row used to
be inert unless it was pending, so nobody had reason to click into the list and
then type; with every row opening, reading one row and pressing `d` would have
dismissed whichever row `j/k` was last left on, so a click now moves the triage
selection too. And a row can change under an open detail - answered on its card,
or triaged from the keyboard - which left the detail saying "waiting" and offering
a card for something already answered; one effect now owns both that and the row
leaving the list entirely.

**The screen was locked for the whole build**, which is the first time that has
been the binding constraint here. `LockedHint` was `yes` from the first check to
roughly two hours later; `import -window` on a locked screen photographs the lock
screen, and there is no Xvfb on this machine, so there was no off-screen path
either. What the wait was spent on: the docs, and one guard worth having - every
`svc("Name")` in bridge.js must appear in the shipped bundle, because a Bridge
method is where the committed-`dist` trap is both silent and fatal (the surface
resolves it by name at runtime, so a stale bundle compiles and fails when the human
clicks). Verified by naming a method the bundle cannot have and watching it fail.
The fixture was also built during the wait, which turned out to be the right shape
rather than a workaround: a veto raised and expired while he was away IS the case
he filed.

**Everything was then exercised on the real screen** - the veto's whole body read
back after it closed, a choice's options with the default and the taken one marked,
a pending row's `Show the card` clicked through to the card, and the detail
re-reading itself when that card was answered. The captures are in FR73's entry.

**Session 46's layout trap came back wider.** Its version was "clicking a row you
already opened closes it and the layout moves". The first click of this run went to
the wrong row without any of that: two pending items were answered by Boris in the
gap between the screenshot the coordinate came from and the click, the Pending
section collapsed, and everything moved up. Any queue change does it, not just
one's own click. Typing a term into the search box first - so the target is the
only row on screen - is what made the rest of the run repeatable, and it is the
better habit for driving this surface at all.

## Forty-sixth session (2026-08-05): the identity colour was two colours, and each fix found a second one

Resumed from session 45's handoff with FR83 finished and its deferred queue in
order: FR85 with FR86 first, then the board's small dead ends. Nothing in the
handoff's live state had drifted except a doc commit landing after it was written,
and one line of its own solo list contradicting its own facts section (the
hook-announced row was photographed twice in session 45; the list said it never
was). The repo won.

**FR85 and FR86 are one bug with two entrances, and both had a second defect
behind them.** The identity hue had two implementations that had never agreed:
Go hashing `agent + " " + project`, the frontend hashing the two around a literal
NUL. Taking Go's separator fixed the split and made tokens.js text again - the NUL
was why `grep -rn identityHue frontend/src` had come back empty, so the second
implementation was invisible to every search for it, which is how a colour bug
survived in a file nobody could find. Then pinning both sides against a table
showed the divergence nobody had looked for: the frontend hashed UTF-16 code units
where Go hashes UTF-8 bytes. Identical for every ASCII identity, and a different
colour for the first project directory that is not one, which on this machine
(Hebrew paths, accented names) is a matter of time rather than a hypothetical.

FR86's second defect was smaller and the same shape. `filepath.Base(cwd)` named
an agent in `frontend/src` the project `src`; `DeriveProject` now rides the walk
`deriveArea` already did. The exotic cwd (`/`) then reports no project at all
rather than the project `/` - which is correct only because `betterIdentity` reads
an empty project as "say nothing" instead of writing it, and that was worth
checking before shipping rather than after.

**Both were demonstrated rather than argued.** From `frontend/src` against the
running daemon, the old binary announced `sleep · src` and the new one
`sleep · agentbox`. For the colour, a toast's pill and the inbox row for the same
item were sampled off the screen at `hsl(225 62% 68%)` each, where the frontend
would have painted stop 30 before this build. Pixels, not "looks right".

**The tests are three-sided on purpose**, because each side alone would have
missed this: a fixed table of eight identities pins Go, node runs the frontend's
own function over the same table (skipped where there is no node), and the shipped
`dist` bundle is checked for the NUL so a fix that was never rebuilt fails
`make check` rather than on Boris's screen. The cross-check was itself verified by
breaking the separator on purpose and watching it name both sides - a green test
that cannot go red proves nothing.

**Two board dead ends, from the handoff's list.** A blocked row said "blocked:
lock X, held by Y" in its chip and "waiting on X for 20s, held by Y" on the line
under it: the same fact twice, trimmed on the surface only, because `sync agents`
has no second line and the CLI still needs the detail. And a shared value
highlighted on hover and did nothing on click; it opens now to the full value and
its owner, with a jump to the owner's row, while a row with nothing more to show
stops highlighting at all - the highlight is the surface's promise that a click
does something. The row is a real `<button>` when it can expand and a plain div
when it cannot, so Enter and Space come from the element rather than a keydown
handler.

**Then Boris unlocked the screen, and the looking began.** Every claim above about
the board was checked on it, and the two board fixes cost three more commits before
they were true. Opening a shared value widened the whole window and pushed the
Break lock buttons off the right edge; `min-width: 0` on the block did not fix it,
`word-break: break-all` did not fix it, and the actual cause was two files away -
the shell's grid gives main a `1fr` track, whose automatic minimum is its content's
min-content width, so ONE unbreakable token in ANY surface could widen the window.
Latent for every surface since the webview port; found by a JSON value with no
spaces in it.

With that fixed, all four things were seen: the blocked chip reads "blocked" with
the wait line under it carrying lock, age and holder; the opened value wraps inside
its row; a short unowned row does not highlight under the pointer (sampled at
`#161920` against the hovered row's `#1c2028`); and the identity colours agree.

**FR84 was answered by him, not decided for him.** A live artifact showed today's
card and three approaches - radio list, select with the choice spelled out under
it, radio list with the controls above the prose - and he picked the second,
including its stated cost (you read the option you already picked, not the ones you
did not). Shipped and watched: a three-field form opened with every control above
the fold, and Tab-then-Down onto a longer option grew the window from 470x274 to
470x309 with the line wrapped to three. Its other half is deliberately unbuilt: a
long body still pushes fields down a scrolling card, which is what the approach he
did not pick addressed.

**The usage assignment was already retried** - session 45's handoff was stale about
that, its ok run predates the note - but the run had found a real defect nobody had
acted on: the prompt asked for a `qq-data` block, which is a tag AgentBox has never
parsed (rename fallout), and it had read a 14-hour-stale cache because it believed
`claude -p /usage` could not answer headless. It can, and this session had already
used it. Both fixed in the prompt, and one run proves it: live figures, `source:
"claude -p /usage"`, and a captured data block.

**Two things about looking, both paid for here.** The screen was locked for the
middle of this session, and the first capture found that out by photographing the
lock screen - an all-grey window with a clock where the board should have been.
`LockedHint` was `no` at 13:30 and `yes` at 13:42, so checking it once at the start
of a session is not checking it. And an earlier capture hit a recycled window id and
photographed an unrelated window of Boris's, which is why every capture after it
verified the window's NAME immediately before the shutter instead of trusting an id
from a listing a second earlier. The stray file was deleted and he was told.

## Forty-fifth session (2026-08-04): FR83's last slice was documentation until it wasn't

Resumed from session 44's handoff, which said all four sync primitives were built
and only slice 5's teaching remained. Slice 5 was the item four sessions had left
alone because it looked like prose: three of its four doors were already open, and
the fourth was a hook recipe sitting in [recipes.md](recipes.md) that nobody had
ever installed. It had been this repo's oldest `[assumed]` since session 40.

**Installing it showed the recipe could not work.** It told you to export a random
`AGENTBOX_SESSION_KEY`, and there is no moment in which to export one: a hook runs
inside an environment Claude Code has already built, so a key a user can reach is
either too late or shared by every session on the machine. Claude's own session id
looked like the bridge, and the check that killed that idea was reading two
environments side by side - this session's mcp child carried `60bb7c0f`, its own
shell carried `bd75f814`, because a child keeps the id it was spawned with and
`/clear` mints a new one. Either way the hook writes a SECOND row, and two
half-stale rows read as two agents on the one surface whose job is saying how many
there are. That failure was then watched happening: an announce run by hand put a
duplicate of this very session on the board.

What both mouths of a session DO share is the process that runs it. The key is now
derived - `proc-PID-STARTTIME`, off the walk `agentName` already did - so the child
and the hook arrive at the same answer without either being told. The start time is
not decoration: pids are recycled, and a new process landing on a dead agent's
number would otherwise inherit its locks and its claims. Minting survives as the
last resort. `inheritedSessionKey` may return empty where `sessionKey` may not,
which is the point: a CLI call belonging to no session must be refused, where a
random key would put a phantom row on the board once per invocation.

**Both halves of the acceptance were run for real, and neither is visible in a
diff.** A fresh `claude -p` in a scratch directory that had never heard of AgentBox
appeared on the roster within one second, no instruction and no token spent. Then a
session in this repo, forbidden in its prompt from using any tool, answered "two
other agents are in this directory with me", named this session's purpose, and said
how it knew: "this session's startup hook auto-announced me and handed back the
roster of peers in my area". Learning without looking, because a SessionStart hook's
stdout is context. A third session, this one calling `announce` itself, kept one row
rather than two.

One thing that looked like a defect and was not: a hook-announced row reads
`[detached]` with no pid behind it, which seemed to mean a row nothing could ever
retire. `provisionalFor` already retires an unattached row after ten minutes - the
design had thought about it, and the ten-minute comment says why.

**Then the board was looked at, and it had a line that slice 5 made false.** A
detached row read "last seen through an item", written when a card was the only way
a keyless row could appear. A SessionStart hook is now the usual way, so the line
was about to say something untrue about nearly every new session on the machine. It
reads "announced on its behalf" now, which is true of both a hook and a call by
hand. Five of five slices have now found something by looking at the surface, and
this one was found in the first screenshot.

One thing that looked like a second defect and was not: this session's own row came
back from each deploy on its ANNOUNCE-time activity rather than the line
`set_activity` had moved it to, which is FR87 exactly - fixed in session 42. The
explanation is in the timestamps: this session's mcp child started at 16:49 and the
FR87 fix landed at 18:59, so the child runs a binary with no `rememberActivity` in
it. **Your own row is not evidence about a fix younger than your mcp child.**

The lesson worth keeping is about the shape of the work, not the key: **a door
nobody has walked through is not a door.** The three open doors had been opened by
writing prose. The one that needed a program to run was wrong, and had been wrong
for four sessions, in a repo that writes down every trap it pays for.

## Forty-fourth session (2026-08-04): shared values, and an ownership check that a daemon restart caught

Resumed from session 43's handoff, which said slice 4 was next and nothing gated
it. It was right, and it was the last slice: **all four sync primitives now
exist.** The handoff's four facts to build on all held - the signal hub was the
door to post through, the store was at 0009, the trim/gap pattern was the one to
copy for owners, and any verb needing an area must carry the cwd.

**Slice 4 shipped: shared values, the compare-and-swap blackboard.** One `shared`
tool with `op: get | set | delete`, `agentbox sync get|set|del` beside it,
migration 0010, and `shared_max_bytes` in `[sync]`. A lock says whose turn it is
and a signal says something happened; neither can say *chunk 7 is mine*, which is
the fact a fanned-out job needs before it starts work a peer already started. Ten
workers over ten `claims/<chunk>` keys is first-writer-wins per item, with no lock,
no read-modify-write and no retry loop.

**Zero is a value here, which is slice 3's decision used the other way round.**
`after_seq` could fold "omitted" into 0 because nothing needed to demand a zero. A
claim is exactly that thing: versions start at 1, so `if_version: 0` means "only if
this key does not exist yet". So `if_version` is a pointer on the wire and a string
flag on the CLI - an int flag cannot tell "0" from "not given", and the difference
is a claim versus an overwrite. Worth reading the two slices together: the same
fact about where a counter starts, and the opposite conclusion.

**Every CAS is one SQL statement, and that was a deliberate refusal.** Read-then-
write inside a mutex would have been atomic - because this daemon happens to be one
process holding one connection pool. That is the kind of true a dev instance, a
second daemon or a future migration tool breaks silently, so all three conditions
are predicates on the write itself: an upsert, an `INSERT ... ON CONFLICT DO
NOTHING`, and an `UPDATE ... WHERE version = N`. `RETURNING` is what turns SQL's
silence about a losing write into an answer. Verified with eight goroutines over
ten keys under `-race`: exactly one winner per key.

**The ownership check was wrong, and only restarting the daemon showed it.**
Ownership was recorded (session key, agent name) and checked against the live
roster - which reads correctly all day and is false for one second per daemon
restart. The roster is memory only on principle, so a restart empties it, and until
every mcp child redials and replays its announce, **every owned claim read as
abandoned**. That is an invitation to take over a chunk somebody is actively
writing: the exact failure this primitive exists to prevent, and migration 0009's
lesson a third time - "gone" cannot be told from "not here yet" by looking at what
is left.

Migration 0011 records the owning process, and a read answers in two steps: on the
roster means alive, otherwise the pid decides. A pid is the one fact about an owner
that outlives the daemon, which is why an orphaned lock is pid-checked too. Zero
means none was recorded, which is honest for a CLI write - a shell knows only its
own pid and that pid dies seconds later, so recording it would manufacture the
false orphan the column removes. Found by writing the acceptance probe, not by
reading the diff, which is now four slices out of five.

**A probe that runs the drainers one after another proves nothing.** The first
draft of `tools/sync-probe.py shared` walked three sessions through the ten keys
sequentially, and session A won all ten - every key was free when the first walker
reached it. Three threads, two going forward and one backward, put the collisions
in the middle. The run then settles the whole acceptance sentence live: 10 of 10
chunks claimed with no doubles, a session killed holding four claims whose keys
read as ownerless and name the agent that abandoned them while the live sessions'
keys do not, a peer parked on `shared:probe:claims/*` woken by the take-over with
the key and version but no value, a real `systemctl --user restart` after which all
ten claims survive with their versions and the live owners still read as live, the
table drained, and the CLI exiting 0 on a won claim and 1 on a lost one.

Unlike the signals scenario, this one cleans up after itself: shared values are
never trimmed by design, so every probe claim is deleted on the way out.

**Retention is the one place shared values deliberately differ from signals.**
Nothing here is ever trimmed. Retention exists because an event is history and
history may be forgotten; a claim is not history, and dropping one hands a chunk to
two agents. So values leave when an agent deletes them, and the cap on how many may
exist (1000) refuses a NEW key rather than evicting an old one - while never
blocking an update, because refusing those would strand a claim its owner is trying
to finish.

**The surface came after the tools, and looking at it paid twice.** The design's
prose promised "the surface and `shared` reads report a value whose owner is no
longer present", so the blackboard got a block of its own beside the lock table -
global state rather than per-agent state, abandoned claims sorted to the top, the
abandoned count in the heading in the warning colour. Nothing else on that board can
say a claim was abandoned: the lock table cannot, a claim not being a lock, and the
agent's row cannot, the agent being gone.

Two defects a real screen turned up. The key and the value were touching
(`claims/chunk-7{"worker":"aider"}`), and the heading read "1 abandoned 4", which
parses as one number. And one hole that was not mine: an empty roster hid the
blackboard AND the orphaned locks behind "No agents attached" - the state where
leftover coordination state matters most, and the same hole the orphan block had
carried since slice 2.

**The live board also demonstrated the pid fix in the field.** A deploy restarts the
daemon, and one frame afterwards showed the roster rows healed to 21 seconds old
while the claims kept their true age of three minutes: presence does not survive a
restart, coordination state does. Two live claims still read as live, and the
abandoned one still read as abandoned, from a daemon that had never seen either
session.

**Two clicking mistakes worth not repeating.** `xdotool mousemove` takes SCREEN
coordinates while `import -window` captures window pixels, so clicks computed off a
screenshot land wherever the window happens to sit - this window was at (370,170),
so two "clicks on a row" both hit the sidebar and navigated to Library, and the
"nothing happened" that looked like a passing test was a click into empty space. Get
the origin from `xdotool getwindowgeometry` and add it. And `pkill -f board-shared.py`
killed the invoking shell with exit 144, which is the trap CLAUDE.md opens with, hit
in the session that quotes it - the same way session 43 hit it. Kill by pid from
`ps -eo pid,args`.

Also this session, at Boris's request: a **global usage-budget rule** in
`~/.claude/CLAUDE.md`. Every session periodically reads `claude -p /usage` and
hands off when the weekly limit is above 98%, or when the session limit is above
98% and its reset is more than 20 minutes away - inside 20 minutes, waiting it out
is cheaper than a handoff. Running out of quota mid-task ends a session wherever it
happens to be, which is the one way work gets lost.

## Forty-third session (2026-08-04): signals, and a gap check that only running it caught

Resumed from session 42's handoff, which said slice 3 was next and nothing gated
it. It was right: the measurement that gated slices 2 and 3 was done, the
keep-alive ticker was already shipped, and the acceptance list read straight off
the design.

**Slice 3 shipped: signals.** `post_signal` and `await_signal` as MCP tools,
`sync post` and `sync await` from a shell, one global sequence as the only cursor,
per-topic and by-age retention, and the built-in `agents:<area>`, `to:<key>` and
`lock:<name>` topics. What it buys is the shape the whole feature was for: "deploy
when the tests are green" is `await_signal(["tests:green"])` then
`acquire_lock("deploy:agentbox")` - two calls, no poll loop spending a model turn
per look, and the chain visible on Boris's board while it happens.

**The design's gap check was wrong, and reading the diff would never have shown
it.** The rule as written - a cursor has fallen off the edge if it is below the
oldest surviving sequence - is obviously correct and false. Retention is *per
topic*, so one quiet topic's ancient row holds the global minimum down while the
topic a caller actually asked about is trimmed away underneath it. Found by
running the thing against a real daemon with `signal_keep = 1`: cursor 1, oldest
surviving 1, sequences 2 and 3 gone from the awaited topic, and the batch came
back reported as complete. That is the silent hole FR61's rule exists to close,
in the one place the design says it must never appear - a batch that skips what
retention ate is how two agents both come to own one chunk of work.

The fix is a recorded fact rather than a cleverer deduction, because "trimmed"
cannot be told from "never existed" by looking at what is left: migration 0009
keeps the highest sequence retention took from each topic, written by the one
function that deletes, and the row outlives the topic's own signals so a family
aged away whole can still be answered for. Per topic and not one global number -
`agents:<area>` is the chattiest topic on this machine, and a global watermark
would report a gap to every unrelated stale cursor, which is how an agent learns
to skim the one answer here it must never skim. Two migrations in one slice is the
honest record of that: 0008 had already been deployed when the live run found the
hole, and migrations are forward-only on principle.

Three smaller things the build settled:

- **The channel is a doorbell, not a delivery.** A woken waiter re-reads the store
  from its cursor instead of taking a payload off the channel. That is what makes
  the batch a batch, it is why a buffer of one is enough for the daemon's first
  multi-consumer hub, and it makes a wake that races a trim harmless - the waiter
  finds nothing and keeps waiting rather than returning an empty batch that would
  read as "the event happened".
- **`after_seq` needs no third state.** Sequences start at 1, so zero *is*
  omitted, so "from now on" and "everything I have not seen" fit in one integer
  with no pointer and no companion flag.
- **The `listening` chip had been on the surface since slice 1 with nothing
  feeding it.** It came from the mock and no daemon ever set it. It does now, and
  it sits below `blocked` deliberately: blocked means an agent cannot proceed and
  somebody else is why, listening means it is waiting to be told and that is the
  feature working. Photographed side by side, because they look alike and mean
  opposite things - and a listening row holds its state rather than decaying to
  `quiet` after 90 seconds, which is the whole "a parked agent must not look like
  a hung one" case. The row beside it in the same photograph had decayed, which is
  what makes the difference visible.

**A slice-1 defect fell out of verifying slice 3, and reading it as noise was the
tempting mistake.** The rider probe - slice 1's, passing all day - started failing
intermittently once signals shipped, and the honest first guess was interference
from another live agent on the same roster. Two consecutive runs failing
*differently* said otherwise. An area is derived from where a session stands and
only the attach carried a cwd; the attach is lazy, so an `announce` arrives first,
always for a hook announcing on a session's behalf. The row then had no area, and
three things followed that nothing on the board showed: it was invisible to every
area-filtered read (so another session's `announce` could answer `alone: true` with
a peer sitting in the same repo - the one claim the design says must be true when
made), its rider cursor was never initialized (so its next call repeated the whole
area as news), and `peersOf` with an empty area filtered nothing and could return
every agent on the machine. FR90, fixed: the announce carries its cwd, the daemon
derives the area there too, and an unknown area now answers "cannot say" rather
than "everybody". Slice 3 did not cause it - it made the attach fractionally slower
and turned a latent race into a flake often enough to chase.

**FR89 got built at the end of the session because he asked twice.** The same
probe-generated "Deadlock refused" toasts came back, and the honest reading of that
is not "let me explain why" - it is that a defect whose only symptom is "the human
keeps having to click something" had been sitting on a solo-work list for a session
while I generated three more of them. Shipped: `agentbox dismiss ID... | --all` for
him, the `retract` tool for an agent (its own items only, because retiring another
agent's question answers it for them), and `agentbox pending` for the ids, which
dismiss-by-id is unusable without. Items had four doors in and none out.

The cause is fixed as well as the symptom: `tools/sync-probe.py` diffs the pending
queue around its run and dismisses exactly what it caused. The diff rather than
`--all` matters twice over - a sweep would clear a real item of his that happened to
be waiting, and the deadlock warning is posted by `agentbox` rather than by the
probe's own sessions, so an ownership-scoped retract cannot reach it. A warning
about agents is the daemon's word, so the harness that provoked it cleans up through
the human's door.

Verified live against the deployed daemon (`tools/sync-probe.py signals`, PASS):
a parked wait woken by a post, two waiters on one topic both woken by one, a
signal fired with nobody listening picked up afterwards by cursor, a timeout
leaving the cursor where it was, a request addressed over `to:<key>` with `@me`
expanded by the child, a departure and a lock release arriving as signals, and the
board reading a parked session as listening. The gap path was proved separately
against a throwaway daemon with retention turned down to one, because the live
store still holds sequence 1 and has nothing to confess - and the probe says which
of the two cases it exercised rather than printing PASS either way.

## Forty-second session (2026-08-04): locks, and the 30-minute fuse under every card

Resumed from session 41's handoff. Its "do this next" was one measurement before
slice 2: the MCP client's tool-call idle cap, the last guessed number in FR83's
design. Measuring it first turned out to matter for a reason nobody had guessed.

**The measurement was a shipped defect.** Claude Code aborts a stdio tool call
that has said nothing for **1800s**, and nothing in the mcp child ever said
anything - so every blocking card was already dying at half an hour while it was
still on Boris's screen. He answers at minute 40, the answer goes to a caller
that is gone, and the agent gets "sent no response or progress for 1800s;
aborting" instead of an answer. `timeout_s: 0`, documented as "waits forever",
was the worst case rather than the safest. Never seen in the field because nobody
had left a card up for half an hour and then answered it - which is exactly the
case AgentBox exists for. FR88, fixed with a keep-alive ticker in receiving
middleware: one progress notification a minute while a call is parked, nothing at
all until a call has already lasted a minute.

How it was measured is the part worth keeping. The cap lives in the *client*, so
no probe written as an MCP client can find it: `tools/idlecap-probe.sh` drives
two headless `claude -p` sessions against a throwaway MCP server
(`tools/idlecap-server.py`) whose only job is to park - one silent, one ticking
progress. The silent one died at 1800s with the client's own message; the ticking
one ran 2100s and returned normally. Two mechanics fell out that the design had
wrong: the client sends `_meta.progressToken` on **every** `tools/call` (so a
server may always keep a call alive), and it **does not tell the server it gave
up** - no cancellation, no closed pipe - which is why a parked daemon call needs a
ceiling of its own.

**Slice 2 shipped: locks.** Acquire (blocking, FIFO), try, release, orphaning
with a pid probe, break, deadlock refusal, stall warnings, the holds and waits on
the Agents board, and the CLI for Makefiles and hooks. Five things the build
changed in the design, all recorded in [09-sync.md](09-sync.md):

*The Makefile wrap the design asked for cannot exist.* `make deploy` stops the
daemon halfway through, and locks are memory only on purpose, so a sync hold
vanishes mid-install and hands the second agent a green light in the worst
possible second. The one resource this daemon cannot arbitrate is the daemon;
deploy takes an flock instead.

*A wrapped hold named the wrong process.* The lock must be taken before the
command starts, so the only pid it can record then is the wrapper's - and a
killed wrapper then looks like finished work while the command it started runs
on. Found by trying to satisfy the design's own acceptance case rather than by
reading the code. The wrap re-points the hold at the command once it exists.

*The two subsystems' lock order is a rule.* The lock table asks the roster who a
holder is while holding its own mutex, so the roster reads every observer before
taking its own. The other order deadlocks the daemon on the first board repaint.

*A lost hold needs no signals.* "The human broke your lock" was designed to ride
a `lock:NAME` signal, which is slice 3; it rides slice 1's discovery rider
instead, so it shipped with the locks. Verified live: the line arrives on the
ex-holder's next `set_activity`.

*The mock's break had to go.* With real holds on screen, a faked break was the
only untrue thing left on the surface.

**Verified by looking, again.** `tools/sync-probe.py locks` drives the whole
acceptance list with two live mcp children; `tools/sync-probe.py board` holds a
holder, a waiter and an orphan on screen long enough to read. Both were run
against the deployed daemon, the board was photographed, and Break lock was
clicked for real (desktop taken with `agentbox control request` first): the
two-step confirm reads "Reassigns the lock. It does not stop the process.", and
after confirming, the orphan block cleared and the board repainted from the
daemon's own push rather than from a local edit.

One test bug worth remembering: a `t.Fatalf` while a tool handler was still
parked deadlocked the SDK session's `Close`, so a broken keep-alive hung the
suite instead of failing it. The parked handler is now released in cleanup.

**FR87 fixed on the way past.** Every `make deploy` in this session rewound every
row on the board to whatever its announce first said, an hour-old line stamped as
fresh - so the child now remembers the newest `set_activity` and replays the
announce carrying that. Verified across a real daemon restart. Its two limits are
recorded with it: the age restarts (nothing survives a restart to date the line
from), and a line a hook sent never passed through the child, so the child cannot
replay it.

## Forty-first session (2026-08-04): looking at the board found four defects, and the rider shipped

Resumed from session 40's handoff, whose one blocked item was that nobody had
seen the Agents surface with real data - his screen had been locked when it was
deployed. It was unlocked this time, so the first act was to put four real rows
on the roster and look.

**Looking found four defects the tests could not.** Every one of them was a
visible lie on the human's board:

*A row never stopped saying `working`.* State is derived when a push happens, and
every push is caused by a verb, so a session that stops reporting causes nothing
and keeps its chip. Photographed: the header read "3 working" beside activity
ages of 4m53s while `agentbox sync agents`, reading the same roster in the same
second, called all four rows `quiet`. A hung agent looked exactly like a busy one,
which is the single failure this surface exists to prevent, and the human and the
agents saw different answers, which the design forbids in as many words.

*`roster.Flush` had no caller anywhere in the tree* - and its own doc comment
said "the daemon ticks this so a final activity line is never the one that got
dropped". So a push dropped by the 250ms throttle waited for the next unrelated
verb, measured in the field at over a minute: one peer's row read "quiet, nothing
reported" while the CLI showed its line. Both failures are now one tick, which
pushes only when the board would otherwise be wrong, so an idle board still costs
nothing. Two rules worth keeping: throttling is only safe with something that
flushes it, and state derived from elapsed time must be recomputed by a clock, not
by traffic.

*A group header was captioned with whichever member came first.* `LAPTOP-SETUP`
sat over the agentbox path, because the heading comes from the area and the path
came from a row's cwd. It fails a second way too, without anybody declaring
anything: an agent in `frontend/src` would caption its whole repo with the
subdirectory. The caption is the area's own path now, and empty when there is no
honest answer.

*Every hook-created row was named `systemd`.* The attach recipe this project
publishes is `setsid agentbox sync attach`, and setsid reparents to init, so the
name read from the parent process was init's. The same mechanism had already
displayed a control holder as `timeout`, which was recorded in a comment and not
generalized. Names now walk up past shells and wrappers, fall back to `agent`
rather than naming one of them, and a row wearing that placeholder is renamed when
the session's own child announces - which the old code could not do either,
because attach stamped its identity over the row wholesale. `AGENTBOX_AGENT`
skips the guessing.

**Then the discovery rider, slice 1's last piece.** When a session's area gains
or loses an agent, one line rides back on the next response envelope and the child
appends it to that tool's result: who arrived, the purpose they announced, the
state they are in, and a sentence telling the reader to coordinate. Each arrival
is reported once; `announce` and `list_agents` move the cursor silently because
their own results already show the roster.

Three things it taught:

- **A rider needs an audience.** A session's hooks call the CLI with that
  session's own key several times a minute, so the first shape would have had
  every arrival consumed by whichever hook fired next, with the model never
  hearing about company at all. `proto.Identity.Via` marks a caller `mcp` or
  `cli`, and only a child with a tool result to write on spends the news. Caught
  by reading the hook recipe against the new code, before it shipped.
- **The child dials the daemon through six separate helpers.** Wiring two of
  them looked complete and worked in a unit test, then failed live on
  `set_activity` - the most frequently called tool there is, and the one the
  probe happened to use. Fixed by routing every helper through the same path.
- **A departure can only name what was remembered.** The row is gone by the time
  the line is written, so the cursor keeps the name each peer was reported under;
  otherwise the warning names a hex session key its reader has never seen.

**Verified, not asserted.** The board was photographed before and after: the same
four rows that read "3 working" at 4m53s now read `quiet` with the working count
gone from the header, agreeing with the CLI at the same instant. The rider was
proved end to end by two real mcp children over stdio JSON-RPC against the
deployed daemon - announce carries no rider, an unrelated call with nothing new is
silent, a peer joining puts the line on the next `set_activity`, it is said once,
and a hook's CLI call fired in between does not eat it. One of session 40's
`[assumed]` facts also came out true along the way: after `make deploy` restarted
the daemon, the child re-attached on its own and replayed its announce.

**Two new field defects, recorded not fixed.** FR86: `Project` is
`filepath.Base(cwd)`, so an agent in a subdirectory reports project `src` and, via
the identity hash, wears a different colour from its peers in the same repo - the
same story as FR85 arriving by a second route. FR87: the redial replays the
*announce's* activity, so a restart brings every row back with a line that was
true an hour ago, timestamped as fresh.

## Fortieth session (2026-08-04): FR83 slice 1, and two shipped bugs it uncovered

Boris triaged the three open questions (all three answered in the design's
favour, recorded at the foot of [09-sync.md](09-sync.md)), then asked for the
feature implemented "perfectly", with a weekly usage cap to work inside.

**What shipped, in order.** The surface mock over canned rows
(`agentbox webui-demo agents`), walked and clicked on the real desktop. The
session key on `proto.Identity`, with FR74's ownership check moved onto it. The
`Conn.Serve` defer-order fix. Then the roster itself: attach, announce,
generalized `set_activity`, `list_agents`, area derivation, the CLI, the Agents
rail surface, and the teaching in the same commit as the tools.

**Two shipped bugs, both found by building on top of the code rather than by
reading it.**

*A blocking handler never learned its caller hung up.* `Conn.Serve` registered
`cancel()` before `wg.Wait()`, and deferred calls run last-registered-first, so
`wg.Wait()` ran first and waited for a handler that was waiting for the cancel.
The peer closing its socket was never noticed. **FR45's caller-gone indicator has
therefore never fired in the field**, and the reason it survived is worth
keeping: its test cancels the context by hand rather than closing a socket, so
the mechanism was verified by simulating the very thing that was broken. FR83's
presence design rests entirely on this, since an attach is a call whose context
IS the session's liveness. The new test closes a real socket and fails on the old
order.

*Two identity hues for one agent* (recorded as FR85, deferred). Go hashes
`agent + " " + project`; the frontend hashes `agent + "\0" + project`. Four of
five sampled identities get different colours between a card's pill and an inbox
row. The literal NUL in the JS source is also why `grep` and `rg` classify
`tokens.js` as binary and skip it silently, which is how a second implementation
stayed invisible. The Agents surface uses the Go value, joining the majority
rather than widening the split. While writing that entry the same NUL byte got
written into the docs file describing it, which took a byte-level fix.

**A measured number replaced a guessed one.** The design assumed a CLI hold could
run to `wait_max_s` (1500s). Measured: a foreground shell call from a Claude Code
session is killed at **exactly 120s** (SIGTERM, exit 143), and an explicit
timeout caps at 600s. `make deploy` on this repo runs longer than that, so
`--ttl` is the normal path for a wrapped command rather than the corner case the
Locks section implied, and a wrapped hold must release on SIGTERM or every long
command leaves an orphan.

**A defect the mock could not have shown, found by using the feature.** A CLI or
hook `announce` creates a row for a session whose own child has not attached, and
nothing ever removed it: an attached row goes when its attach ends, a provisional
one had no such event. The SessionStart hook in recipes.md announces on every
session start, so Boris's board would have filled with sessions that ended days
ago. Rows now record whether anything holds them open, read as `detached` rather
than `working` when nothing does, and are reaped after ten idle minutes - while
an attached row is never reaped, however long its agent thinks.

**Five defects the mock caught before Boris saw it**, which is the working rule
earning its keep: the activity age had drifted to the far edge away from the
activity it belongs to, the unannounced row said "no purpose given" twice, a
blocked row claimed "nothing reported" beside a named wait, four rows in one area
were indistinguishable until the session key went on the row, and the orphaned
lock (the only thing on the surface asking the human to act) was last instead of
first.

**Verified live, not read.** A fresh `agentbox mcp` child registers `announce`
and `list_agents`, announces itself, gets the other session's row back with its
purpose and activity (`alone: false`, and the teaching note about coordinating),
and its row is gone within the grace when the child dies. `partial: true` then
fired in the wild for the honest reason: this session's own mcp child predates
the deploy, so its items arrive without a key.

**Not verified.** The Agents surface rendering *real* roster data. Boris's screen
was black by the time the daemon was deployed (the whole root captured as solid
black), and waking it was not mine to do. The canned mock was walked and clicked;
the live surface has only been proved as far as the data path and the push.

**Tooling.** `xdotool` was missing on this machine, which is why the mock's click
paths nearly went unexercised; `python3-xlib` served instead (the same XTEST path
`drive_desktop` uses). Boris installed xdotool, and the desktop-verification set
is now written down in `~/me/laptop-setup/playbooks/03-packages.md` with the
reason. The global `~/.claude/CLAUDE.md` also turned out never to have been
versioned: `snapshot.sh` copies it deliberately, but a blanket `CLAUDE.md` rule in
the global gitignore meant git had always skipped it. Fixed with a repo-local
negation.

## Thirty-ninth session (2026-08-04): the sync design, proved by writing it

Boris asked for multi-agent coordination and, across four follow-ups, for the
half that matters more: he must be able to see what every agent is doing right
now, every agent must declare its purpose and current activity and keep them
current, agents must find others working the same area, and they should discover
and message each other. Designed, not built: [09-sync.md](09-sync.md), with the
field case as FR83.

The design was written while two other agents worked this same checkout, and the
session became its own evidence. Coordination happened through session 37's
hand-written ledger at `/tmp/agentbox-agents.md` - file claims standing in for
locks, a `Current holder: NOBODY` line standing in for a lease, timestamped
notes standing in for messages, and re-reading the file standing in for delivery.
Three things went wrong in one hour, each one a requirement in the doc now:
a catch-all `git add` from the parallel agent swept the unfinished design doc
into an unrelated commit; a `git reset` dropped a finished commit off main while
its author was still working (recovered from the reflog, nothing lost); and the
parallel agent's handoff declared "nothing is in flight" while a third stream
was mid-write. Not one of the three was reachable by asking - no agent could see
another's state, so each acted on a stale reading of a shared tree.

What that put in the design, beyond the obvious roster: the mandate is enforced
in four layers because one that trusts model discipline is a wish; a dead mcp
child **orphans** a lock rather than releasing it, since a dead child does not
mean a dead `make deploy` and a five-second grace would hand a live resource to
a second agent; discovery pushes a rider on ordinary tool results rather than
expecting a working Claude session to park on a topic; and every parked wait
gets a ceiling under the client's own idle cap, with a cursor so re-arming
misses nothing.

An adversarial review of the first draft is folded in. The three findings that
forced real design changes: the session key never travelled on per-call
requests, so every ownership check would have collapsed to agent-name equality
and inherited FR74's collision bug as a "precedent"; indefinite parking does
not exist in the runtime the agents actually have, which invalidated the
central efficiency claim until the child learned to keep a parked call alive;
and the first draft's `timeout_s: 0` meant try-acquire, the exact inverse of
what six shipped tools teach, which would have trained agents into the poll
loop the feature exists to end.

## Thirty-eighth session (2026-08-04): the custom panel runs

M12's last open piece shipped: an assignment's `panel_html` hydrates in the
artifact sandbox instead of showing a note. The block carries `data-panel`,
which reroutes its emits away from the waiting-agent path - `emit("params",
{...})` lands in `SetAssignmentParams`, merged over the stored values so
unsent keys survive (turning a typed knob now preserves panel-only keys the
same way), and any other event name is reported in the panel's bar as
undelivered. Values flow the other way too: `window.agentbox.params` plus an
`agentbox:params` window event, pushed on load and on every change. A new
`agentbox:assignments` poke from the daemon after every mutation (save,
values, enable, delete, run start/finish/skip, whoever asked) replaced the
surface's 3s poll, which is what lets an agent's `update_assignment` reach a
panel somebody is looking at. The panel producer joined the no-auto-fetch
policy sweep and the `{@html}` allowlist; the sandbox itself is unchanged.

One defect fixed on the way in: `launch()` ran a run's values through
`assign.Merge`, which starts from the spec's defaults and keeps only declared
keys - with no spec that is an empty map, so every saved value vanished at the
door while the save path had deliberately kept them. Both paths now use
`mergeParams`.

A second defect only the live pass could find: the panel rendered its chrome
unstyled with code and preview both showing, because every `.k-artifact` rule
in `app.css` is scoped under `.k-md` and the host div did not carry it. The
markdown knob beside it was worse - `use:markdown` only hydrates a node that
already holds the HTML, and that div was empty, so a `markdown` block in a
spec had been rendering nothing at all since M12. Both fixed in `e77029f`.

Commits `c15fce6`, `bacb64b`, `368ffbf`, `e77029f`; 568 tests green.

**Exercised live** on the deployed daemon (app window, real `claude` children,
values read back out of SQLite each time), with a probe assignment carrying
both typed knobs and a React panel:

- the panel loads in the sandbox already holding its values
  (`{"threshold":80,"window":"7d"}`) with its controls set to them, so the
  inbound push beats first paint;
- clicking `24h` inside the panel wrote `window` to the database AND moved the
  typed enum knob above it;
- typing into a `note` field the spec has no knob for stored that key too, and
  then dragging the typed threshold slider left it intact - the regression the
  `currentValues()` change exists for;
- `update_assignment` from an agent, with nobody touching the window, moved the
  panel's own buttons and slider and both typed knobs (the `agentbox:assignments`
  poke; the surface no longer polls at all). It also dropped the undeclared
  `note` key with a warning, which is the save path's deliberate rule and worth
  knowing: **a key only the panel writes survives until an agent's next
  `params` update**, so declare a knob for every key a panel writes;
- a run substituted the values it was given (`Window 7d, threshold 80.`) and
  the surface showed it start and finish without a poll.

Not exercised: the bar note for a wrong event name (the routing is unit-tested,
the sentence is not). One accident to know about: a stray click fired one
manual run of Boris's own "Claude usage check" assignment - read-only, but it
is in his history and was not his.

Three mechanics, all of which cost time here:

- **`agentbox control release` matches on agent identity**, and the CLI takes its
  identity from the process. Requesting through `timeout 180 agentbox control
  request` registers the holder as `timeout`, and a bare `agentbox control
  release` then answers `held by timeout` and does nothing. Wrap both ends the
  same way.
- **A config reload resizes windows** (`resizeToConfig`), so another agent
  touching `config.toml` mid-script moves the window your coordinates were
  measured against. Two `hotkey.rebound` pairs in the log are what a stray
  click looks like afterwards.
- **`import -window` can capture a window that is not repainting** (occluded,
  or on another monitor): the shot looked plausible and was minutes stale, and
  it was the mismatch between it and `read_assignment` that gave it away. Raise
  the window first - `agentbox drive --window TITLE run -` with a `move` step
  does it, and its target lock reports which window each event went into.

## Thirty-seventh session (2026-08-04): one visual language (FR81)

FR81's second half shipped: History, Inbox, Library, Settings, Session and
the app shell now speak the voice Home and Assignments established, so the
rail no longer switches between dialects. The rules the pass enforced: UI
caps for section labels and mono only for data (numbers, paths, times, key
hints); Home's tile grammar wherever a count is shown (number first, mono
tabular, lowercase label under it); one segmented-control recipe, one
ghost-button recipe, one search-box recipe; serif, centered empty states
that invite the first action. Library changed the most - rebuilt on the
shared header/search/joined-rows shape, its onboarding sentence moved into
the empty state, ages on the shared 1Hz ticker, delete revealed on hover
behind an inline confirm - and its inverted hover rule (border LIGHTENED on
hover, a `var(--k-edge-soft, --k-accent)` fallback that could never fire)
went with the rewrite. Viewer already conformed and was not touched.

Verified live, not from the diff: every restyled surface screenshotted from
a demo build (dark, plus History and Library in light), and the populated
Library exercised against a copy of the real store - row, hover reveal,
confirm strip, Keep - with the pointer driven by uidrive. `make check`
green; dist rebuilt from exactly the committed sources.

Two mechanics worth keeping: a second agent worked the same checkout in
parallel (the M12 panel), so this session's build, tests and dist came from
a git worktree holding HEAD plus only its own edits - the only way "commit
what you tested" stays true in a shared tree; the agents coordinated
through a claims ledger at `/tmp/agentbox-agents.md` (file ownership, dist
rules, who may displace the desktop's one daemon). And a new costume for
the CLAUDE.md pkill trap: `pkill -f` with any pattern that appears in your
own compound command kills the invoking shell - match by exact PID, never
by pattern.

## Thirty-sixth session (2026-08-03): AgentBox

The project took a new name, AgentBox, and the rename went all the way down
in one session: module `github.com/borismilner/agentbox` (now matching the
public GitHub home), binary and CLI `agentbox`, wire methods `agentbox.v1.*`,
socket, config, state, cache and webview dirs (copied with the daemon
stopped, so history and settings survived - `agentbox stats --since 30d`
answered 311 interruptions right after the move), unit `agentbox.service`,
desktop entry and icon, MCP registration (user scope; Claude sessions
running through the rename hold dead tools until they restart), the embedded
manuals, the deck (regenerated), the recorded takes in `~/Videos/`, and the
logo - the speech bubble now spells AgentBox in the lettering's own palette
(Cantarell Extra Bold over a reconstructed interior; the robot did not
change). The application icon went back to the head alone at Boris's call -
the full logo in a switcher tile shrinks the robot to a sliver - so every
icon is now the face, and genicon says why.

Two more decisions Boris made in the session: the old git history was not
worth carrying under the new name, so the tree ships as fifteen fresh
commits grouped by subsystem (commit hashes quoted in older entries below
belong to the retired history), and the public mirrors follow - the GitLab
project force-pushed, the GitHub repository renamed. User-facing strings
took a brand-casing pass (AgentBox in prose and titles, `agentbox` for the
binary, commands and window titles). `make check` green; the card, the app
window and the speech path exercised live through the new daemon. A
compatibility symlink covers the old binary name, and the old config, state
and cache dirs were left on disk as a fallback - delete them once a few
days pass quietly.

The docs were decluttered the same day, nothing lost: the roadmap's M8-M11
slice narratives were folded into this file's "What M9/M10/M11 shipped"
sections (the three cutover defects moved with them) and the roadmap keeps
plan, acceptance and pointers; STATUS lost its superseded queue preamble
(the 2026-07-31 priority quote moved to session 31's entry above) and had
its stale counts refreshed - the MCP server serves 30 tools since M12, the
suite is 564 tests across 21 packages, and the git notes now describe the
fresh history.

## Thirty-fifth session (2026-08-01): assignments, running

Session 34 built the assignment engine and wired it to nothing. This session
finished M12: the seven MCP tools an agent authors with, the runner that carries
a run out as a session, and the surface. All three exercised on the live
desktop, not read off the diff.

**The tools (slice 3).** `list/read/create/update/delete/run_assignment` and
`assignment_runs`, over six new wire methods. The shape worth keeping is the
update: every field is a pointer, so a nil is "leave it as the human set it"
rather than "make it empty". An agent sending a better prompt must not blank a
schedule by not mentioning it, and `params` merges over the stored values so
setting one knob does not clear the rest. Refusals teach - every spec fault at
once, an unparseable schedule named - and everything merely unfinished comes
back as a warning: a placeholder nothing fills, a knob the prompt never reads, a
custom panel with no typed spec to fall back on. `TestManualListsEveryTool` now
scans the whole package, because a family of tools that grew its own file was
otherwise exempt from the one check that keeps the manuals honest.

**The runner (slice 4).** `SetRunner` and `StartAssignments` existed and were
called from nowhere, so the tick loop had never started. A run is now an
ordinary session with the assignment's model, dir and mode and a brief of its
own (`internal/manual/assignment.md`) saying nobody typed this prompt and nobody
may be watching. Its last assistant message is the summary; a fenced
```agentbox-data block in it is lifted into the run's `data` column, which is what
Boris's "collecting usage statistics for later analysis" turns into - prose for
him, a series for later.

Three rules the tests pin, each of which was a plausible bug: a run must not
take the human's selection (an assignment firing while the panel is open would
move them off what they were typing into), it must stop its child while keeping
the transcript (thirty daily runs would otherwise be thirty idle `claude`
processes), and a child that exits without a word is a FAILED run - recording it
as a success with an empty summary is how an assignment quietly stops working
for a month. `spawn` took a struct after this; eight positional arguments was
the alternative.

**The surface (slice 5).** A rail entry, a door on Home, master and detail. The
knobs are drawn from a descriptor Go builds, the way Settings does it, so a knob
type added to `assign.Param` cannot half-exist because the frontend was not
told, and a markdown block is rendered where all of AgentBox's markdown is. Every
write goes through the daemon's own `AssignmentSave` rather than around it: an
editor with its own idea of a valid schedule would be a second answer to the
same question. A knob writes as it is turned, through its own method - a slider
that could rewrite a prompt would be one bad payload away from destroying an
assignment.

**Demonstrated, not inferred.** Against the deployed daemon: an assignment
created through the real MCP stdio server, run once by an agent (3.3s, summary
and data both landing), then armed `every 1m` and left alone until the scheduler
fired it by itself - `trigger: "schedule"`, using the stored parameter rather
than the earlier override, which is the rule the whole overrides design rests
on. Then on the desktop, with the strip up: the surface open on real data, an
enum and a text knob and a slider each written through and read back from the
database, Run now watched live (the button disabled, the row's running chip, the
list refreshing itself when it finished four seconds later), a run expanded to
its recorded data and the values it actually used, a new assignment written in
the editor, and the smoke test deleted. Two things that pass showed up only
there: the unused-knob warning was computed and never drawn, and a one-line run
report was printed twice.

FR77's target lock refused a click for the second time in two sessions, and
correctly: a `<select>`'s popup is its own X window titled `agentbox`, so `-window agentbox`
was ambiguous and the lock said so instead of clicking the wrong surface.

**Left open, deliberately:** the custom HTML panel stores, edits and round-trips
but does not run in the artifact sandbox. It needs a channel of its own - the
existing machinery routes `window.agentbox.emit` to whichever agent is awaiting an
artifact, which is the wrong destination for a parameter panel - and the typed
knobs remain the way in that always works, which is the rule that made the
escape hatch safe in the first place.

## Thirty-fourth session (2026-08-01): the panel Boris opens, the icon he sees, and the start of assignments

Two halves. It opened as a continuation - the pinned chip Boris had been asked
about (FR78's last piece), and FR74's fullscreen marker - and then he redirected
it, at length and in pieces, into a milestone: a main panel worth opening, the
brand robot in the tray, and recurring AI assignments.

**The chip, answered.** A block is rendered either from the source captured when
the review was written or from the file as it is now, and the surface said
nothing. It now carries a quiet chip: `captured` reads as a whisper, `live file`
takes the warning hue, because it is the only one of the two that can drift out
from under the prose beside it.

**FR74's fullscreen marker.** The strip is top-most against everything, and that
was the wrong answer for one case: an agent takes the desktop while Boris is
watching something fullscreen, and a 620x62 strip sits over the picture for the
whole run. Dropping it was not an option either. So while the FOCUSED window is
fullscreen the strip stops claiming the top and a 4px amber line takes the very
top edge of that screen instead. The marker follows the fullscreen window's
monitor, not the pointer's - mid-run the pointer is wherever the agent last
moved it. **Built and unit-tested, not yet seen on a real fullscreen window.**

**A real race, finally caught.** `make check` failed on
`TestAloudReadsARegionAsOneUtterance`, the -race flake session 25 logged as a
watch item and could not reproduce. It is a genuine bug: `start` set playing,
launched the reader goroutine, then re-read the state to answer with - and on a
short line the reader finishes inside that gap and clears it, so the caller is
told the reading it just began is not playing. It now answers with the state the
command established, under the lock that set it.

**The tray had one working action.** Boris: "the system-tray icon only has a
hide functionality and never show functionality." `ToggleApp` read
`appWin != nil` as open and called `Hide()`, which does not close a window and
therefore fires no `WindowClosing` - so the flag never flipped and the item went
on hiding an already hidden window. Visibility, not existence, is what the tray
labels itself from now.

**The robot, twice.** genicon drew a dot with two ripples; it now renders the
brand robot's head from `docs/img/logo.png`. The first attempt was too small,
and Boris said so with a screenshot. Two causes: a StatusNotifierItem hands the
host a bitmap to SCALE, so a 24px source on a HiDPI panel is drawn at 24
physical pixels - half its neighbours - and the whole head is 204x152, so
squaring it padded a quarter of the icon with nothing. Render at 128 and crop
the head square, cutting the ear knobs. The crop still caught the AgentBox speech
bubble behind it, which at panel size is a dark smudge floating off the corner,
so it is masked to an ellipse: a head is round, its corners are somebody else's
picture.

**Home.** Boris: "now when I show AgentBox it goes straight to SESSIONS which is empty
and is not that important functionality." The app now opens on a surface with
one job in two halves - say something true about right now, and be a door into
everything else. The Go side had to change too: the frontend default was
already Home, but `ShowApp` writes the tab into the URL and still said
"session", so AgentBox went on opening the empty column.

**Assignments (M12/FR82), engine only.** Work AgentBox hands to a Claude agent on its
own - the inversion of everything AgentBox did before, where an agent summoned AgentBox.
`internal/assign` owns the meaning, migration 0007 the rows,
`internal/daemon/assignments.go` the schedule. Three decisions carry it, all
Boris's: a missed slot is counted and never caught up; parameters are typed
knobs by default with custom HTML as the escape hatch, values in the database
either way so a broken panel can never make an assignment uneditable; and the
agent authors the assignment, so the CRUD is an MCP surface before it is a UI.

The scheduler's own tests caught a real one: `launch` stamped the last run from
the struct in hand, which still held the stale next-run, and wrote it back over
the time `arm` had just placed - so a due assignment stayed due and fired on
every tick.

**Nothing runs yet.** No Runner is wired and `StartAssignments` is never called.
The MCP tools, the Runner and the Assignments surface are the next three slices.

## Thirty-third session (2026-08-01): a title bar that was missing, a click that did not look, and a review that forgot its code

Three asks, each one starting from Boris looking at the board, and each one
turning out to be bigger underneath than the sentence that opened it.

**The board had no title bar** (FR76). It asked for minimise and maximise icons
and a shorter Submit; it got those, and then restore-down exposed what only ever
being maximised had hidden. The header did not shrink, so below about 1150px the
controls walked off the right edge, close button among them. And the window could
not be moved at all: frameless, with no `--wails-draggable` region anywhere on
the header. Boris found that one himself, mid-session, with the board sitting on
the wrong monitor. Title ellipses, repo and sha leave first, whole strip drags,
buttons opt out of the grip. The maximise glyph reads its state back from the
window rather than remembering it, because the window manager changes it without
telling the page.

**A click that does not look at what it hits** (FR77). Out of the same live test,
where a drive script moved a window Boris had just moved himself: "I want that
part of the interaction logic with the mouse and the keyboard to be verification
of the parts being clicked and typed in. I bet they can be identified somehow."
They can, on the connection `hand` already holds: `QueryPointer` walked to the
deepest child names the window a click will land in, `GetInputFocus` names the
window a keystroke will go to, and both carry a name, a `WM_CLASS` and a pid. So
`window TITLE` stopped being a coordinate frame and became a lock - raise, follow,
and check every event against it before sending. Proved against two `zenity`
windows the toolkit stacked at identical coordinates: the lock raised the one
underneath and typed into it, a click aimed outside was refused by name, and with
the target closed mid-script and the decoy left on its pixels the `type` step
refused rather than writing into it. That last one is the whole feature; before
it, the text went into the wrong window. The one thing that nearly broke was
menus, which are override-redirect windows parented to the root, so a chain from
a menu never reaches the window that opened it - allowed explicitly, with a
right-click Select All in gnome-text-editor to prove it.

**A review that forgot its code** (FR78). Boris, on a board showing `cannot read
cmd/ssvc-backfill/main.go at the pinned path`: "I thought the code is embedded
and not read each time." It was not. A walkthrough stored the citation and the
diff and nothing else, and the board read the file off disk on every render. The
error he saw is the honest failure; the one that matters is the third case, where
the file is edited in place and still long enough, and the board renders whatever
now sits at those line numbers under the original prose and margin notes, saying
nothing. The pinned SHA was decoration: validated as hex, shown in the header,
read by nobody. Creation now captures each cited range from the tree the
authoring agent was reading; the board prefers the capture and still falls back
to the file, so everything stored before this keeps working. `agentbox walkthrough
repair` recovers the older ones from `git cat-file blob <pinned>:<path>`, which
is the first use that SHA has ever had. Run over Boris's own library: 19 ranges
across two reviews, none missing, and the block from his screenshot renders with
the file still absent from his working tree.

Snapshot on create rather than always reading from git, because a clone can be
deleted, moved or gc'd and the review should outlive it. Git is the repair path,
not the durable one.

## Thirty-second session (2026-07-31): the annotation margin, and two things the manual should have said

Boris sent one screenshot of the board with two arrows on it: a vertical line
that "is sometimes hidden", and number circles that are "black on blue, not easy
on the eye". Both were real and both were one line of CSS.

The rule was the aside's `border-left`, so its length was that box's height - and
`.margin.tail` sets `max-height: 0` on purpose, so the closing prose can start
where the code ends rather than where a tall stack of notes does (FR69). On the
last block of every step the rule was therefore 45px of padding and nothing else:
a stub floating in the margin, with the notes it belonged to running on below it
unruled. The rule now belongs to each `.note`. It is exactly as long as the
annotations are, consecutive notes draw one stroke, and the hovered note lights
its own segment - which turned an ornament into something that answers "which
note am I on".

The numbers were `--k-surface` on a 78%-accent fill: a dark numeral on saturated
blue at 11px, 4.1:1 in the dark theme and worse in the light one, where the same
declaration puts white on a pale wash. Both pills now take a wash with a ring and
draw the numeral in `color-mix(accent, ink)`, which lightens for the dark theme
and darkens for the light one from one line, and both are circles centred on
their line instead of ovals sitting high. Photographed in both themes.

Third thing in the same header, unprompted but the same defect: two identical
copy glyphs, each carrying `margin-left: auto`, so they split the free space and
the anchor control floated mid-bar with nothing to belong to. They sit together
at the end now and the one that copies a reference is drawn as a link.

The two lessons cost more than the fixes did.

**A second AgentBox shows no windows.** `AGENTBOX_INSTANCE=dev` gives a second socket and a
second store, and the CLAUDE.md line about running "beside" the deployed daemon
was read as meaning a second UI. It is not: the first process owns
`org.wails.agentbox`, so a later one is a remote GApplication instance and its window
requests go to the holder. Twenty minutes went into a board that logged
`walkthrough.opened`, exited 0 and drew nothing. Seeing a UI from a working copy
means holding the desktop's only daemon - `make stop`, run the build against a
copied `XDG_STATE_HOME`, `make restart-daemon` after. Now in CLAUDE.md.

Both lessons then got paid off in the same session. The FR74 MCP tools are
written (`request_control`, `set_activity`, `release_control` - 23 tools now), and
the first thing they were used for was the one check this session still owed:
the note margin below its 1180px breakpoint, where the notes stack under the
block and now carry a per-note rule. Held the desktop with `request_control`,
narrated the resize with `set_activity`, photographed the board at 1100px, gave
it back with `release_control`. The stacked notes line up into one left edge and
the hovered note still lights its own segment, so the breakpoint path is now seen
rather than reasoned about. A running MCP child cannot see tools added after its
handshake, so the tools were driven over a fresh stdio server; the session's own
`mcp__agentbox__*` list will only carry them after a restart.

**The hands-off strip already existed and was not used.** Needing the desktop for
before/after photographs, this session hand-rolled a `confirm_action` asking
Boris to keep his hands off - and he was interrupted mid-run anyway, because a
card is answered and gone while the driving takes minutes. FR74's strip, built
earlier the same day, is the thing for exactly this, and the reason it was missed
is that nothing an agent reads mentions it: it has no MCP tool yet (queue item 1)
and the agent manual documented `drive_desktop` without it. The manual now has a
section of its own for the handover, a decision-guide row, an anti-pattern, and a
CLI-table entry, and the global agent instructions carry the same rule: keep the
strip up for the whole run, `activity` as you go, `release` when you stop.

## Thirty-first session (2026-07-31): read-aloud measured, not guessed; then the desktop got a handover

The handoff arrived pointing at one open defect and a hypothesis about it: a
passage loses its last words, probably because the engine abandons a line's tail
when the next arrives. The hypothesis was wrong, and it was wrong in a way that
five minutes of reading settled - `kokoro-say` is `for line in sys.stdin:
say(line)`, strictly sequential and blocking, so a new line cannot interrupt
anything. Measuring rather than reasoning found the real cause in the next
half hour: the engine emits NOTHING until it has synthesised a whole line (66
chars 1.70s, 218 chars 5.74s, 743 chars 14.15s), while `drain` gave a line five
seconds to produce its first byte. From two sentences up, the wait declared the
line silent, released its waiter, and the reader advanced while forty seconds of
speech was still being made. Every position the transport tracked ran ahead of
the sound, and everything acting on one cut audio nobody had heard.

Boris settled the fix before the analysis was finished: "The read-aloud was
running perfectly before I asked to be able to pause stop and rewind. If they
harm this feature, I don't mind having only play or whatever maximal
functionality that keeps perfect functionality of kokoro speech." And then the
shape he actually wanted, which was better than the one being repaired: a control
per region, so he hears the paragraph above a code block, reads the block, and
then asks for the next. That is FR72. The splitter is gone, a region is one
utterance, and `lead` and `close` are read for the first time - read-aloud had
been written against the step shape from before FR69 added them, so the takeaway,
the sentence a step exists to land, was never spoken at all.

Then the session's real subject arrived, because Boris kept interrupting the
drive sequences: "we need a better way of interacting, a permanent on-screen sign
that means 'hands off' for me. Otherwise I keep interrupting you and we work
against one another." It grew over five messages into FR74 - the strip asks for
the desktop and lets him deny, then reports what is being done while he is hands
off, and going away is how he learns the desktop is his again. Two states, not
four: a quiet "working, but you may touch things" state was drafted and cut,
because presence is the whole signal and an ambiguous presence is worth nothing.
It outranks every window including AgentBox's own ("even AgentBox can't cover it"), which
took a notification window type and a keeper that restacks it, since the type is
a hint and a card mapping later wins otherwise.

FR74's first live run immediately produced FR75. The strip and a toast both
computed the same top-centre inset and neither knew the other existed: `toast
430x78 at 745,48`, `hands off 620x62 at 650,48`, overlapping, with the toast
invisible underneath. Boris saw it coming in the same breath - "if a second one
comes it should be below the previous one" - so position stopped being a
per-surface decision and the top-centre column got one owner.

Two things worth keeping beyond the features. First, the FR68 mystery that two
handoffs blamed on a small button: synthetic clicks were landing on whatever
window was on top, and Boris diagnosed it in one line - "your testing failed
because the AgentBox application went to background while I write to you in the
terminal." Raise the window in the same command as the click. The button was
never the problem. Second, this session lost four interruptions to cards that
closed before he could read them, and the two features he asked for - a body he
can read back, and a sign that stays up - are both the same complaint. A tool
whose purpose is that a message is not lost had two ways to lose one.

Before the session closed Boris set the priority: the two desktop-sharing
field requests come first, "since it will keep biting us all the way until
it is there". (Reset 2026-08-01, session 34, in favour of FR81/FR82; the
queue in STATUS.md carries the current order.)

## Thirtieth session (2026-07-30): teaching, not just rendering (FR68-71)

The session started from a single instruction about the *content* of a review
and ended four field requests later, all of them about the same thing: a
walkthrough is read by a person with a finite amount of attention, and the board
had been built to render one rather than to teach one.

**FR68, the glossary.** Boris asked for definitions of the domain words a review
uses - NVD, SSVC - without those definitions cluttering the reading, and rejected
hover-pops in the same breath. `glossary: [{term, short, body?, also?}]` on the
spec; the first occurrence of each term in each step gets a quiet dotted
underline in a different ink from a bound phrase, and the definition opens on a
click or on `g`. One matcher in `internal/walkthrough/glossary.go` serves both
the renderer and a validator warning about entries no prose can reach; it fired
five times across two regenerations of the field review, and every one was a
real gap. Segments are cut into runs in Go rather than shipping offsets, because
Go bytes and JavaScript code units stop agreeing the moment prose stops being
ASCII.

**The standard was rewritten** around the reader rather than around the spec:
the job, plain writing, the shape of a step, ordering the code by the path the
data takes rather than by the file, which channel each kind of text belongs in,
the glossary, and hunting for the aha the diff does not hand you.

**FR69, lead and close.** Boris, on three steps in a row: two code blocks with no
text between them - is that by design, does it read better that way. It was not
and it does not. `prose` and `code` are separate arrays, so every word rendered
above every block. Blocks gained `lead` (the sentence that hands the reader into
this block) and steps gained `close` (the takeaway, under the code it is about).
The rule behind his question went into the standard: the weight of the
explanation belongs in the notes, beside the lines - prose opens, notes explain,
close concludes. The field review went from 41 notes to 81 under it.

**FR70, the library.** "How do I save and load it?" Reviews had been durable
since the store landed, but the only door was the CLI, and a door you cannot see
is a feature you assume is missing. A Library tab in the app window lists every
stored review with its progress, opens one onto the board (retargeting the
window rather than opening a second), and deletes behind a confirm - with the
board retargeted or closed when the review under it goes. Same commit fixed the
dead space a tall notes column left under the last block of a step: the margin
no longer sets that row's height, so the closing paragraph starts where the code
ends.

**FR71, icons and the hover tip.** Header chips became an open book and a shelf,
the copy controls a glyph that ticks, the read-aloud button dropped the label
that repeated its own triangle. And a marked word now answers both of the
questions it gets: hover gives the one-line short after 220ms, click still opens
the full entry. First attempt rendered the tip inside the step, where it lands
in the scrolling column's paint order and the prose drew straight over it; the
surface owns the element now.

**Process, learned the hard way twice.** Driving the desktop to verify UI without
telling Boris first cost him two interruptions - once when he closed a window
mid-verification, once when he was typing. The rule now: announce through an AgentBox
card before and after taking the mouse, never through the terminal, because the
terminal is behind the application being tested. His words: "you must also let me
know before and after so I know."

## Twenty-ninth session, part two (2026-07-29): the real board, slice 1

Boris accepted round 7's visuals and said "next big thing"; the plan
(ADR-0012) and slice 1 shipped the same evening. The walkthrough is now a
durable object: spec validation with teaching errors (internal/walkthrough),
the diff-as-manifest parser (internal/change), SQLite tables with the
spec/annotation split (migration 0005), daemon methods + `agentbox walkthrough`
CLI, the per-line chroma renderer, a native board window and the lazy-loaded
Board surface. The first real walkthrough reviews this slice's own diff -
created live, walked by Boris live. Two field finds within minutes, both
fixed and redeployed: a block whose citation missed EOF by three lines
rendered the honest FR61 error but shipped `lines: null` and froze the step
swap (the board now wears render errors as text instead of freezing, and an
errored block always has a layable shape); and a webview keeps its bundle
from open time, so a deploy needs the window reopened before it shows.
Deferred by plan: submission/receipt + MCP tools (slice 2), annotation
badges/margins and bind lighting (slice 3), find (4), coverage/drift (5),
amendment (6), webui-demo hook. Boris's first poke already asked where the
numbered whys went - slice 3's answer.

## Twenty-ninth session (2026-07-29): round 7 lands whole

One thread: the markup half of the round-7 spec (items 1 to 14), applied to
the mock in the expert's impact order and exercised live before hand-back -
station buttons, the u verdict with its selection fill and the next button
lighting up, answer reveal, the gate's newly wired copy swap, the submission
receipt with its delivered-at line, all screenshot-verified on the reopened
board. Applying item 9 confirmed the spec's cascade diagnosis: the mock's
own style block beats Tailwind classes by document order inside the sandbox,
so every size the spec names on a prose/mono/button element went inline. The
spec file is deleted per its own header; what it decided lives in the FR58
round-7 paragraph. Boris got the round-8 poke and the one-time zoom note
(his 130% A+ base renders the re-scaled board at 1.69x until A-). Driving
note for later sessions: `agentbox drive` scripts are one step per line via
`run -`, and scroll's sign is positive-down.

## Twenty-eighth session (2026-07-29): the diff card learns what a file is

Boris opened with the complaint FR62 now records: the review card's structure
was its dimmest ink, "not engaging", no felt value against an HTML kit like
`~/Desktop/ssvc-review-kit`. The session's answer has two halves. The value
argument: a review card costs the agent nothing (`--diff-file` never passes
through the model) where a bespoke kit costs tens of thousands of output
tokens, and the card closes the loop (approve/comment lands back in the
agent's context) where a kit is one-way. The pixels argument: he was right,
and the card was rebuilt.

The card now parses its unified diff in the frontend - hunk line counts
disambiguate content from "---"/"+++" lookalikes, verified in isolation
against `git diff --numstat` on this session's own diff plus new/rename/
plain-diff/mangled fixtures - and renders per-file sections with sticky
headers, per-file stats and new/deleted/renamed badges. Multi-file diffs get
a left rail of file steps: click to jump, n/p keys, scroll-spy, seen files
go quiet while remaining ones stay bright (unread work is what should catch
the eye). Diff cards open at 560/780px; `Fit` had to learn the width follows
the item, because it re-applied the configured width on every resize and
would have snapped the card back to 470.

Field-tested by Boris live, four rounds, which caught what code reading did
not. His first real note ("the request changes textbox doesn't fit") went
out as an APPROVAL because Enter in the note submitted approve - the note
is now a growing textarea where Enter is a newline and Esc hands the
keyboard back to a/r. Round two came back through the request-changes path
(exit 1, multi-line comment intact), closing the loop on both answers. The
note's missing top border took the longest: layout was provably right
(Chrome and the artifact webview both measured the 10px gap), but WebKitGTK
paints a rounded scroll container's horizontal scrollbar over the
container's own bottom edge and past it, erasing the pane/note boundary.
Reproduced in an artifact-window probe only once the probe lines actually
overflowed - a probe that does not reproduce the trigger proves nothing.
Border and radius now live on an outer box, scrolling on an inner one, and
the scrollbar cannot reach the frame.

The session's second half turned out to be the thread the opening complaint
was actually about: the FR58 review board (the "left part where the steps
are" was the board's rail, not the diff card). Rounds 6 and 7 of the mock
are recorded in the FR58 entry; round 7 follows a dedicated UI/UX design
pass whose spec and application state live in tools/mockups/round7-spec.md
(half-applied at session end - the file compiles and renders, at the old
markup sizes with the new palette).

Two traps for later sessions, learned the slow way: a second AgentBox process
cannot own webui windows while the deployed daemon runs (GTK single-instance
hands them to the primary), so `AGENTBOX_INSTANCE=dev` isolates the daemon but NOT
window testing - test presentation via deploy, `webui-demo` with the daemon
stopped, or not at all; and `item.held_idle` means a card fired while Boris
looks away waits in the inbox, so a test card can be "displayed" with no
window mapped. Also: announce input-sensitive tests to Boris in advance -
his hands and my synthetic keys raced over the same card and the first
test round's evidence was garbage.

## Twenty-seventh session (2026-07-28, night): the ghost card is dead

One thing, done end to end: STATUS queue item 2, the card-shaped window
that survived every item's resolution (seen live in session 25, type
UTILITY, bare title, daemon counting 0 pending). Reading the code against
the Wails source gave the whole mechanism, and it was two holes, not one.

First, the orphan race. showCard decided reuse-or-replace at the call
site, but the window only lands in u.prompt inside the deferred onMain
closure. Present runs outside the daemon lock on purpose ("Present must
not call back into the daemon"), so two near-simultaneous items - a
restart burst, or just two connections - both read a nil prompt, each
closure opened a window, and the second assignment orphaned the first.
An orphan never closes: emit broadcasts to every open window, so it
repaints whatever view comes next, forever. The decision now runs inside
the closure, serialized on the UI thread with the creation it decides
about, and closeCard travels under the same onMain key, so a close
ordered after a show can no longer overtake it and leave a card up for a
resolved item (a third hole, found on the way).

Second, the unprepared map. Wails creates the native window lazily
(pendingRun around startup, drained in goroutines), so NativeWindow can
answer nil right after NewWithOptions; xidOf == 0 then fell through to a
bare win.Show() - a focus-stealing gtk_window_present with none of the
card hints, which is exactly the dress the ghost wore. The path now
late-binds: win.Run() is idempotent and Hidden in the options keeps it
unmapped, so the hints still go on before the first map. The remaining
genuine fallback (Wayland, no X11 surface) logs webui.card_unprepared;
the original incident was invisible in the log.

Two regression tests in loop_test.go pin both fixes and were verified to
fail on the unfixed code (both panic - the test UI has no application to
open a duplicate window with, which is the point). make check green, 18
packages with -race; a fresh count put the suite at 397 top-level tests
(the 362 STATUS carried had gone stale). Deployed as a6ebb1a995a7 after a 45 s
veto card (not stopped), the review board reopened on the same build, and
the live check ran the STATUS repro recipe: two bursts racing a real ask
card against an info toast at the deployed daemon. Both resolved; the
only daemon-owned window left standing was the review board. One CLI
lesson for the recipe: agentbox ask refuses a single --option, so a scripted
card needs two.

The same night, on the owner's ask, code highlighting went from five
token roles to eleven: types, constants and builtins, tags/attributes/
decorators, escapes and interpolation inside strings, operators and
punctuation each got their own colour; the preprocessor left the comment
bucket (a #include is an instruction, not a remark); chroma's generic
tokens now ride the status colours, which gives diff fences tinted
add/remove lines for free. Palettes fall back role-by-role (op to ink2,
punct to ink3, the rest to their nearest sibling), so a five-colour
palette still renders the full set. onedark and dracula joined the
code_theme enum, the settings surface following by construction. Chosen
the way he asked - "show me them in a way that I can browse and choose,
you can use AgentBox for that": a picker artifact (tools/mockups/
code-theme-picker.html, data baked in by gen-picker-data.go) previewing
every theme in both modes on a sample rendered by the real pipeline,
answered over window.agentbox.emit; he pressed Use on onedark and the config
write reloaded live. Deployed as c5bf400b5690.

A lesson paid for on the way: uidrive's `keys` sends synthetic X11
keystrokes to the FOCUSED window, not its --title target, and a scroll
sequence aimed at the viewer landed as a row of k's in the owner's
terminal prompt. Wheel and `shot` are position/window-bound and safe;
for keys on the live desktop the tool is `agentbox drive --window`, announced
out loud first. The announce-desktop-driving rule now says so
explicitly.

## Twenty-sixth session (2026-07-28, evening): find reaches into the sandbox

FR58 mock round 4, driven by Boris live, and one real AgentBox feature it forced
out. His find complaint ("does practically nothing, I should be able to
search the code block or blocks") closed STATUS queue item 4 for real: the
viewer forwards find into the artifact sandbox over the existing postMessage
channel, the frame searches its own text, paints via the CSS Highlight API
and answers with the count. The adversarial pass he then ordered found the
build's three real holes - the sandbox focus trap eating Ctrl+F (chords now
forward out in capture phase), find scoped to the rendered step instead of
the review (the board answers with an "also in step N" strip; requirement
recorded), and highlight ranges dying silently on React re-renders (a
MutationObserver re-find, debounced 150ms, position held, no scroll theft).
F3/Shift+F3 joined the find vocabulary on both sides of the sandbox
boundary, and F3 cycles in the viewer's own documents too, so the vocabulary
is one. The frame search skips invisible text, scripts, SVG, textareas and
anything under `data-agentbox-find-exclude` (an artifact's own find UI must not
count as a hit), so the count only counts what can be seen. WebKitGTK here
is 2.52; the Highlight API needs 2.44+, and without it hits still count and
scroll, just unpainted.

The mock itself moved the spec four steps: the closing sentence became a
multiline closing note (floor, not cap); numbered click-to-pop annotations -
draggable, closeable, reopenable - replaced the look-here diamond and carry
the agent's why beside the code; deleted and changed lines render with their
old line numbers (the reviewed diff's real latin_layout() removal and the
real Title option change are on display); and a step now holds one code
block or several. Re-verifying citations before the round caught two stale
anchors in our own mock - hand.go:379 for a loop at 381, perform.py:171 for
a function at 179 - FR61's silent-staleness class, live in the tool being
built to kill it. Everything is under the FR58 entry as Mock round 4.

Round 5 followed the same evening, on the deployed round-4 build. Esc now
discards a comment draft outright (owner overturned the round-3 guard: a
protective Esc read as a broken key). The mouse-travel jitter Boris
screenshotted was the comment-peek footer living inside the code panel's
scroll layout and rendering in every block of the step; it moved below the
scroller, scoped to the hovered block, with hover state set only on change.
Annotation pops widened to 460px with a scrolling body. And A+/A- changed
meaning by owner's call - mechanism four of the zoom saga: the sandbox
document's rem root is scaled instead of an html zoom rule, with the base
size sourced from [font] size_pt via --k-size like every other AgentBox surface;
the mock's type went rem throughout so it participates. All recorded under
FR58 as Mock round 5.

## Twenty-fifth session (2026-07-28): the guards guard, the keyboard obeys

Four items of the STATUS queue landed and deployed (build dd375a3cb2c7),
plus FR61 - the coverage-and-citation-integrity field request written the
day before during a real review in ~/work/minimus - committed and pushed so
it exists beyond this machine.

The showcase gained the assertion it was missing: an `("above", title)`
step in perform.py that reads `_NET_CLIENT_LIST_STACKING` and fails a
rehearsal unless the named window is on screen AND stacked in front of the
fullscreen stage. `agentbox drive where` could never answer that - the daemon
knows the window it made, not whether anyone can see it, and slide 11's
progress bar existed for its whole slide while the film showed an empty
slide. Slides 11, 16, 17 and 19 now assert their windows (progress,
artifact, report, panel, and the app window once it had its own title).
Verified live three ways: a progress bar over a fullscreened viewer read as
above, a window genuinely under the stage read as "UNDER the fullscreen
stage", a made-up title read as "never appeared".

The keyboard-group defect got its real fix, and the investigation buried
the workaround. `internal/hand/xkb.go` speaks three XKB requests built byte
by byte from XKBproto.h (jezek/xgb has no xkb package): the version
handshake, GetState, LatchLockState. The first design - lock group 1 for
the length of a Type call - measurably fails on GNOME: the desktop
re-asserts the human's input source within about a millisecond of seeing
the group move (one read in two hundred caught the lock before the revert).
What holds is ordering within one X connection: every synthetic press now
sends its own unchecked lock immediately before it, so the server processes
the pair back-to-back and the revert always arrives after. Verified end to
end with Hebrew staged through the real Alt+Shift_L switch: the release tag
that once reached a card as `2026ץ7ץ3` arrived as `2026.7.3`, and the
Hebrew layout survived the call. The war story's supposed mitigation turned
out to be a placebo: GNOME 46 ignores the deprecated gsettings key
`input-sources current` in both directions, so record.sh's prepare step and
perform.py's `latin_layout()` never moved anything - both are removed, and
the trap table in showcase.md now records the fix instead.

Window titles are one per surface now. The card keeps bare `agentbox` (every
script and recipe targets `"=agentbox"`); toasts are `agentbox · toast` and the app
window `agentbox · app`, joining progress, panel and the viewer. It took two
commits: the Title option turned out not to survive framelessness (Wails
never calls gtk_window_set_title for a frameless window - the bare `agentbox`
the drivers matched all along was GTK's program-name fallback), so the name
is written onto the mapped window via x11 setName, the way the other
surfaces already did. Verified with wmctrl: three surfaces, three titles;
the one remaining bare `agentbox` is the 1x1 helper window Choose already
size-filters.

`--help` no longer performs the action it asks about. `agentbox inbox --help`
opened the inbox window - and `agentbox quit --help` actually quit the daemon,
found while fixing the class: inbox, quit, summon and mcp all ignored their
arguments. All four now parse through a FlagSet like every flagged command
and print one line of purpose.

A watch item: one `make deploy` run failed its `-race` test gate and the
immediate identical re-run passed; the failing package scrolled away
uncaptured. Nothing flaky has been seen before in 362 tests. If it happens
again, keep the log.

The afternoon of the same day opened FR58 and closed a ghost. Boris asked
"why is this notification still on my screen" over a card the daemon had
resolved thirteen minutes earlier: a 470x194 window with type UTILITY and
the bare fallback title - the marks of showCard's `xidOf == 0` escape path,
where a window maps via plain Show() and falls off `u.prompt` tracking, then
repaints whatever view is broadcast next, forever. Evidence saved, window
closed, filed as a STATUS queue item with the suspected mechanism.

FR58 then left the armchair by its own rule: mock before building, a real
review inside. `tools/mockups/review-board.jsx` is an AgentBox artifact reviewing
this very session's diff - route rail with ground/nothing/gate stations,
three-valued state, two visual channels on code, prose-to-code binding both
ways, select-to-comment with exact anchors, comprehension checks, FR61
coverage, pinned commit, one submission. Boris drove three rounds the same
afternoon ("the general idea seems correct"); every decision is recorded
under the FR58 entry as mock rounds 1-3: full screen by default with free
resize wanted, syntax highlighting non-negotiable, comments visible from
the code and editable, reveal toggles as icons, Ctrl+Enter/Esc in the
composer, the composer opening at the selection, the path copying in one
click, and - on his direct challenge - submission became the one primary
act, with copy-as-markdown demoted to an export nicety.

The mock also did what the working rule promised: it flushed real AgentBox
defects within minutes of contact. The artifact code view trapped the
reader (a flex min-width hole clipped the preview/code tabs off-screen).
The reader's zoom took three mechanisms to get right: rem font-size never
reached the sandbox at all; CSS zoom on the frame applied only at frame
build (A+ "worked" only after reload) and drifted WebKit's hit-testing,
which corrupted comment anchors to lines nobody selected; a compensated
transform applied live but composited blurry text. The one that holds:
an `html { zoom }` rule injected into the sandbox's own base style over
the existing style channel - crisp, live, native hit-testing, surviving
reload and theme change. The zoom level now shows beside A-/A+ (the
footer's copy was off the eye's path, so clamp-bound clicks read as
broken; clicking the number returns to 100%). Artifact windows open
maximized (a working surface, not a page), and the block chrome - tabs
and reload - renders only when an artifact sits inside a conversation.
Found and filed, not fixed: the viewer's find cannot see into a running
artifact's frame.

One working practice was born mid-session: before any agentbox drive sequence on
the live desktop, say it out loud (`agentbox say`), and say when the keyboard is
his again - his find-bar typing and a drive click fought over the same
window, and the warning is cheaper than the collision.

## Twenty-fourth session (2026-07-26): the icon deployed, the fix seen working

The mascot icon reached the running daemon (`make deploy`, build
e1198a5cdd84), and the top-most fix was finally seen doing the thing that
failed on camera: a progress bar run over the deck's fullscreen slideshow
showed `_NET_WM_STATE_ABOVE` in xprop and the bar visibly over the show in a
screenshot. The slide-11 failure mode is closed; the re-record waits only on
Boris's calendar.

The README got its missing picture: the app window, shot live on the inbox
around the same checkout-api story, two pending questions on top and the
day's history behind them. Two staging mechanics were learned. With two or
more items pending, Esc-defer only rotates the cards - the display refills
instantly from the queue - so DND is how items are held in the inbox with no
card on screen. And the inbox's live search (`checkout-api` typed into the
box) is how a public shot keeps off-story history rows out of frame without
touching the DB, which stays Boris's alone to prune. Mid-session Boris
flagged the README's showcase section as redundant; it was condensed first,
then removed outright on his second note - the README is the repo's public
front page and carries no internal tooling. The deck's docs (showcase.md,
recording.md) became rows in the docs index instead.

Stage notes against the next take: OnlyOffice's slideshow button sits ~16px
above the window bottom in the current layout, not the ~64px the last
handoff recorded, and starting the show replaces the editor window with a
new X window id, so a held id goes stale. The progress window already
carries a distinct title (`agentbox · progress`); cards and the app window are
still both exactly `agentbox`.

## Twenty-third session (2026-07-26): the old name is out of the documents

The licence now credits AgentBox. The old name was removed from every document,
comment and filename in the repo - the ADRs included; the naming ADRs were
eventually dropped with the names they recorded, so no document carries an
old name. The recorded takes in `~/Videos/`
were renamed to `agentbox-showcase-*` and every reference updated (the films
themselves still show the old brand on screen, which is one of the two
reasons for the planned re-record). The showcase prune snippet matches the
pre-rename demo agent by pattern rather than by name, because the history DB
keeps its real agent names by Boris's decision. STATUS.md was split: current
state stays there, the session narrative moved here, and the two interleaved
"do this next" lists became one. Later the same session the 197-commit git
history was condensed into nine era commits - same final tree, messages that
reflect the arc rather than the step-by-step record.

The same session gave AgentBox a face and rebuilt the README as a showcase. Boris
generated a robot mascot, cut its head into the launcher and tray icon
(`internal/tray/icons/app-256.png` - embedded, so it ships with the next
deploy) and put the full scene at the top of the README; the crop took three
rounds until the monitor sat flush against the top-left corner and the sign
had air on the right. The README was then rewritten element by element -
card, diff review, progress, viewer, artifact - with every screenshot
retaken live through the daemon around one story, checkout-api release
2026.7.30 (the same world the deck performs): a release-bot choice card, a
test-runner diff review approved on camera, a reindex at 67%, a release
report in the viewer (compact mermaid, error-budget math, a chart, code),
and a canary rollout console as the artifact. The Matrix-themed props
(mission-briefing.md, the red-pill console) were replaced by these; sources
live in `docs/img/src/`. The Documentation table now lists only
reader-facing docs. Driving the shots retaught one mechanic the hard way:
cards never take focus, so a synthetic keystroke lands in the focused
terminal - `agentbox summon` first, then the key.

## Twenty-second session (2026-07-26): the rename

The project shed its working name - the door metaphor had sold the wrong
thing, an interruption at your door, when the whole pitch is that being
reached is cheap. The rename was total: module
`github.com/borismilner/agentbox`, binary and CLI `agentbox`, socket `agentbox.sock`, config
`~/.config/agentbox/` (moved), state `~/.local/state/agentbox/` (the old state dir was
MOVED, so history survived - `agentbox stats --since 30d` showed 270 interruptions
right after), unit `agentbox.service` (the old unit, desktop entry and icon
removed), MCP server `agentbox` (user scope, tools `mcp__agentbox__*`; running Claude
sessions must restart to see them), artifact API `window.agentbox.emit`, frontend
`agentbox:*` events, wire methods `agentbox.v1.*` (old binaries cannot talk to the new
daemon). `docs/agentbox-showcase.pptx` replaced the old deck (regenerated, copy
rewritten around the quick-question story; the door metaphor retired). The
README screenshots were retaken under the new brand (sources in
`docs/img/src/`). `make check` green after the rename (one test updated for
the new casing - `manual_test.go` expects "AgentBox session"), smoke test through
the new daemon passed. Later the same day Boris renamed the repo directory
(`~/me/projects/agentbox`) and the GitLab project path (`gitlab.com/fu-bar/agentbox`;
his first attempt changed only the project *name* - the *path* is a separate
field, and only the path changes the URL); `origin` was re-pointed, `main`
pushed and in sync, the old GitLab path redirects. He also decided to
re-record the film from scratch and reupload it, which retires the old-brand
take as the public one. `.venv-deck` was rebuilt after the directory move (a
venv pins absolute paths; pip's shebang had died); `make deck` verified at
the new path.

## Twenty-first session (2026-07-26): the showcase is recorded

`~/Videos/agentbox-showcase-20260725-2344.mp4` is 17:36, all 22 slides, rehearsal
clean and `verify.sh` clean at 106 samples; it was uploaded to YouTube.
Getting there cost three defects in the tooling, two of which had been
misdiagnosed before: `take.sh` resolved the deck **by window title** and so
drove the terminal the take was started from (an older window with
"showcase" in its title) instead of the deck; `perform.py`'s `close` steps
**never ran** (`INPUT_STEPS` is matched first, so the close branch was dead
code - that, not a missed click, is why slide 17's report window survived
its close in the 22:33 take); and `agentbox dnd off` **delivers what DND held**,
so slide 13's changelog notice landed on top of slide 14. All fixed and
rehearsed clean. The film has one gap: **the progress bar (slide 11) is
missing**, because that window was the only surface AgentBox puts up that was not
top-most, and a focus-free window maps *underneath* a focused fullscreen
app. Boris's rule (2026-07-26): every surface must be top-most. `x11.above`
+ `progress.go` do that now; the fix was committed this session and deployed
with the rename. Full postmortems: [recording.md](recording.md) under "What
the 22:33 take cost", "The progress bar nobody could see" and "What the
23:44 take cost".

## Twentieth session (2026-07-25): the pitch

The deck became a sales pitch, not a feature tour: the problem, what it buys
you, then proof. Its content rules, all from Boris watching a take:

- **Nothing addressed to the presenter is on a slide** - no file names, no
  act numbers, no "the one to sell hardest". `docs/showcase.md` is the
  presenter's copy.
- **No invented world.** The story is one team's afternoon shipping a
  release, so each tool appears in an example a viewer recognises: a
  dependency scan, green tests, a disk at 86%, a failed build, a climbing
  error rate, where to deploy, what to tag it, a certificate rotation nobody
  stops, a release checklist, a reindex reporting progress, a deploy token,
  a timeout fix to approve, and how much traffic the canary takes. Four
  agents ask (`release-bot`, `test-runner`, `dependency-bot`,
  `oncall-helper`, project `checkout-api`), which is what makes the history
  tab worth showing.
- **Every element appears**, the progress bar and the report included, and
  each is held long enough to read. Narration beats sit between the actions
  rather than in front of them; `perform.py` warns when the deck's beats and
  the plan's beat slots disagree.
- **The self-driving pointer is not in the pitch at all.** Two slides of
  reveal were cut. It stays as the mechanism, and `perform.py` says so at
  the top of the file.
- **A "what is next" slide**, because a stakeholder wants to know it is
  alive: away delivery, sessions from the panel, Wayland and packages.
- **The tagline is "Stop babysitting your agents."** (The name question was
  reopened and settled two days later.)

The wrong-monitor bug was also fixed this session: `x11.place` had one
caller, `bridge.go:99`, so cards and toasts were placed and every other
window was left to Mutter - which on this desk means the portrait primary,
off camera. The viewer, the artifact and the app window now call
`UI.placeOn` right after the map (`internal/webui/webui.go`). Verified live
with the pointer on the wide monitor: artifact +1590, report +1590, app
window +1450, progress bar in that monitor's bottom-right corner. Before the
fix, `agentbox show` opened the viewer at 90,570. Placement was only half of it:
the progress bar was in the right corner of the right monitor for the whole
of the 23:44 take and still never reached the film - it was under the
fullscreen deck. Being on the right monitor and being visible are two
different checks, and only the first one was made.

## Nineteenth session (2026-07-25): the recorder, and `agentbox say --wait`

`tools/showcase/record.sh` runs the camera and `tools/showcase/perform.py`
runs the show; [recording.md](recording.md) is the reference for both.
1080p60 on the iGPU, audio from the sink's monitor so no microphone is in
the graph, two marks so the finished file starts on the first slide,
-14 LUFS for upload. The narration lives in the deck's speaker notes and is
spoken with `agentbox say --wait`, which is what keeps twelve minutes of it in
sync with twenty-two slides without a tuned sleep.

`agentbox say --wait` itself shipped this session: the engine's PCM
passes through a counting copy, so the daemon answers when a line has been
heard rather than when it was queued. Measured against a hand-run
engine-to-player pipeline with the model load subtracted: 3.21 s against
3.09 s, and 11.87 s against 11.91 s - inside the 120 ms quiet window the
wait ends on. `--wait` on `agentbox say` (with `--timeout`, which bounds the wait
and not the line), `wait` on the `speak` tool.

Three findings worth keeping:

- **PipeWire's monitor tap is pre-volume.** The same line captured with the
  speakers at 10% and 60% peaked at -7.6 and -8.6 dB - line-to-line
  variation, not the knob. So the human's volume cannot affect a capture,
  and no null sink is needed to protect it.
- **A measured pixel offset is only true for the card it was measured on.**
  The secret card's Send was clicked at the form card's Submit offset,
  missed by ten pixels, and sat on screen until it expired. Card answers now
  click, verify, and fall back to Return - which `Card.svelte:133` routes to
  the same `send()` the button calls.
- **OnlyOffice ignores a file argument** and restores its previous session
  instead, and its `x2t` converter dies with a V8 error, so there is no
  headless pptx-to-pdf route. Ctrl+F5 starts its slideshow for a human but
  not as a synthetic keystroke; the play button in the status bar (23 px in,
  64 px up from the window's bottom) does work.

## Eighteenth session (2026-07-25): the panel, after a day of using it

The two-monitor bug was fixed and the drop-down panel became a console you
would keep open. Windows now land on the monitor the pointer is on
(RandR 1.5) instead of in the middle of the X root; the panel got a live
session the moment it opens, a real `claude` child answered a typed prompt
in it (the last unverified path in the project), and using it for ten
minutes flushed out a dozen defects that are all fixed. Sessions are now
named by Claude's own first words, renameable, saved to disk and reopenable
with `--resume`, and each turn shows a 24-hour clock and how long the model
thought. AgentBox also gained its **licence** this session: beerware with a
visible-credit clause.

Everything below was found by Boris opening the panel and working in it, and
every one is fixed. Kept because each is a lesson about this stack, not just
a bug:

- **Windows were centred in the X root.** Two monitors make one root - the
  bounding box of both - so "the centre of the screen" was the seam.
  `internal/webui/monitor.go` is the rectangle maths (tested, no X in it)
  and `x11.go` asks RandR 1.5 for the monitor under the pointer at every
  placement. Measured: a card at 305,853 on the portrait screen and 1805,433
  on the wide one, a toast at 1825,48, progress at 2572,911, the panel
  top-centre of whichever screen the pointer is on.
- **Wails' `SetSize` cannot resize a mapped window.** It is
  `gtk_window_set_default_size`, and a default size is what a window opens
  at. The panel mapped one pixel tall and stayed there while
  `agentbox panel state` reported it open. Size goes through the WM now
  (`x11.moveResize`), the same route the position already took.
- **Two goroutines dispatching to the UI thread do not order.** `Show` sized
  the window shut while `slide` sized it open; when the frame won, the panel
  was invisible. A roll owns the UI thread for its duration and dispatches
  everything, including the map, from one place.
- **Mutter refuses skip-taskbar for a `_NET_WM_WINDOW_TYPE_DIALOG`** - as a
  pre-map property and as a client message alike. Measured on one live
  window by changing only its type: a dialog keeps "ABOVE", while utility,
  dock, splash and notification all become "SKIP_PAGER, SKIP_TASKBAR,
  ABOVE". Cards and the panel are utility windows now; only a toast is a
  notification. That is what kept an icon in the dock for months.
- **`_NET_WM_STATE_DEMANDS_ATTENTION` was never interned**, so the call that
  clears Mutter's attention flag had been sending atom 0 since it was
  written.
- **`--permission-mode full` is not a thing.** The choices are acceptEdits,
  auto, bypassPermissions, manual, dontAsk and plan. AgentBox's "full" now maps to
  bypassPermissions - not acceptEdits, because AgentBox does not speak the
  stream-json permission protocol yet and any mode that can ask would stall
  behind a prompt nothing renders. Boris's instruction: ordinary permissions
  must not be asked for, the harness is the guard. (Before the fix, Full had
  never worked: `full` is not a `--permission-mode` value, so the child
  exited 1 the moment anybody chose it - and the control was two `<span>`s
  with no handler.)
- **A mode switch is a new child.** `--permission-mode` is spawn-time, so
  switching kills the child and starts one with `--resume <claude session
  id>`, seeded with the transcript and keeping AgentBox's own session id (the
  switcher is ordered by it, the child carries it as `AGENTBOX_SESSION_ID`, and a
  pending question routes by it). Verified: `resumed=true turns=2`, no gap
  on screen.
- **Claude Code sends one assistant message per tool call.** A reply that
  ran five commands drew six identity pills. `encodeTurns` joins a run of
  consecutive assistant messages into one turn.
- **A measured, bottom-anchored content box goes stale.** The panel surface
  laid itself out once at a height Go measured; move the panel to a monitor
  of a different size and the box keeps the old height, leaving a band of
  empty background that reads exactly like a dragged window. The surface
  fills the window now.
- **`agentbox panel status` toggled the panel.** An unrecognised positional was
  ignored, leaving the action as "toggle", so asking the panel how it was
  changed it. Bare verbs are accepted and an unknown one is an error.
- **A session in a directory was named after the directory**, so three
  sessions in one project were three chips reading the same word. Claude's
  own first words are the name; a double-click renames it.

## Seventeenth session (2026-07-25): AgentBox works the desktop itself

`internal/hand` synthesises real X11 input, reachable as `agentbox drive`, the
`agentbox.v1.drive` RPC and the `drive_desktop` MCP tool (14 tools total), which
is how the showcase answers its own cards with nobody at the keyboard.
Demonstrating it flushed out four shipped-but-broken things, all fixed: the
**review card was never ported to the webview** (every review silently
approved), a **form with a checkbox could never be submitted**, `make
deploy` **killed every agent's `agentbox mcp` child**, and `agentbox dnd status`
**printed "off" while auto-DND held every card**. New: `make bootstrap` /
`make doctor`, a sales showcase (`docs/showcase.md` plus an 18-slide deck),
and a rewritten README.

## Sixteenth session (2026-07-25): cutover, M10, M11, deployed

The webview port completed and cut over, M10 was done, AgentBox gained a voice
(M11), and AgentBox was DEPLOYED - running under systemd and registered as a
user-scope MCP server, so every Claude Code session in every project can
reach Boris through it. A hotkey rolls a session panel down from the top of
the screen (M10 slice 1), an agent can show interactive HTML that AgentBox runs
and hear what the human does in it (slices 2 and 3), and slice 4 closed the
parity gap with math and images. M11 added speech. `agentbox daemon` renders every
surface through `internal/webui` (Wails v3 + WebKitGTK) and the Gio UI is
gone. Boris had reopened the no-webview constraint to make the agent-facing
surfaces beautiful and every element more polished than its Gio original;
ADR-0009 records the decision and supersedes ADR-0002.

The suite fell to 231 tests at the cutover, down from 258: the Gio offscreen
screenshot harness (30 tests, surfaceless EGL over llvmpipe, gated by an env
var) and the markdown Document golden tests (4) went with their packages,
and nothing replaces the screenshot harness - though the same session added
the policy tests, which are the part that mattered. The panel, the hotkey
parser and the main-loop gate added six back; math, images, speech and the
encoders built it up to 330 and later 362.

**Deploying was reworked this session.** `make deploy` replaces whatever is
at the installed path and leaves the daemon running the new build. Three
things about it were wrong and are not any more. It installed before
stopping the daemon, and a daemon whose file is replaced underneath it keeps
serving the old code from an unlinked inode - so the binary was new and the
behaviour was not; it now stops, replaces, restarts. It could not tell
whether the restart took, because `agentbox status` printed the *client's* version
over whatever the daemon replied - the daemon now reports the build that is
actually serving, status prints that, and says so plainly when the two
differ, which is exactly the "new binary, old daemon" case. And `make
deploy` now FAILS on that signal instead of printing a hopeful line. The
replaced build is kept as `agentbox.prev`, so `make rollback` is one command;
`make deployed` is the check on its own. `pkill -f "$(BIN) daemon"` became
`pkill -x $(BIN)`: the -f form matches the shell running the recipe, because
the string is in the recipe. `install` is load-bearing - it unlinks the
destination first, so it can replace a running binary where plain `cp`
fails with "Text file busy".

**Packaging (M7) was verified live this session, and verifying it found a
bug.** `make install` puts the binary, the `.desktop` launcher, the icon and
the systemd `--user` unit in place, and the service is enabled and active
with systemd owning the daemon. The bug: the unit's `ExecStop` was
`agentbox quit`, and because AgentBox is single-instance by flock and auto-spawns,
`agentbox daemon` prints "already running" and exits 0 when one is up - so systemd
saw a Type=simple main process finish, considered the service done, and ran
ExecStop, quitting the healthy daemon. **Enabling the unit removed AgentBox
entirely.** ExecStop is gone; SIGTERM already drains through the same
graceful path, and the unit explains why.

**Removed: Demo cards** (owner's call). Gone: the tray's "Demo cards" item,
the `agentbox demo` subcommand and its `runDemo` sequence, the `tray.Hooks.RunDemo`
field and the daemon's re-exec that spawned it, and the `make demo` target.
It fired six real interactions through the live daemon - toasts, a choice, a
confirm, a veto - which meant the way to look at AgentBox's cards was to put six
items in the real queue, on top of whatever an agent had waiting, and answer
them. `agentbox webui-demo` does the same job without a daemon and without
touching the queue, store or inbox, so nothing was lost but the
interference.

### Verified live at the cutover, through the real daemon (2026-07-25)

- The focus policy: a card and a toast map without changing
  `_NET_ACTIVE_WINDOW`; `agentbox summon` then focuses the card. The ~500 ms
  post-map input grace once suggested in 04-platform.md was never needed.
- `agentbox show --watch` with a live edit re-rendering in place; an `agentbox progress`
  pipe filling its own window and the interrupted notice when the pipe dies;
  the app window on every rail surface; the settings surface saving one key
  surgically with `config.reloaded` ~0.6 s later and the behaviour actually
  changing (actions.enabled off -> the buttons stop rendering). Colour emoji
  render. Cards answer by key after `agentbox summon`, and the FR28 undo grace
  then delivers.
- Artifacts (M10 slices 2-3): a React artifact written in the claude.ai
  shape ran in the sandbox with the injected React and Tailwind; a click in
  it reached a parked `agentbox artifact wait` as `run {"rows":500,"dryRun":false}`
  (exit 0); three slider moves with no agent waiting read back as one
  coalesced value; `--watch` re-ran an edited artifact in place; the
  code/preview toggle works; and a probe artifact confirmed it escapes
  nothing (the table is in ADR-0010). `[artifact] enabled = false` removed a
  running frame and left its source, and true started it again.
- Math and images (M10 slice 4): all five spellings of math typeset (inline
  `$...$`, inline `\(...\)`, a `$$` block, a `\[ \]` block with
  `\begin{aligned}`, and a ```math fence), `$5 and $10` stayed money in the
  same paragraph, a formula KaTeX refuses fell back to its own highlighted
  TeX while the formula after it still typeset, a PNG at an absolute path
  was read from disk and inlined, a `data:` URI image rendered, and a remote
  host and a relative path both rendered as placeholders naming the reason.
  The `img-src` policy does not disturb artifact hydration.
- Document-relative images: `agentbox show docs/sample.md` resolved
  `![](../internal/tray/icons/app-256.png)` against `docs/` and inlined AgentBox's
  own icon; the identical markdown piped in on stdin rendered "needs an
  absolute path" in the same window seconds later; a resolved path with no
  file behind it reads "file not found" rather than asking for an absolute
  path; and `--watch` re-resolved a relative image after an edit.
- Speech (M11): detection found piper and picked `en_US-lessac-high` on its
  own tier and read 22050 from the voice's JSON; an error notification
  chimed and then spoke its line; `agentbox say` worked from an argument and from
  a pipe; `agentbox mcp` lists 13 tools with `speak` present and a `speak` field
  on the item tools; and the same thing ran against Kokoro by changing only
  `[speech] command` and `rate`, which is the engine contract doing its job.
  Boris confirmed the audio by ear both times. The engine process exits with
  the daemon.

## What M10 shipped (slices 1-4, 2026-07-25), in detail

- **The drop-down panel.** A hotkey rolls a session down from the top edge
  over whatever you were doing, and rolls it back up: `Ctrl+Alt+grave` by
  default, grabbed by the daemon itself (`internal/hotkey`, XGrabKey on the
  X11 root) so it needs no desktop configuration, with
  `agentbox panel [--show|--hide|--state]` as the fallback for a bound shortcut or
  a Wayland session. It shows the same sessions the app window shows, and it
  takes the keyboard - the `agentbox summon` exception, because you asked for it.
  Esc or its own button rolls it up. `internal/webui/panel.go` +
  `frontend/src/surfaces/Panel.svelte`; `agentbox webui-demo panel` shows it with
  a canned conversation.
- **How it animates, and why that is not obvious.** Two measurements against
  this GTK4/Mutter/X11 stack decided it, both recorded in panel.go: Mutter
  **clamps** a managed window to the work area (so a fixed-size window
  cannot be parked above the top edge and slid down - a negative y is
  silently refused), and the window is **not translucent** even with
  `WindowIsTranslucent` and a transparent background (so the CSS-transform
  approach is out too). The panel therefore pins itself at y=0 and animates
  its *height*, while the surface anchors its content to the bottom of the
  viewport at a fixed height: nothing reflows, and the conversation rides
  the growing edge down. The frames must be dispatched one per main-loop
  turn - a loop that blocks the UI thread shows two sizes and nothing
  between.
- **Everything is a knob, live.** Every window shape, the reading measure,
  the panel's geometry and the session defaults are configuration
  (`[window]`, `[panel]`, `[session]`), and the config watcher hands the
  whole config to `UI.SetConfig`: tokens go out as a CSS variable write and
  the open windows resize as you edit. The watcher polls at 400ms (was 2s)
  so tuning is a conversation, and the panel's hotkey rebinds live rather
  than at the next restart. Two new settings sections (Windows & panel,
  Sessions) carry all of it.
- **Interactive artifacts, and a door back to the agent.** An agent can show
  HTML AgentBox *runs*: `show_artifact` (MCP), an `artifact` fence in a
  conversation, or `agentbox show --artifact FILE [--watch]`. It runs in an iframe
  with `sandbox="allow-scripts"` and no `allow-same-origin` (opaque origin)
  under `default-src 'none'` with no network directive at all, so **React 19
  and Tailwind v4 are injected as text** from AgentBox's own bundle plus a JSX
  transform in the surface: an artifact written for claude.ai runs offline
  as written. What the human does inside reaches the agent through
  `window.agentbox.emit`, the only way out - `await_artifact_event` blocks on it
  (the `ask_user` metaphor, with the same timeout and event log),
  `read_artifact_events` drains it coalesced so a dragged slider is one
  value rather than forty. Every artifact has a code/preview toggle, and
  `[artifact] enabled` is a live, retroactive trust switch. The bargain and
  the probe that verified it escapes nothing are in
  [decisions/ADR-0010-artifact-sandbox.md](decisions/ADR-0010-artifact-sandbox.md);
  the code is `internal/webui/artifact.go`,
  `frontend/src/lib/artifact.svelte.js`,
  `frontend/src/lib/artifact-runtime.js` and `internal/daemon/artifacts.go`.
  Those guarantees are re-checkable at any time:
  `agentbox show --artifact tools/artifact-probe.html`.
- **Math and images (slice 4).** TeX in all four spellings agents write it
  in - `$...$`, `\(...\)`, `$$...$$`, `\[...\]`, plus a ```math fence -
  parsed in Go (`internal/webui/math.go`, not a dependency: the interesting
  part is the money rule that keeps `$5 and $10` money) and typeset by KaTeX
  in the surface with `trust: false`. A formula KaTeX refuses shows its own
  TeX. KaTeX moved from being one of mermaid's dependencies to a direct one
  in its own 259 kB chunk, so a formula does not drag in a 3.1 MB diagram
  engine. **Images were a security fix, not a feature**: raw HTML is dropped
  before rendering, but `![alt](https://host/p.gif?d=...)` was ordinary
  markdown that goldmark turned into a live remote `<img src>`, so rendering
  agent prose made a request to a host the human never saw.
  `internal/webui/images.go` now reads an absolute local path itself, checks
  the bytes really are PNG/JPEG/GIF/WebP by magic number, and inlines them
  as a `data:` URI under a 2 MB ceiling (cached by path, mtime and size,
  because `encodeTurns` re-renders every turn on every stream push); a
  remote host or an unknown scheme renders as a placeholder that says why
  and keeps the alt text. `frontend/index.html` carries
  `img-src 'self' data: blob:` so it is enforced, not merely intended. The
  rule is recorded beside the artifact bargain in ADR-0010.
- **A relative image path, where there is an honest base** (the one
  follow-up slice 4 left open). `agentbox show FILE` and the viewer's watch loop
  render through `RenderMarkdownIn(src, baseDir)`, so `![](out/chart.png)`
  in a document resolves against the document's own directory - the file
  stays portable and reads correctly in an editor. The base travels in
  goldmark's parser context to an AST transformer, because one engine serves
  every surface and a NodeRenderer is handed only the node. Prose over the
  socket - a card body, an agent's turn - still gets no base and is still
  refused: the daemon's working directory is not the agent's. A resolved
  path faces the same stat, sniff and ceiling as one written out in full, so
  this added a way to *name* a file, not a way to reach one.

## What M11 shipped (2026-07-25), in detail

- **AgentBox speaks.** `internal/speech` holds one synthesiser open and pipes it
  into a raw-PCM player, because loading a piper voice costs ~2.5s while a
  sentence costs ~70ms - shelling out per notification would make every one
  arrive three seconds late. The engine contract is one line of text on
  stdin, raw s16le PCM on stdout, so it is not a piper integration; with
  `[speech] command` empty AgentBox finds piper and the best voice itself and
  reads the rate from the voice's own JSON. Quality is not a knob: the
  highest tier installed, and pw-play's resampler at 15 of 15 rather than
  the default 4, because a 22.05 kHz voice on a 48 kHz sink is resampled
  every time.
- **The agent writes the line.** `proto.Item.Speak`, `--speak` on the CLI,
  and a `speak` field on every item-creating MCP tool, so what is heard is a
  sentence an agent chose rather than a title read aloud. Silent unless one
  is written. It rides on the item, so idle hold, mute, DND and quiet hours
  all apply already, and escalation repeats it. `daemon.Sounder` grew
  `Speak` instead of becoming a second interface, so a new `Play` site
  cannot forget the voice.
- **A line with nothing behind it**: the `speak` tool and `agentbox say "..."` (or
  a pipe), for something worth hearing that is not worth a card. It never
  touches the queue, the store or the inbox.
- **Waiting for a line to be over**: `agentbox say --wait`, and `wait` on the
  `speak` tool, answer when the audio has finished rather than when it was
  queued - which is what lets an agent narrate a sequence without timing it
  by hand. The engine's PCM passes through a counting copy on its way to the
  player, and raw PCM converts a byte count to a duration exactly, so the
  wait ends when the stream has been quiet 120 ms *and* everything handed
  over has had time to play. Neither condition works alone: quiet by itself
  cuts a line off the moment the engine gets ahead and fills the pipe (the
  sound is loudest right then), and the arithmetic by itself ends the wait
  in a gap between two bursts of synthesis. `Speak` stays non-blocking; the
  waiting rides on the queued utterance as a channel that is closed on every
  path out, including a line displaced from a full queue.
- **A session can be ended from the switcher.** A ✕ on a row, which asks
  first - "End this session?", or "Claude is working here. End it anyway?" -
  because a mis-click kills a running agent and drops an unsaved
  conversation. Closing kills the child, moves the selection to the
  neighbour, and re-presents a question waiting in that conversation as a
  card.

## What M9 shipped (the webview port), in detail

All seven M9 slices are done (they went to `main` via the `wails-v3`
branch): the app shell + card surface, the session surface, the inbox +
history surfaces, the settings surface, the viewer + toasts + progress with
the markdown renderer rebuilt behind them, the inline ask panel, and the
cutover. `internal/ui` and `internal/markdown` are deleted; gio, gio/x,
gonum/plot and the embedded DejaVu fonts are out of go.mod. The port was a
toolkit swap behind an existing seam: `internal/webui` satisfies
`daemon.Presenter` and calls the same `Resolver` the Gio UI did, so daemon,
queue, store, protocol, MCP, sound and the session driver were untouched by
it.

- Slice 7, the cutover: `cmd/agentbox/daemon.go` builds `webui.New(res, log, cfg)`
  and gives `u.Run()` the main goroutine. The old theme block is gone -
  `webui.New` takes the config and resolves `auto` against GNOME's
  `color-scheme` itself - and `config.Watch` now calls `u.SetTheme(c)`,
  which is the live-theming route the Gio build had no equivalent for. The
  FR29/FR44 idle monitor moved to `internal/presence` (a side X11 connection
  plus a gsettings read, no toolkit in it) so the daemon depends on a
  presence signal rather than on a UI package. `make build` gained a
  `frontend` prerequisite: editing a `.svelte` file and running `make build`
  used to embed the old `frontend/dist` with no warning. Three defects the
  cutover surfaced, each fixed with a test: any daemon start with an
  unresolved item in the store died on SIGSEGV - `daemon.New` re-presents
  the restored item before `Run`, and `application.InvokeSync` dereferenced
  a platform application that did not exist yet, so window work arriving
  early is now queued and replayed when the loop starts, keyed so a repeat
  replaces its predecessor (a cold-start `agentbox show`, `agentbox app` and
  `agentbox progress` all landed in the same hole). `agentbox show FILE --watch`
  silently dropped `--watch` - Go's flag package stops at the first
  positional, and flag-last is the order the docs used; `show`, `mute` and
  `unmute` now parse flags around their positionals. And Wails quits when
  the last window closes, which for a tray-resident daemon whose windows
  are transient meant answering the first question killed AgentBox;
  `DisableQuitOnLastWindowClosed` is set.
- `internal/webui` - Wails v3 app; the `Bridge` service (the only thing the
  webview can call); `tokens.go` turning config into CSS custom properties;
  `mdhtml.go` (goldmark + chroma with class-based highlighting, so code
  follows a theme change); `sessions.go` (switcher + conversation, pushes
  coalesced to ~16Hz); `app.go` (the frameless application window).
- Slice 5: `toast.go` + `Toast.svelte` (the notify strip), `viewer.go` +
  `Viewer.svelte` (FR36-38, also the app's viewer surface), `progress.go` +
  `Progress.svelte` (FR21's own window), and the markdown renderer rebuilt
  for HTML: GitHub alerts as tinted panels, `chart` fences as themed SVG
  drawn in Go (bar/line/area/scatter/pie/doughnut), mermaid diagrams
  rendered by a bundled engine loaded on demand, copy buttons and line
  numbers on code blocks, task lists, footnotes, definition lists, and links
  that open in the desktop browser rather than navigating the window away.
  One stylesheet (`.k-md` in app.css) serves the card, the session and the
  viewer. The ADR-0008 amendment records the reasoning. `x11.go` gained
  top-center and corner placement, a quiet map (no focus) and `setName`,
  because Wails skips `gtk_window_set_title` on frameless windows and every
  AgentBox window is frameless.
- Slice 6: `inline.go` + `frontend/src/lib/AskPanel.svelte` - the inline ask
  panel (FR49). A question from an agent running in the session surface is
  answered in its conversation, above the composer, instead of in a card
  over the window the answer is being read in. `inlineRoutable` is the whole
  rule: tagged with a session the surface is showing, a kind the panel can
  take (choice/confirm/notify), not urgent, and the app window open.
  `askOptions` builds every control including its key cap and which one is
  the default, so the surface loops over a list and switches on nothing;
  `Bridge.AskKey` runs the keystroke through `triageFor`, the inbox's own
  table. The panel does not take focus - keys act only when no field has it,
  Esc in the composer hands the keyboard over, and the hint line names
  whichever mode you are in. A switcher row marks itself when its
  conversation is the one waiting. Closing the app window with a question in
  the panel re-presents the item as a card (`rerouteAsk`), which the Gio
  build never needed because a session there died with its window.
- `internal/webui/x11.go` - vision principle 3 under GTK4. GTK4 removed
  `set_accept_focus`, and Wails' `Show()` calls `gtk_window_present()`,
  which is a focus request. The working sequence: realize early, read the
  X11 id via `gdk_x11_surface_get_xid`, set the `_NET_WM` hints (including
  `_NET_WM_USER_TIME = 0`), map with `gtk_widget_set_visible`, then settle
  stacking and placement by client message - Mutter overwrites a pre-map
  `_NET_WM_STATE` with `DEMANDS_ATTENTION` and re-places the window at 0,0.
- `frontend/` - Svelte 5 + Tailwind v4 + Vite, built to `frontend/dist` and
  embedded with `go:embed` (dist IS committed; the binary must build without
  npm). Surfaces: `Card.svelte` (all item kinds, full keyboard map),
  `App.svelte` + `Rail.svelte` + `Session.svelte`.
- `[theme]` in config grew `ground`, `contrast`, `accent`, `density`,
  `radius`, `motion`, `[font]` grew `family`/`reading`/`mono`, and
  `[markdown] code_theme` (documented since M6) was finally wired: `auto`
  derives from the ground, `nord`/`gruvbox`/`github` are dark/light pairs.
  All of it applies live, so theme and font no longer carry an "applies on
  restart" tag - only the knobs the daemon genuinely reads once
  (`dnd.start_in_dnd`, `history.*`, `log.*`) do. `theme.contrast = "high"`
  lifts the muted inks and the hairlines toward the ink rather than swapping
  in a second palette.
- `internal/webui/settings.go` (slice 4) - a descriptor table (section →
  group → knob) drives the whole settings surface: the control a knob
  deserves, its bounds, its valid values and whether it needs a restart are
  declared once, in Go. `Bridge.Settings` reads the file fresh (the file is
  the baseline, not the daemon's in-memory config, because the file is what
  Save edits and the user may have hand-edited it since).
  `Bridge.PreviewTheme` resolves pending values through `BuildTheme` so the
  preview panel cannot drift from the result. `Bridge.SaveSettings` writes
  only changed keys via `config.Write`, returns the exact lines written,
  refuses a value that cannot mean anything (bad enum, malformed hex,
  unparseable quiet hours) without blocking the other keys, clamps
  out-of-range numbers, and pushes the new tokens at once instead of waiting
  for `config.Watch` to notice. `frontend/src/surfaces/Settings.svelte`
  paints it.
- `internal/webui/inbox.go` + `stats.go` (slice 3) - the inbox surface
  (FR10) and its triage rules (FR34/FR50), and the history surface (FR35).
  Both read through `webui.Source`, which was a copy of the Gio package's
  interface rather than an import of it - which is why deleting that package
  touched nothing. `UI.Present` pushes `agentbox:inbox` (coalesced) on every queue
  change, so no surface polls. `Bridge.Triage(id, key)` takes a keystroke
  and decides in Go what it means, so the card and the inbox share one
  keyboard vocabulary; `Bridge.CopyItem(id)` takes an id, never text, so the
  surface cannot put anything on the clipboard that AgentBox is not holding.
  `frontend/src/surfaces/Inbox.svelte` and `History.svelte` paint them.
- `Bridge.Theme()` - a window asks for its tokens on boot and applies them
  before mounting. Wails serves `Options.Flags` over a runtime call rather
  than injecting them into the page, so the old `window.wails.flags.theme`
  read was always undefined and every window rendered the CSS defaults
  (dark) regardless of config. Light mode, a configured ground/accent and a
  font size now apply at startup, on every surface including the card.

## Milestone timeline (how the phases landed)

- M0-M4 done by 2026-06-13. M5 done (tray, FR32 action buttons, FR34 triage,
  FR33 diff review, FR35 stats, and the calm/multi-agent refinements
  FR44/45/47); the last M5 items (runtime agent mute FR47,
  missed-while-away FR44, caller-alive FR45) closed in the session before
  the FR29 one. Per-agent sound signatures (FR46) were dropped as redundant
  (identity hue + waiting dots + FR47 already cover it; the M5 roadmap note
  has the rationale).
- M6 (markdown engine + viewer + charts) done; M7 packaging done (Wayland
  deferred; owner not on Wayland).
- One session shipped four things at once: the FR29 presence gate
  (hold_when_idle, fullscreen_auto_dnd, respect_desktop_dnd on top of the
  FR44 idle monitor), FR21 progress (CLI pipe, `report_progress` MCP tool,
  dedicated window), an error-logging robustness sweep (panic recovery +
  logging on every goroutine and swallowed-error path, owner requirement),
  and M7 packaging (systemd user service, .desktop, app icon, Makefile
  install targets).
- Ninth session (2026-06-13): M8 slice 3 - the settings tab editing
  `~/.config/agentbox/config.toml` through a surgical, comment-preserving writer
  (only the keys you change, defaults never materialized), reloaded live by
  the daemon's existing config.Watch - plus an extended live UI-polish pass:
  tabbed-app close-to-tray, tray show/hide, graceful shutdown,
  official-client session layout, find next/prev, collapsible thinking,
  embedded DejaVu, veto card auto-close fix, and docs/agent-manual.md. M8
  complete: slice 1 (the tabbed app shell) and slice 2 (the Claude session
  tab, FR49) had shipped earlier sessions.
- Tenth session (2026-07-04): FR50, small - blocking questions can be
  dismissed for good (shift+Esc on the card, d/backspace in inbox triage),
  resolving the ask as unanswered, same as a timeout. Motivated by nudge
  (~/me/projects/nudge) posting voluntary prompts (idea capture): no agent
  is owed an answer there, and without a dismiss an abandoned card re-queued
  forever. Veto and diff keep their must-act keys. Spec in 01-requirements.md
  FR50; covered by TestDismissUnblocksAsk and the triage table. Same
  session: the footer "expires in Ns" now counts down for real - the daemon
  records each timed question's arrival-anchored deadline (View.ExpiresAt,
  set before first present) and the footer ticks against it like the header
  countdown; it previously echoed the static configured total.
- M9 (the webview port) landed over sessions 13-16; M10 and M11 on
  2026-07-25 (sixteenth session); the sessions after that were the showcase
  (17-21) and the rename (22).

## Session UI decisions (M8, live-tuned with the owner)

Tuned against the official Claude client; the M9 web surface keeps the same
decisions. The conversation uses the official-client layout - the user's
prompt in a right-aligned rounded bubble, Claude's reply full width with no
label, the empty assistant turn the stream opens skipped so no stray header.
The agent renders in mono by default (the user stays sans; the Mono toggle
forces all-mono). Code blocks sit in a distinct panel (raised background
plus a 1px border ring; the page background blended in) with a copy button,
and a literal tab renders as spaces (no tofu). Find gained prev/next with an
"n/m" counter that scrolls the matching turn into view and tints matching
code panels. The chart title/axes/legend render in Liberation Sans (the
serif clashed); Noto symbol faces are a glyph fallback (♪, arrows,
dingbats). The plan/full selector, locked once a child spawns, opens a NEW
session in the chosen mode instead of silently ignoring the click.

Design decisions made with Boris during M9 (sketches reviewed and approved
before code): the chrome carries no hue, so the only saturated colour on
screen is an agent identity or a severity; three type roles (Cantarell
chrome, Bitstream Charter for agent prose, JetBrains Mono for code); a left
surface rail instead of horizontal tabs; a permanent status strip; cards
keep their own window and only session-tagged questions land inline.

## Test-suite timeline

258 tests before the cutover; 231 after it (the Gio offscreen screenshot
harness, 30 tests, and the markdown Document goldens, 4, went with their
packages); 330 after the sixteenth session; 362 as of the 2026-07-26
rename; 564 as of the AgentBox rename (2026-08-03, session 36). The
M8-era ui-package tests (tab-switch wrap/clamp/select, the stats->markdown
builder, session add/switch/mode, settings descriptor mapping) and every
offscreen `TestShot*` screenshot went with `internal/ui`; the
`internal/session` and `internal/config` suites survived unchanged, and the
webui/frontend suites described in STATUS.md replaced the rest. The old
harness was gated by an env var so `make check` skipped it; nothing gates
anything now - the whole suite runs everywhere.

Git archaeology, for the record: `main` was fast-forwarded from the Gio
build (2026-07-04) through the whole `wails-v3` lineage - a clean
fast-forward, no merge commit - with the M10 work landing via
`m10-math-images`; both working branches are deleted. On 2026-07-26 the
197-commit history was condensed into nine era commits with the same final
tree, so commit hashes recorded before that date no longer resolve. On
2026-08-03 the history was restarted outright for the AgentBox rename -
fifteen subsystem commits, same tree - and the era hashes went the same
way.
