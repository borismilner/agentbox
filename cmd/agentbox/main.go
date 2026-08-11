// agentbox is a desktop interaction hub for AI agents: notifications,
// blocking questions and sounds that cannot be missed the way a terminal
// can. See docs/ or `agentbox help`.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/borismilner/agentbox/internal/client"
	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/hand"
	"github.com/borismilner/agentbox/internal/manual"
	agentboxmcp "github.com/borismilner/agentbox/internal/mcp"
	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/server"
	"github.com/borismilner/agentbox/internal/version"
)

// Exit codes are a stable contract (FR41).
const (
	exitOK         = 0
	exitNo         = 1
	exitUsage      = 2
	exitUnanswered = 3
	exitError      = 4
)

const usageText = `agentbox - desktop interaction hub for AI agents

Usage: agentbox <command> [flags]

Commands:
  daemon    run the resident daemon (auto-spawned by other commands)
  notify    fire-and-forget event       agentbox notify --level warning --title "Disk almost full"
  ask       blocking choice             agentbox ask --title "Deploy?" --option "Yes" --option "No"
  input     blocking free text          agentbox input --title "Tag name?" [--multiline]
  confirm   blocking yes/no             agentbox confirm --title "Push to main?"
  veto      act unless stopped          agentbox veto --in 15 --title "Pushing to main"
  secret    masked secret input         agentbox secret --title "API token" --to-file ./token
  review    approve/reject a diff       agentbox review --title "Apply patch?" --diff-file changes.diff
  progress  live progress bar from stdin  long_task | agentbox progress --title "Migrating"
  form      several fields, one card    agentbox form --title "Release" --field choice:env:staging,prod --field text:tag
  quit      stop the daemon gracefully
  status    daemon liveness and pending count
  app       open the tabbed app window
            agentbox app [--tab home|session|agents|assignments|inbox|history|viewer|library|settings]
  inbox     open the inbox window (pending + recent history)
  show      render a markdown file in the viewer  agentbox show --watch README.md
            or run interactive HTML in a sandbox  agentbox show --artifact app.html
  artifact  hear what the human did in an artifact  agentbox artifact wait --id a1b2
  walkthrough  durable step-by-step reviews on the board (FR58)
            agentbox walkthrough create --spec review.json | open [ID] | list | read ID [--ack]
                                | await [ID] | repair [ID] | delete ID
  panel     roll the session panel down/up  agentbox panel [show|hide|toggle|state]
  say       read a line out loud       agentbox say "the build is green"   (or pipe it)
            --wait returns when it has been heard, for a narrated sequence
  drive     move the pointer, click and type as a person would
                                       agentbox drive --window agentbox click 25% -46
  control   take the desktop before driving it, and give it back
            agentbox control request "reason" | activity "line" | release | state
            pause | resume are the human's, and nothing an agent calls resumes a pause
            quiet | loud demote the hands-off strip to 4 pixels while recording
  sync      say what this session is for, and see who else is here
                                       agentbox sync announce "porting the settings surface"
                                       agentbox sync agents
            announce | activity | agents | peers | attach   presence
            lock | unlock | locks | break                   named leases
            post | await                                    signals
            get | set | del                                 shared values
  summon    raise + focus the current card (bind to a desktop shortcut)
  stats     interruption insights       agentbox stats [--since 7d]
  pending   what is still waiting for you, with ids
  dismiss   clear pending items without the mouse
                                       agentbox dismiss --all   (or dismiss ID...)
  dnd       do-not-disturb              agentbox dnd on|off|status
  mute      silence an agent now        agentbox mute claude-code   (agentbox mute --list)
  unmute    let an agent through again  agentbox unmute claude-code
  logs      print the daemon event log (--follow to tail)
  docs      embedded manual            agentbox docs agent
  schema    JSON Schema for the wire protocol and item kinds
  mcp       MCP stdio server for agents (register in .mcp.json)
  version   build provenance (git commit, build time)
  webui-demo  open any surface on canned data, with no daemon and nothing stored
            agentbox webui-demo [cards|notify|viewer|progress|ask|app|artifact|panel|board|agents]

Blocking commands print the answer on stdout.
Exit codes: 0 answered/yes/proceeded, 1 no/vetoed, 2 usage error, 3 unanswered/timeout, 4 error.
Common flags: --agent NAME --project NAME --session ID --json
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(exitUsage)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "daemon":
		runDaemon()
	case "notify":
		os.Exit(runSubmit(proto.KindNotify, args))
	case "ask":
		os.Exit(runSubmit(proto.KindChoice, args))
	case "input":
		os.Exit(runSubmit(proto.KindText, args))
	case "confirm":
		os.Exit(runSubmit(proto.KindConfirm, args))
	case "veto":
		os.Exit(runVeto(args))
	case "secret":
		os.Exit(runSecret(args))
	case "review":
		os.Exit(runReview(args))
	case "progress":
		os.Exit(runProgress(args))
	case "form":
		os.Exit(runForm(args))
	case "quit":
		os.Exit(runQuit(args))
	case "status":
		os.Exit(runStatus(args))
	case "inbox":
		os.Exit(runInbox(args))
	case "app":
		os.Exit(runApp(args))
	case "show":
		os.Exit(runShow(args))
	case "panel":
		os.Exit(runPanel(args))
	case "artifact":
		os.Exit(runArtifact(args))
	case "walkthrough":
		os.Exit(runWalkthrough(args))
	case "say":
		os.Exit(runSay(args))
	case "drive":
		os.Exit(runDrive(args))
	case "control":
		os.Exit(runControl(args))
	case "sync":
		os.Exit(runSync(args))
	case "summon":
		os.Exit(runSummon(args))
	case "stats":
		os.Exit(runStats(args))
	case "dismiss":
		os.Exit(runDismiss(args))
	case "pending":
		os.Exit(runPending(args))
	case "dnd":
		os.Exit(runDnd(args))
	case "mute":
		os.Exit(runMute(args))
	case "unmute":
		os.Exit(runUnmute(args))
	case "webui-demo":
		runWebUIDemo(args)
	case "logs":
		os.Exit(runLogs(args))
	case "docs":
		os.Exit(runDocs(args))
	case "schema":
		fmt.Println(manual.Schema())
	case "mcp":
		os.Exit(runMCP(args))
	case "version":
		fmt.Println(version.Get().String())
	case "help", "-h", "--help":
		fmt.Print(usageText)
	default:
		fmt.Fprintf(os.Stderr, "agentbox: unknown command %q\n\n%s", cmd, usageText)
		os.Exit(exitUsage)
	}
}

func stateDir() string {
	name := "agentbox"
	if inst := os.Getenv("AGENTBOX_INSTANCE"); inst != "" {
		// A named instance owns its state too (NFR12), not just its socket.
		name = "agentbox-" + inst
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, name)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", name)
}

func runtimeDir() string {
	return server.RuntimeDir(os.Getenv("AGENTBOX_INSTANCE"))
}

// identityFlags wires the common caller-identity flags with discoverable
// defaults: the parent process name and the working directory.
func identityFlags(fs *flag.FlagSet) *proto.Identity {
	id := &proto.Identity{Via: proto.ViaCLI}
	fs.StringVar(&id.Agent, "agent", agentName(), "calling agent name")
	fs.StringVar(&id.Project, "project", projectName(), "project the agent works on")
	fs.StringVar(&id.Session, "session", "", "agent session id")
	// A CLI call is not a session and has no key of its own (FR83). It can act
	// on behalf of one, which is what a hook script wrapping an agent's work
	// does; the default works that out from the environment and the process tree
	// so no recipe has to pass a flag. See inheritedSessionKey.
	fs.StringVar(&id.Key, "key", inheritedSessionKey(), "session key to act on behalf of")
	return id
}

// sessionKey is the identity of one session (FR83). AgentBox's own session tab
// already hands the child an id, and reusing it keeps one name for one session
// across both doors; failing that the session is named after the process that
// runs it, and only failing THAT does the child mint a key nobody else can guess.
func sessionKey() string {
	if k := inheritedSessionKey(); k != "" {
		return k
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not short-read on Linux. If it ever does, the clock
		// keeps two sessions apart rather than colliding both on zeros.
		binary.BigEndian.PutUint64(b[:], uint64(time.Now().UnixNano()))
	}
	return hex.EncodeToString(b[:])
}

// inheritedSessionKey is the key of the session this process belongs to, or ""
// if it belongs to none. It is what a hook needs and what a random key cannot be:
// the hook and the model's own mcp child have to arrive at the SAME answer or the
// board shows one session as two rows, and neither can pass the other a secret.
//
// So the answer is a fact both can look up - the agent process (see
// agentProcess) - rather than something either one invents. The recipe used to
// say "export a random key", which cannot work: a hook runs inside an
// environment Claude Code has already built, so there is no moment left in which
// to export anything. Claude's own session id cannot bridge them either, because
// the mcp child keeps the id it was spawned with while a /clear gives the session
// a new one.
//
// Unlike sessionKey this can come back empty, and the difference matters: a CLI
// call that belongs to no session must say so and be refused, where minting a
// random key would put a phantom row on the board for one invocation.
func inheritedSessionKey() string {
	if k := os.Getenv("AGENTBOX_SESSION_KEY"); k != "" {
		return k
	}
	if s := os.Getenv("AGENTBOX_SESSION_ID"); s != "" {
		return s
	}
	if pid, _, ok := agentProcess(); ok {
		if key, err := procSessionKeyFor(pid); err == nil {
			return key
		}
	}
	return ""
}

// procSessionKeyFor names a session after the process that runs it.
//
// Readable on purpose: this key shows up in the daemon's log and in `sync locks`,
// and "proc-361992-..." is a thing a human can check with ps where eight random
// bytes are a thing they can only compare. The start time rides along because
// pids are recycled - see procStartTime.
func procSessionKeyFor(pid int) (string, error) {
	start, err := procStartTime(pid)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("proc-%d-%d", pid, start), nil
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// projectName is what to say the caller works on: the repo it is in, not the
// subdirectory it stands in (FR86). Deliberately the daemon's own derivation, so
// a row and the area heading above it cannot name two different places.
func projectName() string { return daemon.DeriveProject(mustGetwd()) }

// agentName is who to say is calling. The parent process alone is not an answer:
// a hook runs as claude -> sh -> agentbox and would be called "sh", and the
// prescribed `setsid agentbox sync attach` is reparented to init and was called
// "systemd" on Boris's board for every hook-driven session.
//
// So: an explicit AGENTBOX_AGENT wins, because it is the only thing that survives
// setsid cutting the process tree. Failing that, walk up the tree past the shells
// and the wrappers to the first name that means something. Failing that, say
// "agent" - honest, and a placeholder a later call is allowed to replace, which
// "systemd" would not have been.
func agentName() string {
	if v := strings.TrimSpace(os.Getenv("AGENTBOX_AGENT")); v != "" {
		return v
	}
	if _, comm, ok := agentProcess(); ok {
		return comm
	}
	return "agent"
}

// agentProcess is the process the caller belongs to: the nearest ancestor whose
// name is an agent rather than a shell, a wrapper or a terminal.
//
// Both doors into one session land on the same process, which is the whole point.
// An mcp child's parent IS the agent, and it already reports that pid as its own
// (internal/mcp/sync.go). A hook reaches the same pid by stepping over the shell
// that ran it. So the two can agree on who they are without either being told.
//
// It answers false rather than guessing when the walk finds nothing: under setsid
// the tree is cut at init, and a caller with no agent above it is a caller that
// belongs to no session.
func agentProcess() (pid int, comm string, ok bool) {
	return agentProcessFrom(os.Getppid())
}

// agentProcessFrom is agentProcess's walk, from a given starting pid so a test
// can run it over a tree it built rather than over its own.
func agentProcessFrom(from int) (pid int, comm string, ok bool) {
	// Bounded, because a walk up /proc is a loop over data the kernel can change
	// underneath it, and no real tree is this deep.
	pid = from
	for range 12 {
		if pid <= 1 {
			return 0, "", false
		}
		name, ppid, err := procParent(pid)
		if err != nil {
			return 0, "", false
		}
		if !proto.PlaceholderAgent(name) {
			return pid, name, true
		}
		pid = ppid
	}
	return 0, "", false
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(s string) error {
	*m = append(*m, s)
	return nil
}

func runSubmit(kind proto.Kind, args []string) int {
	name := map[proto.Kind]string{
		proto.KindNotify: "notify", proto.KindChoice: "ask",
		proto.KindText: "input", proto.KindConfirm: "confirm",
	}[kind]
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	id := identityFlags(fs)
	title := fs.String("title", "", "card title (required)")
	body := fs.String("body", "", "card body, markdown")
	speak := fs.String("speak", "", "a line read out loud when the card appears (needs [speech] enabled)")
	level := fs.String("level", "", "info|success|warning|error|urgent")
	timeout := fs.Int("timeout", 0, "seconds until the default applies (blocking commands)")
	deflt := fs.String("default", "", "answer applied on timeout")
	asJSON := fs.Bool("json", false, "print the full result object")
	var options, actions multiFlag
	var strict, multiline *bool
	if kind == proto.KindChoice {
		fs.Var(&options, "option", "a choice, repeat 2-9 times; \"Label\" or \"Label::description\"")
	}
	if kind == proto.KindNotify {
		fs.Var(&actions, "action", `a button "Label::command" run on click, repeat up to 3 times (FR32)`)
	}
	if kind == proto.KindChoice || kind == proto.KindConfirm {
		strict = fs.Bool("strict", false, "disable the free-text reply hatch")
	}
	if kind == proto.KindText {
		multiline = fs.Bool("multiline", false, "multi-line editor, Ctrl+Enter sends")
	}
	fs.Parse(args)

	if *title == "" {
		fmt.Fprintf(os.Stderr, "agentbox %s: --title is required\nexample: %s\n", name, exampleFor(name))
		return exitUsage
	}
	it := proto.Item{
		Kind: kind, Level: proto.Level(*level), Title: *title, Body: *body, Speak: *speak,
		TimeoutS: *timeout, Default: *deflt, Identity: *id,
	}
	if strict != nil {
		it.Strict = *strict
	}
	if multiline != nil {
		it.Multiline = *multiline
	}
	for _, o := range options {
		label, desc, _ := strings.Cut(o, "::")
		it.Options = append(it.Options, proto.Option{Label: label, Desc: desc})
	}
	for _, a := range actions {
		label, cmd, ok := strings.Cut(a, "::")
		if !ok || cmd == "" {
			fmt.Fprintf(os.Stderr, "agentbox %s: --action wants \"Label::command\", got %q\n", name, a)
			return exitUsage
		}
		it.Actions = append(it.Actions, proto.Action{Label: label, Exec: cmd})
	}
	if len(it.Actions) > 0 {
		it.Cwd = mustGetwd() // where the daemon runs an action's command (FR32)
	}
	if err := it.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox %s: %v\nexample: %s\n", name, err, exampleFor(name))
		return exitUsage
	}

	method := proto.MethodAsk
	if kind == proto.KindNotify {
		method = proto.MethodNotify
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()

	var res proto.Result
	if err := conn.Call(context.Background(), method, &it, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	if *asJSON {
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
	}
	if kind == proto.KindNotify {
		return exitOK
	}
	if !*asJSON {
		switch {
		case res.Reply != "":
			fmt.Println(res.Reply)
		case res.Answer != "":
			fmt.Println(res.Answer)
		}
	}
	switch {
	case res.Answered && kind == proto.KindConfirm && res.Answer == "no":
		return exitNo
	case res.Answered:
		return exitOK
	default:
		return exitUnanswered
	}
}

// runProgress drives a live progress card (FR21) from stdin. Each non-empty
// line either starts with a number ("47" or "47 migrating users" or "47%") to
// set the percent and an optional status, or is taken as a status line on its
// own. The card holds the last known state, so a status-only line keeps the
// bar where it was. EOF finishes it as success; Ctrl-C/TERM finishes it as
// interrupted; an outright kill is reaped by the daemon (the report is held to
// this connection). Example: `long_task | agentbox progress --title "Migrating"`.
func runProgress(args []string) int {
	fs := flag.NewFlagSet("progress", flag.ExitOnError)
	id := identityFlags(fs)
	title := fs.String("title", "", "progress card title (required)")
	indeterminate := fs.Bool("indeterminate", false, "start as a spinner (unknown fraction) until a percent line arrives")
	fs.Parse(args)
	if *title == "" {
		fmt.Fprintln(os.Stderr, "agentbox progress: --title is required")
		fmt.Fprintln(os.Stderr, `example: long_task | agentbox progress --title "Migrating"`)
		return exitUsage
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()

	ctx := context.Background()
	// Hold ties the report to this connection: if we die without sending done,
	// the daemon reaps the bar and records why (FR21 robustness).
	var created proto.ProgressResult
	if err := conn.Call(ctx, proto.MethodProgress, &proto.ProgressUpdate{
		Title: *title, Indeterminate: *indeterminate, Hold: true, Identity: *id,
	}, &created); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		// Best-effort clean finish; the daemon would reap it anyway on the
		// dropped connection.
		_ = conn.Call(context.Background(), proto.MethodProgress,
			&proto.ProgressUpdate{ID: created.ID, Done: true, Error: "interrupted"}, nil)
		os.Exit(exitError)
	}()

	// The CLI carries the full known state so a partial line never resets the
	// bar (the daemon overwrites with each update).
	cur := proto.ProgressUpdate{ID: created.ID, Indeterminate: *indeterminate}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if pct, status, ok := parseProgressLine(line); ok {
			cur.Percent = pct
			cur.Indeterminate = false
			if status != "" {
				cur.Status = status
			}
		} else {
			cur.Status = line
		}
		if err := conn.Call(ctx, proto.MethodProgress, &cur, nil); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: progress update failed: %v\n", err)
			return exitError
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: reading stdin: %v\n", err)
		_ = conn.Call(ctx, proto.MethodProgress, &proto.ProgressUpdate{ID: created.ID, Done: true, Error: "input error: " + err.Error()}, nil)
		return exitError
	}
	if err := conn.Call(ctx, proto.MethodProgress, &proto.ProgressUpdate{ID: created.ID, Done: true}, nil); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	return exitOK
}

// parseProgressLine reads a leading percent off a progress line: "47",
// "47%", or "47 migrating users" (the rest becomes the status). It reports
// ok=false when the line does not start with a number.
func parseProgressLine(line string) (percent int, status string, ok bool) {
	tok := line
	if i := strings.IndexAny(line, " \t"); i >= 0 {
		tok, status = line[:i], strings.TrimSpace(line[i+1:])
	}
	n, err := strconv.Atoi(strings.TrimSuffix(tok, "%"))
	if err != nil {
		return 0, "", false
	}
	if n < 0 {
		n = 0
	} else if n > 100 {
		n = 100
	}
	return n, status, true
}

func exampleFor(name string) string {
	switch name {
	case "notify":
		return `agentbox notify --level success --title "Tests passed" --body "412 tests, 0 failures"`
	case "ask":
		return `agentbox ask --title "Deploy?" --option "Now" --option "Skip::wait for the next train" --timeout 300 --default "Skip"`
	case "input":
		return `agentbox input --title "Release tag?"`
	case "veto":
		return `agentbox veto --in 15 --title "Pushing to main" --body "3 commits, CI green"`
	default:
		return `agentbox confirm --title "Push to main?"`
	}
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "machine-readable status")
	fs.Parse(args)

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: daemon unreachable: %v\n", err)
		return exitError
	}
	defer conn.Close()
	var res map[string]any
	if err := conn.Call(dialCtx, proto.MethodStatus, nil, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	// The daemon reports its own build; this binary reports itself separately.
	// They differ exactly when a new binary is installed and the old daemon is
	// still serving, which is the one failure `make deploy` has to catch - so it
	// is said out loud rather than papered over with the client's own version.
	daemonVer := daemonVersion(res)
	res["version"] = daemonVer
	res["client_version"] = version.Get().String()
	if *asJSON {
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
		return exitOK
	}
	fmt.Printf("daemon running, %v pending\n%s\n", res["pending"], daemonVer)
	if mine := version.Get().String(); daemonVer != mine && daemonVer != "" {
		fmt.Printf("this client is %s - restart the daemon to run the new build\n", mine)
	}
	return exitOK
}

// daemonVersion pulls the running daemon's build out of a status reply. A daemon
// too old to send one says so rather than borrowing this binary's answer, which
// is what the printed line used to do.
func daemonVersion(res map[string]any) string {
	raw, ok := res["version"]
	if !ok {
		return "an older build that does not report its version"
	}
	var info version.Info
	blob, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(blob, &info) != nil {
		return "an older build that does not report its version"
	}
	return info.String()
}

// parseFieldSpec turns "type:key[:opt1,opt2][=default]" into a Field.
func parseFieldSpec(spec string) (proto.Field, error) {
	var f proto.Field
	spec, deflt, _ := strings.Cut(spec, "=")
	parts := strings.SplitN(spec, ":", 3)
	if len(parts) < 2 {
		return f, fmt.Errorf("field %q: want type:key[:options][=default], e.g. choice:env:staging,prod=staging", spec)
	}
	f.Type = proto.FieldType(parts[0])
	f.Key = parts[1]
	f.Default = deflt
	if len(parts) == 3 {
		f.Options = strings.Split(parts[2], ",")
	}
	switch f.Type {
	case proto.FieldChoice, proto.FieldText, proto.FieldBool:
	default:
		return f, fmt.Errorf("field %q: unknown type %q (choice|text|bool)", spec, parts[0])
	}
	return f, nil
}

func runForm(args []string) int {
	fs := flag.NewFlagSet("form", flag.ExitOnError)
	id := identityFlags(fs)
	title := fs.String("title", "", "card title (required)")
	body := fs.String("body", "", "card body")
	speak := fs.String("speak", "", "a line read out loud when the card appears (needs [speech] enabled)")
	timeout := fs.Int("timeout", 0, "seconds until the form expires unanswered")
	// --json is documented as a common flag on every command, and form rejecting
	// it was a usage error waiting for whoever wrote the obvious script. A form's
	// answer is a map of fields, so the object IS the bare answer here: the flag
	// is accepted for symmetry and changes nothing about the output.
	fs.Bool("json", true, "print the full result object (a form always does; accepted for symmetry)")
	var fields multiFlag
	fs.Var(&fields, "field", "type:key[:opt1,opt2][=default], repeat 1-6 times")
	fs.Parse(args)

	it := proto.Item{Kind: proto.KindForm, Title: *title, Body: *body, Speak: *speak, TimeoutS: *timeout, Identity: *id}
	for _, spec := range fields {
		f, err := parseFieldSpec(spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox form: %v\n", err)
			return exitUsage
		}
		it.Fields = append(it.Fields, f)
	}
	if err := it.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox form: %v\nexample: agentbox form --title \"Release\" --field choice:env:staging,prod --field text:tag --field bool:notify=yes\n", err)
		return exitUsage
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()
	var res proto.Result
	if err := conn.Call(context.Background(), proto.MethodAsk, &it, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	out, _ := json.Marshal(res)
	fmt.Println(string(out))
	if !res.Answered {
		return exitUnanswered
	}
	return exitOK
}

// runVeto announces an action with a countdown and proceeds unless stopped
// (FR22). Exit 0 = proceeded, 1 = vetoed, in line with confirm's yes/no.
func runVeto(args []string) int {
	fs := flag.NewFlagSet("veto", flag.ExitOnError)
	id := identityFlags(fs)
	title := fs.String("title", "", "what is about to happen (required)")
	body := fs.String("body", "", "card body, markdown")
	speak := fs.String("speak", "", "a line read out loud when the card appears (needs [speech] enabled)")
	in := fs.Int("in", 0, "countdown seconds before the action proceeds (default: server's [veto] window)")
	level := fs.String("level", "warning", "info|success|warning|error|urgent")
	asJSON := fs.Bool("json", false, "print the full result object")
	fs.Parse(args)

	if *title == "" {
		fmt.Fprintf(os.Stderr, "agentbox veto: --title is required\nexample: %s\n", exampleFor("veto"))
		return exitUsage
	}
	it := proto.Item{
		Kind: proto.KindVeto, Level: proto.Level(*level), Title: *title, Body: *body, Speak: *speak,
		TimeoutS: *in, Identity: *id,
	}
	// A zero --in defers to the daemon's configured window; only a negative
	// value is a usage error here (the daemon fills and validates the rest).
	if *in < 0 {
		fmt.Fprintf(os.Stderr, "agentbox veto: --in must be >= 0\nexample: %s\n", exampleFor("veto"))
		return exitUsage
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()

	var res proto.Result
	if err := conn.Call(context.Background(), proto.MethodAsk, &it, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	if *asJSON {
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
	} else if res.Vetoed {
		fmt.Println("vetoed")
	} else {
		fmt.Println("proceeding")
	}
	if res.Vetoed {
		return exitNo
	}
	return exitOK
}

// runSecret prompts for a masked value (FR23). By default the daemon writes
// it to --to-file (mode 0600) and it never touches stdout; --stdout opts the
// value into stdout (and shows a warning on the card). The value is never
// logged or stored.
func runSecret(args []string) int {
	fs := flag.NewFlagSet("secret", flag.ExitOnError)
	id := identityFlags(fs)
	title := fs.String("title", "", "what secret to request (required)")
	body := fs.String("body", "", "card body, markdown")
	speak := fs.String("speak", "", "a line read out loud when the card appears (needs [speech] enabled)")
	toFile := fs.String("to-file", "", "write the value to this file (mode 0600)")
	stdout := fs.Bool("stdout", false, "also return the value on stdout (enters the transcript; warns on the card)")
	timeout := fs.Int("timeout", 0, "seconds until the prompt expires unanswered")
	asJSON := fs.Bool("json", false, "print the full result object")
	fs.Parse(args)

	if *title == "" {
		fmt.Fprintf(os.Stderr, "agentbox secret: --title is required\nexample: agentbox secret --title \"npm token\" --to-file ./token\n")
		return exitUsage
	}
	if *toFile == "" && !*stdout {
		fmt.Fprintf(os.Stderr, "agentbox secret: pass --to-file PATH or --stdout (where should the value go?)\n")
		return exitUsage
	}
	// The daemon writes the file; resolve the path against the caller's cwd
	// so a relative --to-file does not land in the daemon's directory.
	sink := *toFile
	if sink != "" {
		if abs, err := filepath.Abs(sink); err == nil {
			sink = abs
		}
	}
	it := proto.Item{
		Kind: proto.KindSecret, Title: *title, Body: *body, Speak: *speak,
		Sink: sink, Stdout: *stdout, TimeoutS: *timeout, Identity: *id,
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()
	var res proto.Result
	if err := conn.Call(context.Background(), proto.MethodAsk, &it, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	if *asJSON {
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
	}
	if !res.Answered {
		if !*asJSON {
			fmt.Fprintln(os.Stderr, "agentbox secret: no value provided")
		}
		return exitUnanswered
	}
	if !*asJSON {
		if *stdout {
			fmt.Println(res.Secret)
		} else {
			fmt.Printf("secret written to %s\n", res.SecretPath)
		}
	}
	return exitOK
}

func runQuit(args []string) int {
	noFlags("quit", "Stop the daemon gracefully, ending its windows and sessions.", args)
	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), func() error {
		return fmt.Errorf("no daemon running")
	})
	if err != nil {
		fmt.Println("agentbox daemon not running")
		return exitOK
	}
	defer conn.Close()
	if err := conn.Call(dialCtx, proto.MethodQuit, struct{}{}, nil); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	fmt.Println("agentbox daemon stopped")
	return exitOK
}

func runDnd(args []string) int {
	var set *bool
	if len(args) > 0 {
		switch args[0] {
		case "on":
			v := true
			set = &v
		case "off":
			v := false
			set = &v
		case "status":
		default:
			fmt.Fprintf(os.Stderr, "agentbox dnd: want on, off or status, got %q\n", args[0])
			return exitUsage
		}
	}
	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()
	var res struct {
		Enabled  bool   `json:"enabled"`
		AutoHeld bool   `json:"auto_held"`
		Reason   string `json:"reason"`
	}
	if err := conn.Call(dialCtx, proto.MethodDnd, map[string]any{"set": set}, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	if res.Enabled {
		fmt.Println("do not disturb: on")
	} else {
		fmt.Println("do not disturb: off")
	}
	// The switch is not the whole answer (FR29): a focused fullscreen window or the
	// desktop's own do-not-disturb hold interruptions with agentbox's switch off, and
	// a status that stopped at "off" while nothing reached the screen was
	// indistinguishable from a broken install.
	if res.AutoHeld {
		fmt.Printf("  interruptions are held anyway: %s\n", res.Reason)
	}
	return exitOK
}

// runMute silences an agent at runtime (FR47): `agentbox mute <agent>` adds it,
// `agentbox mute --list` prints the muted set. Ephemeral - cleared on restart.
func runMute(args []string) int {
	fs := flag.NewFlagSet("mute", flag.ExitOnError)
	list := fs.Bool("list", false, "print the currently muted agents")
	asJSON := fs.Bool("json", false, "machine-readable output")
	agent := first(parsePositional(fs, args))
	if !*list && agent == "" {
		fmt.Fprintf(os.Stderr, "agentbox mute: name an agent to silence, or --list\nexample: agentbox mute claude-code\n")
		return exitUsage
	}
	return muteCall(agent, false, *asJSON)
}

// runUnmute lifts a runtime mute (FR47); held items surface again.
func runUnmute(args []string) int {
	fs := flag.NewFlagSet("unmute", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	agent := first(parsePositional(fs, args))
	if agent == "" {
		fmt.Fprintf(os.Stderr, "agentbox unmute: name the agent to unmute\nexample: agentbox unmute claude-code\n")
		return exitUsage
	}
	return muteCall(agent, true, *asJSON)
}

// muteCall sends one mute/unmute (or a bare list) and prints the resulting
// muted set. An empty agent just queries.
func muteCall(agent string, unmute, asJSON bool) int {
	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()
	var res struct {
		Muted []string `json:"muted"`
	}
	params := map[string]any{"agent": agent, "unmute": unmute}
	if err := conn.Call(dialCtx, proto.MethodMute, params, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	if asJSON {
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
		return exitOK
	}
	if agent != "" {
		if unmute {
			fmt.Printf("unmuted %s\n", agent)
		} else {
			fmt.Printf("muted %s\n", agent)
		}
	}
	if len(res.Muted) == 0 {
		fmt.Println("no agents muted")
	} else {
		fmt.Printf("muted: %s\n", strings.Join(res.Muted, ", "))
	}
	return exitOK
}

// runDrive moves the pointer, clicks and types as the person at the keyboard
// would (internal/hand). It talks to the X server itself rather than to the
// daemon: this is the one agentbox command that does not need a daemon at all, and
// keeping it in-process is what lets a shell script drive a window on a machine
// where nothing else of agentbox is running. The MCP tool takes the other route,
// through the daemon, so an agent's synthetic input lands in agentbox's own log.
func runDrive(args []string) int {
	fs := flag.NewFlagSet("drive", flag.ExitOnError)
	window := fs.String("window", "", "make coordinates relative to the window whose title contains this")
	speed := fs.Float64("speed", 1, "movement speed multiplier (2 = twice as fast as a hand)")
	wpm := fs.Int("wpm", 0, "typing speed in words per minute (default 300)")
	seed := fs.Int64("seed", 0, "fix the randomness, so a demo traces the same path twice")
	verbose := fs.Bool("verbose", false, "print each step as it runs")
	fs.Parse(args)

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, driveUsage)
		return exitUsage
	}

	h, err := hand.Open(*seed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox drive: %v\n", err)
		return exitError
	}
	defer h.Close()
	h.SetSpeed(*speed)
	h.SetWPM(*wpm)
	if *verbose {
		h.Trace = func(line string) { fmt.Fprintln(os.Stderr, line) }
	}

	// `where` answers the question every other step depends on: where is that
	// window? It prints "x y w h", so a shell can read it into four variables.
	if rest[0] == "where" {
		r := h.Screen()
		if title := strings.Join(rest[1:], " "); title != "" {
			if r, err = h.Look(title); err != nil {
				fmt.Fprintf(os.Stderr, "agentbox drive: %v\n", err)
				return exitError
			}
		}
		fmt.Println(r)
		return exitOK
	}

	var src string
	switch {
	case rest[0] == "run" && len(rest) == 2 && rest[1] == "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox drive: reading the script: %v\n", err)
			return exitError
		}
		src = string(data)
	case rest[0] == "run" && len(rest) == 2:
		data, err := os.ReadFile(rest[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox drive: %v\n", err)
			return exitError
		}
		src = string(data)
	case rest[0] == "run":
		fmt.Fprintf(os.Stderr, "agentbox drive run wants one file (or - for stdin)\n")
		return exitUsage
	default:
		// Everything else is one step, spelled the way a script line is, so
		// there is one grammar to learn rather than two.
		src = strings.Join(rest, " ")
	}

	steps, err := hand.ParseScript(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox drive: %v\n", err)
		return exitUsage
	}
	if *window != "" {
		if _, err := h.UseWindow(*window); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox drive: %v\n", err)
			return exitError
		}
	}
	// No latch on this path: `agentbox drive` is the human driving his own
	// desktop from his own shell, and pausing himself is not a thing to protect
	// against. The daemon's drive is the one an agent reaches, and that one parks.
	if _, err := h.Run(steps); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox drive: %v\n", err)
		return exitError
	}
	return exitOK
}

const driveUsage = `agentbox drive - move the pointer, click and type as a person would

  agentbox drive move 960 540              one step, the same spelling a script uses
  agentbox drive click 25% -46             25% across, 46px up from the frame's bottom
  agentbox drive type Hello there
  agentbox drive key ctrl+alt+t
  agentbox drive where "agentbox"             print a window's x y w h
  agentbox drive run tour.txt              a whole script (- reads stdin)

