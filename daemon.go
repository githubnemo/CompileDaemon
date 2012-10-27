package main

import (
	"log"
	"flag"
	"os/exec"
	"regexp"
	"time"
	"path/filepath"
	"github.com/howeyc/fsnotify"
)

// Seconds to wait for the next job to begin
const WorkDelay = 5

// Pattern to match files which trigger a build
const FilePattern = `(.+\.go|.+\.c)$`

var (
	flag_directory = flag.String("directory", "", "Directory to watch for changes")
	flag_pattern = flag.String("pattern", FilePattern, "Pattern of watched files")
	flag_recursive = flag.Bool("recursive", true, "Watch all dirs. recursively")
)


// Run `go build` and print the output if something's gone wrong.
func build() {
	log.Println("Running build command!")

	cmd := exec.Command("go", "build")

	cmd.Dir = *flag_directory

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

	if *flag_recursive == true {
		filepath.Walk(*flag_directory, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				return watcher.Watch(path)
			}
			return err
		})

	} else {
		err := watcher.Watch(*flag_directory)

		if err != nil {
			log.Fatal(err)
		}
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
