// FR58 review board - THE MOCK. For deciding, not for shipping (the working
// rule in docs/07-field-requests.md). It is loaded with a real review: the
// session-25 diff of agentbox itself, so every step below points at code that
// actually shipped today. Throw this file away without regret once the
// requirements settle; what it decides goes into the FR58 entry.
//
// What it exercises, against the stated requirements: the step rail drawn as
// a route (ground and gate ride along but only code steps count), three-valued
// state where "unclear" is the loud one, two separate visual channels on code
// (diff status vs numbered annotations, with a legend), one step holding
// several code blocks, deleted and changed lines shown with their old numbers,
// click-to-pop numbered annotations that drag and reopen, prose-to-code
// binding in both directions, select-code-to-comment with exact anchors,
// comprehension checks with hidden answers, a per-step closing note (multiline,
// any length), FR61 coverage with an out-of-scope bucket, a pinned commit with
// a drift chip, cross-step find hits (the board answers the window's find with
// where else the text lives), the gate as the last station, and one submission
// that emits everything at once.
import React, { useEffect, useMemo, useRef, useState } from "react";

// ---------------------------------------------------------------- the review
// A declarative spec, which is the whole FR58 contract: the agent supplies
// this, agentbox supplies everything below it.

const REVIEW = {
  title: "agentbox, the twenty-fifth session",
  repo: "~/me/projects/agentbox",
  // Anchors display repo-relative (the FR58 rule: never truncate a package
  // away), but a COPY yields the absolute path - it goes into tools that
  // know nothing about the repo (owner, 2026-07-28).
  root: "/home/boris-milner/me/projects/agentbox",
  pinned: "dd375a3cb2c7",
  drift: "tree matches",
  coverage: { hunks: 22, covered: 18, outOfScope: 4, uncovered: 0, note: "out of scope: docs and handoff prose" },
};

// Each code step carries `codes`: one or more blocks, because one location
// rarely tells a whole story (round 4: "the code block or blocks"). A block:
//   path, start   - the anchor, verified against the tree (FR61)
//   lines         - the NEW side of the file
//   new: true     - every line in this block was added
//   added: [...]  - or: exactly these new-file lines were added
//   del: [...]    - blocks of REMOVED lines: shown after new-file line
//                   `after`, numbered from their OLD-file line `old`
//   notes: [...]  - numbered annotations: {at:[from,to], text}. The agent's
//                   "why", popped on click right where the code is.

