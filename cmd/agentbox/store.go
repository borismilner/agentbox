package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/borismilner/agentbox/internal/store"
)

// runStore answers questions about the database on disk without opening it the
// way the daemon does. Today there is one: what schema version it is at, and
// whether THIS build could open it.
//
// It exists for `make rollback` (R-23). The store refuses a database written by
// a newer binary, deliberately (ADR-0005), so a rollback across a migration
// installs a binary that exits on startup and leaves the desktop with no hub at
// the moment somebody is already recovering from something else. The build being
// restored is the only thing that knows which migrations it carries, so it is
// the one asked, and it is asked BEFORE anything is replaced.
//
// The exit code is the answer and the Makefile reads it: 0 this build can open
// the store, 1 it cannot, 4 the question could not be answered. A build old
// enough to predate this command exits 2 on the unknown command, which the
// caller must treat as "cannot check" rather than as consent.
func runStore(args []string) int {
	if len(args) == 0 || args[0] != "schema" {
		fmt.Fprint(os.Stderr, "usage: agentbox store schema [--db PATH]\n")
		return exitUsage
	}
	fs := flag.NewFlagSet("store schema", flag.ExitOnError)
	dbPath := fs.String("db", "", "database to inspect; default is this instance's own store")
	fs.Parse(args[1:])

	path := *dbPath
	if path == "" {
		path = filepath.Join(stateDir(), "agentbox.db")
	}
	onDisk, known, err := store.Inspect(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentbox: cannot read the store at %s: %v\n", path, err)
		return exitError
	}
	fmt.Printf("store schema %d, this build knows %d\n", onDisk, known)
	if onDisk > known {
		fmt.Printf("this build would refuse that store: it was written by a newer agentbox.\n")
		return exitNo
	}
	return exitOK
}
