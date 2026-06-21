// Package collector implements a Prometheus collector that scrapes a ZTE
// H3600P router on every collection cycle.
package collector

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cristianat98/zte-exporter/internal/config"
	"github.com/cristianat98/zte-exporter/internal/zteclient"
)

// Collector implements prometheus.Collector, fetching fresh data from the
// router on every Collect call rather than caching between scrapes.
type Collector struct {
	cfg *config.Config

	up                prometheus.Gauge
	lanConnectedTotal prometheus.Gauge
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
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.up.Describe(ch)
	c.lanConnectedTotal.Describe(ch)
}

// Collect implements prometheus.Collector. On a failed scrape it reports
// zte_up=0 and intentionally omits the rest of the metrics, instead of
// emitting fabricated or zeroed values.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.ScrapeTimeout)
	defer cancel()

	devices, err := c.scrape(ctx)
	if err != nil {
		slog.Error("scrape failed", "error", err)
		c.up.Set(0)
		c.up.Collect(ch)
		return
	}

	c.up.Set(1)
	c.lanConnectedTotal.Set(float64(len(devices)))

	c.up.Collect(ch)
	c.lanConnectedTotal.Collect(ch)
}

func (c *Collector) scrape(ctx context.Context) ([]zteclient.Device, error) {
	client, err := zteclient.NewClient(c.cfg.Host, c.cfg.Username, c.cfg.Password, c.cfg.ScrapeTimeout)
	if err != nil {
		return nil, err
	}

	if err := client.Login(ctx); err != nil {
		return nil, err
	}

	return client.GetLANDevices(ctx)
}