const STEPS = [
  {
    id: "ground",
    kind: "ground",
    title: "What this diff lives on",
    purpose: "Vocabulary only. This stop does not count toward the total.",
    prose: [
      { t: "Three facts carry every change in this review. First: an X keyboard holds up to four " },
      { b: "groups", t: "groups" },
      { t: " (layouts), and a keycode resolves in whichever group is active at that instant, no matter who planned it. Synthetic XTEST input is indistinguishable from fingers, so it is resolved the same way. Second: the window manager keeps one bottom-to-top list of every window on screen, " },
      { code: "_NET_CLIENT_LIST_STACKING" },
      { t: ", and it is the only honest answer to “is this window in front”. Third: agentbox renders an item as a card or a toast (its " },
      { b: "treatment", t: "treatment" },
      { t: "), and keeps one prompt window at a time, reused while the treatment stays the same." },
    ],
    checks: [],
  },
  {
    id: "xkb",
    kind: "code",
    title: "The per-stroke group lock",
    purpose: "Serves: typed text must be the planned text (the release tag that reached a card in Hebrew). Decided by: docs/history.md.",
    prose: [
      { t: "The layout table is read from the first group, so every stroke is planned there. The fix is a guard that locks the " },
      { b: "planned", t: "planned group" },
      { t: " into the server immediately before each press. It is decided once per call: no XKB, or the human already on group one, and the guard is a no-op. The lock is " },
      { b: "unchecked", t: "unchecked on purpose" },
      { t: ": a checked request would wait a round trip between the lock and the press, and that gap is exactly where the desktop's revert wins. At the end the human's group is " },
      { b: "restated", t: "restated once" },
      { t: ". All of this file is new." },
    ],
    allNew: true,
    codes: [
      {
        path: "internal/hand/xkb.go",
        start: 118,
        new: true,
        lines: [
          "// guardGroup decides once per Type or Press call whether guarding is needed:",
          "// only when XKB works and the human's locked group is not the planned one.",
          "func (h *Hand) guardGroup() groupGuard {",
          "\twas, ok := h.lockedGroup()",
          "\tif !ok || was == 0 {",
          "\t\treturn groupGuard{}",
          "\t}",
          "\th.trace(\"keyboard group %d is active; each stroke locks group 1 first\", was+1)",
          "\treturn groupGuard{h: h, was: was}",
          "}",
          "",
          "// hold re-locks the planned group. Call immediately before every synthetic",
          "// key press, with nothing in between - see the file comment for why once is",
          "// not enough.",
          "func (g groupGuard) hold() {",
          "\tif g.h != nil {",
          "\t\tg.h.lockGroupUnchecked(0)",
          "\t}",
          "}",
          "",
          "// release states the human's group one last time. On a desktop that already",
          "// re-asserted it this repeats the truth; anywhere else it undoes the guard.",
          "func (g groupGuard) release() {",
          "\tif g.h != nil {",
          "\t\tg.h.lockGroup(g.was)",
          "\t}",
          "}",
        ],
        notes: [
          { at: [121, 124], text: "Decided once per call: no XKB, or the human already on group one, and this returns the zero guard - hold and release become no-ops and typing behaves exactly as before the fix." },
          { at: [134, 134], text: "Unchecked on purpose. A checked request waits a full round trip between the lock and the press, and that gap is where the desktop's revert wins. Ordering inside one X connection is the only thing that reliably holds (measured: 1 of 200 immediate reads caught the lock before GNOME's revert)." },
          { at: [142, 142], text: "Restated, not restored: on GNOME the desktop already re-asserted the human's group by now, so this repeats the truth. On any other desktop it undoes the guard." },
        ],
      },
    ],
    binds: { planned: [120, 127], unchecked: [132, 136], restated: [140, 144] },
    checks: [
      {
        q: "Why lock before every press instead of once per Type call?",
        a: "GNOME re-asserts the human's input source within about a millisecond of seeing the group move (measured: 1 of 200 immediate reads caught the lock before the revert). The only thing that reliably holds is ordering inside one X connection, so each press carries its own lock.",
      },
      {
        q: "What happens on a desktop with no XKB, or with group one already active?",
        a: "guardGroup returns the zero guard: hold and release are no-ops and typing behaves exactly as before the fix.",
      },
    ],
  },
  {
    id: "type",
    kind: "code",
    title: "Where the guard sits in Type",
    purpose: "Serves: the same defect, at its call site. Decided by: the shift-gap measurement in the session log.",
    prose: [
      { t: "The stroke loop holds the guard " },
      { b: "beforeshift", t: "before the shift press" },
      { t: " and again " },
      { b: "beforekey", t: "before the key itself" },
      { t: ". The second hold is not caution: h.send is a checked call, so the shift press costs a full round trip, and that gap is long enough for the desktop to take the group back." },
    ],
    codes: [
      {
        path: "internal/hand/hand.go",
        start: 379,
        added: [384, 392],
        lines: [
          "\tdefer release()",
          "",
          "\tfor _, s := range strokes {",
          "\t\ttime.Sleep(s.After)",
          "\t\tif s.Shift && !held && shiftOK {",
          "\t\t\tguard.hold()",
          "\t\t\tif err := h.send(xproto.KeyPress, shift, 0, 0); err != nil {",
          "\t\t\t\treturn fmt.Errorf(\"holding shift: %w\", err)",
          "\t\t\t}",
          "\t\t\theld = true",
          "\t\t} else if !s.Shift && held {",
          "\t\t\trelease()",
          "\t\t}",
          "\t\tguard.hold()",
          "\t\tif err := h.send(xproto.KeyPress, s.Code, 0, 0); err != nil {",
          "\t\t\treturn fmt.Errorf(\"typing %q: %w\", s.Rune, err)",
          "\t\t}",
        ],
        notes: [
          { at: [384, 384], text: "Before the shift press: h.send is a checked call, a full round trip to the server, and that is long enough for the desktop to take the group back." },
          { at: [392, 392], text: "And again immediately before the key itself, so the lock and the press sit back-to-back in one connection's queue. The second hold is the fix, not caution." },
        ],
      },
    ],
    binds: { beforeshift: [384, 384], beforekey: [392, 392] },
    checks: [
      {
        q: "Press() takes the same guard. Which of its presses are guarded?",
        a: "Every modifier press and the main key press each get their own hold; the releases do not need one, because a release pairs by keycode, not by keysym.",
      },
    ],
  },
  {
    id: "titles",
    kind: "code",
    title: "One title per surface, written after the map",
    purpose: "Serves: “=agentbox” must mean the card and nothing else (STATUS item 6). Decided by: Choose prefers the bigger match.",
    prose: [
      { t: "The card keeps the bare name every script targets; toasts and the app window join the agentbox· family. It took two commits, and the blocks below tell it in order: the per-surface name, the changed option line, and the trap - the Title option never reaches X for a frameless window, because Wails skips gtk_window_set_title there. The bare agentbox the drivers matched for weeks was GTK's program-name fallback. So the name is written " },
      { b: "aftermap", t: "onto the mapped window" },
      { t: ", the way progress, panel and the viewer already did." },
    ],
    codes: [
      {
        path: "internal/webui/webui.go",
        start: 400,
        new: true,
        lines: [
          "\t// One title per surface, so a driver can say which window it means",
          "\t// (progress, panel and the viewer already carry theirs). The card keeps",
          "\t// the bare name every script and recipe targets as \"=agentbox\".",
          "\ttitle := \"agentbox\"",
          "\tif kind == \"toast\" {",
          "\t\ttitle = \"agentbox · toast\"",
          "\t}",
        ],
        notes: [
          { at: [403, 403], text: "The card keeps the bare name deliberately: every drive script and recipe on this machine targets \"=agentbox\". Renaming the other surfaces removes the ambiguity without breaking a single caller." },
        ],
      },
      {
        path: "internal/webui/webui.go",
        start: 408,
        added: [411],
        del: [
          { after: 410, old: 403, lines: ["\t\t\tTitle:         \"agentbox\","] },
        ],
        lines: [
          "\tu.onMain(\"card\", func() {",
          "\t\twin := u.app.Window.NewWithOptions(application.WebviewWindowOptions{",
          "\t\t\tName:          \"agentbox-\" + kind,",
          "\t\t\tTitle:         title,",
          "\t\t\tWidth:         w,",
          "\t\t\tHeight:        h,",
        ],
        notes: [
          { at: [411, 411], text: "Changed, not added - the option hard-coded the bare name before, and the removed line above shows it. It also turns out Wails never delivers Title for a frameless window at all; the write in the next block is what actually lands on X." },
        ],
      },
      {
        path: "internal/webui/webui.go",
        start: 436,
        added: [441, 442, 443],
        lines: [
          "\t\t\tif xid := xidOf(win.NativeWindow()); xid != 0 {",
          "\t\t\t\tu.x.prepare(xid, kind == \"toast\")",
          "\t\t\t\tshowNoActivate(win.NativeWindow())",
          "\t\t\t\t_, _, inset := u.toastGeom()",
          "\t\t\t\tu.x.settle(xid, w, h, kind == \"toast\", inset)",
          "\t\t\t\t// The Title option does not survive framelessness",
          "\t\t\t\t// (x11.go setName); write it onto the mapped window.",
          "\t\t\t\tu.x.setName(xid, title)",
          "\t\t\t\tu.armCard(payload)",
          "\t\t\t\treturn",
        ],
        notes: [
          { at: [443, 443], text: "The write that survives framelessness: after the map, straight onto the X window, the way progress, panel and the viewer already carry their names." },
        ],
      },
    ],
    binds: { aftermap: [441, 443] },
    checks: [
      {
        q: "Why does the card keep the bare name instead of getting agentbox · card?",
        a: "Every drive script, recipe and doc targets the card as \"=agentbox\". Renaming the other surfaces removes the ambiguity without breaking a single caller.",
      },
    ],
  },
  {
    id: "help",
    kind: "code",
    title: "Help must not perform",
    purpose: "Serves: STATUS item 7, widened to the class. Found while fixing it: agentbox quit --help actually quit the daemon.",
    prose: [
      { t: "Four commands ignored their arguments: inbox, quit, summon, mcp. All four now parse through a FlagSet like every flagged command, so -h and --help print one line of purpose and perform " },
      { b: "nothing", t: "nothing" },
      { t: ". The daemon survived agentbox quit --help on the current build; it did not survive it on the old one, mid-session, which is how the severity was discovered." },
    ],
    codes: [
      {
        path: "cmd/agentbox/main.go",
        start: 1337,
        new: true,
        lines: [
          "// noFlags gives a flagless command the same --help behaviour every flagged",
          "// command gets from its FlagSet. Without it, \"agentbox inbox --help\" opened the",
          "// inbox window and \"agentbox quit --help\" actually quit the daemon: a request for",
          "// the manual must never perform the action it asks about.",
          "func noFlags(name, what string, args []string) {",
          "\tfs := flag.NewFlagSet(name, flag.ExitOnError)",
          "\tfs.Usage = func() {",
          "\t\tfmt.Fprintf(os.Stderr, \"usage: agentbox %s\\n%s\\n\", name, what)",
          "\t}",
          "\tfs.Parse(args)",
          "}",
        ],
        notes: [
          { at: [1342, 1342], text: "flag.ExitOnError: an unknown flag prints the error plus the usage line and exits 2 before any daemon call happens. The manual can never perform the action it asks about." },
        ],
      },
    ],
    binds: { nothing: [1341, 1346] },
    checks: [
      {
        q: "What does an unknown flag do now, e.g. agentbox inbox --frobnicate?",
        a: "flag.ExitOnError prints the error plus the usage line and exits 2, before any daemon call is made.",
      },
    ],
  },
  {
    id: "stage",
    kind: "code",
    title: "The stage assertion, and the workaround it buried",
    purpose: "Serves: “slide 11 with no camera” (STATUS item 2). Decided by: the uploaded take's missing progress bar.",
    prose: [
      { t: "A rehearsal step that fails unless a named window is on screen and stacked in front of the fullscreen stage. agentbox drive where could never answer this: the daemon knows the window it made, not whether anyone can see it. The three " },
      { b: "verdicts", t: "verdicts" },
      { t: " were each seen live before the step went into the slide plan: above, UNDER, and never appeared. Right below it, the deletion this fix earned: the file's gsettings input-source workaround, measured doing nothing, reduced to a comment saying why." },
    ],
    codes: [
      {
        path: "tools/showcase/perform.py",
        start: 187,
        new: true,
        lines: [
          "    deadline = time.time() + seconds",
          "    verdict = \"never appeared\"",
          "    while time.time() < deadline:",
          "        win = window_id(title)",
          "        order = stacking_order()",
          "        if win is not None and win in order:",
          "            stage = stage_window()",
          "            if stage is None or stage not in order or order.index(win) > order.index(stage):",
          "                return None",
          "            verdict = \"on screen but UNDER the fullscreen stage\"",
          "        time.sleep(0.3)",
          "    return verdict",
        ],
        notes: [
          { at: [194, 195], text: "The stacking test: later in _NET_CLIENT_LIST_STACKING is nearer the eye, so win after stage means visible. No stage window at all (a rehearsal on a bare desk) degrades to: appearing is enough." },
          { at: [188, 188], text: "\"never appeared\" is the starting verdict and the loop only ever upgrades it. The pessimistic default is what makes the timeout honest." },
        ],
      },
      {
        path: "tools/showcase/perform.py",
        start: 199,
        added: [201, 202, 203, 204],
        del: [
          {
            after: 200,
            old: 201,
            lines: [
              "def latin_layout():",
              "    \"\"\"Put GNOME's input source back to the first one, and say so if it had moved.",
              "",
              "    AgentBox plans a keystroke against group 1's keysyms, but the X server types the",
              "    keycode in whatever group is *active*: with a Hebrew layout selected, the release",
              "    tag `2026.7.3` reached the card as `2026ץ7ץ3`, on camera. `record.sh prepare`",
              "    sets this for a take; this re-asserts it before every step that types, because a",
              "    per-window input source can put it back at any moment.\"\"\"",
              "    r = subprocess.run([\"gsettings\", \"get\", \"org.gnome.desktop.input-sources\", \"current\"],",
              "                       capture_output=True, text=True)",
              "    cur = r.stdout.split()[-1] if r.returncode == 0 and r.stdout.split() else \"0\"",
              "    if cur == \"0\":",
              "        return None",
              "    subprocess.run([\"gsettings\", \"set\", \"org.gnome.desktop.input-sources\", \"current\", \"0\"],",
              "                   capture_output=True, text=True)",
              "    return cur",
              "",
            ],
          },
        ],
        lines: [
          "",
          "",
          "# There is no input-source handling here any more. agentbox locks the planned keyboard",
          "# group around every synthetic key press itself (internal/hand/xkb.go), and the",
          "# gsettings write this file used to make was measured doing nothing: GNOME 46",
          "# ignores the deprecated `input-sources current` key in both directions.",
          "",
          "MAX_SPOKEN = 200  # AgentBox's own [speech] max_chars is 240; stay clear of it",
        ],
        notes: [
          { at: [201, 204], text: "Seventeen lines of workaround become four lines of why they are gone. Measured this session: GNOME 46 ignores the deprecated key in both directions, so none of this ever protected a take - the takes it seemed to protect were protected by the first group already being active." },
        ],
      },
    ],
    binds: { verdicts: [188, 196] },
    checks: [
      {
        q: "When there is no fullscreen stage at all, what does the step require?",
        a: "Only that the window appears. stage_window() returns None outside a take, and appearing is enough - the check degrades instead of failing a rehearsal run on a bare desk.",
      },
    ],
  },
  {
    id: "nothing",
    kind: "none",
    title: "Removals and documents",
    purpose: "Closing off territory explicitly, so nobody hunts for behavior that is not there.",
    prose: [
      { t: "perform.py's latin_layout() deletion is on display at the stage stop; record.sh's input-source block went the same way (deleted, not moved - the gsettings key it wrote is ignored by GNOME 46 in both directions, measured this session, so it never protected anything). The knowledge lives in showcase.md's trap table. Everything else outside the steps above is prose: history, STATUS, the handoff. There is nothing here to review." },
    ],
    checks: [],
  },
  {
    id: "gate",
    kind: "check",
    title: "The gate",
    purpose: "Finishing is an observation, not a feeling (FR61). Recorded 2026-07-28.",
    prose: [
      { t: "Two commands close the review. The expected results are stated, and the output recorded below is the one actually seen, not the one predicted." },
    ],
    cmds: [
      { cmd: "make check", expect: "18 packages, -race, all green", actual: "ok × 18, gofmt clean, vet clean (2026-07-28 16:58)" },
      { cmd: "make deployed", expect: "the daemon serves the reviewed commit", actual: "agentbox dd375a3cb2c7 built 2026-07-28T14:00:46Z" },
    ],
    checks: [],
  },
];

