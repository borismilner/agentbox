package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/sound"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/version"
)

type fakeUI struct {
	mu          sync.Mutex
	views       []View
	appTabs     []string // tab ids passed to ShowApp, in call order
	summonCount int
	docs        []proto.ShowRequest
	progress    [][]ProgressState
	panelOpen   bool
	boards      []string // walkthrough ids passed to ShowBoard, in call order
	assignPokes int      // AssignmentsChanged calls
}

func (f *fakeUI) ShowBoard(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.boards = append(f.boards, id)
}

func (f *fakeUI) AssignmentsChanged() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assignPokes++
}

func (f *fakeUI) assignmentPokes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.assignPokes
}

func (f *fakeUI) ShowDocument(req proto.ShowRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.docs = append(f.docs, req)
}

// The drop-down panel (M10). The fake records where it is so a test can assert
// that an RPC moved it.
func (f *fakeUI) TogglePanel() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.panelOpen = !f.panelOpen
}

func (f *fakeUI) ShowPanel() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.panelOpen = true
}

func (f *fakeUI) HidePanel() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.panelOpen = false
}

func (f *fakeUI) PanelOpen() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.panelOpen
}

func (f *fakeUI) ShowProgress(reports []ProgressState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.progress = append(f.progress, reports)
}

// lastProgress returns the most recent progress report set the UI was given.
func (f *fakeUI) lastProgress() []ProgressState {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.progress) == 0 {
		return nil
	}
	return f.progress[len(f.progress)-1]
}

func (f *fakeUI) Present(v View) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.views = append(f.views, v)
}

func (f *fakeUI) ShowApp(tab string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.appTabs = append(f.appTabs, tab)
}

func (f *fakeUI) Summon() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.summonCount++
}

func (f *fakeUI) last() View {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.views) == 0 {
		return View{}
	}
	return f.views[len(f.views)-1]
}

// sawCaller reports whether any presented view carried the given caller
// state - robust against a short-lived view being replaced before a poll.
func (f *fakeUI) sawCaller(c CallerState) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.views {
		if v.Caller == c {
			return true
		}
	}
	return false
}

type fakeSound struct {
	mu     sync.Mutex
	played []sound.Class
	spoken []string
	waited []string // the lines the caller asked to be told about
	hold   chan struct{}
	stops  int // times the voice was cut off (the read-aloud transport)
}

// StopSpeaking records the interruption and releases anybody held on hold, the
// way a real Stop cuts off the line being heard. A reader blocked in SpeakWait
// has to come back, or a paused reading would keep its goroutine for ever.
// It re-arms rather than clearing hold, because a reading is stopped more than
// once in a life (start silences the previous one, then pause silences this one)
// and a fake that stops blocking after the first would let the rest of the
// reading run to completion instantly.
func (f *fakeSound) StopSpeaking() {
	f.mu.Lock()
	f.stops++
	hold := f.hold
	if hold != nil {
		f.hold = make(chan struct{})
	}
	f.mu.Unlock()
	if hold != nil {
		close(hold)
	}
}

// ReadWait is the reading path: recorded exactly like SpeakWait, since a test
// cares which lines were spoken and in what order, not which cap applied.
func (f *fakeSound) ReadWait(ctx context.Context, text string) { f.SpeakWait(ctx, text) }

func (f *fakeSound) stopCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stops
}

func (f *fakeSound) Play(c sound.Class) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.played = append(f.played, c)
}

// Speak records only what would actually be said, the way the real speaker
// discards an empty line, so a test can assert on the spoken lines alone.
func (f *fakeSound) Speak(text string) {
	if text == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spoken = append(f.spoken, text)
}

// SpeakWait records the line as spoken and as waited on. hold, when a test sets
// it, stands in for a sentence still playing: the call returns when the test
// closes it or the context ends, which is how the blocking half is tested without
// an audio device.
func (f *fakeSound) SpeakWait(ctx context.Context, text string) {
	f.Speak(text)
	f.mu.Lock()
	hold := f.hold
	if text != "" {
		f.waited = append(f.waited, text)
	}
	f.mu.Unlock()
	if hold == nil {
		return
	}
	select {
	case <-hold:
	case <-ctx.Done():
	}
}

func (f *fakeSound) waitedLines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.waited...)
}

func (f *fakeSound) spokenLines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.spoken...)
}

// fakePresence answers the FR29/FR44 presence signals. The mutex lets a test
// flip a signal while the escalation goroutine reads it without racing.
type fakePresence struct {
	mu         sync.Mutex
	idle       bool
	fullscreen bool
	desktopDnd bool
}

func (f *fakePresence) IdleFor(time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.idle
}

func (f *fakePresence) FullscreenActive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fullscreen
}

func (f *fakePresence) DesktopDND() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.desktopDnd
}

func (f *fakePresence) set(idle, fullscreen, desktopDnd bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idle, f.fullscreen, f.desktopDnd = idle, fullscreen, desktopDnd
}

