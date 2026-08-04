package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/borismilner/agentbox/internal/client"
	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
)

// `agentbox sync lock` (FR83 slice 2): the lock's other door, for the things
// that are not Claude sessions - a Makefile, a hook, a non-Claude agent.
//
// This repo is the first customer: `make deploy` wrapped in
// `agentbox sync lock deploy:agentbox -- ...` retires a CLAUDE.md trap by
// construction instead of by everybody remembering it.
//
// One measured fact shaped the whole verb. A foreground shell call from a Claude
// session is killed at exactly 120s (SIGTERM, exit 143), and the tool's own
// timeout caps at 600s - both far below how long `make deploy` takes on this
// repo. So the wrap cannot be the only form: `--ttl` takes a hold that outlives
// the shell, and the wrap releases on ANY exit, signals included, or every long
// command would leave an orphan behind.

// rosterBridge is the Agents surface's keyhole into the lock table: read the
// table, and break one lock. Nothing else, so the surface cannot reach the rest
// of the daemon through it - the same shape Handover and Voice already use.
type rosterBridge struct{ d *daemon.Daemon }

func (r rosterBridge) Locks() []proto.SyncLockState { return r.d.LockSnapshot() }

// BreakLock never fails today: breaking a lock nobody holds is a no-op with a
// sentence, not an error. The error return exists because the surface has to be
// able to show one when a later reason to refuse appears.
func (r rosterBridge) BreakLock(name string) error {
	r.d.BreakLock(name)
	return nil
}

// lockCLI is what `sync lock` needs from the command line, gathered so the two
// forms (wrap a command, or take a detached hold) share one path.
type lockCLI struct {
	name    string
	timeout int
	ttl     int
	note    string
	wrapped []string
	asJSON  bool
	area    string
}

// runSyncLock takes the lock and then either runs the command or returns.
func runSyncLock(in lockCLI) int {
	if in.name == "" {
		fmt.Fprintln(os.Stderr, "agentbox sync lock: wants a lock name, in the kind:scope idiom (deploy:agentbox, repo:agentbox, vm:boris-vm)")
		return exitNo
	}
	if len(in.wrapped) == 0 && in.ttl <= 0 {
		fmt.Fprintln(os.Stderr, "agentbox sync lock: give it a command to wrap (-- CMD ...) or a --ttl for a detached hold.")
		fmt.Fprintln(os.Stderr, "A hold with neither would end the moment this process exits, which is the same as never taking it.")
		return exitNo
	}

	// A wrap is its own session, with its own key, and never borrows the calling
	// agent's: announcing under a live session's key would overwrite the purpose
	// the human is reading on the board with "holding a lock".
	id := proto.Identity{
		Agent:   lockAgentName(in.wrapped),
		Project: filepath.Base(mustGetwd()),
		Key:     newSessionKey(),
		Via:     proto.ViaCLI,
	}
	purpose := "holding " + in.name
	if len(in.wrapped) > 0 {
		purpose += ": " + strings.Join(in.wrapped, " ")
	} else if in.note != "" {
		purpose += ": " + in.note
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := client.Dial(ctx, runtimeDir(), func() error { return fmt.Errorf("no daemon running") })
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentbox sync lock: no daemon running")
		return exitError
	}
	defer conn.Close()

	// A row on the human's board for as long as this process lives, so a lock held
	// by a Makefile is as visible as one held by an agent. It also satisfies the
	// announce gate the lock verbs apply.
	cwd, _ := os.Getwd()
	attachCtx, stopAttach := context.WithCancel(context.Background())
	defer stopAttach()
	var attachWG sync.WaitGroup
	if len(in.wrapped) > 0 {
		attachWG.Go(func() {
			holdRow(attachCtx, id, cwd, in.area)
		})
	}
	ann := proto.SyncAnnounceParams{Identity: id, Purpose: purpose, Area: in.area,
		Activity: "waiting for " + in.name}
	var annRes proto.SyncResult
	if err := conn.Call(ctx, proto.MethodSyncAnnounce, &ann, &annRes); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox sync lock: %v\n", err)
		return exitError
	}

	req := proto.SyncLockParams{
		Identity: id, Name: in.name, TimeoutS: in.timeout, Note: in.note,
		TTLS: in.ttl, PID: os.Getpid(),
	}
	var res proto.SyncLockResult
	// No deadline of our own: the daemon's ceiling decides, and a wrap that gave
	// up early would hand the work to nobody.
	if err := conn.Call(context.Background(), proto.MethodSyncLock, &req, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox sync lock: %v\n", err)
		return exitError
	}
	if in.asJSON {
		printJSON(res)
	}
	if !res.Granted {
		if !in.asJSON {
			fmt.Fprintln(os.Stderr, "agentbox sync lock: "+lockRefusal(in.name, res))
		}
		return exitNo
	}
	if !in.asJSON {
		if len(in.wrapped) > 0 {
			fmt.Fprintf(os.Stderr, "holding %s\n", in.name)
		} else {
			fmt.Printf("holding %s for %ds - release it with: agentbox sync unlock %s --key %s\n",
				in.name, in.ttl, in.name, id.Key)
		}
	}
	if len(in.wrapped) == 0 {
		// A detached hold: the key is printed above because releasing needs it, and
		// the ttl frees it if nobody does.
		return exitOK
	}

	code := runHeld(conn, id, in, attachCtx)
	stopAttach()
	attachWG.Wait()
	return code
}