const COUNTED = STEPS.filter((s) => s.kind === "code").map((s) => s.id);

// ---------------------------------------------------------------- mechanics

// Rounds 6-7 lifted the grey ramp twice: the owner's "the numbers are dim and
// it's all dim in general" survived the first half-step, so round 7 finished
// it with measured ratios (ink 13.8:1, mut 8.2:1, dim 5.2:1 on their grounds).
// Hierarchy survives as ink > mut > dim; nothing informative renders below dim.
const K = {
  bg: "#141416", panel: "#1c1c20", line: "#2a2a30", ink: "#e3e0d6", mut: "#b3afa1",
  dim: "#8e8a7d",
  codeBg: "#17171a", add: "#3fa46a", del: "#c96a6a", look: "#58a6d4", unclear: "#d9a13b",
  ok: "#85a471", sel: "#a08fdb",
};

// Type is rem throughout (round 5): agentbox's A+/A- moves the document's rem root
// (base size from [font] size_pt), so anything sized in px would refuse to
// scale. Chrome that should hold still (borders, radii, shadows) stays px.
const CSS = `
  .rb { background:${K.bg}; color:${K.ink}; font-family:Cantarell,system-ui,sans-serif; }
  .rb ::selection { background:${K.sel}44; }
  .rb-prose { font-family:'Bitstream Charter',Charter,Georgia,serif; font-size:1.25rem; line-height:1.62; }
  .rb-mono { font-family:'JetBrains Mono','Fira Code',monospace; font-size:1rem;
             font-variant-ligatures:none; }
  .rb-bind { border-bottom:1px dotted ${K.look}; cursor:pointer; }
  .rb-bind:hover, .rb-bind.pulse { background:${K.look}22; border-bottom-style:solid; }
  .rb-ln { display:flex; white-space:pre; line-height:1.55; }
  .rb-ln:hover { background:#ffffff08; }
  .rb-ln.lit { background:${K.look}1f; }
  .rb-ln .no { width:3.5em; text-align:right; padding-right:14px; color:${K.dim}; flex:none;
               user-select:none; border-left:3px solid transparent; }
  .rb-ln.add .no { border-left-color:${K.add}; }
  .rb-ln.del .no { border-left-color:${K.del}; color:#a57676; }
  .rb-ln.del .txt { background:${K.del}12; color:#ab8c8c; }
  .rb-ln.cmt .no { color:${K.sel}; font-weight:700; }
  .rb-ln.cmt .no::before { content:"\\258f"; color:${K.sel}; margin-right:2px; }
  .rb-ann { display:inline-flex; align-items:center; justify-content:center;
            min-width:1.6em; height:1.6em; margin-left:10px; padding:0 0.3em; border-radius:0.8em;
            border:1px solid ${K.look}; color:${K.look}; font-size:0.78rem; font-weight:700;
            cursor:pointer; user-select:none; vertical-align:middle; }
  .rb-ann:hover { background:${K.look}22; }
  .rb-ann.on { background:${K.look}; color:${K.bg}; }
  .rb-annpop { position:absolute; width:min(32rem, 92%); z-index:7; border:1px solid ${K.look}66;
               border-radius:10px; background:${K.panel}; box-shadow:0 10px 30px #000a; }
  .rb-annpop .hd { display:flex; align-items:center; gap:6px; padding:4px 6px 4px 8px;
                   border-bottom:1px solid ${K.line}; cursor:grab; user-select:none; }
  .rb-annpop .hd:active { cursor:grabbing; }
  .rb-annpop .bd { max-height:40vh; overflow:auto; }
  .rb-findstrip { border-bottom:1px solid ${K.line}; padding:4px 16px; display:flex;
                  align-items:center; gap:8px; flex-wrap:wrap; }
  .rb-btn { border:1px solid ${K.line}; border-radius:8px; padding:5px 14px; font-size:0.95rem;
            background:transparent; color:${K.ink}; cursor:pointer;
            transition:border-color 140ms ease, background 140ms ease, color 140ms ease, transform 140ms ease; }
  .rb-btn:hover { border-color:#4a4a52; } .rb-btn:focus-visible { outline:2px solid ${K.look}; }
  .rb-btn:active { transform:scale(0.98); }
  .rb-station { display:flex; width:100%; text-align:left; background:none; border:0;
                margin:0 -6px; padding:2px 6px; border-radius:8px; cursor:pointer;
                color:inherit; font:inherit; transition:background 120ms ease; }
  .rb-station:hover { background:#ffffff08; }
  .rb-station:hover .st-title { color:${K.ink}; }
  .rb-station:focus-visible { outline:2px solid ${K.look}; }
  .st-title { display:-webkit-box; -webkit-line-clamp:2; -webkit-box-orient:vertical; overflow:hidden; }
  textarea.rb-in, input.rb-in { background:${K.codeBg}; border:1px solid ${K.line}; color:${K.ink};
            border-radius:8px; padding:7px 10px; font-size:1.05rem; width:100%;
            transition:border-color 120ms ease, box-shadow 120ms ease; }
  textarea.rb-in { resize:vertical; }
  .rb-in::placeholder { color:${K.dim}; }
  .rb-in:focus { outline:none; border-color:${K.look}88; box-shadow:0 0 0 2px ${K.look}33; }
  .rb-margin { border:1px solid ${K.look}33; border-left:2px solid ${K.look};
               border-radius:10px; background:${K.panel}; padding:9px 12px; cursor:pointer;
               transition:border-color 160ms ease, background 160ms ease; }
  .rb-margin:hover, .rb-margin.flash { border-color:${K.look}; background:${K.look}12; }
  .rb-step { animation:rbstep 180ms ease-out; }
  @keyframes rbstep { from { opacity:0; transform:translateY(6px); } }
  .rb-pulse { display:inline-block; animation:rbpulse 320ms ease; }
  @keyframes rbpulse { 50% { transform:scale(1.06); } }
  .rb-reveal { border-left:2px solid ${K.line}; padding-left:10px; animation:rbreveal 160ms ease-out; }
  @keyframes rbreveal { from { opacity:0; } }
  @media (prefers-reduced-motion: reduce) { .rb * { transition:none !important; animation:none !important; } }
`;

