package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/howeyc/fsnotify"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// Milliseconds to wait for the next job to begin after a file change
const WorkDelay = 900

// Default pattern to match files which trigger a build
const FilePattern = `(.+\.go|.+\.c)$`

type globList []string

var excludedDirs globList
var excludedFiles globList
var includedFiles globList

func (g *globList) String() string {
	return fmt.Sprint(*g)
}
func (g *globList) Set(value string) error {
	*g = append(*g, value)
	return nil
}
func (g *globList) Matches(value string) bool {
	for _, v := range *g {
		if match, err := filepath.Match(v, value); err != nil {
			log.Fatalf("Bad pattern \"%s\": %s", v, err.Error())
		} else if match {
			return true
		}
	}
	return false
}

var (
	flag_directory = flag.String("directory", ".", "Directory to watch for changes")
	flag_pattern   = flag.String("pattern", FilePattern, "Pattern of watched files")
	flag_command   = flag.String("command", "", "Command to run and restart after build")
	flag_recursive = flag.Bool("recursive", true, "Watch all dirs. recursively")
	flag_build     = flag.String("build", "go build", "Command to rebuild after changes")
)

// Run `go build` and print the output if something's gone wrong.
func build() bool {
	log.Println("Running build command!")

	args := strings.Split(*flag_build, " ")
	if len(args) == 0 {
		// If the user has specified and empty then we are done.
		return true
	}

	cmd := exec.Command(args[0], args[1:]...)

	cmd.Dir = *flag_directory

	output, err := cmd.CombinedOutput()

	if err == nil {
		log.Println("Build ok.")
	} else {
		log.Println("Error while building:\n", string(output))
	}

	return err == nil
}

func matchesPattern(pattern *regexp.Regexp, file string) bool {
	return pattern.MatchString(file)
}

// Accept build jobs and start building when there are no jobs rushing in.
// The inrush protection is WorkDelay milliseconds long, in this period
// every incoming job will reset the timer.
func builder(jobs <-chan string, buildDone chan<- struct{}) {
	createThreshold := func() <-chan time.Time {
		return time.After(time.Duration(WorkDelay * time.Millisecond))
	}

	threshold := createThreshold()

	for {
		select {
		case <-jobs:
			threshold = createThreshold()
		case <-threshold:
			if build() {
				buildDone <- struct{}{}
			}
		}
	}
}

func logger(pipeChan <-chan io.ReadCloser) {
	dumper := func(pipe io.ReadCloser, prefix string) {
		reader := bufio.NewReader(pipe)

	readloop:
		for {
			line, err := reader.ReadString('\n')

			if err != nil {
				break readloop
			}

			log.Print(prefix, " ", line)
		}
	}

	for {
		pipe := <-pipeChan
		go dumper(pipe, "stdout:")

		pipe = <-pipeChan
		go dumper(pipe, "stderr:")
	}
}

// Start the supplied command and return stdout and stderr pipes for logging.
func startCommand(command string) (cmd *exec.Cmd, stdout io.ReadCloser, stderr io.ReadCloser, err error) {
	args := strings.Split(command, " ")
	cmd = exec.Command(args[0], args[1:]...)

	if stdout, err = cmd.StdoutPipe(); err != nil {
		err = fmt.Errorf("can't get stdout pipe for command: %s", err)
		return
	}

	if stderr, err = cmd.StderrPipe(); err != nil {
		err = fmt.Errorf("can't get stderr pipe for command: %s", err)
		return
	}

	if err = cmd.Start(); err != nil {
		err = fmt.Errorf("can't start command: %s", err)
		return
	}

	return
}

// Run the command in the given string and restart it after
// a message was received on the buildDone channel.
func runner(command string, buildDone <-chan struct{}) {
	var currentProcess *os.Process
	pipeChan := make(chan io.ReadCloser)

	go logger(pipeChan)

	for {
		<-buildDone

		if currentProcess != nil {
			if err := currentProcess.Kill(); err != nil {
				log.Fatal("Could not kill child process. Aborting due to danger of infinite forks.")
			}

			_, werr := currentProcess.Wait()

			if werr != nil {
				log.Fatal("Could not wait for child process. Aborting due to danger of infinite forks.")
			}
		}

		log.Println("Restarting the given command.")
		cmd, stdoutPipe, stderrPipe, err := startCommand(command)

		if err != nil {
			log.Fatal("Could not start command:", err)
		}

		pipeChan <- stdoutPipe
		pipeChan <- stderrPipe

		currentProcess = cmd.Process
	}
}

func flusher(buildDone <-chan struct{}) {
	for {
		<-buildDone
	}
}

func main() {
	flag.Var(&excludedDirs, "exclude-dir", " Don't watch directories matching this name")
	flag.Var(&excludedFiles, "exclude", " Don't watch files matching this name")
	flag.Var(&includedFiles, "include", " Watch files matching this name")

	flag.Parse()

	if *flag_directory == "" {
		fmt.Fprintf(os.Stderr, "-directory=... is required.\n")
		os.Exit(1)
	}

	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		log.Fatal(err)
	}

	defer watcher.Close()

	if *flag_recursive == true {
		err = filepath.Walk(*flag_directory, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				if excludedDirs.Matches(info.Name()) {
					return filepath.SkipDir
				} else {
					return watcher.Watch(path)
				}
			}
			return err
		})

		if err != nil {
			log.Fatal("filepath.Walk():", err)
		}

	} else {
		if err := watcher.Watch(*flag_directory); err != nil {
			log.Fatal("watcher.Watch():", err)
		}
	}

	pattern := regexp.MustCompile(*flag_pattern)
	jobs := make(chan string)
	buildDone := make(chan struct{})

	go builder(jobs, buildDone)

	if *flag_command != "" {
		go runner(*flag_command, buildDone)
	} else {
		go flusher(buildDone)
	}

	for {
		select {
		case ev := <-watcher.Event:
			if ev.Name != "" {
				base := filepath.Base(ev.Name)

				if includedFiles.Matches(base) || matchesPattern(pattern, ev.Name) {
					if !excludedFiles.Matches(base) {
						jobs <- ev.Name
					}
				}
			}

		case err := <-watcher.Error:
			if v, ok := err.(*os.SyscallError); ok {
				if v.Err == syscall.EINTR {
					continue
				}
				log.Fatal("watcher.Error: SyscallError:", v)
			}
			log.Fatal("watcher.Error:", err)
		}
	}
}
