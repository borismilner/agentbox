package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/borismilner/agentbox/internal/client"
	"github.com/borismilner/agentbox/internal/proto"
)

// `agentbox dismiss` (FR89): retire pending items without reaching for the mouse.
//
// The asymmetry this closes: agentbox had four doors to CREATE an item and none to
// retire one. A warning stays pending until it is clicked, pending items survive a
// daemon restart by design, and so a toast that turned out to be noise came back
// after every restart until somebody clicked it. That happened to Boris twice with
// the same four probe-generated "Deadlock refused" toasts, which is what this verb
// is for.
//
// This door is the HUMAN's, so it may clear everything. An agent's is the `retract`
// MCP tool, which can only ever touch what that agent posted.

func dismissUsage() {
	fmt.Fprintln(os.Stderr, "usage: agentbox dismiss ID...        # retire these pending items")
	fmt.Fprintln(os.Stderr, "       agentbox dismiss --all        # retire every pending item")
	fmt.Fprintln(os.Stderr, "       agentbox pending [--json]     # what is pending, with ids")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Clears items from the queue and the screen without clicking. A dismissal is")
	fmt.Fprintln(os.Stderr, "an ordinary resolution: whoever is blocked on the item is told, and history")
	fmt.Fprintln(os.Stderr, "records it.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Exit codes: 0 dismissed, 1 nothing matched, 4 agentbox itself failed.")
}

// runPending is the read half, and it exists for the same reason dismiss does: the
// queue had no terminal door at all. `agentbox status` counts pending items without
// saying what they are, so naming one to dismiss meant opening a window - which is
// the thing this whole verb is trying to avoid.
func runPending(args []string) int {
	fs := flag.NewFlagSet("pending", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: agentbox pending [--json]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Every item still waiting for you, with the id `agentbox dismiss` takes.")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(ctx, runtimeDir(), func() error {
		return fmt.Errorf("no daemon running")
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentbox pending: no daemon running, so nothing is pending")
		return exitError
	}
	defer conn.Close()

	// The stored shape, read loosely: this is a human-facing listing, and it must
	// not break every time an item type gains a field.
	var res struct {
		Pending []struct {
			ID       string `json:"id"`
			Kind     string `json:"kind"`
			Level    string `json:"level"`
			Title    string `json:"title"`
			Identity struct {
				Agent string `json:"agent"`
			} `json:"identity"`
		} `json:"pending"`
	}
	if err := conn.Call(ctx, proto.MethodList, struct{}{}, &res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox pending: %v\n", err)
		return exitError
	}
	if *asJSON {
		return printJSON(res)
	}
	if len(res.Pending) == 0 {
		fmt.Println("nothing pending")
		return exitOK
	}
	for _, it := range res.Pending {
		who := it.Identity.Agent
		if who == "" {
			who = "?"
		}
		fmt.Printf("%s  %-8s %-8s %s · %s\n", it.ID, it.Kind, it.Level, who, it.Title)
	}
	return exitOK
}

func runDismiss(args []string) int {
	fs := flag.NewFlagSet("dismiss", flag.ExitOnError)
	all := fs.Bool("all", false, "retire every pending item")
	asJSON := fs.Bool("json", false, "machine-readable output")
	fs.Usage = dismissUsage
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	ids := fs.Args()
	if len(ids) == 0 && !*all {
		dismissUsage()
		return exitUsage
	}
	if len(ids) > 0 && *all {
		fmt.Fprintln(os.Stderr, "agentbox dismiss: name items or pass --all, not both")
		return exitUsage
	}

	reqs := make([]proto.DismissParams, 0, max(len(ids), 1))
	// Human: this is the person's own door, which is what unlocks --all. An agent
	// reaches the same method through the retract tool and cannot set this.
	if *all {
		reqs = append(reqs, proto.DismissParams{All: true, Human: true})
	}
	for _, id := range ids {
		reqs = append(reqs, proto.DismissParams{ID: id, Human: true})
	}

	total := 0
	for _, req := range reqs {
		var res proto.DismissResult
		if code := dismissCall(&req, &res); code != exitOK {
			return code
		}
		total += res.Dismissed
		if *asJSON {
			if code := printJSON(res); code != exitOK {
				return code
			}
			continue
		}
		for _, id := range res.IDs {
			fmt.Println("dismissed", id)
		}
	}
	if total == 0 {
		if !*asJSON {
			fmt.Println("nothing pending to dismiss")
		}
		return exitNo
	}
	return exitOK
}

func dismissCall(req, res any) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := client.Dial(ctx, runtimeDir(), func() error {
		// Deliberately no auto-spawn: there is nothing to dismiss in a daemon that
		// was not running, and raising one to be told so would be absurd.
		return fmt.Errorf("no daemon running")
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentbox dismiss: no daemon running, so nothing is pending")
		return exitError
	}
	defer conn.Close()
	if err := conn.Call(ctx, proto.MethodDismiss, req, res); err != nil {
		fmt.Fprintf(os.Stderr, "agentbox dismiss: %v\n", err)
		return exitNo
	}
	return exitOK
}
