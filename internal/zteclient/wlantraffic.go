package zteclient

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
)

// WLANSSIDTraffic represents the cumulative traffic counters for a single
// WLAN SSID slot (AP) at the time of the scrape. BytesReceived/BytesSent/
// PacketsReceived/PacketsSent are pointers so an unparseable or missing
// field on one slot doesn't discard its otherwise-valid siblings.
type WLANSSIDTraffic struct {
	// APID is the stable AP slot id (e.g. "DEV.WIFI.AP1"), which anchors
	// the metric's identity across an ESSID rename in the router UI.
	APID string

	// ESSID and Band are joined in from OBJ_WLANAP_ID and
	// OBJ_WLANSETTING_ID respectively. A join miss on either leaves that
	// field as "" rather than dropping the entity.
	ESSID string
	Band  string

	// BytesReceived/BytesSent/PacketsReceived/PacketsSent are the SSID's
	// cumulative traffic counters (reset on router reboot/interface
	// reset, monotonically increasing otherwise), sourced from
	// wlan_status_lua.lua.
	BytesReceived   *uint64
	BytesSent       *uint64
	PacketsReceived *uint64
	PacketsSent     *uint64
}

// wlanTrafficScript is the menuData script that returns per-SSID WLAN
// traffic counters, confirmed against a live H3600P.
const wlanTrafficScript = "wlan_status_lua.lua"

// wlanConfigDrvIDElement wraps the per-SSID traffic Instances in
// wlan_status_lua.lua's response, confirmed against a live H3600P.
const wlanConfigDrvIDElement = "OBJ_WLANCONFIGDRV_ID"

// wlanAPIDElement wraps the per-SSID ESSID/Enable Instances in
// wlan_status_lua.lua's response, joined to wlanConfigDrvIDElement by
// _InstID.
const wlanAPIDElement = "OBJ_WLANAP_ID"

// wlanSettingIDElement wraps the per-radio Instances (one per band) in
// wlan_status_lua.lua's response, joined to wlanConfigDrvIDElement's
// WLANViewName.
const wlanSettingIDElement = "OBJ_WLANSETTING_ID"

// GetWLANTraffic fetches the router's per-SSID WLAN traffic counters, one
// entry per physical SSID slot regardless of the slot's Enable state. It
// only returns an error when the page itself couldn't be fetched or
// parsed; a malformed individual field, or a join miss on ESSID/Band, is
// logged and left as the zero value on that slot's entry rather than
// dropping the slot or failing the whole fetch.
func (c *Client) GetWLANTraffic(ctx context.Context) ([]WLANSSIDTraffic, error) {
	// Independently re-primed via fetchMenuData, the same way
	// GetWLANDevices does, rather than chaining off another fetch's
	// priming.
	body, err := c.fetchMenuData(ctx, localNetStatusTag, wlanTrafficScript, "WLAN traffic")
	if err != nil {
		return nil, err
	}

	traffic, err := parseWLANTraffic(body)
	if err != nil {
		return nil, err
	}

	slog.Debug("fetched WLAN traffic", "count", len(traffic))
	return traffic, nil
}

// parseWLANTraffic extracts the per-SSID traffic counters nested under
// wlanConfigDrvIDElement, joined to their ESSID (wlanAPIDElement, by
// _InstID) and band (wlanSettingIDElement, by WLANViewName).
func parseWLANTraffic(body []byte) ([]WLANSSIDTraffic, error) {
	var resp ajaxResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing WLAN traffic XML: %w", err)
	}
	if err := resp.checkError(); err != nil {
		return nil, err
	}

	sections, err := findMultiSections(body, wlanConfigDrvIDElement, wlanAPIDElement, wlanSettingIDElement)
	if err != nil {
		return nil, err
	}

	apByID := indexInstancesByInstID(sections[wlanAPIDElement])
	settingByID := indexInstancesByInstID(sections[wlanSettingIDElement])

	// seenAPID guards against a malformed response repeating the same
	// AP slot's _InstID: emitting two WLANSSIDTraffic entries with the
	// same APID would produce two Prometheus series with identical
	// labels, which the registry rejects at scrape time and fails the
	// whole /metrics response, not just this fetch's own metrics. A
	// repeat is logged and the later duplicate skipped instead.
	seenAPID := make(map[string]bool)
	var traffic []WLANSSIDTraffic
	for _, section := range sections[wlanConfigDrvIDElement] {
		for _, inst := range section.Instances {
			ssid := wlanSSIDTrafficFromInstance(inst, apByID, settingByID)
			if seenAPID[ssid.APID] {
				slog.Warn("skipping duplicate WLAN SSID instance", "ap_id", ssid.APID)
				continue
			}
			seenAPID[ssid.APID] = true
			traffic = append(traffic, ssid)
		}
	}
	return traffic, nil
}

// indexInstancesByInstID flattens sections' Instances into a map keyed by
// each instance's _InstID param, for the ESSID/band joins in
// wlanSSIDTrafficFromInstance. An instance without an _InstID is skipped.
func indexInstancesByInstID(sections []instanceContainer) map[string]map[string]string {
	index := make(map[string]map[string]string)
	for _, section := range sections {
		for _, inst := range section.Instances {
			params := inst.params()
			id := params["_InstID"]
			if id == "" {
				continue
			}
			index[id] = params
		}
	}
	return index
}

// wlanSSIDTrafficFromInstance converts a single raw OBJ_WLANCONFIGDRV_ID
// <Instance> entry into a WLANSSIDTraffic, joining in its ESSID (by
// _InstID against apByID) and band (by WLANViewName against
// settingByID). A join miss on either leaves that field as "";
// individual byte/packet counters degrade independently on parse
// failure.
func wlanSSIDTrafficFromInstance(inst instance, apByID, settingByID map[string]map[string]string) WLANSSIDTraffic {
	params := inst.params()
	apID := params["_InstID"]

	traffic := WLANSSIDTraffic{APID: apID}

	if apParams, ok := apByID[apID]; ok {
		traffic.ESSID = apParams["ESSID"]
	} else {
		slog.Warn("joining WLAN traffic to ESSID failed", "ap_id", apID, "error", "no matching OBJ_WLANAP_ID instance")
	}

	viewName := params["WLANViewName"]
	if settingParams, ok := settingByID[viewName]; ok {
		traffic.Band = settingParams["Band"]
	} else {
		slog.Warn("joining WLAN traffic to band failed", "ap_id", apID, "wlan_view_name", viewName, "error", "no matching OBJ_WLANSETTING_ID instance")
	}

	if received, err := parseRequiredUint(params, "TotalBytesReceived"); err != nil {
		slog.Warn("parsing WLAN traffic field failed", "ap_id", apID, "field", "TotalBytesReceived", "error", err)
	} else {
		traffic.BytesReceived = &received
	}

	if sent, err := parseRequiredUint(params, "TotalBytesSent"); err != nil {
		slog.Warn("parsing WLAN traffic field failed", "ap_id", apID, "field", "TotalBytesSent", "error", err)
	} else {
		traffic.BytesSent = &sent
	}

	if received, err := parseRequiredUint(params, "TotalPacketsReceived"); err != nil {
		slog.Warn("parsing WLAN traffic field failed", "ap_id", apID, "field", "TotalPacketsReceived", "error", err)
	} else {
		traffic.PacketsReceived = &received
	}

	if sent, err := parseRequiredUint(params, "TotalPacketsSent"); err != nil {
		slog.Warn("parsing WLAN traffic field failed", "ap_id", apID, "field", "TotalPacketsSent", "error", err)
	} else {
		traffic.PacketsSent = &sent
	}

	return traffic
}
