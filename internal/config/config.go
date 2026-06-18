package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds the exporter's runtime configuration, sourced from
// environment variables so credentials never need to be passed as
// CLI flags (which would leak into process listings).
type Config struct {
	Host          string
	Username      string
	Password      string
	Model         string
	ScrapeTimeout time.Duration
}

const (
	defaultModel         = "H3600P"
	defaultScrapeTimeout = 10 * time.Second
)

// Load reads the exporter configuration from environment variables:
//
//	ZTE_HOST            router address, e.g. 192.168.1.1 (required)
//	ZTE_USERNAME         router web UI username (required)
//	ZTE_PASSWORD         router web UI password (required)
//	ZTE_MODEL             router model, defaults to "H3600P"
//	ZTE_SCRAPE_TIMEOUT   per-scrape timeout, e.g. "10s" (defaults to 10s)
func Load() (*Config, error) {
	cfg := &Config{
		Host:          os.Getenv("ZTE_HOST"),
		Username:      os.Getenv("ZTE_USERNAME"),
		Password:      os.Getenv("ZTE_PASSWORD"),
		Model:         os.Getenv("ZTE_MODEL"),
		ScrapeTimeout: defaultScrapeTimeout,
	}

	if cfg.Model == "" {
		cfg.Model = defaultModel
	}

	if raw := os.Getenv("ZTE_SCRAPE_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid ZTE_SCRAPE_TIMEOUT %q: %w", raw, err)
		}
		cfg.ScrapeTimeout = d
	}

	var missing []string
	if cfg.Host == "" {
		missing = append(missing, "ZTE_HOST")
	}
	if cfg.Username == "" {
		missing = append(missing, "ZTE_USERNAME")
	}
	if cfg.Password == "" {
		missing = append(missing, "ZTE_PASSWORD")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}

	return cfg, nil
}
