# Field requests

Use cases found while actually using AgentBox on real work, and improvements to
existing tools that the same sessions asked for. This list is picked up
**before** the "Later / parked" bucket in [05-roadmap.md](05-roadmap.md): these
are things a live session wanted and could not get, not speculative features.

**How to add an entry.** One entry per use case. Continue the FR numbering from
[01-requirements.md](01-requirements.md) (highest there is FR50) so an entry can
be folded into the requirements unchanged once accepted. Every entry states:
what the session was doing, what AgentBox could not do, what was done instead, and the
proposed shape. Record the workaround even when it is ugly, because the cost of
the workaround is the argument for the feature. Facts verified during the
session go in "Mechanics discovered" at the bottom rather than inside an entry,
so they stay useful after the entry ships.

Status tags follow the requirements doc: `[must]`, `[should]`, `[later]`. A new
field entry starts as `[field]` until it is triaged.

## Working rule: mock it before building it

**Every entry in this list gets mocked up before it gets implemented.** Look and
behaviour first, as something throwaway that can be looked at and clicked, then
iterate on that until the requirements and the decisions are settled. Only then
write the real thing. The mock is for deciding, not for shipping, and it should
be cheap enough to throw away without regret.

This is not general good practice pasted in; it is what produced this list. The
review-board requirements under FR58 came out of building one review as a folder
of static HTML by hand, and the specifics that matter most (reference code by
path rather than pasting it, amend one step without disturbing the others, the
preamble that must not count toward the total, three-valued state where the
middle value is the real output) were all invisible until something existed to
look at. A spec written from the armchair would have missed every one of them,
and would have been implemented confidently and wrongly.

So for FR58 in particular: mock the board, put a real review in it, use it on
real work, and let it tell you what the spec should say. Heavy implementation
starts after that, not before.

---

## FR51 [field] IDE code anchors

**Session.** 2026-07-27, reviewing a backend feature branch (8 files, 4
commits). The agent had produced a map of 26 specific locations worth looking
at, each with a reason. Getting Boris's eyes onto each location in turn was the
whole job.

**What was missing.** AgentBox can render the map (`show_document` takes markdown, so
the list and the reasons display fine), but a `path:line` in an AgentBox card is inert
text. Reading "pkg/images/postgres/client.go:1367" and then navigating there by
hand, 26 times, costs more than the map saves.

**What was tried instead, and why it failed.**
- GoLand's own MCP server was probed live: 44 tools, and `open_file_in_editor`
  takes a path with **no line or column parameter**. There is no scroll tool, no
  caret tool, no selection tool, and no close tool. File granularity only.
- Printing the anchors into the IDE's integrated terminal via
  `execute_terminal_command`, betting on JetBrains linkifying `path:line`. The
  command ran (exit 0) but the result was judged not useful.

**Proposed shape.** A code anchor is a first-class thing a card can contain:
a path, a line, an optional column, and a one-line reason. Clicking it opens the
file at that position in the configured editor. AgentBox runs the launcher itself, so
there is no round trip through the agent and the anchor still works after the
agent has moved on or exited. Anchors belong in `show_document` markdown (some
link syntax AgentBox recognises and rewrites) and as a native element artifacts can
emit.

**Configuration.** An editor launcher command with placeholders, defaulted per
project or globally, since the invocation differs per IDE and the argument order
is fussy (see Mechanics). Falling back to `$EDITOR` or `xdg-open` when unset.

**Workaround available today.** `show_artifact` renders the map as interactive
HTML, `await_artifact_event` blocks until a click, the agent receives the event
and runs the launcher. This works and needs no new AgentBox code, but it costs an
agent round trip per anchor and dies the moment the session ends, which is
exactly when a review document is most useful.

---

## FR52 [field] Review walkthrough card - subsumed by FR58

**Status.** Kept as the minimal first slice of FR58, not as a competing design.
Build it only as a stepping stone, and only if FR58 is too big to start
directly. Everything below still holds; FR58 is the same idea taken seriously.


**Session.** Same one. Once anchors are clickable the natural next thing is a
surface that tracks a walkthrough rather than dumping a list.

**Proposed shape.** An ordered set of anchors, each with its reason, presented
one at a time or as a list with state: seen, flagged for discussion, skipped.
Progress is visible ("7 of 26"). At the end the agent gets back the flagged
subset, which becomes the actual conversation. This turns a review from "here is
a wall of locations" into a walk with a result.

**Why it is a card and not just an artifact.** The state should survive the
session that created it, the same way the inbox does. A review interrupted at
anchor 7 should be resumable, and the flagged set is worth keeping in history
(FR6) next to whatever was decided.

**Relation to FR51.** FR52 is worthless without FR51 and should not be built
first. FR51 alone is already useful.

---

## FR53 [field] Window targeting and focus

**Session.** 2026-07-27, same review. Driving the human's view means acting on
an application that is not AgentBox: find GoLand, bring it to the front, and only then
send keys to it.

**What is missing.** `drive_desktop` can already find a window by title, but
`window TITLE` only points the coordinate frame at it (`Hand.UseWindow` in
`internal/hand/hand.go`). It does not raise the window, does not activate it,
and does not give it keyboard focus. Input goes to whatever holds focus already,
which is why the tool's own guidance is to click first. Inside a code editor
that click is not free: it moves the caret and can drop a selection, so the
"click to focus" workaround corrupts the thing being navigated.

Title matching is also the wrong handle for a long-lived app. A JetBrains window
title carries the current file, so it changes as soon as the view moves. The
stable identity is the window class or application id.

**Proposed shape.**
- A `focus` step (or an option on `window`) that raises and activates the target
  and waits until it actually holds focus before the next step runs, rather than
  firing keys into a race.
- Match on a stable identity, window class or app id, with the title as an
  optional narrowing filter for picking between several windows of the same app.
- **Target by process id when precision is needed.** Title and even window class
  are ambiguous the moment two projects are open in the same IDE, or two IDEs
  from the same vendor are running. A pid is exact. X11 exposes `_NET_WM_PID` on
  most windows, so a window can be resolved to a process and back, and the agent
  can pick the pid from the process list when it already knows which one it
  means. Requirement: accept a pid as a target wherever a title is accepted, and
  when a title or class match is ambiguous, report the candidates with their
  pids and refuse to guess rather than acting on the wrong window. Falling back
  to a match is fine when there is exactly one candidate; silently picking the
  first of several is not.
- A way to ask what is running and focusable without acting, so the agent can
  branch instead of guessing.
- Launch the app when it is not running, given a configured command per app id,
  and wait for its window to appear.

**Platform note.** The current implementation is X11 through XTEST. A Wayland
session has no equivalent for either synthetic input or foreign-window
activation without a portal, so this requirement needs the 04-platform.md
answer settled before it can be relied on.

---

## FR54 [field] Knowing what the target application has open

**Session.** Same. Deciding how to reach a location depends on whether the file
is already open: focus and jump, or open it first.

**Proposed shape.** Report the target application's open documents and which one
is active. Prefer the application's own interface over reading the screen: a
JetBrains IDE answers this exactly through its MCP server
(`get_all_open_file_paths` returns the active editor plus the other open
editors), and that is authoritative in a way a parsed window title is not. Fall
back to the window title only for applications with no such interface, and mark
the answer as a guess when it comes from there.

**Why it belongs in AgentBox rather than in the agent.** The agent can query an IDE it
happens to know about. AgentBox is the component that already knows which window is on
screen, so it is where "what is the human actually looking at" belongs, and it
is reusable across applications.

---

## FR55 [field] Guided navigation to a location

**Session.** Same. This is the verb FR51's anchors call.

**Proposed shape.** Given a file, a line and an optional column, put that
location in front of the human by the best route available, and report which
route was taken:

1. File already open in a focused-capable window: focus it (FR53), then the
   application's own go-to-line path. For a JetBrains IDE that is `ctrl+G`, the
   line number, `Return`.
2. File not open: launch through the IDE's own opener, which lands on the line
   directly (see Mechanics), then focus.
3. No IDE available: fall back to `$EDITOR` or `xdg-open` and say so.

"Best possible" also covers what the view looks like on arrival. Landing with
the target line at the very bottom of the viewport is a miss. Centre it, and
consider a brief selection or highlight of the line so the eye finds it without
counting rows. Whether the IDE can be asked to centre, or whether it takes a
scroll nudge, is an open question to answer during design.

**Failure reporting matters here.** A navigation that silently does nothing is
worse than one that refuses: the human is told to look at something and is
looking at the wrong thing. Every route needs a positive confirmation that it
landed, not an exit code (see the launcher trap in Mechanics).

---

## FR56 [field] Export a panel's content to a self-contained file

**Session.** 2026-07-27. Occasional, but valuable when it happens: a document or
artifact on screen is worth keeping, sending on, or attaching to something.

**Proposed shape.** An export control on document and artifact panels that
writes the content out as a standalone, self-contained file. Markdown panels
export as markdown, or as HTML when the rendered form is the point. Artifacts
export as a single HTML file with styles, scripts and images inlined, so it
opens correctly with nothing else beside it. One file, no sibling assets, no
network dependency.

**The hard constraint: it must not steal focus.** This is a rarely-used control
on a surface whose whole purpose is to not interrupt. Exporting must not raise a
window, must not open a file dialog that grabs the keyboard, and must not move
the pointer. Write to a predictable location with a derived filename, confirm
with something ambient (a brief inline state change on the icon itself, or an
existing low-severity channel), and let the human find the file later. If a
destination ever needs choosing, that belongs in configuration, set once, not in
a modal at export time.

**Discoverability without noise.** The icon should be quiet, present on the
panel rather than in a menu, and unobtrusive enough that it costs nothing on the
majority of panels where it is never touched.

**Field note, 2026-07-27.** In the hand-built review kit the export equivalent
turned out to be the highest-value control on the page, and it was not a file
export: it was "copy my review to the clipboard as markdown", so the whole
annotated pass could be pasted back to the agent in one paste. Both belong here.
A file is for keeping; a clipboard copy is for handing back. The clipboard form
is the one that gets used every session, so it should not be the afterthought.

---

## FR57 [field] Letting the agent see the screen, cheaply

**Session.** 2026-07-27, prompted by the question of how the agent sees the
monitor at all.

**Current state, stated plainly.** It does not. AgentBox exposes no screenshot to
agents. Cards can *display* an image the agent supplies
(`01-requirements.md`, and `internal/webui/images.go` caps the size), and
`tools/uidrive/uidrive.py` can grab a window during development, but there is no
path from "what is on Boris's screen right now" to the agent. Anything the agent
knows about the desktop today, it knows because it was told.

**Why a naive screenshot tool is the wrong first answer.** A full-resolution
capture of a large display is expensive every single time, and it is re-sent on
later turns unless carefully managed, which is the exact problem
`internal/webui/images.go` already comments on for card images. It is also a
poor representation: the agent needs to know *what* is there, and pixels make it
infer that from rendering.

**Cheaper and more effective options, in preference order.**
1. **Ask the application.** Structured state beats pixels: a JetBrains IDE
   answers "what is open, what is active" exactly (FR54). Precise, tiny, and
   correct.
2. **Read the accessibility tree.** On Linux, AT-SPI exposes window structure
   and text content as data rather than an image. Worth evaluating as the
   general answer for applications with no API of their own: far cheaper than a
   capture and directly queryable. Needs a check of what is actually reachable
   on this desktop, and how much a JetBrains window exposes through it.
3. **Targeted capture as the fallback.** When pixels are genuinely the point
   (layout, a rendering bug, a visual diff), capture a named window or a region
   rather than the whole display, downscale to something legible rather than
   native resolution, and make each capture a deliberate act rather than an
   ambient feed.

**Requirement.** Whatever is built, the agent should be able to state which of
the three it used, because the confidence attached to the answer differs.

**Privacy.** This reads the human's screen. The same restraint the rest of AgentBox
applies to interruption applies here to looking: capture on request, never on a
timer, and log that a capture happened without logging its content, mirroring
how `drive_desktop` logs the shape of a script but never the text it typed.

---

## FR58 [field] The review board

**Status.** Major feature, deliberately not fully specified yet. Boris will
expand this. What follows is the shape as stated on 2026-07-27, recorded now so
the expansion has something to build on. Subsumes FR52; depends on FR51 and
FR55 for the jump-to-code path. **Read FR61 before expanding this entry**: it adds
the coverage and citation-integrity requirements, found the next day, that this
shape does not yet account for.

**The problem it solves.** Reviewing a body of work with an agent currently
degrades into one round trip per point. The agent raises a location, the human
reacts, the agent responds, repeat. For a feature-sized review that is dozens of
turns, the human loses the thread between them, and the agent never sees the
review as a whole.

**The inversion.** The human should be able to walk the entire change at their
own pace and leave every comment and question along the way, then hand the whole
set back in **one turn**. The agent answers a review, not a remark.

**Stated requirements.**
- **Step-based.** A review is an ordered sequence of steps, of the kind produced
  by hand during this session's SSVC review: a location, and why it is worth
  looking at.
- **Full navigation between steps.** Forwards, backwards, and jumping directly
  to any step. Never a forced linear march.
- **Strong visual design.** This is a surface the human will sit in for a while.
  It should use visual structure properly rather than being a list of links.
- **Select code inside the element and comment on it.** The selection is part of
  the comment: the agent receives the exact file, the exact line range, and the
  exact text the human highlighted.
- **Multi-line comments, several per step.** A comment can be a statement, a
  question, or several questions. More than one comment per location.
- **Everything in one submission.** The human navigates all the changes and
  leaves everything they want to say, then submits once.
- **The agent has access to the board**, so it can populate the steps, and read
  back the comments with their anchors.
- **A reactive window with real tools**, not a static card. Expect it to grow
  options over time.

**Why this is the strongest item on this list.** Every other entry here makes a
single interaction better. This one changes the shape of the collaboration: it
converts a serial conversation into a batched one, and it gives the agent
something it currently never gets, which is the human's reaction anchored to
exact code rather than described from memory.

**Settled 2026-07-27.**
- **The board renders the code itself.** Not a list of pointers to the IDE. The
  code is present in the board, which is what makes marking a region and
  annotating it convenient and what makes a selection precise enough to send
  back as an anchor. Rendering is the primary mode, not a fallback.
- **Each location can also jump into the IDE** (FR55), as an action on a step
  rather than the way the code is read. The board is where the review happens;
  the jump is there for when the surrounding code, the full file, or the real
  tools are wanted.
- **A review persists across sessions, and can be reopened and amended.** So a
  review is a durable, addressable object with its own identity and lifetime,
  not a transient card. It survives the agent that created it and the session it
  was created in, and a partially-completed review can be picked up later and
  added to.
- **The agent gets the whole review as context, not one comment at a time.** It
  can read the steps, the code shown in each, every selection with its anchor,
  and every comment, in order. A reply to one comment is written knowing all the
  others, so answers land in context and can reference each other rather than
  being answered blind and in isolation. This is the point of batching: the
  agent responds to a review, not to a remark.

**The agent supplies content, AgentBox supplies the mechanics.** This is the load-bearing
requirement, stated 2026-07-27 after the hand-built kit made the alternative
obvious. The agent must not author a UI. It submits a **declarative review spec**
(structured data: ordered steps, each with a title, its one question, the
requirement and design links it serves, code references, prose, runnable checks,
and self-check question/answer pairs) and AgentBox owns everything else.

AgentBox owns, so that no agent ever writes it again: layout and visual design, the
rail and progress display, the three-valued per-step state, per-step note
capture, selection-to-comment, show/hide of answers, copy controls, code
rendering with line numbers and highlighted lines, path handling, the IDE jump
(FR51/FR55), persistence, and the return payload.

Three reasons this split is the right one, all of them learned rather than
assumed:

1. **The wheel is genuinely large.** The hand-built kit needed a stylesheet, a
   script for state and navigation, and a per-page skeleton before a single word
   of review content existed. An agent re-deriving that each time will produce a
   different and worse version every time, and will spend most of its effort on
   mechanics rather than on the review.
2. **Improvements have to reach past reviews.** If the review is data, a better
   renderer improves every review ever submitted, including ones already
   annotated. If the review is a rendered page, every improvement is a rewrite
   and old reviews rot. This is the whole reason to want a framework rather than
   a template.
3. **Annotations must survive a framework upgrade.** The human's marks and
   comments attach to step identities in the spec, not to positions in generated
   markup, so AgentBox can change how a step looks without losing what was said
   about it.

**Corollary: reference code by path and line range, not by pasted snippet.** The
hand-built kit embeds frozen copies of the code, and they are stale the moment
the code is edited, which is exactly what happens during a review. If a step
says "`pkg/images/postgres/client.go` lines 2752 to 2792, highlight 2755" and AgentBox
reads the file itself at render time, the review stays true as the code changes,
a reopened review shows current code, and a selection the human made can be
re-anchored rather than pointing into a snapshot. Inline snippets should be the
fallback for code that is not in a file (a proposed diff, an external payload),
not the normal case.

