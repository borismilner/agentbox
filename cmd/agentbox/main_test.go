package main

import (
	"flag"
	"testing"

	"github.com/borismilner/agentbox/internal/proto"
)

func TestParseProgressLine(t *testing.T) {
	cases := []struct {
		line    string
		percent int
		status  string
		ok      bool
	}{
		{"47", 47, "", true},
		{"47%", 47, "", true},
		{"47 migrating users", 47, "migrating users", true},
		{"100%  done", 100, "done", true},
		{"150", 100, "", true},          // clamps high
		{"-5", 0, "", true},             // clamps low
		{"connecting...", 0, "", false}, // not a percent line
		{"", 0, "", false},              // empty token
		{"3   rows left", 3, "rows left", true},
	}
	for _, c := range cases {
		pct, status, ok := parseProgressLine(c.line)
		if ok != c.ok || (ok && (pct != c.percent || status != c.status)) {
			t.Errorf("parseProgressLine(%q) = (%d, %q, %v), want (%d, %q, %v)",
				c.line, pct, status, ok, c.percent, c.status, c.ok)
		}
	}
}

func TestParseFieldSpec(t *testing.T) {
	cases := []struct {
		spec    string
		want    proto.Field
		wantErr bool
	}{
		{spec: "choice:env:staging,prod", want: proto.Field{Type: proto.FieldChoice, Key: "env", Options: []string{"staging", "prod"}}},
		{spec: "choice:env:staging,prod=staging", want: proto.Field{Type: proto.FieldChoice, Key: "env", Options: []string{"staging", "prod"}, Default: "staging"}},
		{spec: "text:tag", want: proto.Field{Type: proto.FieldText, Key: "tag"}},
		{spec: "bool:notify=yes", want: proto.Field{Type: proto.FieldBool, Key: "notify", Default: "yes"}},
		{spec: "slider:volume", wantErr: true},
		{spec: "justakey", wantErr: true},
	}
	for _, tc := range cases {
		got, err := parseFieldSpec(tc.spec)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected error", tc.spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tc.spec, err)
			continue
		}
		if got.Type != tc.want.Type || got.Key != tc.want.Key || got.Default != tc.want.Default ||
			len(got.Options) != len(tc.want.Options) {
			t.Errorf("%q: got %+v, want %+v", tc.spec, got, tc.want)
		}
	}
}

// A flag written after the file name must still count: `agentbox show FILE --watch`
// is the order people type, and Go's flag package would otherwise stop at FILE
// and leave --watch as a silently ignored positional.
func TestParsePositionalAcceptsFlagsAfterArgs(t *testing.T) {
	cases := [][]string{
		{"--watch", "doc.md"},
		{"doc.md", "--watch"},
		{"doc.md", "--watch", "--title", "T"},
		{"--title", "T", "doc.md", "--watch"},
	}
	for _, args := range cases {
		fs := flag.NewFlagSet("show", flag.ContinueOnError)
		watch := fs.Bool("watch", false, "")
		title := fs.String("title", "", "")
		pos := parsePositional(fs, args)
		if first(pos) != "doc.md" {
			t.Errorf("%v: positional = %q, want doc.md", args, first(pos))
		}
		if !*watch {
			t.Errorf("%v: --watch was dropped", args)
		}
		if len(args) > 2 && *title != "T" {
			t.Errorf("%v: --title = %q, want T", args, *title)
		}
	}
}

func TestFirstPositional(t *testing.T) {
	if got := first(nil); got != "" {
		t.Errorf("first(nil) = %q, want empty", got)
	}
	if got := first([]string{"a", "b"}); got != "a" {
		t.Errorf("first = %q, want a", got)
	}
}
