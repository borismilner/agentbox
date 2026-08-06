package change

import (
	"reflect"
	"testing"
)

// One git diff exercising the shapes that matter: a changed line (del+add),
// a pure addition, a pure deletion, a new file, a rename, and a body line
// that starts with "---" which must not be read as a file header.
const gitDiff = `diff --git a/pkg/a.go b/pkg/a.go
index 111..222 100644
--- a/pkg/a.go
+++ b/pkg/a.go
@@ -400,7 +408,7 @@ func head() {
 	ctx1
 	ctx2
-	Title: "agentbox",
+	Title: title,
 	ctx3
 	ctx4
 	ctx5
 	ctx5b
@@ -420,4 +428,6 @@ func tail() {
 	ctx6
+	added1
+	added2
 	ctx7
 	ctx8
diff --git a/pkg/b.py b/pkg/b.py
--- a/pkg/b.py
+++ b/pkg/b.py
@@ -10,4 +10,2 @@
 	keep
-	gone1
--- not a header
 	keep2
diff --git a/pkg/new.go b/pkg/new.go
new file mode 100644
--- /dev/null
+++ b/pkg/new.go
@@ -0,0 +1,2 @@
+first
+second
diff --git a/old.go b/renamed.go
similarity index 90%
rename from old.go
rename to renamed.go
`

func TestParseGitDiff(t *testing.T) {
	s := Parse(gitDiff)
	if len(s.Files) != 4 {
		t.Fatalf("files = %d, want 4", len(s.Files))
	}
	a := s.File("pkg/a.go")
	if a == nil || len(a.Hunks) != 2 {
		t.Fatalf("pkg/a.go hunks = %+v", a)
	}
	if got := a.AddedIn(408, 414); !reflect.DeepEqual(got, []int{410}) {
		t.Errorf("AddedIn first hunk = %v, want [410]", got)
	}
	if got := a.AddedIn(428, 433); !reflect.DeepEqual(got, []int{429, 430}) {
		t.Errorf("AddedIn second hunk = %v, want [429 430]", got)
	}
	dels := a.DeletionsIn(408, 414)
	want := []Deletion{{After: 409, Old: 402, Lines: []string{"\tTitle: \"agentbox\","}}}
	if !reflect.DeepEqual(dels, want) {
		t.Errorf("DeletionsIn = %+v, want %+v", dels, want)
	}

	b := s.File("pkg/b.py")
	if b == nil || len(b.Hunks) != 1 {
		t.Fatalf("pkg/b.py = %+v", b)
	}
	// "--- not a header" is hunk BODY (a deletion continuing the same run):
	// the hunk's line counts must have consumed it, and the two consecutive
	// removed lines form one Deletion.
	bdels := b.DeletionsIn(10, 11)
	want2 := []Deletion{{After: 10, Old: 11, Lines: []string{"\tgone1", "-- not a header"}}}
	if !reflect.DeepEqual(bdels, want2) {
		t.Errorf("body line starting with --- misparsed: %+v", bdels)
	}

	n := s.File("pkg/new.go")
	if n == nil || n.Badge != "new" {
		t.Fatalf("new.go = %+v", n)
	}
	if got := n.AddedIn(1, 2); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("new file AddedIn = %v", got)
	}

	r := s.File("renamed.go")
	if r == nil || r.Badge != "renamed" || r.OldPath != "old.go" {
		t.Errorf("rename = %+v", r)
	}
}

func TestParsePlainDiff(t *testing.T) {
	plain := `--- a.txt	2026-07-29 10:00:00
+++ a.txt	2026-07-29 10:01:00
@@ -1 +1,2 @@
 one
+two
`
	s := Parse(plain)
	if len(s.Files) != 1 || s.Files[0].Path != "a.txt" {
		t.Fatalf("plain diff = %+v", s.Files)
	}
	if got := s.File("a.txt").AddedIn(1, 2); !reflect.DeepEqual(got, []int{2}) {
		t.Errorf("AddedIn = %v, want [2]", got)
	}
}

func TestParseHunkWithoutCounts(t *testing.T) {
	s := Parse("--- f\n+++ f\n@@ -3 +3 @@\n-old\n+new\n")
	f := s.File("f")
	if f == nil || len(f.Hunks) != 1 {
		t.Fatalf("f = %+v", f)
	}
	if got := f.AddedIn(3, 3); !reflect.DeepEqual(got, []int{3}) {
		t.Errorf("AddedIn = %v, want [3]", got)
	}
	dels := f.DeletionsIn(3, 3)
	if len(dels) != 1 || dels[0].After != 2 || dels[0].Old != 3 {
		t.Errorf("DeletionsIn = %+v", dels)
	}
}

func TestDeletionAtRangeTopIsKept(t *testing.T) {
	// A deletion sitting just above a cited range's first line belongs to
	// the range's story: After == from-1 must be included.
	s := Parse("--- f\n+++ f\n@@ -5,3 +5,2 @@\n ctx\n-gone\n ctx2\n")
	dels := s.File("f").DeletionsIn(6, 6)
	if len(dels) != 1 || dels[0].After != 5 {
		t.Errorf("seam deletion dropped: %+v", dels)
	}
}

func TestParseGarbageIsQuiet(t *testing.T) {
	for _, raw := range []string{"", "not a diff at all\njust prose\n", "@@ mangled @@\n+x\n"} {
		s := Parse(raw)
		for _, f := range s.Files {
			if f.Path == "" && len(f.Hunks) == 0 {
				t.Errorf("kept an empty artifact for %q", raw)
			}
		}
	}
}

func TestNilFileHelpers(t *testing.T) {
	var f *File
	if f.AddedIn(1, 2) != nil || f.DeletionsIn(1, 2) != nil {
		t.Error("nil *File helpers must return nil")
	}
}

// FuzzParse is the guard on a parser that reads agent-authored text. Every
// walkthrough create runs a diff through it, and now every walkthrough READ does
// too (the coverage arithmetic), so a panic here is a review that cannot be
// opened rather than one that fails to be written. The corpus is the shapes that
// have historically confused unified-diff parsers: counts that lie, headers with
// no body, bodies that look like headers.
func FuzzParse(f *testing.F) {
	f.Add("")
	f.Add("@@ -1 +1 @@\n-a\n+b\n")
	f.Add("@@ -1,99 +1,99 @@\n+one\n") // counts far past the body
	f.Add("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -0,0 +1,2 @@\n+--- not a header\n+++ also not\n")
	f.Add("--- /dev/null\n+++ b/new\n@@ -0,0 +1 @@\n+hello\n")
	f.Add("diff --git a/a b/b\nrename to b\n")
	f.Add("@@ -1,2 +0,0 @@\n-gone\n-gone\n")
	f.Add("\\ No newline at end of file\n")
	f.Add("@@@ -1,1 -1,1 +1,1 @@@\n++combined\n") // a combined (merge) diff header

	f.Fuzz(func(t *testing.T, raw string) {
		set := Parse(raw)
		for _, file := range set.Files {
			for _, h := range file.Hunks {
				// The invariant the rest of the tree relies on: a hunk's declared
				// counts are never negative, so a span built from them cannot
				// invert and a slice built from them cannot panic.
				if h.NewN < 0 || h.OldN < 0 || h.NewFrom < 0 || h.OldFrom < 0 {
					t.Fatalf("negative geometry in %q: %+v", file.Path, h)
				}
			}
		}
	})
}
