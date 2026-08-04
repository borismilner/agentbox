import { Call, Events, Window } from "@wailsio/runtime";

// One place that knows how the frontend talks to the daemon. Everything the
// UI can do to an item goes through Bridge.* on the Go side; everything the
// daemon wants to say arrives as a Wails event.

// Wails keys bound methods by full import path (bindings.go: FQN).
const PKG = "github.com/borismilner/agentbox/internal/webui";
const svc = (m) => `${PKG}.Bridge.${m}`;

export const bridge = {
  answer: (id, label) => Call.ByName(svc("Answer"), id, label),
  reply: (id, text) => Call.ByName(svc("Reply"), id, text),
  answerForm: (id, values) => Call.ByName(svc("AnswerForm"), id, values),
  confirm: (id, yes) => Call.ByName(svc("Confirm"), id, yes),
  secret: (id, value) => Call.ByName(svc("Secret"), id, value),
  review: (id, approved, comment) => Call.ByName(svc("Review"), id, approved, comment),
  veto: (id) => Call.ByName(svc("Veto"), id),
  defer: (id) => Call.ByName(svc("Defer"), id),
  dismiss: (id) => Call.ByName(svc("Dismiss"), id),
  undo: (id) => Call.ByName(svc("Undo"), id),
  runAction: (id, index) => Call.ByName(svc("RunAction"), id, index),
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
  promote: (id) => Call.ByName(svc("Promote"), id),
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
  breakLock: (name) => Call.ByName(svc("BreakLock"), name),

  // The hands-off strip (FR74). control() is the run to paint on mount, so a
  // window that opens mid-run does not start blank; the two answers go back the
  // same way, which is what makes the strip the place the decision happens.
  control: () => Call.ByName(svc("Control")),
  controlDeny: (id) => Call.ByName(svc("ControlDeny"), id ?? ""),
  controlAllow: (id) => Call.ByName(svc("ControlAllow"), id ?? ""),

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
