package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeBasePrefersTheSessionVariable(t *testing.T) {
	got := runtimeBase("/run/user/1000", 1000, func(string) bool { t.Fatal("must not stat when XDG_RUNTIME_DIR is set"); return false })
	if got != "/run/user/1000" {
		t.Fatalf("got %q", got)
	}
}

func TestRuntimeBaseFallsBackToLogindWithoutTheVariable(t *testing.T) {
	// A cron job: no XDG_RUNTIME_DIR, but the session's directory exists.
	got := runtimeBase("", 1000, func(p string) bool { return p == "/run/user/1000" })
	if got != "/run/user/1000" {
		t.Fatalf("got %q, want the logind directory the desktop session uses", got)
	}
}

func TestRuntimeBaseUsesTempDirOnlyWhenNothingElseExists(t *testing.T) {
	want := filepath.Join(os.TempDir(), "agentbox-1000")
	if got := runtimeBase("", 1000, func(string) bool { return false }); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// No uid at all (Windows reports -1): never probe /run/user/-1.
	if got := runtimeBase("", -1, func(string) bool { t.Fatal("must not probe a negative uid"); return false }); got != filepath.Join(os.TempDir(), "agentbox--1") {
		t.Fatalf("got %q", got)
	}
}
