/*
CompileDaemon is a very simple compile daemon for Go.

CompileDaemon watches your .go files in a directory and invokes `go build`
if a file changes.

Examples

In its simplest form, the defaults will do. With the current working directory set
to the source directory you can simply…

    $ CompileDaemon

… and it will recompile your code whenever you save a source file.

If you want it to also run your program each time it builds you might add…

    $ CompileDaemon -command="./MyProgram -my-options"

… and it will also keep a copy of your program running. Killing the old one and
starting a new one each time you build. For advanced usage you can also supply
the changed file to the command by doing…

	$ CompileDaemon -command="./MyProgram -my-options %[1]s"

…but note that this will not be set on the first start.

You may find that you need to exclude some directories and files from
monitoring, such as a .git repository or emacs temporary files…

    $ CompileDaemon -exclude-dir=.git -exclude=".#*"

If you want to monitor files other than .go and .c files you might…

    $ CompileDaemon -include=Makefile -include="*.less" -include="*.tmpl"

Options

There are command line options.

	FILE SELECTION
	-directory=XXX    – Which directory to monitor for changes
	-recursive=XXX    – Look into subdirectories
	-exclude-dir=XXX  – Exclude directories matching glob pattern XXX
	-exlude=XXX       – Exclude files whose basename matches glob pattern XXX
	-include=XXX      – Include files whose basename matches glob pattern XXX
	-pattern=XXX      – Include files whose path matches regexp XXX

	MISC
	-color            - Enable colorized output
	-log-prefix       - Enable/disable stdout/stderr labelling for the child process
	-graceful-kill    - On supported platforms, send the child process a SIGTERM to
	                    allow it to exit gracefully if possible.
	-graceful-timeout - Duration (in seconds) to wait for graceful kill to complete
	-verbose          - Print information about watched directories.

	ACTIONS
	-build=CCC        – Execute CCC to rebuild when a file changes
	-command=CCC      – Run command CCC after a successful build, stops previous command first

*/
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
)

// Milliseconds to wait for the next job to begin after a file change
const WorkDelay = 900

// Default pattern to match files which trigger a build
const FilePattern = `(.+\.go|.+\.c)$`

type globList []string

func (g *globList) String() string {
	return fmt.Sprint(*g)
}
func (g *globList) Set(value string) error {
	*g = append(*g, filepath.Clean(value))
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
	flagPattern         = flag.String("pattern", FilePattern, "Pattern of watched files")
	flagCommand         = flag.String("command", "", "Command to run and restart after build")
	flagCommandStop     = flag.Bool("command-stop", false, "Stop command before building")
	flagRecursive       = flag.Bool("recursive", true, "Watch all dirs. recursively")
	flagBuild           = flag.String("build", "go build", "Command to rebuild after changes")
	flagBuildDir        = flag.String("build-dir", "", "Directory to run build command in.  Defaults to directory")
	flagRunDir          = flag.String("run-dir", "", "Directory to run command in.  Defaults to directory")
	flagColor           = flag.Bool("color", false, "Colorize output for CompileDaemon status messages")
	flagLogPrefix       = flag.Bool("log-prefix", true, "Print log timestamps and subprocess stderr/stdout output")
	flagGracefulKill    = flag.Bool("graceful-kill", false, "Gracefully attempt to kill the child process by sending a SIGTERM first")
	flagGracefulTimeout = flag.Uint("graceful-timeout", 3, "Duration (in seconds) to wait for graceful kill to complete")
	flagVerbose         = flag.Bool("verbose", false, "Be verbose about which directories are watched.")

	// initialized in main() due to custom type.
	flagDirectories   globList
	flagExcludedDirs  globList
	flagExcludedFiles globList
	flagIncludedFiles globList
)

func okColor(format string, args ...interface{}) string {
	if *flagColor {
		return color.GreenString(format, args...)
	} else {
		return fmt.Sprintf(format, args...)
	}
}

func failColor(format string, args ...interface{}) string {
	if *flagColor {
		return color.RedString(format, args...)
	} else {
		return fmt.Sprintf(format, args...)
	}
}

// Run `go build` and print the output if something's gone wrong.
func build() bool {
	log.Println(okColor("Running build command!"))

	args := strings.Split(*flagBuild, " ")
	if len(args) == 0 {
		// If the user has specified and empty then we are done.
		return true
	}

	cmd := exec.Command(args[0], args[1:]...)

	if *flagBuildDir != "" {
		cmd.Dir = *flagBuildDir
	} else if len(flagDirectories) > 0 {
		cmd.Dir = flagDirectories[0]
	}

	output, err := cmd.CombinedOutput()

	if err == nil {
		log.Println(okColor("Build ok."))
	} else {
		log.Println(failColor("Error while building:\n"), failColor(string(output)))
	}

	return err == nil
}

func matchesPattern(pattern *regexp.Regexp, file string) bool {
	return pattern.MatchString(file)
}

