// What the two surfaces that can end a session agree on (U-09).
//
// Ending a session kills the child and an unsaved conversation goes with it
// (bridge.js:34-36). Two surfaces offer it - the app window's session list and
// the drop-down panel - and they used to disagree about when to ask. The app
// window always asked. The panel asked only when the agent was mid-turn, so an
// idle session was gone on the first click, with no confirmation and no undo.
//
// The stricter half won, for a reason worth keeping written down: "idle" means
// the agent is not mid-turn, it does not mean the conversation is worthless, and
// the app window's own wording says exactly that about exactly the same session.
// The panel is also the surface reached by a hotkey while doing something else,
// which is the worst context in which to make a destructive guard weaker.
//
// What is shared here is the DECISION and the WORDING, not the widget. The two
// surfaces have different room and keep their own interactions: an app-window row
// can hold a sentence and two buttons, a 224px panel chip cannot, so the panel
// keeps its arm-and-expire ✕ and reaches the sentence through its tooltip. One
// place to change what the question says, two places to ask it.

// endQuestion: the sentence a human reads before a child dies. It varies by state
// because the two losses are different - work in flight, or a conversation - and
// naming the right one is the whole value of asking.
export function endQuestion(session) {
  return session?.state === "working"
    ? "Claude is working here. End it anyway?"
    : "End this session? An unsaved conversation goes with it.";
}

// ARM_MS: how long the panel's ✕ stays armed before it forgets. A guard that
// never disarms is a guard you eventually click through by muscle memory; three
// seconds is long enough to mean it and short enough that a stray first click
// does not leave a loaded button behind.
export const ARM_MS = 3000;
