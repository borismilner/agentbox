// Package proto defines the wire types and JSON-RPC framing shared by the
// agentbox CLI, daemon and bridges. It has no dependencies outside the stdlib.
package proto

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

type Kind string

const (
	KindNotify  Kind = "notify"
	KindChoice  Kind = "choice"
	KindText    Kind = "text"
	KindConfirm Kind = "confirm"
	KindForm    Kind = "form"
	KindVeto    Kind = "veto"   // act-unless-stopped countdown (FR22)
	KindSecret  Kind = "secret" // masked entry; value bypasses the transcript (FR23)
	KindDiff    Kind = "diff"   // unified diff review: approve/reject + comment (FR33)
	// KindStack is the collapse of a flooding agent's burst (FR30). The daemon
	// makes it and no caller may submit one: it is the only kind whose author is
	// agentbox itself, which is why Validate accepts it (the daemon builds a real
	// item) while the submit path rejects it by name.
	KindStack Kind = "stack"
)

// StackEntry is one item collapsed into a stack card (FR30). It carries what a
// row has to read and nothing more - the item itself stays in the store under
// its own ID, and opening a row promotes that item back onto the screen, which
// is how a blocking call inside a burst still gets answered.
type StackEntry struct {
	ID       string `json:"id"`
	Kind     Kind   `json:"kind"`
	Level    Level  `json:"level,omitempty"`
	Title    string `json:"title"`
	Blocking bool   `json:"blocking,omitempty"`
	AtMS     int64  `json:"at_ms"` // arrival, so the card can say "7 notifications in 20s"
	// Done marks a row whose item has since been resolved - answered from the
	// stack, dismissed, or expired. The row stays (a list that reflows under the
	// pointer is how the wrong thing gets clicked) and goes quiet instead. Without
	// it a question answered through its own row came back to a stack card still
	// saying "waiting on you" underneath the answer that had just shipped.
	Done bool `json:"done,omitempty"`
}

type FieldType string

const (
	FieldChoice FieldType = "choice"
	FieldText   FieldType = "text"
	FieldBool   FieldType = "bool"
)

// Field is one typed entry of a form item (FR26).
type Field struct {
	Key     string    `json:"key"`
	Label   string    `json:"label,omitempty"`
	Type    FieldType `json:"type"`
	Options []string  `json:"options,omitempty"`
	Default string    `json:"default,omitempty"`
}

const MaxFields = 6

type Level string

const (
	LevelInfo    Level = "info"
	LevelSuccess Level = "success"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
	LevelUrgent  Level = "urgent"
)

// Identity names the caller. Agent is required; the daemon fills nothing in.
type Identity struct {
	Agent   string `json:"agent"`
	Project string `json:"project,omitempty"`
	Session string `json:"session,omitempty"`

	// Key names THIS session and only this one (FR83). The other three fields
	// cannot: Agent is the parent process name, Project is a directory
	// basename, and Session is empty unless AgentBox spawned the agent - so two
	// Claude sessions in one repo are the identical triple, and FR74 shipped an
	// ownership check that a same-named second session could walk straight
	// through. Observed live on 2026-08-04: a control run displayed its holder
	// as `timeout`, because that session had been launched under the `timeout`
	// command.
	//
	// The mcp child mints one at startup and stamps it on every call it makes.
	// A CLI caller is not a session and normally has none; it can act on behalf
	// of one with --key.
	Key string `json:"key,omitempty"`

	// Via says what kind of caller this is: an mcp child making a tool call for a
	// model, or a shell. It exists for the discovery rider (FR83), which is a line
	// for a model to read and must therefore be spent on a call that has somewhere
	// to put it. A session's hooks call the CLI with that session's own key several
	// times a minute, and without this each of those calls would consume the news
	// the child was supposed to deliver.
	Via string `json:"via,omitempty"`
}

// The two kinds of caller. Empty means unknown, which is treated as a shell:
// assuming a model is listening when none is would lose the message.
const (
	ViaMCP = "mcp"
	ViaCLI = "cli"
)

