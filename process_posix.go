// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

package main

import (
	"log"
	"os"
	"syscall"
	"time"
)

func terminateGracefully(process *os.Process) {
	// If enabled, attempt to do a graceful shutdown of the child process.
	done := make(chan error, 1)
	go func() {
		log.Println(okColor("Gracefully stopping the current process.."))
		if err := process.Signal(syscall.SIGTERM); err != nil {
			done <- err
			return
		}
		_, err := process.Wait()
		done <- err
	}()

	select {
	case <-time.After(3 * time.Second):
		log.Println(failColor("Could not gracefully stop the current process, proceeding to hard stop."))
		if err := process.Kill(); err != nil {
			log.Fatal(failColor("Could not kill child process. Aborting due to danger of infinite forks."))
		}
		<-done
	case err := <-done:
		if err != nil {
			log.Fatal(failColor("Could not kill child process. Aborting due to danger of infinite forks."))
		}
	}
}

func gracefulTerminationPossible() bool {
	return true
}
