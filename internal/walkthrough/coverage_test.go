package walkthrough

import "testing"

// A two-file diff: one hunk the review stands on, one it never mentions.
const twoFileDiff = `diff --git a/internal/daemon/daemon.go b/internal/daemon/daemon.go
--- a/internal/daemon/daemon.go
+++ b/internal/daemon/daemon.go
@@ -10,3 +10,5 @@ func one() {
 	keep()
+	added()
+	more()
 	tail()
diff --git a/internal/webui/settings.go b/internal/webui/settings.go
--- a/internal/webui/settings.go
+++ b/internal/webui/settings.go
@@ -100,2 +100,3 @@ func two() {
 	before()
+	knob()
`

func TestCoverageCountsWhatTheStepsStoodOn(t *testing.T) {
	cov := Cover(twoFileDiff, []Citation{
		{Path: "internal/daemon/daemon.go", From: 8, To: 16},
	}, nil)

	if !cov.Computed {
		t.Fatal("a diff was passed and nothing was computed")
	}
	if cov.Hunks != 2 || cov.Covered != 1 {
		t.Errorf("hunks=%d covered=%d, want 2 and 1", cov.Hunks, cov.Covered)
	}
	if len(cov.Uncovered) != 1 {
		t.Fatalf("uncovered=%+v, want the settings hunk alone", cov.Uncovered)
	}
	got := cov.Uncovered[0]
	if got.Path != "internal/webui/settings.go" || got.From != 100 || got.To != 102 || got.Kind != "add" {
		t.Errorf("uncovered hunk is %+v", got)
	}
}

func TestNoDiffIsUncomputedRatherThanClean(t *testing.T) {
	// The failure mode this exists to prevent: a walkthrough with no manifest
	// reporting "0 uncovered", which is the most misleading answer available.
	cov := Cover("   \n", []Citation{{Path: "a.go", From: 1, To: 2}}, nil)
	if cov.Computed {
		t.Error("no diff, and it claimed to have computed something")
	}
	if cov.Uncovered == nil {
		t.Error("uncovered must always be present, empty rather than nil")
	}
}

func TestATouchingCitationCoversTheHunk(t *testing.T) {
	// Overlap, not containment: a step that cites the lines around a change has
	// stood on it, and demanding exact spans would make the arithmetic disagree
	// with every honest review.
	for _, c := range []Citation{
		{Path: "internal/daemon/daemon.go", From: 1, To: 10},  // laps the top
		{Path: "internal/daemon/daemon.go", From: 14, To: 40}, // laps the bottom
		{Path: "internal/daemon/daemon.go", From: 11, To: 12}, // sits inside
	} {
		cov := Cover(twoFileDiff, []Citation{c}, nil)
		if cov.Covered != 1 {
			t.Errorf("citation %d-%d covered %d hunks, want 1", c.From, c.To, cov.Covered)
		}
	}
	// And one that misses it entirely does not.
	cov := Cover(twoFileDiff, []Citation{{Path: "internal/daemon/daemon.go", From: 40, To: 60}}, nil)
	if cov.Covered != 0 {
		t.Errorf("a citation past the hunk covered %d", cov.Covered)
	}
}

func TestAStatedExclusionIsNotAHole(t *testing.T) {
	cov := Cover(twoFileDiff, nil, []Scope{
		{Paths: "internal/webui/**", Reason: "the surface is unchanged in behaviour"},
	})
	if cov.OutOfScope != 1 {
		t.Errorf("out_of_scope=%d, want the webui hunk", cov.OutOfScope)
	}
	if len(cov.Uncovered) != 1 || cov.Uncovered[0].Path != "internal/daemon/daemon.go" {
		t.Errorf("uncovered=%+v, want the daemon hunk alone", cov.Uncovered)
	}

	// One file, narrowed to lines: a scope that misses the hunk excludes nothing.
	near := Cover(twoFileDiff, nil, []Scope{
		{Path: "internal/webui/settings.go", Lines: [2]int{1, 20}, Reason: "the header"},
	})
	if near.OutOfScope != 0 {
		t.Errorf("a line-scoped exclusion swallowed a hunk it does not cover")
	}
}

