# Grey

A lightweight log and HTTP watcher with alerting and Prometheus metrics. Watch log files for regex patterns, poll HTTP endpoints for status codes, send alerts to Discord, Slack, or any webhook, and expose Prometheus metrics — all from a single binary with zero runtime dependencies.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Part of the [SavageMunk](https://github.com/SavageMunk) open source suite alongside [Mystique](https://github.com/SavageMunk/Mystique).

---

## Installation

### Pre-built binary

Download the latest release from the [releases page](https://github.com/SavageMunk/grey/releases) and place it anywhere in your `$PATH`.

### Build from source

Requires Go 1.21+.

```bash
git clone https://github.com/SavageMunk/grey.git
cd grey
go build -o grey ./cmd/grey
```

### Docker

```bash
docker pull savagemunkee/grey:latest
```

---

## Quick Start

1. Copy the example config:

```bash
cp config.example.yaml config.yaml
```

2. Open `config.yaml` and set your log paths, webhook URLs, and alert targets. At minimum you need one watcher and one alert destination:

```yaml
watchers:
  - name: nginx errors
    type: log
    path: /var/log/nginx/error.log
    pattern: "ERROR|CRITICAL"
    cooldown: 10m
    alert: discord

alerts:
  discord:
    webhook: https://discord.com/api/webhooks/YOUR_ID/YOUR_TOKEN
```

3. Run:

```bash
./grey -config config.yaml
```

---

## Configuration Reference

Grey is configured entirely via a single YAML file. No flags, no environment variables, no database.

```yaml
watchers:
  - name: nginx errors          # display name used in alerts and metrics
    type: log                   # "log" or "http"
    path: /var/log/nginx/error.log
    pattern: "ERROR|CRITICAL"   # Go regex
    cooldown: 10m               # silence this watcher for 10 minutes after firing
    alert: discord              # references a key in the alerts section

  - name: app health check
    type: http
    url: http://localhost:8080/health
    expect_status: 200          # alert if any other status code is received
    interval: 30s               # how often to poll
    cooldown: 5m
    alert: slack

alerts:
  discord:
    webhook: https://discord.com/api/webhooks/...
  slack:
    webhook: https://hooks.slack.com/services/...
  myserver:
    webhook: https://my-custom-endpoint.com/alert

metrics:
  enabled: true
  port: 9101                    # Prometheus metrics served at http://host:9101/metrics
```

### Watcher fields

| Field | Type | Log | HTTP | Default | Description |
|---|---|---|---|---|---|
| `name` | string | required | required | — | Display name used in alerts and metrics labels |
| `type` | string | required | required | — | `log` or `http` |
| `path` | string | required | n/a | — | Absolute path to the log file to watch |
| `pattern` | string | required | n/a | — | Go regex matched against each new line appended to the file |
| `url` | string | n/a | required | — | URL to poll |
| `expect_status` | int | n/a | optional | `200` | Expected HTTP status code — any other code triggers an alert |
| `interval` | duration | n/a | optional | `30s` | How often to poll the endpoint |
| `cooldown` | duration | optional | optional | `10m` | How long to silence this watcher after an alert fires |
| `alert` | string | optional | optional | — | Key from the `alerts` section to use for notifications |

### Alert payload formats

Grey auto-detects the alert format based on the key name in the `alerts` section.

**Discord** (key is `discord`):
```json
{"content": "[2024-01-01T12:00:00Z] nginx errors | pattern matched in /var/log/nginx/error.log: 2024/01/01 ERR connection refused"}
```

**Slack** (key is `slack`):
```json
{"text": "[2024-01-01T12:00:00Z] app health check | HTTP check failed: http://localhost:8080/health returned 503 (expected 200)"}
```

**Generic webhook** (any other key):
```json
{
  "watcher": "app health check",
  "type": "http",
  "message": "HTTP check failed: http://localhost:8080/health returned 503 (expected 200)",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

---

## Docker Usage

```bash
docker run -d \
  --name grey \
  -v /path/to/config.yaml:/etc/grey/config.yaml:ro \
  -v /var/log/nginx:/var/log/nginx:ro \
  -p 9101:9101 \
  savagemunkee/grey:latest
```

See [docker-compose.example.yml](docker-compose.example.yml) for a full compose setup with Grey and Prometheus running side by side.

---

## Prometheus Metrics

Grey exposes the following metrics at `http://host:9101/metrics`:

| Metric | Type | Labels | Description |
|---|---|---|---|
| `grey_alerts_total` | counter | `watcher`, `type` | Total number of alerts fired |
| `grey_pattern_matches_total` | counter | `watcher` | Total number of log lines matched |
| `grey_watcher_up` | gauge | `watcher`, `type` | `1` if last check was healthy, `0` if not |
| `grey_http_response_code` | gauge | `watcher` | Last HTTP status code received |

### Prometheus scrape config

Add this to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: grey
    static_configs:
      - targets: ['localhost:9101']
```

### Grafana

Build panels using the metrics above. Useful PromQL queries to get started:

```promql
# Alert rate per watcher over the last 5 minutes
rate(grey_alerts_total[5m])

# HTTP watchers currently reporting unhealthy
grey_watcher_up{type="http"} == 0

# Last HTTP status code received per watcher
grey_http_response_code
```

---

## Graceful Shutdown

Grey handles `SIGINT` and `SIGTERM`. On shutdown it stops all file watchers and HTTP pollers cleanly before exiting.

---

## Contributing

Issues and pull requests are welcome. Please open an issue before submitting a large change so we can discuss the approach first.

---

## License

MIT — see [LICENSE](LICENSE) for details.