// placeholderAgents are the process names that name no agent. Agent is read from
// the parent process, and for anything but an mcp child the parent is whatever
// happened to exec it: a shell, sudo, timeout - and under setsid, init. That last
// one is not hypothetical. The attach recipe in docs/recipes.md is
// `setsid agentbox sync attach`, so every hook-driven row arrived on Boris's
// board labelled `systemd`, and the same mechanism had already displayed a
// control holder as `timeout`.
var placeholderAgents = map[string]bool{
	"": true, "unknown": true, "agent": true,
	"systemd": true, "init": true, "setsid": true,
	"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true, "ksh": true,
	"su": true, "sudo": true, "env": true, "nohup": true, "timeout": true,
	"xargs": true, "make": true, "tmux": true, "screen": true, "script": true,
	"login": true, "systemd-run": true,
	// Terminals, because the walk up the tree from a human's own shell ends at
	// one, and "alacritty" is no more an agent than "zsh" was.
	"gnome-terminal-": true, "gnome-terminal-server": true, "konsole": true,
	"xterm": true, "urxvt": true, "st": true, "alacritty": true, "kitty": true,
	"wezterm": true, "wezterm-gui": true, "foot": true, "footclient": true,
	"tilix": true, "terminator": true, "ghostty": true, "kgx": true,
}

// PlaceholderAgent reports whether a name says nothing about which agent this is.
// It is the licence to rename: a row wearing a placeholder may be relabelled by a
// later call that knows better, and a row with a real name never is.
func PlaceholderAgent(name string) bool { return placeholderAgents[strings.TrimSpace(name)] }

// SameSession reports whether two identities are the same session, for the
// ownership checks that predate the key.
//
// Deliberately lenient in one direction: two identities that BOTH carry a key
// are compared by key alone, which is what fixes the collision. If either side
// has no key, it falls back to agent-name equality, which is exactly today's
// behaviour - a hook script or a Makefile has no key to offer, and tightening
// this would break every keyless caller to fix a case they are not part of.
//
// The sync primitives do NOT use this. They compare Key directly and refuse a
// caller without one, because a lock whose owner is "whatever exec'd the agent"
// is not a lock.
func (i Identity) SameSession(other Identity) bool {
	if i.Key != "" && other.Key != "" {
		return i.Key == other.Key
	}
	return i.Agent == other.Agent
}

type Option struct {
	Label string `json:"label"`
	Desc  string `json:"desc,omitempty"`
}

// Action is a caller-supplied button on a notify item (FR32). Clicking it
// runs Exec locally; the command is shown verbatim on hover so a misleading
// label cannot disguise what it does. No new privilege boundary: the caller
// and the clicker are the same user.
type Action struct {
	Label string `json:"label"`
	Exec  string `json:"exec"`
}

const MaxActions = 3 // buttons fit one card row

