# Session history

The dated, session-by-session record that used to accumulate at the top of
[STATUS.md](STATUS.md). STATUS.md now carries only the current state; this
file carries how it got there, newest first. Nothing here is needed to build
or run AgentBox - it is the record of decisions, defects and verifications, kept
because each cost something to learn.

The project has worn earlier names; prose here uses the current name
throughout, including in entries dated before a rename.

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

Commits `c15fce6`, `bacb64b`, `368ffbf`; 568 tests green. **This entry was
reconstructed from those commits and STATUS after the fact** (the session that
wrote them left STATUS naming session 38 but added no entry here), so it
records what the code says and not what that session saw on screen - the
panel's live behaviour is theirs, and unverified elsewhere in these docs.

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
