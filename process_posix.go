// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

package main

import (
	"os"
	"syscall"
)

var fatalSignals = []os.Signal{
	syscall.SIGINT,
	syscall.SIGTERM,
	syscall.SIGQUIT,
}

func terminateGracefully(process *os.Process) error {
	return syscall.Kill(-process.Pid, syscall.SIGTERM)
}

func terminateHard(process *os.Process) error {
	return syscall.Kill(-process.Pid, syscall.SIGKILL)
}

func gracefulTerminationPossible() bool {
	return true
}
