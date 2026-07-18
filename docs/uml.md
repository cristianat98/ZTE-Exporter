# Architecture (UML)

## Module overview

```mermaid
classDiagram
    class main {
        +main()
        +buildInfoCollector() prometheus.Collector
    }

    class Config {
        +Host string
        +Username string
        +Password string
        +Model string
        +ScrapeTimeout time.Duration
        +LogLevel slog.Level
        +Load() (*Config, error)
    }

    class Collector {
        -cfg *Config
        -up prometheus.Gauge
        -lanConnectedTotal prometheus.Gauge
        -wlanConnectedTotal prometheus.Gauge
        -cpuUsagePercent prometheus.Gauge
        -memoryUsedBytes prometheus.Gauge
        -memoryTotalBytes prometheus.Gauge
        -memoryUsagePercent prometheus.Gauge
        -uptimeSeconds prometheus.Gauge
        -wanConnected prometheus.Gauge
        -wanUptimeSeconds prometheus.Gauge
        -wanLeaseRemainingSeconds prometheus.Gauge
        +New(cfg *Config) *Collector
        +Describe(ch chan~*prometheus.Desc~)
        +Collect(ch chan~prometheus.Metric~)
        -collectLAN(ctx, client, ch)
        -collectWLAN(ctx, client, ch)
        -collectHealth(ctx, client, ch)
        -collectWANStatus(ctx, client, ch)
    }

    class Client {
        -baseURL string
        -username string
        -password string
        -httpClient *http.Client
        -guid int64
        +NewClient(host, username, password string, timeout time.Duration) (*Client, error)
        +Login(ctx context.Context) error
        +GetLANDevices(ctx context.Context) ([]Device, error)
        +GetWLANDevices(ctx context.Context) ([]Device, error)
        +GetHealth(ctx context.Context) (*Health, error)
        +GetWANStatus(ctx context.Context) (*WANStatus, error)
        -getSessionToken(ctx context.Context) (string, error)
        -getLoginToken(ctx context.Context) (string, error)
    }

    class Device {
        +MACAddress string
        +IPAddress string
        +HostName string
        +Active bool
        +NetworkType string
    }

    class Health {
        +CPUUsagePercent float64
        +HasMemoryBytes bool
        +MemoryUsedBytes uint64
        +MemoryTotalBytes uint64
        +MemoryUsagePercent float64
        +UptimeSeconds uint64
    }

    class WANStatus {
        +Connected bool
        +UptimeSeconds uint64
        +LeaseRemainingSeconds uint64
    }

    main --> Config : loads
    main --> Collector : registers
    Collector --> Config : reads
    Collector --> Client : creates per scrape
    Client --> Device : returns
    Client --> Health : returns
    Client --> WANStatus : returns
```

## Notes

- `Collector` implements `prometheus.Collector`. `Collect` logs in once per
  scrape, then runs the LAN, WLAN, health, and WAN status fetches
  independently: a login failure reports `zte_up=0` and omits every other
  metric, but once login succeeds `zte_up=1` regardless of what happens
  next, and each fetch's failure only omits that fetch's own metrics for
  the cycle.
- `Client` is created fresh per scrape in this version; session reuse
  across scrapes is tracked separately (see project task "Reliability:
  scrape failures & session handling").
- `Device` is shared between LAN and WLAN collection; the `NetworkType`
  field distinguishes them. `GetWLANDevices` reuses the same
  `parseDevices` machinery as `GetLANDevices`, parameterized by script
  name and id element.
- `Health` and `WANStatus` are parsed via a shared flat-parameter helper
  (`parseFlatParams`), since their router responses are a flat
  `ParaName`/`ParaValue` sequence rather than the nested per-device
  `Instance` sections `Device` parsing uses.
