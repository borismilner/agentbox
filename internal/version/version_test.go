package version

import (
	"strings"
	"testing"
)

// Test binaries carry no VCS stamps, so this exercises the fallback path;
// the real values are verified in the CLI smoke test against a git build.
func TestGetNeverPanicsAndStringIsStable(t *testing.T) {
	info := Get()
	if info.GoVersion == "" {
		t.Fatal("go version missing")
	}
	s := info.String()
	if !strings.HasPrefix(s, "agentbox ") || !strings.Contains(s, info.GoVersion) {
		t.Fatalf("unexpected format: %q", s)
	}
}

// An unstamped build is not a release, so nothing may offer it an upgrade. This
// is the default for every build from source, including this test binary.
func TestUnstampedBuildIsNotARelease(t *testing.T) {
	if Get().Released() {
		t.Error("a build with no tag reports itself as a release")
	}
	// Named fields, not Get(), so a dirty checkout running the suite cannot make
	// this pass or fail for the wrong reason.
	plain := Info{Revision: "abc123", BuildTime: "t", GoVersion: "go1"}
	if got := plain.String(); got != "agentbox abc123 built t with go1" {
		t.Errorf("unstamped format changed: %q", got)
	}
}

// The stamp is what a release carries, and a dirty tree revokes it: the code no
// longer matches the tag, so it must not be treated as that version.
func TestStampedBuildReportsItsTag(t *testing.T) {
	t.Cleanup(func() { Tag = "" })
	Tag = "v9.9.9"

	info := Get()
	if info.Tag != "v9.9.9" {
		t.Fatalf("tag = %q, want v9.9.9", info.Tag)
	}
	if !strings.Contains(info.String(), "v9.9.9") {
		t.Errorf("String() hides the tag: %q", info.String())
	}
	if !info.Released() {
		t.Error("a stamped clean build is not reported as a release")
	}

	dirty := info
	dirty.Dirty = true
	if dirty.Released() {
		t.Error("a stamped DIRTY build is reported as a release")
	}
}
