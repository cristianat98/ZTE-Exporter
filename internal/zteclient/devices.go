package zteclient

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// Device represents a single device connected to the router, either via
// a LAN port or WiFi.
type Device struct {
	MACAddress  string
	IPAddress   string
	HostName    string
	Active      bool
	NetworkType string // "LAN" or "WLAN"
}

const (
	lanIDElement = "OBJ_ACCESSDEV_ID"
	lanScript    = "accessdev_landevs_lua.lua"

	// The WLAN device list script returns devices under the same
	// OBJ_ACCESSDEV_ID element as LAN (confirmed against a live
	// H3600P) rather than a WLAN-specific element name, so there is no
	// separate wlanIDElement.
	wlanScript = "accessdev_ssiddev_lua.lua"
)

// instance models a router XML <Instance> element, whose children are a
// flat, alternating sequence of <ParaName> and <ParaValue> elements
// (e.g. <ParaName>MACAddress</ParaName><ParaValue>aa:bb:..</ParaValue>).
// Since encoding/xml preserves document order within each repeated tag,
// ParaNames[i] and ParaValues[i] always refer to the same parameter.
type instance struct {
	ParaNames  []string `xml:"ParaName"`
	ParaValues []string `xml:"ParaValue"`
}

func (i instance) params() map[string]string {
	params := make(map[string]string, len(i.ParaNames))
	for idx, name := range i.ParaNames {
		if idx < len(i.ParaValues) {
			// Router XML is sometimes pretty-printed, so a value can
			// carry incidental leading/trailing whitespace; trim it the
			// same way getLoginToken already does for router text content.
			params[name] = strings.TrimSpace(i.ParaValues[idx])
		}
	}
	return params
}

type instanceContainer struct {
	Instances []instance `xml:"Instance"`
}

// localNetStatusTag is the menuView tag for the router's local-network
// status page, which serves both the LAN and WLAN device lists.
const localNetStatusTag = "localNetStatus"

// GetLANDevices fetches the list of devices currently connected to the
// router's LAN ports.
func (c *Client) GetLANDevices(ctx context.Context) ([]Device, error) {
	return c.getDevices(ctx, lanScript, lanIDElement, "LAN")
}

// GetWLANDevices fetches the list of devices currently connected to the
// router's WLAN (WiFi).
func (c *Client) GetWLANDevices(ctx context.Context) ([]Device, error) {
	return c.getDevices(ctx, wlanScript, lanIDElement, "WLAN")
}

// getDevices fetches and parses a device list from the local-network
// status page, shared by GetLANDevices and GetWLANDevices.
func (c *Client) getDevices(ctx context.Context, script, idElement, networkType string) ([]Device, error) {
	body, err := c.fetchMenuData(ctx, localNetStatusTag, script, networkType+" devices")
	if err != nil {
		return nil, err
	}

	devices, err := parseDevices(body, idElement, networkType)
	if err != nil {
		return nil, err
	}

	slog.Debug("fetched "+networkType+" devices", "count", len(devices))
	return devices, nil
}

// parseSingleInstance decodes a router XML response for a single-record
// page (WAN status, device info) into a name->value map. These pages
// nest one <Instance> under a page-specific element — the same document
// shape the device-list pages use, confirmed against a live H3600P for
// both WAN status and device info, but with exactly one Instance
// instead of many.
func parseSingleInstance(body []byte, idElement string) (map[string]string, error) {
	var resp ajaxResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing response XML: %w", err)
	}
	if err := resp.checkError(); err != nil {
		return nil, err
	}

	sections, err := findSections(body, idElement)
	if err != nil {
		return nil, err
	}
	for _, section := range sections {
		if len(section.Instances) > 0 {
			return section.Instances[0].params(), nil
		}
	}
	return nil, fmt.Errorf("no %s instance found in response", idElement)
}

// parseRequiredUint looks up key in params and parses it as a uint64,
// returning an error naming the field when it's missing or unparseable.
func parseRequiredUint(params map[string]string, key string) (uint64, error) {
	raw, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("missing %s in response", key)
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}
	return value, nil
}

// parseDevices extracts the devices nested under the <idElement> section
// of a router XML response (e.g. OBJ_ACCESSDEV_ID for LAN, OBJ_WLAN_AD_ID
// for WiFi), tagging each with networkType.
func parseDevices(body []byte, idElement, networkType string) ([]Device, error) {
	var resp ajaxResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing devices XML: %w", err)
	}
	if err := resp.checkError(); err != nil {
		return nil, err
	}

	sections, err := findSections(body, idElement)
	if err != nil {
		return nil, err
	}

	var devices []Device
	for _, section := range sections {
		devices = append(devices, devicesFromInstances(section.Instances, networkType)...)
	}
	return devices, nil
}

// findSections walks body's XML tokens and decodes every section whose
// element name matches idElement. idElement's tag name is only known at
// runtime, so it can't be bound to a static struct field tag. A thin
// wrapper around findMultiSections so single- and multi-element callers
// share one token-walking implementation.
func findSections(body []byte, idElement string) ([]instanceContainer, error) {
	sections, err := findMultiSections(body, idElement)
	if err != nil {
		return nil, err
	}
	return sections[idElement], nil
}

// findMultiSections walks body's XML tokens in a single decoder pass,
// decoding every section whose element name is one of idElements, keyed
// by that element name. A page needing several different sections joined
// together (e.g. wlan_status_lua.lua) calls this once instead of calling
// findSections once per element name, each re-walking the same body from
// scratch.
func findMultiSections(body []byte, idElements ...string) (map[string][]instanceContainer, error) {
	want := make(map[string]bool, len(idElements))
	for _, name := range idElements {
		want[name] = true
	}

	decoder := xml.NewDecoder(bytes.NewReader(body))
	sections := make(map[string][]instanceContainer, len(idElements))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok || !want[start.Name.Local] {
			continue
		}
		var section instanceContainer
		if err := decoder.DecodeElement(&section, &start); err != nil {
			return nil, fmt.Errorf("parsing %s section: %w", start.Name.Local, err)
		}
		sections[start.Name.Local] = append(sections[start.Name.Local], section)
	}
	return sections, nil
}

// devicesFromInstances converts a section's raw <Instance> entries into
// Devices, skipping any entry without a MAC address.
func devicesFromInstances(instances []instance, networkType string) []Device {
	var devices []Device
	for _, inst := range instances {
		params := inst.params()
		mac := params["MACAddress"]
		if mac == "" {
			continue
		}
		devices = append(devices, Device{
			MACAddress:  mac,
			IPAddress:   params["IPAddress"],
			HostName:    params["HostName"],
			Active:      params["Active"] == "" || params["Active"] == "1" || params["Active"] == "true",
			NetworkType: networkType,
		})
	}
	return devices
}
