package zteclient

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// WANStatus represents the router's WAN connection state at the time of
// the scrape. Connected reports whether the connection is fully up;
// intermediate states (e.g. "Connecting") count as not connected (KD3).
type WANStatus struct {
	Connected             bool
	UptimeSeconds         uint64
	LeaseRemainingSeconds uint64
}

const wanStatusScript = "wan_internetstatus_lua.lua"

// connectedStatus is the router's status string for a fully-established
// WAN connection. Unverified against a live router (see plan Q2); every
// other status value (including "Connecting") maps to Connected: false.
const connectedStatus = "Connected"

// GetWANStatus fetches the router's WAN connection status, connection
// uptime, and remaining DHCP lease time.
//
// The wan_internetstatus_lua.lua field names below (ConnectionStatus,
// WANUptime, LeaseTimeRemain) are unverified against a live router (see
// plan Q2) and will need adjusting once the actual response is inspected.
func (c *Client) GetWANStatus(ctx context.Context) (*WANStatus, error) {
	// First request sets up the menu context the router expects before
	// serving menuData, mirroring what the browser UI does.
	if _, err := c.get(ctx, fmt.Sprintf("?_type=menuView&_tag=wanStatus&_=%d", c.nextGUID())); err != nil {
		return nil, fmt.Errorf("setting up WAN status context: %w", err)
	}

	body, err := c.get(ctx, fmt.Sprintf("?_type=menuData&_tag=%s&_=%d", wanStatusScript, c.nextGUID()))
	if err != nil {
		return nil, fmt.Errorf("fetching WAN status: %w", err)
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
