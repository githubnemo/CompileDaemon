package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	pollingWatcher "github.com/radovskyb/watcher"
)

func directoryShouldBeTracked(cfg *WatcherConfig, path string) bool {
	return cfg.flagRecursive == true && !cfg.flagExcludedDirs.Matches(path)
}

func pathMatches(cfg *WatcherConfig, path string) bool {
	base := filepath.Base(path)
	return (cfg.flagIncludedFiles.Matches(base) || matchesPattern(cfg.pattern, path)) &&
		!cfg.flagExcludedFiles.Matches(base)
}

type WatcherConfig struct {
	flagVerbose         bool
	flagPolling         bool
	flagRecursive       bool
	flagPollingInterval int
	flagDirectories     globList
	flagExcludedDirs    globList
	flagExcludedFiles   globList
	flagIncludedFiles   globList
	pattern             *regexp.Regexp
}

type FileWatcher interface {
	Close() error
	AddFiles() error
	add(path string) error
	Watch(jobs chan<- string)
	getConfig() *WatcherConfig
}

type NotifyWatcher struct {
	watcher *fsnotify.Watcher
	cfg     *WatcherConfig
}

func (n NotifyWatcher) Close() error {
	return n.watcher.Close()
}

func (n NotifyWatcher) AddFiles() error {
	return addFiles(n)
}

func (n NotifyWatcher) Watch(jobs chan<- string) {
	for {
		select {
		case ev := <-n.watcher.Events:
			if ev.Op&fsnotify.Remove == fsnotify.Remove || ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Create == fsnotify.Create {
				// Assume it is a directory and track it.
				if directoryShouldBeTracked(n.cfg, ev.Name) {
					n.watcher.Add(ev.Name)
				}
				if pathMatches(n.cfg, ev.Name) {
					jobs <- ev.Name
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

func (n NotifyWatcher) add(path string) error {
	return n.watcher.Add(path)
}

func (n NotifyWatcher) getConfig() *WatcherConfig {
	return n.cfg
}

type PollingWatcher struct {
	watcher *pollingWatcher.Watcher
	cfg     *WatcherConfig
}

func (p PollingWatcher) Close() error {
	p.watcher.Close()
	return nil
}

func (p PollingWatcher) AddFiles() error {
	p.watcher.AddFilterHook(pollingWatcher.RegexFilterHook(p.cfg.pattern, false))

	return addFiles(p)
}

func (p PollingWatcher) Watch(jobs chan<- string) {
	// Start the watching process.
	go func() {
		if err := p.watcher.Start(time.Duration(p.cfg.flagPollingInterval) * time.Millisecond); err != nil {
			log.Fatalln(err)
		}
	}()

	for {
		select {
		case event := <-p.watcher.Event:
			if p.cfg.flagVerbose {
				// Print the event's info.
				fmt.Println(event)
			}

			if pathMatches(p.cfg, event.Path) {
				jobs <- event.String()
			}
		case err := <-p.watcher.Error:
			if err == pollingWatcher.ErrWatchedFileDeleted {
				continue
			}
			log.Fatalln(err)
		case <-p.watcher.Closed:
			return
		}
	}
}

func (p PollingWatcher) add(path string) error {
	return p.watcher.Add(path)
}

func (p PollingWatcher) getConfig() *WatcherConfig {
	return p.cfg
}

func NewWatcher(cfg *WatcherConfig) (FileWatcher, error) {
	if cfg == nil {
		err := errors.New("no config specified")
		return nil, err
	}
	if cfg.flagPolling {
		w := pollingWatcher.New()
		return PollingWatcher{
			watcher: w,
			cfg:     cfg,
		}, nil
	} else {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, err
		}
		return NotifyWatcher{
			watcher: w,
			cfg:     cfg,
		}, nil
	}
}

func addFiles(fw FileWatcher) error {
	cfg := fw.getConfig()
	for _, flagDirectory := range cfg.flagDirectories {
		if cfg.flagRecursive == true {
			err := filepath.WalkDir(flagDirectory, func(path string, entry os.DirEntry, err error) error {
				if err == nil && entry.IsDir() {
					if cfg.flagExcludedDirs.Matches(path) {
						return filepath.SkipDir
					} else {
						if cfg.flagVerbose {
							log.Printf("Watching directory '%s' for changes.\n", path)
						}
						return fw.add(path)
					}
				}
				return err
			})

			if err != nil {
				return fmt.Errorf("filepath.Walk(): %v", err)
			}

			if err := fw.add(flagDirectory); err != nil {
				return fmt.Errorf("FileWatcher.Add(): %v", err)
			}
		} else {
			if err := fw.add(flagDirectory); err != nil {
				return fmt.Errorf("FileWatcher.AddFiles(): %v", err)
			}
		}
	}
	return nil
}
