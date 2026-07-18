// Package collector implements a Prometheus collector that scrapes a ZTE
// H3600P router on every collection cycle.
package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cristianat98/zte-exporter/internal/config"
	"github.com/cristianat98/zte-exporter/internal/zteclient"
)

// Collector implements prometheus.Collector, fetching fresh data from the
// router on every Collect call rather than caching between scrapes. Once
// login succeeds, each of the four data fetches below (LAN, WLAN, health,
// WAN) is independently guarded: a single fetch failure omits only that
// fetch's metrics for the cycle, leaving the rest of the scrape intact.
type Collector struct {
	cfg *config.Config

	up                       prometheus.Gauge
	lanConnectedTotal        prometheus.Gauge
	wlanConnectedTotal       prometheus.Gauge
	cpuUsagePercent          prometheus.Gauge
	memoryUsedBytes          prometheus.Gauge
	memoryTotalBytes         prometheus.Gauge
	memoryUsagePercent       prometheus.Gauge
	uptimeSeconds            prometheus.Gauge
	wanConnected             prometheus.Gauge
	wanUptimeSeconds         prometheus.Gauge
	wanLeaseRemainingSeconds prometheus.Gauge
}

// New creates a Collector for the router described by cfg.
func New(cfg *config.Config) *Collector {
	return &Collector{
		cfg: cfg,
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_up",
			Help: "Whether the last scrape of the router succeeded (1) or failed (0).",
		}),
		lanConnectedTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_lan_connected_devices",
			Help: "Number of devices currently connected to the router's LAN ports.",
		}),
		wlanConnectedTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_wlan_connected_devices",
			Help: "Number of devices currently connected to the router's WLAN (WiFi).",
		}),
		cpuUsagePercent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_cpu_usage_percent",
			Help: "Router CPU usage percentage (0-100).",
		}),
		memoryUsedBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_memory_used_bytes",
			Help: "Router memory currently used, in bytes. Only reported when the router exposes raw memory bytes.",
		}),
		memoryTotalBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_memory_total_bytes",
			Help: "Router total memory, in bytes. Only reported when the router exposes raw memory bytes.",
		}),
		memoryUsagePercent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_memory_usage_percent",
			Help: "Router memory usage percentage (0-100). Only reported when the router does not expose raw memory bytes.",
		}),
		uptimeSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_uptime_seconds",
			Help: "Router system uptime, in seconds.",
		}),
		wanConnected: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_wan_connected",
			Help: "Whether the router's WAN connection is up (1) or not (0). Intermediate states such as \"Connecting\" report 0.",
		}),
		wanUptimeSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_wan_uptime_seconds",
			Help: "WAN connection uptime, in seconds. Distinct from zte_uptime_seconds (router system uptime).",
		}),
		wanLeaseRemainingSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "zte_wan_lease_remaining_seconds",
			Help: "Remaining time on the WAN connection's DHCP lease, in seconds.",
		}),
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.up.Describe(ch)
	c.lanConnectedTotal.Describe(ch)
	c.wlanConnectedTotal.Describe(ch)
	c.cpuUsagePercent.Describe(ch)
	c.memoryUsedBytes.Describe(ch)
	c.memoryTotalBytes.Describe(ch)
	c.memoryUsagePercent.Describe(ch)
	c.uptimeSeconds.Describe(ch)
	c.wanConnected.Describe(ch)
	c.wanUptimeSeconds.Describe(ch)
	c.wanLeaseRemainingSeconds.Describe(ch)
}

// Collect implements prometheus.Collector. A login failure reports
// zte_up=0 and omits every other metric, matching the router being
// entirely unreachable. Once login succeeds, zte_up=1 regardless of what
// happens next: each of the four data fetches below runs independently,
// and a fetch failure only omits that fetch's own metrics for this cycle.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.ScrapeTimeout)
	defer cancel()

	slog.Debug("starting scrape", "host", c.cfg.Host)
	start := time.Now()

	client, err := zteclient.NewClient(c.cfg.Host, c.cfg.Username, c.cfg.Password, c.cfg.ScrapeTimeout)
	if err != nil {
		slog.Error("creating client failed", "error", err, "duration", time.Since(start))
		c.up.Set(0)
		c.up.Collect(ch)
		return
	}

	if err := client.Login(ctx); err != nil {
		slog.Error("login failed", "error", err, "duration", time.Since(start))
		c.up.Set(0)
		c.up.Collect(ch)
		return
	}

	slog.Debug("login succeeded", "duration", time.Since(start))
	c.up.Set(1)
	c.up.Collect(ch)

	c.collectLAN(ctx, client, ch)
	c.collectWLAN(ctx, client, ch)
	c.collectHealth(ctx, client, ch)
	c.collectWANStatus(ctx, client, ch)

	slog.Debug("scrape finished", "duration", time.Since(start))
}

func (c *Collector) collectLAN(ctx context.Context, client *zteclient.Client, ch chan<- prometheus.Metric) {
	devices, err := client.GetLANDevices(ctx)
	if err != nil {
		slog.Warn("LAN devices fetch failed", "error", err)
		return
	}
	c.lanConnectedTotal.Set(float64(len(devices)))
	c.lanConnectedTotal.Collect(ch)
}

func (c *Collector) collectWLAN(ctx context.Context, client *zteclient.Client, ch chan<- prometheus.Metric) {
	devices, err := client.GetWLANDevices(ctx)
	if err != nil {
		slog.Warn("WLAN devices fetch failed", "error", err)
		return
	}
	c.wlanConnectedTotal.Set(float64(len(devices)))
	c.wlanConnectedTotal.Collect(ch)
}

func (c *Collector) collectHealth(ctx context.Context, client *zteclient.Client, ch chan<- prometheus.Metric) {
	health, err := client.GetHealth(ctx)
	if err != nil {
		slog.Warn("health fetch failed", "error", err)
		return
	}

	c.cpuUsagePercent.Set(health.CPUUsagePercent)
	c.cpuUsagePercent.Collect(ch)
	c.uptimeSeconds.Set(float64(health.UptimeSeconds))
	c.uptimeSeconds.Collect(ch)

	if health.HasMemoryBytes {
		c.memoryUsedBytes.Set(float64(health.MemoryUsedBytes))
		c.memoryUsedBytes.Collect(ch)
		c.memoryTotalBytes.Set(float64(health.MemoryTotalBytes))
		c.memoryTotalBytes.Collect(ch)
		return
	}
	c.memoryUsagePercent.Set(health.MemoryUsagePercent)
	c.memoryUsagePercent.Collect(ch)
}

func (c *Collector) collectWANStatus(ctx context.Context, client *zteclient.Client, ch chan<- prometheus.Metric) {
	status, err := client.GetWANStatus(ctx)
	if err != nil {
		slog.Warn("WAN status fetch failed", "error", err)
		return
	}

	c.wanConnected.Set(boolToFloat(status.Connected))
	c.wanConnected.Collect(ch)
	c.wanUptimeSeconds.Set(float64(status.UptimeSeconds))
	c.wanUptimeSeconds.Collect(ch)
	c.wanLeaseRemainingSeconds.Set(float64(status.LeaseRemainingSeconds))
	c.wanLeaseRemainingSeconds.Collect(ch)
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
