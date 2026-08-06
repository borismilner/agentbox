package webui

// The review board window (FR58): a stored walkthrough rendered as a native
// surface. The window follows the viewer's recipe - frameless, titled
// through X11 after the map, maximized on open because a review wants the
// whole screen (owner's rule from the mock rounds) - and the surface talks
// back through the Board* keyhole verbs below, each of which validates
// before it writes: Go is the side that must not be talked into anything.

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/borismilner/agentbox/internal/editor"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/walkthrough"
)

// BoardStore feeds the board and takes its annotations. Implemented by the
// daemon (structurally, like Source) and set after construction.
type BoardStore interface {
	BoardData(id string) (store.Walkthrough, []store.Mark, []store.Comment, error)
	BoardVerdict(id, stepID, verdict string) error
	BoardNote(id, stepID, note string) error
	BoardReveal(id, stepID string, revealed []int) error
	BoardPos(id string, pos int) error
	BoardCommentAdd(id, stepID, path, side string, from, to int, exact, body string) (string, error)
	BoardCommentEdit(id, commentID, body string) error
	BoardCommentDelete(id, commentID string) error
	BoardSubmit(id string) (delivered bool, atMS int64, err error)
	BoardLibrary() ([]proto.WalkthroughSummary, error)
	BoardDelete(id string) (bool, error)
}

// SetBoardStore wires the daemon in after it exists (construction order -
// the daemon is handed this UI as its Presenter first).
func (u *UI) SetBoardStore(bs BoardStore) {
	u.mu.Lock()
	u.boardStore = bs
	u.mu.Unlock()
}

// Voice reads one region of a screen out loud on request, and stops. Its own
// keyhole rather than a method on BoardStore, because reading is not an
// annotation and every surface with text on it wants it.
type Voice interface {
	Aloud(action, region, text string) proto.AloudResult
}

// SetVoice wires the daemon's reader. Nil is a valid state: a demo board or a
// build with no daemon behind it simply cannot read, and the surface hides the
// control rather than offering one that does nothing.
func (u *UI) SetVoice(v Voice) {
	u.mu.Lock()
	u.voice = v
	u.mu.Unlock()
}

func (u *UI) voiceSrc() Voice {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.voice
}

// Aloud is the surface asking for a region to be read, or stopped, or reporting
// which one is playing. It returns the state that resulted so the control paints
// from the answer, and a build with no voice reports nothing playing rather than
// an error - there is nothing for the human to fix.
func (b *Bridge) Aloud(action, region, text string) proto.AloudResult {
	v := b.ui.voiceSrc()
	if v == nil {
		return proto.AloudResult{}
	}
	return v.Aloud(action, region, text)
}

func (u *UI) boardSrc() BoardStore {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.boardStore
}

type boardWin struct {
	ui  *UI
	mu  sync.Mutex
	id  string // the walkthrough on display
	win *application.WebviewWindow
}

func newBoard(u *UI) *boardWin { return &boardWin{ui: u} }

// ShowBoard opens the board for a walkthrough, or retargets and raises the
// window that is already up - one board, like one viewer.
func (u *UI) ShowBoard(id string) {
	b := u.board
	b.mu.Lock()
	b.id = id
	w := b.win
	b.mu.Unlock()
	if w == nil {
		b.openWindow()
		return
	}
	b.push()
	b.retitle()
	u.onMain("board.raise", func() {
		w.Show()
		w.Focus()
	})
}

// target is the walkthrough currently on the board, "" when the window is
// down or empty.
func (b *boardWin) target() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.id
}

// close takes the window down and forgets its target. Used when the review
// on it is deleted: a board showing something that no longer exists would
// take annotations nothing could store.
func (b *boardWin) close() {
	b.mu.Lock()
	w := b.win
	b.id = ""
	b.mu.Unlock()
	if w == nil {
		return
	}
	b.ui.onMain("board.close", func() { w.Close() })
}

func (u *UI) boardGeom() (w, h int) {
	c := u.conf()
	return c.Window.BoardWidth, c.Window.BoardHeight
}