Coordinates: 400 from the near edge, -46 from the far edge, 60% across,
center, ~ where the pointer is, ~+30 relative to it.
Steps: window TITLE, screen, move, click, double, drag, scroll, type, key,
wait MS, speed N, wpm N. One per line; # comments; type takes the rest verbatim.

window TITLE locks onto that window: it is raised, followed if it moves, and
every click and keystroke is checked against it before it is sent. A step that
would land somewhere else fails, naming what was there. screen gives the lock
up. --verbose prints what each event actually went into.
Flags: --window TITLE --speed N --wpm N --seed N --verbose
`

// runSummon raises and focuses the current card (FR15). No daemon means no
// card to summon, so it does not auto-spawn one.
func runSay(args []string) int {
	fs := flag.NewFlagSet("say", flag.ExitOnError)
	wait := fs.Bool("wait", false, "return when the line has been heard, not when it has been queued")
	timeout := fs.Float64("timeout", 120, "seconds to wait with --wait before giving up")
	fs.Parse(args)

	// A line of prose, from the arguments or from a pipe, so `... | agentbox say`
	// works the way a shell user expects.
	text := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(text) == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox say: %v\n", err)
			return exitError
		}
		text = string(data)
	}
	if strings.TrimSpace(text) == "" {
		fmt.Fprintf(os.Stderr, "agentbox say: nothing to say\nexample: agentbox say \"the build is green\"\n")
		return exitUsage
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), func() error {
		return fmt.Errorf("no daemon running")
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox say: no daemon running\n")
		return exitError
	}
	defer conn.Close()

	// The dial has a short deadline because a daemon either answers its socket or
	// does not. Waiting for a sentence to finish is a different clock, so --wait
	// gets its own: the 5s dial budget would cut off the very thing it asked for.
	callCtx := dialCtx
	if *wait {
		var cancelCall context.CancelFunc
		callCtx, cancelCall = context.WithTimeout(context.Background(),
			time.Duration(*timeout*float64(time.Second)))
		defer cancelCall()
	}
	req := proto.SpeakRequest{Text: text, Wait: *wait}
	if err := conn.Call(callCtx, proto.MethodSpeak, &req, nil); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox say: %v\n", err)
		return exitError
	}
	return exitOK
}

// partitionControlFlags splits `control`'s command line into its flag tokens and
// its positional words, so a flag may appear anywhere.
//
// Scoped to this command on purpose. The lookahead below assumes every flag TAKES
// A VALUE, which is true of control's one flag (--window) and false of a boolean
// like --wait, where it would swallow the next positional. Do not lift this to a
// command with boolean flags without teaching it which is which.
func partitionControlFlags(args []string) (flags, words []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			words = append(words, a)
			continue
		}
		flags = append(flags, a)
		if strings.Contains(a, "=") {
			continue
		}
		// --name value: take the next token too, unless it is itself a flag or
		// there is nothing left, in which case flag.Parse reports it properly.
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return flags, words
}

// waitingSuffix names who is parked behind a latch, when anyone is. A pause with
// an agent waiting and a pause on an idle desktop are different situations and
// the line has to say which (FR94).
func waitingSuffix(res proto.ControlResult) string {
	if res.HeldBy == "" {
		return ""
	}
	return fmt.Sprintf(" · %s is parked, waiting for you", res.HeldBy)
}

// quietSuffix says recording mode is on wherever the state is printed (FR95). It
// rides on every line rather than replacing one, because quiet is orthogonal to
// what the run is doing: a demoted sign over a live run and a demoted sign over an
// empty desktop are both worth knowing, and neither is a state of the run.
func quietSuffix(res proto.ControlResult) string {
	if !res.Quiet {
		return ""
	}
	line := fmt.Sprintf(" · quiet: the sign is demoted for %s more",
		(time.Duration(res.QuietLeftS) * time.Second).Round(time.Minute))
	if held := heldPhrase(res.QuietHeld); held != "" {
		line += ", " + held
	}
	return line
}

// heldPhrase says what going loud is about to put on screen. Worth knowing before
// he stops recording rather than after: five cards arriving at once is a surprise,
// and one of them may be a question an agent has been parked on for ten minutes.
func heldPhrase(n int) string {
	switch {
	case n <= 0:
		return ""
	case n == 1:
		return "1 card waiting"
	default:
		return fmt.Sprintf("%d cards waiting", n)
	}
}

// runControl is the desktop handover from a shell (FR74). It exists beside the MCP
// tools because not every agent speaks MCP - Boris asked for this to work "while
// they drive my debug chrome or other automations", and a shell script is the
// lowest common denominator those reach for.
//
//	agentbox control request "clicking through the review board" --window 20
//	agentbox control activity "reading the takeaway aloud"
//	agentbox control release
//
// And the human's own two (FR94), which no agent may call:
//
//	agentbox control pause      # take the keyboard back; the run keeps its place
//	agentbox control resume     # hand it on
//	agentbox control quiet      # demote the sign to 4px for a recording (FR95)
//	agentbox control loud       # put the strip back
//
// request exits 0 when granted and 3 when denied or held by somebody else, so a
// script can gate on it: `agentbox control request "..." || exit`.
func runControl(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "agentbox control: want request|activity|release|state|pause|resume|quiet|loud")
		return exitError
	}
	verb, rest := args[0], args[1:]

	fs := flag.NewFlagSet("control "+verb, flag.ExitOnError)
	window := fs.Int("window", 20, "seconds before silence counts as consent (request only)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: agentbox control request REASON [--window N] | activity LINE | release | state | pause | resume | quiet | loud")
		fmt.Fprintln(os.Stderr, "\nOne strip on screen while an agent has the desktop. While it is up, hands off;")
		fmt.Fprintln(os.Stderr, "when it goes, the desktop is the human's again.")
		fs.PrintDefaults()
	}
	// Flags first is a rule of Go's flag package, not of the world: it stops
	// parsing at the first non-flag argument, so `control request "reason" --window
	// 12` silently kept the default and the countdown ran for 20 seconds while the
	// caller had asked for 12 - with "--window 12" tacked onto the reason the human
	// reads. An agent writes the reason first because that is the natural order, so
	// the flags are lifted out before parsing rather than being a trap.
	flags, words := partitionControlFlags(rest)
	if err := fs.Parse(flags); err != nil {
		return exitError
	}
	text := strings.TrimSpace(strings.Join(words, " "))

	action := ""
	switch verb {
	case "request":
		action = proto.ControlRequest
		if text == "" {
			fmt.Fprintln(os.Stderr, "agentbox control request: wants a reason - it is what the human reads before allowing it")
			return exitError
		}
	case "activity":
		action = proto.ControlActivity
		if text == "" {
			fmt.Fprintln(os.Stderr, "agentbox control activity: wants a line saying what is happening now")
			return exitError
		}
	case "release":
		action = proto.ControlRelease
	case "state":
		action = proto.ControlState
	case "pause":
		// The human's verb, not an agent's (FR94). It is here so the hotkey has
		// something to call and so a compositor binding works on a desktop where
		// no X11 grab is possible - the same reason `agentbox panel` exists beside
		// the drop-down's own key.
		action = proto.ControlPause
		if text == "" {
			text = "the command line"
		}
	case "resume":
		action = proto.ControlResume
		if text == "" {
			text = "the command line"
		}
	case "quiet", "loud":
		// Recording mode (FR95). Here rather than only on a hotkey because the
		// natural place to arm it is the line above `obs`, in whatever script starts
		// the recording - the same argument that put `control request` in a shell.
		action = proto.ControlQuiet
		if verb == "loud" {
			action = proto.ControlLoud
		}
		if text == "" {
			text = "the command line"
		}
	default:
		fmt.Fprintf(os.Stderr, "agentbox control: unknown verb %q\n", verb)
		return exitError
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), func() error {
		return fmt.Errorf("no daemon running")
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentbox control: no daemon running")
		return exitError
	}
	defer conn.Close()

	// request blocks for the length of the countdown plus however long the human
	// takes, so it gets its own clock: the 5s dial budget would cut off the very
	// thing it asked for.
	callCtx := dialCtx
	if action == proto.ControlRequest {
		var cancelCall context.CancelFunc
		callCtx, cancelCall = context.WithTimeout(context.Background(),
			time.Duration(*window)*time.Second+2*time.Minute)
		defer cancelCall()
	}

	req := proto.ControlRequestParams{
		Action: action,
		Identity: proto.Identity{
			Agent: agentName(), Project: projectName(),
			Session: os.Getenv("AGENTBOX_SESSION_ID"), Key: os.Getenv("AGENTBOX_SESSION_KEY"),
			Via: proto.ViaCLI,
		},
		Reason:  text,
		WindowS: *window,
	}
	if action == proto.ControlActivity {
		req.Reason, req.Activity = "", text
	}
	var res proto.ControlResult
	if err := conn.Call(callCtx, proto.MethodControl, &req, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox control: %v\n", err)
		return exitError
	}

	// The exit codes are the ones already in the contract (FR41): 1 is a no from
	// the human, 3 is nobody answered - and "somebody else is holding it" is the
	// second of those, because no question was ever put.
	// `state` is a question, so it never takes the held-by branch below: HeldBy is
	// always set while a run is live, and short-circuiting on it meant state could
	// only ever answer "held by ..." even to the agent doing the holding.
	if action == proto.ControlPause || action == proto.ControlResume {
		if res.Paused {
			fmt.Printf("paused: the desktop is yours%s\n", waitingSuffix(res))
		} else {
			fmt.Println("resumed: agents may drive again")
		}
		return exitOK
	}

	if action == proto.ControlQuiet || action == proto.ControlLoud {
		if res.Quiet {
			// The fuse is printed, not implied. A mode that expires and does not say
			// when is a mode he has to remember, which is the thing it exists to
			// avoid.
			// The column comes down with the sign, so say so here too: an agent
			// that notifies during the recording will not put anything on screen,
			// and a card that was up when he armed it is already in the count.
			held := ""
			if p := heldPhrase(res.QuietHeld); p != "" {
				held = ", " + p
			}
			fmt.Printf("quiet: the sign is four pixels on the top edge, a window can cover it, and cards wait instead of appearing%s (loud again in %s)\n",
				held, (time.Duration(res.QuietLeftS) * time.Second).Round(time.Minute))
		} else {
			fmt.Println("loud: the hands-off strip is back on top of everything, and anything that queued is on screen now")
		}
		return exitOK
	}

	if action == proto.ControlState {
		switch {
		case res.Paused:
			fmt.Printf("paused: the desktop is yours%s%s\n", waitingSuffix(res), quietSuffix(res))
		case !res.Live:
			fmt.Printf("no run: the desktop is the human's%s\n", quietSuffix(res))
		case res.Activity != "":
			fmt.Printf("%s · %s · %s%s\n", res.State, res.HeldBy, res.Activity, quietSuffix(res))
		default:
			fmt.Printf("%s · %s · %s%s\n", res.State, res.HeldBy, res.Reason, quietSuffix(res))
		}
		return exitOK
	}

	switch {
	case res.HeldBy != "":
		fmt.Printf("held by %s: %s\n", res.HeldBy, res.Reason)
		return exitUnanswered
	case action == proto.ControlRequest && res.Denied:
		fmt.Println("denied - the desktop is not yours")
		return exitNo
	case action == proto.ControlRequest && res.Granted:
		fmt.Println("granted - hands off is on screen until you release it")
	case action == proto.ControlRequest:
		fmt.Println("not granted (the caller went away)")
		return exitUnanswered
	}
	return exitOK
}