function stateColor(v) {
  return v === "understood" ? K.ok : v === "unclear" ? K.unclear : "#4a4a52";
}

// ------------------------------------------------------- syntax highlighting
// A toy tokenizer, enough to decide the look. The real board gets chroma
// (M6 already renders every fenced block through it); this exists so the mock
// shows colored code rather than asking anyone to imagine it.

const KW = {
  go: /\b(func|return|if|else|for|range|package|import|type|struct|interface|map|chan|go|defer|select|switch|case|break|continue|var|const|nil|true|false|error|string|byte|int|bool|uint16|uint32)\b/,
  py: /\b(def|return|if|else|elif|for|while|import|from|as|with|try|except|raise|pass|None|True|False|in|not|and|or|lambda|class|is|global)\b/,
};

// HL.cm sits at 5.4:1 on the code ground (round 7): in this codebase the
// comments carry the why, so they read as text, not decoration. HL.nu is
// hue-adjacent to K.unclear; syntax and severity are different channels, but
// never let a theme push it to amber's lightness.
const HL = { cm: "#8a9080", st: "#a3a86b", kw: "#86a3c3", nu: "#c9976b" };

function hl(line, lang) {
  const cmAt = lang === "py" ? line.indexOf("#") : line.indexOf("//");
  let code = line, tail = null;
  if (cmAt >= 0) { code = line.slice(0, cmAt); tail = line.slice(cmAt); }
  const out = [];
  let rest = code, k = 0;
  // KW's source is "\b(...)\b" and carries the one capture group itself, so it
  // slots in as group 4 unwrapped; numbers stay group 5.
  const rx = new RegExp(`("(?:[^"\\\\]|\\\\.)*")|(\`[^\`]*\`)|('(?:[^'\\\\]|\\\\.)*')|${KW[lang].source}|(\\b\\d[\\d._]*\\b)`);
  while (rest) {
    const m = rest.match(rx);
    if (!m) { out.push(<span key={k++}>{rest}</span>); break; }
    if (m.index > 0) out.push(<span key={k++}>{rest.slice(0, m.index)}</span>);
    const color = m[1] || m[2] || m[3] ? HL.st : m[5] ? HL.nu : HL.kw;
    out.push(<span key={k++} style={{ color }}>{m[0]}</span>);
    rest = rest.slice(m.index + m[0].length);
  }
  if (tail !== null) out.push(<span key={k++} style={{ color: HL.cm, fontStyle: "italic" }}>{tail}</span>);
  return out;
}

// everything a step shows or hides, flattened for the cross-step find count
function stepSearchText(s) {
  const parts = [s.title, s.purpose];
  s.prose.forEach((p) => parts.push(p.t || p.code || ""));
  (s.codes || []).forEach((c) => {
    parts.push(c.path, ...c.lines);
    (c.del || []).forEach((d) => parts.push(...d.lines));
    (c.notes || []).forEach((nt) => parts.push(nt.text));
  });
  (s.cmds || []).forEach((c) => parts.push(c.cmd, c.expect, c.actual));
  s.checks.forEach((c) => parts.push(c.q, c.a));
  return parts.join("\n").toLowerCase();
}

