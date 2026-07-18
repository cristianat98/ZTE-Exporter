package zteclient

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// WANStatus represents the router's WAN connection state at the time of
// the scrape. Connected reports whether the connection is fully up;
// intermediate states (e.g. "Connecting") count as not connected.
type WANStatus struct {
	Connected             bool
	UptimeSeconds         uint64
	LeaseRemainingSeconds uint64
}

const wanStatusScript = "wan_internetstatus_lua.lua"

// wanStatusViewTag is the menuView tag for the router's WAN status page.
const wanStatusViewTag = "wanStatus"

// connectedStatus is the router's status string for a fully-established
// WAN connection, not yet verified against a live router; every other
// status value (including "Connecting") maps to Connected: false.
const connectedStatus = "Connected"

// GetWANStatus fetches the router's WAN connection status, connection
// uptime, and remaining DHCP lease time.
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

	status, err := wanStatusFromParams(params)
	if err != nil {
		return nil, err
	}

	slog.Debug("fetched WAN status", "connected", status.Connected)
	return status, nil
}

func wanStatusFromParams(params map[string]string) (*WANStatus, error) {
	rawStatus, ok := params["ConnectionStatus"]
	if !ok {
		return nil, fmt.Errorf("missing ConnectionStatus in WAN status response")
	}

	uptime, err := parseRequiredUint(params, "WANUptime")
	if err != nil {
		return nil, err
	}

	lease, err := parseRequiredUint(params, "LeaseTimeRemain")
	if err != nil {
		return nil, err
	}

	return &WANStatus{
		Connected:             strings.EqualFold(rawStatus, connectedStatus),
		UptimeSeconds:         uptime,
		LeaseRemainingSeconds: lease,
	}, nil
}
