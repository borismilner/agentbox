# Backlog - what would make AgentBox answerable

Written 2026-08-07 against commit `74b3503`, by reading `frontend/src/surfaces/*`
in full for Card, Inbox, Toast, App, Progress, Settings and Agents, substantially
for Board, Panel and Viewer, and `frontend/src/lib/*` for the bridge, the rail,
the ask panel and the token applier. `docs/wiki/FACTS.md` was the starting point
for what is true; where a doc and the code disagreed, the code won. The Go side
was read wherever a surface's behaviour is decided there, which for the answer
path is most of it.

The bar is the product's own claim, in its own words: the card is "one blocking
item, answerable in under two seconds without the mouse" (`Card.svelte:8-9`), and
principle 2 is keyboard first. Every item below is measured against that, so the
ordering is by what the human loses and not by how hard it is to hit. A surface
that swallows an answer is a different product from a window that is 40px too
tall, and the bands say which is which.

Numbering is `U-nn`, sequential across bands, and does not collide with `R-nn` in
`robustness.md`, `F-nn` in `features.md` or the `FR` series. Where an item is the
UI-side face of a defect `robustness.md` already owns, it says so and does not
restate it.

Nothing here was found by looking at a running window. `robustness.md`'s R-40 is
the reason: no test renders any Svelte file, and this audit could not either
without taking Boris's daemon down. Every item is therefore marked with what
would settle it on screen.

**The bands**

