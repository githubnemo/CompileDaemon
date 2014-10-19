package main

import (
	"errors"
	"os"
)

func terminateGracefully(process *os.Process) error {
	return errors.New("terminateGracefully not implemented on windows")
}

func gracefulTerminationPossible() bool {
	return false
}
