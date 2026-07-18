package zteclient

import (
	"context"
	"log/slog"
	"strings"
)

// WANStatus represents the router's WAN connection state at the time of
// the scrape. Each field is a pointer so a single unparseable or missing
// field doesn't discard its otherwise-valid siblings: a bad lease
// reading, for example, still leaves Connected populated. Connected
// reports whether the connection is fully up; intermediate states (e.g.
// "Connecting") count as not connected.
type WANStatus struct {
	Connected             *bool
	UptimeSeconds         *uint64
	LeaseRemainingSeconds *uint64
}

const wanStatusScript = "wan_internetstatus_lua.lua"

// wanStatusViewTag is the menuView tag for the router's WAN status page.
const wanStatusViewTag = "wanStatus"

// connectedStatus is the router's status string for a fully-established
// WAN connection, not yet verified against a live router; every other
// status value (including "Connecting") maps to Connected: false.
const connectedStatus = "Connected"

// GetWANStatus fetches the router's WAN connection status, connection
// uptime, and remaining DHCP lease time. It only returns an error when
// the page itself couldn't be fetched or parsed; a malformed individual
// field is logged and left nil on the returned WANStatus rather than
// failing the whole fetch.
//
// The wan_internetstatus_lua.lua field names below (ConnectionStatus,
// WANUptime, LeaseTimeRemain) are not yet verified against a live router
// and may need adjusting once the actual response is inspected.
func (c *Client) GetWANStatus(ctx context.Context) (*WANStatus, error) {
	body, err := c.fetchMenuData(ctx, wanStatusViewTag, wanStatusScript, "WAN status")
	if err != nil {
		return nil, err
	}

	params, err := parseFlatParams(body)
	if err != nil {
		return nil, err
	}

	status := wanStatusFromParams(params)
	slog.Debug("fetched WAN status",
		"connected_available", status.Connected != nil,
		"uptime_available", status.UptimeSeconds != nil,
		"lease_available", status.LeaseRemainingSeconds != nil,
	)
	return status, nil
}

func wanStatusFromParams(params map[string]string) *WANStatus {
	status := &WANStatus{}

	if rawStatus, ok := params["ConnectionStatus"]; !ok {
		slog.Warn("parsing WAN status field failed", "field", "ConnectionStatus", "error", "missing")
	} else {
		connected := strings.EqualFold(rawStatus, connectedStatus)
		status.Connected = &connected
	}

	if uptime, err := parseRequiredUint(params, "WANUptime"); err != nil {
		slog.Warn("parsing WAN status field failed", "field", "WANUptime", "error", err)
	} else {
		status.UptimeSeconds = &uptime
	}

	if lease, err := parseRequiredUint(params, "LeaseTimeRemain"); err != nil {
		slog.Warn("parsing WAN status field failed", "field", "LeaseTimeRemain", "error", err)
	} else {
		status.LeaseRemainingSeconds = &lease
	}

	return status
}
