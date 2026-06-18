package config

import (
	"testing"
	"time"
)

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, "ZTE_HOST", "192.168.1.1")
	setEnv(t, "ZTE_USERNAME", "admin")
	setEnv(t, "ZTE_PASSWORD", "secret")
	setEnv(t, "ZTE_MODEL", "")
	setEnv(t, "ZTE_SCRAPE_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != defaultModel {
		t.Errorf("expected default model %q, got %q", defaultModel, cfg.Model)
	}
	if cfg.ScrapeTimeout != defaultScrapeTimeout {
		t.Errorf("expected default timeout %v, got %v", defaultScrapeTimeout, cfg.ScrapeTimeout)
	}
}

func TestLoadCustomValues(t *testing.T) {
	setEnv(t, "ZTE_HOST", "10.0.0.1")
	setEnv(t, "ZTE_USERNAME", "user")
	setEnv(t, "ZTE_PASSWORD", "pass")
	setEnv(t, "ZTE_MODEL", "H3600P")
	setEnv(t, "ZTE_SCRAPE_TIMEOUT", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ScrapeTimeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", cfg.ScrapeTimeout)
	}
}

func TestLoadMissingRequiredFields(t *testing.T) {
	setEnv(t, "ZTE_HOST", "")
	setEnv(t, "ZTE_USERNAME", "")
	setEnv(t, "ZTE_PASSWORD", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error when required fields are missing")
	}
}

func TestLoadInvalidTimeout(t *testing.T) {
	setEnv(t, "ZTE_HOST", "192.168.1.1")
	setEnv(t, "ZTE_USERNAME", "admin")
	setEnv(t, "ZTE_PASSWORD", "secret")
	setEnv(t, "ZTE_SCRAPE_TIMEOUT", "not-a-duration")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an invalid scrape timeout")
	}
}