func runSummon(args []string) int {
	noFlags("summon", "Give the current card the keyboard focus it never takes itself.", args)
	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), func() error {
		return fmt.Errorf("no daemon running")
	})
	if err != nil {
		fmt.Println("agentbox: nothing to summon (no daemon running)")
		return exitOK
	}
	defer conn.Close()
	if err := conn.Call(dialCtx, proto.MethodSummon, struct{}{}, nil); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	return exitOK
}

// runPanel drives the drop-down session panel (M10). The daemon grabs a hotkey
// for this itself, so the CLI is the fallback: a desktop shortcut, a script, or a
// Wayland session where a client cannot grab a key globally.
func runPanel(args []string) int {
	fs := flag.NewFlagSet("panel", flag.ExitOnError)
	show := fs.Bool("show", false, "roll it down (no-op if it is already down)")
	hide := fs.Bool("hide", false, "roll it up")
	state := fs.Bool("state", false, "print whether it is open, change nothing")
	asJSON := fs.Bool("json", false, "machine-readable output")
	pos := parsePositional(fs, args)

	action := "toggle"
	switch {
	case *state:
		action = "state"
	case *show && *hide:
		fmt.Fprintf(os.Stderr, "agentbox panel: --show and --hide are opposites\n")
		return exitUsage
	case *show:
		action = "show"
	case *hide:
		action = "hide"
	}
	// A bare verb works too, because `agentbox dnd on|off|status` set that habit and
	// nobody remembers which commands take flags. It has to be a verb we know: an
	// unrecognised word used to be ignored, so `agentbox panel status` silently
	// TOGGLED the panel - a status query with a side effect, which cost an hour of
	// this session chasing a panel that kept closing itself.
	if w := first(pos); w != "" {
		switch w {
		case "show", "down", "open":
			action = "show"
		case "hide", "up", "close":
			action = "hide"
		case "state", "status":
			action = "state"
		case "toggle":
			action = "toggle"
		default:
			fmt.Fprintf(os.Stderr, "agentbox panel: %q is not a panel action (show, hide, toggle, state)\n", w)
			return exitUsage
		}
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), func() error {
		return fmt.Errorf("no daemon running")
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox panel: no daemon running (start one with `agentbox daemon`)\n")
		return exitError
	}
	defer conn.Close()

	var res struct {
		Open bool `json:"open"`
	}
	if err := conn.Call(dialCtx, proto.MethodPanel, map[string]string{"action": action}, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox panel: %v\n", err)
		return exitError
	}
	if *asJSON {
		fmt.Printf("{\"open\":%t}\n", res.Open)
		return exitOK
	}
	if res.Open {
		fmt.Println("panel down")
	} else {
		fmt.Println("panel up")
	}
	return exitOK
}

