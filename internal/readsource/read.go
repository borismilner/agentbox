// Package readsource reads a file whose path a caller chose, for a process that
// has to survive the answer (R-16).
//
// It exists as its own package because three surfaces need the identical rule and
// two of them are in packages that cannot import each other: the reading window
// and the review board (internal/webui), and the walkthrough capture
// (internal/daemon, which webui already imports). The rule is small enough that
// copying it would have looked cheaper and left the third door open.
package readsource

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

// MaxBytes is what a surface will read off disk on a caller's word.
//
// Sized off the documents this product is actually asked to show. The largest in
// its own tree is its session history at 254 kB, so 4 MB is far above any honest
// document while still refusing what is not a document at all. It is deliberately
// not the 2 MB image ceiling: prose costs nothing to store, and an agent showing a
// generated report should not have to think about the limit.
//
// The ceiling is about the read ENDING. What a large document then costs to
// RENDER is a different defect (R-17) with its own ceiling to add, in the markdown
// path where the amplification happens.
const MaxBytes = 4 << 20

// Read returns the contents of path, refusing anything that is not a bounded
// regular file, along with the stat of the file it actually read.
//
// The whole of R-16 is that it stats BEFORE it reads. `os.ReadFile("/dev/zero")`
// grows a buffer until the OOM killer takes the daemon, and with it every parked
// agent, the in-memory undo grace and the flood state; on a fifo the same call
// blocks its goroutine for ever instead. One ordinary tool call, available to any
// agent, no human involved. The identical guard had sat in the image inliner since
// the beginning - "Reading /dev/zero would not end" - and never reached the
// document, the board or the capture.
//
// The error is the sentence a reader will see in place of the document, so it
// names the path and the rule that refused it.
func Read(path string) ([]byte, os.FileInfo, error) {
	return ReadCapped(path, MaxBytes)
}

// ReadCapped is Read with its ceiling given, so a test can refuse eight bytes
// instead of writing four megabytes.
func ReadCapped(path string, limit int64) ([]byte, os.FileInfo, error) {
	// Stat, never open, first: opening a fifo is itself what blocks, so a check
	// made on the handle would arrive too late to be the guard.
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if err := allowed(path, info, limit); err != nil {
		return nil, nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	// The handle gets the same two checks, and the read is bounded even though the
	// size is now known twice. One boring reason and one that bites: a file can
	// grow between the stat and the read, and a regular file is not the same thing
	// as a file with a size - everything under /proc reports zero bytes and then
	// hands over as much as it likes.
	if info, err = f.Stat(); err != nil {
		return nil, nil, err
	}
	if err := allowed(path, info, limit); err != nil {
		return nil, nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > limit {
		return nil, nil, pastCeiling(path, int64(len(data)), limit)
	}
	return data, info, nil
}

// allowed is the pair of rules, applied to a path and then to the handle opened
// from it.
func allowed(path string, info os.FileInfo, limit int64) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file: a directory, a device or a pipe cannot be read as a document", path)
	}
	if info.Size() > limit {
		return pastCeiling(path, info.Size(), limit)
	}
	return nil
}

func pastCeiling(path string, size, limit int64) error {
	return fmt.Errorf("%s is %s, past the %s one document may be", path, bytesText(size), bytesText(limit))
}

// bytesText writes a size the way the sentence needs it read, which is not in
// bytes once it is large enough to matter.
func bytesText(n int64) string {
	switch {
	case n >= 1<<20:
		return strconv.FormatInt((n+(1<<20)-1)>>20, 10) + " MB"
	case n >= 1<<10:
		return strconv.FormatInt((n+1023)>>10, 10) + " kB"
	default:
		return strconv.FormatInt(n, 10) + " bytes"
	}
}
