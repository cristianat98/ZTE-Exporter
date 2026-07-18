package zteclient

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
)

// Health represents the router's system health at the time of the
// scrape. Each field is a pointer so that a single unparseable or
// missing field doesn't discard its otherwise-valid siblings: a bad
// memory reading, for example, still leaves CPUUsagePercent and
// UptimeSeconds populated. A nil field means the router didn't provide
// it (or it didn't parse) this cycle.
type Health struct {
	CPUUsagePercent *float64
	UptimeSeconds   *uint64

	// MemoryUsedBytes/MemoryTotalBytes are populated together when the
	// router exposes raw memory bytes; otherwise MemoryUsagePercent is
	// the fallback (Product Contract KD4). At most one form is set.
	MemoryUsedBytes    *uint64
	MemoryTotalBytes   *uint64
	MemoryUsagePercent *float64
}

const healthScript = "devmgr_statusmgr_lua.lua"

// healthViewTag is the menuView tag for the router's device-status page.
const healthViewTag = "deviceStatus"

// GetHealth fetches the router's CPU usage, memory usage, and uptime.
// It only returns an error when the page itself couldn't be fetched or
// parsed; a malformed individual field is logged and left nil on the
// returned Health rather than failing the whole fetch.
//
// The devmgr_statusmgr_lua.lua field names below (CPUUsage, MemTotal,
// MemFree, MemUsage, SysUpTime) are not yet verified against a live
// router and may need adjusting once the actual response is inspected.
func (c *Client) GetHealth(ctx context.Context) (*Health, error) {
	body, err := c.fetchMenuData(ctx, healthViewTag, healthScript, "health")
	if err != nil {
		return nil, err
	}

	params, err := parseFlatParams(body)
	if err != nil {
		return nil, err
	}

	health := healthFromParams(params)
	slog.Debug("fetched health",
		"cpu_available", health.CPUUsagePercent != nil,
		"uptime_available", health.UptimeSeconds != nil,
		"memory_bytes_available", health.MemoryUsedBytes != nil,
		"memory_percent_available", health.MemoryUsagePercent != nil,
	)
	return health, nil
}

func healthFromParams(params map[string]string) *Health {
	health := &Health{}

	if cpu, err := parseRequiredFloat(params, "CPUUsage"); err != nil {
		slog.Warn("parsing health field failed", "field", "CPUUsage", "error", err)
	} else {
		health.CPUUsagePercent = &cpu
	}

	if uptime, err := parseRequiredUint(params, "SysUpTime"); err != nil {
		slog.Warn("parsing health field failed", "field", "SysUpTime", "error", err)
	} else {
		health.UptimeSeconds = &uptime
	}

	// MemTotal is the discriminator: once the router provides it, a
	// missing/unparseable MemFree is a malformed response, not a signal
	// to fall back to the percent form.
	if _, hasTotal := params["MemTotal"]; hasTotal {
		totalBytes, errTotal := parseRequiredUint(params, "MemTotal")
		freeBytes, errFree := parseRequiredUint(params, "MemFree")
		switch {
		case errTotal != nil:
			slog.Warn("parsing health field failed", "field", "MemTotal", "error", errTotal)
		case errFree != nil:
			slog.Warn("parsing health field failed", "field", "MemFree", "error", errFree)
		case freeBytes > totalBytes:
			slog.Warn("health field inconsistent", "field", "MemFree/MemTotal", "free", freeBytes, "total", totalBytes)
		default:
			used := totalBytes - freeBytes
			health.MemoryTotalBytes = &totalBytes
			health.MemoryUsedBytes = &used
		}
		return health
	}

	if memPercent, err := parseRequiredFloat(params, "MemUsage"); err != nil {
		slog.Warn("parsing health field failed", "field", "MemUsage", "error", err)
	} else {
		health.MemoryUsagePercent = &memPercent
	}
	return health
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
