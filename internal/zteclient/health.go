package zteclient

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
)

// Health represents the router's system health at the time of the scrape.
type Health struct {
	CPUUsagePercent float64
	UptimeSeconds   uint64

	// HasMemoryBytes reports whether MemoryUsedBytes/MemoryTotalBytes were
	// populated from the router response. When false, only
	// MemoryUsagePercent is valid (KD4 percentage fallback).
	HasMemoryBytes     bool
	MemoryUsedBytes    uint64
	MemoryTotalBytes   uint64
	MemoryUsagePercent float64
}

const healthScript = "devmgr_statusmgr_lua.lua"

// GetHealth fetches the router's CPU usage, memory usage, and uptime.
//
// The devmgr_statusmgr_lua.lua field names below (CPUUsage, MemTotal,
// MemFree, MemUsage, SysUpTime) are unverified against a live router (see
// plan Q1) and will need adjusting once the actual response is inspected.
func (c *Client) GetHealth(ctx context.Context) (*Health, error) {
	// First request sets up the menu context the router expects before
	// serving menuData, mirroring what the browser UI does.
	if _, err := c.get(ctx, fmt.Sprintf("?_type=menuView&_tag=deviceStatus&_=%d", c.nextGUID())); err != nil {
		return nil, fmt.Errorf("setting up health context: %w", err)
	}

	body, err := c.get(ctx, fmt.Sprintf("?_type=menuData&_tag=%s&_=%d", healthScript, c.nextGUID()))
	if err != nil {
		return nil, fmt.Errorf("fetching health: %w", err)
	}

	params, err := parseFlatParams(body)
	if err != nil {
		return nil, err
	}

	health, err := healthFromParams(params)
	if err != nil {
		return nil, err
	}

	slog.Debug("fetched health", "cpu", health.CPUUsagePercent, "uptime", health.UptimeSeconds)
	return health, nil
}

func healthFromParams(params map[string]string) (*Health, error) {
	cpu, err := parseRequiredFloat(params, "CPUUsage")
	if err != nil {
		return nil, err
	}

	uptime, err := parseRequiredUint(params, "SysUpTime")
	if err != nil {
		return nil, err
	}

	health := &Health{
		CPUUsagePercent: cpu,
		UptimeSeconds:   uptime,
	}

	if total, hasTotal := params["MemTotal"]; hasTotal {
		if free, hasFree := params["MemFree"]; hasFree {
			totalBytes, err := strconv.ParseUint(total, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing MemTotal: %w", err)
			}
			freeBytes, err := strconv.ParseUint(free, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing MemFree: %w", err)
			}
			if freeBytes > totalBytes {
				return nil, fmt.Errorf("MemFree (%d) exceeds MemTotal (%d)", freeBytes, totalBytes)
			}
			health.HasMemoryBytes = true
			health.MemoryTotalBytes = totalBytes
			health.MemoryUsedBytes = totalBytes - freeBytes
			return health, nil
		}
	}

	memPercent, err := parseRequiredFloat(params, "MemUsage")
	if err != nil {
		return nil, fmt.Errorf("no memory bytes fields and %w", err)
	}
	health.MemoryUsagePercent = memPercent
	return health, nil
}

func parseRequiredFloat(params map[string]string, key string) (float64, error) {
	raw, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("missing %s in health response", key)
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}
	return value, nil
}

func parseRequiredUint(params map[string]string, key string) (uint64, error) {
	raw, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("missing %s in health response", key)
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}
	return value, nil
}