func newTestDaemon(t *testing.T, cfg Config) (*Daemon, *fakeUI, *fakeSound, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "agentbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ui := &fakeUI{}
	snd := &fakeSound{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d, err := New(cfg, log, st, snd, ui)
	if err != nil {
		t.Fatal(err)
	}
	return d, ui, snd, st
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func notifyItem(level proto.Level) proto.Item {
	return proto.Item{
		Kind: proto.KindNotify, Level: level, Title: "build done",
		Identity: proto.Identity{Agent: "claude-code"},
	}
}

func askItem() proto.Item {
	return proto.Item{
		Kind: proto.KindChoice, Title: "Deploy?",
		Options:  []proto.Option{{Label: "Yes"}, {Label: "No"}},
		Identity: proto.Identity{Agent: "claude-code"},
	}
}

func callNotify(t *testing.T, d *Daemon, it proto.Item) proto.Result {
	t.Helper()
	res, rpcErr := d.Handle(context.Background(), proto.MethodNotify, mustJSON(t, it))
	if rpcErr != nil {
		t.Fatalf("notify: %v", rpcErr)
	}
	return res.(proto.Result)
}

func TestNotifyReturnsImmediatelyAndPresents(t *testing.T) {
	d, ui, snd, _ := newTestDaemon(t, Config{})
	res := callNotify(t, d, notifyItem(proto.LevelWarning))
	if res.ID == "" {
		t.Fatal("no item id")
	}
	v := ui.last()
	if v.Item == nil || v.Item.ID != res.ID {
		t.Fatalf("presented view = %+v, want item %s", v, res.ID)
	}
	snd.mu.Lock()
	defer snd.mu.Unlock()
	if len(snd.played) != 1 || snd.played[0] != sound.ClassWarning {
		t.Fatalf("played = %v, want [twotone]", snd.played)
	}
}

func TestInfoToastAutoDismisses(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{ToastDuration: 50 * time.Millisecond})
	res := callNotify(t, d, notifyItem(proto.LevelInfo))

	deadline := time.Now().Add(2 * time.Second)
	for ui.last().Item != nil {
		if time.Now().After(deadline) {
			t.Fatal("toast never auto-dismissed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	recent, err := st.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if recent[0].ID != res.ID || recent[0].State != store.StateDismissed {
		t.Fatalf("stored state = %s, want dismissed", recent[0].State)
	}
}

func TestWarningToastSticks(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{ToastDuration: 30 * time.Millisecond})
	callNotify(t, d, notifyItem(proto.LevelWarning))
	time.Sleep(150 * time.Millisecond)
	if ui.last().Item == nil {
		t.Fatal("warning toast auto-dismissed; must stick until dismissed")
	}
}

func askAsync(t *testing.T, d *Daemon, it proto.Item) chan proto.Result {
	t.Helper()
	ch := make(chan proto.Result, 1)
	go func() {
		res, rpcErr := d.Handle(context.Background(), proto.MethodAsk, mustJSON(t, it))
		if rpcErr != nil {
			t.Error(rpcErr)
			close(ch)
			return
		}
		ch <- res.(proto.Result)
	}()
	return ch
}

func waitForItem(t *testing.T, ui *fakeUI) *proto.Item {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if it := ui.last().Item; it != nil {
			return it
		}
		if time.Now().After(deadline) {
			t.Fatal("nothing presented")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAskBlocksUntilAnswered(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)

	select {
	case <-ch:
		t.Fatal("ask returned before any answer")
	case <-time.After(50 * time.Millisecond):
	}

	d.Answer(it.ID, "Yes")
	select {
	case res := <-ch:
		if !res.Answered || res.Answer != "Yes" {
			t.Fatalf("result = %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("answer never delivered")
	}

	recent, _ := st.Recent(1)
	if recent[0].State != store.StateAnswered || recent[0].Answer != "Yes" {
		t.Fatalf("stored: %+v", recent[0])
	}
	if ui.last().Item != nil {
		t.Fatal("display not cleared after answer")
	}
}

func TestAskTimeoutAppliesDefault(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{})
	it := askItem()
	it.TimeoutS = 1
	it.Default = "No"
	ch := askAsync(t, d, it)
	waitForItem(t, ui)

	select {
	case res := <-ch:
		if res.Answered || !res.DefaultApplied || res.Answer != "No" {
			t.Fatalf("result = %+v, want default applied", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout never fired")
	}
	recent, _ := st.Recent(1)
	if recent[0].State != store.StateExpired {
		t.Fatalf("stored state = %s, want expired", recent[0].State)
	}
}

func TestAnswerDeliveredExactlyOnce(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)

	d.Answer(it.ID, "Yes")
	d.Answer(it.ID, "No") // must lose: already resolved

	res := <-ch
	if res.Answer != "Yes" {
		t.Fatalf("delivered %q, want the first answer", res.Answer)
	}
}

func TestQueueAdvancesInOrder(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	first := askItem()
	first.Title = "first"
	second := askItem()
	second.Title = "second"
	ch1 := askAsync(t, d, first)
	it1 := waitForItem(t, ui)
	if it1.Title != "first" {
		t.Fatalf("displaying %q first", it1.Title)
	}
	ch2 := askAsync(t, d, second)

	deadline := time.Now().Add(2 * time.Second)
	for ui.last().Waiting != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("waiting count = %d, want 1", ui.last().Waiting)
		}
		time.Sleep(5 * time.Millisecond)
	}

	d.Answer(it1.ID, "Yes")
	<-ch1
	deadline = time.Now().Add(2 * time.Second)
	for {
		v := ui.last()
		if v.Item != nil && v.Item.Title == "second" && v.Waiting == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("second never displayed; view = %+v", ui.last())
		}
		time.Sleep(5 * time.Millisecond)
	}
	d.Answer(ui.last().Item.ID, "No")
	<-ch2
}

func TestUrgentPreemptsAndRestores(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	ch1 := askAsync(t, d, askItem())
	plain := waitForItem(t, ui)

	urgent := askItem()
	urgent.Level = proto.LevelUrgent
	urgent.Title = "URGENT"
	ch2 := askAsync(t, d, urgent)

	deadline := time.Now().Add(2 * time.Second)
	for ui.last().Item == nil || ui.last().Item.Title != "URGENT" {
		if time.Now().After(deadline) {
			t.Fatalf("urgent never preempted; showing %+v", ui.last().Item)
		}
		time.Sleep(5 * time.Millisecond)
	}

	d.Answer(ui.last().Item.ID, "Yes")
	<-ch2
	deadline = time.Now().Add(2 * time.Second)
	for ui.last().Item == nil || ui.last().Item.ID != plain.ID {
		if time.Now().After(deadline) {
			t.Fatal("preempted card never came back")
		}
		time.Sleep(5 * time.Millisecond)
	}
	d.Answer(plain.ID, "No")
	<-ch1
}

func TestCancelUnblocksCaller(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)

	_, rpcErr := d.Handle(context.Background(), proto.MethodCancel, mustJSON(t, map[string]string{"id": it.ID}))
	if rpcErr != nil {
		t.Fatalf("cancel: %v", rpcErr)
	}
	res := <-ch
	if res.Answered {
		t.Fatalf("cancelled ask reported answered: %+v", res)
	}
}

// A timed question's View carries the live expiry deadline, so the
// card footer counts down the truth instead of echoing the configured
// total.
func TestTimedAskViewCarriesExpiry(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	timed := askItem()
	timed.TimeoutS = 60
	ch := askAsync(t, d, timed)
	it := waitForItem(t, ui)

	v := ui.last()
	if v.ExpiresAt.IsZero() {
		t.Fatal("timed ask view has no expiry deadline")
	}
	if until := time.Until(v.ExpiresAt); until <= 0 || until > 60*time.Second {
		t.Fatalf("expiry deadline out of range: %v", until)
	}

	d.Dismiss(it.ID)
	<-ch
}

// FR50: dismissing a blocking question (shift+Esc on the card, d in
// triage) resolves the ask unanswered, exactly like a timeout.
func TestDismissUnblocksAsk(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)

	d.Dismiss(it.ID)
	res := <-ch
	if res.Answered {
		t.Fatalf("dismissed ask reported answered: %+v", res)
	}
	if res.Answer != "" || res.Reply != "" {
		t.Fatalf("dismissed ask carried an answer: %+v", res)
	}
}

func TestCancelUnknownItemErrs(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	_, rpcErr := d.Handle(context.Background(), proto.MethodCancel, mustJSON(t, map[string]string{"id": "ghost"}))
	if rpcErr == nil || rpcErr.Code != proto.CodeItemNotFound {
		t.Fatalf("got %v, want item-not-found", rpcErr)
	}
}

func TestDeferSendsToBack(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	first := askItem()
	first.Title = "first"
	ch1 := askAsync(t, d, first)
	it1 := waitForItem(t, ui)
	second := askItem()
	second.Title = "second"
	ch2 := askAsync(t, d, second)

	deadline := time.Now().Add(2 * time.Second)
	for ui.last().Waiting != 1 {
		if time.Now().After(deadline) {
			t.Fatal("second never queued")
		}
		time.Sleep(5 * time.Millisecond)
	}

	d.Defer(it1.ID)
	deadline = time.Now().Add(2 * time.Second)
	for ui.last().Item == nil || ui.last().Item.Title != "second" {
		if time.Now().After(deadline) {
			t.Fatal("defer did not advance to second")
		}
		time.Sleep(5 * time.Millisecond)
	}

	d.Answer(ui.last().Item.ID, "Yes")
	<-ch2
	deadline = time.Now().Add(2 * time.Second)
	for ui.last().Item == nil || ui.last().Item.Title != "first" {
		if time.Now().After(deadline) {
			t.Fatal("deferred card never returned")
		}
		time.Sleep(5 * time.Millisecond)
	}
	d.Answer(it1.ID, "No")
	<-ch1
}

func TestPendingRestoredOnStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentbox.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	it := askItem()
	it.ID = "k-restored"
	if err := st.CreateItem(&it); err != nil {
		t.Fatal(err)
	}
	st.Close()

	st2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st2.Close() })
	ui := &fakeUI{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := New(Config{}, log, st2, &fakeSound{}, ui); err != nil {
		t.Fatal(err)
	}
	v := ui.last()
	if v.Item == nil || v.Item.ID != "k-restored" {
		t.Fatalf("restored view = %+v, want k-restored displayed", v)
	}
}

func TestKindMethodMismatchTeaches(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	_, rpcErr := d.Handle(context.Background(), proto.MethodAsk, mustJSON(t, notifyItem(proto.LevelInfo)))
	if rpcErr == nil || rpcErr.Code != proto.CodeInvalidParams {
		t.Fatalf("got %v, want invalid-params", rpcErr)
	}
	if want := "agentbox.v1.notify"; !json.Valid([]byte(`"x"`)) || rpcErr.Message == "" || !containsStr(rpcErr.Message, want) {
		t.Fatalf("error %q does not teach the correct method (%s)", rpcErr.Message, want)
	}
}