func (b *boardWin) openWindow() {
	bw, bh := b.ui.boardGeom()
	b.ui.onMain("board", func() {
		w := b.ui.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:      "agentbox-board",
			Title:     b.title(),
			Width:     bw,
			Height:    bh,
			MinWidth:  700,
			MinHeight: 500,
			// Frameless like the viewer: the surface's own header carries the
			// review's name, the progress pips and the submit control.
			Frameless:        true,
			URL:              "/?surface=board",
			BackgroundType:   application.BackgroundTypeSolid,
			BackgroundColour: rgba(b.ui.themeGround()),
		})

		b.mu.Lock()
		b.win = w
		b.mu.Unlock()

		w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			b.mu.Lock()
			b.win = nil
			b.mu.Unlock()
		})

		w.Show()
		w.Focus()
		b.ui.placeOn(w, bw, bh)
		// A review wants the whole screen (owner's rule, from the mock
		// rounds); the configured size is what un-maximizing restores.
		w.Maximise()
		b.retitle()
	})
}

// resizeToConfig applies a [window] reload to the un-maximized restore size.
func (b *boardWin) resizeToConfig() {
	b.mu.Lock()
	w := b.win
	b.mu.Unlock()
	if w == nil {
		return
	}
	bw, bh := b.ui.boardGeom()
	b.ui.onMain("board.resize", func() {
		if cw, ch := w.Size(); cw != bw || ch != bh {
			w.SetSize(bw, bh)
		}
	})
}

func (b *boardWin) title() string {
	src := b.ui.boardSrc()
	b.mu.Lock()
	id := b.id
	b.mu.Unlock()
	if src == nil || id == "" {
		return "agentbox · review board"
	}
	w, _, _, err := src.BoardData(id)
	if err != nil {
		return "agentbox · review board"
	}
	return "agentbox · review board · " + w.Title
}

func (b *boardWin) retitle() {
	b.mu.Lock()
	w, x := b.win, b.ui.x
	b.mu.Unlock()
	if w == nil {
		return
	}
	title := b.title()
	application.InvokeSync(func() {
		w.SetTitle(title)
		if x != nil {
			if xid := xidOf(w.NativeWindow()); xid != 0 {
				x.setName(xid, title)
			}
		}
	})
}

// snapshot renders the current walkthrough for the wire. Nil when no
// walkthrough is targeted or the store is not wired yet.
func (b *boardWin) snapshot() (*wireBoard, error) {
	src := b.ui.boardSrc()
	b.mu.Lock()
	id := b.id
	b.mu.Unlock()
	if src == nil || id == "" {
		return nil, errors.New("no walkthrough on the board")
	}
	w, marks, comments, err := src.BoardData(id)
	if err != nil {
		return nil, err
	}
	log := b.ui.log
	return boardSnapshot(w, marks, comments, time.Now().UnixMilli(),
		func(step, path, reason string) {
			log.Warn("board.render_miss", "component", "webui", "wt_id", w.ID,
				"step", step, "path", path, "reason", reason)
		})
}

// push sends a fresh snapshot to the surface (an open window re-rendering
// under a ShowBoard retarget or a future amendment).
func (b *boardWin) push() {
	wb, err := b.snapshot()
	if err != nil {
		b.ui.log.Warn("board.push_failed", "component", "webui", "err", err.Error())
		return
	}
	b.ui.emit("agentbox:board", wb)
}

// --- the keyhole verbs -------------------------------------------------

// Caps mirror the annotation columns; the surface enforces nothing the
// store would not survive without.
const (
	boardNoteMax  = 16 << 10
	boardExactMax = 400
)

var boardVerdicts = map[string]bool{"": true, "understood": true, "unclear": true, "seen": true}

// Board is the surface's pull-on-mount: the whole review, rendered.
func (b *Bridge) Board() (*wireBoard, error) {
	return b.ui.board.snapshot()
}

func (b *Bridge) boardWrite(what string, err error) error {
	if err != nil {
		b.ui.log.Warn("board.write_failed", "component", "webui", "what", what, "err", err.Error())
	}
	return err
}

func (b *Bridge) BoardVerdict(id, stepID, verdict string) error {
	src := b.ui.boardSrc()
	if src == nil {
		return errors.New("board store not wired")
	}
	if !boardVerdicts[verdict] {
		return fmt.Errorf("verdict %q is not one of understood, unclear, seen", verdict)
	}
	return b.boardWrite("verdict", src.BoardVerdict(id, stepID, verdict))
}

func (b *Bridge) BoardNote(id, stepID, note string) error {
	src := b.ui.boardSrc()
	if src == nil {
		return errors.New("board store not wired")
	}
	if len(note) > boardNoteMax {
		return fmt.Errorf("note is %d bytes; the cap is %d", len(note), boardNoteMax)
	}
	return b.boardWrite("note", src.BoardNote(id, stepID, note))
}

