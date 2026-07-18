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
// "Connecting") count as not connected. LeaseRemainingSeconds is only
// meaningful for DHCP-based WAN connections; PPPoE connections have no
// lease concept and will simply leave it nil.
type WANStatus struct {
	Connected             *bool
	UptimeSeconds         *uint64
	LeaseRemainingSeconds *uint64
}

// wanConfigIDElement is the element wrapping the single WAN-connection
// Instance in wan_internet_lua.lua's response, confirmed against a live
// H3600P (a PPPoE connection).
const wanConfigIDElement = "ID_WAN_COMFIG"

const wanStatusScript = "wan_internet_lua.lua"

// wanStatusViewTag is the menuView tag for the router's WAN status page.
// Not yet confirmed against a live router.
const wanStatusViewTag = "wanStatus"

// connectedStatus is the router's status string for a fully-established
// WAN connection, confirmed against a live H3600P (field ConnStatus);
// every other status value (including "Connecting") maps to
// Connected: false.
const connectedStatus = "Connected"

// GetWANStatus fetches the router's WAN connection status, connection
// uptime, and remaining DHCP lease time (when the connection type has
// one). It only returns an error when the page itself couldn't be
// fetched or parsed; a malformed individual field is logged and left
// nil on the returned WANStatus rather than failing the whole fetch.
func (c *Client) GetWANStatus(ctx context.Context) (*WANStatus, error) {
	body, err := c.fetchMenuData(ctx, wanStatusViewTag, wanStatusScript, "WAN status")
	if err != nil {
		return nil, err
	}

	params, err := parseSingleInstance(body, wanConfigIDElement)
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

	if rawStatus, ok := params["ConnStatus"]; !ok {
		slog.Warn("parsing WAN status field failed", "field", "ConnStatus", "error", "missing")
	} else {
		connected := strings.EqualFold(rawStatus, connectedStatus)
		status.Connected = &connected
	}

	if uptime, err := parseRequiredUint(params, "UpTime"); err != nil {
		slog.Warn("parsing WAN status field failed", "field", "UpTime", "error", err)
	} else {
		status.UptimeSeconds = &uptime
	}

	// LeaseTimeRemain is a placeholder name, unconfirmed: no DHCP lease
	// field appears anywhere in a live PPPoE connection's response, so
	// this stays nil for that connection type by design. Revisit if a
	// DHCP-based WAN response surfaces the real field name.
	if lease, err := parseRequiredUint(params, "LeaseTimeRemain"); err != nil {
		slog.Debug("WAN lease field not present (expected for non-DHCP connections)", "field", "LeaseTimeRemain", "error", err)
	} else {
		status.LeaseRemainingSeconds = &lease
	}

	return status
}