// runArtifact is the CLI half of the artifact channel (M10): the same two ways
// back from an artifact that the MCP tools expose, for an agent driven from a
// shell rather than through MCP.
//
//	agentbox artifact wait --id a1b2 --timeout 60    # block on the human, print what they did
//	agentbox artifact read --id a1b2                 # take what they have done already
//
// Exit codes follow the blocking commands: 0 an event arrived, 3 the window
// elapsed without one.
func runArtifact(args []string) int {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "wait", "read":
	default:
		fmt.Fprintf(os.Stderr, "agentbox artifact: say `wait` or `read`\nexample: agentbox artifact wait --id a1b2 --timeout 60\n")
		return exitUsage
	}

	fs := flag.NewFlagSet("artifact "+sub, flag.ExitOnError)
	id := fs.String("id", "", "the artifact to listen to (default: any)")
	name := fs.String("name", "", "only this event name (repeatable as a comma list)")
	timeout := fs.Int("timeout", 0, "seconds to wait before giving up (wait only; 0 waits forever)")
	asJSON := fs.Bool("json", false, "machine-readable output")
	parsePositional(fs, args)

	req := proto.ArtifactWait{ArtifactID: *id, TimeoutS: *timeout}
	for n := range strings.SplitSeq(*name, ",") {
		if n = strings.TrimSpace(n); n != "" {
			req.Names = append(req.Names, n)
		}
	}

	// No deadline on the context: a wait is meant to last as long as the human
	// takes, and --timeout is how you bound it.
	ctx := context.Background()
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()

	if sub == "read" {
		var res proto.ArtifactReadResult
		if err := conn.Call(ctx, proto.MethodArtifactRead, req, &res); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
			return exitError
		}
		if *asJSON {
			return printJSON(res)
		}
		for _, ev := range res.Events {
			printEvent(ev)
		}
		return exitOK
	}

	var res proto.ArtifactWaitResult
	if err := conn.Call(ctx, proto.MethodArtifactWait, req, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	if *asJSON {
		if err := printJSON(res); err != exitOK {
			return err
		}
	} else if res.Event != nil {
		printEvent(*res.Event)
	}
	if !res.Received {
		return exitUnanswered
	}
	return exitOK
}

