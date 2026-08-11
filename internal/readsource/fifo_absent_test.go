//go:build !unix

package readsource

import "testing"

// testFifo where there is no mkfifo (ADR-0013: a build tag with nothing on the
// other side is a hole, not a gate). The fifo case is skipped rather than faked -
// Windows named pipes are not filesystem paths os.Open would block on, so there
// is nothing here for the guard to be tested against.
func testFifo(t *testing.T, _ string) (string, bool) {
	t.Helper()
	return "", false
}
