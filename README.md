# ZTE-Exporter

A Prometheus exporter for the ZTE H3600P router. It authenticates against
the router's web UI on every scrape and exposes connected-device and
router-health metrics — no SNMP or CLI access required.

The first release targets the H3600P specifically; it does not claim
compatibility with other ZTE router models.

## Metrics

| Metric | Description |
| --- | --- |
| `zte_up` | Whether the last scrape of the router succeeded (1) or failed (0). |
| `zte_lan_connected_devices` | Number of devices currently connected to the router's LAN ports. |
| `zte_wlan_connected_devices` | Number of devices currently connected to the router's WLAN (WiFi). |
| `zte_cpu_usage_percent` | Router CPU usage percentage (0-100). |
| `zte_memory_used_bytes` | Router memory currently used, in bytes. Only reported when the router exposes raw memory bytes. |
| `zte_memory_total_bytes` | Router total memory, in bytes. Only reported when the router exposes raw memory bytes. |
| `zte_memory_usage_percent` | Router memory usage percentage (0-100). Only reported when the router does not expose raw memory bytes. |
| `zte_uptime_seconds` | Router system uptime, in seconds. |
| `zte_wan_connected` | Whether the router's WAN connection is up (1) or not (0). Intermediate states such as "Connecting" report 0. |
| `zte_wan_uptime_seconds` | WAN connection uptime, in seconds. Distinct from `zte_uptime_seconds` (router system uptime). |
| `zte_wan_lease_remaining_seconds` | Remaining time on the WAN connection's DHCP lease, in seconds. Only meaningful for DHCP-based WAN connections; not reported for PPPoE, which has no lease concept. |
| `zte_exporter_build_info` | Constant `1` metric labeled by exporter version. |

If login fails (bad credentials, unreachable router), the exporter reports
`zte_up=0` and omits every other metric rather than emitting zeroed or
fabricated values. Once login succeeds, `zte_up=1` and each of the LAN,
WLAN, health, and WAN metric groups above is fetched and reported
independently: a failure fetching one group (e.g. an unparseable WAN
status response) only omits that group's own metrics for the cycle,
leaving the rest of the scrape intact. Within the health and WAN groups,
individual fields degrade the same way: a malformed memory reading, for
example, omits only the memory metric(s) while CPU usage and uptime are
still reported.

## Configuration

The exporter is configured via environment variables:

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `ZTE_HOST` | yes | — | Router address, e.g. `192.168.1.1` |
| `ZTE_USERNAME` | yes | — | Router web UI username |
| `ZTE_PASSWORD` | yes | — | Router web UI password |
| `ZTE_MODEL` | no | `H3600P` | Router model |
| `ZTE_SCRAPE_TIMEOUT` | no | `10s` | Per-scrape timeout (Go duration syntax) |
| `LOG_LEVEL` | no | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |

The HTTP listen address is set via the `-web.listen-address` flag
(default `0.0.0.0:9111`).

## Running

```sh
ZTE_HOST=192.168.1.1 ZTE_USERNAME=admin ZTE_PASSWORD=secret go run ./cmd
```

Metrics are then available at `http://localhost:9111/metrics`.

A sample env file is provided at [`.example.env`](.example.env); copy it to
`.env` and adjust the values, then load it with your shell or compose tool
of choice.

### Docker

```sh
docker build -t zte-exporter .
docker run --rm -p 9111:9111 \
  -e ZTE_HOST=192.168.1.1 \
  -e ZTE_USERNAME=admin \
  -e ZTE_PASSWORD=secret \
  zte-exporter
```

Pre-built images are published to
[`cristianat98/zte-exporter`](https://hub.docker.com/r/cristianat98/zte-exporter)
on every push to `master` (see [Release](#release) below).

### Prometheus scrape config

```yaml
scrape_configs:
  - job_name: zte-exporter
    static_configs:
      - targets: ["localhost:9111"]
```

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

### pre-commit

Code style, vetting, builds, and linting are enforced via
[pre-commit](https://pre-commit.com/):

```sh
pip install pre-commit
pre-commit install
pre-commit run --all-files
```

See [`.pre-commit-config.yaml`](.pre-commit-config.yaml) for the configured
hooks and [`.golangci.yml`](.golangci.yml) for the lint ruleset.

See [`docs/uml.md`](docs/uml.md) and [`docs/flows.md`](docs/flows.md) for
the project's architecture and scrape-flow diagrams.

## CI/CD

- **[Check code](.github/workflows/check-code.yml)** runs on every pull
  request and on pushes to `master`: pre-commit hooks, tests with coverage,
  and a [SonarCloud](https://sonarcloud.io/) scan. The Sonar job requires
  the `SONAR_TOKEN` secret plus the `SONAR_ORGANIZATION` and
  `SONAR_PROJECT_KEY` repo variables.
- **[Release](.github/workflows/release.yml)** runs on pushes to `master`:
  bumps the semantic version tag and builds/pushes the Docker image to
  Docker Hub. Requires the `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`
  secrets.