func (b *Bridge) BoardReveal(id, stepID string, revealed []int) error {
	src := b.ui.boardSrc()
	if src == nil {
		return errors.New("board store not wired")
	}
	return b.boardWrite("reveal", src.BoardReveal(id, stepID, revealed))
}

func (b *Bridge) BoardPos(id string, pos int) error {
	src := b.ui.boardSrc()
	if src == nil {
		return errors.New("board store not wired")
	}
	if pos < 0 {
		pos = 0
	}
	return b.boardWrite("pos", src.BoardPos(id, pos))
}

func (b *Bridge) BoardCommentAdd(id, stepID, path, side string, from, to int, exact, body string) (string, error) {
	src := b.ui.boardSrc()
	if src == nil {
		return "", errors.New("board store not wired")
	}
	if strings.TrimSpace(body) == "" {
		return "", errors.New("a comment needs words")
	}
	if len(body) > boardNoteMax {
		return "", fmt.Errorf("comment is %d bytes; the cap is %d", len(body), boardNoteMax)
	}
	if side != "" && side != "new" && side != "old" {
		return "", fmt.Errorf("side %q is not one of new, old", side)
	}
	if from < 0 || to < from {
		return "", fmt.Errorf("anchor [%d,%d] is not a range", from, to)
	}
	if len(exact) > boardExactMax {
		exact = exact[:boardExactMax]
	}
	cid, err := src.BoardCommentAdd(id, stepID, path, side, from, to, exact, body)
	return cid, b.boardWrite("comment_add", err)
}

func (b *Bridge) BoardCommentEdit(id, commentID, body string) error {
	src := b.ui.boardSrc()
	if src == nil {
		return errors.New("board store not wired")
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("a comment needs words; delete it instead")
	}
	return b.boardWrite("comment_edit", src.BoardCommentEdit(id, commentID, body))
}

// BoardOpenInEditor raises the human's editor on a cited block's first line
// (FR65). The surface names a REVIEW and a repo-relative path, never a file: the
// root comes from the stored walkthrough on this side, so a surface cannot ask
// for a path outside the repository it is reviewing, and rel is checked to be
// inside that root after cleaning. This is the second place a surface can name
// something outside agentbox (OpenURL is the first) and it launches a program, so
// it gets the same treatment.
//
// The error is what the reader is shown, so each one says what to do: no editor
// found says which key sets one, and a fallback that loses the line is reported
// as success with a warning in the log rather than as a failure - the file did
// open.
func (b *Bridge) BoardOpenInEditor(id, rel string, line int) error {
	src := b.ui.boardSrc()
	if src == nil {
		return errors.New("board store not wired")
	}
	w, _, _, err := src.BoardData(id)
	if err != nil {
		return err
	}
	root := strings.TrimRight(w.RepoRoot, "/")
	if root == "" || !filepath.IsAbs(root) {
		return errors.New("this review has no repository root, so there is no file to open")
	}
	abs, err := underRoot(root, rel)
	if err != nil {
		b.ui.log.Warn("board.open_editor_rejected", "component", "webui", "path", rel, "err", err.Error())
		return err
	}
	argv, source, err := editor.Command(b.ui.conf().Editor.Command, editor.Target{
		Dir: root, File: abs, Line: line, Col: 1,
	})
	if err != nil {
		b.ui.log.Warn("board.open_editor_failed", "component", "webui", "err", err.Error())
		return err
	}
	if _, err := editor.Start(argv); err != nil {
		b.ui.log.Warn("board.open_editor_failed", "component", "webui", "argv", strings.Join(argv, " "), "err", err.Error())
		return fmt.Errorf("could not start %s: %w", argv[0], err)
	}
	b.ui.log.Info("board.open_editor", "component", "webui", "source", source, "file", abs, "line", line)
	return nil
}

// underRoot resolves a repo-relative citation against the review's root and
// refuses anything that leaves it. Symlinks are deliberately not followed: the
// path came out of a diff, the file may not exist on disk at all (a review of a
// deleted file, a checkout on another branch), and letting the editor report
// that is better than refusing to try.
func underRoot(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", errors.New("this block has no file to open")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%q is an absolute path; a citation is relative to the repository", rel)
	}
	abs := filepath.Clean(filepath.Join(root, rel))
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is outside the repository being reviewed", rel)
	}
	return abs, nil
}

