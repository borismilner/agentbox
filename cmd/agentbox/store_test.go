package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/borismilner/agentbox/internal/store"
)

// bumpSchema writes a migration row no build has ever applied, which is what a
// store written by a newer agentbox looks like to an older one.
func bumpSchema(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (99, '0099_future.sql', 0)`); err != nil {
		t.Fatal(err)
	}
}

// R-23. `make rollback` reads these exit codes and nothing else, so they are the
// contract: 0 the build can open the store, 1 it cannot and the rollback must
// stop, anything else the question could not be answered.

func TestStoreSchemaAgreesWithABuildThatCanOpenIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentbox.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	if code := runStore([]string{"schema", "--db", path}); code != exitOK {
		t.Fatalf("exit %d on a store this build wrote, want %d", code, exitOK)
	}
}

func TestStoreSchemaRefusesADatabaseFromTheFuture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentbox.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	// The shape of a rollback across a migration, seen from the older binary: the
	// file on disk carries a version this build has never heard of.
	bumpSchema(t, path)

	if code := runStore([]string{"schema", "--db", path}); code != exitNo {
		t.Fatalf("exit %d on a store from the future, want %d - a rollback would be waved through", code, exitNo)
	}
}

func TestStoreSchemaOnAMissingStoreIsFine(t *testing.T) {
	// A fresh machine has no store, and a rollback there is never a schema
	// problem. Refusing here would block the one rollback that is always safe.
	path := filepath.Join(t.TempDir(), "agentbox.db")
	if code := runStore([]string{"schema", "--db", path}); code != exitOK {
		t.Fatalf("exit %d with no store at all, want %d", code, exitOK)
	}
}

func TestStoreWithoutASubcommandIsAUsageError(t *testing.T) {
	if code := runStore(nil); code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
}
