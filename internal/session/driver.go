package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/logging"
)

// DefaultBin is the Claude Code executable agentbox spawns; resolved on PATH.
const DefaultBin = "claude"

// Config configures a session driver. Zero values are sensible: Bin defaults
// to "claude", Mode to "plan" (read-only).
type Config struct {
	Bin  string // executable name or path; "" -> DefaultBin
	Dir  string // working directory for the child; "" -> daemon's cwd
	Mode string // agentbox's mode: "plan" or "full"; "" -> "plan". See permissionMode.
	// Model is passed to --model. Empty means whatever `claude` defaults to,
	// which is the right answer for an interactive session; an assignment (M12)
	// pins one, because "check my usage" running on a different model each week
	// is a different assignment each week.
	Model   string
	Env     []string // extra environment entries (AGENTBOX_INSTANCE, AGENTBOX_SESSION_ID)
	Partial bool     // pass --include-partial-messages (live token deltas)

	// Brief is appended to the child's system prompt. agentbox uses it to tell a
	// session it is running inside agentbox and how to interrupt well
	// (manual.Session): a session that does not know it can reach the human
	// behaves like any other headless agent, which is the thing agentbox exists to
	// fix. Empty passes no flag at all, so a caller that wants a plain child gets
	// one.
	Brief string

	// Resume is a Claude session id to carry on from (--resume). It is what makes
	// two things possible that a fresh child cannot do: switching a session
	// between plan and full, which is a spawn-time flag and therefore a new child,
	// and reopening a conversation after a restart with its context intact rather
	// than only its transcript on screen.
	Resume string

	// MCPConfig, when set, is passed via --mcp-config so the child can reach
	// agentbox's own MCP server; AllowedTools pre-approves tools (Phase 7).
	MCPConfig    string
	AllowedTools []string

	// SendTimeout bounds how long Send waits for the child to take a prompt
	// (R-25). It is a bound on a FREEZE and not on delivery: Send is called from
	// the goroutine that draws, so what this number buys is the length of the
	// worst pause the window can suffer when a child has stopped reading its
	// stdin. Zero means the default below.
	SendTimeout time.Duration
}

// defaultSendTimeout is generous for the case it has to allow - a prompt larger
// than the 64 kB pipe buffer, handed to a child that is reading it - and short
// enough that the window is not visibly stuck when the child is not.
const defaultSendTimeout = 5 * time.Second

// Driver owns a headless `claude` child and the goroutines that pump its
// stream-json I/O. The conversation it builds is read with Turns()/State().
type Driver struct {
	cfg  Config
	log  *slog.Logger
	conv *conversation

	mu       sync.Mutex
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	started  bool
	stopping bool

	// writeMu serialises the prompts going into the child. It is not d.mu and
	// must never be taken with it: a write can block for as long as the child
	// declines to read, and d.mu is taken by everything else here.
	writeMu sync.Mutex
}

// New builds a driver. onUpdate is called (off the frame goroutine) whenever
// the conversation changes, so the owner can wake the UI; a nil logger is
// tolerated.
func New(cfg Config, log *slog.Logger, onUpdate func()) *Driver {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.Bin == "" {
		cfg.Bin = DefaultBin
	}
	if cfg.Mode == "" {
		cfg.Mode = "plan"
	}
	if cfg.SendTimeout <= 0 {
		cfg.SendTimeout = defaultSendTimeout
	}
	return &Driver{cfg: cfg, log: log, conv: &conversation{onUpdate: onUpdate}}
}

// Seed puts turns into a fresh driver's conversation before it is started, so a
// reopened session shows what was said last time and a mode switch keeps its
// transcript across the new child. Only meaningful before Start.
func (d *Driver) Seed(turns []Turn) {
	d.conv.mu.Lock()
	d.conv.turns = append(d.conv.turns, turns...)
	d.conv.mu.Unlock()
	d.conv.notify()
}

func (d *Driver) Turns() []Turn     { return d.conv.snapshot() }
func (d *Driver) State() State      { _, _, s, _ := d.conv.info(); return s }
func (d *Driver) SessionID() string { id, _, _, _ := d.conv.info(); return id }
func (d *Driver) Model() string     { _, m, _, _ := d.conv.info(); return m }
func (d *Driver) LastError() string { _, _, _, e := d.conv.info(); return e }

