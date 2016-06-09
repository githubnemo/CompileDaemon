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
starting a new one each time you build.

You may find that you need to exclude some directories and files from
monitoring, such as a .git repository or emacs temporary files…

    $ CompileDaemon -exclude-dir=.git -exclude=".#*"

If you want to monitor files other than .go and .c files you might…

    $ CompileDaemon -include=Makefile -include="*.less" -include="*.tmpl"

Options

There are command line options.

	FILE SELECTION
	-directory=XXX    – which directory to monitor for changes
	-recursive=XXX    – look into subdirectories
	-exclude-dir=XXX  – exclude directories matching glob pattern XXX
	-exlude=XXX       – exclude files whose basename matches glob pattern XXX
	-include=XXX      – include files whose basename matches glob pattern XXX
	-pattern=XXX      – include files whose path matches regexp XXX

	MISC
	-color            - enable colorized output
	-log-prefix       - Enable/disable stdout/stderr labelling for the child process
	-graceful-kill    - On supported platforms, send the child process a SIGTERM to
	                    allow it to exit gracefully if possible.
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
	"gopkg.in/fsnotify.v1"
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
	flag_directory    = flag.String("directory", ".", "Directory to watch for changes")
	flag_pattern      = flag.String("pattern", FilePattern, "Pattern of watched files")
	flag_command      = flag.String("command", "", "Command to run and restart after build")
	flag_command_stop = flag.Bool("command-stop", false, "Stop command before building")
	flag_recursive    = flag.Bool("recursive", true, "Watch all dirs. recursively")
	flag_build        = flag.String("build", "go build", "Command to rebuild after changes")
	flag_build_dir    = flag.String("build-dir", "", "Directory to run build command in.  Defaults to directory")
	flag_color        = flag.Bool("color", false, "Colorize output for CompileDaemon status messages")
	flag_logprefix    = flag.Bool("log-prefix", true, "Print log timestamps and subprocess stderr/stdout output")
	flag_gracefulkill = flag.Bool("graceful-kill", false, "Gracefully attempt to kill the child process by sending a SIGTERM first")

	// initialized in main() due to custom type.
	flag_excludedDirs  globList
	flag_excludedFiles globList
	flag_includedFiles globList
)

func okColor(format string, args ...interface{}) string {
	if *flag_color {
		return color.GreenString(format, args...)
	} else {
		return fmt.Sprintf(format, args...)
	}
}

func failColor(format string, args ...interface{}) string {
	if *flag_color {
		return color.RedString(format, args...)
	} else {
		return fmt.Sprintf(format, args...)
	}
}

// Run `go build` and print the output if something's gone wrong.
func build() bool {
	log.Println(okColor("Running build command!"))

	args := strings.Split(*flag_build, " ")
	if len(args) == 0 {
		// If the user has specified and empty then we are done.
		return true
	}

	cmd := exec.Command(args[0], args[1:]...)

	if *flag_build_dir != "" {
		cmd.Dir = *flag_build_dir
	} else {
		cmd.Dir = *flag_directory
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
func builder(jobs <-chan string, buildStarted chan<- struct{}, buildDone chan<- bool) {
	createThreshold := func() <-chan time.Time {
		return time.After(time.Duration(WorkDelay * time.Millisecond))
	}

	threshold := createThreshold()

	for {
		select {
		case <-jobs:
			threshold = createThreshold()
		case <-threshold:
			buildStarted <- struct{}{}
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

			if *flag_logprefix {
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
func runner(command string, buildStarted <-chan struct{}, buildSuccess <-chan bool) {
	var currentProcess *os.Process
	pipeChan := make(chan io.ReadCloser)

	go logger(pipeChan)

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, fatalSignals...)
		<-sigChan
		log.Println(okColor("Received signal, terminating cleanly."))
		if currentProcess != nil {
			killProcess(currentProcess)
		}
		os.Exit(0)
	}()

	for {
		<-buildStarted
		if !*flag_command_stop {
			if !<-buildSuccess {
				continue
			}
		}

		if currentProcess != nil {
			killProcess(currentProcess)
		}
		if *flag_command_stop {
			log.Println(okColor("Command stopped. Waiting for build to complete."))
			<-buildSuccess
		}

		log.Println(okColor("Restarting the given command."))
		cmd, stdoutPipe, stderrPipe, err := startCommand(command)

		if err != nil {
			log.Fatal(failColor("Could not start command:", err))
		}

		pipeChan <- stdoutPipe
		pipeChan <- stderrPipe

		currentProcess = cmd.Process
	}
}

func killProcess(process *os.Process) {
	if *flag_gracefulkill {
		killProcessGracefully(process)
	} else {
		killProcessHard(process)
	}
}

func killProcessHard(process *os.Process) {
	log.Println(okColor("Hard stopping the current process.."))

	if err := process.Kill(); err != nil {
		log.Fatal(failColor("Could not kill child process. Aborting due to danger of infinite forks."))
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
	case <-time.After(3 * time.Second):
		log.Println(failColor("Could not gracefully stop the current process, proceeding to hard stop."))
		killProcessHard(process)
		<-done
	case err := <-done:
		if err != nil {
			log.Fatal(failColor("Could not kill child process. Aborting due to danger of infinite forks."))
		}
	}
}

func flusher(buildStarted <-chan struct{}, buildSuccess <-chan bool) {
	for {
		<-buildStarted
		<-buildSuccess
	}
}

func main() {
	flag.Var(&flag_excludedDirs, "exclude-dir", " Don't watch directories matching this name")
	flag.Var(&flag_excludedFiles, "exclude", " Don't watch files matching this name")
	flag.Var(&flag_includedFiles, "include", " Watch files matching this name")

	flag.Parse()

	if !*flag_logprefix {
		log.SetFlags(0)
	}

	if *flag_directory == "" {
		fmt.Fprintf(os.Stderr, "-directory=... is required.\n")
		os.Exit(1)
	}

	if *flag_gracefulkill && !gracefulTerminationPossible() {
		log.Fatal("Graceful termination is not supported on your platform.")
	}

	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		log.Fatal(err)
	}

	defer watcher.Close()

	if *flag_recursive == true {
		err = filepath.Walk(*flag_directory, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				if flag_excludedDirs.Matches(info.Name()) {
					return filepath.SkipDir
				} else {
					return watcher.Add(path)
				}
			}
			return err
		})

		if err != nil {
			log.Fatal("filepath.Walk():", err)
		}

		if err := watcher.Add(*flag_directory); err != nil {
			log.Fatal("watcher.Add():", err)
		}

	} else {
		if err := watcher.Add(*flag_directory); err != nil {
			log.Fatal("watcher.Add():", err)
		}
	}

	pattern := regexp.MustCompile(*flag_pattern)
	jobs := make(chan string)
	buildSuccess := make(chan bool)
	buildStarted := make(chan struct{})

	go builder(jobs, buildStarted, buildSuccess)

	if *flag_command != "" {
		go runner(*flag_command, buildStarted, buildSuccess)
	} else {
		go flusher(buildStarted, buildSuccess)
	}

	for {
		select {
		case ev := <-watcher.Events:
			if ev.Op&fsnotify.Remove == fsnotify.Remove || ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Create == fsnotify.Create {
				base := filepath.Base(ev.Name)

				if flag_includedFiles.Matches(base) || matchesPattern(pattern, ev.Name) {
					if !flag_excludedFiles.Matches(base) {
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