| Band | What defines it |
|---|---|
| [A](#band-a) | The human answers and cannot tell whether it worked |
| [B](#band-b) | A control states something nothing checks |
| [C](#band-c) | The window is the wrong shape for what is in it |
| [D](#band-d) | The same destructive act is guarded differently by window |
| [E](#band-e) | Keyboard first, on some surfaces |
| [F](#band-f) | We would not find out |

---

## Band A

*The human answers and cannot tell whether it worked. This is the band that
decides whether the card is what it says it is.*

### U-01. The card has no failure path, in any of its twenty-six calls to the daemon

**How it fails.** `Card.svelte` makes 26 `bridge.*` calls. Not one is awaited, not
one has a `.catch`, and the file contains no `try {`. Counted across the tree, the
three surfaces where a human actually answers an agent - the card, the toast and
the inline ask panel - hold 38 calls between them and zero error handling of any
kind:

| Surface | `bridge.*` calls | `.catch` | `try {` | awaited |
|---|---|---|---|---|
| `Card.svelte` | 26 | 0 | 0 | 0 |
| `Panel.svelte` | 13 | 0 | 0 | 0 |
| `AskPanel.svelte` | 6 | 0 | 0 | 0 |
| `Toast.svelte` | 6 | 0 | 0 | 0 |
| `Session.svelte` | 5 | 0 | 0 | 0 |
| `Board.svelte` | 17 | 12 | 2 | 3 |

`Call.ByName` returns a promise that rejects when the runtime cannot dispatch:
a torn-down window, a serialization failure, a method name that no longer binds.
The board knows this and installs `window.addEventListener("unhandledrejection")`
at `Board.svelte:24` so a render error wears itself on screen. The card installs
nothing. A rejected `bridge.review(...)` goes to a console no human is reading,
and the card stays exactly as it was.
**Consequence.** The human presses `a` to approve, the card does not move, and
there is no difference on screen between "the daemon is thinking about it" and
"that keystroke went nowhere". The documented promise is a two-second answer; the
failure mode is an answer that looks like it is still being typed. The agent stays
parked, which is the product's own worst case reached from the human's side rather
than the agent's.
**Where.** `frontend/src/surfaces/Card.svelte` (all 26 sites),
`frontend/src/lib/bridge.js:11-28`, contrast `frontend/src/surfaces/Board.svelte:23-24`.
**Fix.** One wrapper in `bridge.js` rather than 26 call sites: wrap each answer-path
method so a rejection sets a surface-visible error, and give the card the same two
`window` listeners the board has. The tempting wrong fix is a `.catch(() => {})` per
call, which converts a silent failure into a silent failure that passes review.
**Test that would have caught it.** Mount `Card.svelte` with a bridge stub whose
`answer` rejects, press the key, and assert something in the DOM says so. Needs the
vitest harness R-40 asks for.
**On screen.** Stop the daemon with a card open, press the answer key, watch nothing
happen.
**Size.** hours.
**Confidence.** Confirmed by reading, with the counts verified by grep over the tree.

### U-02. The answer path in Go returns nothing, so the daemon cannot report a refusal even when it refuses

**How it fails.** Every answer-path method on the bridge is declared to return no
value:

```go
func (b *Bridge) Answer(id, label string) { b.ui.res.Answer(id, label) }
func (b *Bridge) Defer(id string)         { b.ui.res.Defer(id) }
func (b *Bridge) Promote(id string)       { ... }
```

(`internal/webui/bridge.go:43-65`, `:261`.) So even where the daemon knows perfectly
well that it did nothing, the shape of the call gives it nowhere to say so. This is
not hypothetical: `Promote` on an item that is pending but not in memory returns
without a log line or a repaint, which is `robustness.md`'s R-01 - the collapsed
question that no key can answer. R-01 is filed as a daemon defect and its fix is a
store fallback, correctly. This is the other half: had `Promote` been able to answer
"I could not", the surface could have said so, and R-01 would have been a visible
complaint rather than a key that does nothing forever.
**Consequence.** The UI can never distinguish "done" from "declined" on any answer.
Every future defect of R-01's shape will present identically: a control that does
nothing, twice, with no diagnostic anywhere.
**Where.** `internal/webui/bridge.go:43-65`, `:261-271`; the R-01 instance at
`internal/daemon/daemon.go:2089-2099`.
**Fix.** Give the answer-path methods a return value in the shape `Triage` already
has (`bool`, or better a short sentence for the surface to show). This is a wide but
shallow change and it is the precondition for U-01 being worth anything: a card that
can display a failure still needs something to be told.
**Test that would have caught it.** A Go test asserting `Promote` on a pending
out-of-memory item reports failure. Today there is nothing it could assert against.
**On screen.** R-01's repro: collapse a `request_review` behind three notifies,
dismiss the stack, press enter on the inbox row.
**Size.** a day.
**Confidence.** Confirmed by reading the signatures.

### U-03. The inbox throws away the one answer the daemon does give it

**How it fails.** `Triage` is the single exception to U-02: it returns `bool`, and
`inbox.act` returns `false` in three real cases - the item is not found, the item is
no longer pending, or the key maps to no intent for that kind
(`internal/webui/inbox.go:588-613`). The surface calls it and discards the answer:

```js
if (TRIAGE_KEYS.has(e.key)) {
  e.preventDefault();
  bridge.triage(chosen.id, e.key);
}
```

(`Inbox.svelte:165-168`.) No assignment, no await, no branch.
**Consequence.** Triage is the surface built for clearing a backlog fast, from the
keyboard, without reading a manual (`Inbox.svelte:7-11`). Pressing `y` on a row that
another window answered a moment ago, or pressing a key that means nothing for that
item's kind, looks exactly like pressing a key that worked: the row does not move
and nothing is said. In a burst, where rows resolve under the reader by design, the
false case is not an edge - it is what happens when two windows are open.
**Where.** `frontend/src/surfaces/Inbox.svelte:165-168`, `internal/webui/inbox.go:588-613`.
**Fix.** Await it and paint the false case. The row already has a hint line
(`Inbox.svelte:252-254`) that is the natural place for "that key does nothing here".
**Test that would have caught it.** Mount the inbox with a bridge stub returning
false and assert the surface says something. Needs R-40's harness.
**On screen.** Open the inbox beside a card, answer an item on the card, then press
its triage key in the inbox.
**Size.** hours.
**Confidence.** Confirmed by reading both sides.

---

## Band B

*A control states something nothing checks.*

### U-04. The daemon-health indicator is a literal

**How it fails.** The app window states that the daemon is up in two places, and
neither is connected to anything. `App.svelte:21` declares `let daemonUp = $state(true)`
and no line in the tree ever assigns it - grep finds exactly two occurrences, the
declaration and its one reader at `:71`. The status strip does not even go that far:

```svelte
<span class="up">● daemon up</span>
```

(`App.svelte:176`), with no condition at all.
**Consequence.** Smaller than it first looks, and worth saying precisely: the app
window is served by the daemon process, so if the daemon exits the window goes with
it and the green dot is never seen lying. What remains is real, though. A daemon
that is alive but wedged - R-19's GTK thread, R-21's panicked goroutine, a bridge
that has stopped dispatching - looks identical to a healthy one, and the one
indicator a human would check to tell those apart is a constant. The dot promises a
health signal the app does not compute.
**Where.** `frontend/src/surfaces/App.svelte:21`, `:71`, `:176`.
**Fix.** Either make it real (a heartbeat over the bridge, with the dot and the
strip reading its age) or remove both, because an indicator that cannot indicate is
worse than no indicator. The heartbeat is the more useful half: it is also what
would surface R-10's dead keepalive.
**Test that would have caught it.** A lint rule for declared-and-never-assigned
`$state`. Svelte's compiler does not warn on this and nothing else looks.
**On screen.** Not observable today, which is the point.
**Size.** hours to remove, a day for the heartbeat.
**Confidence.** Confirmed by grep over the whole tree.

### U-05. `theme.motion = "reduced"` is honoured by four components out of the thirteen that animate

**How it fails.** The knob has three values, `full | reduced | none`
(`internal/config/config.go:196`, `:509`), and it does reach the page:
`applyThemeTo` writes `el.dataset.motion = theme.motion` onto the root
(`frontend/src/lib/tokens.js:58`, called from `main.js:21`). What reads it back is
the problem. The global kill-switch in `app.css:999` is scoped to `"none"`:

```css
:root[data-motion="none"] * { animation-duration: 0.01ms !important; ... }
```

`"reduced"` is matched by five rules in four components and nowhere else: Progress
(`:194`), board's ModeToggle (`:93`), board's Rail (`:315`) and Board's domain
banner (`:590`). Thirteen files declare `@keyframes` or `animation:`. So App's
breathing "?" on a session waiting for an answer (`App.svelte:440-446`), the card,
the toast's drop-in, Home, Session, Agents, Mark, Control, the ask panel, Glossary
and Step all animate at `"reduced"` exactly as they do at `"full"`.

The OS-level setting is fine: `app.css:993` handles `prefers-reduced-motion: reduce`
with the same `*` sweep, and nine files add their own `@media` block on top of it.
It is the app's own middle setting that is nearly inert.
**Consequence.** "Reduced" is the value a person picks when motion bothers them but
they do not want to freeze every transition, and it is the obvious middle choice in
a three-item control. It delivers almost none of the animation reduction its name
promises, and there is no way to discover this short of counting keyframes. The one
infinite loop most likely to be the actual complaint - the pulsing question mark on
a session row - is not among the four.
**Where.** `frontend/src/app.css:993-1002`, `frontend/src/lib/tokens.js:58`, the five
`"reduced"` rules, and the thirteen animating files.
**Fix.** Decide what "reduced" means once and put it in `app.css` beside the other
two: the usual reading is that looping and decorative motion stops while
short state transitions stay. That is one rule killing `animation` globally at
`[data-motion="reduced"]` while leaving `transition` alone, after which the four
component rules can go.
**Test that would have caught it.** A CSS assertion that every `@keyframes` in the
bundle is reachable by a `[data-motion="reduced"]` disable rule. `frontend/policy_test.go`
already audits the shipped bundle as text and is the right place.
**On screen.** Set `theme.motion = "reduced"`, open the app with a session waiting
on an answer, watch the "?" keep breathing.
**Size.** hours.
**Confidence.** Confirmed by reading; the producer and all five consumers were
grepped for.

---

## Band C

*The window is the wrong shape for what is in it. The frameless surfaces size
themselves from their own content, so a measurement that does not happen is empty
space or a scrollbar.*

### U-06. The card's shrink list is hand-maintained, and two controls that shrink it are not on it

**How it fails.** A frameless card is exactly as tall as its content, measured by a
`ResizeObserver` and pushed to Go (`Card.svelte:146-177`). The observer can only see
the card grow: `.card` is `min-height: 100%`, so below the window's height the shell
is pinned and there is nothing to observe. `Card.svelte:179-185` says so, and
compensates with an explicit list of things that shrink it:

```js
$effect(() => {
  void proseOpen; void stackOpen; void replying;
  void view?.item?.id; void view?.graced;
  queueMicrotask(() => remeasure());
});
```

Two controls that shrink the card are not on that list.

The first is a form's `choice` field. `Card.svelte:442-444` renders a `.spelled`
span under the select only while the chosen option is longer than 34 characters.
Picking a long option grows the card, and the observer catches it - the comment at
`:440` says exactly that, and it is the half that works. Picking a short one after
it removes the span, and nothing remeasures.

The second is the review note. `use:autogrow` (`:111-119`) sets the textarea's
height from `scrollHeight` on every `input`, up to 104px. Growing is observed;
deleting text shrinks it and nothing remeasures.
**Consequence.** A band of dead window under the card until something else on the
list happens to fire. This is the same defect FR84 already paid for once, from the
same cause: `:152-154` records that a folding control left 190px of content in a
384px window. The list fixed the instances known that day and is now a standing
invitation to add the sixth control and forget the seventh.
**Where.** `frontend/src/surfaces/Card.svelte:186-193`, `:442-444`, `:111-119`.
**Fix.** Stop maintaining the list. Measure `scrollHeight` of the card's inner
content rather than the pinned shell, which is observable in both directions, or
observe a child that is not `min-height`-pinned. Either removes the class of bug
instead of its two current members.
**Test that would have caught it.** Mount the card with a form whose choice field
has one long and one short option, select each, and assert the reported fit height
goes down as well as up. Nothing can run this today.
**On screen.** `ask_user_form` with a choice field holding one 40-character option
and one short one. Pick the long, then the short.
**Size.** hours.
**Confidence.** Confirmed by reading. The mechanism is documented in the file; the
two omissions are not.

### U-07. A reused toast keeps the previous item's "click to expand"

**How it fails.** The toast decides whether a body is clipped by measuring it:

```js
$effect(() => {
  if (!body) return;
  clamped = !expanded && body.scrollHeight - body.clientHeight > 4;
});
```

(`Toast.svelte:58-61`.) The effect's tracked dependencies are `body` and `expanded`,
both `$state`. `body.scrollHeight` is a DOM read and is not tracked. The toast window
is reused across items - `on("agentbox:view", ...)` at `:35-39` swaps `view` and
resets `expanded`, and the `{#if view.bodyHtml}` block stays true, so `bind:this`
keeps the same node. A new item therefore replaces the body's content without
re-running the measurement.
**Consequence.** Both directions are wrong and both are silent. A long notification
arriving after a short one is clipped to three lines with no "click to expand" and
no way to know there is more - which is the exact failure FR73 built the inbox
detail to end, reappearing one window over. A short one arriving after a long one
offers to expand and does nothing when clicked.
**Where.** `frontend/src/surfaces/Toast.svelte:58-61`, `:35-39`, `:131`.
**Fix.** Take the reactive read: `void view?.item?.id` inside the effect, next to
`expanded`. It is the same one-line shape as `Card.svelte:186-193`, and it has the
same weakness, so measuring after paint on any view change is the better version.
**Test that would have caught it.** Mount the toast, push a short item then a long
one, assert `.more` appears.
**On screen.** `notify_user` with a one-line body, then one with six lines, into the
same toast window.
**Size.** hours.
**Confidence.** Confirmed by reading, against Svelte 5's tracking rules.

### U-08. An expanded toast dismisses itself when you click into it to read

**How it fails.** The whole strip is one click target, and the handler is:

```js
function onClick() {
  if (clamped && !expanded) { expanded = true; return; }
  dismiss();
}
```

(`Toast.svelte:71-77`.) Once expanded, `clamped` is false by construction, so every
subsequent click anywhere in the strip dismisses. The expanded body is
`max-height: 210px; overflow-y: auto` (`:251-256`), so a long notification is
scrollable - and clicking its scrollbar, or clicking to place a cursor before
selecting a passage, lands on `.toast` and takes it away. The body is `.selectable`
(`:131`), so selecting text is invited.
**Consequence.** The one gesture for reading a long notification destroys it. Recovery
is the inbox, which is fine, but the human does not know that at the moment the strip
vanishes. Wheel scrolling is unaffected, so the defect is intermittent by input habit,
which is the kind that gets reported as "it sometimes just disappears".
**Where.** `frontend/src/surfaces/Toast.svelte:71-77`, `:101-107`, `:251-256`.
**Fix.** Once expanded, dismissal belongs to the close button and to the
keyboard, not to the strip. That button already exists for sticky notices
(`:117-124`); show it whenever
expanded, and drop the click-to-dismiss on the body.
**Test that would have caught it.** Mount an expanded toast, dispatch a click inside
the body, assert `dismiss` was not called.
**On screen.** `notify_user` at `warning` with a twenty-line body, expand it, click
into the text.
**Size.** hours.
**Confidence.** Confirmed by reading. That a scrollbar click fires `click` on the
element is browser behaviour and was not checked in WebKitGTK; the
select-a-passage path does not depend on it.

---

## Band D

*The same destructive act is guarded differently depending on which window you are
in.*

### U-09. The panel ends an idle session on one click; the app window always asks

**How it fails.** Ending a session kills the child and takes an unsaved conversation
with it - `bridge.js:34-36` says so. Two surfaces offer it, with two different
guards. `App.svelte:104-119` always asks, in a sentence, and varies the sentence by
state: "Claude is working here. End it anyway?" or "End this session? An unsaved
conversation goes with it." `Panel.svelte:63-71` asks only when the session is
working, by arming its close button into a "sure?" that disarms itself after
three seconds:

```js
function close(s) {
  if (s.state === "working" && arm !== s.id) { arm = s.id; setTimeout(...3000); return; }
  arm = ""; bridge.closeSession(s.id);
}
```

An idle session in the panel is gone on the first click, with no confirmation and no
undo.
**Consequence.** "Idle" means the agent is not mid-turn. It does not mean the
conversation is worthless, and the app window's own wording says as much on exactly
the same session. The panel is the surface reached by a hotkey while doing something
else, which is the worst context in which to make the guard weaker.
**Where.** `frontend/src/surfaces/Panel.svelte:63-71`, `:146-148`; contrast
`frontend/src/surfaces/App.svelte:104-119`, `:128-135`.
**Fix.** One guard, in one place, used by both. The app window's is the right one to
keep, and the panel's arm-and-expire is the right interaction to keep it in given the
width - the disagreement to settle is whether idle needs confirming, and it does.
**Test that would have caught it.** Nothing structural. This is what a keyboard-and-
click pass over both surfaces is for, which is why band F matters.
**On screen.** Open the panel, click the close button on an idle session.
**Size.** hours.
**Confidence.** Confirmed by reading both.

### U-10. Leaving Settings with unsaved changes discards them without a word

**How it fails.** Settings tracks dirty state well and says so: `dirty` at
`Settings.svelte:28`, a per-knob dot at `:134`, and a footer that counts the keys
still to write at `:286-289`. Nothing acts on it when the surface goes away. The rail
swaps components (`App.svelte:146-172`), so clicking any other rail icon destroys
`<Settings />`, and `load()` re-reads from Go on the next mount. There is no
`beforeunload`, no guard on the tab change, and no other copy of `values`.
**Consequence.** A knob changed and not saved is gone, and the surface has just spent
three affordances telling the reader it is being tracked. The live preview makes this
more likely rather than less: the theme is already applied to the preview subtree, so
the change looks committed.
**Where.** `frontend/src/surfaces/Settings.svelte:28`, `:33-41`, `:286-297`;
`frontend/src/surfaces/App.svelte:146-172`.
**Fix.** The rail should refuse to leave a dirty Settings without asking, which means
the shell needs to be able to ask - a small piece of shared machinery that Board's
`SubmitModal` is the closest existing thing to. Lifting `values` into the shell so it
survives a tab change is the cheaper half and worth doing either way.
**Test that would have caught it.** Mount the shell, dirty a knob, switch tab, switch
back, assert the value is still pending.
**On screen.** Settings, drag a slider, click Inbox, click Settings.
**Size.** hours.
**Confidence.** Confirmed by reading.

### U-11. Esc in a card's form field defers the card instead of leaving the field

**How it fails.** `Card.svelte:227-257` handles Escape. It blurs for a review's note
(`typing && kind === "diff"`), closes the reply hatch if one is open, and otherwise
dismisses a notify or stack and **defers everything else**. A `form` or `text` card
with the cursor in a field takes the last branch: Esc while typing defers the whole
card.

Every other field in the product does the opposite. The inbox blurs its search box
(`Inbox.svelte:123-133`), the board blurs any input (`Board.svelte:314-317`), the
panel explicitly ignores Esc while typing and says why (`Panel.svelte:88-98`), the
viewer closes find (`Viewer.svelte:159-165`), and the card itself blurs in the one
case it special-cases.
**Consequence.** The convention everywhere else in the app, and in most software, is
that Esc leaves the control. Here it removes the question, mid-sentence, along with
whatever was typed into it. Whether the typed values come back with the card was not
determined: `formValues` is reset only when the item id changes
(`Card.svelte:122-137`), so it depends on what the daemon shows in between, and
nobody has watched it. The header hint reads "Esc defer · ⇧Esc dismiss", which is
accurate and does not warn that a half-filled form is what is being deferred.
**Where.** `frontend/src/surfaces/Card.svelte:227-257`, `:264-271`; the four
contrasting surfaces above.
**Fix.** Extend the diff special-case to every kind: while typing, Esc blurs, and a
second Esc defers. That is already the documented reasoning at `:229-231`, applied to
one kind out of four.
**Test that would have caught it.** Mount a form card, focus a field, press Escape,
assert `defer` was not called.
**On screen.** `ask_user_form`, type into a field, press Esc.
**Size.** hours.
**Confidence.** Confirmed by reading. The fate of the typed values is not confirmed
and is the part to watch.

---

## Band E

*Keyboard first, on some surfaces.*

### U-12. Four rail surfaces have no keyboard path beyond Tab

**How it fails.** Eight of seventeen surface components install no key handler at
all: Assignments, Control, History, Home, Library, Mark, Progress and Settings. Four
of those are full surfaces reachable from the rail - Assignments, History, Library
and Settings - where the entire interaction is pointer or Tab order. Settings is the
sharpest case, because it is a dense form: no Ctrl+S to save, no Esc to revert, no
way to move between sections without the mouse, next to a Save button that is the
whole point of the surface.

For contrast, the surfaces that do have keys have good ones: the card, the inbox, the
board and the viewer each carry a documented table, and the inbox and the card
deliberately share their meanings through Go so they cannot drift
(`Inbox.svelte:13-14`).
**Consequence.** Principle 2 is keyboard first and `FACTS.md:190-191` records it as
borne out, citing the key tables in `Card.svelte`, `Inbox.svelte` and `Board.svelte`.
Those three are exactly the surfaces that have tables. The claim is true of the
product's centre and not of its edges, and the edges are where a long session
actually spends time.
**Where.** `frontend/src/surfaces/{Assignments,Control,History,Home,Library,Mark,Progress,Settings}.svelte`.
**Fix.** Settings first and on its own merits: Ctrl+S, Esc, and section switching.
The others are a smaller debt - History and Library want the inbox's `j`/`k` and `/`,
which is a pattern that already exists and could be a shared action rather than a
fourth copy.
**Test that would have caught it.** An inventory test asserting every rail surface
installs a key handler would have caught the absence, though not the quality.
**On screen.** Open Settings and try to change and save anything without the mouse.
**Size.** a day.
**Confidence.** Confirmed by grep, then by reading each of the eight.

### U-13. The rail has no shortcuts, and does not tell a screen reader which surface is current

**How it fails.** Nine surfaces, all reachable only by clicking or by tabbing through
in order (`Rail.svelte:24-88`). There is no Ctrl+1..9, and the shell installs no key
handler of its own - `App.svelte`'s only `onkeydown` is on a session row. Separately,
the current surface is marked with `class:on` and nothing else: grep finds no
`aria-current` anywhere in the tree, the `<nav>` has no `aria-label`, and the buttons
are icon-only with `title` as their sole accessible name.
**Consequence.** The most-repeated navigation in the app costs a pointer or up to
nine Tab stops. For a screen reader, the rail is nine similarly-named buttons with
no statement of which one is active.
**Where.** `frontend/src/lib/Rail.svelte:24-88`, `frontend/src/surfaces/App.svelte`.
**Fix.** `aria-current="page"` on the active button and an `aria-label` on the nav
are both one-liners. Ctrl+1..9 belongs in the shell, where the `tab` state already
lives.
**Test that would have caught it.** An a11y assertion over a mounted shell. Needs the
R-40 work plus an axe-style checker, which is a decision beyond it.
**On screen.** Tab through the app window and count.
**Size.** hours.
**Confidence.** Confirmed by reading and by grep for `aria-current`.

### U-14. `c` copies on three surfaces and is advertised on none

**How it fails.** `c` copies the current item on the card (`Card.svelte:279-283`), the
toast (`Toast.svelte:86-89`), the inbox (`Inbox.svelte:160-164`) and the ask panel
(`AskPanel.svelte:43-47`). None of the four says so. The card is the clearest case,
because it is otherwise diligent: it prints `Esc` and `⇧Esc` in its header, the
option numbers and `y`/`n` on the buttons themselves, `a`/`r`/`n`/`p` on the review
row, `e` on the stack, `?` on the fold and `/` in the footer - eleven keys across
five places, and not `c`. The toast is the worst of the four: it displays no keys at
all, and answers to Esc, Enter and Space as well without saying so.
**Consequence.** A working feature nobody finds. Copying what an agent said is
precisely the affordance a human wants at the moment a card is about to close, and it
exists on every surface that could offer it. The toast is the worst of the four,
because it has no hint line at all and is the surface most likely to be a reader's
first contact with the product.
**Where.** the four handlers above; the hint sites at `Card.svelte:381-385`, `:492-493`,
`:558`, `:569` and `Inbox.svelte:182-184`.
**Fix.** Add `c` to the card footer and the inbox hint, and give the toast a hint
line. There is space: the toast's `.top` row already carries a spacer and a clock.
**Test that would have caught it.** Nothing structural; a keymap inventory checked
against the rendered hints would do it, and would be cheap once anything can
mount a component at all.
**On screen.** Look at a toast and try to find out what any key does.
**Size.** hours.
**Confidence.** Confirmed by reading all four handlers and all three hint sites.

---

## Band F

*We would not find out.*

### U-15. Every item above was found by reading, and nothing in the tree could have found any of them

**How it fails.** `robustness.md`'s R-40 owns this and should be read there rather
than restated: no vite, no jsdom, no Svelte compilation in any test, 32 files and
13,158 lines never executed. What this audit adds is the UX-specific shape of the
gap. Of the fifteen items above, eleven (U-01, U-03, U-05, U-06, U-07, U-08, U-10,
U-11, U-12, U-13, U-14) would be caught by a test that mounts a component and asserts
against the DOM, which is the harness R-40 already specifies. Three more need
something R-40 does not mention:

- an a11y checker, for U-13 and for the icon-only-button pattern the rail is not
  alone in using;
- a contrast check over the token sets, which nothing performs and which the theme
  system makes cheap to get wrong (the inbox badge is `--k-warning` behind
  `--k-ground` text at 0.58rem, `Rail.svelte:146-162`, and no rule anywhere says that
  pair must stay legible);
- a keymap inventory checked against rendered hints, for U-14.

U-04 is caught by neither and wants a compiler lint for declared-and-never-assigned
`$state`.
**Consequence.** The audit that produced this file is the mechanism the project has,
and it runs when somebody asks for it. Between asks, a surface defect is found by
Boris using the product, which for the frameless windows means it is found while he
is doing something else.
**Where.** `frontend/package.json`, `frontend/policy_test.go:26-30`, `Makefile:169`.
**Fix.** R-40's, plus the three above once it exists. The ordering R-40 gives is
right, and U-06 is the strongest argument for its first test: the card's fit height
is a number, assertable in both directions, and it already has two known-wrong cases
to pin.
**Test that would have caught it.** This is the entry.
**Size.** covered by R-40's week; the three additions are a day each.
**Confidence.** Confirmed by grepping the test tree, and by this document existing.

---

## Checked and found correct

Recorded so nobody refiles them.

- **The card's height is measured, not estimated.** A `ResizeObserver` reads the real
  laid-out height and Go resizes around it (`Card.svelte:143-177`), with the
  re-entrancy guard and the `min-height` circularity both handled and both explained
  at the line. An earlier note in this project's handoff described the height as
  estimated from body length; it is not, and U-06 is a much narrower defect than that
  would have been.
- **The identity hue is one hash in two languages, deliberately.** `tokens.js:79-94`
  hashes UTF-8 bytes to match Go, records that Go owns it, and documents the NUL-byte
  separator that made the JS copy invisible to grep. Reds are excluded so no agent can
  look like an error.
- **The inbox detail cannot outlive its row.** `Inbox.svelte:68-73` closes the detail
  when its row leaves the list and re-reads it when the row changes, and
  `Agents.svelte:164-182` does the same with a `detFor` guard, each citing the other's
  mistake. Both drop a reply that arrives after the reader has moved on.
- **`min-width: 0` is applied where a `1fr` track would otherwise widen the window.**
  `App.svelte:458-469` pins the main track and records that the defect was found by
  opening a shared value whose JSON had no spaces; `Inbox.svelte:628-636` pins the
  detail block for the same reason.
- **The stack card's footer states what dismissing costs before it is pressed**
  (`Card.svelte:570-578`), counting only rows still waiting, and Esc's meaning is
  varied by kind with the reason written down (`:240-257`). This is the fix for a real
  complaint and it holds.
- **The ask panel never takes the keyboard.** `AskPanel.svelte:11-15` and `:40-58`
  keep the composer focused, act only when nothing is being typed, and change the hint
  to match which of those states you are in.
- **Destructive actions in Library and Assignments both confirm first**
  (`Library.svelte:17`, `Assignments.svelte:23`), so U-09 is a disagreement between
  two surfaces rather than a missing guard across the product.
- **Reduced motion at the OS level works everywhere.** `app.css:993-997` sweeps every
  animation and transition, and nine components add their own `@media` block on top.
  U-05 is about the app's own knob only.
- **The toast's clamp cap is in px for a stated reason** (`Toast.svelte:247-256`): a
  `vh` cap would feed back into a window sized from the element, and the file says so.
- **A form's choice field cannot submit a value that was never on the menu.**
  `Card.svelte:206-213` starts an undefaulted choice on its first option, and
  `:215-221` stringifies booleans because one boolean in the map failed the whole
  call and closed the card as if nothing had happened.
