package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/borismilner/agentbox/internal/client"
	"github.com/borismilner/agentbox/internal/proto"
)

// runWalkthrough is the CLI face of the walkthrough family (FR58/FR59):
// durable reviews the board renders. Verbs mirror the daemon methods;
// amend joins when the amendment round builds it.
func runWalkthrough(args []string) int {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "create", "open", "list", "read", "await", "delete", "repair":
	default:
		fmt.Fprint(os.Stderr, "agentbox walkthrough: say `create`, `open`, `list`, `read`, `await`, `delete` or `repair`\n"+
			"example: agentbox walkthrough create --spec review.json\n")
		return exitUsage
	}

	fs := flag.NewFlagSet("walkthrough "+sub, flag.ExitOnError)
	var (
		specPath = fs.String("spec", "-", "spec JSON file, - for stdin (create)")
		noShow   = fs.Bool("no-show", false, "store without opening the board (create)")
		find     = fs.String("find", "", "match title, step content or cited paths (list)")
		state    = fs.String("state", "", "filter: open, submitted or delivered (list)")
		limit    = fs.Int("limit", 20, "rows to return (list)")
		ack      = fs.Bool("ack", false, "take a waiting submission, exactly once (read)")
		timeout  = fs.Int("timeout", 0, "seconds to wait before giving up; 0 waits forever (await)")
		asJSON   = fs.Bool("json", false, "machine-readable output")
	)
	identity := identityFlags(fs)
	rest := parsePositional(fs, args)

	// open falls back to the most recently touched review: reopening the one
	// you were reading is the common case, and having to look its id up first
	// is friction on the path a human takes every time they come back.
	needID := sub == "read" || sub == "delete"
	id := ""
	if len(rest) > 0 {
		id = rest[0]
	}
	if needID && id == "" {
		fmt.Fprintf(os.Stderr, "agentbox walkthrough %s: which one? pass the id, e.g. agentbox walkthrough %s w3f9c2a1b4d5\n", sub, sub)
		return exitUsage
	}

	ctx := context.Background()
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	conn, err := client.Dial(dialCtx, runtimeDir(), nil)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot reach daemon: %v\n", err)
		return exitError
	}
	defer conn.Close()

	switch sub {
	case "create":
		var raw []byte
		if *specPath == "-" {
			raw, err = io.ReadAll(os.Stdin)
		} else {
			raw, err = os.ReadFile(*specPath)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: read spec: %v\n", err)
			return exitError
		}
		wid, err := proto.NewWalkthroughID()
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
			return exitError
		}
		req := proto.WalkthroughCreate{ID: wid, Spec: raw, NoShow: *noShow, Identity: *identity}
		var res proto.WalkthroughCreateResult
		if err := conn.Call(ctx, proto.MethodWalkthroughCreate, req, &res); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
			return exitError
		}
		for _, w := range res.Warnings {
			fmt.Fprintf(os.Stderr, "agentbox: warning: %s\n", w)
		}
		if *asJSON {
			return printJSON(res)
		}
		fmt.Println(res.ID)
		return exitOK

	case "open":
		if id == "" {
			var lst proto.WalkthroughListResult
			if err := conn.Call(ctx, proto.MethodWalkthroughList, proto.WalkthroughList{Limit: 1}, &lst); err != nil {
				fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
				return exitError
			}
			if len(lst.Walkthroughs) == 0 {
				fmt.Fprint(os.Stderr, "agentbox walkthrough open: the library is empty; create one first\n")
				return exitUsage
			}
			id = lst.Walkthroughs[0].ID
			fmt.Fprintf(os.Stderr, "agentbox: opening the most recent review, %s (%s)\n", id, lst.Walkthroughs[0].Title)
		}
		var res map[string]bool
		if err := conn.Call(ctx, proto.MethodWalkthroughOpen, proto.WalkthroughOpen{ID: id}, &res); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
			return exitError
		}
		return exitOK

	case "list":
		req := proto.WalkthroughList{Query: *find, State: *state, Limit: *limit}
		var res proto.WalkthroughListResult
		if err := conn.Call(ctx, proto.MethodWalkthroughList, req, &res); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
			return exitError
		}
		if *asJSON {
			return printJSON(res)
		}
		for _, w := range res.Walkthroughs {
			line := fmt.Sprintf("%s  %-9s  %d/%d understood", w.ID, w.State, w.Understood, w.CountedSteps)
			if w.Unclear > 0 {
				line += fmt.Sprintf(" · %d unclear", w.Unclear)
			}
			if w.Comments > 0 {
				line += fmt.Sprintf(" · %d comments", w.Comments)
			}
			fmt.Printf("%s  %s\n", line, w.Title)
		}
		return exitOK

	case "read":
		req := proto.WalkthroughRead{ID: id, Ack: *ack}
		var res proto.WalkthroughState
		if err := conn.Call(ctx, proto.MethodWalkthroughRead, req, &res); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
			return exitError
		}
		return printJSON(res)

	case "await":
		// The whole point is to block on the human; only the daemon's own
		// timeout ends the wait, so the call rides a plain context.
		req := proto.WalkthroughAwait{ID: id, TimeoutS: *timeout, Identity: *identity}
		var res proto.WalkthroughAwaitResult
		if err := conn.Call(ctx, proto.MethodWalkthroughAwait, req, &res); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
			return exitError
		}
		switch {
		case res.Submitted:
			return printJSON(res.Payload)
		case res.Gone:
			fmt.Fprintln(os.Stderr, "agentbox: the walkthrough was deleted while you waited")
			return exitError
		default:
			fmt.Fprintln(os.Stderr, "agentbox: no submission within the window")
			return exitUnanswered
		}

	case "repair":
		// Reviews written before agentbox captured its citations kept only line
		// numbers, so they read whatever the file says today. This puts the
		// pinned commit's version back, for one review or for the library.
		var res proto.WalkthroughRepairResult
		if err := conn.Call(ctx, proto.MethodWalkthroughRepair, proto.WalkthroughRepair{ID: id}, &res); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
			return exitError
		}
		if *asJSON {
			return printJSON(res)
		}
		if len(res.Repaired) == 0 {
			fmt.Println("nothing to repair: every citation already has its source")
			return exitOK
		}
		stuck := 0
		for _, r := range res.Repaired {
			fmt.Printf("%s  %d recovered, %d still missing  %s\n", r.ID, r.Recovered, r.Missing, r.Title)
			for _, n := range r.Notes {
				fmt.Printf("    %s\n", n)
			}
			stuck += r.Missing
		}
		if stuck > 0 {
			// Not an error: a range git cannot serve is a fact about the clone,
			// and the board still falls back to the working tree for it.
			fmt.Fprintf(os.Stderr, "\n%d range(s) could not be recovered - those blocks still read the working tree\n", stuck)
		}
		return exitOK

	case "delete":
		var res proto.WalkthroughDeleteResult
		if err := conn.Call(ctx, proto.MethodWalkthroughDelete, proto.WalkthroughDelete{ID: id}, &res); err != nil {
			fmt.Fprintf(os.Stderr, "agentbox: %v\n", err)
			return exitError
		}
		if !res.Deleted {
			fmt.Fprintf(os.Stderr, "agentbox: no walkthrough %s\n", id)
			return exitError
		}
		return exitOK
	}
	return exitUsage
}
