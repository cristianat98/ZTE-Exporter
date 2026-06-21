package zteclient

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
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
			params[name] = i.ParaValues[idx]
		}
	}
	return params
}

type instanceContainer struct {
	Instances []instance `xml:"Instance"`
}

// GetLANDevices fetches the list of devices currently connected to the
// router's LAN ports.
func (c *Client) GetLANDevices(ctx context.Context) ([]Device, error) {
	// First request sets up the menu context the router expects before
	// serving menuData, mirroring what the browser UI does.
	if _, err := c.get(ctx, fmt.Sprintf("?_type=menuView&_tag=localNetStatus&_=%d", c.nextGUID())); err != nil {
		return nil, fmt.Errorf("setting up LAN devices context: %w", err)
	}

	body, err := c.get(ctx, fmt.Sprintf("?_type=menuData&_tag=%s&_=%d", lanScript, c.nextGUID()))
	if err != nil {
		return nil, fmt.Errorf("fetching LAN devices: %w", err)
	}

	devices, err := parseDevices(body, lanIDElement, "LAN")
	if err != nil {
		return nil, err
	}

	slog.Debug("fetched LAN devices", "count", len(devices))
	return devices, nil
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

	// idElement's tag name is only known at runtime, so we can't bind it
	// to a static struct field tag; walk tokens instead and decode just
	// the section(s) matching it.
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var devices []Device
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
		for _, inst := range section.Instances {
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
	}

	return devices, nil
}
