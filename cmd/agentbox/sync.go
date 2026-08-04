package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/borismilner/agentbox/internal/client"
	"github.com/borismilner/agentbox/internal/proto"
)

// `agentbox sync ...` (FR83): the roster's second door.
//
// The CLI is not a convenience here, it is what makes the mandate reachable by
// things that are not Claude sessions: a SessionStart hook announcing a purpose
// for nothing, a PostToolUse hook keeping the activity line honest when a model
// forgets, a Makefile, a non-Claude agent. Hooks are shell, so they cost no
// tokens, which is the only reason coverage can be complete rather than
// aspirational.
//
// A CLI call is not itself a session and has no key of its own. It acts on
// behalf of one with --key, defaulting to AGENTBOX_SESSION_KEY so a hook script
// inherits it without every recipe repeating the flag.

func syncUsage() {
	fmt.Fprintln(os.Stderr, "usage: agentbox sync announce PURPOSE [--area A] [--activity LINE]")
	fmt.Fprintln(os.Stderr, "       agentbox sync activity LINE")
	fmt.Fprintln(os.Stderr, "       agentbox sync agents [--area A] [--project P] [--json]")
	fmt.Fprintln(os.Stderr, "       agentbox sync peers [--json]")
	fmt.Fprintln(os.Stderr, "       agentbox sync attach [--area A]")
	fmt.Fprintln(os.Stderr, "       agentbox sync lock NAME [--timeout N] -- CMD ...   # hold it for one command")
	fmt.Fprintln(os.Stderr, "       agentbox sync lock NAME [--timeout N] --ttl N      # detached hold")
	fmt.Fprintln(os.Stderr, "       agentbox sync unlock NAME")
	fmt.Fprintln(os.Stderr, "       agentbox sync locks [--json]")
	fmt.Fprintln(os.Stderr, "       agentbox sync post TOPIC [DATA]                    # tell the others")
	fmt.Fprintln(os.Stderr, "       agentbox sync await TOPIC... [--after SEQ] [--timeout N]")
	fmt.Fprintln(os.Stderr, "       agentbox sync get KEY                              # or KEY* for the family")
	fmt.Fprintln(os.Stderr, "       agentbox sync set KEY VALUE [--if-version N] [--own]")
	fmt.Fprintln(os.Stderr, "       agentbox sync del KEY [--if-version N]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Who else is here, what they are for, and what they are doing right now.")
	fmt.Fprintln(os.Stderr, "Every call acts on behalf of a session: --key, or AGENTBOX_SESSION_KEY.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "A wrapped lock releases on ANY exit, signals included. If the shell that")
	fmt.Fprintln(os.Stderr, "ran it is killed (an agent's foreground call dies at 120s), the hold is")
	fmt.Fprintln(os.Stderr, "released rather than left behind. --ttl is the form for a command that")
	fmt.Fprintln(os.Stderr, "outlives its shell: take the hold, run the work, unlock when it is done.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "A signal topic is a name (tests:green) or, when awaiting, a prefix ending")
	fmt.Fprintln(os.Stderr, "in * (done:*). DATA that parses as JSON is sent as JSON; anything else is")
	fmt.Fprintln(os.Stderr, "sent as a JSON string. await prints one line per signal and its cursor,")
	fmt.Fprintln(os.Stderr, "which --after resumes from without missing anything in between.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "A shared value is small state with compare-and-swap. --if-version 0 claims a")
	fmt.Fprintln(os.Stderr, "key only if nobody has it yet, which is how a fanned-out job splits work with")
	fmt.Fprintln(os.Stderr, "one key per item; --if-version N writes only if it is still at N. --own records")
	fmt.Fprintln(os.Stderr, "this session as the owner, so a claim left by a session that died reads as")
	fmt.Fprintln(os.Stderr, "orphaned instead of blocking the table.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Exit codes: 0 done, 1 refused or not granted, 4 agentbox itself failed.")
	fmt.Fprintln(os.Stderr, "An await that times out is 1; a wrap exits with the command's own code.")
	fmt.Fprintln(os.Stderr, "A set or del that lost the compare-and-swap is 1, with the current value on")
	fmt.Fprintln(os.Stderr, "stdout: that is the shape a claim loop wants from a shell.")
}

func runSync(args []string) int {
	if len(args) == 0 {
		syncUsage()
		return exitUsage
	}
	verb, rest := args[0], args[1:]

	// A wrapped command is split out before any flag parsing: everything after the
	// first bare `--` belongs to the command, including its own flags, and letting
	// the flag package near it would eat them.
	var wrapped []string
	for i, a := range rest {
		if a == "--" {
			wrapped = rest[i+1:]
			rest = rest[:i]
			break
		}
	}

	fs := flag.NewFlagSet("sync "+verb, flag.ExitOnError)
	area := fs.String("area", "", "kind:scope area tag")
	project := fs.String("project", "", "filter by project")
	activity := fs.String("activity", "", "current activity (announce only)")
	asJSON := fs.Bool("json", false, "machine-readable output")
	key := fs.String("key", os.Getenv("AGENTBOX_SESSION_KEY"), "session key to act on behalf of")
	timeout := fs.Int("timeout", 0, "seconds to wait for a lock; 0 waits as long as the daemon allows")
	ttl := fs.Int("ttl", 0, "seconds a detached hold lives without a wrapped command")
	note := fs.String("note", "", "what the lock is being held for")
	after := fs.Int64("after", 0, "signal cursor to resume from; 0 waits for what happens from now on")
	// A string rather than an int because zero is a real value here and a meaningful
	// one: it claims a key only if nobody has it yet. An int flag cannot tell "0" from
	// "not given", and the difference is between a claim and an unconditional write.
	ifVersion := fs.String("if-version", "", "compare-and-swap: 0 writes only if the key is absent, N only if it is still at N")
	own := fs.Bool("own", false, "record this session as the value's owner")
	fs.Usage = syncUsage

	// Same trap the control verb documents: Go's flag package stops at the first
	// non-flag argument, so `sync announce "porting the settings surface" --area
	// subsystem:webui` would silently keep the default and fold the flag into the
	// purpose the human reads. Lift the flags out first.
	flags, words := partitionControlFlags(rest)
	if err := fs.Parse(flags); err != nil {
		return exitError
	}
	text := strings.TrimSpace(strings.Join(words, " "))

	id := proto.Identity{
		Agent: agentName(), Project: filepath.Base(mustGetwd()),
		Session: os.Getenv("AGENTBOX_SESSION_ID"), Key: *key,
		// A shell, even when it acts for a session: a rider spent here would be
		// read by nobody, and a session's hooks call this several times a minute.
		Via: proto.ViaCLI,
	}

	switch verb {
	case "announce":
		if text == "" {
			fmt.Fprintln(os.Stderr, "agentbox sync announce: wants a purpose - one line saying what this session is for")
			return exitNo
		}
		if id.Key == "" {
			fmt.Fprintln(os.Stderr, "agentbox sync announce: wants --key (or AGENTBOX_SESSION_KEY): one session, one key")
			return exitNo
		}
		// The cwd rides along so the daemon can derive the area: a hook announces
		// before the session's own child has attached, and a row with no area is
		// invisible to the peers it exists to be found by (FR90).
		req := proto.SyncAnnounceParams{Identity: id, Purpose: text, Activity: *activity,
			Cwd: mustGetwd(), Area: *area}
		var res proto.SyncResult
		if code := syncCLICall(proto.MethodSyncAnnounce, &req, &res); code != exitOK {
			return code
		}
		if *asJSON {
			return printJSON(res)
		}
		fmt.Printf("announced: %s\n", text)
		printPeers(res)
		return exitOK

	case "activity":
		if text == "" {
			fmt.Fprintln(os.Stderr, "agentbox sync activity: wants a line saying what is happening now")
			return exitNo
		}
		// Routed through the control verb on purpose: set_activity is ONE verb
		// that writes the roster always and the hands-off strip additionally, and
		// a second door with different behaviour would be exactly the drift this
		// feature exists to remove.
		req := proto.ControlRequestParams{Action: proto.ControlActivity, Identity: id, Activity: text}
		var res proto.ControlResult
		if code := syncCLICall(proto.MethodControl, &req, &res); code != exitOK {
			return code
		}
		if *asJSON {
			return printJSON(res)
		}
		return exitOK

	case "agents", "peers":
		req := proto.SyncListParams{Identity: id, Area: *area, Project: *project}
		var res proto.SyncResult
		if code := syncCLICall(proto.MethodSyncList, &req, &res); code != exitOK {
			return code
		}
		if *asJSON {
			return printJSON(res)
		}
		if len(res.Agents) == 0 {
			if res.Partial {
				fmt.Println("no attached agents, but the roster cannot see everybody - do not read this as being alone")
			} else {
				fmt.Println("no agents attached")
			}
			return exitOK
		}
		for _, a := range res.Agents {
			purpose := a.Purpose
			if purpose == "" {
				purpose = "no purpose given"
			}
			line := fmt.Sprintf("%s · %s [%s] %s", a.Agent, a.Project, a.State, purpose)
			if a.Activity != "" {
				line += " · " + a.Activity
			}
			fmt.Println(line)
		}
		if res.Partial {
			fmt.Println("(partial: at least one session predates sync and has no row)")
		}
		return exitOK

	case "lock":
		// The lock's CLI door (synclock.go). It mints its own session key rather
		// than borrowing --key: a wrap announces "holding deploy:agentbox" and that
		// must not overwrite the purpose of the session that started it.
		return runSyncLock(lockCLI{
			name: text, timeout: *timeout, ttl: *ttl, note: *note,
			wrapped: wrapped, asJSON: *asJSON, area: *area,
		})

	case "unlock":
		return runSyncUnlock(text, id.Key, *asJSON)

	case "break":
		// The human's verb, the same one the Agents surface offers, for when the
		// surface is not open. It is deliberately not something an agent should
		// reach for: taking a lock away from a peer is a decision about whose work
		// matters more, which is his.
		return runSyncBreak(text, *asJSON)

	case "locks":
		return runSyncLocks(*asJSON)

	case "post":
		// The first word is the topic and the rest is the payload, so a hook can
		// write `sync post tests:green '{"suite":"race"}'` without quoting rules of
		// its own.
		if len(words) == 0 {
			fmt.Fprintln(os.Stderr, "agentbox sync post: wants a topic - kind:scope, e.g. tests:green or to:<session key>")
			return exitNo
		}
		if id.Key == "" {
			fmt.Fprintln(os.Stderr, "agentbox sync post: wants --key (or AGENTBOX_SESSION_KEY): one session, one key")
			return exitNo
		}
		req := proto.SyncPostParams{Identity: id, Topic: words[0]}
		if raw := strings.TrimSpace(strings.Join(words[1:], " ")); raw != "" {
			req.Data = jsonOrString(raw)
		}
		var res proto.SyncPostResult
		if code := syncCLICall(proto.MethodSyncPost, &req, &res); code != exitOK {
			return code
		}
		if *asJSON {
			return printJSON(res)
		}
		fmt.Printf("posted %s as seq %d, %d listener(s) woken\n", res.Topic, res.Seq, res.Delivered)
		return exitOK

	case "await":
		if len(words) == 0 {
			fmt.Fprintln(os.Stderr, "agentbox sync await: wants at least one topic - an exact name, or a prefix ending in *")
			return exitNo
		}
		if id.Key == "" {
			fmt.Fprintln(os.Stderr, "agentbox sync await: wants --key (or AGENTBOX_SESSION_KEY): one session, one key")
			return exitNo
		}
		return runSyncAwait(id, words, *after, *timeout, *asJSON)

	case "get", "set", "del", "delete":
		// The blackboard's CLI door (syncshared.go). Three verbs rather than one with
		// an --op flag: a shell reads better as `sync set claims/3 mine` than as a verb
		// plus a mode, and the MCP side folds them for a reason that does not apply
		// here (a tool costs context in every session; a subcommand costs nothing).
		if len(words) == 0 {
			fmt.Fprintf(os.Stderr, "agentbox sync %s: wants a key\n", verb)
			return exitNo
		}
		return runSyncShared(sharedCLI{
			verb: verb, id: id, key: words[0], value: strings.Join(words[1:], " "),
			ifVersion: *ifVersion, own: *own, asJSON: *asJSON,
		})

	case "attach":
		// Holds presence open for as long as this process runs, which is the door
		// a non-Claude agent uses: same contract as the mcp child, wrapped around
		// whatever the agent actually is.
		if id.Key == "" {
			fmt.Fprintln(os.Stderr, "agentbox sync attach: wants --key (or AGENTBOX_SESSION_KEY): one session, one key")
			return exitNo
		}
		cwd, _ := os.Getwd()
		req := proto.SyncAttachParams{Identity: id, Cwd: cwd, PID: os.Getpid(), Area: *area}
		dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err := client.Dial(dialCtx, runtimeDir(), func() error {
			return fmt.Errorf("no daemon running")
		})
		cancel()
		if err != nil {
			fmt.Fprintln(os.Stderr, "agentbox sync attach: no daemon running")
			return exitError
		}
		defer conn.Close()
		var res proto.SyncResult
		// No timeout: the whole point is to stay. Ctrl-C or the process ending is
		// what ends the row.
		if err := conn.Call(context.Background(), proto.MethodSyncAttach, &req, &res); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox sync attach: %v\n", err)
			return exitError
		}
		return exitOK

	default:
		fmt.Fprintf(os.Stderr, "agentbox sync: unknown verb %q\n", verb)
		syncUsage()
		return exitUsage
	}
}

// syncCLICall makes one short call and maps a refusal to exit 1, following the
// house exit-code grammar rather than inventing one.
func syncCLICall(method string, req, res any) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(ctx, runtimeDir(), func() error {
		return fmt.Errorf("no daemon running")
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentbox sync: no daemon running")
		return exitError
	}
	defer conn.Close()
	if err := conn.Call(ctx, method, req, res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox sync: %v\n", err)
		return exitNo
	}
	return exitOK
}

// jsonOrString sends a payload as JSON when it is JSON, and as a JSON string when
// it is not. A hook that has an object sends the object; one that has a word sends
// the word, and neither has to know which one agentbox wanted.
func jsonOrString(raw string) json.RawMessage {
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	return b
}

// runSyncAwait parks on the topics and prints what arrived. Its own call rather
// than syncCLICall's because that one gives up after five seconds, which is the
// right ceiling for a question and the wrong one for a wait.
//
// The daemon bounds the park at [sync] wait_max_s whatever this asks for, so the
// local deadline only has to be generous enough not to be the thing that fires
// first.
func runSyncAwait(id proto.Identity, topics []string, after int64, timeout int, asJSON bool) int {
	req := proto.SyncAwaitParams{Identity: id, Topics: topics, AfterSeq: after, TimeoutS: timeout}

	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, err := client.Dial(dialCtx, runtimeDir(), func() error {
		return fmt.Errorf("no daemon running")
	})
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentbox sync await: no daemon running")
		return exitError
	}
	defer conn.Close()

	// No deadline of our own when the caller named none: the daemon's ceiling is the
	// bound, and a second one here would turn a resumable timeout into a transport
	// error that says nothing about the cursor.
	ctx := context.Background()
	if timeout > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, time.Duration(timeout)*time.Second+10*time.Second)
		defer stop()
	}
	var res proto.SyncAwaitResult
	if err := conn.Call(ctx, proto.MethodSyncAwait, &req, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox sync await: %v\n", err)
		return exitNo
	}
	if asJSON {
		if code := printJSON(res); code != exitOK {
			return code
		}
		if len(res.Signals) == 0 {
			return exitNo
		}
		return exitOK
	}
	if res.Gap {
		fmt.Fprintf(os.Stderr, "gap: signals between your cursor and %d were trimmed by retention\n", res.OldestSeq)
	}
	if len(res.Signals) == 0 {
		fmt.Printf("nothing on %s; cursor %d\n", strings.Join(topics, ", "), res.Cursor)
		return exitNo
	}
	for _, sig := range res.Signals {
		line := fmt.Sprintf("%d %s", sig.Seq, sig.Topic)
		if sig.Agent != "" {
			line += " from " + sig.Agent
		}
		if len(sig.Data) > 0 {
			line += " " + string(sig.Data)
		}
		fmt.Println(line)
	}
	fmt.Printf("cursor %d\n", res.Cursor)
	if res.More {
		fmt.Println("more waiting: call again with --after", res.Cursor)
	}
	return exitOK
}

// printPeers is the whole point of announce answering with something: an agent
// learns it has company in its first call, and so does a human running it by
// hand.
func printPeers(res proto.SyncResult) {
	if res.Partial {
		fmt.Println("the roster cannot see everybody: at least one session predates sync, so do not conclude you are alone")
	}
	if len(res.Agents) == 0 {
		if !res.Partial {
			fmt.Println("nobody else is in your area")
		}
		return
	}
	fmt.Printf("%d other agent(s) in your area:\n", len(res.Agents))
	for _, a := range res.Agents {
		purpose := a.Purpose
		if purpose == "" {
			purpose = "no purpose given"
		}
		fmt.Printf("  %s · %s [%s] %s\n", a.Agent, a.Project, a.State, purpose)
	}
}
