// What a surface says when an answer did not land (U-01).
//
// The card makes 26 calls to the daemon and used to await none of them, catch
// none of them and handle no failure from any of them. Call.ByName rejects on a
// torn-down window, a serialization failure or a method that no longer binds,
// and the daemon itself can now refuse in words (U-02) - so there were two ways
// for a keystroke to do nothing and the card showed neither. The human pressed
// the key, the card did not move, and there was no difference on screen between
// "the daemon is thinking" and "that went nowhere".
//
// One place holds the last such failure, because the alternative is a check at
// every call site and 26 chances to forget one. bridge.js writes it; the answer
// surfaces read it.

let current = $state("");

// The state is exported through a getter rather than as a value: a plain export
// would be read once at import and never update.
export const trouble = {
  get text() {
    return current;
  },
};

// note is what bridge.js calls when a call refuses or rejects. An empty message
// means it landed, which is how a working keystroke clears the last failure -
// nothing else has to remember to.
export function note(message) {
  current = message ?? "";
}

// forget clears the line without a call behind it: a fresh item on the card, or
// the human having read it. A failure from the last question is not news about
// this one.
export function forget() {
  current = "";
}

// why turns whatever a rejected Call.ByName threw into something worth showing.
// Wails hands back an Error, a string or an object depending on where it broke,
// and "[object Object]" on a card is worse than no line at all.
export function why(err) {
  const text = err instanceof Error ? err.message : typeof err === "string" ? err : (err?.message ?? "");
  const trimmed = String(text).trim();
  return trimmed ? "agentbox did not take that: " + trimmed : "agentbox did not take that. Nothing was sent.";
}
