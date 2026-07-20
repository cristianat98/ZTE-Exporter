# Flows

This exporter has a single HTTP endpoint, `/metrics`, scraped by
Prometheus.

## `/metrics` scrape flow

```mermaid
sequenceDiagram
    participant Prometheus
    participant Exporter as Exporter (/metrics)
    participant Collector
    participant Router as ZTE H3600P

    Prometheus->>Exporter: GET /metrics
    Exporter->>Collector: Collect()
    Collector->>Router: GET login_entry (session token)
    Router-->>Collector: sess_token (JSON)
    Collector->>Router: GET login_token
    Router-->>Collector: login_token (XML)
    Collector->>Router: POST login_entry (sha256(password+login_token))
    alt login failed
        Router-->>Collector: loginErrMsg / lockingTime
        Collector-->>Exporter: zte_up=0 (no other metrics)
    else login succeeded
        Router-->>Collector: login_need_refresh=0
        Collector-->>Exporter: zte_up=1
        Collector->>Router: GET menuView + menuData accessdev_landevs_lua.lua
        alt LAN fetch failed
            Collector-->>Exporter: zte_lan_connected_devices omitted
        else LAN fetch succeeded
            Collector-->>Exporter: zte_lan_connected_devices=N
        end
        Collector->>Router: GET menuView + menuData accessdev_ssiddev_lua.lua
        alt WLAN fetch failed
            Collector-->>Exporter: zte_wlan_connected_devices omitted
        else WLAN fetch succeeded
            Collector-->>Exporter: zte_wlan_connected_devices=N
        end
        Collector->>Router: GET menuView + menuData devmgr_statusmgr_lua.lua
        alt health fetch failed
            Collector-->>Exporter: health metrics omitted
        else health fetch succeeded
            Collector-->>Exporter: zte_cpu_usage_percent, memory gauge(s), zte_uptime_seconds
        end
        Collector->>Router: GET menuView(ethWanStatus) + menuData eth_interface_status_lua.lua
        Collector->>Router: GET menuData wan_internet_lua.lua
        alt WAN fetch failed
            Collector-->>Exporter: WAN metrics omitted
        else WAN fetch succeeded
            Collector-->>Exporter: zte_wan_connected, zte_wan_uptime_seconds, zte_wan_lease_remaining_seconds (when present)
        end
    end
    Exporter-->>Prometheus: metrics response
```

## Notes

- A fresh login is performed on every scrape in this version; no session
  is cached between scrapes.
- A login failure results in `zte_up=0` and the scrape omits every other
  metric, rather than reporting zeroed or fabricated values.
- Once login succeeds, `zte_up=1` regardless of what happens next. The
  LAN, WLAN, health, and WAN fetches then run independently — the four
  are guarded separately, so a single fetch failure (e.g. an unparseable
  WAN response) only omits that fetch's own metrics for the cycle,
  leaving the rest of the scrape intact.
- Login and all four fetches run sequentially against the exporter's
  single `ScrapeTimeout`, with no per-fetch sub-budget; a slow fetch can
  crowd out later ones within the same cycle.
- The WAN fetch primes with `menuView` once, then issues two `menuData`
  calls in sequence rather than re-priming between them, confirmed
  against a live H3600P: `eth_interface_status_lua.lua`'s response isn't
  used for any metric, but fetching it is what establishes the WAN
  sub-page's server-side context — `wan_internet_lua.lua` (the actual
  data source) only succeeds when fetched immediately after it.