// runHeld runs the wrapped command with the lock held, and releases it however
// the command ends - a clean exit, a failure, or this process being killed.
//
// The signal half is not defensive coding: an agent's foreground shell call is
// SIGTERMed at 120s, so a wrap without it would leave an orphaned hold behind on
// every command longer than two minutes.
func runHeld(conn *proto.Conn, id proto.Identity, in lockCLI, attachCtx context.Context) int {
	cmd := exec.Command(in.wrapped[0], in.wrapped[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Its own process group, so a signal aimed at this wrapper can be passed on to
	// the whole command tree rather than only to its top process.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	release := func() {
		req := proto.SyncLockParams{Identity: id, Name: in.name}
		var res proto.SyncLockResult
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Call(ctx, proto.MethodSyncUnlock, &req, &res)
	}

	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(sigs)

	setActivity(id, "running "+strings.Join(in.wrapped, " "))
	if err := cmd.Start(); err != nil {
		release()
		fmt.Fprintf(os.Stderr, "agentbox sync lock: %v\n", err)
		return exitError
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	for {
		select {
		case err := <-done:
			release()
			return exitCodeOf(err)
		case sig := <-sigs:
			// Pass it on to the command's group, then wait for it to go. The hold is
			// released either way, which is the whole reason this loop exists.
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
			}
			select {
			case err := <-done:
				release()
				return exitCodeOf(err)
			case <-time.After(5 * time.Second):
				release()
				if s, ok := sig.(syscall.Signal); ok {
					return 128 + int(s)
				}
				return exitError
			}
		case <-attachCtx.Done():
			release()
			return exitError
		}
	}
}

// runSyncUnlock releases a hold, for the detached form and for a human undoing
// one by hand from the roster.
func runSyncUnlock(name, key string, asJSON bool) int {
	if name == "" {
		fmt.Fprintln(os.Stderr, "agentbox sync unlock: wants a lock name")
		return exitNo
	}
	if key == "" {
		fmt.Fprintln(os.Stderr, "agentbox sync unlock: wants --key (or AGENTBOX_SESSION_KEY): only the holder may release a lock.")
		fmt.Fprintln(os.Stderr, "`agentbox sync locks` lists what is held and by which key. The human breaks a lock from the Agents surface instead.")
		return exitNo
	}
	req := proto.SyncLockParams{
		Identity: proto.Identity{Agent: agentName(), Project: filepath.Base(mustGetwd()),
			Key: key, Via: proto.ViaCLI},
		Name: name,
	}
	var res proto.SyncLockResult
	if code := syncCLICall(proto.MethodSyncUnlock, &req, &res); code != exitOK {
		return code
	}
	if asJSON {
		return printJSON(res)
	}
	if !res.Released {
		fmt.Fprintf(os.Stderr, "agentbox sync unlock: %s\n", firstLine(res.Note, "not released"))
		return exitNo
	}
	fmt.Printf("released %s\n", name)
	return exitOK
}

// runSyncLocks lists the table. Ungated like every other read: the human and the
// agents must never see different answers.
func runSyncLocks(asJSON bool) int {
	var res proto.SyncLocksResult
	if code := syncCLICall(proto.MethodSyncLocks, &struct{}{}, &res); code != exitOK {
		return code
	}
	if asJSON {
		return printJSON(res)
	}
	if len(res.Locks) == 0 {
		fmt.Println("no locks held")
		return exitOK
	}
	for _, l := range res.Locks {
		line := fmt.Sprintf("%s · held by %s for %s", l.Name, nameOrKey(l.HolderAgent, l.HolderKey),
			(time.Duration(l.HeldMS) * time.Millisecond).Round(time.Second))
		if l.Orphaned {
			line += fmt.Sprintf(" · ORPHANED (holder's session gone, pid %d still alive)", l.PID)
		}
		if l.ExpiresInMS > 0 {
			line += fmt.Sprintf(" · ttl %s left", (time.Duration(l.ExpiresInMS) * time.Millisecond).Round(time.Second))
		}
		if l.Waiters > 0 {
			line += fmt.Sprintf(" · %d waiting", l.Waiters)
		}
		if l.Note != "" {
			line += " · " + l.Note
		}
		fmt.Println(line)
	}
	return exitOK
}

// holdRow keeps a roster row open for the life of a wrap, so a lock held by a
// Makefile is as visible on the board as one held by an agent. Its own
// connection: the lock call on the main one parks, and presence must not queue
// behind it.
func holdRow(ctx context.Context, id proto.Identity, cwd, area string) {
	conn, err := client.Dial(ctx, runtimeDir(), func() error { return fmt.Errorf("no daemon running") })
	if err != nil {
		return
	}
	defer conn.Close()
	req := proto.SyncAttachParams{Identity: id, Cwd: cwd, PID: os.Getpid(), Area: area}
	var res proto.SyncResult
	_ = conn.Call(ctx, proto.MethodSyncAttach, &req, &res)
}

// setActivity narrates the wrap on the board: waiting, then running.
func setActivity(id proto.Identity, line string) {
	req := proto.ControlRequestParams{Action: proto.ControlActivity, Identity: id, Activity: line}
	var res proto.ControlResult
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := client.Dial(ctx, runtimeDir(), nil)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.Call(ctx, proto.MethodControl, &req, &res)
}

// lockRefusal says what happened in one line a human or a script can act on.
func lockRefusal(name string, res proto.SyncLockResult) string {
	who := "another agent"
	if res.Holder != nil {
		who = nameOrKey(res.Holder.Agent, res.Holder.Key)
		if res.Holder.Purpose != "" {
			who += " (" + res.Holder.Purpose + ")"
		}
	}
	switch {
	case res.Deadlock != "":
		return "refused, would deadlock: " + res.Deadlock
	case res.Orphaned:
		return fmt.Sprintf("%s is orphaned: its holder's session is gone but pid %d is still running, so it is not free. It frees itself when that process ends, or break it from the Agents surface.", name, res.HolderPID)
	case res.TimedOut:
		return fmt.Sprintf("not granted: %s is held by %s, %d waiting. Nothing changed by asking; try again or coordinate with them.",
			name, who, res.Queue)
	default:
		return fmt.Sprintf("not granted: %s is held by %s.", name, who)
	}
}

// lockAgentName names the row after the work rather than after this binary: the
// human reading the board wants to see `make`, not `agentbox`.
func lockAgentName(wrapped []string) string {
	if len(wrapped) > 0 {
		return filepath.Base(wrapped[0])
	}
	return agentName()
}

func newSessionKey() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("cli%d", os.Getpid())
	}
	return hex.EncodeToString(b[:])
}

func exitCodeOf(err error) int {
	if err == nil {
		return exitOK
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return exitError
}

func nameOrKey(name, key string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	if key != "" {
		return "session " + key
	}
	return "somebody"
}

func firstLine(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}
