package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// R-23. `make rollback` installs the previous binary and restarts, and the store
// refuses a database written by a newer one (ADR-0005), so a rollback across a
// migration is a total outage at the moment somebody is already recovering from
// something else. Inspect is what the rollback asks before it touches anything,
// and what it must NOT do matters as much as what it does: answering through
// Open would apply the very migration being asked about.

func TestInspectSeesASchemaThisBuildWouldRefuse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentbox.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (99, '0099_future.sql', 0)`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	onDisk, known, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect on a store from the future: %v", err)
	}
	if onDisk != 99 {
		t.Fatalf("on disk = %d, want 99", onDisk)
	}
	if known >= onDisk {
		t.Fatalf("this build claims %d migrations, which would make the store's 99 openable", known)
	}
	// And the refusal it predicts is real: same file, opened the way the daemon
	// opens it.
	if _, err := Open(path); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("Open = %v, want ErrSchemaTooNew - then Inspect predicts nothing", err)
	}
}

func TestInspectMatchesAStoreItCanOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentbox.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := s.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	onDisk, known, err := Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	if onDisk != want || known != want {
		t.Fatalf("Inspect = (%d on disk, %d known), want both %d", onDisk, known, want)
	}
}

func TestInspectNeitherCreatesNorMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentbox.db")
	onDisk, known, err := Inspect(path)
	if err != nil {
		t.Fatalf("Inspect with no store yet: %v", err)
	}
	if onDisk != 0 || known == 0 {
		t.Fatalf("Inspect = (%d on disk, %d known), want 0 on disk and a build that knows some", onDisk, known)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Inspect created a database at %s; asking a question must not be a write", path)
	}

	// A store one version behind must still read as one version behind. Answering
	// through Open would migrate it here and then report a match, which is the
	// answer that waves a rollback through.
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM schema_migrations WHERE version = ?`, known); err != nil {
		t.Fatal(err)
	}
	s.Close()
	behind, _, err := Inspect(path)
	if err != nil {
		t.Fatal(err)
	}
	if behind != known-1 {
		t.Fatalf("on disk = %d, want %d: something migrated the store", behind, known-1)
	}
}