// printEvent is one line per event: the name, then the payload as the artifact
// sent it, so `agentbox artifact wait | read name data` works in a shell.
func printEvent(ev proto.ArtifactEvent) {
	data := strings.TrimSpace(string(ev.Data))
	if data == "" {
		fmt.Println(ev.Name)
		return
	}
	fmt.Printf("%s\t%s\n", ev.Name, data)
}

func printJSON(v any) int {
	out, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	fmt.Println(string(out))
	return exitOK
}

// runReview shows a unified diff (from --diff-file or stdin) for approval.
// Exit codes: 0 approved, 1 rejected/changes requested, 3 unanswered.
func runReview(args []string) int {
	fs := flag.NewFlagSet("review", flag.ExitOnError)
	id := identityFlags(fs)
	title := fs.String("title", "", "card title (required)")
	body := fs.String("body", "", "context above the diff, markdown")
	speak := fs.String("speak", "", "a line read out loud when the card appears (needs [speech] enabled)")
	diffFile := fs.String("diff-file", "", "file with the unified diff; omit to read stdin")
	timeout := fs.Int("timeout", 0, "seconds before it returns unanswered")
	asJSON := fs.Bool("json", false, "print the full result object")
	fs.Parse(args)

	if *title == "" {
		fmt.Fprintf(os.Stderr, "agentbox review: --title is required\nexample: agentbox review --title \"Apply patch?\" --diff-file changes.diff\n")
		return exitUsage
	}
	var diff string
	if *diffFile != "" {
		data, err := os.ReadFile(*diffFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox review: %v\n", err)
			return exitError
		}
		diff = string(data)
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox review: reading stdin: %v\n", err)
			return exitError
		}
		diff = string(data)
	}
	it := proto.Item{Kind: proto.KindDiff, Title: *title, Body: *body, Speak: *speak, Diff: diff, TimeoutS: *timeout, Identity: *id}
	if err := it.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox review: %v\nexample: agentbox review --title \"Apply patch?\" --diff-file changes.diff\n", err)
		return exitUsage
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()

	var res proto.Result
	if err := conn.Call(context.Background(), proto.MethodAsk, &it, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	if *asJSON {
		out, _ := json.Marshal(res)
		fmt.Println(string(out))
	} else if res.Reply != "" {
		fmt.Println(res.Reply) // the review comment
	}
	switch {
	case res.Answered && res.Approved:
		return exitOK
	case res.Answered:
		return exitNo
	default:
		return exitUnanswered
	}
}

