// Grey watches log files and HTTP endpoints for problems and sends alerts to
// Discord, Slack, or any webhook. It also exposes Prometheus metrics.
// Configuration is a single YAML file; run with -config to specify the path.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/savagemunk/grey/internal/alerter"
	"github.com/savagemunk/grey/internal/config"
	"github.com/savagemunk/grey/internal/metrics"
	"github.com/savagemunk/grey/internal/watcher"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grey: %v\n", err)
		os.Exit(1)
	}

	metrics.Register()

	al := alerter.New(cfg.Alerts)

	var logWatchers []*watcher.LogWatcher
	var httpWatchers []*watcher.HTTPWatcher

	for _, wcfg := range cfg.Watchers {
		wcfg := wcfg
		switch wcfg.Type {
		case "log":
			lw, err := watcher.NewLogWatcher(wcfg, func(name, line string) {
				metrics.PatternMatchesTotal.WithLabelValues(name).Inc()
				msg := fmt.Sprintf("pattern matched in %s: %s", wcfg.Path, truncate(line, 200))

				if wcfg.Alert != "" {
					metrics.AlertsTotal.WithLabelValues(name, "log").Inc()
					if err := al.Send(alerter.Alert{
						Watcher:   name,
						AlertName: wcfg.Alert,
						Type:      "log",
						Message:   msg,
					}); err != nil {
						log.Printf("alert send failed for %q: %v", name, err)
					}
				}
			}, nil)
			if err != nil {
				// Partial startup is intentional — grey runs whatever watchers
				// it can start successfully and skips the rest.
				log.Printf("skipping log watcher %q: %v", wcfg.Name, err)
				continue
			}
			if err := lw.Run(); err != nil {
				log.Printf("starting log watcher %q: %v", wcfg.Name, err)
				continue
			}
			metrics.WatcherUp.WithLabelValues(wcfg.Name, "log").Set(1)
			logWatchers = append(logWatchers, lw)

		case "http":
			hw := watcher.NewHTTPWatcher(wcfg,
				func(name string, code int, ok bool) {
					up := 0.0
					if ok {
						up = 1.0
					}
					metrics.WatcherUp.WithLabelValues(name, "http").Set(up)
					if code > 0 {
						metrics.HTTPResponseCode.WithLabelValues(name).Set(float64(code))
					}
				},
				func(name, msg string) {
					if wcfg.Alert == "" {
						return
					}
					metrics.AlertsTotal.WithLabelValues(name, "http").Inc()
					if err := al.Send(alerter.Alert{
						Watcher:   name,
						AlertName: wcfg.Alert,
						Type:      "http",
						Message:   msg,
					}); err != nil {
						log.Printf("alert send failed for %q: %v", name, err)
					}
				},
			)
			hw.Run()
			httpWatchers = append(httpWatchers, hw)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	if cfg.Metrics.Enabled {
		go func() {
			log.Printf("metrics listening on :%d", cfg.Metrics.Port)
			if err := metrics.Serve(ctx, cfg.Metrics.Port); err != nil {
				log.Printf("metrics server: %v", err)
			}
		}()
	}

	log.Printf("grey started — watching %d watcher(s)", len(logWatchers)+len(httpWatchers))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("grey shutting down...")
	// cancel() stops the metrics server by cancelling its context.
	// Stop() closes each watcher's done channel so its goroutine exits on the
	// next select iteration. Watchers finish asynchronously; we allow up to
	// 10 seconds for any in-flight alert sends or log reads to complete.
	cancel()
	for _, lw := range logWatchers {
		lw.Stop()
	}
	for _, hw := range httpWatchers {
		hw.Stop()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	select {
	case <-time.After(200 * time.Millisecond):
	case <-shutdownCtx.Done():
		log.Println("grey: shutdown timed out after 10s")
	}

	log.Println("grey stopped")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