export default function ReviewBoard() {
  const [at, setAt] = useState(0);
  const [notes, setNotes] = useState({});   // id -> {state, sentence, comments:[], revealed:{}}
  const [lit, setLit] = useState(null);      // [from,to] highlighted lines
  const [pulse, setPulse] = useState(null);  // bind id flashed from a code click
  const [pend, setPend] = useState(null);    // pending selection {a,b,text,top,bi,path}
  const [hovCmt, setHovCmt] = useState(null); // {bi, key, list} comments under the hovered line
  const [copied, setCopied] = useState("");   // which copy control just fired
  const [modal, setModal] = useState(false);
  const [emitted, setEmitted] = useState(null); // "HH:MM" once submitted; drives the receipt
  const [pops, setPops] = useState({});      // "stepid:n" -> {x,y} open annotation pops
  const [drag, setDrag] = useState(null);    // {key,dx,dy} while a pop is dragged
  const [findQ, setFindQ] = useState("");    // the window's find, echoed by agentbox into the frame
  const [wide, setWide] = useState(false);   // room for margin notes right of the code
  const [pulseNote, setPulseNote] = useState(null); // margin note flashed from its number chip
  const codeRefs = useRef({});               // block index -> scroller element
  const bodyRef = useRef(null);              // the step column, pops position against it
  const popRef = useRef(null);

  // Round 6, on the owner's "make smart use of the dead space outside the
  // code": with room to spare, the agent's numbered whys stop hiding behind
  // clicks and sit open in the right margin, beside the blocks they explain.
  // The click-to-pop mechanic stays for narrow windows, unchanged.
  useEffect(() => {
    const mq = window.matchMedia("(min-width: 1680px)");
    const upd = () => setWide(mq.matches);
    upd();
    mq.addEventListener("change", upd);
    return () => mq.removeEventListener("change", upd);
  }, []);

  // A selection near the panel's fold would open the composer below the
  // visible area, leaving the reviewer typing blind; bring it into view.
  useEffect(() => {
    if (pend) popRef.current?.scrollIntoView({ block: "nearest" });
  }, [pend && pend.top]);

  const step = STEPS[at];
  const n = (id) => notes[id] || { comments: [], revealed: {} };
  const setN = (id, patch) => setNotes((p) => ({ ...p, [id]: { ...n(id), ...patch } }));

  const doneCount = COUNTED.filter((id) => n(id).state === "understood").length;
  const unclear = COUNTED.filter((id) => n(id).state === "unclear");

  // Round 7: the keyboard covers the whole loop - move, judge, move on - and
  // the footer bar below makes it discoverable (the adversarial pass's
  // "keyboard is invisible" finding).
  useEffect(() => {
    const h = (e) => {
      if (/INPUT|TEXTAREA/.test(e.target.tagName) || modal) return;
      if (e.key === "ArrowRight") setAt((a) => Math.min(a + 1, STEPS.length - 1));
      if (e.key === "ArrowLeft") setAt((a) => Math.max(a - 1, 0));
      if ((e.key === "u" || e.key === "x") && STEPS[at].kind === "code") {
        setN(STEPS[at].id, { state: e.key === "u" ? "understood" : "unclear" });
      }
      if (e.key === "Enter") {
        const after = STEPS.findIndex((s, j) => j > at && s.kind === "code" && !(notes[s.id] || {}).state);
        const any = after !== -1 ? after : STEPS.findIndex((s) => s.kind === "code" && !(notes[s.id] || {}).state);
        if (any !== -1) setAt(any);
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [modal, at, notes]);

  useEffect(() => { setLit(null); setPulse(null); setPend(null); setHovCmt(null); }, [at]);

  // The window's find reaches the board as the same message agentbox's runtime
  // answers; listening here is what lets the board say "also in step N" for
  // text that lives in steps not currently rendered. A find that silently
  // searches only the visible step would read as "nowhere in the review".
  useEffect(() => {
    const h = (e) => {
      if (e.source !== window.parent || !e.data || e.data.from !== "agentbox") return;
      if (e.data.type === "find") setFindQ(String(e.data.query || "").trim().toLowerCase());
    };
    window.addEventListener("message", h);
    return () => window.removeEventListener("message", h);
  }, []);

  const elsewhere = useMemo(() => {
    if (findQ.length < 2) return [];
    return STEPS.map((s, i) => {
      const t = stepSearchText(s);
      let c = 0, idx = t.indexOf(findQ);
      while (idx !== -1) { c++; idx = t.indexOf(findQ, idx + findQ.length); }
      return [i, c];
    }).filter(([i, c]) => c > 0 && i !== at);
  }, [findQ, at]);

  // dragging an annotation pop
  useEffect(() => {
    if (!drag) return;
    const move = (e) => setPops((p) => ({ ...p, [drag.key]: { x: e.clientX - drag.dx, y: e.clientY - drag.dy } }));
    const up = () => setDrag(null);
    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
    return () => { window.removeEventListener("mousemove", move); window.removeEventListener("mouseup", up); };
  }, [drag]);

  // selection -> anchored comment
  const onCodeMouseUp = (bi, path) => () => {
    const s = window.getSelection();
    const host = codeRefs.current[bi];
    if (!s || s.isCollapsed || !host) return;
    const ln = (node) => {
      for (let el = node instanceof Element ? node : node.parentElement; el; el = el.parentElement)
        if (el.dataset && el.dataset.ln) return +el.dataset.ln;
      return null;
    };
    let a = ln(s.anchorNode), b = ln(s.focusNode);
    if (a == null && b == null) return;
    if (a == null) a = b;
    if (b == null) b = a;
    const lo = Math.min(a, b), hi = Math.max(a, b);
    // The composer opens where the eyes already are: just under the selected
    // lines, inside the code panel, not at the bottom of the step.
    const el = host.querySelector(`[data-ln="${hi}"]`);
    const top = el ? el.offsetTop + el.offsetHeight + 4 : 0;
    setPend({ a: lo, b: hi, text: s.toString().trim().slice(0, 400), draft: "", top, bi, path });
  };

  // The sandbox has no clipboard permission, so copying goes the old way.
  const copyText = (label, text) => {
    const ta = document.createElement("textarea");
    ta.value = text;
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    ta.remove();
    setCopied(label);
    setTimeout(() => setCopied(""), 1200);
  };

  const scrollTo = (from) => {
    for (const host of Object.values(codeRefs.current)) {
      const el = host && host.querySelector(`[data-ln="${from}"]`);
      if (el) { el.scrollIntoView({ block: "center", behavior: "smooth" }); return; }
    }
  };

  const bindFor = (line) =>
    step.binds && Object.entries(step.binds).find(([, [f, t]]) => line >= f && line <= t);

  // annotations numbered across the step's blocks, in order
  const annots = useMemo(() => {
    let num = 0;
    return (step.codes || []).map((c) => (c.notes || []).map((nt) => ({ ...nt, num: ++num })));
  }, [step]);

  const togglePop = (key) => (e) => {
    if (pops[key]) {
      setPops((p) => { const q = { ...p }; delete q[key]; return q; });
      return;
    }
    const host = bodyRef.current ? bodyRef.current.getBoundingClientRect() : { left: 0, top: 0 };
    const r = e.currentTarget.getBoundingClientRect();
    setPops((p) => ({ ...p, [key]: { x: Math.min(r.left - host.left + 26, 380), y: r.top - host.top + 20 } }));
  };

  const payload = useMemo(() => {
    const lines = [`# Review: ${REVIEW.title}`, `pinned ${REVIEW.pinned} · ${REVIEW.drift}`, ""];
    lines.push(`## Unclear (${unclear.length}) - answer these first`);
    unclear.forEach((id) => {
      const s = STEPS.find((x) => x.id === id);
      lines.push(`- **${s.title}**${n(id).sentence ? ` - ${n(id).sentence.replace(/\n+/g, " ")}` : ""}`);
    });
    lines.push("", "## Steps");
    STEPS.forEach((s) => {
      const d = n(s.id);
      lines.push(`### ${s.title} [${s.kind}] - ${d.state || "unread"}`);
      if (d.sentence) d.sentence.split("\n").forEach((l) => lines.push(`> ${l}`));
      (d.comments || []).forEach((c) =>
        lines.push(`- ${c.path || ""}:${c.a}${c.b !== c.a ? "-" + c.b : ""} “${c.text}”\n  ${c.note}`));
    });
    const notReviewed = COUNTED.filter((id) => !n(id).state);
    lines.push("", `## Not reviewed (${notReviewed.length})`);
    notReviewed.forEach((id) => lines.push(`- ${STEPS.find((x) => x.id === id).title}`));
    lines.push("", `## Coverage`, `${REVIEW.coverage.covered} of ${REVIEW.coverage.hunks} hunks covered, ${REVIEW.coverage.outOfScope} out of scope, ${REVIEW.coverage.uncovered} uncovered (${REVIEW.coverage.note})`);
    return lines.join("\n");
  }, [notes, unclear.length]);

  const glyph = (s, i) => {
    const idx = COUNTED.indexOf(s.id);
    if (s.kind === "ground") return "◇";
    if (s.kind === "none") return "∅";
    if (s.kind === "check") return "$";
    return String(idx + 1);
  };

  return (
    <div className="rb flex h-screen flex-col overflow-hidden">
      <style>{CSS}</style>

      {/* header. Round 7: progress is pips you can watch fill, not a dim
          string; the count pulses once on completion. */}
      <div className="flex items-center gap-4 border-b px-5 py-3" style={{ borderColor: K.line }}>
        <div className="text-lg font-bold">Review · {REVIEW.title}</div>
        <div className="rb-mono text-[0.9rem]" style={{ color: K.mut }}>{REVIEW.repo}</div>
        <div className="rb-mono rounded-full border px-2 py-0.5 text-[0.9rem]" style={{ borderColor: K.line, color: K.mut }}>
          pinned {REVIEW.pinned} · <span style={{ color: K.ok }}>{REVIEW.drift}</span>
        </div>
        <div className="ml-auto flex items-center gap-1" data-agentbox-find-exclude>
          {COUNTED.map((id) => (
            <span key={id} style={{
              width: "1rem", height: "0.375rem", borderRadius: 2, transition: "background 240ms ease",
              background: n(id).state === "understood" ? K.ok : n(id).state === "unclear" ? K.unclear : K.line,
            }} />
          ))}
        </div>
        <div className={"text-[1rem]" + (doneCount === COUNTED.length ? " rb-pulse" : "")}
          style={{ color: doneCount === COUNTED.length ? K.ok : K.ink }}>
          {doneCount} of {COUNTED.length} understood
          {unclear.length > 0 && <span style={{ color: K.unclear }}> · {unclear.length} unclear</span>}
        </div>
        {/* The submit is an action, not a warning: it wears the board's action
            blue (round 6: "yellow for some reason" - the round-5 amber read as
            severity where none exists). Amber stays reserved for unclear. */}
        <button className="rb-btn" style={{ borderColor: K.look, color: K.look }} onClick={() => setModal(true)}>
          Submit review
        </button>
      </div>

      {/* where else the window's find hits: the review is bigger than the
          rendered step, and "no matches" must never mean "not in the review".
          data-agentbox-find-exclude keeps this strip out of agentbox's own hit count. */}
      {findQ.length >= 2 && elsewhere.length > 0 && (
        <div className="rb-findstrip rb-mono" data-agentbox-find-exclude style={{ color: K.look, fontSize: "0.85rem" }}>
          <span style={{ color: K.mut }}>“{findQ}” also in:</span>
          {elsewhere.map(([i, c]) => (
            <button key={i} className="rb-btn" style={{ fontSize: "0.85rem", borderColor: K.look + "55", color: K.look, padding: "1px 8px" }}
              onClick={() => setAt(i)}>
              {STEPS[i].title} ({c})
            </button>
          ))}
        </div>
      )}

      <div className="flex min-h-0 flex-1">
        {/* the route. Round 6: wider, bigger type, and the reading order of a
            worklist - the UNREAD stations are the bright ones, understood goes
            quiet, so remaining work is what catches the eye (the same inversion
            the FR62 diff card ships). Numbers render in ink, never in grey. */}
        <div className="flex w-80 flex-none flex-col overflow-y-auto border-r px-5 py-4" style={{ borderColor: K.line }}>
          {STEPS.map((s, i) => {
            const d = n(s.id);
            const counted = s.kind === "code";
            return (
              <button key={s.id} className="rb-station flex gap-3" onClick={() => setAt(i)}>
                <div className="flex flex-col items-center">
                  <div className="rb-mono flex h-8 w-8 flex-none items-center justify-center rounded-full border text-[1rem]"
                    style={{
                      borderColor: i === at ? K.look : d.state ? stateColor(d.state) : "#4a4a52",
                      color: d.state === "understood" ? K.ok : d.state === "unclear" ? K.unclear : counted ? K.ink : K.mut,
                      background: i === at ? K.look + "1a" : d.state === "unclear" ? K.unclear + "22" : "transparent",
                      fontWeight: i === at ? 700 : 500,
                    }}>
                    {d.state === "understood" && counted ? "✓" : glyph(s, i)}
                  </div>
                  {i < STEPS.length - 1 && <div className="w-px flex-1" style={{ background: K.line, minHeight: 20 }} />}
                </div>
                <div className="pb-4 pt-1.5 min-w-0">
                  <div className="st-title text-[1.05rem]"
                    style={{
                      color: i === at ? K.ink : d.state === "understood" ? K.mut : "#d5d2c8",
                      fontWeight: i === at ? 700 : 400,
                    }}>
                    {s.title}
                  </div>
                  {!counted && <div className="text-[0.85rem]" style={{ color: K.dim }}>
                    {s.kind === "ground" ? "does not count" : s.kind === "none" ? "nothing to review" : "the gate"}
                  </div>}
                </div>
              </button>
            );
          })}
          <div className="mt-auto border-t pt-2 text-[0.9rem]" style={{ borderColor: K.line, color: K.mut }}>
            <div className="mb-1 font-bold" style={{ color: K.ink }}>Coverage
              <span className="ml-1 font-normal" style={{ color: K.dim }}>· illustrative in the mock</span></div>
            <div className="mb-1 flex h-1.5 overflow-hidden rounded-full">
              <div style={{ flex: REVIEW.coverage.covered, background: K.ok }} />
              <div style={{ flex: REVIEW.coverage.outOfScope, background: "#4a4a52" }} />
              {REVIEW.coverage.uncovered > 0 && <div style={{ flex: REVIEW.coverage.uncovered, background: K.unclear }} />}
            </div>
            {REVIEW.coverage.covered} of {REVIEW.coverage.hunks} hunks covered · {REVIEW.coverage.outOfScope} out of scope · {REVIEW.coverage.uncovered} uncovered
          </div>
        </div>

        {/* the step. When margin notes are up, the column and its 340px margin
            center as one ensemble instead of leaving the notes in the gutter. */}
        <div className="min-w-0 flex-1 overflow-y-auto px-10 py-8">
          <div key={step.id} className="rb-step relative" ref={bodyRef}
            style={{ maxWidth: "58rem", margin: wide ? "0 auto 0 max(32px, calc((100% - 1336px)/2))" : "0 auto" }}>
            <div className="mb-1 flex items-baseline gap-3">
              <h1 className="text-[1.5rem] font-bold">{step.title}</h1>
              {step.allNew && <span className="rounded-full px-2 py-0.5 text-[0.8rem]" style={{ background: K.add + "26", color: K.add }}>all of this is new</span>}
            </div>
            <div className="mb-6 text-[0.95rem]" style={{ color: K.mut }}>{step.purpose}</div>

            <p className="rb-prose mb-6">
              {step.prose.map((seg, i) =>
                seg.b ? (
                  <span key={i} className={"rb-bind" + (pulse === seg.b ? " pulse" : "")}
                    onMouseEnter={() => step.binds && setLit(step.binds[seg.b])}
                    onMouseLeave={() => setLit(null)}
                    onClick={() => step.binds && scrollTo(step.binds[seg.b][0])}>
                    {seg.t}
                  </span>
                ) : seg.code ? (
                  <code key={i} className="rb-mono rounded px-1" style={{ background: K.codeBg }}>{seg.code}</code>
                ) : (
                  <span key={i}>{seg.t}</span>
                )
              )}
            </p>

            {(step.codes || []).map((blk, bi) => {
              const lang = blk.path.endsWith(".py") ? "py" : "go";
              const blkNotes = annots[bi] || [];
              return (
                <div key={bi} className="relative mb-5">
                <div className="overflow-hidden rounded-lg border" style={{ borderColor: K.line }}
                  onMouseLeave={() => setHovCmt(null)}>
                  <div className="flex items-center gap-2 border-b px-4 py-2" style={{ borderColor: K.line, background: K.panel }}>
                    <button className="rb-mono" title={"copy " + REVIEW.root + "/" + blk.path}
                      style={{ background: "none", border: 0, fontSize: "0.95rem", color: copied === "path" + bi ? K.ok : K.ink, cursor: "pointer", padding: 0 }}
                      onClick={() => copyText("path" + bi, REVIEW.root + "/" + blk.path)}>
                      {copied === "path" + bi ? "full path copied ✓" : blk.path}
                    </button>
                    <span className="rb-mono" style={{ color: K.mut, fontSize: "0.8rem" }}>
                      {blk.start}–{blk.start + blk.lines.length - 1}
                    </span>
                    {blk.new && !step.allNew && (
                      <span className="rounded-full px-1.5 text-[0.8rem]" style={{ background: K.add + "26", color: K.add }}>new</span>
                    )}
                    <span className="ml-auto" />
                    <button className="rb-btn" style={{ fontSize: "0.85rem" }} onClick={() => window.agentbox.emit("jump", { anchor: blk.path + ":" + blk.start })}>open in IDE</button>
                    <button className="rb-btn" style={{ fontSize: "0.85rem" }}
                      onClick={() => copyText("code" + bi, blk.lines.join("\n"))}>
                      {copied === "code" + bi ? "copied ✓" : "copy"}
                    </button>
                  </div>
                  <div ref={(el) => { codeRefs.current[bi] = el; }} className="rb-mono relative overflow-auto py-2" style={{ background: K.codeBg, maxHeight: "min(60vh, 40rem)" }}
                    onMouseUp={onCodeMouseUp(bi, blk.path)}>
                    {blk.lines.map((t, i) => {
                      const num = blk.start + i;
                      const isAdd = step.allNew || blk.new || (blk.added || []).includes(num);
                      const isLit = lit && num >= lit[0] && num <= lit[1];
                      const onLine = n(step.id).comments.filter((c) => c.path === blk.path && num >= c.a && num <= c.b);
                      const note = blkNotes.find((nt) => nt.at[0] === num);
                      const popKey = note && step.id + ":" + note.num;
                      const rows = [
                        <div key={num} data-ln={num}
                          className={"rb-ln" + (isAdd ? " add" : "") + (isLit ? " lit" : "") + (onLine.length ? " cmt" : "")}
                          onMouseEnter={() => {
                            // Only state that CHANGES: re-setting an identical hover
                            // re-rendered the step on every line the mouse crossed,
                            // and the footer toggling in and out of the scroller's
                            // layout was the round-5 jitter.
                            if (!onLine.length) { if (hovCmt) setHovCmt(null); return; }
                            const key = bi + ":" + onLine.map((c) => c.a + "-" + c.b).join(",");
                            if (!hovCmt || hovCmt.key !== key) setHovCmt({ bi, key, list: onLine });
                          }}
                          onClick={() => { const b = bindFor(num); if (b) { setPulse(b[0]); setTimeout(() => setPulse(null), 900); } }}>
                          <span className="no">{num}</span>
                          <span className="txt" style={{ background: isAdd ? K.add + "10" : "transparent", flex: 1, paddingRight: 12 }}>
                            {t ? hl(t, lang) : " "}
                            {note && (
                              <span className={"rb-ann" + (pops[popKey] ? " on" : "")}
                                title={wide ? "the note is in the margin" : pops[popKey] ? "close this note" : "why? click to pop the note"}
                                onMouseEnter={() => { if (wide) setPulseNote(popKey); }}
                                onMouseLeave={() => { if (wide) setPulseNote(null); }}
                                onClick={(e) => {
                                  e.stopPropagation();
                                  if (wide) { setPulseNote(popKey); setTimeout(() => setPulseNote(null), 900); return; }
                                  togglePop(popKey)(e);
                                }}
                                onMouseUp={(e) => e.stopPropagation()}>{note.num}</span>
                            )}
                          </span>
                        </div>,
                      ];
                      (blk.del || []).filter((d) => d.after === num).forEach((d, di) =>
                        d.lines.forEach((dt, dj) =>
                          rows.push(
                            <div key={"d" + di + "-" + dj} className="rb-ln del">
                              <span className="no">{d.old + dj}</span>
                              <span className="txt" style={{ flex: 1, paddingRight: 12 }}>{dt ? hl(dt, lang) : " "}</span>
                            </div>
                          )
                        )
                      );
                      return rows;
                    })}
                    {pend && pend.bi === bi && (
                      <div ref={popRef} className="absolute rounded-lg border p-2"
                        style={{ top: pend.top, left: 64, width: "min(40rem, 80%)", zIndex: 5, borderColor: K.sel, background: K.panel, boxShadow: "0 8px 28px #0009" }}>
                        <div className="rb-mono mb-1" style={{ color: K.sel, fontSize: "0.85rem" }}>
                          :{pend.a}{pend.b !== pend.a ? "–" + pend.b : ""} “{pend.text.slice(0, 70)}{pend.text.length > 70 ? "…" : ""}”
                        </div>
                        <textarea className="rb-in rb-prose" rows={2} autoFocus placeholder="Say it here. Ctrl+Enter adds it, Esc discards."
                          value={pend.draft} onChange={(e) => setPend({ ...pend, draft: e.target.value })}
                          onKeyDown={(e) => {
                            if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
                              e.preventDefault();
                              if (pend.draft.trim()) setN(step.id, { comments: [...n(step.id).comments, { a: pend.a, b: pend.b, text: pend.text, note: pend.draft.trim(), path: pend.path }] });
                              setPend(null);
                            }
                            // Esc discards, written words included - the owner's
                            // call (round 5), overturning the round-3 guard: Esc
                            // means "off my screen", the button stays for the mouse.
                            if (e.key === "Escape") { e.preventDefault(); setPend(null); }
                          }} />
                        <div className="mt-1.5 flex gap-2">
                          <button className="rb-btn" style={{ fontSize: "0.85rem" }} onClick={() => {
                            if (pend.draft.trim()) setN(step.id, { comments: [...n(step.id).comments, { a: pend.a, b: pend.b, text: pend.text, note: pend.draft.trim(), path: pend.path }] });
                            setPend(null);
                          }}>Add comment</button>
                          <button className="rb-btn" style={{ fontSize: "0.85rem" }} onClick={() => setPend(null)}>Discard</button>
                        </div>
                      </div>
                    )}
                  </div>
                  {/* Below the scroller, not inside it: appearing in the scroll
                      layout toggled the scrollbar and shifted the lines under
                      the pointer (the round-5 jitter). Here the panel grows
                      downward and the code above holds still. */}
                  {hovCmt && hovCmt.bi === bi && (
                    <div className="rb-prose border-t px-3 py-1.5 text-sm"
                      style={{ borderColor: K.sel, background: K.panel }}>
                      {hovCmt.list.map((c, i) => (
                        <div key={i}><span className="rb-mono text-[0.69rem]" style={{ color: K.sel }}>:{c.a}{c.b !== c.a ? "–" + c.b : ""}</span> {c.note}</div>
                      ))}
                    </div>
                  )}
                </div>
                {/* The dead space earns its keep (round 6): on a wide window the
                    whys sit open beside the code they explain. Hover lights the
                    lines; the number chip in the code flashes its note here. */}
                {wide && blkNotes.length > 0 && (
                  <div className="absolute flex flex-col gap-2"
                    style={{ left: "calc(100% + 1.5rem)", top: 0, width: "21rem" }}>
                    {blkNotes.map((nt) => {
                      const key = step.id + ":" + nt.num;
                      return (
                        <div key={key} className={"rb-margin" + (pulseNote === key ? " flash" : "")}
                          onMouseEnter={() => setLit(nt.at)} onMouseLeave={() => setLit(null)}
                          onClick={() => scrollTo(nt.at[0])}>
                          <div className="mb-1 flex items-center gap-2">
                            <span className="rb-ann on" style={{ marginLeft: 0 }}>{nt.num}</span>
                            <span className="rb-mono" style={{ color: K.mut, fontSize: "0.85rem" }}>
                              {blk.path.split("/").pop()}:{nt.at[0]}{nt.at[1] !== nt.at[0] ? "–" + nt.at[1] : ""}
                            </span>
                          </div>
                          <div className="rb-prose" style={{ fontSize: "1.15rem" }}>{nt.text}</div>
                        </div>
                      );
                    })}
                  </div>
                )}
                </div>
              );
            })}
            {step.codes && (
              <div className="mt-2 mb-6 flex flex-wrap gap-4 text-[0.85rem]" style={{ color: K.mut }}>
                <span><span style={{ color: K.add }}>▍</span> added in this diff</span>
                <span><span style={{ color: K.del }}>▍</span> removed (old line numbers)</span>
                <span><span className="rb-ann" style={{ marginLeft: 0 }}>1</span> {wide ? "the why - open in the margin; hover one to light its lines" : "the why - click to pop, drag it aside, close and reopen"}</span>
                <span className="ml-auto">select code to comment · click a marked phrase above to light its lines</span>
              </div>
            )}

            {/* popped annotations float over the step, draggable by their header */}
            {(step.codes || []).flatMap((blk, bi) =>
              (annots[bi] || []).map((nt) => {
                const key = step.id + ":" + nt.num;
                const pos = pops[key];
                if (!pos) return null;
                return (
                  <div key={key} className="rb-annpop" style={{ left: pos.x, top: pos.y }}>
                    <div className="hd"
                      onMouseDown={(e) => { e.preventDefault(); setDrag({ key, dx: e.clientX - pos.x, dy: e.clientY - pos.y }); }}>
                      <span className="rb-ann on" style={{ marginLeft: 0 }}>{nt.num}</span>
                      <span className="rb-mono" style={{ color: K.mut, fontSize: "0.8rem" }}>
                        {blk.path.split("/").pop()}:{nt.at[0]}{nt.at[1] !== nt.at[0] ? "–" + nt.at[1] : ""}
                      </span>
                      <span className="ml-auto" />
                      <button className="rb-btn" style={{ border: 0, padding: "0 4px", fontSize: "0.8rem" }} title="close (the number reopens it)"
                        onClick={() => setPops((p) => { const q = { ...p }; delete q[key]; return q; })}>✕</button>
                    </div>
                    <div className="bd rb-prose px-3 py-2" style={{ fontSize: "1.1rem" }}
                      onMouseEnter={() => setLit(nt.at)} onMouseLeave={() => setLit(null)}>
                      {nt.text}
                    </div>
                  </div>
                );
              })
            )}

            {step.cmds && step.cmds.map((c, i) => (
              <div key={i} className="mb-3 overflow-hidden rounded-lg border" style={{ borderColor: K.line }}>
                <div className="flex items-center gap-2 px-3 py-1.5" style={{ background: K.panel }}>
                  <code className="rb-mono" style={{ fontSize: "0.95rem" }}>{c.cmd}</code>
                  <button className="rb-btn ml-auto" style={{ fontSize: "0.85rem" }}
                    onClick={() => copyText("cmd" + i, c.cmd)}>
                    {copied === "cmd" + i ? "copied ✓" : "copy"}
                  </button>
                </div>
                <div className="px-3 py-2 text-[0.95rem]" style={{ color: K.mut }}>
                  expect: {c.expect}<br />
                  <span style={{ color: K.ok }}>seen: {c.actual}</span>
                </div>
              </div>
            ))}

            {n(step.id).comments.map((c, i) => (
              <div key={i} className="group mb-2 flex items-baseline gap-2 rounded-lg border-l-2 py-2 pl-4" style={{ borderColor: K.sel }}>
                <span className="rb-mono" style={{ color: K.sel, fontSize: "0.85rem" }} title={c.path}>:{c.a}{c.b !== c.a ? "–" + c.b : ""}</span>
                <span className="rb-prose min-w-0 flex-1" style={{ fontSize: "1.15rem" }}>{c.note}</span>
                <button className="rb-btn" style={{ fontSize: "0.8rem" }} title="edit"
                  onClick={() => {
                    const bi = Math.max(0, (step.codes || []).findIndex((cb) => cb.path === c.path && c.a >= cb.start && c.a < cb.start + cb.lines.length));
                    setN(step.id, { comments: n(step.id).comments.filter((_, j) => j !== i) });
                    setPend({ a: c.a, b: c.b, text: c.text, draft: c.note, top: 0, bi, path: c.path });
                  }}>edit</button>
                <button className="rb-btn" style={{ fontSize: "0.8rem" }} title="delete"
                  onClick={() => setN(step.id, { comments: n(step.id).comments.filter((_, j) => j !== i) })}>✕</button>
              </div>
            ))}

            {/* comprehension checks */}
            {step.checks.length > 0 && (
              <div className="mb-4 mt-8">
                <div className="mb-2 text-[0.9rem] font-bold" style={{ color: K.mut, letterSpacing: "0.02em" }}>Check yourself</div>
                {step.checks.map((c, i) => {
                  const open = n(step.id).revealed[i];
                  return (
                    <div key={i} className="mb-3 rounded-lg border px-4 py-3" style={{ borderColor: K.line }}>
                      <div className="flex items-start gap-2">
                        <button className="rb-btn text-[0.69rem]" style={{ border: 0, padding: "2px 4px" }}
                          title={open ? "hide the answer" : "reveal the answer"}
                          onClick={() => setN(step.id, { revealed: { ...n(step.id).revealed, [i]: !open } })}>
                          {open ? "▾" : "▸"}
                        </button>
                        <div className="min-w-0 flex-1">
                          <div className="rb-prose" style={{ cursor: "pointer", fontSize: "1.15rem" }}
                            onClick={() => setN(step.id, { revealed: { ...n(step.id).revealed, [i]: !open } })}>{c.q}</div>
                          {open && <div className="rb-reveal"><div className="rb-prose mt-1" style={{ color: K.mut, fontSize: "1.15rem" }}>{c.a}</div></div>}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}

            {/* the step's verdict */}
            {step.kind === "code" && (
              <div className="mt-8 rounded-lg border p-5" style={{ borderColor: K.line, background: K.panel }}>
                {/* Free-form and multiline (round 4: "let me add whatever text I
                    need"). The forcing function survives as the floor, not a cap:
                    write something before moving on, at any length. Keyed by step
                    so the grown height never leaks onto the next step's note. */}
                <textarea key={step.id} className="rb-in rb-prose mb-2" rows={3} style={{ fontSize: "1.15rem" }}
                  placeholder="What you take from this step, before moving on. Any length. If nothing will come, the step is not done with you."
                  value={n(step.id).sentence || ""}
                  ref={(el) => { if (el && el.value) { el.style.height = "auto"; el.style.height = el.scrollHeight + 2 + "px"; } }}
                  onChange={(e) => {
                    setN(step.id, { sentence: e.target.value });
                    e.target.style.height = "auto";
                    e.target.style.height = e.target.scrollHeight + 2 + "px";
                  }} />
                <div className="flex items-center gap-2">
                  {/* The verdict moment (round 7): each button rests in a tint
                      of its own meaning, fills on selection, and the next
                      button lights up once a verdict exists - no auto-advance,
                      the reader leaves when they leave. */}
                  <button className="rb-btn" style={{ fontSize: "1rem", padding: "8px 18px", ...(n(step.id).state === "understood"
                      ? { borderColor: K.ok, color: K.ok, background: K.ok + "14" }
                      : { borderColor: K.ok + "44", color: K.ink }) }}
                    onClick={() => setN(step.id, { state: "understood" })}>Understood</button>
                  <button className="rb-btn" style={{ fontSize: "1rem", padding: "8px 18px", ...(n(step.id).state === "unclear"
                      ? { borderColor: K.unclear, color: K.unclear, background: K.unclear + "16" }
                      : { borderColor: K.unclear + "44", color: K.ink }) }}
                    onClick={() => setN(step.id, { state: "unclear" })}>Unclear – needs the agent</button>
                  <div className="ml-auto flex gap-2">
                    <button className="rb-btn" disabled={at === 0} onClick={() => setAt(at - 1)}>← back</button>
                    <button className="rb-btn" disabled={at === STEPS.length - 1}
                      style={n(step.id).state ? { borderColor: K.look, color: K.look } : {}}
                      onClick={() => setAt(at + 1)}>next →</button>
                  </div>
                </div>
              </div>
            )}
            {step.kind !== "code" && (
              <div className="mt-6 flex gap-2">
                <button className="rb-btn" disabled={at === 0} onClick={() => setAt(at - 1)}>← back</button>
                <button className="rb-btn" disabled={at === STEPS.length - 1} onClick={() => setAt(at + 1)}>next →</button>
              </div>
            )}
            <div className="h-8" />
          </div>
        </div>
      </div>

      {/* the whole keyboard, visible (round 7): keys nobody can discover are
          mouse-only in practice - the adversarial pass's finding, closed. */}
      <div className="flex flex-none items-center border-t px-5 text-[0.85rem]" data-agentbox-find-exclude
        style={{ height: "2.5rem", borderColor: K.line, color: K.mut }}>
        ← → step · u understood · x unclear · Enter next unread · select code to comment
      </div>

      {/* submission */}
      {modal && (
        <div className="fixed inset-0 flex items-center justify-center" style={{ background: "#000a" }}>
          <div className="flex max-h-[80%] w-[760px] flex-col rounded-xl border p-6" style={{ background: K.panel, borderColor: K.line }}>
            {emitted ? (
              /* The receipt: "the handback has no receipt" (adversarial pass),
                 answered at the submission moment itself. */
              <div className="flex flex-col items-center gap-2 py-10" data-agentbox-find-exclude>
                <div style={{ color: K.ok, fontSize: "2rem" }}>✓</div>
                <div className="text-[1.125rem] font-bold">Delivered to the agent · {emitted}</div>
                <div className="text-[0.9rem]" style={{ color: K.mut }}>
                  {doneCount} understood · {unclear.length} unclear · {Object.values(notes).reduce((t, d) => t + (d.comments || []).length, 0)} comments
                </div>
                <button className="rb-btn mt-3" onClick={() => { setModal(false); setEmitted(null); }}>Close</button>
              </div>
            ) : (
              <>
                <div className="mb-2 flex items-baseline">
                  <div className="text-[1.125rem] font-bold">What goes back to the agent, in one turn</div>
                  <div className="ml-auto text-[0.9rem]" style={{ color: K.mut }}>
                    {unclear.length} unclear · {COUNTED.filter((id) => !n(id).state).length} not reviewed
                  </div>
                </div>
                <textarea readOnly className="rb-in rb-mono flex-1" style={{ minHeight: 260, fontSize: "0.95rem" }} value={payload} />
                <div className="mt-3 flex items-center gap-2">
                  <button className="rb-btn" style={{ borderColor: K.look, color: K.look }}
                    onClick={() => { window.agentbox.emit("review-submitted", { payload }); setEmitted(new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })); }}>
                    Submit to the agent
                  </button>
                  <button className="rb-btn" style={{ fontSize: "0.85rem" }} title="an export nicety, not the channel"
                    onClick={(e) => { const ta = e.target.closest("div").previousSibling; ta.select(); document.execCommand("copy"); }}>
                    copy as markdown
                  </button>
                  <button className="rb-btn ml-auto" onClick={() => setModal(false)}>Keep reviewing</button>
                </div>
                <div className="mt-2 text-[0.8rem]" style={{ color: K.mut }}>
                  Submission talks to the agent directly - agentbox owns delivery. The clipboard is an export nicety, kept for pasting a review somewhere else.
                  Mock: state lives in this window only; the real board persists it and never loses a note to an amended step.
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
