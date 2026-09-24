package main

import (
	"path/filepath"
	"testing"
)

func TestUIBinaryForSitsInLibBesideBin(t *testing.T) {
	exe := filepath.Join("/home", "u", ".local", "bin", "agentbox")
	want := filepath.Join("/home", "u", ".local", "lib", "agentbox", "agentbox")
	if got := uiBinaryFor(exe, ""); got != want {
		t.Fatalf("uiBinaryFor(%q) = %q, want %q", exe, got, want)
	}
}

func TestUIBinaryForSurvivesAReplacedClient(t *testing.T) {
	// A client started before a deploy replaced its file reports this path.
	exe := filepath.Join("/opt", "bin", "agentbox") + " (deleted)"
	want := filepath.Join("/opt", "lib", "agentbox", "agentbox")
	if got := uiBinaryFor(exe, ""); got != want {
		t.Fatalf("uiBinaryFor(%q) = %q, want %q", exe, got, want)
	}
}

func TestUIBinaryForHonoursTheOverride(t *testing.T) {
	if got := uiBinaryFor("/x/bin/agentbox", "/y/agentbox"); got != "/y/agentbox" {
		t.Fatalf("override ignored: %q", got)
	}
}