// Item is the unit of interaction. The daemon assigns ID; callers leave it
// empty.
type Item struct {
	ID        string   `json:"id,omitempty"`
	Kind      Kind     `json:"kind"`
	Level     Level    `json:"level,omitempty"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Options   []Option `json:"options,omitempty"`
	Fields    []Field  `json:"fields,omitempty"`
	TimeoutS  int      `json:"timeout_s,omitempty"`
	Default   string   `json:"default,omitempty"`
	Strict    bool     `json:"strict,omitempty"`    // disables the reply hatch (FR27)
	Multiline bool     `json:"multiline,omitempty"` // text items: multi-line editor
	Sink      string   `json:"sink,omitempty"`      // secret items: file the daemon writes the value to (0600)
	Stdout    bool     `json:"stdout,omitempty"`    // secret items: also return the value to the caller (opt-in, FR23)
	Actions   []Action `json:"actions,omitempty"`   // notify items: caller-supplied buttons (FR32)
	Cwd       string   `json:"cwd,omitempty"`       // working directory the daemon runs an action's Exec in (FR32)
	Diff      string   `json:"diff,omitempty"`      // diff items: the unified diff to review (FR33)
	// Stack is the burst a stack item collapses (FR30), oldest first. Only kind
	// stack carries it.
	Stack []StackEntry `json:"stack,omitempty"`
	// Speak is a line read out loud when the item announces itself, just after its
	// earcon. It is the agent's own sentence and never the title: agentbox does not
	// read a screen aloud, so what is heard is what an agent decided was worth
	// hearing. Empty means the earcon alone, which is every item by default.
	Speak    string   `json:"speak,omitempty"`
	Identity Identity `json:"identity"`
}

// Result is what a blocking call gets back. When Answered is true, exactly
// one of Answer, Reply or Values is set.
type Result struct {
	ID             string            `json:"id"`
	Answered       bool              `json:"answered"`
	Answer         string            `json:"answer,omitempty"`
	Reply          string            `json:"reply,omitempty"`
	Values         map[string]string `json:"values,omitempty"`
	DefaultApplied bool              `json:"default_applied,omitempty"`
	Vetoed         bool              `json:"vetoed,omitempty"`      // veto items (FR22): the user stopped the action
	Secret         string            `json:"secret,omitempty"`      // secret items, only when --stdout: the value (never logged/stored)
	SecretPath     string            `json:"secret_path,omitempty"` // secret items: file the value was written to (0600)
	Approved       bool              `json:"approved,omitempty"`    // diff items (FR33): the reviewer approved; Reply carries any comment
	// Outcome says HOW the question ended, which `answered: false` never did (R-09).
	// A lapsed window, a human pressing Esc and a cancel were one answer, and an
	// agent working unattended has to act on the difference: "he declined" and
	// "nobody was there" call for opposite behaviour. One of the Outcome* constants
	// below; empty for a non-blocking item, which never ends in any of these senses.
	Outcome string `json:"outcome,omitempty"`
}

// How a blocking item ended, for Result.Outcome (R-09). The names are the store's
// states in the three cases where they coincide, on purpose: the audit trail and the
// answer an agent reads should not use two vocabularies for one event.
const (
	OutcomeAnswered  = "answered" // the human answered, including inside the undo grace
	OutcomeExpired   = "expired"  // the window lapsed; a veto that proceeded reads as this too
	OutcomeDismissed = "dismissed"
	OutcomeCancelled = "cancelled"
	// OutcomeCallerGone is recorded for the caller that dropped mid-question (FR45).
	// Nothing reads it: the socket it would be written to is the one that went. It is
	// here so the set is complete and so the daemon's own paths say what happened.
	OutcomeCallerGone = "caller_gone"
)

// Stats summarizes interruption history over a window (FR35): how often
// agents interrupted, how many were questions, and how fast they were
// answered. SinceMS = 0 means all time.
type Stats struct {
	SinceMS        int64       `json:"since_ms"`
	Total          int         `json:"total"`            // every item that surfaced
	Questions      int         `json:"questions"`        // blocking items
	Answered       int         `json:"answered"`         // resolved by a user answer
	MedianAnswerMS int64       `json:"median_answer_ms"` // median time-to-answer, answered items
	ByAgent        []AgentStat `json:"by_agent"`         // busiest first
	ByDay          []DayCount  `json:"by_day"`           // oldest day first
}

// AgentStat is one agent's slice of Stats.
type AgentStat struct {
	Agent          string `json:"agent"`
	Total          int    `json:"total"`
	Questions      int    `json:"questions"`
	Answered       int    `json:"answered"`
	MedianAnswerMS int64  `json:"median_answer_ms"`
}

// DayCount is the interruption count for one calendar day (local time).
type DayCount struct {
	Day   string `json:"day"` // YYYY-MM-DD
	Count int    `json:"count"`
}

const MaxOptions = 9 // options map to the 1-9 number row

// The size caps on what an agent may put in an item (R-04).
//
// Until these existed the only bound in the path was the wire line itself
// (rpc.go's 4 MB scanner limit). Passing it did not produce a refusal: the
// daemon's scanner returned ErrTooLong, Serve returned, and the connection
// closed with NO response written - so the agent got "connection closed during
// agentbox.v1.ask: EOF", naming neither the size nor the field at fault, and
// nothing was stored, so there was not even a history row to find afterwards.
//
// Sized so that a maximal item clears the wire cap with room to spare, the way
// internal/walkthrough/spec.go sizes its own: 2 MB of diff plus 256 kB of body
// plus the rest of the envelope is comfortably under 4 MB. The wrong fix is
// raising the scanner limit, which moves the cliff and leaves the same silence
// at the new edge.
const (
	MaxTitleBytes = 4 << 10   // one line on a card; 4 kB is already absurd for it
	MaxBodyBytes  = 256 << 10 // markdown prose, rendered into a window
	MaxDiffBytes  = 2 << 20   // the same ceiling walkthrough.MaxDiffBytes uses
	MaxSpeakBytes = 4 << 10   // a sentence to read out loud
)

// MaxDiffFiles bounds the STRUCTURE of a review diff, which the byte cap above
// does not (R-26).
//
// The card draws a rail button and a section per file and caps only how many
// LINES render, so 2 MB of nothing but `diff --git` headers - measured at 73,081
// of them, well inside MaxDiffBytes - is 73,081 rail buttons on a card meant to
// be read. (R-26's entry says 200,000 headers at 5 MB. That diff is refused by
// the byte cap already; the reachable shape is a third of it and just as
// unreadable.)
//
// 500 is far past any diff a human reads on a card in one sitting and far short
// of what breaks one. The check itself lives in the daemon, next to the other
// door this package cannot hold: counting files means parsing the diff, and
// proto has no dependencies outside the stdlib.
const MaxDiffFiles = 500

func (it *Item) Validate() error {
	switch it.Kind {
	case KindNotify, KindText, KindConfirm:
	case KindVeto:
		if it.TimeoutS <= 0 {
			return errors.New("veto items need a positive timeout_s (the countdown window)")
		}
	case KindSecret:
		if it.Sink == "" && !it.Stdout {
			return errors.New("secret items need a sink (file path) or stdout=true: where should the value go?")
		}
	case KindDiff:
		if it.Diff == "" {
			return errors.New("diff items need a diff (the unified diff to review)")
		}
	case KindStack:
		if len(it.Stack) == 0 {
			return errors.New("stack items need at least 1 collapsed entry")
		}
	case KindForm:
		if len(it.Fields) == 0 {
			return errors.New("form items need at least 1 field")
		}
		if len(it.Fields) > MaxFields {
			return fmt.Errorf("form items allow at most %d fields", MaxFields)
		}
		seen := map[string]bool{}
		for i, f := range it.Fields {
			if f.Key == "" {
				return fmt.Errorf("field %d has an empty key", i+1)
			}
			if seen[f.Key] {
				return fmt.Errorf("duplicate field key %q", f.Key)
			}
			seen[f.Key] = true
			switch f.Type {
			case FieldChoice:
				if len(f.Options) < 2 || len(f.Options) > MaxOptions {
					return fmt.Errorf("field %q needs 2-%d options", f.Key, MaxOptions)
				}
				if f.Default != "" && !oneOf(f.Options, f.Default) {
					return fmt.Errorf("field %q default %q is not an option", f.Key, f.Default)
				}
			case FieldBool:
				if f.Default != "" && f.Default != "yes" && f.Default != "no" {
					return fmt.Errorf("field %q default must be yes or no", f.Key)
				}
			case FieldText:
			default:
				return fmt.Errorf("field %q has unknown type %q (choice|text|bool)", f.Key, f.Type)
			}
		}
	case KindChoice:
		if len(it.Options) < 2 {
			return errors.New("choice items need at least 2 options")
		}
		if len(it.Options) > MaxOptions {
			return fmt.Errorf("choice items allow at most %d options", MaxOptions)
		}
		for i, o := range it.Options {
			if o.Label == "" {
				return fmt.Errorf("option %d has an empty label", i+1)
			}
		}
		if it.Default != "" && !it.HasOption(it.Default) {
			return fmt.Errorf("default %q is not one of the options", it.Default)
		}
	default:
		return fmt.Errorf("unknown kind %q", it.Kind)
	}
	if it.Title == "" {
		return errors.New("title is required")
	}
	// R-04. Named field, named limit, named actual size: the failure this
	// replaces told the agent none of the three, and an agent that cannot see
	// which field was too big cannot shorten it.
	for _, cap := range []struct {
		field string
		val   string
		max   int
	}{
		{"title", it.Title, MaxTitleBytes},
		{"body", it.Body, MaxBodyBytes},
		{"diff", it.Diff, MaxDiffBytes},
		{"speak", it.Speak, MaxSpeakBytes},
	} {
		if len(cap.val) > cap.max {
			return fmt.Errorf("%s is %d bytes, over the %d-byte limit: "+
				"put the long version in a file and show_document it, or trim it",
				cap.field, len(cap.val), cap.max)
		}
	}
	if it.Identity.Agent == "" {
		return errors.New("identity.agent is required")
	}
	switch it.Level {
	case "", LevelInfo, LevelSuccess, LevelWarning, LevelError, LevelUrgent:
	default:
		return fmt.Errorf("unknown level %q", it.Level)
	}
	if it.TimeoutS < 0 {
		return errors.New("timeout_s must be >= 0")
	}
	if len(it.Actions) > 0 {
		if it.Kind != KindNotify {
			return errors.New("action buttons are only allowed on notify items (FR32)")
		}
		if len(it.Actions) > MaxActions {
			return fmt.Errorf("at most %d action buttons", MaxActions)
		}
		for i, a := range it.Actions {
			if a.Label == "" {
				return fmt.Errorf("action %d has an empty label", i+1)
			}
			if a.Exec == "" {
				return fmt.Errorf("action %q has an empty command", a.Label)
			}
		}
	}
	return nil
}

func (it *Item) HasOption(label string) bool {
	for _, o := range it.Options {
		if o.Label == label {
			return true
		}
	}
	return false
}

func oneOf(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

// FieldLabel is what the UI shows for a field: the label, or the key when
// the caller gave none.
func (f Field) FieldLabel() string {
	if f.Label != "" {
		return f.Label
	}
	return f.Key
}

// Rank orders levels by importance; retention and queue policy compare
// levels through it.
func (l Level) Rank() int {
	switch l {
	case LevelUrgent:
		return 4
	case LevelError:
		return 3
	case LevelWarning:
		return 2
	case LevelSuccess:
		return 1
	default:
		return 0
	}
}

// EffectiveLevel returns the level with the kind-appropriate default applied.
func (it *Item) EffectiveLevel() Level {
	if it.Level != "" {
		return it.Level
	}
	return LevelInfo
}

// Blocking reports whether the caller waits for a user response. A stack card
// has no caller of its own - it is agentbox's own summary of a burst, and the
// blocking calls inside it wait on their own items - so it belongs with notify
// rather than with the questions.
func (it *Item) Blocking() bool {
	return it.Kind != KindNotify && it.Kind != KindStack
}

// ClipboardText renders the item for pasting into an agent conversation
// (FR43): plain text, every field, no decoration to strip.
func (it *Item) ClipboardText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "[agentbox %s] %s/%s", it.ID, it.Kind, it.EffectiveLevel())
	fmt.Fprintf(&b, "\nfrom: %s", it.Identity.Agent)
	if it.Identity.Project != "" {
		fmt.Fprintf(&b, " (%s)", it.Identity.Project)
	}
	if it.Identity.Session != "" {
		fmt.Fprintf(&b, " session=%s", it.Identity.Session)
	}
	fmt.Fprintf(&b, "\ntitle: %s", it.Title)
	if it.Body != "" {
		fmt.Fprintf(&b, "\nbody: %s", it.Body)
	}
	if len(it.Options) > 0 {
		b.WriteString("\noptions:")
		for i, o := range it.Options {
			fmt.Fprintf(&b, "\n  %d. %s", i+1, o.Label)
			if o.Desc != "" {
				fmt.Fprintf(&b, " (%s)", o.Desc)
			}
		}
	}
	if len(it.Fields) > 0 {
		b.WriteString("\nfields:")
		for _, f := range it.Fields {
			fmt.Fprintf(&b, "\n  %s (%s)", f.Key, f.Type)
			if len(f.Options) > 0 {
				fmt.Fprintf(&b, ": %s", strings.Join(f.Options, " | "))
			}
		}
	}
	if it.Default != "" {
		fmt.Fprintf(&b, "\ndefault: %s", it.Default)
	}
	if it.TimeoutS > 0 {
		fmt.Fprintf(&b, "\ntimeout_s: %d", it.TimeoutS)
	}
	if len(it.Actions) > 0 {
		b.WriteString("\nactions:")
		for _, a := range it.Actions {
			fmt.Fprintf(&b, "\n  %s -> %s", a.Label, a.Exec)
		}
	}
	return b.String()
}