// args builds the claude invocation. Verified against v2.1.177: -p with
// stream-json in/out requires --verbose.
// permissionMode translates agentbox's two-word vocabulary into what Claude Code's
// --permission-mode actually accepts.
//
// "full" was passed through verbatim, and it is not one of the choices, so the
// child exited 1 the moment anybody chose it: "Session ended unexpectedly: exit
// status 1", which is what Boris hit the first time Full became the default. It had
// been unreachable in the surface until then, so nothing had ever tried it.
//
// Full means bypassPermissions rather than acceptEdits on purpose. agentbox does not
// speak the stream-json permission protocol yet, so a mode that asks would stall
// invisibly: the child waits for an answer to a prompt nothing renders. The choice
// is between a session that can work and one that hangs.
func permissionMode(mode string) string {
	if mode == "full" {
		return "bypassPermissions"
	}
	return "plan"
}

func (d *Driver) args() []string {
	a := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", permissionMode(d.cfg.Mode),
	}
	if d.cfg.Partial {
		a = append(a, "--include-partial-messages")
	}
	if m := strings.TrimSpace(d.cfg.Model); m != "" {
		a = append(a, "--model", m)
	}
	if strings.TrimSpace(d.cfg.Brief) != "" {
		a = append(a, "--append-system-prompt", d.cfg.Brief)
	}
	if strings.TrimSpace(d.cfg.Resume) != "" {
		a = append(a, "--resume", d.cfg.Resume)
	}
	if d.cfg.MCPConfig != "" {
		a = append(a, "--mcp-config", d.cfg.MCPConfig)
	}
	if len(d.cfg.AllowedTools) > 0 {
		a = append(a, "--allowed-tools", strings.Join(d.cfg.AllowedTools, ","))
	}
	return a
}

