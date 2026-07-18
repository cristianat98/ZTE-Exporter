package zteclient

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
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

	// wlanIDElement is assumed to mirror lanIDElement's naming
	// convention; not yet verified against a live router.
	wlanIDElement = "OBJ_SSIDDEV_ID"
	wlanScript    = "accessdev_ssiddev_lua.lua"
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
	return c.getDevices(ctx, wlanScript, wlanIDElement, "WLAN")
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

// flatResponse models a router XML response whose <ParaName>/<ParaValue>
// pairs are direct children of the root element, rather than nested under
// per-device <Instance> sections. Health and WAN status pages return a
// single flat record this way, unlike the device-list pages in this file.
type flatResponse struct {
	XMLName  xml.Name `xml:"ajax_response_xml_root"`
	ErrorStr string   `xml:"IF_ERRORSTR"`
	instance
}

// parseFlatParams decodes a flat router XML response into a name->value
// map, reusing instance's existing ParaName/ParaValue pairing logic.
func parseFlatParams(body []byte) (map[string]string, error) {
	var resp flatResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing response XML: %w", err)
	}
	if err := (ajaxResponse{ErrorStr: resp.ErrorStr}).checkError(); err != nil {
		return nil, err
	}
	return resp.instance.params(), nil
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
// runtime, so it can't be bound to a static struct field tag.
func findSections(body []byte, idElement string) ([]instanceContainer, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var sections []instanceContainer
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != idElement {
			continue
		}
		var section instanceContainer
		if err := decoder.DecodeElement(&section, &start); err != nil {
			return nil, fmt.Errorf("parsing %s section: %w", idElement, err)
		}
		sections = append(sections, section)
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