func TestPromotePullsQueuedToFront(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	first := askItem()
	first.Title = "first"
	ch1 := askAsync(t, d, first)
	it1 := waitForItem(t, ui)
	second := askItem()
	second.Title = "second"
	ch2 := askAsync(t, d, second)

	deadline := time.Now().Add(2 * time.Second)
	for ui.last().Waiting != 1 {
		if time.Now().After(deadline) {
			t.Fatal("second never queued")
		}
		time.Sleep(5 * time.Millisecond)
	}
	var secondID string
	d.mu.Lock()
	secondID = d.queue[0].ID
	d.mu.Unlock()

	d.Promote(secondID)
	deadline = time.Now().Add(2 * time.Second)
	for ui.last().Item == nil || ui.last().Item.ID != secondID {
		if time.Now().After(deadline) {
			t.Fatal("promote never displayed the queued item")
		}
		time.Sleep(5 * time.Millisecond)
	}

	d.Answer(secondID, "Yes")
	<-ch2
	deadline = time.Now().Add(2 * time.Second)
	for ui.last().Item == nil || ui.last().Item.ID != it1.ID {
		if time.Now().After(deadline) {
			t.Fatal("demoted item never came back")
		}
		time.Sleep(5 * time.Millisecond)
	}
	d.Answer(it1.ID, "No")
	<-ch1
}

func TestInboxMethodOpensAppOnInboxTab(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	_, rpcErr := d.Handle(context.Background(), proto.MethodInbox, mustJSON(t, struct{}{}))
	if rpcErr != nil {
		t.Fatalf("inbox: %v", rpcErr)
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.appTabs) != 1 || ui.appTabs[0] != "inbox" {
		t.Fatalf("appTabs = %v, want [inbox]", ui.appTabs)
	}
}

func TestAppMethodOpensRequestedTab(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	_, rpcErr := d.Handle(context.Background(), proto.MethodApp, mustJSON(t, map[string]string{"tab": "stats"}))
	if rpcErr != nil {
		t.Fatalf("app: %v", rpcErr)
	}
	if _, rpcErr := d.Handle(context.Background(), proto.MethodApp, mustJSON(t, struct{}{})); rpcErr != nil {
		t.Fatalf("app (no tab): %v", rpcErr)
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.appTabs) != 2 || ui.appTabs[0] != "stats" || ui.appTabs[1] != "" {
		t.Fatalf("appTabs = %v, want [stats, \"\"]", ui.appTabs)
	}
}

func TestSummonMethodSummonsUI(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	res, rpcErr := d.Handle(context.Background(), proto.MethodSummon, mustJSON(t, struct{}{}))
	if rpcErr != nil {
		t.Fatalf("summon: %v", rpcErr)
	}
	if ok, _ := res.(map[string]bool)["ok"]; !ok {
		t.Fatalf("summon result = %+v, want {ok:true}", res)
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.summonCount != 1 {
		t.Fatalf("summonCount = %d, want 1", ui.summonCount)
	}
}

func TestGraceDelaysDeliveryThenDelivers(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{UndoGrace: 150 * time.Millisecond})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)

	d.Answer(it.ID, "Yes")
	v := ui.last()
	if !v.Graced || v.GracedText != "Answered: Yes" {
		t.Fatalf("expected graced view, got %+v", v)
	}
	select {
	case <-ch:
		t.Fatal("answer delivered inside the grace window")
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case res := <-ch:
		if !res.Answered || res.Answer != "Yes" {
			t.Fatalf("result = %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("answer never delivered after grace")
	}
}

func TestUndoRestoresCardAndSecondAnswerWins(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{UndoGrace: 10 * time.Second})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)

	d.Answer(it.ID, "Yes")
	if !ui.last().Graced {
		t.Fatal("not graced")
	}
	d.Undo(it.ID)
	v := ui.last()
	if v.Graced || v.Item == nil || v.Item.ID != it.ID {
		t.Fatalf("undo did not restore the card: %+v", v)
	}

	d.Answer(it.ID, "No")
	d.mu.Lock()
	g := d.graced
	d.mu.Unlock()
	if g == nil {
		t.Fatal("second answer not graced")
	}
	d.finalizeGrace(g)
	res := <-ch
	if res.Answer != "No" {
		t.Fatalf("delivered %q, want the post-undo answer", res.Answer)
	}
}

func TestTimeoutCannotStealGracedAnswer(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{UndoGrace: 300 * time.Millisecond})
	it := askItem()
	it.TimeoutS = 1
	it.Default = "No"
	ch := askAsync(t, d, it)
	shown := waitForItem(t, ui)

	// Answer just before the timeout fires; the grace outlives it.
	time.Sleep(900 * time.Millisecond)
	d.Answer(shown.ID, "Yes")
	select {
	case res := <-ch:
		if !res.Answered || res.Answer != "Yes" || res.DefaultApplied {
			t.Fatalf("result = %+v, want the user's answer, not the timeout default", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("nothing delivered")
	}
}

func TestFormRoundTrip(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{})
	form := proto.Item{
		Kind: proto.KindForm, Title: "Release",
		Fields: []proto.Field{
			{Key: "env", Type: proto.FieldChoice, Options: []string{"staging", "prod"}},
			{Key: "tag", Type: proto.FieldText},
		},
		Identity: proto.Identity{Agent: "claude-code"},
	}
	ch := askAsync(t, d, form)
	it := waitForItem(t, ui)

	d.AnswerForm(it.ID, map[string]string{"env": "prod", "tag": "v2.0"})
	select {
	case res := <-ch:
		if !res.Answered || res.Values["env"] != "prod" || res.Values["tag"] != "v2.0" {
			t.Fatalf("result = %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("form result never delivered")
	}
	recent, _ := st.Recent(1)
	if recent[0].Values["env"] != "prod" {
		t.Fatalf("values not persisted: %+v", recent[0].Values)
	}
}

func TestQuitMethod(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	called := make(chan struct{})
	d.OnQuit = func() { close(called) }
	_, rpcErr := d.Handle(context.Background(), proto.MethodQuit, mustJSON(t, struct{}{}))
	if rpcErr != nil {
		t.Fatalf("quit: %v", rpcErr)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("OnQuit never called")
	}
}

func (f *fakeSound) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.played)
}

func TestEscalationReplaysUpToCap(t *testing.T) {
	d, ui, snd, _ := newTestDaemon(t, Config{
		EscalationInterval: 40 * time.Millisecond,
		EscalationCount:    2,
	})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)

	// Initial chime + 2 escalations, then silence.
	deadline := time.Now().Add(2 * time.Second)
	for snd.count() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("escalations never fired; played %d", snd.count())
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)
	if n := snd.count(); n != 3 {
		t.Fatalf("played %d sounds, want exactly 3 (cap respected)", n)
	}

	d.Answer(it.ID, "Yes")
	<-ch
}

func TestEscalationStopsOnAnswer(t *testing.T) {
	d, ui, snd, _ := newTestDaemon(t, Config{
		EscalationInterval: 50 * time.Millisecond,
		EscalationCount:    100,
	})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)
	d.Answer(it.ID, "Yes")
	<-ch
	base := snd.count()
	time.Sleep(200 * time.Millisecond)
	if snd.count() != base {
		t.Fatal("escalation continued after the item was answered")
	}
}

