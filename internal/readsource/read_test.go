package readsource

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadsARegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte("# Title\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	data, info, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(data) != "# Title\n" {
		t.Errorf("data = %q", data)
	}
	// Callers use this instead of a second stat of their own, so it has to be the
	// stat of the file that was actually read.
	if info == nil || info.Size() != 8 {
		t.Errorf("info = %+v, want the read file's own stat", info)
	}
}

// A symlink to an ordinary file is an ordinary file. This is not a hypothetical
// case in this repo: its own STATUS.md, history.md and backlog are symlinks into
// another tree, so a guard written with Lstat would refuse the documents most
// likely to be shown.
func TestReadsThroughASymlinkToARegularFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.md")
	if err := os.WriteFile(real, []byte("# real\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}

	data, _, err := Read(link)
	if err != nil {
		t.Fatalf("a symlinked document was refused: %v", err)
	}
	if string(data) != "# real\n" {
		t.Errorf("data = %q", data)
	}
}

// A directory, a device and a fifo are the three shapes of R-16, and what each
// case asserts is that the call RETURNS: os.ReadFile grows a buffer on /dev/zero
// until the OOM killer arrives, and blocks for ever on a fifo.
func TestRefusesWhatIsNotAFile(t *testing.T) {
	dir := t.TempDir()
	cases := []struct{ what, path string }{
		{"a directory", dir},
	}
	if _, err := os.Stat("/dev/zero"); err == nil {
		cases = append(cases, struct{ what, path string }{"a device", "/dev/zero"})
	}
	if p, ok := testFifo(t, dir); ok {
		cases = append(cases, struct{ what, path string }{"a fifo", p})
	}

	for _, c := range cases {
		data, _, err := Read(c.path)
		if err == nil {
			t.Errorf("%s (%s) was read as a document, %d bytes of it", c.what, c.path, len(data))
			continue
		}
		if !strings.Contains(err.Error(), c.path) || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("%s: error = %q, want the path and the rule in it", c.what, err)
		}
	}
}

func TestRefusesPastTheCeiling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.md")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 64)), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := ReadCapped(path, 8)
	if err == nil {
		t.Fatal("64 bytes passed an 8-byte ceiling")
	}
	if !strings.Contains(err.Error(), "64 bytes") || !strings.Contains(err.Error(), "8 bytes") {
		t.Errorf("error = %q, want the size and the ceiling in it", err)
	}
}

// A file exactly at the ceiling is under it. The off-by-one matters because the
// bounded read asks for one byte more than the limit to notice the overrun.
func TestAcceptsExactlyTheCeiling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exact.md")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 8)), 0o644); err != nil {
		t.Fatal(err)
	}

	data, _, err := ReadCapped(path, 8)
	if err != nil || len(data) != 8 {
		t.Errorf("ReadCapped(8 bytes, ceiling 8) = %d bytes, %v", len(data), err)
	}
}

// A regular file whose size is a lie. Everything under /proc reports zero bytes
// and then hands over as much as it likes, so the ceiling cannot be enforced from
// the stat alone: the read itself has to be bounded.
func TestBoundsAFileThatUnderstatesItsSize(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("no /proc to lie about its size")
	}
	const path = "/proc/self/status"
	info, err := os.Stat(path)
	if err != nil {
		t.Skip(err)
	}
	if info.Size() != 0 {
		t.Skipf("%s reports %d bytes, so it is not the case this test is about", path, info.Size())
	}

	if _, _, err := ReadCapped(path, 8); err == nil {
		t.Error("a zero-byte file handed over more than the ceiling and was accepted")
	}
	// The same file is fine under a ceiling it fits in, which is what keeps this a
	// bound rather than a ban.
	data, _, err := ReadCapped(path, MaxBytes)
	if err != nil || len(data) == 0 {
		t.Errorf("ReadCapped(%s, MaxBytes) = %d bytes, %v", path, len(data), err)
	}
}

func TestBytesTextReadsAsASentenceWould(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 bytes"},
		{64, "64 bytes"},
		{1 << 10, "1 kB"},
		{1500, "2 kB"},
		{1 << 20, "1 MB"},
		{MaxBytes, "4 MB"},
	}
	for _, c := range cases {
		if got := bytesText(c.n); got != c.want {
			t.Errorf("bytesText(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
