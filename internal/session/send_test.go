package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// R-25. Send used to write to the child's stdin with no deadline, from the
// goroutine that draws, and record the user's turn BEFORE the write. So a child
// that had stopped reading - or any prompt past the 64 kB pipe buffer - froze
// the window for the life of the session, with the transcript already showing a
// turn that had not been sent.

// writeScript writes an executable shell stub with the given body. It is
// writeStub's sibling for the stubs that have to do something other than print:
// what matters in these tests is whether the child reads its stdin at all.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude-stub")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// sendAsync runs Send off the test goroutine so a Send that never returns fails
// this test rather than hanging the suite - which is precisely what the defect
// did to the goroutine that draws.
func sendAsync(t *testing.T, d *Driver, prompt string, within time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- d.Send(prompt) }()
	select {
	case err := <-done:
		return err
	case <-time.After(within):
		t.Fatalf("Send did not return within %s: it is blocked on the child's stdin, which is the UI goroutine frozen", within)
		return nil
	}
}

// bigPrompt is comfortably past the 64 kB pipe buffer, so a child that never
// reads leaves the write blocked in the kernel rather than absorbed.
func bigPrompt() string { return strings.Repeat("x", 256<<10) }

func TestSendGivesUpOnAChildThatNeverReadsItsInput(t *testing.T) {
	d := New(Config{Bin: writeScript(t, "sleep 30"), SendTimeout: 300 * time.Millisecond}, nil, nil)
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(d.Kill)

	start := time.Now()
	err := sendAsync(t, d, bigPrompt(), 3*time.Second)
	if err == nil {
		t.Fatal("Send reported success for a prompt the child never took")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Send held the caller for %s, want about the 300ms deadline", elapsed)
	}
}

func TestAPromptThatDidNotLandIsNotInTheTranscript(t *testing.T) {
	d := New(Config{Bin: writeScript(t, "sleep 30"), SendTimeout: 300 * time.Millisecond}, nil, nil)
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(d.Kill)

	if err := sendAsync(t, d, bigPrompt(), 3*time.Second); err == nil {
		t.Fatal("Send reported success for a prompt the child never took")
	}
	for _, turn := range d.Turns() {
		if turn.Role == RoleUser {
			t.Fatal("the transcript shows the prompt as sent; the child never received a byte of it")
		}
	}
	// And it says so, rather than leaving a composer that emptied itself and no
	// explanation anywhere.
	var said bool
	for _, turn := range d.Turns() {
		if turn.Role == RoleSystem && strings.Contains(turn.Err, "not reading its input") {
			said = true
		}
	}
	if !said {
		t.Fatalf("nothing in the conversation says the prompt was not delivered: %+v", d.Turns())
	}
	// The state must not read as working either: nothing is working on it.
	if d.State() == StateWorking {
		t.Fatal("the session reports it is working on a prompt it never received")
	}
}

func TestSendKeepsWorkingWhenTheChildReads(t *testing.T) {
	// The ordinary path, with a prompt far past the pipe buffer to prove the
	// deadline is a bound on a wedged child and not a size limit.
	d := New(Config{Bin: writeScript(t, "cat > /dev/null"), SendTimeout: 5 * time.Second}, nil, nil)
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(d.Kill)

	if err := sendAsync(t, d, bigPrompt(), 6*time.Second); err != nil {
		t.Fatalf("Send to a child that reads: %v", err)
	}
	turns := d.Turns()
	if len(turns) != 1 || turns[0].Role != RoleUser {
		t.Fatalf("want one user turn recorded after a successful write, got %+v", turns)
	}
}

func TestSendReportsAClosedPipeInTheConversation(t *testing.T) {
	d := New(Config{Bin: writeScript(t, "sleep 30"), SendTimeout: 3 * time.Second}, nil, nil)
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(d.Kill)
	d.Stop() // closes the child's stdin: every later write fails at once

	if err := sendAsync(t, d, "hello", 2*time.Second); err == nil {
		t.Fatal("Send reported success writing to a closed pipe")
	}
	var said bool
	for _, turn := range d.Turns() {
		if turn.Role == RoleSystem && strings.Contains(turn.Err, "did not reach the session") {
			said = true
		}
	}
	if !said {
		t.Fatalf("a failed write left nothing in the conversation: %+v", d.Turns())
	}
}
