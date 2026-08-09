import { Call, Events, Window } from "@wailsio/runtime";
import { note, why } from "./trouble.svelte.js";

// One place that knows how the frontend talks to the daemon. Everything the
// UI can do to an item goes through Bridge.* on the Go side; everything the
// daemon wants to say arrives as a Wails event.

// Wails keys bound methods by full import path (bindings.go: FQN).
const PKG = "github.com/borismilner/agentbox/internal/webui";
const svc = (m) => `${PKG}.Bridge.${m}`;

// answering wraps the calls a human's keystroke makes, and it is the whole of
// U-01's fix: one wrapper instead of a .catch at 26 call sites, because 26 call
// sites is 26 chances to forget one and the tempting version - .catch(() => {})
// per call - turns a silent failure into a silent failure that passes review.
//
// Two things can go wrong and both end up in the same place. The call can reject
// (a torn-down window, a serialization failure, a method that no longer binds),
// and the daemon can answer with a sentence saying it refused (U-02). Either
// way trouble.text gets it and whichever surface is on screen shows it. A call
// that lands clears the line, so nothing has to remember to.
//
// The wrapper never rethrows: an unhandled rejection reaching the window is
// exactly the state this exists to end. It returns the sentence instead, so a
// caller that wants to branch (the inbox does) still can.
const answering =
  (call) =>
  async (...args) => {
    try {
      const refused = await call(...args);
      note(typeof refused === "string" ? refused : "");
      return refused;
    } catch (e) {
      const text = why(e);
      note(text);
      return text;
    }
  };