func TestADeletedFileIsNotCountedAgainstTheAuthor(t *testing.T) {
	// There is no new-side file left to cite. Counting it as uncovered would only
	// teach the habit of writing an out_of_scope entry for every deletion.
	const gone = `diff --git a/old/thing.go b/old/thing.go
deleted file mode 100644
--- a/old/thing.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package old
-
-func gone() {}
`
	cov := Cover(gone, nil, nil)
	if cov.Hunks != 0 || len(cov.Uncovered) != 0 {
		t.Errorf("hunks=%d uncovered=%+v, want the deletion excluded", cov.Hunks, cov.Uncovered)
	}
	if len(cov.Deleted) != 1 || cov.Deleted[0] != "old/thing.go" {
		t.Errorf("deleted=%v, want it named so the author can say what went", cov.Deleted)
	}
}

func TestAPureDeletionHunkCanStillBeCovered(t *testing.T) {
	// `@@ -10,4 +9,0 @@` has no new-side lines at all, so its span collapses to
	// the seam the deletion left. Without that it could never be covered by
	// anything, and every review that removed code would read as incomplete.
	const cut = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -10,4 +9,0 @@ func f() {
-	one()
-	two()
-	three()
-	four()
`
	cov := Cover(cut, []Citation{{Path: "a.go", From: 5, To: 12}}, nil)
	if cov.Hunks != 1 || cov.Covered != 1 {
		t.Errorf("hunks=%d covered=%d, want a covered deletion", cov.Hunks, cov.Covered)
	}
	if len(cov.Uncovered) != 0 {
		t.Errorf("uncovered=%+v", cov.Uncovered)
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"docs/**", "docs/07-field-requests.md", true},
		{"docs/**", "docs/mocks/fr95.html", true},
		{"docs/**", "internal/docs.go", false},
		{"**/*_test.go", "internal/daemon/control_test.go", true},
		{"**/*_test.go", "internal/daemon/control.go", false},
		{"frontend/dist/**", "frontend/dist/assets/index.js", true},
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false}, // path.Match: * does not cross a separator
		{"internal/**/settings.go", "internal/webui/settings.go", true},
	}
	for _, c := range cases {
		if got := globMatch(c.pattern, c.path); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestPathsAreComparableWhicheverWayTheyArrive(t *testing.T) {
	cov := Cover(twoFileDiff, []Citation{
		{Path: "./internal/daemon/daemon.go", From: 10, To: 14},
	}, nil)
	if cov.Covered != 1 {
		t.Error("a ./-prefixed citation missed the hunk it is standing on")
	}
}

// FuzzCover guards the arithmetic that now runs on every walkthrough create AND
// every read: a panic here is a review that cannot be opened, and the input is a
// diff somebody's agent wrote. The invariants are the ones a caller reads off the
// report without checking - that the parts add up to the whole, and that no hunk
// comes back with an inverted span.
func FuzzCover(f *testing.F) {
	f.Add(twoFileDiff)
	f.Add("")
	f.Add("@@ -1 +1 @@\n-a\n+b\n")
	f.Add("@@ -10,4 +9,0 @@\n-cut\n")
	f.Add("diff --git a/x b/x\ndeleted file mode 100644\n--- a/x\n+++ /dev/null\n@@ -1 +0,0 @@\n-gone\n")
	f.Add("@@ -1,99999999999999999999 +1,1 @@\n+overflowing\n")

	cites := []Citation{{Path: "internal/daemon/daemon.go", From: 1, To: 20}, {Path: "", From: 0, To: 0}}
	scopes := []Scope{{Paths: "internal/webui/**", Reason: "surface"}, {Path: "a.go", Lines: [2]int{2, 1}, Reason: "inverted on purpose"}}

	f.Fuzz(func(t *testing.T, diff string) {
		c := Cover(diff, cites, scopes)
		if c.Covered+c.OutOfScope+len(c.Uncovered) != c.Hunks {
			t.Fatalf("the parts do not add up: %+v", c)
		}
		if c.Uncovered == nil {
			t.Fatal("uncovered came back nil, so silence would read as covered")
		}
		for _, h := range c.Uncovered {
			if h.From > h.To || h.From < 0 {
				t.Fatalf("inverted or negative span: %+v", h)
			}
		}
	})
}
