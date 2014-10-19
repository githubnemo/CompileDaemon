// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

package main

import (
	"os"
	"syscall"
)

func terminateGracefully(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

func gracefulTerminationPossible() bool {
	return true
}