// parsePositional parses a flag set whose command also takes positional
// arguments, honouring flags written after them and returning the positionals in
// order. Go's flag package stops at the first non-flag argument, so
// `agentbox show FILE --watch` - the order everyone types, and the order the docs
// used - would otherwise drop --watch without a word.
func parsePositional(fs *flag.FlagSet, args []string) []string {
	var pos []string
	fs.Parse(args)
	for rest := fs.Args(); len(rest) > 0; rest = fs.Args() {
		pos = append(pos, rest[0])
		fs.Parse(rest[1:])
	}
	return pos
}

// first returns the first positional argument, or "".
func first(pos []string) string {
	if len(pos) == 0 {
		return ""
	}
	return pos[0]
}

// runShow renders a markdown file (or stdin) in the viewer. The path is
// resolved to absolute here since the daemon's cwd differs from the client's.
// With --artifact the payload is interactive HTML that agentbox runs in its sandbox
// instead of prose it renders (M10); with --watch, every save re-runs it, which
// is the shortest loop there is for writing one.
func runShow(args []string) int {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	watch := fs.Bool("watch", false, "live-reload the file when it changes")
	title := fs.String("title", "", "window title")
	artifact := fs.Bool("artifact", false, "run the file as an interactive HTML artifact")
	pos := parsePositional(fs, args)

	req := proto.ShowRequest{Title: *title, Watch: *watch, Artifact: *artifact}
	// An artifact is minted a name so `agentbox artifact wait` can listen to this one
	// rather than to anything that emits: `id=$(agentbox show --artifact app.html)`.
	if *artifact {
		id, err := proto.NewArtifactID()
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox show: %v\n", err)
			return exitError
		}
		req.ArtifactID = id
	}
	switch arg := first(pos); arg {
	case "", "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox show: reading stdin: %v\n", err)
			return exitError
		}
		if len(data) == 0 {
			fmt.Fprintf(os.Stderr, "agentbox show: give a FILE or pipe markdown on stdin\nexample: agentbox show README.md\n")
			return exitUsage
		}
		req.Content = string(data)
		req.Watch = false // stdin has nothing to watch
		if req.Title == "" {
			req.Title = "stdin"
		}
	default:
		abs, err := filepath.Abs(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox show: %v\n", err)
			return exitError
		}
		if _, err := os.Stat(abs); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox show: %v\n", err)
			return exitError
		}
		req.Path = abs
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()
	if err := conn.Call(dialCtx, proto.MethodShow, &req, nil); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	if req.ArtifactID != "" {
		fmt.Println(req.ArtifactID)
	}
	return exitOK
}

