package logging

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEventsAreParseableJSONLines(t *testing.T) {
	dir := t.TempDir()
	log, closer, err := Open(dir, slog.LevelInfo, 10)
	if err != nil {
		t.Fatal(err)
	}
	log.Info(EvItemCreated, "item_id", "k1", "agent", "claude-code", "component", "daemon")
	log.Error(EvSoundFailed, "err", "exec: pw-play: not found")
	closer.Close()

	f, err := os.Open(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("line is not JSON: %q: %v", sc.Text(), err)
		}
		lines = append(lines, m)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0]["msg"] != EvItemCreated || lines[0]["item_id"] != "k1" {
		t.Fatalf("first event mangled: %v", lines[0])
	}
	if lines[1]["level"] != "ERROR" || !strings.Contains(lines[1]["err"].(string), "pw-play") {
		t.Fatalf("error event mangled: %v", lines[1])
	}
}

func TestDebugFilteredAtInfoLevel(t *testing.T) {
	dir := t.TempDir()
	log, closer, err := Open(dir, slog.LevelInfo, 10)
	if err != nil {
		t.Fatal(err)
	}
	log.Debug(EvIPCCall, "method", "agentbox.v1.notify")
	closer.Close()

	data, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("debug line written at info level: %q", data)
	}
}

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	log, closer, err := Open(dir, slog.LevelInfo, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	// ~1.5 MB of events against a 1 MB cap forces exactly one rotation.
	padding := strings.Repeat("x", 1024)
	for range 1536 {
		log.Info(EvIPCCall, "pad", padding)
	}

	if _, err := os.Stat(filepath.Join(dir, "log", rotatedName)); err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}
	st, err := os.Stat(Path(dir))
	if err != nil {
		t.Fatalf("active log missing after rotation: %v", err)
	}
	if st.Size() == 0 {
		t.Fatal("active log empty after rotation")
	}
	if st.Size() > 1024*1024 {
		t.Fatalf("active log exceeds cap: %d bytes", st.Size())
	}
}

func TestAppendAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	for range 2 {
		log, closer, err := Open(dir, slog.LevelInfo, 10)
		if err != nil {
			t.Fatal(err)
		}
		log.Info(EvDaemonStart)
		closer.Close()
	}
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), EvDaemonStart); n != 2 {
		t.Fatalf("got %d start events, want 2 (append lost on reopen)", n)
	}
}
