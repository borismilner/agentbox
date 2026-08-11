//go:build linux

package server

import (
	"net"

	"golang.org/x/sys/unix"
)

// peerUID is NFR8: the socket is 0700 in a 0700 directory, and every accepted
// connection is checked against the owner's uid anyway. Belt and braces on
// purpose - a filesystem permission is a thing a person can change by accident,
// and what is on this socket is every question an agent has asked and every
// answer given.
//
// SO_PEERCRED is Linux's answer and it is the strongest of the three: the kernel
// fills in the credentials at connect time, so there is nothing for the peer to
// forge and nothing to race.
func peerUID(conn *net.UnixConn) (int, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return -1, err
	}
	uid := -1
	var credErr error
	ctlErr := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
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
