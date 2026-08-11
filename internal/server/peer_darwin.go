//go:build darwin

package server

import (
	"net"

	"golang.org/x/sys/unix"
)

// peerUID on macOS and the BSDs. Same guarantee as the Linux path, different call:
// LOCAL_PEERCRED answers an xucred instead of a ucred, and the kernel fills it in
// at connect time exactly the same way - so this is a rename, not a weakening.
//
// The one difference worth knowing is that xucred carries no pid, which nothing
// here wants: NFR8 is a question about WHO, and the pid would only be a second
// thing to keep in step.
func peerUID(conn *net.UnixConn) (int, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return -1, err
	}
	uid := -1
	var credErr error
	ctlErr := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil {
			credErr = err
			return
		}
		uid = int(cred.Uid)
	})
	if ctlErr != nil {
		return -1, ctlErr
	}
	return uid, credErr
}
