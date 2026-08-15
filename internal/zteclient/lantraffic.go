package zteclient

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
)

// LANPortTraffic represents the cumulative traffic counters for a single
// physical LAN port at the time of the scrape. BytesReceived/BytesSent
// are pointers so an unparseable or missing field on one port doesn't
// discard its otherwise-valid siblings (e.g. Port stays populated even
// when a byte counter can't be parsed).
type LANPortTraffic struct {
	// Port is the port label, preferring the router's AliasName (e.g.
	// "LAN1") and falling back to the raw _InstID when AliasName is
	// missing or empty.
	Port string

	// BytesReceived/BytesSent are the LAN port's cumulative traffic
	// counters (reset on router reboot/interface reset, monotonically
	// increasing otherwise), sourced from eth_lanstatus_lua.lua.
	BytesReceived *uint64
	BytesSent     *uint64
}

// lanTrafficScript is the menuData script that returns per-LAN-port
// traffic counters, confirmed against a live H3600P.
const lanTrafficScript = "eth_lanstatus_lua.lua"

// lanTrafficIDElement is the element wrapping the per-port Instances in
// eth_lanstatus_lua.lua's response, confirmed against a live H3600P.
// It happens to share the same literal value as wanstatus.go's
// ethInterfaceIDElement, but is named separately since the two scripts
// are unrelated and could diverge.
const lanTrafficIDElement = "OBJ_ETH_ID"

// GetLANTraffic fetches the router's per-LAN-port traffic counters, one
// entry per physical LAN port regardless of the port's current link
// status. It only returns an error when the page itself couldn't be
// fetched or parsed; a malformed individual field on a given port is
// logged and left nil on that port's entry rather than dropping the
// port or failing the whole fetch.
func (c *Client) GetLANTraffic(ctx context.Context) ([]LANPortTraffic, error) {
	body, err := c.fetchMenuData(ctx, localNetStatusTag, lanTrafficScript, "LAN traffic")
	if err != nil {
		return nil, err
	}

	ports, err := parseLANTraffic(body)
	if err != nil {
		return nil, err
	}

	slog.Debug("fetched LAN traffic", "count", len(ports))
	return ports, nil
}

// parseLANTraffic extracts the per-port traffic counters nested under
// the OBJ_ETH_ID section of eth_lanstatus_lua.lua's response.
func parseLANTraffic(body []byte) ([]LANPortTraffic, error) {
	var resp ajaxResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing LAN traffic XML: %w", err)
	}
	if err := resp.checkError(); err != nil {
		return nil, err
	}

	sections, err := findSections(body, lanTrafficIDElement)
	if err != nil {
		return nil, err
	}

	// seenPort guards against a malformed response repeating the same
	// port label (duplicate _InstID, or two ports sharing an
	// AliasName): emitting two LANPortTraffic entries with the same
	// Port would produce two Prometheus series with identical labels,
	// which the registry rejects at scrape time and fails the whole
	// /metrics response, not just this fetch's own metrics. A repeat is
	// logged and the later duplicate skipped instead.
	seenPort := make(map[string]bool)
	var ports []LANPortTraffic
	for _, section := range sections {
		for _, inst := range section.Instances {
			port := lanPortTrafficFromInstance(inst)
			if seenPort[port.Port] {
				slog.Warn("skipping duplicate LAN port instance", "port", port.Port)
				continue
			}
			seenPort[port.Port] = true
			ports = append(ports, port)
		}
	}
	return ports, nil
}

// lanPortTrafficFromInstance converts a single raw <Instance> entry into
// a LANPortTraffic, applying the AliasName/_InstID label fallback and
// degrading individual byte counters independently on parse failure.
func lanPortTrafficFromInstance(inst instance) LANPortTraffic {
	params := inst.params()

	port := params["AliasName"]
	if port == "" {
		port = params["_InstID"]
	}

	traffic := LANPortTraffic{Port: port}

	if received, err := parseRequiredUint(params, "BytesReceived"); err != nil {
		slog.Warn("parsing LAN traffic field failed", "port", port, "field", "BytesReceived", "error", err)
	} else {
		traffic.BytesReceived = &received
	}

	if sent, err := parseRequiredUint(params, "BytesSent"); err != nil {
		slog.Warn("parsing LAN traffic field failed", "port", port, "field", "BytesSent", "error", err)
	} else {
		traffic.BytesSent = &sent
	}

	return traffic
}