export const bridge = {
  answer: answering((id, label) => Call.ByName(svc("Answer"), id, label)),
  reply: answering((id, text) => Call.ByName(svc("Reply"), id, text)),
  answerForm: answering((id, values) => Call.ByName(svc("AnswerForm"), id, values)),
  confirm: answering((id, yes) => Call.ByName(svc("Confirm"), id, yes)),
  secret: answering((id, value) => Call.ByName(svc("Secret"), id, value)),
  review: answering((id, approved, comment) => Call.ByName(svc("Review"), id, approved, comment)),
  veto: answering((id) => Call.ByName(svc("Veto"), id)),
  defer: answering((id) => Call.ByName(svc("Defer"), id)),
  dismiss: answering((id) => Call.ByName(svc("Dismiss"), id)),
  undo: answering((id) => Call.ByName(svc("Undo"), id)),
  runAction: answering((id, index) => Call.ByName(svc("RunAction"), id, index)),
  // FR30: lift one row out of a stack card and make it a card again. The stack
  // id travels so the daemon can refuse an item the human is not looking at.
  openStacked: answering((stackId, itemId) => Call.ByName(svc("OpenStacked"), stackId, itemId)),
  copy: (id) => Call.ByName(svc("Copy"), id),

  // session surface
  sessions: () => Call.ByName(svc("Sessions")),
  newSession: (cwd, mode) => Call.ByName(svc("NewSession"), cwd, mode),
  selectSession: (id) => Call.ByName(svc("SelectSession"), id),
  sendPrompt: (id, prompt) => Call.ByName(svc("SendPrompt"), id, prompt),
  stopSession: (id) => Call.ByName(svc("StopSession"), id),
  // Ends the session and drops its row. Ask first - it kills a running agent and
  // an unsaved conversation goes with it.
  closeSession: (id) => Call.ByName(svc("CloseSession"), id),
  // Plan <-> full. It replaces the child (a spawn-time flag) but keeps the id, the
  // conversation and the child's memory of it, so the surface does nothing special.
  setSessionMode: (id, mode) => Call.ByName(svc("SetSessionMode"), id, mode),
  // A label the human puts on a session so it can be found tomorrow; empty gives
  // it back to the name Claude chose from its own first words.
  renameSession: (id, name) => Call.ByName(svc("RenameSession"), id, name),
  // Conversations on disk, newest first, and putting one back on screen.
  savedSessions: () => Call.ByName(svc("SavedSessions")),
  reopenSession: (path) => Call.ByName(svc("ReopenSession"), path),
  // The one setting worth reaching without opening the settings surface; it is
  // written to the config file, so it survives the window.
  bumpFontSize: (delta) => Call.ByName(svc("BumpFontSize"), delta),
  // The inline ask panel (FR49) sends the keystroke, not a decision - the same
  // arrangement triage has, and the same table behind it.
  askKey: (id, key) => Call.ByName(svc("AskKey"), id, key),
  minimiseApp: () => Call.ByName(svc("MinimiseApp")),
  hideApp: () => Call.ByName(svc("HideApp")),
  // the drop-down panel (M10); Esc and the ⌃ button roll it back up
  hidePanel: () => Call.ByName(svc("HidePanel")),

  // inbox + history surfaces. triage sends the keystroke, not a decision: which
  // key answers what is decided in Go so the card and the inbox cannot drift.
  inbox: () => Call.ByName(svc("Inbox")),
  // One row read back in full (FR73). Asked for when a row opens, never shipped
  // with the snapshot: the rows carry a snippet precisely so a hundred rendered
  // bodies do not ride every repaint. `found: false` means the item has aged out.
  itemDetail: (id) => Call.ByName(svc("ItemDetail"), id),
  // Wrapped like the rest of the answer path: a row whose item is gone answers
  // with a sentence rather than opening a card (U-02), and that has to be
  // something other than a click that appears to work.
  promote: answering((id) => Call.ByName(svc("Promote"), id)),
  triage: (id, key) => Call.ByName(svc("Triage"), id, key),
  copyItem: (id) => Call.ByName(svc("CopyItem"), id),
  stats: (window) => Call.ByName(svc("Stats"), window),

  // rendered markdown: a link opens in the desktop browser instead of navigating
  // the window away, and a code block's copy button falls back to the daemon's
  // clipboard when the webview refuses the async one.
  openURL: (url) => Call.ByName(svc("OpenURL"), url),
  copyText: (text) => Call.ByName(svc("CopyText"), text),

  // The one way out of an artifact's sandbox (M10): what the human did inside it,
  // on its way to whichever agent is waiting. The payload crosses as JSON text
  // because it is the artifact's own vocabulary - Go carries it, it does not read
  // it (internal/webui/artifact.go).
  artifactEvent: (artifactId, name, dataJSON) =>
    Call.ByName(svc("ArtifactEvent"), artifactId, name, dataJSON),

  // review board (FR58). The whole review is pulled on mount; every
  // annotation is written through as it happens, so a daemon restart or a
  // closed window loses nothing the human did.
  board: () => Call.ByName(svc("Board")),
  boardVerdict: (id, stepId, verdict) => Call.ByName(svc("BoardVerdict"), id, stepId, verdict),
  boardNote: (id, stepId, note) => Call.ByName(svc("BoardNote"), id, stepId, note),
  boardReveal: (id, stepId, revealed) => Call.ByName(svc("BoardReveal"), id, stepId, revealed),
  boardPos: (id, pos) => Call.ByName(svc("BoardPos"), id, pos),
  boardCommentAdd: (id, stepId, path, side, from, to, exact, body) =>
    Call.ByName(svc("BoardCommentAdd"), id, stepId, path, side, from, to, exact, body),
  boardCommentEdit: (id, commentId, body) => Call.ByName(svc("BoardCommentEdit"), id, commentId, body),
  boardCommentDelete: (id, commentId) => Call.ByName(svc("BoardCommentDelete"), id, commentId),
  // FR65: raise the reader's editor on a cited block. The review id and the
  // repo-relative path, never a file path - Go owns the root and refuses
  // anything outside it.
  boardOpenInEditor: (id, path, line) => Call.ByName(svc("BoardOpenInEditor"), id, path, line),
  boardSubmit: (id) => Call.ByName(svc("BoardSubmit"), id),
  // the library surface (FR70): what is stored, put one on the board, remove one
  library: () => Call.ByName(svc("Library")),
  libraryOpen: (id) => Call.ByName(svc("LibraryOpen"), id),
  libraryDelete: (id) => Call.ByName(svc("LibraryDelete"), id),
  showLibrary: () => Call.ByName(svc("ShowLibrary")),

  // Assignments (M12/FR82): the work agentbox runs on its own. Every write goes
  // through the daemon's own save, so the editor gets the same refusals and the
  // same warnings an agent does - a field the editor did not send is a field it
  // does not want changed. save/params/enable/delete/run return "" on success or
  // a sentence to show.
  assignments: () => Call.ByName(svc("Assignments")),
  assignment: (id) => Call.ByName(svc("Assignment"), id),
  saveAssignment: (values) => Call.ByName(svc("SaveAssignment"), values),
  setAssignmentParams: (id, paramsJSON) => Call.ByName(svc("SetAssignmentParams"), id, paramsJSON),
  enableAssignment: (id, on) => Call.ByName(svc("EnableAssignment"), id, on),
  deleteAssignment: (id) => Call.ByName(svc("DeleteAssignment"), id),
  runAssignment: (id) => Call.ByName(svc("RunAssignment"), id),

  // Read-aloud (FR66, reshaped by FR72). The human asking to hear ONE region of a
  // screen: action is start (with the region's name and its text), stop, or state.
  // Region is the surface's own name for the part being read, handed back
  // unchanged, so a page with a control per region knows which to paint. Every
  // call answers with the reader's state, so the control paints from the daemon
  // rather than from a guess about what the voice is doing.
  aloud: (action, region, text) =>
    Call.ByName(svc("Aloud"), action, region ?? "", text ?? ""),

  // The Agents surface (FR83). agents() is the roster to paint on mount, so a
  // window opened between two pushes does not start blank; the roster then
  // arrives on agentbox:agents whenever it changes. breakLock answers "" or a
  // sentence to show, and it reassigns the lock without stopping its holder.
  agents: () => Call.ByName(svc("Agents")),
  // One opened row's history, pulled per row rather than carried in every push -
  // the roster goes out once a second while anything moves.
  agentDetail: (key) => Call.ByName(svc("AgentDetail"), key),
  breakLock: (name) => Call.ByName(svc("BreakLock"), name),

  // The hands-off strip (FR74). control() is the run to paint on mount, so a
  // window that opens mid-run does not start blank; the two answers go back the
  // same way, which is what makes the strip the place the decision happens.
  control: () => Call.ByName(svc("Control")),
  controlDeny: (id) => Call.ByName(svc("ControlDeny"), id ?? ""),
  controlAllow: (id) => Call.ByName(svc("ControlAllow"), id ?? ""),
  // FR94. No agent-facing twin of these two on purpose: pausing and resuming
  // are the human's, and a tool that could resume would make the pause advice.
  controlPause: () => Call.ByName(svc("ControlPause")),
  controlResume: () => Call.ByName(svc("ControlResume")),

  // viewer + progress. The surface asks for the document; it never names a path,
  // so it cannot read anything the daemon was not asked to show.
  document: () => Call.ByName(svc("Document")),
  progress: () => Call.ByName(svc("Progress")),
  fitProgress: (height) => Call.ByName(svc("FitProgress"), height),

  // settings surface
  settings: () => Call.ByName(svc("Settings")),
  previewTheme: (values) => Call.ByName(svc("PreviewTheme"), values),
  saveSettings: (values) => Call.ByName(svc("SaveSettings"), values),

  // surfaces
  theme: () => Call.ByName(svc("Theme")),
  ready: (surface) => Call.ByName(svc("Ready"), surface),
  fit: (height) => Call.ByName(svc("Fit"), height),
  closeSelf: () => Window.Close(),
  // Window buttons for the frameless surfaces, which have no title bar of
  // their own. These act on the calling window (the runtime's Window is this
  // window), so one line serves every surface that draws its own header.
  minimiseSelf: () => Window.Minimise(),
  toggleMaximiseSelf: () => Window.ToggleMaximise(),
  isMaximisedSelf: () => Window.IsMaximised(),
};

export function on(event, fn) {
  return Events.On(event, (e) => fn(e.data));
}

// The surface this window is: card, toast, app or viewer.
export function surfaceName() {
  return new URLSearchParams(location.search).get("surface") || "app";
}
