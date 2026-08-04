package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/borismilner/agentbox/internal/proto"
)

// `agentbox sync get|set|del` (FR83 slice 4): the blackboard from a shell.
//
// This is the door that makes a claim table reachable by things that are not Claude
// sessions - a Makefile splitting a build, a hook, a non-Claude agent - and the
// output is shaped for a shell rather than for a reader. A lost compare-and-swap
// exits 1 with the current value on stdout, which is exactly the loop a claiming
// script wants:
//
//	for chunk in $(seq 0 9); do
//	  agentbox sync set "claims/$chunk" "$AGENT" --if-version 0 --own >/dev/null || continue
//	  do_work "$chunk" && agentbox sync del "claims/$chunk"
//	done
//
// Nothing here parks, so unlike `sync await` there is no ceiling to explain and no
// cursor to carry.

type sharedCLI struct {
	verb      string
	id        proto.Identity
	key       string
	value     string
	ifVersion string
	own       bool
	asJSON    bool
}

func runSyncShared(in sharedCLI) int {
	op := proto.SharedOpGet
	switch in.verb {
	case "set":
		op = proto.SharedOpSet
	case "del", "delete":
		op = proto.SharedOpDelete
	}

	req := proto.SyncSharedParams{Identity: in.id, Op: op, Key: in.key, Own: in.own}
	// Parsed rather than defaulted: an empty flag means "no condition", and "0" means
	// "only if absent". Collapsing the two would turn a claim into an overwrite.
	if in.ifVersion != "" {
		n, err := strconv.ParseInt(strings.TrimSpace(in.ifVersion), 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentbox sync %s: --if-version wants a number (0 claims a key nobody has yet)\n", in.verb)
			return exitNo
		}
		req.IfVersion = &n
	}
	if op == proto.SharedOpSet {
		if strings.TrimSpace(in.value) == "" {
			fmt.Fprintln(os.Stderr, "agentbox sync set: wants a value after the key")
			return exitNo
		}
		// The same rule `sync post` uses for a payload: JSON travels as JSON, anything
		// else as a JSON string, and a caller that has a word does not have to know
		// which agentbox wanted.
		req.Value = jsonOrString(in.value)
	}
	if op != proto.SharedOpGet && in.id.Key == "" {
		fmt.Fprintf(os.Stderr, "agentbox sync %s: wants --key (or AGENTBOX_SESSION_KEY): one session, one key\n", in.verb)
		return exitNo
	}

	var res proto.SyncSharedResult
	if code := syncCLICall(proto.MethodSyncShared, &req, &res); code != exitOK {
		return code
	}
	if in.asJSON {
		if code := printJSON(res); code != exitOK {
			return code
		}
		return sharedExit(op, res)
	}

	switch op {
	case proto.SharedOpGet:
		if len(res.Values) > 0 {
			for _, v := range res.Values {
				fmt.Println(sharedLine(v))
			}
			if res.More {
				fmt.Println("more keys under this prefix than were returned; read a narrower one")
			}
			return exitOK
		}
		if !res.Found {
			// A prefix that matches nothing and a key that is not there are different
			// facts, and only one of them has a version. The daemon's note already draws
			// the distinction, so use it rather than restating half of it.
			if strings.HasSuffix(in.key, "*") {
				fmt.Println(res.Note)
			} else {
				fmt.Printf("%s does not exist (version 0)\n", in.key)
			}
			return exitNo
		}
		fmt.Println(sharedLine(*res.Value))
	default:
		if res.Value != nil {
			fmt.Println(sharedLine(*res.Value))
		}
		if res.Stale {
			// To stderr, so a script's stdout stays the value it can act on while a
			// human still sees why the write did not land.
			fmt.Fprintln(os.Stderr, res.Note)
		}
	}
	return sharedExit(op, res)
}

// sharedExit maps the house grammar onto a compare-and-swap: a refused write is 1,
// the same as a lock that was not granted, because both mean "somebody else has it"
// rather than "agentbox failed".
func sharedExit(op string, res proto.SyncSharedResult) int {
	if op == proto.SharedOpGet {
		if res.Found {
			return exitOK
		}
		return exitNo
	}
	if res.Applied {
		return exitOK
	}
	return exitNo
}

// sharedLine is one value as a shell shows it. The owner is named only when there is
// one, and a dead owner is said out loud: an orphaned claim is the whole reason
// ownership is recorded, so it must not be something the reader has to notice.
func sharedLine(v proto.SharedValue) string {
	line := fmt.Sprintf("%s v%d %s", v.Key, v.Version, string(v.Value))
	switch {
	case v.OwnerGone:
		who := v.OwnerAgent
		if who == "" {
			who = v.Owner
		}
		line += fmt.Sprintf(" (owner %s is gone - this work was abandoned, not finished)", who)
	case v.OwnerAgent != "":
		line += " (owner " + v.OwnerAgent + ")"
	case v.Owner != "":
		line += " (owner " + v.Owner + ")"
	}
	return line
}
