package watcher

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/savagemunk/grey/internal/config"
)

// LogWatcher tails a log file and calls onMatch for each new line that matches
// the configured regex pattern, subject to a per-watcher cooldown.
type LogWatcher struct {
	cfg      config.WatcherConfig
	re       *regexp.Regexp
	onMatch  func(watcher, line string)
	onError  func(error)
	mu       sync.Mutex
	once     sync.Once
	cooldown time.Time
	done     chan struct{}
}

// NewLogWatcher compiles the pattern and returns a LogWatcher ready to Run.
// onMatch is called for each matching line once the cooldown allows.
// onError is called for fsnotify errors; pass nil to ignore them.
func NewLogWatcher(cfg config.WatcherConfig, onMatch func(watcher, line string), onError func(error)) (*LogWatcher, error) {
	re, err := regexp.Compile(cfg.Pattern)
	if err != nil {
		return nil, fmt.Errorf("watcher %q: invalid pattern: %w", cfg.Name, err)
	}
	return &LogWatcher{
		cfg:     cfg,
		re:      re,
		onMatch: onMatch,
		onError: onError,
		done:    make(chan struct{}),
	}, nil
}

// Run opens the log file, seeks to EOF so only new lines are processed, and
// starts a background goroutine that watches for writes and handles rotation.
func (lw *LogWatcher) Run() error {
	f, err := os.Open(lw.cfg.Path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", lw.cfg.Path, err)
	}

	// Seek to end so we only tail new content.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		f.Close()
		return err
	}
	if err := watcher.Add(lw.cfg.Path); err != nil {
		f.Close()
		watcher.Close()
		return err
	}

	reader := bufio.NewReader(f)

	go func() {
		// Defers handle cleanup for both the normal exit and the case where
		// reopenFile fails mid-loop; f may have already been closed in that
		// branch, but the error from closing an already-closed file is ignored.
		defer f.Close()
		defer watcher.Close()
		for {
			select {
			case <-lw.done:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					lw.readNewLines(reader)
				}
				if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
					// File rotated — reopen.
					f.Close()
					f, err = lw.reopenFile()
					if err != nil {
						return
					}
					reader = bufio.NewReader(f)
					watcher.Add(lw.cfg.Path)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				if lw.onError != nil {
					lw.onError(err)
				}
			}
		}
	}()

	return nil
}

func (lw *LogWatcher) readNewLines(r *bufio.Reader) {
	for {
		line, err := r.ReadString('\n')
		if line != "" && lw.re.MatchString(line) {
			lw.mu.Lock()
			now := time.Now()
			if now.After(lw.cooldown) {
				lw.cooldown = now.Add(lw.cfg.Cooldown)
				lw.mu.Unlock()
				lw.onMatch(lw.cfg.Name, line)
			} else {
				lw.mu.Unlock()
			}
		}
		if err != nil {
			return
		}
	}
}

func (lw *LogWatcher) reopenFile() (*os.File, error) {
	// Wait briefly for the new file to appear after rotation.
	var err error
	for i := 0; i < 10; i++ {
		var f *os.File
		f, err = os.Open(lw.cfg.Path)
		if err == nil {
			return f, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, err
}

// Stop shuts down the log watcher. Safe to call more than once.
func (lw *LogWatcher) Stop() {
	lw.once.Do(func() { close(lw.done) })
}