// Start spawns the child and launches the reader and stderr-drain goroutines.
// A missing executable or pipe failure returns an error and leaves the driver
// in the error state.
func (d *Driver) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return nil
	}
	path, err := exec.LookPath(d.cfg.Bin)
	if err != nil {
		d.conv.setState(StateError)
		d.conv.append(systemErrorTurn("Cannot start a session: %q not found on PATH. Install Claude Code or set its path.", d.cfg.Bin))
		return fmt.Errorf("session: %w", err)
	}
	cmd := exec.Command(path, d.args()...)
	cmd.Dir = d.cfg.Dir
	cmd.Env = append(os.Environ(), d.cfg.Env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return d.startFailed(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return d.startFailed(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return d.startFailed(err)
	}
	if err := cmd.Start(); err != nil {
		return d.startFailed(err)
	}
	d.cmd = cmd
	d.stdin = stdin
	d.started = true
	d.log.Info("session.started", "component", "session", "bin", path, "mode", d.cfg.Mode)

	go d.drainStderr(stderr)
	go d.readLoop(stdout)
	return nil
}

func (d *Driver) startFailed(err error) error {
	d.conv.setState(StateError)
	d.conv.append(systemErrorTurn("Cannot start a session: %s", err.Error()))
	d.log.Error("session.start_failed", "component", "session", "err", err.Error())
	return fmt.Errorf("session: %w", err)
}

// readLoop consumes the child's stdout until EOF, then reaps it and records the
// final state. All heavy parsing happens inside conv.consume, on this
// goroutine - never the frame goroutine.
func (d *Driver) readLoop(stdout io.ReadCloser) {
	defer func() {
		if r := recover(); r != nil {
			d.log.Error(logging.EvPanic, "component", "session", "goroutine", "read", "panic", fmt.Sprint(r))
		}
	}()
	d.conv.consume(stdout)

	err := d.cmd.Wait()
	d.mu.Lock()
	stopping := d.stopping
	d.mu.Unlock()
	if err != nil && !stopping {
		d.conv.setState(StateError)
		d.conv.append(systemErrorTurn("Session ended unexpectedly: %s", err.Error()))
		d.log.Error("session.exited", "component", "session", "err", err.Error())
		return
	}
	d.conv.setState(StateEnded)
	d.log.Info("session.ended", "component", "session")
}

// drainStderr keeps the child's stderr pipe flowing (a full pipe would stall
// it) and logs anything it writes - spawn diagnostics, the auth wrapper.
func (d *Driver) drainStderr(stderr io.ReadCloser) {
	defer func() {
		if r := recover(); r != nil {
			d.log.Error(logging.EvPanic, "component", "session", "goroutine", "stderr", "panic", fmt.Sprint(r))
		}
	}()
	br := bufio.NewReader(stderr)
	for {
		line, err := br.ReadString('\n')
		if s := strings.TrimSpace(line); s != "" {
			d.log.Warn("session.stderr", "component", "session", "line", s)
		}
		if err != nil {
			return
		}
	}
}

// Send writes a user prompt to the child and records it in the conversation.
// It is safe to call from the UI goroutine, and R-25 is what that sentence used
// to be worth: the write went straight out with no deadline, so a child that had
// stopped reading its stdin - or a prompt bigger than the 64 kB pipe buffer -
// froze the goroutine that draws, for as long as the session lived.
//
// The write now happens on a goroutine of its own and this waits out
// SendTimeout for it. The ordinary case is unchanged, because a small prompt
// into a pipe with room in it lands in microseconds; what changes is the worst
// case, which is now a bounded pause and a sentence in the conversation instead
// of a window that never comes back.
//
// The user's turn is recorded by the writer, once the bytes are gone. It used to
// be recorded before the write, so a prompt the child never received sat in the
// transcript as though it had been sent - and the transcript said "working"
// about a turn that was never delivered.
func (d *Driver) Send(prompt string) error {
	prompt = strings.TrimRight(prompt, "\n")
	if strings.TrimSpace(prompt) == "" {
		return nil
	}
	d.mu.Lock()
	stdin := d.stdin
	started := d.started
	d.mu.Unlock()
	if !started || stdin == nil {
		return fmt.Errorf("session: not started")
	}
	line, err := encodeUserMessage(prompt)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		// One prompt at a time: two writers into one pipe would interleave their
		// JSON lines, and a caller whose predecessor is stuck parks here rather
		// than corrupting the stream behind it.
		d.writeMu.Lock()
		defer d.writeMu.Unlock()
		_, err := stdin.Write(line)
		if err == nil {
			// Late is possible and it is still the truth: a write that lands after
			// this call gave up records its turn then, below the line saying it had
			// not arrived, in the order things actually happened.
			d.conv.addUserPrompt(prompt)
		}
		done <- err
	}()

	timer := time.NewTimer(d.cfg.SendTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			d.conv.setState(StateError)
			d.conv.append(systemErrorTurn("That prompt did not reach the session: %s", err.Error()))
			d.log.Error("session.send_failed", "component", "session", "err", err.Error())
			return err
		}
		return nil
	case <-timer.C:
		// Not StateError: the child may still be working and may still take this
		// prompt. What is certain is that it has not taken it yet, and saying so
		// is the difference between a session that looks slow and one that looks
		// answered.
		d.conv.append(systemErrorTurn(
			"The session has not taken that prompt after %s - it is not reading its input. Nothing was sent.",
			d.cfg.SendTimeout))
		d.log.Error("session.send_stalled", "component", "session",
			"waited_s", d.cfg.SendTimeout.Seconds(), "bytes", len(line))
		return fmt.Errorf("session: the child has not read its input for %s", d.cfg.SendTimeout)
	}
}

// Stop closes the child's stdin so it finishes the current turn and exits
// cleanly; the read loop reaps it. Safe to call more than once.
func (d *Driver) Stop() {
	d.mu.Lock()
	if !d.started || d.stopping {
		d.mu.Unlock()
		return
	}
	d.stopping = true
	stdin := d.stdin
	d.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
}

// Kill terminates the child immediately (window close / shutdown). Stop is the
// graceful path; Kill is the backstop.
func (d *Driver) Kill() {
	d.Stop()
	d.mu.Lock()
	cmd := d.cmd
	d.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// encodeUserMessage builds one stream-json input line for a prompt.
func encodeUserMessage(prompt string) ([]byte, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type envelope struct {
		Type    string `json:"type"`
		Message msg    `json:"message"`
	}
	b, err := json.Marshal(envelope{Type: "user", Message: msg{Role: "user", Content: prompt}})
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
