package main

import (
	"log"
	"flag"
	"os/exec"
	"regexp"
	"time"
	"github.com/howeyc/fsnotify"
)

// Seconds to wait for the next job to begin
const WorkDelay = 5

// Pattern to match files which trigger a build
const FilePattern = `(.+\.go|.+\.c)$`

var flag_directory = flag.String("directory", "", "Directory to watch for changes")

var flag_pattern = flag.String("pattern", FilePattern, "Pattern of watched files")

// Run `go build` and print the output if something's gone wrong.
func build() {
	log.Println("Running build command!")

	cmd := exec.Command("go", "build")

	output, err := cmd.Output()

	if err == nil {
		log.Println("Build ok.")
	} else {
		log.Println("Error while building:\n",string(output))
	}
}

func matchesPattern(pattern *regexp.Regexp, file string) bool {
	return pattern.MatchString(file)
}

// Call `build()` periodically (every WorkDelay seconds) if
// there are any jobs to do. Jobs are detected and fed by the
// FS watcher.
func builder(jobs <-chan string) {
	ticker := time.Tick(time.Duration(WorkDelay * 1e9))

	for {
		<-jobs

		build()

		<-ticker
	}
}

func main() {
	flag.Parse()

	watcher, err := fsnotify.NewWatcher()
	defer watcher.Close()

	if err != nil {
		log.Fatal(err)
	}

	err = watcher.Watch(*flag_directory)

	if err != nil {
		log.Fatal(err)
	}

	pattern := regexp.MustCompile(*flag_pattern)
	jobs := make(chan string)

	go builder(jobs)

	for {
		select {
		case ev := <-watcher.Event:
			if ev.Name != "" && matchesPattern(pattern, ev.Name) {
				select {
					case jobs <- ev.Name:
					default:
				}
			}
		case err := <-watcher.Error:
			log.Fatal(err)
		}
	}
}