// noFlags gives a flagless command the same --help behaviour every flagged
// command gets from its FlagSet. Without it, "agentbox inbox --help" opened the
// inbox window and "agentbox quit --help" actually quit the daemon: a request for
// the manual must never perform the action it asks about.
func noFlags(name, what string, args []string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: agentbox %s\n%s\n", name, what)
	}
	fs.Parse(args)
}

func runInbox(args []string) int {
	noFlags("inbox", "Open the app window on the inbox: pending items and history.", args)
	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()
	if err := conn.Call(dialCtx, proto.MethodInbox, struct{}{}, nil); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	return exitOK
}

// runApp opens the tabbed app window (M8). --tab selects the initial tab.
func runApp(args []string) int {
	fs := flag.NewFlagSet("app", flag.ExitOnError)
	tab := fs.String("tab", "", "initial surface: home, session, agents, assignments, inbox, history, viewer, library, or settings")
	fs.Parse(args)

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()
	if err := conn.Call(dialCtx, proto.MethodApp, map[string]string{"tab": *tab}, nil); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	return exitOK
}

func runStats(args []string) int {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	since := fs.String("since", "7d", "window: 24h, 7d, 30d, or 0 for all time")
	asJSON := fs.Bool("json", false, "print the full stats object")
	fs.Parse(args)

	var sinceMS int64
	allTime := *since == "0" || *since == "all"
	if !allTime {
		dur, err := parseSince(*since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox stats: %v\nexample: agentbox stats --since 7d\n", err)
			return exitUsage
		}
		sinceMS = time.Now().Add(-dur).UnixMilli()
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()

	var st proto.Stats
	if err := conn.Call(context.Background(), proto.MethodStats, map[string]int64{"since_ms": sinceMS}, &st); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
		return exitError
	}
	if *asJSON {
		out, _ := json.Marshal(st)
		fmt.Println(string(out))
		return exitOK
	}
	window := "all time"
	if !allTime {
		window = "last " + *since
	}
	printStats(st, window)
	return exitOK
}