func TestDndQueuesSilentlyAndRevealsOnOff(t *testing.T) {
	d, ui, snd, _ := newTestDaemon(t, Config{})
	d.DndSet(true)

	callNotify(t, d, notifyItem(proto.LevelWarning))
	if v := ui.last(); v.Item != nil {
		t.Fatalf("DND displayed an item: %+v", v.Item)
	}
	if snd.count() != 0 {
		t.Fatal("DND played a sound")
	}

	d.DndSet(false)
	deadline := time.Now().Add(2 * time.Second)
	for ui.last().Item == nil {
		if time.Now().After(deadline) {
			t.Fatal("queued item never revealed after DND off")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if snd.count() != 1 {
		t.Fatalf("reveal should chime once, played %d", snd.count())
	}
}

func TestUrgentBreaksThroughDnd(t *testing.T) {
	d, ui, snd, _ := newTestDaemon(t, Config{})
	d.DndSet(true)

	urgent := askItem()
	urgent.Level = proto.LevelUrgent
	ch := askAsync(t, d, urgent)
	it := waitForItem(t, ui)
	if snd.count() == 0 {
		t.Fatal("urgent must sound through DND")
	}
	d.Answer(it.ID, "Yes")
	<-ch
}

func TestDndMethodTogglesAndReports(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	res, rpcErr := d.Handle(context.Background(), proto.MethodDnd, mustJSON(t, map[string]bool{"set": true}))
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if !res.(map[string]any)["enabled"].(bool) {
		t.Fatal("set true reported false")
	}
	res, _ = d.Handle(context.Background(), proto.MethodDnd, mustJSON(t, map[string]any{}))
	if !res.(map[string]any)["enabled"].(bool) {
		t.Fatal("status query flipped the state")
	}
}

// The switch is not the whole answer. With do-not-disturb OFF, FR29's auto-DND
// rules can still be holding everything back, and a status that stopped at "off"
// while nothing reached the screen was indistinguishable from a broken install -
// which is exactly how it was found, in the middle of a demo.
func TestDndStateNamesTheRuleThatIsHoldingThings(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{FullscreenAutoDnd: true, RespectDesktopDnd: true})

	// Nothing on: off, nothing held, nothing to explain.
	if on, auto, why := d.DndState(); on || auto || why != "" {
		t.Fatalf("a quiet daemon reported on=%v auto=%v why=%q", on, auto, why)
	}

	// A focused fullscreen window holds interruptions with the switch still off.
	fp := &fakePresence{}
	d.SetPresence(fp)
	fp.set(false, true, false)
	on, auto, why := d.DndState()
	if on {
		t.Error("the switch reported on when nobody touched it")
	}
	if !auto {
		t.Error("a focused fullscreen window did not register as held")
	}
	if !strings.Contains(why, "fullscreen") || !strings.Contains(why, "fullscreen_auto_dnd") {
		t.Errorf("the reason does not name the rule or the knob: %q", why)
	}

	// The desktop's own do-not-disturb is the other rule, and both can hold at once.
	fp.set(false, true, true)
	if _, _, why = d.DndState(); !strings.Contains(why, "desktop") {
		t.Errorf("the desktop rule is missing from %q", why)
	}

	// Turning the rules off in config takes the explanation with them.
	d.SetPolicy(Config{})
	if _, auto, why = d.DndState(); auto || why != "" {
		t.Errorf("with both knobs off: auto=%v why=%q", auto, why)
	}

	// And the switch alone still explains itself.
	d.DndSet(true)
	if on, _, why = d.DndState(); !on || !strings.Contains(why, "switched on") {
		t.Errorf("manual DND reported on=%v why=%q", on, why)
	}

	// The RPC carries all three, so `agentbox dnd status` can say it.
	res, rpcErr := d.Handle(context.Background(), proto.MethodDnd, mustJSON(t, map[string]any{}))
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	m := res.(map[string]any)
	if _, ok := m["auto_held"]; !ok {
		t.Error("the dnd result has no auto_held field")
	}
	if _, ok := m["reason"]; !ok {
		t.Error("the dnd result has no reason field")
	}
}

// Wall-clock check of the production default after a user report of a
// 10+ s strip: the answer must ship 3 s after the click, give or take
// scheduler slack, and the view must carry the deadline.
func TestGraceDefaultIsThreeSecondsWallClock(t *testing.T) {
	if testing.Short() {
		t.Skip("3 s wall-clock test")
	}
	d, ui, _, _ := newTestDaemon(t, Config{UndoGrace: 3 * time.Second})
	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)

	start := time.Now()
	d.Answer(it.ID, "Yes")
	v := ui.last()
	if v.GraceUntil.IsZero() || time.Until(v.GraceUntil) > 3100*time.Millisecond {
		t.Fatalf("GraceUntil = %v, want ~3 s out", v.GraceUntil)
	}
	res := <-ch
	elapsed := time.Since(start)
	if !res.Answered || res.Answer != "Yes" {
		t.Fatalf("result = %+v", res)
	}
	if elapsed < 2800*time.Millisecond || elapsed > 4500*time.Millisecond {
		t.Fatalf("answer shipped after %v, want ~3 s", elapsed)
	}
}

