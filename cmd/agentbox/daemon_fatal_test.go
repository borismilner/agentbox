//go:build !noui

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

// fatal lives in daemon.go, which the client-only build (-tags noui) leaves out.

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

type errSchemaTooNewLike struct{}

func (errSchemaTooNewLike) Error() string { return "schema is newer than this binary" }
