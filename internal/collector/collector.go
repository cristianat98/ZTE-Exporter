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
// login succeeds, each of the four data fetches below (LAN, WLAN, device
// info, WAN) is independently guarded: a single fetch failure omits only
// that fetch's metrics for the cycle, leaving the rest of the scrape intact.
//
// Collector holds only immutable *prometheus.Desc fields (built once in
// New and never mutated); Collect computes every value locally and sends
// fully-formed metrics to ch via prometheus.NewConstMetric, rather than
// storing mutable prometheus.Gauge state on the struct. This keeps
// concurrent Collect calls (e.g. overlapping scrapes) from racing on
// shared Set/Collect pairs.
type Collector struct {
	cfg *config.Config

	upDesc                       *prometheus.Desc
	lanConnectedDesc             *prometheus.Desc
	wlanConnectedDesc            *prometheus.Desc
	routerInfoDesc               *prometheus.Desc
	wanConnectedDesc             *prometheus.Desc
	wanUptimeSecondsDesc         *prometheus.Desc
	wanLeaseRemainingSecondsDesc *prometheus.Desc
	wanBytesReceivedDesc         *prometheus.Desc
	wanBytesSentDesc             *prometheus.Desc
}

// New creates a Collector for the router described by cfg.
func New(cfg *config.Config) *Collector {
	return &Collector{
		cfg: cfg,
		upDesc: prometheus.NewDesc(
			"zte_up",
			"Whether the last scrape of the router succeeded (1) or failed (0).",
			nil, nil,
		),
		lanConnectedDesc: prometheus.NewDesc(
			"zte_lan_connected_devices",
			"Number of devices currently connected to the router's LAN ports.",
			nil, nil,
		),
		wlanConnectedDesc: prometheus.NewDesc(
			"zte_wlan_connected_devices",
			"Number of devices currently connected to the router's WLAN (WiFi).",
			nil, nil,
		),
		routerInfoDesc: prometheus.NewDesc(
			"zte_router_info",
			"Router identity and firmware information. Value is always 1; details are in labels.",
			[]string{"model", "software_version", "hardware_version", "serial_number", "boot_version", "build_date"},
			nil,
		),
		wanConnectedDesc: prometheus.NewDesc(
			"zte_wan_connected",
			"Whether the router's WAN connection is up (1) or not (0). Intermediate states such as \"Connecting\" report 0.",
			nil, nil,
		),
		wanUptimeSecondsDesc: prometheus.NewDesc(
			"zte_wan_uptime_seconds",
			"WAN connection uptime, in seconds. Distinct from zte_uptime_seconds (router system uptime).",
			nil, nil,
		),
		wanLeaseRemainingSecondsDesc: prometheus.NewDesc(
			"zte_wan_lease_remaining_seconds",
			"Remaining time on the WAN connection's DHCP lease, in seconds.",
			nil, nil,
		),
		wanBytesReceivedDesc: prometheus.NewDesc(
			"zte_wan_received_bytes_total",
			"Cumulative bytes received on the WAN interface. Resets on router reboot/interface reset.",
			nil, nil,
		),
		wanBytesSentDesc: prometheus.NewDesc(
			"zte_wan_sent_bytes_total",
			"Cumulative bytes sent on the WAN interface. Resets on router reboot/interface reset.",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.upDesc
	ch <- c.lanConnectedDesc
	ch <- c.wlanConnectedDesc
	ch <- c.routerInfoDesc
	ch <- c.wanConnectedDesc
	ch <- c.wanUptimeSecondsDesc
	ch <- c.wanLeaseRemainingSecondsDesc
	ch <- c.wanBytesReceivedDesc
	ch <- c.wanBytesSentDesc
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
		ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, 0)
		return
	}

	if err := client.Login(ctx); err != nil {
		slog.Error("login failed", "error", err, "duration", time.Since(start))
		ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, 0)
		return
	}

	slog.Debug("login succeeded", "duration", time.Since(start))
	ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, 1)

	c.collectLAN(ctx, client, ch)
	c.collectWLAN(ctx, client, ch)
	c.collectDeviceInfo(ctx, client, ch)
	c.collectWANStatus(ctx, client, ch)

	slog.Debug("scrape finished", "duration", time.Since(start))
}

func (c *Collector) collectLAN(ctx context.Context, client *zteclient.Client, ch chan<- prometheus.Metric) {
	devices, err := client.GetLANDevices(ctx)
	if err != nil {
		slog.Warn("LAN devices fetch failed", "error", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.lanConnectedDesc, prometheus.GaugeValue, float64(len(devices)))
}

func (c *Collector) collectWLAN(ctx context.Context, client *zteclient.Client, ch chan<- prometheus.Metric) {
	devices, err := client.GetWLANDevices(ctx)
	if err != nil {
		slog.Warn("WLAN devices fetch failed", "error", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.wlanConnectedDesc, prometheus.GaugeValue, float64(len(devices)))
}

// collectDeviceInfo emits the router's identity/firmware info as a
// single labeled gauge (value 1); an individual missing field becomes
// an empty label value rather than omitting the whole metric, since
// these fields are all sourced from one successful fetch.
func (c *Collector) collectDeviceInfo(ctx context.Context, client *zteclient.Client, ch chan<- prometheus.Metric) {
	info, err := client.GetDeviceInfo(ctx)
	if err != nil {
		slog.Warn("device info fetch failed", "error", err)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.routerInfoDesc, prometheus.GaugeValue, 1,
		stringOrEmpty(info.Model),
		stringOrEmpty(info.SoftwareVersion),
		stringOrEmpty(info.HardwareVersion),
		stringOrEmpty(info.SerialNumber),
		stringOrEmpty(info.BootVersion),
		stringOrEmpty(info.BuildDate),
	)
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// collectWANStatus emits whichever WAN fields GetWANStatus was able to
// parse this cycle; a malformed individual field (e.g. lease time) does
// not prevent its siblings (e.g. connected status) from being reported.
func (c *Collector) collectWANStatus(ctx context.Context, client *zteclient.Client, ch chan<- prometheus.Metric) {
	status, err := client.GetWANStatus(ctx)
	if err != nil {
		slog.Warn("WAN status fetch failed", "error", err)
		return
	}

	if status.Connected != nil {
		ch <- prometheus.MustNewConstMetric(c.wanConnectedDesc, prometheus.GaugeValue, boolToFloat(*status.Connected))
	}
	if status.UptimeSeconds != nil {
		ch <- prometheus.MustNewConstMetric(c.wanUptimeSecondsDesc, prometheus.GaugeValue, float64(*status.UptimeSeconds))
	}
	if status.LeaseRemainingSeconds != nil {
		ch <- prometheus.MustNewConstMetric(c.wanLeaseRemainingSecondsDesc, prometheus.GaugeValue, float64(*status.LeaseRemainingSeconds))
	}
	if status.BytesReceived != nil {
		ch <- prometheus.MustNewConstMetric(c.wanBytesReceivedDesc, prometheus.CounterValue, float64(*status.BytesReceived))
	}
	if status.BytesSent != nil {
		ch <- prometheus.MustNewConstMetric(c.wanBytesSentDesc, prometheus.CounterValue, float64(*status.BytesSent))
	}
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