func TestViewCarriesWaitingIdentities(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	first := askItem()
	ch1 := askAsync(t, d, first)
	it1 := waitForItem(t, ui)

	second := askItem()
	second.Identity = proto.Identity{Agent: "minibot", Project: "sigs"}
	ch2 := askAsync(t, d, second)

	deadline := time.Now().Add(2 * time.Second)
	for ui.last().Waiting != 1 {
		if time.Now().After(deadline) {
			t.Fatal("second never queued")
		}
		time.Sleep(5 * time.Millisecond)
	}
	v := ui.last()
	if len(v.WaitingFrom) != 1 || v.WaitingFrom[0].Agent != "minibot" {
		t.Fatalf("WaitingFrom = %+v, want the queued agent's identity", v.WaitingFrom)
	}

	d.Answer(it1.ID, "Yes")
	<-ch1
	deadline = time.Now().Add(2 * time.Second)
	for ui.last().Item == nil || ui.last().Item.Identity.Agent != "minibot" {
		if time.Now().After(deadline) {
			t.Fatal("second agent's card never displayed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	d.Answer(ui.last().Item.ID, "No")
	<-ch2
}

func TestToastViewCarriesDismissDeadline(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{ToastDuration: time.Minute})
	callNotify(t, d, notifyItem(proto.LevelInfo))
	v := ui.last()
	if v.DismissAt.IsZero() || time.Until(v.DismissAt) > time.Minute {
		t.Fatalf("info toast DismissAt = %v", v.DismissAt)
	}

	d.Dismiss(v.Item.ID)
	callNotify(t, d, notifyItem(proto.LevelWarning))
	if v := ui.last(); !v.DismissAt.IsZero() {
		t.Fatalf("sticky warning has DismissAt = %v, want zero", v.DismissAt)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestStatsRPC(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	callNotify(t, d, notifyItem(proto.LevelInfo))

	raw, rpcErr := d.Handle(context.Background(), proto.MethodStats, mustJSON(t, map[string]int64{"since_ms": 0}))
	if rpcErr != nil {
		t.Fatalf("stats rpc: %v", rpcErr)
	}
	// The result must survive the JSON round trip into proto.Stats (wire contract).
	var st proto.Stats
	if err := json.Unmarshal(mustJSON(t, raw), &st); err != nil {
		t.Fatal(err)
	}
	if st.Total != 1 || len(st.ByAgent) != 1 || st.ByAgent[0].Agent != "claude-code" {
		t.Fatalf("unexpected stats: %+v", st)
	}
	// Empty params ({}) are tolerated and mean all-time.
	if _, rpcErr := d.Handle(context.Background(), proto.MethodStats, mustJSON(t, struct{}{})); rpcErr != nil {
		t.Fatalf("empty params: %v", rpcErr)
	}
}

func vetoItem(timeoutS int) proto.Item {
	return proto.Item{
		Kind: proto.KindVeto, Level: proto.LevelWarning, Title: "Pushing to main",
		TimeoutS: timeoutS, Identity: proto.Identity{Agent: "claude-code"},
	}
}

func TestVetoProceedsOnTimeout(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{})
	ch := askAsync(t, d, vetoItem(1))
	shown := waitForItem(t, ui)
	if shown.Kind != proto.KindVeto {
		t.Fatalf("expected a veto card, got %q", shown.Kind)
	}
	select {
	case res := <-ch:
		if res.Vetoed {
			t.Fatalf("window elapsed unstopped, want proceed (vetoed=false): %+v", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("veto never proceeded after its window")
	}
	recent, _ := st.Recent(1)
	if recent[0].State != store.StateExpired {
		t.Fatalf("stored state = %q, want expired (proceeded)", recent[0].State)
	}
}

func TestVetoStoppedByUser(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{})
	ch := askAsync(t, d, vetoItem(30))
	shown := waitForItem(t, ui)
	d.Veto(shown.ID)
	select {
	case res := <-ch:
		if !res.Vetoed {
			t.Fatalf("user stopped it, want vetoed=true: %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("veto stop never delivered")
	}
	recent, _ := st.Recent(1)
	if recent[0].State != store.StateAnswered {
		t.Fatalf("stored state = %q, want answered (vetoed)", recent[0].State)
	}
	if ui.last().Item != nil {
		t.Fatal("display not cleared after veto")
	}
}

func TestVetoDefaultWindowApplied(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{VetoWindow: 30 * time.Second})
	ch := askAsync(t, d, vetoItem(0)) // no window: daemon fills the configured default
	shown := waitForItem(t, ui)
	if shown.TimeoutS != 30 {
		t.Fatalf("default veto window not applied: timeout_s=%d, want 30", shown.TimeoutS)
	}
	d.Veto(shown.ID)
	<-ch
}

func TestSecretWritesFileNotStore(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{})
	sink := filepath.Join(t.TempDir(), "token")
	it := proto.Item{Kind: proto.KindSecret, Title: "npm token", Sink: sink,
		Identity: proto.Identity{Agent: "claude-code"}}
	ch := askAsync(t, d, it)
	shown := waitForItem(t, ui)
	d.Secret(shown.ID, "hunter2")
	res := <-ch
	if !res.Answered {
		t.Fatalf("secret not answered: %+v", res)
	}
	if res.Secret != "" {
		t.Fatalf("value must not cross the socket without stdout, got %q", res.Secret)
	}
	if res.SecretPath != sink {
		t.Fatalf("secret_path = %q, want %q", res.SecretPath, sink)
	}
	data, err := os.ReadFile(sink)
	if err != nil || string(data) != "hunter2" {
		t.Fatalf("secret file contents %q err=%v", data, err)
	}
	if fi, _ := os.Stat(sink); fi.Mode().Perm() != 0o600 {
		t.Fatalf("secret file mode = %v, want 0600", fi.Mode().Perm())
	}
	recent, _ := st.Recent(1)
	if recent[0].State != store.StateAnswered {
		t.Fatalf("state = %q, want answered", recent[0].State)
	}
	// The value must appear nowhere in the persisted row.
	if strings.Contains(fmt.Sprintf("%+v", recent[0]), "hunter2") {
		t.Fatalf("secret value leaked into the store: %+v", recent[0])
	}
}

func TestSecretStdoutReturnsValue(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	it := proto.Item{Kind: proto.KindSecret, Title: "token", Stdout: true,
		Identity: proto.Identity{Agent: "a"}}
	ch := askAsync(t, d, it)
	shown := waitForItem(t, ui)
	d.Secret(shown.ID, "sk-123")
	res := <-ch
	if res.Secret != "sk-123" || res.SecretPath != "" {
		t.Fatalf("stdout opt-in should return the value and no path: %+v", res)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunActionExecutesCommandInCwd(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	dir := t.TempDir()
	it := notifyItem(proto.LevelWarning)
	it.Cwd = dir
	it.Actions = []proto.Action{{Label: "Run", Exec: "touch ran"}}
	res := callNotify(t, d, it)

	d.RunAction(res.ID, 0)
	waitFor(t, "action command", func() bool {
		_, err := os.Stat(filepath.Join(dir, "ran"))
		return err == nil
	})
}

func TestRunActionDisabledNeverRuns(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{ActionsDisabled: true})
	dir := t.TempDir()
	it := notifyItem(proto.LevelWarning)
	it.Cwd = dir
	it.Actions = []proto.Action{{Label: "Run", Exec: "touch ran"}}
	res := callNotify(t, d, it)
	if ui.last().ActionsEnabled {
		t.Fatal("view should report actions disabled")
	}

	d.RunAction(res.ID, 0)
	time.Sleep(200 * time.Millisecond) // give the goroutine that must not start its chance
	if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
		t.Fatal("disabled action still ran")
	}
}

func TestRunActionOnlyTriggersDisplayedItem(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	dir := t.TempDir()
	it := notifyItem(proto.LevelWarning)
	it.Cwd = dir
	it.Actions = []proto.Action{{Label: "Run", Exec: "touch ran"}}
	res := callNotify(t, d, it)

	d.RunAction(res.ID, 5)          // out of range
	d.RunAction("k-nonexistent", 0) // not the displayed item
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
		t.Fatal("a bad RunAction index/id should be a no-op")
	}
}

func TestRunActionFailureSurfacesErrorCard(t *testing.T) {
	d, _, _, st := newTestDaemon(t, Config{})
	it := notifyItem(proto.LevelWarning)
	it.Actions = []proto.Action{{Label: "Boom", Exec: "exit 7"}}
	res := callNotify(t, d, it)

	d.RunAction(res.ID, 0)
	waitFor(t, "error card", func() bool {
		recent, err := st.Recent(10)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range recent {
			if r.ID != res.ID && r.Level == proto.LevelError && strings.Contains(r.Title, "Action failed") {
				return true
			}
		}
		return false
	})
}

func TestViewReflectsActionsPolicy(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	callNotify(t, d, notifyItem(proto.LevelWarning))
	if !ui.last().ActionsEnabled {
		t.Fatal("actions enabled by default")
	}
	d.SetPolicy(Config{ActionsDisabled: true})
	callNotify(t, d, notifyItem(proto.LevelWarning))
	if ui.last().ActionsEnabled {
		t.Fatal("SetPolicy did not disable actions live")
	}
}

func TestShowDocumentRPC(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	_, rpcErr := d.Handle(context.Background(), proto.MethodShow,
		mustJSON(t, proto.ShowRequest{Path: "/tmp/x.md", Watch: true}))
	if rpcErr != nil {
		t.Fatalf("show: %v", rpcErr)
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.docs) != 1 || ui.docs[0].Path != "/tmp/x.md" || !ui.docs[0].Watch {
		t.Fatalf("ShowDocument not called as expected: %+v", ui.docs)
	}
}

func TestShowDocumentRejectsEmpty(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	if _, rpcErr := d.Handle(context.Background(), proto.MethodShow, mustJSON(t, proto.ShowRequest{})); rpcErr == nil {
		t.Fatal("empty show request should error")
	}
}

func TestReviewRoundTrip(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	it := proto.Item{Kind: proto.KindDiff, Title: "Apply patch?",
		Diff: "@@ -1 +1 @@\n-old\n+new\n", Identity: proto.Identity{Agent: "a"}}
	ch := askAsync(t, d, it)
	shown := waitForItem(t, ui)

	d.Review(shown.ID, true, "looks good")
	res := <-ch
	if !res.Answered || !res.Approved || res.Reply != "looks good" {
		t.Fatalf("approve result = %+v", res)
	}
}

func TestReviewRejectCarriesComment(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	it := proto.Item{Kind: proto.KindDiff, Title: "Apply patch?",
		Diff: "@@ -1 +1 @@", Identity: proto.Identity{Agent: "a"}}
	ch := askAsync(t, d, it)
	shown := waitForItem(t, ui)

	d.Review(shown.ID, false, "rename the field")
	res := <-ch
	if !res.Answered || res.Approved || res.Reply != "rename the field" {
		t.Fatalf("reject result = %+v", res)
	}
}

// mute drives a mute/unmute through the RPC handler and returns the muted set
// (FR47), so the tests exercise the wire path the CLI uses.
func muteRPC(t *testing.T, d *Daemon, agent string, unmute bool) []string {
	t.Helper()
	res, rpcErr := d.Handle(context.Background(), proto.MethodMute,
		mustJSON(t, map[string]any{"agent": agent, "unmute": unmute}))
	if rpcErr != nil {
		t.Fatalf("mute: %v", rpcErr)
	}
	return res.(map[string]any)["muted"].([]string)
}

func (f *fakeSound) plays() []sound.Class {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sound.Class(nil), f.played...)
}

func TestMutedAgentGoesStraightToInbox(t *testing.T) {
	d, ui, snd, st := newTestDaemon(t, Config{})
	muteRPC(t, d, "noisy", false)

	// A warning notify would normally stick on screen; muted, it must not.
	res := callNotify(t, d, proto.Item{Kind: proto.KindNotify, Level: proto.LevelWarning,
		Title: "loop iteration", Identity: proto.Identity{Agent: "noisy"}})
	if v := ui.last(); v.Item != nil {
		t.Fatalf("muted item surfaced: %+v", v.Item)
	}
	if p := snd.plays(); len(p) != 0 {
		t.Fatalf("muted item sounded: %v", p)
	}
	pending, err := st.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != res.ID {
		t.Fatalf("muted item not held pending in store: %+v", pending)
	}
}

func TestMuteDoesNotAffectOtherAgents(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	muteRPC(t, d, "noisy", false)
	res := callNotify(t, d, proto.Item{Kind: proto.KindNotify, Level: proto.LevelWarning,
		Title: "real work", Identity: proto.Identity{Agent: "claude-code"}})
	if v := ui.last(); v.Item == nil || v.Item.ID != res.ID {
		t.Fatalf("unmuted agent's item did not surface: %+v", v)
	}
}

func TestUnmuteRevealsHeldItem(t *testing.T) {
	d, ui, snd, _ := newTestDaemon(t, Config{})
	muteRPC(t, d, "noisy", false)
	res := callNotify(t, d, proto.Item{Kind: proto.KindNotify, Level: proto.LevelWarning,
		Title: "held", Identity: proto.Identity{Agent: "noisy"}})
	if ui.last().Item != nil {
		t.Fatal("item should be held while muted")
	}
	muteRPC(t, d, "noisy", true)
	if v := ui.last(); v.Item == nil || v.Item.ID != res.ID {
		t.Fatalf("unmute did not reveal held item: %+v", v)
	}
	if p := snd.plays(); len(p) != 1 || p[0] != sound.ClassWarning {
		t.Fatalf("reveal did not sound the earcon: %v", p)
	}
}

func TestMutedBlockingItemAnswerableFromInbox(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{})
	muteRPC(t, d, "noisy", false)
	it := askItem()
	it.Identity.Agent = "noisy"
	ch := askAsync(t, d, it)

	// It must not take the screen, but it is pending and answerable.
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		pending, err := st.Pending()
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) == 1 {
			if ui.last().Item != nil {
				t.Fatalf("muted question surfaced: %+v", ui.last().Item)
			}
			d.Answer(pending[0].ID, "Yes")
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("muted question never reached the store")
		}
		time.Sleep(5 * time.Millisecond)
	}
	res := <-ch
	if !res.Answered || res.Answer != "Yes" {
		t.Fatalf("answer from inbox = %+v", res)
	}
}

