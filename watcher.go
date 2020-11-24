package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	pollingWatcher "github.com/radovskyb/watcher"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

type FileWatcher interface {
	Close() error
	AddFiles(pattern *regexp.Regexp) error
	Watch(jobs chan<- string, pattern *regexp.Regexp)
}

type NotifyWatcher struct {
	watcher *fsnotify.Watcher
}

func (n NotifyWatcher) Close() error {
	return n.watcher.Close()
}

func (n NotifyWatcher) AddFiles(pattern *regexp.Regexp) error {
	for _, flagDirectory := range flagDirectories {
		if *flagRecursive == true {
			err := filepath.Walk(flagDirectory, func(path string, info os.FileInfo, err error) error {
				if err == nil && info.IsDir() {
					if flagExcludedDirs.Matches(path) {
						return filepath.SkipDir
					} else {
						if *flagVerbose {
							log.Printf("Watching directory '%s' for changes.\n", path)
						}
						return n.watcher.Add(path)
					}
				}
				return err
			})

			if err != nil {
				return fmt.Errorf("filepath.Walk(): %v", err)
			}

			if err := n.watcher.Add(flagDirectory); err != nil {
				return fmt.Errorf("NotifyWatcher.Add(): %v", err)
			}
		} else {
			if err := n.watcher.Add(flagDirectory); err != nil {
				return fmt.Errorf("NotifyWatcher.Add(): %v", err)
			}
		}
	}
	return nil
}

func (n NotifyWatcher) Watch(jobs chan<- string, pattern *regexp.Regexp) {
	for {
		select {
		case ev := <-n.watcher.Events:
			if ev.Op&fsnotify.Remove == fsnotify.Remove || ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Create == fsnotify.Create {
				base := filepath.Base(ev.Name)

				// Assume it is a directory and track it.
				if *flagRecursive == true && !flagExcludedDirs.Matches(ev.Name) {
					n.watcher.Add(ev.Name)
				}

				if flagIncludedFiles.Matches(base) || matchesPattern(pattern, ev.Name) {
					if !flagExcludedFiles.Matches(base) {
						jobs <- ev.Name
					}
				}
			}

		case err := <-n.watcher.Errors:
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

type PollingWatcher struct {
	watcher *pollingWatcher.Watcher
}

func (p PollingWatcher) Close() error {
	p.watcher.Close()
	return nil
}

func (p PollingWatcher) AddFiles(pattern *regexp.Regexp) error {
	p.watcher.AddFilterHook(pollingWatcher.RegexFilterHook(pattern, false))

	for _, flagDirectory := range flagDirectories {
		if *flagRecursive == true {
			if err := p.watcher.AddRecursive(flagDirectory); err != nil {
				return fmt.Errorf("PollingWatcher.AddFiles(): %v", err)
			}
		} else {
			if err := p.watcher.Add(flagDirectory); err != nil {
				return fmt.Errorf("NotifyWatcher.AddFiles(): %v", err)
			}
		}
	}

	if *flagVerbose {
		for path, f := range p.watcher.WatchedFiles() {
			fmt.Printf("Watching %s: %s\n", path, f.Name())
		}
	}
	return nil
}

func (p PollingWatcher) Watch(jobs chan<- string, pattern *regexp.Regexp) {
	// Parse the interval string into a time.Duration.
	parsedInterval, err := time.ParseDuration(PollingInterval)
	if err != nil {
		log.Fatalln(err)
	}

	// Start the watching process.
	go func() {
		if err := p.watcher.Start(parsedInterval); err != nil {
			log.Fatalln(err)
		}
	}()

	for {
		select {
		case event := <-p.watcher.Event:
			if *flagVerbose {
				// Print the event's info.
				fmt.Println(event)
			}

			base := filepath.Base(event.Path)

			if flagIncludedFiles.Matches(base) || matchesPattern(pattern, event.Path) {
				if !flagExcludedFiles.Matches(base) {
					jobs <- event.String()
				}
			}
		case err := <-p.watcher.Error:
			if err == pollingWatcher.ErrWatchedFileDeleted {
				fmt.Println(err)
				continue
			}
			log.Fatalln(err)
		case <-p.watcher.Closed:
			return
		}
	}
}

func NewWatcher(usePolling bool) (FileWatcher, error) {
	if usePolling {
		w := pollingWatcher.New()
		return PollingWatcher{
			watcher: w,
		}, nil
	} else {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, err
		}
		return NotifyWatcher{
			watcher: w,
		}, nil
	}
}
