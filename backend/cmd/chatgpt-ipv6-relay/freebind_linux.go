//go:build linux

package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func freeBindControl() func(network, address string, conn syscall.RawConn) error {
	return func(_, _ string, conn syscall.RawConn) error {
		var controlErr error
		if err := conn.Control(func(fd uintptr) {
			controlErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_FREEBIND, 1)
		}); err != nil {
			return err
		}
		return controlErr
	}
}