func (b *Bridge) BoardCommentDelete(id, commentID string) error {
	src := b.ui.boardSrc()
	if src == nil {
		return errors.New("board store not wired")
	}
	return b.boardWrite("comment_delete", src.BoardCommentDelete(id, commentID))
}

// wireReceipt answers a submit. Delivered means a parked agent took the
// review in the same instant; otherwise it sits in the store as submitted.
// Gate names the step that refused the submission (an unclear verdict with
// no words) so the surface jumps there instead of wearing a raw error.
type wireReceipt struct {
	Delivered bool   `json:"delivered"`
	AtMS      int64  `json:"atMs"`
	Gate      string `json:"gate,omitempty"`
	GateMsg   string `json:"gateMsg,omitempty"`
}

func (b *Bridge) BoardSubmit(id string) (*wireReceipt, error) {
	src := b.ui.boardSrc()
	if src == nil {
		return nil, errors.New("board store not wired")
	}
	delivered, at, err := src.BoardSubmit(id)
	if gate, ok := errors.AsType[*walkthrough.GateError](err); ok {
		return &wireReceipt{Gate: gate.StepID, GateMsg: gate.Error()}, nil
	}
	if err != nil {
		return nil, b.boardWrite("submit", err)
	}
	return &wireReceipt{Delivered: delivered, AtMS: at}, nil
}

// --- the library (FR70) ------------------------------------------------
//
// Reviews were durable from the day the store landed, but the only door to
// them was the CLI: list, copy an id, open. A human who walked one, closed
// the window and came back the next day had no way to find it without a
// terminal - which reads as "it was not saved" whatever the database holds.
// These three verbs are what the library surface in the app window works.

// wireLibraryRow is one stored review as the library lists it: enough to
// recognise it by, plus where its reader got to.
type wireLibraryRow struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Repo       string `json:"repo"` // ~-abbreviated, like the board header
	Pinned     string `json:"pinned"`
	State      string `json:"state"`
	Steps      int    `json:"steps"`
	Understood int    `json:"understood"`
	Unclear    int    `json:"unclear"`
	Comments   int    `json:"comments"`
	UpdatedMS  int64  `json:"updatedMs"`
	OnBoard    bool   `json:"onBoard"` // the one the board window is showing
}

// Library lists every stored review, most recently touched first.
func (b *Bridge) Library() ([]wireLibraryRow, error) {
	src := b.ui.boardSrc()
	if src == nil {
		return []wireLibraryRow{}, nil
	}
	rows, err := src.BoardLibrary()
	if err != nil {
		return nil, err
	}
	current := b.ui.board.target()
	out := make([]wireLibraryRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, wireLibraryRow{
			ID: r.ID, Title: r.Title, Repo: displayRepo(r.RepoRoot), Pinned: r.Pinned,
			State: r.State, Steps: r.CountedSteps, Understood: r.Understood,
			Unclear: r.Unclear, Comments: r.Comments, UpdatedMS: r.UpdatedAtMS,
			OnBoard: r.ID == current,
		})
	}
	return out, nil
}

// LibraryOpen puts a review on the board, retargeting the window that is
// already up rather than opening a second one.
func (b *Bridge) LibraryOpen(id string) error {
	src := b.ui.boardSrc()
	if src == nil {
		return errors.New("no store behind this window")
	}
	if _, _, _, err := src.BoardData(id); err != nil {
		return fmt.Errorf("cannot open %s: %w", id, err)
	}
	b.ui.ShowBoard(id)
	return nil
}

// LibraryDelete removes a review and everything on it. When the deleted one
// was on the board, the board is retargeted to whatever is left, or its
// window closed - a board still showing a review that no longer exists would
// take annotations nothing could store.
func (b *Bridge) LibraryDelete(id string) (bool, error) {
	src := b.ui.boardSrc()
	if src == nil {
		return false, errors.New("no store behind this window")
	}
	wasCurrent := b.ui.board.target() == id
	deleted, err := src.BoardDelete(id)
	if err != nil {
		return false, err
	}
	if deleted && wasCurrent {
		rows, err := src.BoardLibrary()
		if err == nil && len(rows) > 0 {
			b.ui.ShowBoard(rows[0].ID)
		} else {
			b.ui.board.close()
		}
	}
	return deleted, nil
}

// ShowLibrary raises the app window on the library surface. The board's own
// door to it: a reader who wants another review should not have to know that
// the list lives in a different window, let alone in a CLI.
func (b *Bridge) ShowLibrary() {
	b.ui.ShowApp("library")
}
