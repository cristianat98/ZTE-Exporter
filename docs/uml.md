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
        -upDesc *prometheus.Desc
        -lanConnectedDesc *prometheus.Desc
        -wlanConnectedDesc *prometheus.Desc
        -routerInfoDesc *prometheus.Desc
        -wanConnectedDesc *prometheus.Desc
        -wanUptimeSecondsDesc *prometheus.Desc
        -wanLeaseRemainingSecondsDesc *prometheus.Desc
        -wanBytesReceivedDesc *prometheus.Desc
        -wanBytesSentDesc *prometheus.Desc
        -lanBytesReceivedDesc *prometheus.Desc
        -lanBytesSentDesc *prometheus.Desc
        -wlanBytesReceivedDesc *prometheus.Desc
        -wlanBytesSentDesc *prometheus.Desc
        -wlanPacketsReceivedDesc *prometheus.Desc
        -wlanPacketsSentDesc *prometheus.Desc
        +New(cfg *Config) *Collector
        +Describe(ch chan~*prometheus.Desc~)
        +Collect(ch chan~prometheus.Metric~)
        -collectLAN(ctx, client, ch)
        -collectWLAN(ctx, client, ch)
        -collectDeviceInfo(ctx, client, ch)
        -collectWANStatus(ctx, client, ch)
        -collectLANTraffic(ctx, client, ch)
        -collectWLANTraffic(ctx, client, ch)
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
        +GetDeviceInfo(ctx context.Context) (*DeviceInfo, error)
        +GetWANStatus(ctx context.Context) (*WANStatus, error)
        +GetLANTraffic(ctx context.Context) ([]LANPortTraffic, error)
        +GetWLANTraffic(ctx context.Context) ([]WLANSSIDTraffic, error)
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

    class DeviceInfo {
        +Model *string
        +SoftwareVersion *string
        +HardwareVersion *string
        +SerialNumber *string
        +BootVersion *string
        +BuildDate *string
    }

    class WANStatus {
        +Connected *bool
        +UptimeSeconds *uint64
        +LeaseRemainingSeconds *uint64
        +BytesReceived *uint64
        +BytesSent *uint64
    }

    class LANPortTraffic {
        +Port string
        +BytesReceived *uint64
        +BytesSent *uint64
    }

    class WLANSSIDTraffic {
        +APID string
        +ESSID string
        +Band string
        +BytesReceived *uint64
        +BytesSent *uint64
        +PacketsReceived *uint64
        +PacketsSent *uint64
    }

    main --> Config : loads
    main --> Collector : registers
    Collector --> Config : reads
    Collector --> Client : creates per scrape
    Client --> Device : returns
    Client --> DeviceInfo : returns
    Client --> WANStatus : returns
    Client --> LANPortTraffic : returns
    Client --> WLANSSIDTraffic : returns
```

## Notes

- `Collector` implements `prometheus.Collector`. `Collect` logs in once per
  scrape, then runs the LAN, WLAN, device info, WAN status, LAN traffic,
  and WLAN traffic fetches independently: a login failure reports
  `zte_up=0` and omits every other metric, but once login succeeds
  `zte_up=1` regardless of what happens next, and each fetch's failure
  only omits that fetch's own metrics for the cycle.
- `Collector` holds only immutable `*prometheus.Desc` fields (built once in
  `New`), not `prometheus.Gauge` state. `Collect` computes every value
  locally per call and sends it via `prometheus.NewConstMetric`, so
  concurrent `Collect` calls (e.g. overlapping scrapes) never share
  mutable state.
- `Client` is created fresh per scrape in this version; session reuse
  across scrapes is tracked separately (see project task "Reliability:
  scrape failures & session handling").
- `Device` is shared between LAN and WLAN collection; the `NetworkType`
  field distinguishes them. `GetWLANDevices` reuses the same
  `parseDevices` machinery as `GetLANDevices`, parameterized by script
  name and id element.
- `WANStatus` and `DeviceInfo` are both parsed via `parseSingleInstance`,
  confirmed against a live H3600P to nest one `Instance` under a
  page-specific element (`ID_WAN_COMFIG` for WAN status,
  `OBJ_DEVINFO_ID` for device info) — the same document shape `Device`
  parsing uses, just with exactly one `Instance` instead of many. Every
  field on `DeviceInfo` and `WANStatus` is a pointer: a single
  unparseable or missing field is logged and left `nil` rather than
  discarding its otherwise-valid siblings, so e.g. a malformed lease-time
  reading doesn't blank connection status/uptime too. `DeviceInfo` is
  exposed as a single labeled `zte_router_info` gauge rather than several
  metrics, so a missing field there becomes an empty label value instead
  of a `nil` pointer omitting a metric.
- The WAN status fetch primes its page context once, then issues two
  sequential `menuData` calls rather than re-priming between them:
  `eth_interface_status_lua.lua` establishes the sub-page context that
  `wan_internet_lua.lua` (the connection status/uptime/lease data)
  requires, confirmed against a live H3600P's real request order.
  `eth_interface_status_lua.lua`'s response is also parsed for
  `BytesReceived`/`BytesSent` (the WAN interface's cumulative traffic
  counters), exposed as `zte_wan_received_bytes_total` /
  `zte_wan_sent_bytes_total`.
- `GetLANTraffic` and `GetWLANTraffic` each independently re-prime their
  own `localNetStatus` menuView context via `fetchMenuData`, the same way
  `GetLANDevices`/`GetWLANDevices` already do, rather than chaining like
  the WAN status fetch's two-script sequence — a LAN traffic fetch
  failure cannot cascade into a WLAN traffic failure, or vice versa
  (KTD2). `LANPortTraffic`/`WLANSSIDTraffic` follow `WANStatus`'s
  pointer-per-field degrade philosophy: a single unparseable counter
  field is logged and left `nil` without dropping the rest of that
  port's/slot's fields.
- `GetWLANTraffic` joins each `OBJ_WLANCONFIGDRV_ID` traffic instance to
  its identity via a 3-way join confirmed against a live H3600P (KTD4):
  `_InstID` matches it to an `OBJ_WLANAP_ID` instance for `ESSID`, and its
  `WLANViewName` matches an `OBJ_WLANSETTING_ID` instance's `_InstID` for
  `Band` (`DEV.WIFI.RD1` → `2.4GHz`, `DEV.WIFI.RD2` → `5GHz`). `_InstID`
  also supplies the stable `APID` label, chosen over the user-editable
  `ESSID` so a router-UI SSID rename doesn't fragment Prometheus history
  (KD3). A join miss on either side leaves `ESSID`/`Band` as `""` rather
  than dropping the SSID slot (KTD3); `GetLANTraffic` applies the same
  label-fallback principle, using the port's `_InstID` when `AliasName`
  is missing or empty. Every LAN port and WLAN SSID slot found in the
  response is always emitted, regardless of link status or `Enable`
  state (KD2).
- The `zte_wan_*_bytes_total`, `zte_lan_*_bytes_total`, and
  `zte_wlan_*_{bytes,packets}_total` metrics are this exporter's
  Counter-type metrics; every other metric is a Gauge.
