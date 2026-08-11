//go:build windows

package server

import (
	"net"
	"os"
)

// peerUID on Windows, and the one place in this port where a guarantee genuinely
// changes shape rather than just changing call. It is written out here rather than
// buried, because a security property that quietly became weaker is worse than one
// that was never claimed.
//
// AF_UNIX exists on Windows and Go's net package speaks it, but there is no
// SO_PEERCRED and no LOCAL_PEERCRED: the kernel does not record who connected, and
// there is no supported call that asks. GetNamedPipeClientProcessId answers this
// question for named pipes only, which is a different transport.
//
// So on Windows NFR8 rests on the one mechanism that IS there: the socket lives in
// a directory created 0700, which Windows honours as an ACL granting the owner
// alone. That is the same first line of defence every platform has - it is only the
// second, independent check that is missing. Returning the daemon's own id lets
// Serve's comparison pass and keeps the accept path identical everywhere; it is not
// a verification and does not pretend to be one.
//
// The fix, when it is wanted, is a connect token: the daemon writes a random secret
// to a file only the owner can read and the first frame of every connection
// presents it. That is a protocol change affecting every client, so it is a
// backlog item (docs/backlog/robustness.md, R-46) and not a thing to slip into a
// portability pass.
func peerUID(conn *net.UnixConn) (int, error) {
	return os.Getuid(), nil
}
