// Package metrics registers the Grey Prometheus metrics and serves the
// /metrics and /healthz endpoints.
package metrics

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AlertsTotal counts the number of alerts fired, labelled by watcher name and type.
var AlertsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "grey_alerts_total",
		Help: "Total number of alerts fired.",
	},
	[]string{"watcher", "type"},
)

// PatternMatchesTotal counts the number of log lines that matched a watcher's
// pattern, labelled by watcher name.
var PatternMatchesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "grey_pattern_matches_total",
		Help: "Total number of log pattern matches.",
	},
	[]string{"watcher"},
)

// WatcherUp is 1 if the watcher's last check was healthy, 0 otherwise.
// Labelled by watcher name and type ("log" or "http").
var WatcherUp = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "grey_watcher_up",
		Help: "1 if the watcher's last check was healthy, 0 otherwise.",
	},
	[]string{"watcher", "type"},
)

// HTTPResponseCode holds the last HTTP status code received by each HTTP
// watcher, labelled by watcher name.
var HTTPResponseCode = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "grey_http_response_code",
		Help: "Last HTTP status code received by the watcher.",
	},
	[]string{"watcher"},
)

// Register registers all Grey metrics with the default Prometheus registry.
// Must be called once at startup before any metrics are recorded.
func Register() {
	prometheus.MustRegister(AlertsTotal, PatternMatchesTotal, WatcherUp, HTTPResponseCode)
}

// Serve starts the metrics HTTP server on the given port and blocks until ctx
// is cancelled, then shuts the server down gracefully.
func Serve(ctx context.Context, port int) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	}
}