// parseSince accepts a Go duration (e.g. 24h) or an N-day shorthand (7d).
func parseSince(s string) (time.Duration, error) {
	if rest, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(rest)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("bad --since %q (try 24h, 7d, 30d)", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("bad --since %q (try 24h, 7d, 30d)", s)
	}
	return d, nil
}

func printStats(st proto.Stats, window string) {
	if st.Total == 0 {
		fmt.Printf("no interruptions in %s\n", window)
		return
	}
	fmt.Printf("agentbox stats · %s\n", window)
	line := fmt.Sprintf("  %d interruptions, %d questions, %d answered", st.Total, st.Questions, st.Answered)
	if st.MedianAnswerMS > 0 {
		line += ", median answer " + humanDur(st.MedianAnswerMS)
	}
	fmt.Println(line)
	if len(st.ByAgent) > 0 {
		fmt.Println("  by agent:")
		for _, a := range st.ByAgent {
			row := fmt.Sprintf("    %-20s %3d total  %3d asked  %3d answered", a.Agent, a.Total, a.Questions, a.Answered)
			if a.MedianAnswerMS > 0 {
				row += "  median " + humanDur(a.MedianAnswerMS)
			}
			fmt.Println(row)
		}
	}
	if len(st.ByDay) > 1 {
		fmt.Println("  by day:")
		for _, dc := range st.ByDay {
			fmt.Printf("    %s  %d\n", dc.Day, dc.Count)
		}
	}
}

func humanDur(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", ms)
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// runMCP serves the Model Context Protocol over stdio (ADR-0004), proxying
// to the daemon. Spawned by an MCP host (e.g. Claude Code via .mcp.json).
func runMCP(args []string) int {
	noFlags("mcp", "Serve MCP over stdio; for an agent host to spawn, not for hands.", args)
	// AGENTBOX_SESSION_ID, set by agentbox's own session tab when it spawns claude,
	// tags interactions so the daemon can route them inline to that session
	// (M8, FR49) instead of a standalone card.
	// The key is minted once, here, and rides on every call this child makes for
	// the rest of the session (FR83). One child is one session, so this is the
	// only place in the system with the standing to say what that session is.
	id := proto.Identity{
		Agent: agentName(), Project: projectName(),
		Session: os.Getenv("AGENTBOX_SESSION_ID"), Key: sessionKey(),
		// This child is a model's hands, so it is the one caller a discovery
		// rider can be spent on (FR83).
		Via: proto.ViaMCP,
	}
	if err := agentboxmcp.Serve(context.Background(), runtimeDir(), version.Get().Revision, id); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox mcp: %v\n", err)
		return exitError
	}
	return exitOK
}

// runDocs serves the embedded manual (FR40): no args lists topics, a topic
// name prints it, `agent` is the context-sized quickstart, `setup` emits
// paste-ready MCP and hook config.
func runDocs(args []string) int {
	if len(args) == 0 {
		fmt.Println("agentbox docs - the manual ships in the binary")
		fmt.Println("\ntopics:")
		for _, t := range manual.Topics() {
			fmt.Printf("  %s\n", t)
		}
		fmt.Println("  setup   paste-ready .mcp.json and Claude Code hook snippets")
		fmt.Println("\nagentbox docs agent         the agent quickstart (paste into your instructions)")
		fmt.Println("agentbox docs walkthrough   the standard for authoring a review kit")
		fmt.Println("agentbox schema             JSON Schema for the wire protocol")
		return exitOK
	}
	if args[0] == "setup" {
		printSetup()
		return exitOK
	}
	if s, ok := manual.Get(args[0]); ok {
		fmt.Print(s)
		return exitOK
	}
	fmt.Fprintf(os.Stderr, "agentbox docs: no topic %q; run `agentbox docs` to list topics\n", args[0])
	return exitUsage
}

// printSetup emits ready-to-paste integration config (FR40): the MCP server
// registration and Claude Code hook snippets, pointing at this binary.
func printSetup() {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "agentbox"
	}
	fmt.Println("# Register the agentbox MCP server with Claude Code (.mcp.json):")
	fmt.Printf("{\"mcpServers\":{\"agentbox\":{\"command\":%q,\"args\":[\"mcp\"]}}}\n", exe)
	fmt.Println()
	fmt.Println("# Or skip MCP and use hooks (~/.claude/settings.json):")
	fmt.Println(`{
  "hooks": {
    "Notification": [
      {"hooks": [{"type": "command", "command": "` + exe + ` notify --level info --title \"Claude needs you\""}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "` + exe + ` notify --level success --title \"Agent finished\""}]}
    ]
  }
}`)
	fmt.Println()
	// The roster is only as truthful as the agents on it, and these cost no tokens
	// because they never go through the model (FR83). No session key to set: the
	// call finds the session it belongs to by itself - see docs/recipes.md.
	//
	// `|| true` because a roster ping must never be why a session shows an error:
	// sync exits 4 with no daemon, and a deploy stops the daemon on purpose.
	fmt.Println("# Put every session on the Agents board, with no tokens spent (~/.claude/settings.json):")
	fmt.Println(`{
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "` + exe + ` sync announce \"$(basename \"$PWD\") session (purpose not yet stated)\" || true"}]}
    ],
    "PostToolUse": [
      {"matcher": "Edit|Write|NotebookEdit",
       "hooks": [{"type": "command", "command": "` + exe + ` sync activity \"editing $(jq -r '.tool_input.file_path // \"a file\"')\" || true"}]},
      {"matcher": "Bash",
       "hooks": [{"type": "command", "command": "` + exe + ` sync activity \"$(jq -r '.tool_input.command // \"running a command\"' | cut -c1-70)\" || true"}]}
    ]
  }
}`)
}

func runLogs(args []string) int {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	follow := fs.Bool("follow", false, "keep printing new events")
	fs.Parse(args)
	path := filepath.Join(stateDir(), "log", "agentbox.jsonl")
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: no log at %s (daemon never ran?)\n", path)
		return exitError
	}
	defer f.Close()
	if _, err := f.WriteTo(os.Stdout); err != nil {
		return exitError
	}
	for *follow {
		time.Sleep(500 * time.Millisecond)
		if _, err := f.WriteTo(os.Stdout); err != nil {
			return exitError
		}
	}
	return exitOK
}
