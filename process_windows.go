package main

import (
	"errors"
	"os"
)

var fatalSignals = []os.Signal{
	os.Interrupt,
	os.Kill,
}

func terminateGracefully(process *os.Process) error {
	return errors.New("terminateGracefully not implemented on windows")
}

func gracefulTerminationPossible() bool {
	return false
}