**Step kinds, from the ones that turned out to be needed.** A small vocabulary
rather than one generic step: a **ground** step (vocabulary and rationale, does
not count toward the total), a **code** step (the normal one), a
**nothing-to-review** step (explicitly closing off scope), and a **check** step
(a command to run, with an expected result). Expect this list to grow one entry
at a time as reviews demand it, which is the intended way for the framework to
mature.

**The medium is AgentBox's choice, not the agent's.** HTML in a reactive window is the
likely answer given M9 and M10 already put a webview and an artifact sandbox in
place, but the agent's contract is the spec, so the renderer can change without
touching a single caller.

**Prose and code must be able to point at each other.** Stated 2026-07-27 after
reading the hand-built kit as a reader rather than an author. A step's
explanation and the code it explains are two separate regions of the screen, and
the reader has to hold the connection between them in their head. They should not
have to. The agent must be able to declare that an element in the prose is bound
to a region of code, so that hovering or clicking it **highlights those lines and
brings them into view**. Reading "the guard on the second branch" should light up
the guard.

This is the difference between a page that contains an explanation and a page
that explains. It also makes long code blocks tractable: the reader never has to
locate a line from a number, because the text they are reading does it for them.
Bidirectional is better where it is cheap: clicking a line should also mark which
part of the prose covers it.

**Two things about code display in every AgentBox output, not just reviews.**
- **A copy button on every code and command block.** Confirmed useful in the
  field immediately. It belongs everywhere AgentBox renders a fenced block, in
  documents and artifacts alike, not only in a review surface.
- **Real syntax highlighting.** The hand-built kit deliberately went monochrome
  and marked only the lines that mattered, which was a reasonable constraint
  when hand-writing markup, but it is not the right ceiling for a framework. AgentBox
  should highlight properly, per language, and keep the "these are the lines to
  look at" emphasis as a **separate** visual channel from syntax colour. Which
  leads to the next point, learned the hard way.

**Diff status and reading emphasis are different channels and must look
different.** The kit used one highlight for "this line is worth your attention",
and the reader read it as "this line changed", which is the more natural
assumption when reviewing a branch. Both questions are legitimate and a review
surface has to answer both at once: *what changed here* and *what should I look
at first*. Two visual channels, plus a legend saying which is which, and never
one channel doing double duty. A step should also be able to state plainly when
the answer is "all of this is new", because that is the common case in a feature
branch and leaving the reader to infer it is what caused the confusion.

**Learned by building one by hand, 2026-07-27.** Before AgentBox had any of this, the
same review was built as a folder of static HTML pages
(`~/Desktop/ssvc-review-kit/`). Everything below is a requirement that only
became visible by needing it, and several would not have made it into a spec
written from the armchair.

- **A step can run something and capture the real output.** The single most
  grounding element was not any explanation, it was a `curl | jq` the human
  could paste and watch. Two consequences. A step needs a runnable check with a
  copy control, a stated expected result, and a recorded date for when the
  expectation was last true. And the board should be able to run it and
  **attach the actual output to the step**, because the real answer differs from
  the predicted one: the hand-built kit says "2 of 100 recently-modified CVEs
  carry SSVC, measured 2026-07-27", and whoever reads it next gets a different
  number. The agent needs the number the human actually saw, not the one the
  agent guessed.
- **Amending a review must not destroy annotations on untouched steps.** The
  human refines the code while the review is open, so the agent will need to
  update step 4 while steps 1, 2, 3, 5 and 6 already carry marks and comments. A
  review is therefore a set of independently addressable steps, not a document
  that gets regenerated. Anything that rebuilds the whole review to change one
  step is wrong.
- **A review needs a ground step, and it must not count.** Code steps assume
  vocabulary. The hand-built kit needed a preamble covering what the domain
  even is, the terms that appear in the code, and the one fact that explains
  every design choice. But the finiteness promise ("six stops, that is the whole
  feature") is what makes the review feel survivable, so the preamble navigates
  with the steps while staying outside the count.
- **Every step declares what it is for.** A step carries the requirement it
  serves and the design section that decided it, as links. Reading code without
  knowing its purpose is a main source of scatter, and "which requirement is
  this line serving" turned out to be the question that stopped the drift.
- **Steps that say "there is nothing here to review" are first-class.** Three
  parts of this feature needed no code, and saying so up front prevented a hunt
  for files that do not exist. A review needs to close off territory explicitly,
  not only point at territory.
- **Comprehension checks belong in the step.** Two or three questions per step
  with the answer hidden until asked for, so the human tests themselves rather
  than assuming the read landed. Whether they were answered is part of the
  step's state.
- **State is three-valued, and the middle value is the product.** Unread,
  understood, and unclear. The set of "unclear" steps is what the review is
  actually for; everything else is bookkeeping. Surface that set as the headline
  result, not as a filter buried in a list.
- **A free-text sentence per step, written before moving on.** It is a forcing
  function: a step whose sentence cannot be written is precisely the step that
  needs another pass, which is information the human would not otherwise have.
- **Anchors always carry the full repo-relative path.** Never a basename. This
  repository has two `client.go` and two `client_test.go` in play in a single
  review, so `client.go:1367` is ambiguous and dangerous. The UI must not
  truncate a path in a way that drops the package either.
- **Progress is visible without asking, always.** The rail showing all steps at
  once, with the current one marked and a running "n of six understood", is what
  made the thing feel finite. Nothing may silently grow the step count
  mid-review; that breaks the promise the count made.

**Mock round 1, 2026-07-28.** The first clickable mock
(`tools/mockups/review-board.jsx`, an AgentBox artifact loaded with the real
session-25 diff) went in front of Boris the day it was written. His verdict on
the shape: "the general idea seems correct". Decisions and additions from that
round, each now in the mock:

- **The board takes the whole screen by default, and any size must be
  possible.** "Screen real estate is needed in code review." Artifact windows
  now open maximized (shipped, `viewer.go`); a real resize affordance on
  frameless surfaces is still owed (see STATUS queue).
- **Syntax highlighting is not optional.** The monochrome first draft was
  called out within seconds. The mock got a toy tokenizer; the real board gets
  chroma, which M6 already provides everywhere else.
- **A comment must be visible from the code it annotates.** Hovering a
  commented line shows the comment, and commented lines carry a gutter mark.
  Comments are editable and deletable; the first draft allowed neither.
- **Reveal must be reversible.** A comprehension check's answer can be hidden
  again, so the reader can re-test rather than un-know.
- **The comment composer speaks keyboard.** Ctrl+Enter adds, Esc discards.

The round also proved the working rule itself: within minutes of real use the
mock flushed out three AgentBox defects that were not the board's - the artifact
code/preview toggle trapped the reader off-screen (a flex min-width hole), A+/A-
never reached a sandboxed artifact, and the frameless window read as
fixed-size. All three fixed and deployed the same day (d656eeb).

**Round 2, the same day.** The zoom fix's first mechanism (CSS zoom on the
frame) was itself wrong twice over, and Boris caught both within minutes:
WebKit evaluates it only when the frame is built, so A+/A- appeared dead until
reload was clicked ("that's fucked up" - a view control must take effect when
clicked, never on the next rebuild), and its pointer hit-testing drifts, which
corrupted text selection inside the board: comment anchors recorded lines
nobody selected. Replaced with a transform on a compensated box (d4b9672);
verified live at 80/110/130% with a drag-selection landing on exactly the
dragged lines. Also decided: **the preview/code/reload chrome does not render
on an artifact-as-document page** - the window bar already names the thing and
a working surface is not flipped to source from inside itself. Inline
artifacts in conversations keep all three; the note span stays everywhere
because compile errors land in it.

**Round 3, the same day.** Submission got its priorities corrected on Boris's
challenge ("why do I need copy my review, and why is it the way to send it"):
the clipboard-first framing was inherited from the static kit, where the
clipboard was the only channel back. In AgentBox the board has a real channel, so
**Submit to the agent is the one primary act** and copy-as-markdown is demoted
to an export nicety (the FR56 field note's "clipboard first" is superseded for
reviews). Also decided, each in the mock now:

- **The full repo path copies in one click** - the path in the code panel's
  header is itself the copy control, confirming inline.
- **The comment composer opens at the selection**, inside the code panel,
  never at the bottom of the step: the eyes stay where the code is.
- **Disclosure controls are icons, not words** (reveal/hide became ▸/▾, the
  question line itself clickable).
- Found and filed: the viewer's find cannot see into a running artifact
  (STATUS queue); moot for the real board, which will be an AgentBox surface with
  native find, but real for `agentbox show --artifact` generally.

**Adversarial UX pass, 2026-07-28.** Requested by Boris after round 3: attack
the mock against the user story (walk the whole change alone, say everything
along the way, hand it back once, the agent answers a review) and the stated
requirements. Findings, worst first.

*Story-critical - the promise breaks here:*