// Accept build jobs and start building when there are no jobs rushing in.
// The inrush protection is WorkDelay milliseconds long, in this period
// every incoming job will reset the timer.
func builder(jobs <-chan string, buildStarted chan<- string, buildDone chan<- bool) {
	createThreshold := func() <-chan time.Time {
		return time.After(time.Duration(WorkDelay * time.Millisecond))
	}

	threshold := createThreshold()
	eventPath := ""

	for {
		select {
		case eventPath = <-jobs:
			threshold = createThreshold()
		case <-threshold:
			buildStarted <- eventPath
			buildDone <- build()
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

			if *flagLogPrefix {
				log.Print(prefix, " ", line)
			} else {
				log.Print(line)
			}
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

	if *flagRunDir != "" {
		cmd.Dir = *flagRunDir
	}

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
func runner(commandTemplate string, buildStarted <-chan string, buildSuccess <-chan bool) {
	var currentProcess *os.Process
	pipeChan := make(chan io.ReadCloser)

	go logger(pipeChan)

	// Launch concurrent process watching for signals from outside that
	// indicate termination to kill the running process alongside
	// CompileDaemon to prevent orphan processes.
	go func() {
		processSignalChannel := make(chan os.Signal, 1)
		signal.Notify(processSignalChannel, fatalSignals...)
		<-processSignalChannel

		log.Println(okColor("Received signal, terminating cleanly."))
		if currentProcess != nil {
			killProcess(currentProcess)
		}
		os.Exit(0)
	}()

	for {
		eventPath := <-buildStarted

		// prepend %0.s (which adds nothing) to prevent warning about missing
		// format specifier if the user did not supply one.
		command := fmt.Sprintf("%0.s"+commandTemplate, eventPath)

		if !*flagCommandStop {
			if !<-buildSuccess {
				continue
			}
		}

		if currentProcess != nil {
			killProcess(currentProcess)
		}

		if *flagCommandStop {
			log.Println(okColor("Command stopped. Waiting for build to complete."))
			if !<-buildSuccess {
				continue
			}
		}

		log.Println(okColor("Restarting the given command."))
		cmd, stdoutPipe, stderrPipe, err := startCommand(command)

		if err != nil {
			log.Fatal(failColor("Could not start command: %s", err))
		}

		pipeChan <- stdoutPipe
		pipeChan <- stderrPipe

		currentProcess = cmd.Process
	}
}

func killProcess(process *os.Process) {
	if *flagGracefulKill {
		killProcessGracefully(process)
	} else {
		killProcessHard(process)
	}
}

func killProcessHard(process *os.Process) {
	log.Println(okColor("Hard stopping the current process.."))

	if err := process.Kill(); err != nil {
		log.Println(failColor("Warning: could not kill child process.  It may have already exited."))
	}

	if _, err := process.Wait(); err != nil {
		log.Fatal(failColor("Could not wait for child process. Aborting due to danger of infinite forks."))
	}
}

func killProcessGracefully(process *os.Process) {
	done := make(chan error, 1)
	go func() {
		log.Println(okColor("Gracefully stopping the current process.."))
		if err := terminateGracefully(process); err != nil {
			done <- err
			return
		}
		_, err := process.Wait()
		done <- err
	}()

	select {
	case <-time.After(time.Duration(*flagGracefulTimeout) * time.Second):
		log.Println(failColor("Could not gracefully stop the current process, proceeding to hard stop."))
		killProcessHard(process)
		<-done
	case err := <-done:
		if err != nil {
			log.Fatal(failColor("Could not kill child process. Aborting due to danger of infinite forks."))
		}
	}
}

func flusher(buildStarted <-chan string, buildSuccess <-chan bool) {
	for {
		<-buildStarted
		<-buildSuccess
	}
}

func main() {
	flag.Var(&flagDirectories, "directory", "Directory to watch for changes, can be set more than once")
	flag.Var(&flagExcludedDirs, "exclude-dir", " Don't watch directories matching this name, can be set more than once")
	flag.Var(&flagExcludedFiles, "exclude", " Don't watch files matching this name, can be set more than once")
	flag.Var(&flagIncludedFiles, "include", " Watch files matching this name, can be set more than once")

	flag.Parse()

	if !*flagLogPrefix {
		log.SetFlags(0)
	}

	if len(flagDirectories) == 0 {
		flagDirectories = globList([]string{"."})
	}

	if *flagGracefulKill && !gracefulTerminationPossible() {
		log.Fatal("Graceful termination is not supported on your platform.")
	}

	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		log.Fatal(err)
	}

	defer watcher.Close()

	for _, flagDirectory := range flagDirectories {
		if *flagRecursive == true {
			err = filepath.Walk(flagDirectory, func(path string, info os.FileInfo, err error) error {
				if err == nil && info.IsDir() {
					if flagExcludedDirs.Matches(path) {
						return filepath.SkipDir
					} else {
						if *flagVerbose {
							log.Printf("Watching directory '%s' for changes.\n", path)
						}
						return watcher.Add(path)
					}
				}
				return err
			})

			if err != nil {
				log.Fatal("filepath.Walk():", err)
			}

			if err := watcher.Add(flagDirectory); err != nil {
				log.Fatal("watcher.Add():", err)
			}
		} else {
			if err := watcher.Add(flagDirectory); err != nil {
				log.Fatal("watcher.Add():", err)
			}
		}
	}

	pattern := regexp.MustCompile(*flagPattern)
	jobs := make(chan string)
	buildSuccess := make(chan bool)
	buildStarted := make(chan string)

	go builder(jobs, buildStarted, buildSuccess)

	if *flagCommand != "" {
		go runner(*flagCommand, buildStarted, buildSuccess)
	} else {
		go flusher(buildStarted, buildSuccess)
	}

	for {
		select {
		case ev := <-watcher.Events:
			if ev.Op&fsnotify.Remove == fsnotify.Remove || ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Create == fsnotify.Create {
				base := filepath.Base(ev.Name)

				// Assume it is a directory and track it.
				if *flagRecursive == true && !flagExcludedDirs.Matches(ev.Name) {
					watcher.Add(ev.Name)
				}

				if flagIncludedFiles.Matches(base) || matchesPattern(pattern, ev.Name) {
					if !flagExcludedFiles.Matches(base) {
						jobs <- ev.Name
					}
				}
			}

		case err := <-watcher.Errors:
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
