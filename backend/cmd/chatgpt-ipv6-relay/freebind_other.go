//go:build !linux

package main

import "syscall"

func freeBindControl() func(network, address string, conn syscall.RawConn) error {
	return nil
}
