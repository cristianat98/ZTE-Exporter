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
        Collector->>Router: GET menuView localNetStatus
        Collector->>Router: GET menuData accessdev_landevs_lua.lua
        Router-->>Collector: LAN devices XML
        alt parse/router error
            Collector-->>Exporter: zte_up=0 (no other metrics)
        else success
            Collector-->>Exporter: zte_up=1, zte_lan_connected_devices=N
        end
    end
    Exporter-->>Prometheus: metrics response
```

## Notes

- A fresh login is performed on every scrape in this version; no session
  is cached between scrapes.
- Any failure (network, auth, parsing) results in `zte_up=0` and the
  scrape intentionally omits device/health metrics rather than reporting
  zeroed or fabricated values.
