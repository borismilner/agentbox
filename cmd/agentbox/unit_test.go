package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borismilner/agentbox/internal/logging"
)

// R-24. A crash loop ends with systemd hitting its start limit and giving up,
// and both halves of that were invisible: nothing reported the unit state, and a
// daemon that died on startup left the JSONL with a daemon.start row and no
// explanation, because the reason went to stderr and stderr is the journal.

// This is the block `systemctl --user show agentbox.service -p ActiveState -p
// UnitFileState` printed on this machine, kept verbatim so the parser is tested
// against what systemd actually writes rather than against what it was assumed
// to write.
const liveShowOutput = "ActiveState=active\nUnitFileState=enabled\n"

func TestParseUnitShowReadsWhatSystemdPrints(t *testing.T) {
	active, fileState := parseUnitShow(liveShowOutput)
	if active != "active" || fileState != "enabled" {
		t.Fatalf("parsed (%q, %q), want (active, enabled)", active, fileState)
	}
	// A field the unit does not have comes back empty rather than as a wrong
	// guess, and a value containing an = survives.
	active, fileState = parseUnitShow("ActiveState=failed\nDescription=a=b\n")
	if active != "failed" || fileState != "" {
		t.Fatalf("parsed (%q, %q), want (failed, )", active, fileState)
	}
}

func TestUnitNoteSpeaksOnlyWhenSomethingIsWrong(t *testing.T) {
	cases := []struct {
		name       string
		active     string
		fileState  string
		daemonUp   bool
		wantSaid   bool
		wantSubstr string
	}{
		{name: "healthy", active: "active", fileState: "enabled", daemonUp: true},
		{name: "nobody enabled it", active: "inactive", fileState: "disabled", daemonUp: true},
		{name: "no unit installed", active: "", fileState: "", daemonUp: false},
		{
			name: "systemd gave up, a daemon still answers", active: "failed", fileState: "enabled",
			daemonUp: true, wantSaid: true, wantSubstr: "auto-spawned",
		},
		{
			name: "systemd gave up and nothing answers", active: "failed", fileState: "enabled",
			daemonUp: false, wantSaid: true, wantSubstr: "reset-failed",
		},
		{
			name: "enabled but not running", active: "inactive", fileState: "enabled",
			daemonUp: false, wantSaid: true, wantSubstr: "systemctl --user start",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := unitNote(c.active, c.fileState, c.daemonUp)
			if said := got != ""; said != c.wantSaid {
				t.Fatalf("note = %q, want said = %v", got, c.wantSaid)
			}
			if c.wantSubstr != "" && !strings.Contains(got, c.wantSubstr) {
				t.Fatalf("note = %q, want it to mention %q", got, c.wantSubstr)
			}
		})
	}
}

func TestUnitAdviceIsSilentForANamedInstance(t *testing.T) {
	// A named instance is a daemon of its own (NFR12) and the unit manages the
	// default one, so anything the unit says here would be about another process.
	t.Setenv("AGENTBOX_INSTANCE", "dev")
	if got := unitAdvice(false); got != "" {
		t.Fatalf("advice = %q, want nothing said about another daemon's unit", got)
	}
}

func TestFatalWritesTheReasonWhereAnEngineerReadsIt(t *testing.T) {
	dir := t.TempDir()
	log, closer, err := logging.Open(dir, slog.LevelInfo, 8)
	if err != nil {
		t.Fatal(err)
	}
	var code int
	exitProcess = func(c int) { code = c }
	t.Cleanup(func() { exitProcess = os.Exit })

	fatal(log, closer, "the store could not be opened", errSchemaTooNewLike{})
	if code != exitError {
		t.Fatalf("exit code %d, want %d", code, exitError)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "log", logging.FileName))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		if row["msg"] != logging.EvDaemonStop {
			continue
		}
		found = true
		if row["reason"] != "the store could not be opened" {
			t.Fatalf("stop row carries reason %v", row["reason"])
		}
		if row["err"] != "schema is newer than this binary" {
			t.Fatalf("stop row carries err %v", row["err"])
		}
		if row["graceful"] != false {
			t.Fatalf("stop row claims graceful = %v", row["graceful"])
		}
	}
	if !found {
		// This is the crash loop as the log recorded it before: starts, and
		// nothing at all after them.
		t.Fatalf("no %s row in the log; the reason went only to stderr:\n%s", logging.EvDaemonStop, raw)
	}
}

// The restart budget is only a decision if systemd reads it, and the way it
// silently does not is a section: StartLimitIntervalSec and StartLimitBurst moved
// to [Unit] in systemd 229, and in [Service] they are parsed, warned about once
// in a log nobody keeps, and ignored. `systemd-analyze verify` says "Unknown key
// name 'StartLimitIntervalSec' in section 'Service', ignoring" and still exits 0,
// so nothing in a build would catch it either.
func TestTheUnitsRestartBudgetIsInTheSectionSystemdReads(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "packaging", unitName))
	if err != nil {
		t.Fatal(err)
	}
	section := ""
	found := map[string]string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		if key == "StartLimitIntervalSec" || key == "StartLimitBurst" {
			found[key] = section
		}
	}
	for _, key := range []string{"StartLimitIntervalSec", "StartLimitBurst"} {
		switch found[key] {
		case "[Unit]":
		case "":
			t.Fatalf("%s is not set at all, so the unit inherits five starts in ten seconds and gives up eight seconds into a crash loop", key)
		default:
			t.Fatalf("%s is in %s, where systemd ignores it", key, found[key])
		}
	}
}

type errSchemaTooNewLike struct{}

func (errSchemaTooNewLike) Error() string { return "schema is newer than this binary" }
