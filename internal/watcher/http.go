// Package watcher implements log file tailing and HTTP endpoint polling.
package watcher

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/savagemunk/grey/internal/config"
)

// HTTPWatcher polls an HTTP endpoint at a configured interval and fires
// callbacks for metrics updates (onResult, every check) and alert delivery
// (onAlert, only when a check fails and the cooldown has expired).
type HTTPWatcher struct {
	cfg      config.WatcherConfig
	onResult func(watcher string, statusCode int, ok bool)
	onAlert  func(watcher, message string)
	mu       sync.Mutex
	once     sync.Once
	cooldown time.Time
	client   *http.Client
	done     chan struct{}
}

// NewHTTPWatcher creates an HTTPWatcher. onResult is called on every poll for
// metric updates. onAlert is called on failure once the cooldown has expired.
func NewHTTPWatcher(
	cfg config.WatcherConfig,
	onResult func(watcher string, statusCode int, ok bool),
	onAlert func(watcher, message string),
) *HTTPWatcher {
	return &HTTPWatcher{
		cfg:      cfg,
		onResult: onResult,
		onAlert:  onAlert,
		client:   &http.Client{Timeout: 10 * time.Second},
		done:     make(chan struct{}),
	}
}

// Run starts the polling loop in a background goroutine. It performs an
// immediate check on start, then repeats at the configured interval.
func (hw *HTTPWatcher) Run() {
	go func() {
		hw.check()
		ticker := time.NewTicker(hw.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-hw.done:
				return
			case <-ticker.C:
				hw.check()
			}
		}
	}()
}

func (hw *HTTPWatcher) check() {
	code, err := hw.poll()
	ok := err == nil && code == hw.cfg.ExpectStatus
	hw.onResult(hw.cfg.Name, code, ok)

	if !ok && hw.onAlert != nil {
		hw.mu.Lock()
		now := time.Now()
		if now.After(hw.cooldown) {
			hw.cooldown = now.Add(hw.cfg.Cooldown)
			hw.mu.Unlock()
			hw.onAlert(hw.cfg.Name, hw.buildMessage(code, err))
		} else {
			hw.mu.Unlock()
		}
	}
}

func (hw *HTTPWatcher) poll() (int, error) {
	resp, err := hw.client.Get(hw.cfg.URL)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode, nil
}

func (hw *HTTPWatcher) buildMessage(code int, err error) string {
	if err != nil {
		return fmt.Sprintf("HTTP check failed: %s — %s", hw.cfg.URL, err)
	}
	return fmt.Sprintf("HTTP check failed: %s returned %d (expected %d)", hw.cfg.URL, code, hw.cfg.ExpectStatus)
}

// Stop shuts down the polling loop. Safe to call more than once.
func (hw *HTTPWatcher) Stop() {
	hw.once.Do(func() { close(hw.done) })
}