// waitForDismiss blocks until the screen clears (the toast auto-closed).
func waitForDismiss(t *testing.T, ui *fakeUI) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for ui.last().Item != nil {
		if time.Now().After(deadline) {
			t.Fatal("toast never auto-dismissed")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestToastMissedWhileIdleIsFlagged(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{ToastDuration: 30 * time.Millisecond, IdleAfter: 20 * time.Millisecond})
	d.SetPresence(&fakePresence{idle: true})
	res := callNotify(t, d, notifyItem(proto.LevelInfo))
	waitForDismiss(t, ui)

	recent, err := st.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if recent[0].ID != res.ID || recent[0].State != store.StateDismissed {
		t.Fatalf("stored state = %s, want dismissed", recent[0].State)
	}
	if !recent[0].MissedWhileAway {
		t.Fatal("toast that lapsed while idle was not flagged missed-while-away")
	}
}

func TestToastSeenWhilePresentIsNotFlagged(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{ToastDuration: 30 * time.Millisecond, IdleAfter: 20 * time.Millisecond})
	d.SetPresence(&fakePresence{idle: false})
	callNotify(t, d, notifyItem(proto.LevelInfo))
	waitForDismiss(t, ui)

	recent, err := st.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if recent[0].MissedWhileAway {
		t.Fatal("toast dismissed while present must not be flagged missed-while-away")
	}
}

func TestMissedMarkerOffWhenIdleAfterZero(t *testing.T) {
	// IdleAfter 0 disables the marker even with an idle desktop (FR44 reuses
	// presence.idle_after_s; a zero window opts out).
	d, ui, _, st := newTestDaemon(t, Config{ToastDuration: 30 * time.Millisecond})
	d.SetPresence(&fakePresence{idle: true})
	callNotify(t, d, notifyItem(proto.LevelInfo))
	waitForDismiss(t, ui)

	recent, err := st.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if recent[0].MissedWhileAway {
		t.Fatal("missed marker must stay off when idle_after_s is 0")
	}
}

func TestIdleHoldsInitialChimeThenSummaryOnReturn(t *testing.T) {
	// FR29 hold_when_idle: a card still shows while the user is idle, but its
	// chime holds; returning plays one summary chime, not a backlog.
	d, ui, snd, _ := newTestDaemon(t, Config{HoldWhenIdle: true, IdleAfter: 10 * time.Millisecond})
	fp := &fakePresence{idle: true}
	d.SetPresence(fp)

	callNotify(t, d, notifyItem(proto.LevelWarning)) // a warning notify stays on screen
	if ui.last().Item == nil {
		t.Fatal("the card should show while idle; only the chime holds")
	}
	if snd.count() != 0 {
		t.Fatalf("idle must hold the chime, played %d", snd.count())
	}

	fp.set(false, false, false)
	d.PresencePoll()
	if snd.count() != 1 {
		t.Fatalf("returning from idle should play one summary chime, played %d", snd.count())
	}
}

func TestIdlePausesEscalationThenResumes(t *testing.T) {
	// FR29: escalation pauses while idle (no metronome in an empty room) and
	// picks up the cadence once the user is back.
	d, ui, snd, _ := newTestDaemon(t, Config{
		HoldWhenIdle:       true,
		IdleAfter:          10 * time.Millisecond,
		EscalationInterval: 20 * time.Millisecond,
		EscalationCount:    100,
	})
	fp := &fakePresence{idle: true}
	d.SetPresence(fp)

	ch := askAsync(t, d, askItem())
	it := waitForItem(t, ui)
	time.Sleep(120 * time.Millisecond) // several escalation intervals, all idle
	if n := snd.count(); n != 0 {
		t.Fatalf("escalation should stay silent while idle, played %d", n)
	}

	fp.set(false, false, false)
	deadline := time.Now().Add(2 * time.Second)
	for snd.count() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("escalation never resumed after the user returned")
		}
		time.Sleep(10 * time.Millisecond)
	}
	d.Answer(it.ID, "Yes")
	<-ch
}

func TestFullscreenHoldsCardAndRevealsOnExit(t *testing.T) {
	// FR29 fullscreen_auto_dnd: a focused fullscreen app holds new cards like
	// DND; leaving fullscreen reveals the held card with one chime.
	d, ui, snd, _ := newTestDaemon(t, Config{FullscreenAutoDnd: true})
	fp := &fakePresence{fullscreen: true}
	d.SetPresence(fp)

	callNotify(t, d, notifyItem(proto.LevelWarning))
	if v := ui.last(); v.Item != nil {
		t.Fatalf("a focused fullscreen app must hold the card, shown: %+v", v.Item)
	}
	if snd.count() != 0 {
		t.Fatalf("fullscreen auto-DND must stay silent, played %d", snd.count())
	}

	fp.set(false, false, false)
	d.PresencePoll()
	if ui.last().Item == nil {
		t.Fatal("leaving fullscreen should reveal the held card")
	}
	if snd.count() != 1 {
		t.Fatalf("the reveal should chime once, played %d", snd.count())
	}
}

func TestDesktopDndHoldsButUrgentBreaksThrough(t *testing.T) {
	// FR29 respect_desktop_dnd: the desktop's own DND counts as DND, with the
	// same break-through rule - urgent still pierces (default on).
	d, ui, snd, _ := newTestDaemon(t, Config{RespectDesktopDnd: true})
	d.SetPresence(&fakePresence{desktopDnd: true})

	callNotify(t, d, notifyItem(proto.LevelWarning))
	if ui.last().Item != nil {
		t.Fatal("desktop DND must hold a normal card")
	}

	urgent := askItem()
	urgent.Level = proto.LevelUrgent
	ch := askAsync(t, d, urgent)
	it := waitForItem(t, ui)
	if snd.count() == 0 {
		t.Fatal("urgent must sound through desktop DND")
	}
	d.Answer(it.ID, "Yes")
	<-ch
}

