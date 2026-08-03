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