1. **A comment requires a code selection.** A step-level remark ("this whole
   approach worries me") has nowhere to go, and ground, nothing-to-review and
   gate steps take no comments at all. The requirement says several comments
   per step; the mock delivers several comments per selection. The board
   needs a per-step comment affordance beside the closing sentence.
2. **Unclear can ship hollow.** Marking a step unclear demands nothing - no
   sentence, no comment - so the headline set the whole feature exists for
   can reach the agent as bare labels to guess at. The middle value is the
   product; it must carry at least one sentence, nudged if not enforced.
3. **The handback has no receipt.** Submit emits an event; unless an agent
   is already blocked waiting, nothing tells the reviewer who received it,
   when, or where it now lives. The still-open delivery question below is
   the single blocking design decision for the real build - and whatever the
   answer, the submission moment needs an acknowledgment in the board.
4. **The payload flattens to markdown.** The agent should get structured
   data (steps, states, anchors as path plus range plus exact text,
   sentences, comments) with markdown as the export rendering only.
   Anchors AgentBox kept exact should not be re-parsed out of prose.
5. **Deletions are unrepresentable.** The code model is lines with add
   flags; a real branch removes code, and a reviewer approving deletions
   never sees them. The spec needs the removed side of a change, or at
   least a step-level statement that deletions exist and where.
6. **Window-local state makes even mock rounds single-sitting.** No storage
   exists in a sandboxed opaque origin, so this cannot be shimmed; it
   confirms persistence as the first slice of the real build, not a
   later refinement.

*Frictions - would chafe within one sitting:*

7. **The keyboard is almost absent, and invisible.** Arrows navigate but
   nothing says so; verdicts, reveal and comment flow are mouse-only. AgentBox's
   whole card vocabulary is keys; the board wants j/k, u/x for the
   verdicts, a next-unread jump, and a visible hint line like a card's
   footer.
8. **The story ends at the gate, the submit lives in the header.** After
   the last station the eye is at the bottom; the submission belongs there
   too, with the header button as the shortcut.
9. **Excerpts are fixed windows.** No expand-context control around a cited
   range. Falls out free once AgentBox reads files by path at render time; named
   here so the real build does not forget the control.
10. **Agent replies have no home.** The board shows the human's half of the
    conversation only; comment cards need reply space sketched before the
    real layout hardens (see the still-open item below).
11. **Amendment UX does not exist yet.** "Amend without destroying
    annotations" is stated but has no surface: what does the reviewer see
    when step 4 changed under their marks? Wants its own mock round.

*Fixed during the pass itself:*

- The double composer Boris screenshotted (the old bottom composer stayed
  behind when the popover landed) - removed; one composer, at the selection.
- A selection near the panel's fold opened the composer below the visible
  area - it scrolls itself into view now.
- Esc destroyed a written draft - Esc now only closes an empty composer;
  only Discard throws words away.
- Coverage and drift were hardcoded but looked computed, teaching trust in
  numbers nobody computes - labeled "illustrative in the mock" until they
  are real (FR61: computed, not claimed).
- Path copy yields the absolute path from the filesystem root (display
  stays repo-relative per the anchor rule; copies leave the repo's frame).

*Smaller notes:* a one-click "copy anchor" (path:from-to) is wanted next to
the path copy; non-code steps could take a light "seen" state so ground
silence stops being structural; the skippable closing sentence needs a rule -
soft-mark "understood, unsaid" or hard-block - and that is Boris's call.

**Round 4, 2026-07-28.** Four feedback items, one of them a real AgentBox feature
shipped the same hour, plus an adversarial pass Boris ordered on that feature.

- **The closing sentence is a closing note: multiline, any length.** "Why
  only one sentence, it should be multiline and let me add whatever text I
  need." The forcing function survives as a floor, not a cap - write
  something before moving on, at whatever length the step deserves. The
  mock's verdict box is a growing textarea now; the payload renders a
  multiline note as a full blockquote and collapses it to one line only in
  the unclear-headline list.
- **Find reaches the running board - shipped in AgentBox itself, not mocked**
  (STATUS queue item 4, closed; Boris picked the fix over the disclaimer:
  "the find does practically nothing, I should be able to search the code
  block or blocks"). The viewer forwards the query into the sandboxed frame
  over the existing postMessage style channel; the frame walks its own text
  nodes, paints hits with the CSS Highlight API (wrapping in <mark> would
  mutate React-owned nodes and break the next render), scrolls the current
  hit through nested scroll containers, and answers with n/m for the find
  bar. The adversarial pass on this feature found and fixed three breaks
  the first happy-path build hid:
  1. *The focus trap.* After one click inside the board, every keystroke
     lands in the sandbox and the window never hears Ctrl+F again - find
     could not even be opened from the keyboard. The frame now forwards
     Ctrl+F, `/` (outside editables) and F3/Shift+F3 out as chords, in
     capture phase so artifact code cannot swallow them.
  2. *Scope: the review is bigger than the viewport.* The board renders one
     step at a time, so a plain frame-search answers "no matches" for text
     that lives two stops away - and silence must never read as "nowhere in
     the review" (FR61's rule, again). The board listens for the same find
     message and answers with an "also in: <step> (n)" strip; clicking a
     chip opens that step, and a MutationObserver in the frame re-runs the
     search over the new DOM. **Requirement for the real board: find
     searches the whole review - every step's prose, code and notes -
     grouped by step, and jumping to a hit opens its step.**
  3. *Marks died silently on re-render.* Highlight ranges anchor to text
     nodes; a framework re-render replaces nodes and the marks vanish
     mid-read. The frame re-finds (debounced, position held, no scroll
     theft) whenever the document mutates under an active query.
- **Numbered annotations replace the look-here diamond** ("better than the
  blue diamond that does almost nothing"). The agent can mark regions of a
  code block with an explanation that pops on click, right where the code
  is, instead of burying the why in comments or in prose paragraphs the eye
  has to leave the code to find. Spec addition per block:
  `notes: [{at: [from, to], text}]`, auto-numbered across the step. In the
  mock: a numbered badge sits at the end of the region's first line; click
  pops the note beside the code; the pop is draggable by its header (so it
  never has to cover the code being read), closes with ✕, and the number
  reopens it. Hovering a pop lights its code region - the binding channel
  and the annotation channel agree. The two-visual-channels doctrine
  survives: diff status stays in the gutter, and reading emphasis is now
  the annotation numbers, which carry content instead of a bare glow.
- **Deleted and changed code are representable** ("the examples you show
  have only new additions"). A block may carry
  `del: [{after, old, lines}]`: removed lines render where they died,
  red-tinted, numbered from their OLD file line - and a changed line is a
  deletion adjacent to an addition, which is how the mock now shows the
  real `Title: "agentbox"` → `Title: title` change from the reviewed diff.
  The stage step shows the diff's real deletion: seventeen lines of
  latin_layout() reduced to a four-line comment. Consequence recorded for
  the spec: **a comment anchor needs a side** - old or new file - before
  deleted lines can take comments; the mock does not allow commenting on
  them yet.
- **A step holds one code block or several** ("the code block or blocks").
  `code:` became `codes: [...]`; each block carries its own path copy,
  range, IDE jump and new/added/deleted marking. The titles step now tells
  its two-commit story in three blocks in reading order.
- **FR61 caught our own mock.** Re-verifying every cited range against the
  tree before this round found two stale anchors: the Type-loop block
  claimed hand.go:379 for a loop that sits at 381, and the stage block
  claimed perform.py:171 for a function that starts at 179. Both fixed;
  both are the exact silent-staleness FR61 exists for, and the real board
  reading files by path at render time is what makes the class impossible.

**Round 5, 2026-07-28.** Four items on the round-4 build, poked live.

- **Esc discards the comment draft, written words included.** Owner's call,
  overturning the round-3 guard ("Esc never destroys written words"): his
  Esc means "off my screen", and the Discard button stays for the mouse.
  The protective behavior read as a broken key, which is worse than the
  loss it prevented.
- **The hover jitter, diagnosed and fixed.** The comment-peek footer
  rendered INSIDE each code panel's scroll layout (sticky bottom), so
  crossing a commented line toggled the scroller's content height and
  scrollbar, shifting the lines under the pointer; it also rendered in
  every block of the step, not just the hovered one (visible in Boris's
  screenshot as the same comment under two panels), and hover state was
  re-set on every line the mouse crossed. Now: the footer renders below
  the scroller, only in the hovered block, and hover state only changes
  when the hovered comment set changes. Rule for the real board: nothing
  that appears on hover may change the layout under the pointer.
- **Annotation pops are wider** - min(460px, 92%) with the body scrolling
  past 40vh - because explanations are expected to be verbose. The pop
  stays draggable; the width is for reading, the drag is for placement.
- **A+/A- changes the font size, not a zoom level, and the base comes from
  configuration.** Mechanism four, owner's call: A+ means bigger text, not
  a magnifier. The sandbox document's rem root now comes from `[font]
  size_pt` (via --k-size, the same knob every AgentBox surface reads); A+/A-
  scales that base, so rem-sized text re-lays-out crisply while px-sized
  chrome holds still. Consequences: the injected base style's hardcoded
  14px default is gone - artifacts now inherit AgentBox's configured type size -
  and an artifact that sizes its text in px has opted out of scaling. The
  mock's type is rem throughout for exactly that reason.

**Round 6, 2026-07-29.** The owner, on the round-5 board: the step rail "small
and not easy on the eye", "the numbers are dim and it's all dim in general",
"Submit review is yellow for some reason", the window "doesn't feel ergonomic",
and "maybe we can make smart use of the dead space outside the code for
additional content". Applied same-day: rail widened with the worklist inversion
(unread bright, understood quiet, current accented - the same rule FR62's diff
card ships), numbers render in ink, first grey-floor lift, submit re-colored to
action blue (amber now means unclear and nothing else), and the agent's
numbered whys sit open in the right-margin dead space on wide windows (hover
lights their lines; the chip flashes its note; click-to-pop unchanged on
narrow). Two further owner calls the same day: the 130% render is the new 100%
("anything smaller than this font is too small... it should start from this by
default"), and "bring in an expert on UI/UX".

**Round 7, 2026-07-29, applied in full over two sessions.** A dedicated design
pass (the requested expert) produced a spec, applied item by item and then
deleted per its own header (git keeps it; what it decided is here). The
stance: the board is a reading room, not a dashboard. Prose and code get
book-page size and contrast, chrome recedes, and the only saturated things
are the four meanings (blue look/action, amber unclear, green ok, purple
human), made brighter rather than more numerous. What shipped: a full type
re-scale (the owner's 130% render is the new 100%), a measured contrast ramp
(ink on bg 13.8:1, mut 8.2:1, dim on code 5.2:1, comments 5.4:1, deleted-line
numbers 4.6:1, look 6.9:1, unclear 8:1), a verdict-moment package - Understood
and Unclear rest in tints of their own meaning and fill on selection, the
next button lights action-blue once a verdict exists, no auto-advance -
u/x/Enter keys with a footer bar naming every key, header progress pips that
fill per verdict, a submission receipt in the modal (delivered-at time plus
the understood/unclear/comments tally; the adversarial pass's "the handback
has no receipt", closed), rail stations as real focusable buttons on a wider
rail with two-line titles, a 58rem reading column, code scrollers capped at
min(60vh, 40rem) (the vertical dead-space answer), and margin notes at book
size. Applying it fixed the two latent bugs the pass had caught: the gate's
copy button was dead (wired, with the copied-check swap), and the mock's own
style block silently beat Tailwind's text sizes by document order, so every
size the spec calls out on a prose/mono/button element is set inline - the
one cascade level the sandbox respects unconditionally. Amber audit passed:
amber means unclear and nothing else (coverage's uncovered segment
legitimately shares the hue); blue stays look/action only. Two notes for the
real board: "wide" must derive from container width vs rem-derived need, not
a px media query; and with the 1.3x scale baked in, a `[font] size_pt` still
carrying the owner's pre-round A+ zoom renders the board at 1.69x until A-
restores it - the base scale must live in exactly one place.

**The real build (2026-07-29, ADR-0012).** Slice 1 shipped: the walkthrough
store (spec and annotations in separate SQLite tables, joined by step ids),
spec validation with teaching errors, the diff-as-manifest rule, daemon-side
file reading with per-line chroma, the native board window and surface, and
`agentbox walkthrough create/open/list/read/delete`. Decided with the owner at
kickoff: delivery takes BOTH paths (submit resolves a waiting
`await_walkthrough`, otherwise persists for exactly-once pickup); the
closing note hard-blocks only for unclear (understood may stay silent,
marked `unsaid`); anchors carry an old/new side from day one. Remaining
slices: handback+MCP tools, annotation display, find, coverage/drift,
amendment.

**Still open.**
- Comments ON deleted lines in the surface (the schema already carries the
  side; the selection flow does not offer del rows yet).
- How agent answers come back into the board, so a reopened review shows what
  was already answered next to what is still outstanding.
- A resize story for frameless surfaces: edge grips or a double-click-to-restore
  on AgentBox's own title bar. Today it is maximize-by-default plus the WM's own
  Alt+F8 / Super+middle-drag, which is functional and undiscoverable.

---

## FR59 [field] Walkthrough library: persist, load, search, amend, delete

**Session.** 2026-07-27. A walkthrough is expensive to author. The one built by
hand in this session took most of a working session to produce and revise, and
the revisions were where most of the quality came from. Throwing that away at the
end of a session is the thing to prevent.

**Proposed shape.** A walkthrough is a durable, named object in AgentBox, with the
operations that implies:

- **Save** on submission, automatically, not as an explicit act the agent might
  forget.
- **List** what exists, most recent first, with enough metadata to recognise one:
  title, subject, when it was made, how far through it the human got, how many
  steps are flagged unclear.
- **Load and reopen** any of them, with the human's annotations intact and
  progress where they left it.
- **Search**, and this is the one that makes the library worth more than a
  folder. By title and subject, by content, by the code paths the steps
  reference, and by state (show me everything still flagged unclear). The value
  compounds: a walkthrough of a subsystem is the natural starting point for the
  next piece of work in that subsystem, but only if it can be found six weeks
  later.
- **Delete**, including a way to clear out the ones that were superseded.

**Agents must be able to work on a stored walkthrough, not only create one.**
Load an existing one, revise a step, add a step, correct an explanation the
reader found confusing, and save it back. That is how a walkthrough improves:
incrementally, in response to what the reader said about it, exactly as this
session's stop 1 was rewritten twice from feedback. Regenerating from scratch
each time is both wasteful and destructive, so amendment must preserve the
human's annotations on steps it did not touch (see FR58).

**Why it belongs next to the inbox.** AgentBox already persists items and history
(FR6), so this is an extension of a model that exists rather than a new one.

---

## FR60 [later] Export a walkthrough as standalone HTML

**Deliberately deferred**, stated 2026-07-27: implement only once in-app
walkthroughs are good. The reasoning is the same as the mock-before-building
rule. Exporting a format that is still moving bakes the wrong shape into a second
implementation and doubles the cost of every subsequent change. Get the in-app
experience right, let the step vocabulary settle, then export.

When it happens it is FR56's machinery applied to a multi-part document rather
than a single panel: one self-contained HTML file, or a folder with an index,
with everything inlined and no network dependency. The hand-built kit in
`~/Desktop/ssvc-review-kit/` is the reference for what the output should feel
like, and its own structure (a shared stylesheet and script, one file per step,
metadata in one place) is a reasonable starting point for the generator.

---

## FR61 [field] Coverage, and proof that a review is still true

**Session.** 2026-07-28, one day after the six-stop SSVC kit, and two problems
the stops had hidden. First, a kit can explain a feature well and still never
answer "have I looked at everything?", so a second surface was built:
`review-map.html` ("ROUTE"), 17 stations in commit order, each citing an exact
`path:lines`, together covering every changed line on the branch once. Second,
later the same day two comments in the reviewed code became one-liners, and every
citation below them was silently wrong.

**What AgentBox could not do.** None of this exists yet, so this is input to FR58
rather than a gap in a shipped tool. But FR58 as written measures progress through
curated steps ("4 of six understood") and never states what the curation covered,
and it assumes one traversal per change. Neither part survives contact with a real
branch.

**What was done instead.** The route was authored by hand. Its 44 cited ranges
were checked by a throwaway script (`verify-lines.py`) that greps each range for a
pattern it must contain, and its per-station marks were kept in a separate
localStorage namespace so they could not collide with the stops' marks. When the
code shifted by two lines the script caught two stale ranges. It missed thirteen
more, because those were numbers written inside sentences, and it could not catch
its own: the 44 ranges were hardcoded a second time in the script itself, so
fixing the page left the checker failing. Two references had also been wrong since
the day they were written (three lookups cited as 688/691/694 when they were
684/687/690) and nothing had ever looked.

**Proposed shape.**

- **Coverage is computed, not claimed.** Given the branch diff and a spec whose
  steps cite path and line range, AgentBox can say which changed hunks no step covers.
  Show that next to the progress count, and let a spec mark a hunk deliberately
  out of scope so the remainder means something. A thematic review can be finished
  and still have missed half the diff, and only the tool is in a position to
  notice.
- **A change can carry more than one traversal.** A complete shallow pass in
  commit order, plus thematic deep dives over the same material, cross-linked,
  each with its own state and its own completeness claim, and marks in one never
  touching marks in the other. They answer different questions: "is all of it
  accounted for" and "how does it work".
- **The submission names what was not reviewed.** The route's "copy my findings"
  emits the notes and an explicit not-yet-reviewed list, and that second list is
  what changes how an agent reads the result. Silence has to mean "not looked at",
  never "looked at and fine", because the agent cannot tell the two apart and will
  assume the flattering one.
- **"Reviewed" needs a terminal definition.** The route's last station is the gate
  sequence: build, staticcheck, the two test commands. A spec should be able to
  declare its own exit criteria as a final check step, so that finishing is an
  observation rather than a feeling.
- **A review is pinned to the commit it was authored against, and drift is
  reported.** Record the SHA in the spec; on reopen, compare against the tree and
  show what moved instead of rendering confidently wrong numbers. This is the
  second and harder argument for FR58's "reference by path, not by pasted
  snippet": pasted code goes stale visibly, while a cited line number goes stale
  silently and still looks authoritative.
- **Nothing may hold a second copy of a citation.** The checker reads the spec, it
  does not restate it. Any place that repeats a range is a place that will
  eventually disagree with the others, which is precisely what happened.
- **A line number must never appear as a literal in prose.** "The guard on line
  690" is a fact with a shelf life of one edit. Prose should point at a bound code
  region (FR58's prose-to-code binding) and let AgentBox render the current number, or
  render no number at all and just highlight. That promotes the binding from a
  reading convenience to a correctness requirement.

**Why it matters beyond this review.** The route was singled out in the field as
the most valuable part of the kit, and the reason is worth keeping: it converts
"here is an explanation of the feature" into "here is a finite, ordered, verified
path with a defined end". The explanation is what lets a reviewer understand the
change. The coverage is what lets them sign it.

---

## FR62 [shipped 2026-07-29] Structure in the diff card

**Session.** 2026-07-29. Boris, on the review card: the structure was the
dimmest ink on it, "quite off and not engaging", no felt value over an HTML kit
on the desktop. The card rendered a patch as one flat wall of mono text.

**What shipped (session 28).** The card parses the unified diff it already has -
hunk line counts disambiguate content from headers - and renders per-file
sections with sticky headers, add/del stats and new/deleted/renamed badges.
Past one file, a left rail lists every file as a step: click to jump, n/p from
the keyboard, scroll-spy tracks the current file, seen files go quiet while
remaining ones stay bright. Diff cards open at 560px, 780px with the rail. The
note field is a growing textarea (his first real note did not fit) and Esc in
it hands the keyboard back to a/r instead of deferring.

**The boundary that stays.** The card is for reviews an answer away; the
walkthrough kits (FR58/FR59) are for reviews that need teaching. The card gets
structure for free from the diff text - zero extra agent tokens - which is the
value the flat wall was hiding.

---

## FR63 [shipped 2026-07-30] The board drops three things the spec accepts

**Session.** 2026-07-30. First walkthrough authored against a work repo rather
than against AgentBox itself: one commit, one new 637-line Go file
(`cmd/ssvc-backfill`), eleven steps, eight of them counted. The spec validated
with no warnings, stored, and opened. Then the render was compared against what
had been authored.

**What AgentBox could not do.** Three fields the spec accepts, the validator checks,
and `boardrender.go` puts on the wire are never consumed by any frontend
component. Verified by reading the components, not inferred from the screen.

1. **Notes are invisible.** `renderBlock` numbers every note and ships
   `wireCode.Notes`; `frontend/src/lib/board/CodeBlock.svelte` contains no
   reference to notes at all. Forty-one notes in this review, each one the "why"
   anchored next to the lines it explains, render as nothing. This is the single
   most costly gap, because authoring rule 9 (answer the objection in place) and
   rule 3 (lead-in, code, takeaway) are both built on notes. An author following
   the standard spends most of their effort on the one channel the reader cannot
   see.
2. **Binds do nothing.** `ws.Binds` ships name to `[block, from, to]`;
   no component reads it. The bound phrase renders as ordinary text and lights
   no code. This one is worse than a missing feature: the validator *refuses*
   a literal line number in prose and instructs the author to bind the phrase
   instead, so the spec enforces a discipline whose entire payoff is unbuilt.
   Rule 18 is currently a cost with no benefit.
3. **Prose has no paragraph break.** `Step.svelte` renders prose as one
   `<p class="prose">` of inline spans. Consecutive text segments fuse with no
   separator at all: seven paragraphs of orient / explain / objection / takeaway
   came out as `nothing else.Everything you are about to see is new.` - one wall,
   with words joined across the seam. Segments are inline by design (a bound
   phrase must sit mid-sentence), so this is not a CSS fix; the spec has no way
   to say "new paragraph".

**What was done instead.** For the fusing, a post-pass over the built spec
inserts a literal ` · ` wherever a segment starting with a capital follows a
segment ending in sentence punctuation. Forty-five insertions in eleven steps.
It reads acceptably and is one line to strip later, but it puts presentation
into content, which is exactly what the spec exists to avoid. For notes and
binds there is no workaround: the content was left in the spec (it is stored
correctly and will light up the day the UI lands) and the load-bearing parts
were duplicated into prose, which is what rule 7 says not to do - explanation
and annotation collapsed into one voice because only one of them renders.

**What shipped (2026-07-30).** All three, plus the layout consequences.

- **Paragraphs.** `Seg` gains `P bool` (`"p": true`): a modifier on a text or
  code segment meaning "start a new paragraph here", not a form of its own, so
  it composes with `bind`. `Step.svelte` groups segments into runs and renders
  one `<p>` per run. The ` · ` workaround is gone from the field review.
- **Notes.** A numbered badge on the note's first line and the text in a margin
  column beside the block. The badge has its own fixed-width column between the
  gutter and the code, so an annotated line's number sits exactly where an
  unannotated one's does - the first attempt put the badge inside the gutter and
  Boris caught it immediately: "the numbered annotations should not ruin the
  line numbers".
- **Binds.** The bound phrase renders as a dotted underline and lights its code
  region on hover or focus, keyboard-reachable.
- **The margin is outside the code frame.** First attempt put it inside the
  border, which read as part of the code and stole a third of the block's width
  (the code then clipped sooner). Boris: "the side text should be outside the
  code when possible". The step now lays its children out on a grid - one column
  for everything it says, one for the annotations - and `CodeBlock` emits its
  frame and its notes as *sibling* root elements, so a block and its notes land
  on the same grid row and stay aligned without either measuring the other. The
  margin folds under the block below 1180px.
- **Typography.** A 68ch measure on prose, 1.68 line-height, and paragraph
  spacing, because at full container width the return sweep loses the line.

Also learned: the authoring standard has to say which channels render, or an
author spends their budget on invisible ink. It now does, and it moved into
`internal/manual/walkthrough.md` where the tool can serve it.

---

## FR64 [shipped 2026-07-30] create_walkthrough's MCP schema is a byte array

**Session.** 2026-07-30, authoring FR63's review. Tried the MCP tool first,
fell back to the CLI.

**What AgentBox could not do.** `create_walkthrough`'s `spec` parameter is declared in
the tool schema as `{"type": ["null", "array"], "items": {"type": "integer",
"minimum": 0, "maximum": 255}}` - a Go `[]byte` marshalled straight to JSON
Schema. The description says "the version-1 walkthrough spec object", so the
description and the schema disagree. A client that validates arguments against
the schema before sending cannot pass an object at all, and the honest reading
of the schema is "send me 62,672 integers".

**What was done instead.** `agentbox walkthrough create --spec FILE`, which takes the
JSON file directly and worked first time. Fine for a shell-capable agent,
useless for one talking pure MCP.

**What shipped (2026-07-30).** `wtCreateIn.Spec` is `map[string]any`, which
derives to `{"type": "object"}`, and the handler re-marshals it - one pass over a
document that was about to be parsed and validated anyway. Verified over real
MCP stdio: `tools/list` now reports `spec` as type `object` with no `items`.
`json.RawMessage` in a tool input is the anti-pattern; worth a sweep if another
tool grows one.

---

## FR65 [shipped 2026-08-06] Open a citation in the editor

**Session.** 2026-07-30, same review.

**What AgentBox could not do.** A code block header offers copy-the-path and
copy-the-anchor, and nothing else. Authoring rule 12 requires that *every* code
reference offer both actions, copy and open in the editor, and calls an
inconsistent affordance a mistake that makes the reader check whether they
missed something. Right now the affordance is consistently absent, which is at
least honest, but the reader still has to leave the board, paste, and find the
line themselves - on a 637-line file cited across eight steps, that is the
motion they make most.

**What was done instead.** Nothing available. The reader copies and pastes.

**Proposed shape.** An open button per block, next to copy, running the
configured editor command with the file and the block's first line. The
JetBrains invocation is already recorded in "Mechanics discovered" below
(`goland <projectDir> --line <n> --column <m> <absolutePath>`, project directory
first, routes to an already-open window). Editor command belongs in config as a
template, since the board cannot guess between GoLand, VS Code and a terminal
editor.

**What shipped.** `internal/editor` resolves an argv template and launches it;
`Bridge.BoardOpenInEditor(id, rel, line)` and an arrow beside copy in every block
header. The surface names a REVIEW and a repo-relative path, never a file: the
root comes from the stored walkthrough on the Go side and `underRoot` refuses
anything that resolves outside it, the treatment `OpenURL` already got for being
the other place a surface reaches out of agentbox.

**Four things building it found, in the order they cost time.**

1. **An editor started as a plain child of the daemon dies on the next deploy.**
   `agentbox.service` is `KillMode=control-group` and the Toolbox launcher script
   *execs* the IDE rather than forking it, so the IDE **is** the child and lands
   in the service's cgroup. The launch goes through
   `systemd-run --user --scope --collect` to leave it. Watched for real: the
   window opened by the button was still up after a full `make deploy`. Cold
   start only in practice - with the IDE already running the launcher is a
   short-lived client - which is exactly the case testing would miss.
2. **$EDITOR is not consulted, deliberately.** FR51 proposed falling back to it;
   it is almost always a terminal editor and the daemon has no terminal to give
   it, so honouring it would be a click that silently does nothing. Detection
   covers GUI launchers and `xdg-open` is the last resort (which loses the line
   and says so). A terminal editor is still reachable by writing the terminal
   into the template: `["kitty", "-e", "nvim", "+{line}", "{file}"]`.

   **All four rungs exercised against real PATHs, 2026-08-06** (session 49; until
   then only the first two had ever run, because this machine has `goland` on
   PATH and detection stopped there). With `/usr/bin` alone visible the second
   rung picks `subl`; with only `xdg-open` reachable the argv comes back as
   `["xdg-open", FILE]` with no line in it; with nothing at all it is the honest
   `no editor found; set editor.command in config.toml`. The fallback's own
   launch was then run the way `editor.Start` runs it - through `systemd-run
   --user --scope --collect` - and this desktop hands a `.go` file to Zed, which
   opened at `1:1`. Note that `zed` is itself in the known table, so the line
   survives on a machine where its binary is on PATH; only the desktop-handler
   route drops it.
3. **Opening a project the IDE does not already have raises a modal.** GoLand
   asks "This Window / New Window / Attach" before it does anything, so the first
   open into a cold project is two clicks and the file lands in a background tab
   behind whatever the restored session had open. It DOES land on the line (the
   caret read `141:2`). The second citation, with the project now open, routed to
   that window, switched tabs and raised it - `378:2` - which is the case that
   matters daily.
4. **A defect in the surface, not in this feature.** The board's shortcuts bailed
   for INPUT and TEXTAREA only, so Enter on a focused button ran the button and
   the board's own binding: the file opened and the board jumped to the next
   unread step underneath the reader. True of every button on the board since the
   shortcuts were added; fixed in the same session.

**Verified on screen** (2026-08-06, deployed build): the arrow on a citation with
no `editor.command` set at all, detection picking `goland`, the caret landing on
the cited line both cold and warm, the failure wording beside the block for a
template naming a program that is not there
(`editor "nosucheditor-fr65-probe" is not on PATH`), recovery on the next click
once the config was right, and Enter on the focused button opening the file
without moving the step.

**Not built.** No `[editor]` control in the settings tab: the value is an argv
array and the descriptor table has no kind for one, the same reason
`speech.command` has no control. It is a config.toml key documented in
06-configuration.md.

---

## FR66 [shipped 2026-07-30, defect fixed by FR72 on 2026-07-31] Read a screen aloud, on request

**Session.** 2026-07-30, walking the first work-repo review. Boris, looking at a
step: "I should be able to request to hear the text aloud", then "I should be
able to pause the read-aloud and rewind to start or stop it."

**What AgentBox could not do.** Speech was agent-only. `speak` says one line an agent
chose, capped at `speech.max_chars` (240) and truncated past it, and the manual
said outright that AgentBox never reads a screen aloud. Nothing let the human ask.

**The constraint that shaped it.** Boris, before any of it was built: "The audio
control must under no circumstances harm the quality of the speech in any way. If
it does even in the smallest amount then I want only the parts that don't even if
it means only play and stop."

That killed the obvious design. Chunking to `max_chars` splits mid-sentence, and
an utterance is what the engine synthesises in one pass - so its prosody is
decided as a unit. Break a sentence across two utterances and the first half
gets a falling, finished intonation it should not have. So `speech.Passages`
splits **only where the text already pauses**: paragraph breaks first, sentence
ends if a paragraph is too long to wait on safely (`readCeiling`, sized to stay
inside `drainCeiling`), and never inside a sentence - a single sentence past the
ceiling stays whole on purpose. `ReadWait`/`prepareLong` is `SpeakWait` with the
cap lifted, because the cap is a notification policy and applying it here would
silently drop the end of every paragraph.

**What shipped.** `MethodAloud` with a transport (start, pause, resume, toggle,
rewind, stop) over a daemon-owned reader (`internal/daemon/aloud.go`); a
generation counter invalidates a stale reader goroutine instead of a context per
utterance. `Speaker.Stop` drains the queue and interrupts the line in flight -
`drain` takes a cut channel and the run loop releases the pipeline, because
ending the wait is not what stops the sound. Pause holds its position, so resume
continues from the passage it interrupted rather than skipping it. On the board:
a transport in the step head, `a` toggle, `r` restart, Esc stop, and moving step
stops the reading.

**The defect, since fixed.** Boris: "It doesn't read the whole sections, some last
words are missing." Closed by FR72 on 2026-07-31 - the cause was this feature's own
wait, not the engine. What follows is the reasoning as it stood at the time, and its
final sentence turned out to be wrong - read FR72 for what the measurements actually
said. Measured that it is *not* per-utterance latency: three lines
sequentially took 12.9s, the same words as one line 13.1s - no seam cost. So the
leading hypothesis is that the engine abandons the tail of a line when the next
one is written, which would mean splitting at all is what loses words. If that
holds, the fix is to read a step as ONE utterance and reduce the transport to
play and stop - which is exactly the fallback the constraint above already
authorises. Parked at Boris's request to finish the rest; do not consider this
feature done.

**Also worth keeping.** `⇧A` was the first rewind binding and had to be changed
to `r`: `drive_desktop` sends unshifted keysyms, so a chord binding cannot be
demonstrated through AgentBox's own driver. A binding the tool cannot press is a
binding nobody can prove works.

---

## FR67 [shipped 2026-07-30] A code line's background stopped at the viewport

**Session.** 2026-07-30, same review. Boris: "see how the code seems strange with
different coloring when scrolled horizontally."

**What was wrong.** `.row` was a flex box and `.txt` was `flex: 1`, so a row was
only as wide as the *visible* area. A line longer than that overflowed its own
flex item: the text scrolled on but the row's background - the added-line tint,
the hover, the comment marker - stopped where the viewport did. Scrolling right
showed tinted code becoming untinted mid-line. The line-number gutter also
scrolled away, so reading the end of a long line meant losing track of which
line it was.

**What shipped.** An inner `.rows` wrapper at `width: max-content;
min-width: 100%`, so every row is as wide as the widest line and a row's
background covers the whole line. `.txt` became `flex: 1 0 auto`. The gutter and
the badge column are `position: sticky; left: 0` with **opaque** backgrounds -
mixed *into* the surface rather than layered over it, since code slides
underneath - so numbers stay put while the code moves. Demonstrated by dragging
the block's scrollbar and confirming numbers, badges and full-width tints
survive.

---

## FR68 [shipped 2026-07-30] Definitions, and writing for a finite reader

**Session.** 2026-07-30, after the first field walkthrough of a work change
(`cmd/ssvc-backfill`). Boris, on what the standard was missing: the language has
to stay simple and keep the reader moving; the explanation has to sit next to
the code it explains; the flow of thought must survive from one step to the
next; and the review has to carry everything the reader needs, **including what
the domain words mean** - NVD, SSVC - without those definitions cluttering the
reading. Popping a definition on every hover was raised and rejected in the same
breath: distracting. A glossary the reader opens when they want it, with a quiet
mark on words that have one, was the shape he asked for.

**What shipped, the feature.** `glossary: [{term, short, body?, also?}]` on the
spec.

- **Marked once per step, never popped.** The renderer marks the first
  occurrence of each term in each step with a dotted underline in `--k-ink-3`,
  distinct from a bound phrase's accent underline so the two offers never read
  alike. Clicking one, or pressing `g`, opens a drawer; the opened entry is
  highlighted and scrolled to. Nothing appears on hover.
- **The drawer overlays, it does not take a column.** Opening a definition must
  not reflow the paragraph the reader is in the middle of - the same rule the
  hover comment peek was built under (FR62 round 5).
- **One matcher, two consumers.** `internal/walkthrough/glossary.go` serves the
  validator (warn about an entry no prose can reach) and the renderer (mark what
  a reader can open). Word-boundary, case-folding, longest spelling first so
  "technical impact" is not shadowed by "impact".
- **Runs, not offsets.** Go cuts each segment at its marks and ships the pieces.
  Go bytes and JavaScript code units disagree the moment prose stops being
  ASCII, and the surface should not be counting characters at all. The whole
  segment still travels in `t`, so find and read-aloud are untouched.
- **Bound phrases and code chips are never marked**, because a control inside a
  control is two meanings under one underline. The warning knows this, so it
  cannot send an author chasing a mark that could not exist. It fired three
  times on the regenerated review and each one was a real gap.

**What shipped, the standard.** `internal/manual/walkthrough.md` rewritten in
the order the author works - the job, plain writing, the shape of a step,
ordering the code for understanding rather than for the file, which of the four
channels each kind of text belongs in, the glossary, hunting the aha. Every
earned rule from the old file survives. New rules worth naming: order by the
path the data takes and say so in one sentence up front; end each step by
handing off to the next; never be clever; find the thing a careful reader would
still miss and give it its own beat.

**Also.** The board had no close button. The window is frameless, so the only
way out was knowing `q` closes it - which Boris said plainly the first time he
had to close one.

**Verified live.** Marks render on the first occurrence and repeats stay plain
(screenshot, regenerated ssvc-backfill review). `g` opens and closes the drawer,
Esc closes it behind a composer, the header chip toggles it, and an entry opened
from a marked word is highlighted. Not verified: a pointer click landing on a
marked word - synthetic XTEST clicks focused the button without activating it,
while the same clicks worked on larger targets, so the failure is most likely
the driver rather than the board. A human click is untested.

---

## FR69 [shipped 2026-07-30] Text between the blocks, and under them

**Session.** 2026-07-30, reading the regenerated ssvc-backfill review. Boris, on
three steps in a row: "we have two code blocks but no text between them, meaning
below the first and above the second - is it by design? does it read more
naturally and more understandable this way?" It was not by design and it does
not. Then the rule behind it, in his words: the text beside the code should
carry a major part of the explanation because it is closer and in context, while
the text outside the code introduces and summarises.

**What was wrong.** `prose` and `code` are separate arrays on a step, so every
word an author writes renders above every block. A two-block step is therefore
two walls of code with all its text stacked on top of the first, and the reader
crosses the seam with nothing to hold. There was also no way to put the takeaway
under the code it is about - written above, a takeaway is a promise rather than
a conclusion.

**What shipped.**

- **`lead` on a block.** The sentence or two directly above it: what this is and
  why it comes now. Capped at 600 characters, and the cap is the point - a lead
  that grows into the explanation is a note in the wrong place. Line references
  are refused in it, same as in prose.
- **`close` on a step.** The takeaway, rendered under the last block. Full prose
  segments, so it can bind a phrase to a region and carry glossary marks.
- **Both are prose channels for FR68.** The term memory now walks prose, then
  each lead, then close - the order the reader meets them - so "first
  occurrence" means first on the page. The unreachable-term warning was taught
  the same three channels; before that fix it reported terms the board was busy
  marking.
- **`Step.svelte` renders a paragraph through one snippet**, used by prose, by
  close and (for the marked runs) by leads, so the three cannot drift apart.

**And the rule this exposed**, now in the standard: the weight of the
explanation belongs in the notes, beside the lines. Prose opens, notes explain,
close concludes. A step with long prose and an empty margin is written
backwards. The regenerated review went from 41 notes to 80 under this rule.

**Also.** `agentbox walkthrough open` with no id opens the most recent review. Boris
asked how a review is saved and loaded - it always was, in SQLite, with marks
and reading position - but the only door was `list`, copy the id, `open`. That
is friction on the path a human takes every time they come back to one.

**Verified live.** Screenshots of a two-block step: the lead above the first
block, the second lead in the seam between the blocks with its notes in the
margin beside their lines, and the close under the last block ahead of the
legend and the checks.

---

## FR70 [shipped 2026-07-30] The library, and the hole under the last block

**Session.** 2026-07-30, walking the regenerated review. Two things, both from
the same walk.

**"How do I save and load it?"** Reviews had been durable since the store landed
- spec, marks, notes, comments and the reading position, write-through to SQLite
- but the only door was the CLI: `list`, copy an id, `open`. A feature you
cannot see is a feature you assume is missing, and Boris assumed he would have
to ask for the review to be regenerated after a reboot. His follow-up named the
shape: "should probably be a dedicated surface to manage existing materials."

**What shipped.** A **Library** tab in the app window (`agentbox app --tab library`,
or `l` from the board). Every stored review, most recently touched first: title,
repo, pinned sha, when it was last touched, and a progress bar of understood
steps with unclear and comment counts. Filter box. Click a row to put it on the
board - the existing window retargets rather than opening a second one. Delete
behind a two-step confirm, and deleting the review that is on the board
retargets the board to whatever is left, or closes it: a board showing a review
that no longer exists would take annotations nothing could store. The header
says plainly that nothing needs saving by hand, because that was the actual
question.

Three verbs behind it (`Library`, `LibraryOpen`, `LibraryDelete`) read the same
store rows the CLI prints, so the two doors cannot disagree about what exists.
`agentbox walkthrough open` with no id now opens the most recent, for the same reason.

**The hole under the last block.** The margin notes are a grid sibling of the
code frame, so the row is as tall as whichever is longer. With ten notes against
a short citation, the paragraph closing the step began where the *notes* ended -
half a screen of dead space under the code, which reads as a rendering bug. On
the **last** block of a step the margin now contributes no height
(`max-height: 0; overflow: visible`) and the notes carry on down beside the
closing text, in a column nothing else uses. Only the last block: an earlier one
would collide with the next block's notes. Below 1180px, where notes stack under
the block instead, the height comes back or the text would be written over.

**Verified live.** Screenshots: the library listing three reviews with progress
and delete controls, and the closing paragraph now sitting directly under its
code with the tail of the notes beside it.

---

## FR71 [shipped 2026-07-30] Icons where a word was doing an icon's job

**Session.** 2026-07-30, same walk. Boris: every button that can carry an icon
should, "then it will look better and take less space" - and, on the glossary:
hovering a marked word should pop a short explanation, clicking should open the
full entry as it already does.

**Icons.** The board header carried `glossary (9) · g` and `library · l` as
words, in a bar already holding a title, a repo path, a sha, the pips and the
submit button. Both are glyphs now - an open book and a shelf - with the word
and the key in the tooltip, so nothing is lost to anyone who does not recognise
the drawing. The block header's `copy` and `copy anchor` became a copy glyph
that turns into a tick when it fires, and the read-aloud transport dropped the
label that repeated its own play triangle. Where a word IS the content - the
cited path, `Submit review`, the `delete`/`keep` confirmation in the library -
it stays a word: an icon for a committing or destructive action is a guess the
reader has to make twice.

**Two answers for a marked word.** Hover gets a compact tip with the term, the
one-line `short` and "click for the rest", after a 220ms delay so a pointer
crossing the paragraph never sets it off. Click still opens the drawer at the
full entry. This is not a reversal of FR68's "nothing pops on hover" - what that
rule was against was the *definition* interrupting the reading. A one-liner that
answers "what is that?" without moving anything, and disappears when the pointer
does, is the cheap half of the same offer.

**One bug worth keeping.** Rendered inside the step the tip is a fixed element
inside the scrolling column, and the prose painted straight over it - a tooltip
with the paragraph showing through. The step reports the hover and the word's
rectangle; the surface owns the element, beside the drawer and the modal.

**Verified live**, with the mouse announced through AgentBox before and after: the tip
opaque and above the prose, centred on the word, flipping below it near the top
of the page.

---

## FR72 [shipped 2026-07-31] One region, one utterance, a control on each

**Session.** 2026-07-31. Boris, on the FR66 defect: "The read-aloud was running
perfectly before I asked to be able to pause stop and rewind. If they harm this
feature, I don't mind having only play or whatever maximal functionality that
keeps perfect functionality of kokoro speech." And the shape he wanted instead:
"If there are several parts of text on the page, I should be able to start them
separately, because for example I want to hear the text before a code-block then
I need to read the code-block and not continue hearing what comes after it... So
each should have visual control or controls of its own."

**The cause, measured.** Not the engine, and not the hypothesis the previous
handoff carried. `kokoro-say` reads stdin strictly sequentially and blocking
(`for line in sys.stdin: say(line)`), so a new line cannot abandon the previous
one's tail. What it does do is emit NOTHING until `kokoro.create()` has
phonemised, run the model over every batch and concatenated - and `drain` gave a
line a flat five seconds to produce its first byte. Synthesis time on this
machine (kokoro-onnx 0.5.0, am_michael, one core):

| chars | synthesis | audio | under the old 5s grace |
|---|---|---|---|
| 66 | 1.70s | 4.46s | fits |
| 218 | 5.74s | 14.66s | wait gives up |
| 398 | 7.86s | 25.09s | wait gives up |
| 743 | 14.15s | 46.95s | wait gives up |

So from two sentences up, the wait declared the line silent, released its waiter,
and the reader advanced while tens of seconds of speech was still being made.
Every position the transport tracked ran ahead of the sound, and everything that
acted on one - pause, stop, the stop-on-step-change - cut audio nobody had heard.
`speak` was unaffected because `max_chars` caps it at one sentence, which is
exactly why Boris remembered it as perfect before the transport existed.

**Also proven innocent**, so nobody re-checks: the engine synthesises the whole
text (the same passage minus its last three words is 0.96s shorter, exactly three
words' worth), and `wireProse.T` always carries the full text beside `Runs`, so
glossary marks never cost the reader a word.

**Two losses that were never audio at all.** `stepText` sent the title, the
purpose, the prose and the checks - it was written against the step shape from
before FR69 added `lead` and `close`, so a block's handover paragraph and the
step's takeaway were never spoken. The takeaway is the sentence a step exists to
land.

**What shipped.** `speech.Passages` is gone, replaced by `speech.Utterance`: a
region is handed to the engine whole, because an utterance is what the engine
decides prosody over and every split costs the seam. Both bounds of the wait now
scale with the line (`waitBudget`, ~2x headroom over the measurements above), and
a `drainSettle` covers the one thing the meter cannot see - the player's own
buffer, which makes every wait end slightly early in a voice whose audio ends on
the last word with no trailing pad. `Stop` kills the pipeline instead of asking
it to finish, since `close()` shuts stdin and waits, and an engine blocked
writing a minute of PCM would talk through the whole `drainGrace`.

On the board: `AloudBtn`, one per region - the opening (title, purpose, prose),
each block's `lead`, the `close`, and the checks. Pressing the one that is
playing stops it; pressing another replaces it. No pause, no resume, no rewind:
all three needed the split. `a` reads the opening, Esc stops. The surface polls
`state` while a reading is live, because a reading that ends on its own has
nothing to push.

**Verified live** (deployed `9893471`, board driven with the mouse and keyboard):

- All four control types render and a real mouse click on one activates it
  (`aloud start region=close chars=296`).
- A region reads whole and stops there - one start, one finish, no run-on into
  the next region.
- The wait covers the whole utterance: the 296-char takeaway took 25.64s against
  a 24.29s ground truth (5.81s synthesis + 18.47s audio), the 398-char opening
  34.6s against 32.95s. The old code would have released both at 5.0s.
- Esc tore the pipeline down 90ms after the keypress, killing the player with it
  so no buffered audio survives.
- **Confirmed by ear** (Boris, 2026-07-31): "the read-aloud seems fine". That is
  what closes FR66 - every other check here is arithmetic, and only listening
  could tell whether the tail is actually heard.

**One mechanic worth keeping.** Synthetic clicks that "focus a small button
without activating it" - the FR68 term-click suspicion in two handoffs - were
landing on whatever window was on top. Boris named it: "your testing failed
because the AgentBox application went to background while I write to you in the
terminal." Raise the target window in the same command as the click
(`wmctrl -a "$title"` then click) or the click goes to the terminal. The button
size was never the problem.

---

## FR73 [fixed 2026-08-05, session 47] A card body cannot be read back

**Session.** 2026-07-31. Boris missed a `veto` card while it was up, went looking
for it, and could not recover it: "I've missed what you said - so I tried to look
in the inbox and there doesn't seem to be a way to find it - this is a problem. I
can't see the whole thing you wanted to tell me."

**What AgentBox could not do.** The inbox row carries title, identity, age and outcome,
and the body only as a hover tooltip on a resolved row - which truncates. So a
card that closed on its timer takes its body with it. This is listed under "known
gaps (deliberate)" in STATUS as "no per-item detail; promote to the card to read
a body", and promoting is not offered for a resolved item. For a tool whose whole
purpose is that a message is not lost, losing the message is the wrong failure.

**Proposed shape.** A row opens a detail view - the full body rendered as
markdown, the identity, both timestamps, the outcome and any answer given. It is
the one surface that must never truncate. Everything needed is already stored;
this is a reader, not a schema change.

**Fixed as proposed.** Clicking any row - pending or resolved - opens a detail
under it: the body through `RenderMarkdown`, the same renderer the card used, so
it reads the same way after it closed; both timestamps to the minute plus how
long the item stood; what it offered and which option went back, the default and
the taken one marked separately; a form's answers in the order its fields were
asked, not its map's; the identity with its hue, session key and id. Nothing is
clamped, shortened or ellipsised. `Bridge.ItemDetail(id)` is a call per opened
row rather than a field on the snapshot, because a hundred rendered bodies in
every push is exactly what the row's 140-character `Snippet` exists to avoid.

**Three things building it turned up.**

- **"Everything needed is already stored" is not quite true.** The store's items
  table carries id, kind, level, title, body, options, fields, actions, cwd,
  timeout_s, dflt, identity, state and created_at - and no `speak`, no `diff`. So
  a spoken line and a review's diff DO go with the card. Both were written, both
  were then taken out rather than shipped as fields that are always empty: the
  detail promises nothing it cannot deliver. Persisting them is a schema change
  and its own request (STATUS, known gaps).
- **Clicking a row used to be inert unless it was pending**, so nobody had reason
  to click into the list and then type. Now that every row opens, a click also
  moves the triage selection when the row is pending - otherwise reading one row
  and pressing `d` would dismiss whichever row `j/k` was last left on.
- **A row can change under an open detail.** Answered on its card, or triaged from
  the keyboard: the detail then went on saying "waiting" and offering a card for
  something already answered. One effect owns both cases - a row that LEFT the
  list closes its detail, a row that CHANGED is re-read.

**The keyboard triage path is untouched**: `j/k` still walks the pending run and
the same keys still act on the selection. A row is a real button, so Tab reaches
every row and Enter or Space opens its detail - the only keyboard route to a
resolved one, since the selection never leaves the pending rows. Those two keys
are handed to the focused button rather than to triage, or reading one row would
answer another.

**Watched on the real screen**, on the case Boris filed. A veto card with a
946-character body was raised while he was away and expired on its own; its row
reads `proceeded` and shows 140 characters. Opening it gave the whole body back,
rendered - bold, inline code, a numbered list - with `arrived Aug 5 15:02, ended
Aug 5 15:02, stood 8s` above it and `kind veto / level warning / from claude ·
db-migrations / id k33b0772f589bc668` below. The body ran past the window, and the
rest of it plus the whole meta block scrolled into reach rather than being cut.

Four more things were exercised rather than argued:

- **A choice he had answered himself** read back its three options with
  descriptions, `Beta` carrying both `DEFAULT` and `TAKEN`, `WHAT WENT BACK: Beta`,
  and `stood 2h27m` between the two timestamps.
- **A pending row's detail** showed `Show the card` beside `arrived ... now` and no
  ended line. Clicking it raised the card - one click from the reader.
- **The row changing under an open detail**, which is the case the extra effect
  exists for: answering that card `n` on the card itself moved the row from
  Pending to Recent, dropped the `Show the card` button, and grew `ended Aug 5
  17:34, stood 1m` and `WHAT WENT BACK: no` in place, with nothing touching the
  detail.
- **A row leaving the list closes its detail**: filtering the search box to a term
  the open row does not match shut it rather than leaving it under a different row.

**One trap re-earned.** The first click landed on the wrong row: the list moved
between the screenshot the coordinate came from and the click itself, because two
pending items were answered in that gap and the Pending section collapsed. Session
46 recorded this for a row that had been opened; it is true of any queue change.
The fix that made the rest of the run repeatable was to type a term into the search
box first, so the target is the only row on screen and no coordinate can go stale.

---

## FR74 [shipped 2026-07-31] A permanent sign that says hands off, and what is happening

**Session.** 2026-07-31, after Boris interrupted a drive sequence three times in
ten minutes: "we need a better way of interacting, a permanent on-screen sign
that means 'hands off' for me. Otherwise I keep interrupting you and we work
against one another." And: "The on-screen indication should also detail what is
currently being done so that I know every moment where we are, that nothing is
stuck and so on."

**What AgentBox could not do.** The announcements are events, not state. `veto` says "I
am about to take the mouse" once and then closes; `notify` says "mouse is yours
again" once. Between the two there is nothing on screen, so the only way to know
whether an agent is still driving is to touch something and find out - which is
exactly what breaks the run. The cost is measured: this session lost two drive
sequences to a window that went to the background because Boris typed in the
terminal, and one wasted click test chasing it.

**Proposed shape.** A small always-on-top strip an agent owns for the length of a
run: a state (driving - hands off / working / waiting for you), one line saying
what is being done right now, and the time since that line last changed, so
"stuck" is visible without asking. Loud while driving, quiet otherwise. It is
state, not a card: it must not queue, must not need dismissing, and must survive
every window change. `report_progress` is the nearest thing and is the wrong
shape - it is per-task, has a bar, and opens its own window.

**Decided with Boris, 2026-07-31** (these two were his call, not a default):

- **Top-centre strip**, below the GNOME bar. It borrows the
  screen-is-being-shared convention, which is the exact meaning wanted, and it is
  the only placement with room for a whole activity sentence. Rejected: a
  bottom-right pill (truncates, and shares the corner with AgentBox's own toasts) and a
  thin left-edge bar (no room for the sentence, needs a second surface).
- **On screen only during a run.** It appears when an agent opens one and goes
  when the run ends or the agent dies, so its presence alone means "an agent is
  working". Rejected: always-on with an idle state.

**Constraints that follow from where it sits.** It must never take focus - it
appears while Boris is typing, and a focus steal would be worse than the problem
it solves. It must not queue or need dismissing. And it must not lie when an
agent dies: the run ends on caller-disconnect (the FR45 machinery already knows),
not on a timeout.

**The lifecycle, set by Boris 2026-07-31** - and this is the part that makes it
one element rather than two: "It should be permanent as long as it is relevant,
meaning it is the element that asks to take control and letting me deny, and then
once you take control it shows updates on the work you do while I am hands-off."

So the strip absorbs the veto card's job for a drive sequence instead of sitting
beside it. Two states, one window, no second card anywhere:

1. **asking** - "may I take the mouse?", the reason, a countdown, and a Deny
   button. This is `act_unless_stopped`'s semantics living in the strip: the
   agent's call blocks, silence grants it, Deny returns vetoed.
2. **driving** - granted. Loud (amber), reads *hands off*, and carries the
   activity line the agent keeps updating, plus the age of that line.

Ending the run takes the window with it. That replaces both cards a drive
sequence costs today ("about to take the mouse" and "mouse is yours again") with
one element that is never dismissed and never missed.

**Two states, not four, and that is a rule rather than a saving.** Boris: "once
it's gone, I know the 'hands-off' is over and I can work freely with the keyboard
and the mouse." So the strip's *presence* is the whole signal - on screen means
the desktop is the agent's, gone means it is his. A quiet "working, but you may
touch things" state was drafted and cut: it would make presence ambiguous, which
is the one thing this element cannot afford. An agent that is busy without needing
the desktop shows nothing, and a question still goes in a card.

**Agent-facing shape.**

- `request_control(reason, window_s)` - BLOCKS. Opens the strip in *asking*, then
  either returns granted (silence, or the countdown elapsed) and leaves the strip
  in *driving*, or returns denied and closes it.
- `set_activity(activity, state)` - non-blocking, updates the line and optionally
  moves between driving/working/waiting.
- `release_control()` - ends the run and takes the strip with it.

Keyed by the caller's identity, so an agent never carries a run id. An agent that
forgets to release loses the run to caller-disconnect (the FR45 machinery already
knows), which is the safe direction: the strip must not outlive the agent, and it
must never claim hands-off for a process that is gone.

**What shipped.** `internal/daemon/control.go` holds the run;
`internal/webui/control.go` owns the window and `Control.svelte` paints it. One run
at a time: a second agent is refused with the holder's name rather than queued, and
only the holder may write the activity line or release. The run dies with its
caller's connection.

Reachable two ways on purpose. `agentbox control request|activity|release|state` is the
shell door, because Boris asked for this to serve agents that do not speak MCP
("while they drive my debug chrome or other automations"), and `request` exits 0
granted / 1 denied / 3 held-or-unanswered so a script can gate on it.
`request_control`, `set_activity` and `release_control` are the MCP door (session
32), because MCP is how every Claude session reaches AgentBox: while they were missing,
this session needed the desktop for a screenshot run, hand-rolled a
`confirm_action` asking Boris to keep his hands off, and got interrupted mid-run
anyway - a card is answered and gone, which is the exact failure FR74 exists to
end. Three tools rather than one with a mode, because they differ in the only way
a caller cares about: the request blocks on the human, the other two never do.

**Verified live** (deployed `5c7c45e8c78a`, screenshots in the session):

- Asking paints the reason, the asking agent, a countdown that fills the strip,
  Deny and Now. Silence granted it and the same window switched to HANDS OFF
  without flickering off screen.
- `agentbox control activity` replaced the line and reset its age to 0s.
- It is the topmost window, above AgentBox's own. Read back from
  `_NET_CLIENT_LIST_STACKING` with a run live: `review board` (1920x1152), then
  `agentbox · toast`, then `agentbox · hands off` on top - and visibly over the full-screen
  board in a screenshot.
- `agentbox control release` took the window off the screen and `agentbox control state`
  answered "no run: the desktop is the human's".
- Two bugs the live test found and fixed: `--window` after the reason was silently
  ignored (Go's flag package stops at the first positional, so the countdown ran
  20s when 12 was asked for, with "--window 12" tacked onto the reason the human
  reads), and `agentbox control state` could only ever answer "held by" because the
  refusal branch short-circuited it.
- **Not verified:** behaviour with two real agents racing for the desktop (the
  arbitration is unit-tested, not yet seen with two live sessions), and the strip
  against a genuinely fullscreen app rather than a maximised one.

**The MCP door, verified live** (deployed `2906c9ddd4f9`, session 32). A running
MCP child cannot see tools added after its handshake, so this was driven over a
fresh stdio server rather than from the session's own `mcp__agentbox__*`:
`tools/list` answers 23 tools with `request_control{reason, window_s}`,
`set_activity{activity}` and `release_control{}`. `request_control` blocked, the
strip appeared, silence granted it after 30s and the call returned
`{granted:true, live:true, state:"driving"}`; `agentbox control state` from another
shell agreed (`driving · python3 · <reason>`). Two `set_activity` calls moved the
line while a board was opened and resized under it, and `release_control`
returned `{live:false}`, took the strip off the screen and left `agentbox control
state` at "no run". Worth knowing for the next agent: the run does NOT end when
the request's connection closes (only the asking phase watches ctx), so an MCP
caller holds the desktop across separate tool calls - and a forgetful agent holds
it until something releases it.

**It immediately produced FR75**: the toast and the strip both compute the same
top-centre position, so the toast landed underneath it.

---

## FR75 [shipped 2026-07-31, fullscreen part open] Top-centre surfaces must share one stack

**Session.** 2026-07-31, straight out of FR74's first live test. Boris: "Other
notifications should know about the hands off and in general of other
notifications to not collide on the screen - if a second one comes it should be
below the previous one."

**Measured, not predicted.** With a run live, a warning toast was fired and the
window geometry read back:

```
agentbox · toast       430x78 at 745,48
agentbox · hands off   620x62 at 650,48
```

Same inset, same edge, overlapping rectangles. The stacking order is right - the
strip is on top, which FR74 requires - so the toast was simply hidden underneath
it. Two surfaces are competing for one position because each computes its own
`x.place(..., top: true, inset)` and neither knows the other exists
(`internal/webui/x11.go:place`, called from `bridge.go` for a toast and
`control.go` for the strip). A second toast has the same problem with the first.

**Proposed shape.** One owner of the top-centre column, not a placement call per
surface. It keeps an ordered list of what is up there, assigns each a slot, and
lays them out downward from the inset with a fixed gap; anything closing compacts
the ones below it back up. The strip always holds slot zero while a run is live,
because FR74 says nothing may cover it - so a toast arriving during a run starts
below it rather than under it.

Two details worth settling before writing it: the column has a bottom (past some
number of slots, extra items should queue rather than march down the screen -
AgentBox's existing queue already does this for cards), and a slot must be released
when a window dies without closing cleanly, or the column would leak gaps.

**The fullscreen exception, and it is not yet settled.** Boris, immediately after
the above: "Unless it is a full-screen in which case it's OK to cover but not the
ones that come after it, they pop on top of it regularly."

So a fullscreen window is allowed to cover the hands-off strip, and notifications
arriving after that fullscreen window still pop on top of the fullscreen window as
they do today. This **reverses part of what FR74 shipped**: `control.go`'s
`keepOnTop` currently restacks the strip unconditionally every 1.2s, which is
precisely what beats a fullscreen app (x11.raise carries the Mutter layering note -
a focused fullscreen window is promoted into the always-on-top layer, and inside a
layer the focused window wins, which is why the progress bar once vanished behind a
slide deck).

**What shipped.** `internal/webui/topstack.go`. Claimants take a slot; the column
lays them out downward from the inset and compacts back up when one leaves. The
strip pins the first slot. The column stops at the middle of the screen, which
belongs to cards, and an overflow is logged (`webui.top_stack_overflow`) rather
than piled up silently.

**Verified live** (deployed `8bc739c189d9`): the same measurement that found the
bug now reads `hands off 620x62 at 650,48` and `toast 430x78 at 745,118` - 48 + 62
+ 8, the column arithmetic, with the rectangles no longer intersecting and both
visible in a screenshot.

**Not verified, because it cannot happen yet:** two toasts stacking under each
other. The card window is reused between treatments (`u.prompt`), so only one
toast exists at a time and the queue holds the rest. The column already handles
N claimants; when a second simultaneous toast becomes possible it needs a key per
window rather than the single "toast".

**Decided with Boris:** the strip yields the space to a fullscreen window, but a
small marker stays on top of it - a dot or a thin amber edge, enough that the
hands-off guarantee survives without the strip covering his video. Accepting a
fully covered strip was rejected: an agent driving while he watches something, with
nothing on screen saying so, is the one wrong answer FR74 exists to prevent.

Built in session 34 (`internal/webui/control.go` for the marker and its placement
rule, `controlmark_test.go` for the rule, `x11.go` for the
`_NET_WM_STATE_FULLSCREEN` read), and **watched for the first time on
2026-08-06, session 49.**

**The marker half works exactly as designed.** With the desktop held and a
fullscreen `gnome-terminal` focused, an `agentbox · hands off marker` window maps
at `+0+0`, `1920x4`, amber (`srgb(189,144,60)`) across every column sampled from
x=200 to x=1900. Leaving fullscreen takes it away again, and the window is gone
from `wmctrl` within one keeper beat.

**The other half does not.** The strip is supposed to step aside while the
marker stands in for it, and it does not: the full 620x62 card stays visible on
top of the focused fullscreen window, so a film gets both the card and the line.
`planMark` computes `step` correctly here (one monitor, so the fullscreen
window's monitor equals the strip's), and `beat` does call `x.lower(strip)` on
the transition - but lowering cannot win. The strip is mapped as a NOTIFICATION
type window with `_NET_WM_STATE_ABOVE`, and Mutter layers notifications above a
fullscreen window whatever the stacking order says; `xwininfo -root -children`
confirms the probe is above both agentbox windows in the X order while the strip
is still the thing on screen. The premise written into `fullscreenActive`'s
comment - "Mutter only promotes a fullscreen window above the always-on-top
layer while it HAS the focus" - holds for ABOVE and not for the notification
layer the strip actually sits in.

**Severity is the good direction, which is why this is a decision rather than a
fix.** The failure FR74 exists to prevent is a covered strip reading as "the
desktop is yours"; what happens instead is the strip refusing to be covered. The
fix is to HIDE the strip while fullscreen rather than lower it, and that is a
change worth Boris's word first: an unmap/remap cycle on a live run risks taking
the keyboard back on the way in (the first map goes through `showNoActivate` for
exactly that reason), which would be a worse defect than a card over a film.

---

## FR76 [shipped 2026-08-01] The board's header is also its title bar

**Session.** 2026-08-01. Boris: "The review board doesn't have a minimize and
maximize icons which I think it should. The Submit Review button is nice but
maybe we could replace it with an icon button with just `Submit`."

**What the buttons found.** Minimise and restore/maximise went in beside the
close X, and the maximise glyph reads its state back from the window
(`IsMaximised` on the runtime's own window proxy) rather than remembering it -
the window manager changes it without telling the page, and an icon that lies
about which way it will go is worse than no icon. Restore-down then exposed two
older defects, both invisible while the board only ever ran maximised:

- **The header overflowed.** Nothing in it shrank, so below about 1150px the
  controls walked off the right edge, window buttons first - the close button
  among them. The title now ellipses, the repo path and the pinned sha leave
  before it comes to that, and every control holds its width to the 700px
  minimum the window enforces.
- **The window could not be moved at all.** Frameless, and the header had no
  `--wails-draggable` region, so there was no grip anywhere on it. Boris found
  this one live, mid-session, while the board sat on the wrong monitor. The
  whole strip drags now; chrome is not selectable text, so nothing is lost, and
  the buttons opt out so a press that wanders a pixel is still a press.

`Submit review` became a paper plane and one verb. The title beside it already
says which review this is, which is what made the second word free to drop.

**Verified live** on the real desktop: dragged between monitors, maximised,
restored, minimised (and recovered from the dock), Submit's modal opened and Esc
closed it. Not exercised: light theme, and double-click-to-maximise.

---

## FR77 [shipped 2026-08-01] A click that knows what it is about to hit

**Session.** 2026-08-01, straight out of FR76's live test, where a drive script
moved a window Boris had just moved himself. Boris: "I want that part of the
interaction logic with the mouse and the keyboard to be verification of the
parts being clicked and typed in. I bet they can be identified somehow to avoid
unintentional interaction with unintended windows or documents." And, from the
same run: "Whenever you try to interact with a window, you must make sure to
actively make it active to avoid such issues."

**The gap, as the appendix below already recorded.** `window TITLE` resolved a
rectangle once and never looked again. Between that lookup and the click the
window can move, be covered, lose focus or close, and the events go wherever
those coordinates now point. Typing was worse than clicking: it went to whatever
held keyboard focus, which need not be the window the script named at all.

**Identified how.** Two X11 questions, on the connection `hand` already holds,
no new dependency. `QueryPointer`, walked down to the deepest child, says which
window a click will land in; `GetInputFocus` says which window a keystroke will
go to. Both resolve to a name, a `WM_CLASS` and a pid - proved on Boris's own
desktop before any of this was built:

```
pointer at 527,1225
  under: name="⠐ Add minimize and maximize icons…"  class="terminator.X-terminal-emulator"
keyboard focus:
  name="⠐ Add minimize and maximize icons…"  class="terminator.X-terminal-emulator"
```

**So `window TITLE` is a lock, not a coordinate frame.** It raises the window
(`_NET_ACTIVE_WINDOW`, source 2 - the pager indication window managers honour
without focus-stealing prevention), follows it if it moves, and before every
click and every keystroke compares the window that would receive it against the
lock. A mismatch raises the target and looks once more, which is what a person
would do about a window that ended up behind something; only then does the step
fail, naming what was there instead.

Two allowances keep it from refusing work that was always fine: a menu or
tooltip is an override-redirect window parented to the root, so a chain that
passes through one is accepted, and so is a dialog whose `WM_TRANSIENT_FOR` is
the target. `screen` gives the lock up and enforces nothing - there is nothing
to compare against - but the trace still names where every event went.

**Verified live**, against two throwaway `zenity` windows that the toolkit
placed at identical coordinates, which is the confusable case exactly:

- The lock raised ALPHA from underneath BRAVO and typed into it. BRAVO, at the
  same pixels, stayed empty.
- A click aimed outside the locked window was refused: `the pointer is over
  "Desktop Icons 1" (gjs.Gjs), not "agentboxtest-ALPHA"`.
- With ALPHA closed mid-script and BRAVO left occupying its coordinates, the
  `type` step refused rather than writing into BRAVO. This is the whole feature
  in one test: before it, that text landed in the wrong window.
- A right-click menu in gnome-text-editor, then `Select All` clicked inside the
  popup, still worked - the override-redirect allowance earning its place.

---

## FR78 [shipped 2026-08-01] A review must keep the code it is about

**Session.** 2026-08-01. Boris, on a board showing `cannot read
cmd/ssvc-backfill/main.go at the pinned path`: "I thought the code is embedded
and not read each time. This means whenever the code changes, the walkthroughs
that were based on it get invalidated."

**Confirmed, and worse than the error showed.** A walkthrough stored the
citation (path, line range) and the diff, and nothing else; `boardrender.go`
read the file off disk on every render. Three outcomes, in order of how bad
they are:

1. File gone - the honest orange error above.
2. File shorter than the cited range - `lines 230-240 cited, but the file has
   180 lines`, also honest.
3. **File edited in place and still long enough - renders whatever now sits at
   those line numbers, under the original prose and margin notes, and says
   nothing.** A review that reads as true and is not.

The pinned SHA was a label: validated as hex, shown in the header, read by
nothing. Boris's own case was a checkout, not a delete - `59bc8f7` is still in
the minimus clone and still has the file; he was on another branch.

**Creation captures what it cites**, from the working tree the authoring agent
was reading, into `walkthrough_excerpts` (one row per cited range, not per
file). The board prefers the capture and falls back to reading the tree, so
every review stored before this keeps working exactly as it did. A citation
that cannot be read is a warning on create, never a refusal: a walk over work
that is not all on disk is still worth having.

**This is not the second copy of a citation FR61 forbids.** The citation still
lives in the spec alone and is still what everything derives from. This is the
source it names, kept so the review can be read after the tree has moved.

**`agentbox walkthrough repair` recovers the ones already stored**, from
`git cat-file blob <pinned>:<path>` - the first and only use the pinned SHA has
ever had. It fills gaps and never overwrites an existing capture (a range taken
from the working tree at creation is what the author actually read). One id, or
the whole library with no argument; ranges git cannot serve are reported with
the reason, and those blocks keep falling back to the tree.

Snapshot-on-create rather than always-read-from-git, because a clone can be
deleted, moved or gc'd, and the review should outlive it. Git is the repair
path, not the durable one.

**Verified live** on Boris's own library, with the board reopened on the review
whose file is not in the working tree.

---

## FR79 [shipped 2026-08-01] The tray could hide the window but never show it

Boris, 2026-08-01: *"the system-tray icon only has a hide functionality and
never show functionality."*

`UI.ToggleApp` read `appWin != nil` as "open" and called `Hide()`. But Hide does
not close a window, so it fires no `WindowClosing`: `appWin` stayed non-nil,
`OnAppChange` was never called, and the menu item went on reading "Hide AgentBox" and
hiding an already hidden window. From the panel the tray had exactly one working
action.

The state the tray labels itself from is **visibility, not existence** - a
hidden window is still live and still holds its sessions - so `appShow` carries
it and every path that changes it reports through one place (`appShown`): the
tray item, `agentbox app`, the window's own close button, shutdown.

## FR80 [shipped 2026-08-01] The robot is the icon

Boris: *"the system-tray icon seems wrong, it would be cool had it been the
robot logo we have"*, then *"the robot icon seems small - can't it be as large
as possible so that it looks better?"*

`tools/genicon` drew a dot with two ripples. It now renders from
`docs/img/logo.png`.

Two things had made the first attempt the small one in the row. The source was
24px, and a StatusNotifierItem hands the host a **bitmap to scale** - so on a
HiDPI panel it was drawn at 24 physical pixels, half the height of its
neighbours. And the whole head is 204x152, so squaring it padded a quarter of
the icon with transparency. Render at 128, and crop the head **square**, cutting
the ear knobs: a tray slot is bounded by its height, and the ears are the first
thing that stops being readable anyway.

The crop still catches the AgentBox speech bubble behind the head, which at panel size
is a dark smudge floating off the corner - so the crop is masked to an ellipse.
A head is round; its corners are somebody else's picture.

State rides a badge dot rather than a tint: a desaturated robot in a row of
colourful panel icons reads as *disabled*, not as *quiet*. Idle is the robot,
attention adds blue, urgent adds amber.

## FR81 [shipped: Home 2026-08-01, visual pass 2026-08-04] The main panel must be worth opening

Boris, 2026-08-01: *"The AgentBox main panel should be beautiful and have all existing
functionality available from it in a way that is pleasant for the user"*, and
then the specific complaint: *"now when I show AgentBox it goes straight to SESSIONS
which is empty and is not that important functionality. It should show
interesting information with configurable panels and along the way it should let
me interact with all the real functionalities, such as opening the Library,
History and so on."*

**Shipped: the Home surface.** It has one job with two halves - say something
true about right now, and be a door into everything else. Four tiles (waiting,
agents working, interruptions today, reviews open) which are buttons rather than
trivia; then what is waiting, what is running, the week's shape drawn from the
same stats query History renders, the open reviews, and a row of doors. It is
what `agentbox app` and the tray now open on; a caller that wants a particular surface
still names it.

Panels can be turned off, because a dashboard nobody can quiet is one people
stop reading. That preference is per-machine view state, so it lives in
localStorage rather than earning a knob in the settings surface.

**The visual pass shipped 2026-08-04** (session 37): History, Inbox, Library,
Settings, Session and the app shell adopted the voice Home and Assignments
established - UI caps for labels with mono reserved for data, Home's tile
grammar, one recipe each for segmented controls, ghost buttons, search boxes
and empty states. Library was rebuilt on the shared list shape. Assignments
joined the rail on 2026-08-01 (FR82).

## FR82 [shipped 2026-08-01] Recurring AI assignments

Boris, 2026-08-01, across several messages: work AgentBox hands to a Claude agent on
its own - *"scheduled, periodic or ad-hoc"*, each with its own period and model,
*"fully available of AgentBox so it can interact with the user in the most appropriate
way when appropriate"*, with a configuration panel per assignment, settings
persisted, and create/edit/delete. His worked example: *"periodically check the
usage of Claude and display a summary to the user, making it a warning
notification when getting critical usage and maybe even collecting usage
statistics for later analysis."*

The full design, and the decisions behind it, are in
[08-assignments.md](08-assignments.md). Three that shape everything:

- **A missed slot is skipped and counted, never caught up.** A laptop shut for
  the weekend must not wake and fire three usage checks at once.
- **Parameters are typed knobs by default, custom HTML as the escape hatch.**
  Boris: *"built-in controls and markdown is probably more uniform and will
  allow a more professional look, [custom HTML] to be future-proof."* The values
  live in the database either way, so a panel that fails to render can never
  make an assignment uneditable.
- **The agent writes the assignment.** *"Upon creation, the AI agent itself
  should help with generating the initial prompt and the configuration panel for
  it until the user is satisfied... it should have full access to these so that
  it can help adjusting and improving assignments as we go along."* So the CRUD
  is an MCP surface before it is a UI.

**Shipped:** `internal/assign` (the model, substitution, schedule grammar),
migration 0007, the store layer, the scheduler and the daemon's assignment API,
`--model` on the session driver, seven MCP tools (`list/read/create/update/
delete/run_assignment`, `assignment_runs`), the webui Runner, and the
Assignments surface.

Two mechanics that arrived with the runner and are worth knowing before
touching either:

- **A run's report is its last assistant message**, and a fenced ```agentbox-data
  block inside it is lifted out into the run's `data` column. That is what
  "collecting usage statistics" turned into: prose for the human, a series for
  later. The convention is taught in `internal/manual/assignment.md`, the brief
  every run is spawned with.
- **A run is a session that does not take the human's selection.** It appears in
  the switcher, keeps its transcript, and has its child stopped when it
  finishes - thirty daily runs must not be thirty idle `claude` processes.

**Open:** the custom HTML panel still stores and round-trips but does not run in
the artifact sandbox; its values go through the typed knobs, which is the way in
that always works by design. Concurrency is still one run per assignment.

---

## FR83 [shipped 2026-08-04, sessions 40 to 45] Agents that can see, find and wait for each other

**The design is [09-sync.md](09-sync.md).** This entry is the field case; that
document is the feature, and it has already taken one adversarial review.

**Session.** 2026-08-04. Boris, across five messages: agents on this platform
should *"synchronize among themselves with maximal efficiency"*; he should
*"be able to monitor using the GUI what exactly each such agent is doing at the
moment"*; **every** agent *"must provide a short description of the purpose of
the agent and the current thing the agent is doing and update these as they
change"*; agents *"should easily find existing or new joining agents that are
working on the same area"* so they know they may interfere; and they should
*"communicate among themselves and discover each other using this platform to
achieve maximal cooperation and optimal synchronization."*

**What AgentBox could not do.** All of it. The daemon has no idea which agents
exist: identity rides inside each request, the mcp child dials a fresh
connection per tool call, and the only liveness signal in the system is a
currently-blocking call's context (FR45). There is no roster, no heartbeat, no
lease anywhere, and the one lock that exists is FR74's desktop handover, whose
ownership check is agent-name string equality - so two Claude sessions in one
project, which are the identical `{Agent, Project, Session}` triple when
AGENTBOX_SESSION_ID is empty, can write over each other's hands-off strip. Two
agents cannot see each other, cannot take turns, and cannot tell the human what
they are each doing.

**And the name AgentBox shows for an agent is not the agent.** Observed live
while this entry was being written: `agentbox control state` answered
`driving · timeout · finish the custom panel check`. The holder was a Claude
session, and it displayed as **timeout** because `Agent` is the parent process
name (`parentComm()`) and that session had been launched under the `timeout`
command. So the one field the strip, the mute list, the inbox rows and every
ownership check lean on is whatever happened to exec the agent. The mandated
purpose line is the fix for the human's half of this; the session key is the
fix for the machine's half.

**What was done instead, the same day, in this repo.** Session 37 ran two
agents in one checkout and they coordinated **by hand, through a file**:
a ledger written at `/tmp/agentbox-agents.md`, quoted at length here because
the file itself is volatile and will not survive a reboot. It contains a file
ownership table ("Claims (exclusive edit)"), a hand-maintained mutex for the
one thing that truly cannot be shared (`Current holder: NOBODY (deployed daemon
restored ~11:05)`), a rules section for the files neither could own outright
(`frontend/dist`: NEVER hand-merge), and a message log where each agent posts
timestamped notes to the other and then polls the file to read replies. A third
agent joined at 11:09 the same morning and had to append its own section to the
file before it could safely edit two documentation files.

That ledger works, and every line of it is a primitive AgentBox should have
provided: the claims are locks, the holder line is a lease, the notes are
messages, and the polling is what a signal exists to end. What it cannot do at
any price is answer the human's question - nothing in it says what either agent
is doing *right now*, so Boris's only view into two live agents was two
terminals and a `git status`. The cost is already recorded elsewhere too:
CLAUDE.md's traps are social locks (never `pkill agentbox`, `make run`
displaces the daemon Boris's live sessions reach him through), and the
session-37 handoff opens by telling the next agent to read the ledger and guess
whether the other one is still alive.

**Proposed shape**, in one line each, all detailed in 09-sync.md:

- A **roster** of live agent sessions, keyed to a persistent attach connection,
  each row carrying a mandated purpose and a live activity line, with the state
  chip derived from what the daemon can observe rather than from what the agent
  claims.
- An **Agents surface** in the app window, grouped by area, showing every
  agent's purpose, current activity and age, what it holds and what it waits
  on, with break-lock behind a confirm.
- **Discovery** by area (derived from the git top-level, refined by declared
  tags): joining returns your peers, and every later tool result carries a
  rider when company arrives, so a mid-work agent learns it is no longer alone
  without polling.
- **Locks** with FIFO waiters, deadlock refused by name at acquire time, and a
  dead holder's lock **orphaned rather than released** until its recorded pid is
  gone, because a dead mcp child does not mean a dead `make deploy`.
- **Signals** with durable pickup and cursors (agent-to-agent wake-ups, and
  direct messages over each session's private topic), and **shared values**
  with compare-and-swap for claim tables.
- A **session key** on `proto.Identity`, which is the prerequisite for all of
  it and fixes FR74's ownership check on the way past.

**Triaged and slice 1 shipped, 2026-08-04.** All three open questions were
answered in the design's favour; the answers and the one sharpening they gained
are at the foot of [09-sync.md](09-sync.md).

Built, deployed and verified live: the session key on `proto.Identity` (which
fixed FR74's ownership check on the way past), the attach stream, `announce`
returning same-area peers with `partial`, `set_activity` generalized to write the
roster always, `list_agents`, derived areas, the `agentbox sync` CLI, the Agents
rail surface, and the teaching in the same commit as the tools.

**Slices 2, 3 and 4 shipped the same day** (sessions 42 to 44), each verified live
against the deployed daemon and looked at on screen: the discovery rider, then
named locks with orphaning, deadlock refusal and break, then signals - post/await
over one global cursor, per-topic retention that reports a gap rather than serving
a batch with a hole in it, and the built-in `agents:<area>`, `to:<key>` and
`lock:<name>` topics - and then shared values, the compare-and-swap blackboard,
where `if_version: 0` claims a key only if nobody has it yet and an owner whose
session has died reads as abandoned instead of blocking the table.

**The ledger's four primitives all exist now**: claims are shared values, turns
are locks, the notes are signals, and the polling is over. The social locks
CLAUDE.md wrote down have machinery behind them.

The Agents surface shows all four: rows with their purpose and activity, the lock
table with its orphans, listening chips, and the blackboard in a block of its own
with abandoned claims at the top.

**And the teaching, slice 5, which is what makes it Boris's mandate rather than an
option**: hooks in `~/.claude/settings.json` put every session on the board and tell
it who its peers are, with no tokens and no instruction. Installing them found the
recipe could not work as written, so the session key is derived from the agent
process instead of minted - see "Identity: the session key" in 09-sync.md. Nothing
of FR83 is left to build.

Two things this work turned up that were nothing to do with sync, both recorded
in [history.md](history.md) session 40: `Conn.Serve` never told a blocking
handler its caller had hung up, so **FR45's caller-gone indicator had never
fired in the field**, and the CLI hold ceiling is 120s rather than the 1500s the
design assumed.

---

## FR84 [fixed 2026-08-05, session 46] A form's choice options are cut off mid-word

**Session.** 2026-08-04, FR83 triage. The three design questions went to Boris
as one `ask_user_form` card, each a choice field whose options were
sentence-length ("As designed: sync verbs refuse, reads and presence never").

**What was missing.** Observed on screen: every choice control clipped its
label mid-word at about 38 characters, with no wrap, no widening and no
tooltip, so the three options of a field were indistinguishable from each
other by their visible text. The fields also sat below the fold of a scrolling
body, so the card opened showing prose and no controls.

**What was done instead.** The card was answerable only because the body above
spelled every option out in prose and the recommended one arrived preselected,
so Boris could submit without reading the controls at all. A form whose
options were not explained in its own body would have been unanswerable, and
that is the shape most forms will have.

**Proposed shape.** Undecided on purpose, and it gets a mock like everything
else here. The candidates: wrap an option to two lines; switch a choice field
from a select to a radio list once any label passes a length threshold, which
also puts every option on screen at once; at minimum carry the full text in a
`title` so hovering says what the control cannot. Boris's own framing, sent
with the screenshot: *"we'll have to think of a better visual approach"*.

**Deferred by Boris the same day**, explicitly until FR83 is finished. Recorded
then so it survived the session that found it.

**Fixed 2026-08-05 after he picked the shape**, from a live mock of today's card
plus three approaches (radio list; select with the choice spelled out; radio list
with the controls above the prose). He chose the middle one, seeing its cost
written on the mock: you read the option you already picked, not the ones you did
not, so comparing three still means opening the menu. His call, and the compact
form is what keeps a whole form on screen.

What shipped: the select stays, and the chosen option is spelled out under it in
the value column, wrapped, whenever it is longer than 34 characters. The control
also carries the full text as a `title`, so hovering answers before clicking does.
The card's ResizeObserver regrows the window when the selection changes, and
`cardHeight` leaves room for the extra line in the height the card OPENS at, so it
does not open short and jump. The threshold is one number in two files with a test
that fails if they drift - FR85's lesson, applied the same day it was learnt.

**Watched on screen, both halves.** A three-field form with sentence-length
options opened at 470x437 with every control and the Submit button above the fold,
each spelled line reading its option in full, one of them wrapped to two lines.
Then, by keyboard: Tab to a choice and Down to a longer option grew the window
from 470x274 to 470x309 and wrapped the line to three, which is the resize claim
rather than an assumption about it.

**What this does NOT fix**, because the shape Boris chose does not address it: a
long body still pushes the fields down a scrolling card. The card that found FR84
had three paragraphs of prose above its controls, and it would still open on the
prose. Approach C in the mock (controls first, reasoning one click away) is what
addressed that half; it is unbuilt, and it needs his word rather than a session's
initiative.

---

## FR85 [fixed 2026-08-05, session 46] One agent, two different identity colours

**Session.** 2026-08-04, while building FR83's Agents surface, which needs an
agent to be recognisable by colour across surfaces.

**What is wrong.** There are two implementations of the identity hue and they
disagree. `IdentityHue` in `internal/webui/tokens.go` hashes
`agent + " " + project` with FNV-1a; `identityHue` in
`frontend/src/lib/tokens.js` hashes `agent + "\0" + project`. Same stop table,
same hash, different separator, so the same agent lands on different stops.
Computed, not guessed: `claude-code`/`agentbox` is 205 in Go and 300 in JS, and
four of five sampled identities differ.

The Go value paints Home's rows, the inbox rows, the progress rows and the
session list. The JS value paints `IdentityPill`, which is what cards, toasts
and the session header wear. So the colour whose whole job is to say "this is
the same agent" says the opposite between a card and the inbox row for that
same item.

**Why it survived.** The separator in the JS file is a **literal NUL byte** in
the template literal, which makes `grep` and `rg` classify `tokens.js` as a
binary file and skip it without printing a warning that reads as one. Every
search for `identityHue` across the frontend comes back empty, so the second
implementation is effectively invisible to the tools a session uses to find it.

**Proposed shape.** One separator, asserted in one place: pick the Go value
(it paints more surfaces), and if a NUL separator is kept at all, spell it as
the six characters `\u0000` rather than as the byte, so the file stays
text and stays greppable. Then pin both implementations to a fixed table of
identities in a test, so they cannot drift apart again without something going
red.

**Fixed** as proposed, with one thing the proposal had not seen. The frontend
takes Go's separator (a space), which also makes tokens.js text again, so the
search that used to come back empty now names the file. Pinning the two sides
against a table then showed a SECOND divergence in the same function: the
frontend hashed UTF-16 code units where Go hashes UTF-8 bytes. Identical for
every ASCII identity, and a different colour for the first project directory that
is not one, which on this machine is a matter of time. It hashes `TextEncoder`
output now.

Three tests, because each alone would have missed this: the table of eight
identities (Go, always), both implementations run over that table through node
(skipped where there is no node), and the shipped `dist` bundle checked for the
NUL, so a fix that was never rebuilt fails `make check` rather than on screen. The
cross-check was verified by breaking the separator on purpose and watching it name
both sides.

**Seen, not inferred.** A toast's pill and the inbox row for the same item, both
sampled off the screen: `hsl(225 62% 68%)` on each. The frontend would have
painted stop 30 for that identity before this build.

---

## FR86 [fixed 2026-08-05, session 46] A project is named after whatever directory the agent stood in

**Session.** 2026-08-04, session 41, putting real rows on the Agents board and
looking at them.

**What is wrong.** `Project` is `filepath.Base(cwd)` in every identity the CLI
and the mcp child build. An agent working in `~/me/projects/agentbox/frontend/src`
therefore reports its project as **`src`**, and the board showed
`aider · src` beside `codex · agentbox` for two agents in the same repo. The area
grouped them correctly (that is derived from the git root), so the row disagrees
with the heading it sits under.

It is worse than a label. `IdentityHue` hashes `agent + project`, so the same
agent in two subdirectories of one repo also gets two colours - the same failure
as FR85 arriving by a second route, and one FR85's separator fix would not
address.

**Fixed** as described: `DeriveProject` rides the walk `deriveArea` already did
and used to throw away, and every identity the CLI and the mcp child build calls
it instead of `filepath.Base`. Outside a repo the directory is still the honest
name, and an empty cwd still has none - `betterIdentity` reads an empty project as
"say nothing", not as a value, so the exotic cwd (`/`) loses a meaningless label
rather than gaining a wrong one.

Pinned in both packages: the daemon test walks a fixture repo (root and a
subdirectory produce one project string), and the webui test carries the pair the
board actually showed - one agent at the root and one in `frontend/src`, now one
colour.

**Watched against the running daemon** from `frontend/src`, which is the whole
bug in one line: the old binary announced `sleep · src`, the new one announces
`sleep · agentbox`.

---

## FR87 [fixed 2026-08-04, session 42] A daemon restart rewinds an agent's activity line

**Session.** 2026-08-04, session 41, watching my own row after `make deploy`.

**What is wrong.** The child's redial replays its `announce`, which is right -
the row comes back with its purpose intact, and that was verified live. But the
replay carries the activity the *announce* originally carried, not the line the
session has since moved on to. My own row came back reading "reconciling the
handoff against the repo and the deployed daemon" an hour after that was true,
with a fresh timestamp and a `working` chip, because `activityAt` is reset by the
replayed announce.

So a restart turns every live row into a confident statement about the past. It
is the same class of failure as the stale `working` chip fixed in `0566667`,
arriving from the other direction: there the age was right and the state was
wrong, here the state is right and the line is old.

**Fixed** as described: the child remembers the newest line `set_activity` sent
and replays the announce carrying that instead of the original. Verified live
across a `systemctl --user restart agentbox.service` - the row came back on the
line the session had moved to.

Two limits, stated rather than hidden. The age restarts at the replay, because
the roster is memory only by design and nothing survives to date the line from;
the line is right, its age is the restart's. And a line a HOOK sent
(`agentbox sync activity`) never passed through the child, so the child cannot
replay it - a session whose activity comes only from hooks still comes back on
whatever its model last said. Both are better than the confident lie, neither is
free to fix.

---

## FR90 [fixed 2026-08-04, session 43] A session that announced before it attached had no area

**Session.** 2026-08-04, session 43, while verifying slice 3. The signals probe
passed and the *rider* probe - slice 1's, which had passed all day - started
failing intermittently, differently each time: once "a rider arrived with no
change in the area", once "the agent was never told a peer joined".

**What was wrong.** An area is derived from where a session stands, and only the
**attach** carried a cwd. The attach is lazy (it starts on the first tool call), so
an `announce` can and does arrive first - always, for a hook that announces on a
session's behalf before its child has made a single call. That created a row with
no area at all, and three things follow from it, none of them visible on the board:

- **The row is invisible to the peers it exists to be found by.** Every
  area-filtered read skips a row whose area is empty, so a hook-announced session
  could sit in the same repo while another session's `announce` answered
  `alone: true`. That is the one claim the design says must be true when made.
- **Its rider cursor was never initialized.** `riderFor` returns early on a row with
  no area, *before* recording what the session has been told, so the announce's
  cursor-moving pass did nothing and the session's next call reported its whole
  area as news - a duplicate of what the announce had just shown it.
- **`peersOf` with an empty area filtered nothing**, so the same call could return
  every agent on the machine, telling an agent in one repo to coordinate with
  agents in another.

Latent since slice 1 and only intermittent, because it is a race between the
announce and the lazy attach. Slice 3 made the attach fractionally slower (it posts
a presence signal now), which is what turned an occasional flake into one often
enough to chase.

**Fixed.** `SyncAnnounceParams` carries `Cwd`, both doors send it, and the daemon
derives the area on announce exactly as it already did on attach. `peersOf` now
treats an unknown area as "cannot answer" - no peers, `partial: true` - rather than
as "everybody", because neither an empty list nor a full one is honest there.

**The lesson worth keeping:** the flake was in a probe for a slice that was already
finished, and the temptation was to read it as noise from another live agent.
Two consecutive runs failing *differently* is what said otherwise. A probe that
asserts on real, shared state will occasionally be right when it looks wrong.

## FR89 [fixed 2026-08-04, session 43] A posted item cannot be taken back, or cleared without the mouse

**Session.** 2026-08-04, session 42. Boris, mid-session: "Why do you keep popping
this 'Deadlock refused' panel? Did you need me to see it or to close it or
something?"

**What is wrong.** Nothing was wrong with the toast - a refused deadlock is one of
the two coordination events the design says should interrupt him. What was wrong is
that four of them were about locks named `probe:*` from an acceptance probe, they
came back on every daemon restart (a warning is pending until dismissed, and
pending items survive a restart by design), and **there is no way to clear them
except clicking**. There is no `agentbox dismiss ID`, no RPC behind one, and no
retraction: an agent that posts a toast it later knows to be noise has no way to
withdraw it, and a human at a terminal with no window open cannot empty the queue.

The asymmetry is the defect: agents have four doors to CREATE an item and none to
retire one.

**Fix.** Two verbs, one method: `agentbox dismiss [ID|--all]` for the human, and a
retraction for the agent that posted it (`notify_user` returns an id already;
withdrawing it should be the same id back through a `retract`). Non-blocking, and
it must only ever touch items the caller posted unless `--all` is the human's own
call. A test-only escape hatch is not enough - the real case is an agent that
posts "build failed", fixes it, and should be able to take it back before he ever
looks.

**Fixed, after he asked a second time.** Session 43 shipped it the moment the same
toasts came back: `agentbox dismiss ID... | --all` (the human's door, and only his
own may clear everything), the `retract` MCP tool (an agent's, and it may only ever
touch items that session posted), and `agentbox pending` - added because dismiss by
id is unusable without a terminal way to READ the ids, and `agentbox status` counts
them without saying what they are. One method behind all three; the asymmetry
between the callers is the safety model.

Exercised live rather than by reading the diff: a warning posted, its window seen on
screen (`agentbox · toast`), cleared with `agentbox dismiss --all`, and the window
gone with nothing pending afterwards.

**And the cause is fixed too, not only the symptom.** `tools/sync-probe.py` now
diffs the pending queue around its own run and dismisses exactly what it caused, so
an acceptance run that constructs a lock cycle on purpose no longer leaves a warning
on his screen. The diff matters: `--all` would also clear a real item of his that
happened to be waiting, and the deadlock warning is posted by `agentbox` itself
rather than by the probe's sessions, so an ownership-scoped retract cannot reach it.
That is the general shape - a warning about agents is the daemon's word, so the
harness that provoked it has to clean up through the human's door.

**The lesson about the lesson.** This sat on a solo-work list for a whole session
after he first asked, and it took him asking twice. A defect whose only symptom is
"the human keeps having to click something" reads as small on a list and is not: it
is a tax on every future run of the thing that causes it.

## FR88 [fixed 2026-08-04, session 42] Every blocking card was on a 30-minute fuse

**Session.** 2026-08-04, session 42, measuring the MCP client's idle cap before
building FR83's parked lock waits.

**What was wrong.** Claude Code aborts a stdio tool call that has said nothing
for **1800s**, and nothing in the child ever said anything. So a card the human
had not answered within half an hour was already dead at the agent's end while it
was still on his screen: he answers at minute 40, the answer goes to a caller
that is gone, and the agent got

    MCP server "agentbox" tool "ask_user" sent no response or progress for 1800s;
    aborting.

instead of an answer. Every blocking tool had this - `ask_user`,
`confirm_action`, `ask_user_form`, `request_review`, `await_walkthrough`,
`await_artifact_event`, `request_secret`, `act_unless_stopped` - and `timeout_s:
0`, documented as "waits forever", was the worst case rather than the safest one.

Never seen in the field because nobody had left a card up for half an hour and
then answered it, which is exactly the case AgentBox exists for: the human is away
from the terminal.

**Measured, not guessed.** `tools/idlecap-probe.sh` runs two headless Claude
sessions against `tools/idlecap-server.py`, one parking silently and one ticking
progress notifications. The silent call was aborted at 1800s with the message
above; the ticking one was still alive past it and returned normally. Two
mechanics fell out that the design had wrong:

- **The client sends `_meta.progressToken` on every `tools/call`**, so a server
  may always answer with `notifications/progress`. Its own `onprogress` handler
  resets the idle clock, which is why ticking works.
- **The client does NOT tell the server it gave up.** No
  `notifications/cancelled`, no closed pipe - the request is simply abandoned. A
  parked daemon call therefore has to have a ceiling of its own; nothing will
  arrive to cancel it.
- Two independent timers exist: the 1800s idle cap (stdio; 300s for
  http/sse) and a hard per-call ceiling of 1e8 ms, ~27.8 hours, which is
  effectively no limit. Only the idle one bites, and only progress resets it.
  `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT` (ms, 0 disables) and a per-server
  `timeout` in the MCP settings are the escape hatches; the ticker means neither
  is needed.

**Fixed** in `internal/mcp/keepalive.go`: receiving middleware over `tools/call`
sends one progress notification a minute while a call is parked, starting only
after the call has already lasted a minute so the fast tools - which is nearly
all of them - never tick at all. It covers every blocking tool at once, including
FR83's lock waits, which is why it is middleware rather than a change per tool.

## Authoring rules for walkthroughs - MOVED

This section shipped. It lives in `internal/manual/walkthrough.md`, embedded in
the binary, and reaches an agent three ways:

- MCP resource `agentbox://standards/walkthrough` (the canonical path: a client can
  list and read it on demand, so an agent about to author a review asks for the
  standard first and one doing anything else never pays for it)
- MCP prompt `walkthrough_standard` (a slash command in most clients, for
  putting the standard in front of an agent that did not think to ask)
- `agentbox docs walkthrough`

One copy, three doors. Every rule that was in this section is in that file, plus
the paragraph, note and bind rules the board can now honour. Add new rules
there, not here.

## FR91 [shipped 2026-08-06] Every step needs a TL;DR, and the board should open in it

**Session.** 2026-08-06, session 48, straight after FR65 shipped. Boris: "I want
every page in every walkthrough to have a TL;DR version for people with very
short attention span. So a person can either read just TL;DR or switch into the
exhaustive version and read in-depth."

**The sentence that decided the design**, sent a few minutes later and worth
quoting because the first reading of the request was wrong: "The content in
TL;DR is not necessarily less exhaustive, but it is to be optimally structured
for a person with a very short attention span that must still get a mastery
level of the most important aspects discussed."

So it is not a summary field. Nothing important is cut; what changes is the
STRUCTURE. That killed both obvious designs:

- **A free-text field** would have come back as the paragraph it exists to
  replace. Prose asks to be read from the start and held to the end; this has to
  survive being stopped anywhere.
- **A total character cap** would have made it the lossy version, which is the
  one thing it must not be.

**What shipped.** `tldr` is a shape: `bottom`, the one sentence that has to
survive, and up to six `points`, each a load-bearing fact standing on its own and
in any order. The caps are PER POINT (220 / 280), which bounds the shape without
asking the author to leave anything out - a point that will not fit is two
points, or belongs in the prose. Required on code and check steps with a teaching
error, because the board OPENS in TL;DR: a step without one shows the reader
nothing until they switch.

On the board: a two-state control in the header (TL;DR / Full text), `t` to
toggle, and a step written before this existed renders in full and says why
rather than showing an empty pane. Six rules for writing one went into
`internal/manual/walkthrough.md`, which is what agents actually read.

**Two things the screen said that the diff did not.** `all: unset` on the expand
button wiped the grid-column the step sets on every child, so it auto-placed into
the margin column and floated off to the right of the pane it was offering to
expand. And the first segmented control rendered as two bare words in the header
- styled in the stylesheet, unstyled on screen - until it was moved into its own
component. **Every `var()` in that component now carries a fallback**: a var()
that resolves to nothing takes its whole declaration with it, and a control that
has silently lost its background still reads as working to everything except the
screen.

---

## FR92 [shipped 2026-08-06] Group steps into domains, and show one at a time

**Session.** 2026-08-06, session 48. Boris: "I think that steps can be sometimes
grouped into domains and in such cases it must be shown visually, maybe even
showing one domain at a time which will also make the navigation less cluttered
when there are a lot of steps. The opening and closing of a domain and navigation
between the domains should be elegant and eye-pleasing, maybe even animated."

**What shipped.** A spec may declare two to six `domains` and give every step
one. The rail becomes an accordion: the domain holding the current step lists its
stations, the others collapse to a line carrying their own progress, so the shape
of the whole review stays visible while only one part of it is detailed. `[` and
`]` move by domain - the navigation the grouping exists to make possible, since a
reader who has decided a subject is not theirs should not press → through five
steps of it.

**Clicking a collapsed domain opens it WITHOUT navigating there.** Looking ahead
is a different act from going, and a rail that moved on every peek could not be
used to survey anything.

**Contiguity is validated, not assumed.** The board walks one domain at a time,
so a domain the step order leaves and returns to would open twice and finish
neither time. The error says to reorder or split.

**On the animation, which he asked for by name.** The drawer opens on
`grid-template-rows: 0fr -> 1fr`, so a step whose title wraps to two lines opens
as smoothly as one that does not - no measured pixel height, nothing to go stale.
The domain banner above the step is keyed on the DOMAIN rather than the step, so
it arrives once when the reader crosses into a new subject instead of flickering
on every arrow press. Both are off under `theme.motion` reduced or none.

**Ungrouped reviews render exactly as they always did**, which is the right answer
for a short walk rather than a fallback: under about eight steps the flat rail is
better and the ceremony costs more than the clutter it removes.

**One thing found on screen.** The domain blurb was printed twice - once in the
rail's open drawer, once in the step's banner. The rail is a route; the line
saying what a domain is about belongs where the reader is.

---

## FR93 [shipped 2026-08-06] Esc could not close a notification

**Session.** 2026-08-06, session 48, while other work was in flight. Boris, on two
urgent notifies another agent had raised: "Is it me or there is no way to dismiss
messages like this" and then "No matter how many times I press Esc, it pops back
up."

**What AgentBox was doing.** Esc deferred, ⇧Esc dismissed, and the card's hint
strip named only the first. Deferring is "not now, ask me again" - the right
answer to a question and a trap on a notification, which has nothing to answer:
the item stayed pending and escalation raised it again, every 20 seconds at
urgent. He was pressing the only key the card named, and it was the one key that
could not end it.

**What shipped.** On a notify card Esc dismisses; on everything else it still
defers and ⇧Esc forces dismiss. The hint says which of the two THIS card's Esc
will do rather than one wording for all of them. The toast surface has always
dismissed on Esc; the card was the odd one out.

**Worth keeping.** The affordance existed and was invisible, which is the same
failure FR65 fixed on the review board an hour earlier: a control that works and
is never written down is a control that does not exist.

---

## FR94 [field] Take the keyboard back mid-run, without ending the run

**Session.** 2026-08-06, session 49, straight after watching FR74's marker.
Boris: *"during hands-off I must be able to pause the hands-off and resume it
when I suddenly need the keyboard or mouse urgently."*

**What AgentBox cannot do.** A run is binary. An agent holds the desktop from
`request_control` to `release_control`, and for that whole stretch its
`drive_desktop` calls own the pointer and the keyboard. There is no way for
Boris to take them back for thirty seconds and hand them straight back. The
strip says HANDS OFF and means it.

**What he does instead.** Reaches for the mouse anyway, which is the failure
the whole feature exists to prevent: his click and the agent's click interleave
and neither of them knows. The alternative is worse - waiting for a run he did
not schedule, or killing it and losing whatever multi-step sequence was half
done.

**The shape.** A run gains a third state between held and released:

- **Paused is instant and reachable while an agent is typing.** That rules out
  anything needing focus. A global X11 hotkey grab (`XGrabKey` on the root
  window) is the obvious candidate, and the strip itself is the other - it is
  always on screen and always on top, so a click on it is a target that exists
  by construction. Probably both.
- **Paused must actually stop the input**, not just ask nicely. `drive_desktop`
  blocks (or refuses with a retryable error saying paused) rather than queuing,
  because a queue that drains on resume is a burst of clicks into whatever he
  left on screen.
- **The agent has to learn it, and wait rather than fail.** The natural shape is
  the one `acquire_lock` already has: block until it can proceed, with a timeout,
  and say who is holding it. A run that dies because its human needed the mouse
  is a worse outcome than a run that waits.
- **The strip says which state it is in**, and the paused wording has to be the
  one Boris reads at a glance - the sign's whole job is that the desktop's owner
  is never ambiguous. Amber for held, something else for paused.
- **Resume is his**, and only his. An agent must not be able to un-pause itself,
  or the pause is a suggestion.

**Open questions for the mock.** Whether a pause auto-resumes after some
idle period (a pause he forgets about strands the agent), what happens to a
`drive_desktop` already in flight when the hotkey fires, and whether pause is
per-run or a desktop-wide state that any waiting agent respects.

Related: FR74 (the strip and its guarantee), and `internal/webui/control.go`
plus the desktop lock in the daemon are where a run's state lives today.

---

## Mechanics discovered

Verified facts from field sessions, kept so a later session does not re-derive
them.

**Testing a NEW MCP tool from the session that just wrote it** (verified
2026-08-01, session 35). An MCP client fixes its tool list when the server
starts, and every Claude session holds an `agentbox mcp` child spawned from whatever
binary was deployed at the time. So a tool added this session is **not callable
as `mcp__agentbox__*` in this session** - the child predates it, and restarting the
agent to find out whether a tool works is a bad loop.

Speak to a fresh server over stdio instead. `agentbox mcp` is an ordinary stdio JSON-RPC
server, and its tools proxy to the daemon socket, so a new child sees the new
tools and reaches the same live daemon:

1. `initialize` (`protocolVersion`, `capabilities`, `clientInfo`), read the reply,
2. send the `notifications/initialized` notification (no id, no reply),
3. `tools/call` with `{"name": ..., "arguments": {...}}`; the answer is in
   `result.structuredContent`, and a tool-level refusal comes back as
   `result.isError` with the sentence in `result.content`, not as a JSON-RPC error.

One line per message, ids matched on the way back. Deploy first (`make deploy`)
or the child is the old binary too. This is how the assignment tools were
exercised end to end against the live daemon before any UI existed.

**Opening a JetBrains IDE at a position** (verified 2026-07-27, GoLand via
Toolbox, launcher `~/.local/share/JetBrains/Toolbox/scripts/goland`):

```
goland <projectDir> --line <n> --column <m> <absoluteFilePath>
```

The argument order matters. `goland --line 1367 <file>` is rejected with
`unrecognized option: --line`; the project directory has to come first. With the
project already open the command routes to the existing window rather than
opening a second one. Exit code is 0 either way, including on the rejected form,
so exit status is not a usable success check. `--help` documents the accepted
shape as `[/project/dir|--temp-project] [--wait] [--line <line>] [--column
<column>] file`.

**`drive_desktop`'s `window` step** (read from `internal/hand/hand.go`,
2026-07-27; **superseded by FR77 on 2026-08-01**). `Hand.UseWindow` walks the
X11 tree, picks a viewable window whose title matches, and stores its rect as
the coordinate frame. That is all it does. No raise, no activate, no focus. Keys
and text go to whatever already holds keyboard focus, independent of which
window the coordinate frame points at. The implementation is X11 through XTEST
throughout. FR77 turned this into a lock: raise, follow, and check every event
against the window before sending it.

**The JetBrains IDE MCP server** (probed live 2026-07-27, GoLand, endpoint
`http://127.0.0.1:64422/sse`, SSE transport with a session handshake: `GET /sse`
returns the endpoint, JSON-RPC then goes to `POST /message?sessionId=...`). 44
tools. Relevant to AgentBox only as the thing that cannot do this job: its editor
control is `open_file_in_editor(filePath)` and nothing else. Its `execute_tool`
is a dispatcher over the same registry, so nothing is hidden behind it. The
"brave mode" setting, off by default, makes `execute_terminal_command` and
`execute_run_configuration` block on a confirmation dialog.