func TestProgressLifecycle(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	res, rpcErr := d.Progress(context.Background(), proto.ProgressUpdate{
		Title: "Migrating", Identity: proto.Identity{Agent: "claude-code"},
	})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	id := res.ID
	if id == "" {
		t.Fatal("create returned no progress id")
	}
	if got := ui.lastProgress(); len(got) != 1 || got[0].Title != "Migrating" {
		t.Fatalf("report not shown: %+v", got)
	}

	// Update: percent applies and clamps.
	if _, rpcErr := d.Progress(context.Background(), proto.ProgressUpdate{ID: id, Percent: 150, Status: "halfway"}); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if got := ui.lastProgress(); len(got) != 1 || got[0].Percent != 100 || got[0].Status != "halfway" {
		t.Fatalf("update not applied/clamped: %+v", got)
	}

	// Done clears the bar and surfaces a success toast.
	if _, rpcErr := d.Progress(context.Background(), proto.ProgressUpdate{ID: id, Done: true}); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if got := ui.lastProgress(); len(got) != 0 {
		t.Fatalf("done did not clear the report: %+v", got)
	}
	v := ui.last()
	if v.Item == nil || v.Item.Kind != proto.KindNotify || v.Item.Level != proto.LevelSuccess {
		t.Fatalf("done should surface a success toast, got %+v", v.Item)
	}
	if !strings.Contains(v.Item.Title, "complete") {
		t.Fatalf("success title = %q, want it to mention complete", v.Item.Title)
	}
}

func TestProgressDoneWithErrorSurfacesErrorToast(t *testing.T) {
	d, ui, _, _ := newTestDaemon(t, Config{})
	res, _ := d.Progress(context.Background(), proto.ProgressUpdate{Title: "Deploy", Identity: proto.Identity{Agent: "claude-code"}})
	d.Progress(context.Background(), proto.ProgressUpdate{ID: res.ID, Done: true, Error: "exit 1"})
	v := ui.last()
	if v.Item == nil || v.Item.Level != proto.LevelError || !strings.Contains(v.Item.Title, "failed") {
		t.Fatalf("done+error should surface an error toast, got %+v", v.Item)
	}
	if v.Item.Body != "exit 1" {
		t.Fatalf("error body = %q, want the error message", v.Item.Body)
	}
}

func TestProgressUnknownIDIsRejected(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	_, rpcErr := d.Progress(context.Background(), proto.ProgressUpdate{ID: "kbogus", Percent: 10})
	if rpcErr == nil || rpcErr.Code != proto.CodeItemNotFound {
		t.Fatalf("updating an unknown report should be item-not-found, got %v", rpcErr)
	}
}

func TestProgressCallerGoneReapsAndWarns(t *testing.T) {
	// FR21 robustness: a held report (the CLI pipe) is reaped when its
	// connection drops before Done, and an "interrupted" toast records why.
	d, ui, _, _ := newTestDaemon(t, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	if _, rpcErr := d.Progress(ctx, proto.ProgressUpdate{
		Title: "Indexing", Hold: true, Indeterminate: true, Identity: proto.Identity{Agent: "claude-code"},
	}); rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if len(ui.lastProgress()) != 1 {
		t.Fatal("held report not shown")
	}
	cancel() // the reporting process died

	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(ui.lastProgress()) == 0 && ui.last().Item != nil && ui.last().Item.Level == proto.LevelWarning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("held report not reaped after caller gone; progress=%v last=%+v", ui.lastProgress(), ui.last().Item)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(ui.last().Item.Title, "interrupted") {
		t.Fatalf("interrupted toast title = %q", ui.last().Item.Title)
	}
}

func TestProgressShutdownDoesNotWarn(t *testing.T) {
	// A held report whose connection drops because the daemon is shutting down
	// (FR45 teardown) must not surface an "interrupted" toast.
	d, ui, _, _ := newTestDaemon(t, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	d.Progress(ctx, proto.ProgressUpdate{Title: "Indexing", Hold: true, Indeterminate: true, Identity: proto.Identity{Agent: "claude-code"}})
	d.BeginShutdown()
	cancel()
	time.Sleep(50 * time.Millisecond)
	if v := ui.last(); v.Item != nil {
		t.Fatalf("teardown should not surface a toast, got %+v", v.Item)
	}
}

func TestProgressStaleReaped(t *testing.T) {
	// FR21 backstop: a non-held report untouched past the stale window (an MCP
	// agent that died without Done) is reaped by the poll tick.
	d, ui, _, _ := newTestDaemon(t, Config{})
	res, _ := d.Progress(context.Background(), proto.ProgressUpdate{Title: "Slow", Identity: proto.Identity{Agent: "claude-code"}})
	d.mu.Lock()
	d.progress[res.ID].updated = time.Now().Add(-staleProgressAfter - time.Minute)
	d.mu.Unlock()
	d.ReapStaleProgress()
	if got := ui.lastProgress(); len(got) != 0 {
		t.Fatalf("stale report not reaped: %+v", got)
	}
}

func TestMuteListAndUnmute(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	muteRPC(t, d, "b", false)
	muteRPC(t, d, "a", false)
	got := muteRPC(t, d, "", false) // bare list
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("muted list = %v, want [a b] sorted", got)
	}
	got = muteRPC(t, d, "a", true)
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("after unmute = %v, want [b]", got)
	}
}

// askAsyncCtx submits a blocking item on a cancelable context, so a test can
// simulate the caller's socket dropping (FR45) by cancelling it.
func askAsyncCtx(t *testing.T, d *Daemon, it proto.Item, ctx context.Context) chan proto.Result {
	t.Helper()
	ch := make(chan proto.Result, 1)
	go func() {
		res, rpcErr := d.Handle(ctx, proto.MethodAsk, mustJSON(t, it))
		if rpcErr != nil {
			ch <- proto.Result{}
			return
		}
		ch <- res.(proto.Result)
	}()
	return ch
}

