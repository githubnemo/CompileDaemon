// +build windows

package main

import (
	"log"
)

func terminateGracefully(process *os.Process) {
	log.Fatal("Attempting to terminate gracefully on Windows is not supported.")
}

func gracefulTerminationPossible() bool {
	return false
}
