//go:build darwin || dragonfly || freebsd || linux || nacl || netbsd || openbsd || solaris
// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

package main

import (
	"os"
	"os/exec"
	"syscall"
)

var fatalSignals = []os.Signal{
	syscall.SIGINT,
	syscall.SIGTERM,
	syscall.SIGQUIT,
}

func setProcessGroupId(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