// waitForState polls until the stored item reaches state, or fails.
func waitForState(t *testing.T, st *store.Store, id, state string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		recent, err := st.Recent(20)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range recent {
			if it.ID == id && it.State == state {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("item %s never reached %s", id, state)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestCallerLiveThenGoneAutoDismisses(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{CallerGone: 40 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	ch := askAsyncCtx(t, d, askItem(), ctx)
	shown := waitForItem(t, ui)

	if c := ui.last().Caller; c != CallerLive {
		t.Fatalf("caller state while connected = %v, want live", c)
	}

	cancel() // the caller's socket drops
	<-ch     // handleSubmit returns once it sees the disconnect

	waitForState(t, st, shown.ID, store.StateCancelled)
	if !ui.sawCaller(CallerGone) {
		t.Fatal("a dropped caller was never shown as disconnected")
	}
}

func TestGoneCardDismissesOnKey(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{CallerGone: 10 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	ch := askAsyncCtx(t, d, askItem(), ctx)
	shown := waitForItem(t, ui)
	cancel()
	<-ch

	// The auto-dismiss window is long; a key (Dismiss) must clear it at once.
	deadline := time.Now().Add(2 * time.Second)
	for !ui.sawCaller(CallerGone) {
		if time.Now().After(deadline) {
			t.Fatal("never marked caller-gone")
		}
		time.Sleep(5 * time.Millisecond)
	}
	d.Dismiss(shown.ID)
	waitForState(t, st, shown.ID, store.StateDismissed)
}

func TestRestoredItemAwaitsReconnect(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "agentbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	it := &proto.Item{ID: "k1", Kind: proto.KindChoice, Title: "Deploy?",
		Options: []proto.Option{{Label: "Yes"}, {Label: "No"}}, Identity: proto.Identity{Agent: "a"}}
	if err := st.CreateItem(it); err != nil {
		t.Fatal(err)
	}
	ui := &fakeUI{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := New(Config{}, log, st, &fakeSound{}, ui); err != nil {
		t.Fatal(err)
	}
	if c := ui.last().Caller; c != CallerAwaiting {
		t.Fatalf("restored item caller state = %v, want awaiting-reconnect", c)
	}
}

func TestShutdownDisconnectIsNotCallerGone(t *testing.T) {
	d, ui, _, st := newTestDaemon(t, Config{CallerGone: time.Second})
	d.BeginShutdown()
	ctx, cancel := context.WithCancel(context.Background())
	ch := askAsyncCtx(t, d, askItem(), ctx)
	shown := waitForItem(t, ui)
	cancel()
	<-ch

	waitForState(t, st, shown.ID, store.StateCancelled)
	if ui.sawCaller(CallerGone) {
		t.Fatal("daemon teardown must not present a card as caller-disconnected")
	}
}

// A deploy replaces the binary and restarts; the only way to know the restart
// took is to ask the daemon which build it is. `make deploy` fails on this, so
// the field has to be there and it has to be the SERVING build - not whatever
// binary happened to open the socket.
func TestStatusReportsTheServingBuild(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})

	res, rpcErr := d.Handle(context.Background(), proto.MethodStatus, nil)
	if rpcErr != nil {
		t.Fatalf("status: %v", rpcErr)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("status returned %T, want a map", res)
	}
	if _, ok := m["pending"]; !ok {
		t.Error("status lost its pending count")
	}
	raw, ok := m["version"]
	if !ok {
		t.Fatal("status reports no version, so `make deploy` cannot tell a restarted daemon from a stale one")
	}
	info, ok := raw.(version.Info)
	if !ok {
		t.Fatalf("version is %T, want version.Info", raw)
	}
	if info != version.Get() {
		t.Errorf("version = %v, want this build's %v", info, version.Get())
	}
	// It has to survive the wire, because that is the only place it is read.
	var back version.Info
	if err := json.Unmarshal(mustJSON(t, raw), &back); err != nil {
		t.Fatalf("version does not round-trip as JSON: %v", err)
	}
	if back != info {
		t.Errorf("over the wire = %+v, want %+v", back, info)
	}
}

// fakeDriver stands in for the desktop: it records what it was asked to do and
// never touches an X server, which is the whole point of Driver being an
// interface (this package must stay testable without a display).
type fakeDriver struct {
	scripts []string
	speed   float64
	wpm     int
	err     error
}

func (f *fakeDriver) Drive(script string, speed float64, wpm int) (int, error) {
	f.scripts = append(f.scripts, script)
	f.speed, f.wpm = speed, wpm
	if f.err != nil {
		return 0, f.err
	}
	return len(strings.Split(strings.TrimSpace(script), "\n")), nil
}

func TestSpeakReturnsWithoutWaitingForTheVoice(t *testing.T) {
	// The ordinary spoken line: it reaches the voice and the call comes straight
	// back. A held sound would hold the caller too if this ever went through the
	// waiting path by accident.
	d, _, snd, _ := newTestDaemon(t, Config{})
	snd.hold = make(chan struct{})
	defer close(snd.hold)

	res, rpcErr := d.Handle(context.Background(), proto.MethodSpeak,
		mustJSON(t, proto.SpeakRequest{Text: "the build is green"}))
	if rpcErr != nil {
		t.Fatalf("speak: %v", rpcErr)
	}
	got, ok := res.(proto.SpeakResult)
	if !ok || !got.OK || got.Waited {
		t.Fatalf("speak returned %#v, want ok without waited", res)
	}
	if lines := snd.spokenLines(); len(lines) != 1 || lines[0] != "the build is green" {
		t.Errorf("the voice received %q", lines)
	}
	if waited := snd.waitedLines(); len(waited) != 0 {
		t.Errorf("an ordinary speak waited on %q", waited)
	}
}

func TestSpeakWithWaitAnswersWhenTheLineHasBeenHeard(t *testing.T) {
	// What a narrated sequence rests on: the answer arrives after the sound, so the
	// next line can start on the last word of this one.
	d, _, snd, _ := newTestDaemon(t, Config{})
	snd.hold = make(chan struct{})

	type answer struct {
		res    any
		rpcErr *proto.RPCError
	}
	answered := make(chan answer, 1)
	go func() {
		res, rpcErr := d.Handle(context.Background(), proto.MethodSpeak,
			mustJSON(t, proto.SpeakRequest{Text: "eighteen acts, and this is the last", Wait: true}))
		answered <- answer{res, rpcErr}
	}()

	waitFor(t, "the line reached the voice", func() bool { return len(snd.waitedLines()) == 1 })
	select {
	case a := <-answered:
		t.Fatalf("speak --wait answered %#v while the line was still playing", a.res)
	case <-time.After(150 * time.Millisecond):
	}

	close(snd.hold) // the sound has finished
	select {
	case a := <-answered:
		if a.rpcErr != nil {
			t.Fatalf("speak --wait: %v", a.rpcErr)
		}
		got, ok := a.res.(proto.SpeakResult)
		if !ok || !got.OK || !got.Waited {
			t.Fatalf("speak --wait returned %#v, want ok and waited", a.res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("speak --wait never answered after the sound finished")
	}
}

func TestSpeakWithWaitGivesUpWhenTheCallerDoes(t *testing.T) {
	// A client that disconnects or times out must not leave the handler holding a
	// connection for the length of a sentence it no longer cares about.
	d, _, snd, _ := newTestDaemon(t, Config{})
	snd.hold = make(chan struct{})
	defer close(snd.hold)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Handle(ctx, proto.MethodSpeak, mustJSON(t, proto.SpeakRequest{Text: "never mind", Wait: true}))
		close(done)
	}()
	waitFor(t, "the line reached the voice", func() bool { return len(snd.waitedLines()) == 1 })
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a cancelled speak --wait kept waiting")
	}
}

func TestDriveRunsTheScriptAndReportsItsSteps(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	drv := &fakeDriver{}
	d.SetDriver(drv)

	res, rpcErr := d.Handle(context.Background(), proto.MethodDrive,
		mustJSON(t, proto.DriveRequest{Script: "window =agentbox\nclick 25% -46", Speed: 2, WPM: 200}))
	if rpcErr != nil {
		t.Fatalf("drive: %v", rpcErr)
	}
	got, ok := res.(proto.DriveResult)
	if !ok || got.Steps != 2 {
		t.Fatalf("drive returned %#v, want 2 steps", res)
	}
	if len(drv.scripts) != 1 || !strings.Contains(drv.scripts[0], "click 25% -46") {
		t.Errorf("the driver received %q", drv.scripts)
	}
	if drv.speed != 2 || drv.wpm != 200 {
		t.Errorf("pacing arrived as speed %v wpm %d", drv.speed, drv.wpm)
	}
}

func TestDriveWithoutADisplaySaysSoInsteadOfPanicking(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{}) // no SetDriver: a headless daemon
	_, rpcErr := d.Handle(context.Background(), proto.MethodDrive,
		mustJSON(t, proto.DriveRequest{Script: "click"}))
	if rpcErr == nil {
		t.Fatal("a daemon with no driver accepted a drive")
	}
	if !strings.Contains(rpcErr.Message, "display") {
		t.Errorf("the error does not mention the missing display: %v", rpcErr)
	}
}

func TestDriveRefusesAnEmptyScriptAndReportsAFailure(t *testing.T) {
	d, _, _, _ := newTestDaemon(t, Config{})
	d.SetDriver(&fakeDriver{})
	if _, rpcErr := d.Handle(context.Background(), proto.MethodDrive,
		mustJSON(t, proto.DriveRequest{Script: "   "})); rpcErr == nil {
		t.Error("an empty script was accepted")
	}

	d.SetDriver(&fakeDriver{err: errors.New("line 2 (click sideways): not a button")})
	_, rpcErr := d.Handle(context.Background(), proto.MethodDrive,
		mustJSON(t, proto.DriveRequest{Script: "move 1 1\nclick sideways"}))
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "not a button") {
		t.Errorf("the driver's own error did not reach the caller: %v", rpcErr)
	}
}

// The log has to be keepable. A script can type a password, so what gets recorded
// is the sequence of step names and nothing else.
func TestDriveShapeRecordsTheStepsAndNotWhatWasTyped(t *testing.T) {
	shape := driveShape("# open the vault\nwindow =Vault\nclick center center\ntype hunter2 correct-horse\nkey Return\n")
	if want := "window,click,type,key"; shape != want {
		t.Errorf("driveShape = %q, want %q", shape, want)
	}
	if strings.Contains(shape, "hunter2") || strings.Contains(shape, "Vault") {
		t.Fatalf("the shape leaked script content: %q", shape)
	}
	long := strings.Repeat("click\n", 100)
	if got := driveShape(long); !strings.HasSuffix(got, "...") {
		t.Errorf("a hundred steps were not truncated: %q", got[max(0, len(got)-30):])
	}
}
