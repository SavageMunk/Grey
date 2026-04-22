// Package config loads and validates the Grey configuration file.
// The config file is a single YAML document; this package owns the schema,
// defaults, and validation rules for every field.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level structure for a Grey config file.
type Config struct {
	// Watchers is the list of log and HTTP watchers to run.
	Watchers []WatcherConfig `yaml:"watchers"`
	// Alerts is a named map of webhook destinations that watchers can reference.
	Alerts map[string]AlertConfig `yaml:"alerts"`
	// Metrics controls the Prometheus /metrics endpoint.
	Metrics MetricsConfig `yaml:"metrics"`
}

// WatcherConfig describes a single log or HTTP watcher.
type WatcherConfig struct {
	// Name is the display name used in alerts and Prometheus metric labels.
	Name string `yaml:"name"`
	// Type is the watcher kind: "log" or "http".
	Type string `yaml:"type"`
	// Path is the log file to tail (log watchers only).
	Path string `yaml:"path"`
	// Pattern is the Go regular expression to match against new log lines (log watchers only).
	Pattern string `yaml:"pattern"`
	// URL is the endpoint to poll (http watchers only).
	URL string `yaml:"url"`
	// ExpectStatus is the HTTP status code that means healthy (http watchers only). Defaults to 200.
	ExpectStatus int `yaml:"expect_status"`
	// Interval is how often to poll the endpoint (http watchers only). Defaults to 30s.
	Interval time.Duration `yaml:"interval"`
	// Cooldown is how long to suppress further alerts after one fires. Defaults to 10m.
	Cooldown time.Duration `yaml:"cooldown"`
	// Alert is the key in the alerts map to send notifications to.
	Alert string `yaml:"alert"`
}

// AlertConfig describes a webhook destination that watchers can send alerts to.
type AlertConfig struct {
	Webhook string `yaml:"webhook"`
	// Type controls the alert payload format: "discord", "slack", or "webhook".
	// If omitted, the alert key name is used for backwards compatibility.
	Type string `yaml:"type"`
}

// MetricsConfig controls the Prometheus metrics endpoint.
type MetricsConfig struct {
	// Enabled controls whether the /metrics endpoint is started.
	Enabled bool `yaml:"enabled"`
	// Port is the port the /metrics endpoint listens on. Defaults to 9101.
	Port int `yaml:"port"`
}

func Load(path string) (*Config, error) {
	// Config files are expected to be small YAML documents, not large data files.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Defaults are applied after validation so that validate sees only what
	// the user explicitly configured, not values we silently injected.
	applyDefaults(&cfg)
	return &cfg, nil
}

func validate(cfg *Config) error {
	seen := make(map[string]bool)
	for i, w := range cfg.Watchers {
		if w.Name == "" {
			return fmt.Errorf("watcher[%d]: name is required", i)
		}
		if seen[w.Name] {
			return fmt.Errorf("watcher %q: duplicate name — metric labels must be unique", w.Name)
		}
		seen[w.Name] = true

		switch w.Type {
		case "log":
			if w.Path == "" {
				return fmt.Errorf("watcher %q: path is required for log watchers", w.Name)
			}
			if w.Pattern == "" {
				return fmt.Errorf("watcher %q: pattern is required for log watchers", w.Name)
			}
		case "http":
			if w.URL == "" {
				return fmt.Errorf("watcher %q: url is required for http watchers", w.Name)
			}
		default:
			return fmt.Errorf("watcher %q: unknown type %q (must be log or http)", w.Name, w.Type)
		}

		if w.Alert != "" {
			alert, ok := cfg.Alerts[w.Alert]
			if !ok {
				return fmt.Errorf("watcher %q: alert %q not defined in alerts section", w.Name, w.Alert)
			}
			if alert.Webhook == "" {
				return fmt.Errorf("watcher %q: alert %q has no webhook URL", w.Name, w.Alert)
			}
		}
	}
	return nil
}

func applyDefaults(cfg *Config) {
	if cfg.Metrics.Port == 0 {
		cfg.Metrics.Port = 9101
	}
	for i := range cfg.Watchers {
		w := &cfg.Watchers[i]
		if w.Type == "http" {
			if w.ExpectStatus == 0 {
				w.ExpectStatus = 200
			}
			if w.Interval == 0 {
				w.Interval = 30 * time.Second
			}
		}
		if w.Cooldown == 0 {
			w.Cooldown = 10 * time.Minute
		}
	}
}
