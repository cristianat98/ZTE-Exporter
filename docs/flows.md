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
        Collector->>Router: GET menuView(statusMgr, Menu3Location=0) + menuData devmgr_statusmgr_lua.lua
        alt device info fetch failed
            Collector-->>Exporter: zte_router_info omitted
        else device info fetch succeeded
            Collector-->>Exporter: zte_router_info (model/versions/serial/build date labels)
        end
        Collector->>Router: GET menuView(ethWanStatus) + menuData eth_interface_status_lua.lua
        Collector->>Router: GET menuData wan_internet_lua.lua (TypeUplink=2, pageType=1)
        alt WAN fetch failed
            Collector-->>Exporter: WAN metrics omitted
        else WAN fetch succeeded
            Collector-->>Exporter: zte_wan_connected, zte_wan_uptime_seconds, zte_wan_lease_remaining_seconds, zte_wan_received_bytes_total, zte_wan_sent_bytes_total (each when present)
        end
        Collector->>Router: GET menuView + menuData eth_lanstatus_lua.lua
        alt LAN traffic fetch failed
            Collector-->>Exporter: LAN traffic metrics omitted
        else LAN traffic fetch succeeded
            Collector-->>Exporter: zte_lan_received_bytes_total, zte_lan_sent_bytes_total per port (each when present)
        end
        Collector->>Router: GET menuView + menuData wlan_status_lua.lua
        alt WLAN traffic fetch failed
            Collector-->>Exporter: WLAN traffic metrics omitted
        else WLAN traffic fetch succeeded
            Collector-->>Exporter: zte_wlan_received_bytes_total, zte_wlan_sent_bytes_total, zte_wlan_received_packets_total, zte_wlan_sent_packets_total per SSID slot (each when present)
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
  LAN, WLAN, device info, WAN, LAN traffic, and WLAN traffic fetches then
  run independently — the six are guarded separately, so a single fetch
  failure (e.g. an unparseable WAN response) only omits that fetch's own
  metrics for the cycle, leaving the rest of the scrape intact.
- Login and all six fetches run sequentially against the exporter's
  single `ScrapeTimeout`, with no per-fetch sub-budget; a slow fetch can
  crowd out later ones within the same cycle.
- Each of the six fetches — LAN devices, WLAN devices, device info, WAN
  status, LAN traffic, and WLAN traffic — re-primes its own `menuView`
  context independently rather than chaining off another fetch's
  priming. LAN traffic (`eth_lanstatus_lua.lua`) and WLAN traffic
  (`wlan_status_lua.lua`) both prime the same `localNetStatus` menuView
  tag the existing LAN/WLAN device fetches already use, the same way
  those two fetches do today, rather than following the WAN fetch's
  two-script sequential-priming shape: this keeps a LAN traffic fetch
  failure from being able to cascade into a WLAN traffic failure, and
  vice versa.
- The WAN fetch primes with `menuView` once, then issues two `menuData`
  calls in sequence rather than re-priming between them, confirmed
  against a live H3600P: `eth_interface_status_lua.lua` establishes the
  WAN sub-page's server-side context that `wan_internet_lua.lua` (the
  actual connection status/uptime/lease data) requires. The second call
  also requires the `TypeUplink=2`/`pageType=1` query parameters,
  confirmed against a live H3600P's real request — without them the
  router rejects the request as if the session were invalid.
  `eth_interface_status_lua.lua`'s response also supplies the
  interface's byte counters
  (`zte_wan_received_bytes_total`/`zte_wan_sent_bytes_total`).
- The device info fetch's `menuView` call requires a `Menu3Location=0`
  query parameter alongside the `statusMgr` tag, confirmed against a
  live H3600P; `devmgr_statusmgr_lua.lua`'s response is the router's
  device-info page (model, firmware/hardware/boot versions, serial
  number, firmware build date), not the CPU/memory/uptime data
  originally assumed for this endpoint — no working source for those
  fields has been found on this router/firmware, so they are not
  exposed by this exporter.
