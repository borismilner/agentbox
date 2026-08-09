package store

import (
	"slices"
	"strings"
	"testing"
)

// R-11. One unreadable JSON blob in options, fields, actions or form_values
// failed the WHOLE read. Pending() failed, daemon.New returned that error, the
// process exited, and every auto-spawn repeated it - total outage from one bad
// row, with the only evidence a line in a log file, because an auto-spawned
// daemon sends its stderr nowhere.
//
// A corrupt row is one lost item. Refusing to start is all of them.

// corrupt writes a blob directly, which is the only way to produce a row the
// encoder would never have written. Every column the read decodes gets a turn.
func corrupt(t *testing.T, s *Store, id, column, blob string) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE items SET `+column+` = ? WHERE id = ?`, blob, id); err != nil {
		t.Fatalf("corrupting %s: %v", column, err)
	}
}

func TestOneCorruptRowDoesNotHideTheOthers(t *testing.T) {
	for _, column := range []string{"options", "fields", "actions", "stack", "form_values"} {
		t.Run(column, func(t *testing.T) {
			s := openTemp(t)
			for _, id := range []string{"good-1", "bad", "good-2"} {
				if err := s.CreateItem(testItem(id)); err != nil {
					t.Fatal(err)
				}
			}
			corrupt(t, s, "bad", column, "{")

			got, err := s.Pending()
			if err != nil {
				t.Fatalf("Pending failed on one bad row: %v\n"+
					"that is the whole defect: one unreadable blob stopped the daemon starting", err)
			}
			var ids []string
			for _, it := range got {
				ids = append(ids, it.ID)
			}
			if len(got) != 2 {
				t.Fatalf("got %v, want the two readable rows", ids)
			}
			for _, want := range []string{"good-1", "good-2"} {
				if !slices.Contains(ids, want) {
					t.Errorf("%s is missing from %v; a bad row took a good one with it", want, ids)
				}
			}
			if slices.Contains(ids, "bad") {
				t.Errorf("the corrupt row was returned in %v; it cannot be decoded, so it cannot be shown", ids)
			}
		})
	}
}

// The skip must be SAID. A row silently dropped is the same silence this
// replaced, one item smaller, and the human has no other way to learn that a
// question never arrived.
func TestASkippedRowIsReportedOnceAndDrains(t *testing.T) {
	s := openTemp(t)
	for _, id := range []string{"good", "bad-1", "bad-2"} {
		if err := s.CreateItem(testItem(id)); err != nil {
			t.Fatal(err)
		}
	}
	corrupt(t, s, "bad-1", "options", "{")
	corrupt(t, s, "bad-2", "actions", "not json at all")

	if _, err := s.Pending(); err != nil {
		t.Fatal(err)
	}
	skipped := s.Skipped()
	if len(skipped) != 2 {
		t.Fatalf("Skipped() = %v, want both bad rows", skipped)
	}
	joined := strings.Join(skipped, " ")
	for _, want := range []string{"bad-1", "bad-2", "options", "actions"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Skipped() = %v, missing %q: the card has to name the row AND the column, "+
				"or nobody can find what was lost", skipped, want)
		}
	}
	// Drained, so a later read does not raise the same card again.
	if again := s.Skipped(); len(again) != 0 {
		t.Errorf("Skipped() still reports %v after being drained", again)
	}
}

// A pre-migration-0013 stack is "" and means "not a stack". It already had this
// treatment and must keep it: reading it as corruption would skip every row in a
// store older than FR30, which is the defect at a far larger scale.
func TestAnEmptyStackIsNotCorruption(t *testing.T) {
	s := openTemp(t)
	if err := s.CreateItem(testItem("old")); err != nil {
		t.Fatal(err)
	}
	corrupt(t, s, "old", "stack", "")

	got, err := s.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "old" {
		t.Fatalf("got %+v, want the pre-migration row read normally", got)
	}
	if s := s.Skipped(); len(s) != 0 {
		t.Errorf("an empty stack was counted as corruption: %v", s)
	}
}

// The recorded list is bounded. A comprehensively corrupt store must not turn
// one bad startup into an unbounded slice and a card nobody can read.
func TestTheSkippedListIsBounded(t *testing.T) {
	s := openTemp(t)
	for i := range 60 {
		it := testItem("bad-" + string(rune('a'+i%26)) + string(rune('a'+i/26)))
		if err := s.CreateItem(it); err != nil {
			t.Fatal(err)
		}
		corrupt(t, s, it.ID, "options", "{")
	}
	if _, err := s.Pending(); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Skipped()); n == 0 || n > 32 {
		t.Errorf("Skipped() reported %d rows; want some, capped at 32", n)
	}
}
