package main

import (
	"errors"
	"os"
)

var fatalSignals = []os.Signal{
	os.Interrupt,
	os.Kill,
}

func setProcessGroupId(cmd *exec.Cmd) {
	// TODO implement this for windows as well
}

func terminateHard(process *os.Process) error {
	return process.Kill()
}

func terminateGracefully(process *os.Process) error {
	return errors.New("terminateGracefully not implemented on windows")
}

func gracefulTerminationPossible() bool {
	return false
}
