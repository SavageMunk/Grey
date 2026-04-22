// Package alerter sends alert notifications to Discord, Slack, or any generic
// webhook endpoint using the payload format appropriate for each destination.
package alerter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/savagemunk/grey/internal/config"
)

type Alerter struct {
	alerts map[string]config.AlertConfig
	client *http.Client
}

func New(alerts map[string]config.AlertConfig) *Alerter {
	return &Alerter{
		alerts: alerts,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Alert holds the data for a single alert event.
type Alert struct {
	// Watcher is the name of the watcher that triggered the alert.
	Watcher string
	// AlertName is the key in the alerts config section to deliver to.
	AlertName string
	// Type is the watcher type: "log" or "http".
	Type string
	// Message is the human-readable description of what triggered the alert.
	Message string
}

func (a *Alerter) Send(alert Alert) error {
	cfg, ok := a.alerts[alert.AlertName]
	if !ok {
		return fmt.Errorf("alert %q not configured", alert.AlertName)
	}
	if cfg.Webhook == "" {
		return fmt.Errorf("alert %q has no webhook URL", alert.AlertName)
	}

	payload, err := buildPayload(alert, cfg)
	if err != nil {
		return err
	}

	resp, err := a.client.Post(cfg.Webhook, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("sending alert to %s: %w", cfg.Webhook, err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook %s returned %d", cfg.Webhook, resp.StatusCode)
	}
	return nil
}

func buildPayload(alert Alert, cfg config.AlertConfig) ([]byte, error) {
	ts := time.Now().UTC().Format(time.RFC3339)
	msg := fmt.Sprintf("[%s] %s | %s", ts, alert.Watcher, alert.Message)

	// Explicit config type wins; fall back to key name for backwards compatibility
	// with configs that name their alert "discord" or "slack".
	format := cfg.Type
	if format == "" {
		format = alert.AlertName
	}

	var body any
	switch format {
	case "discord":
		body = map[string]string{"content": msg}
	case "slack":
		body = map[string]string{"text": msg}
	default:
		body = map[string]string{
			"watcher":   alert.Watcher,
			"type":      alert.Type,
			"message":   alert.Message,
			"timestamp": ts,
		}
	}

	return json.Marshal(body)
}
