package zteclient

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// DeviceInfo represents the router's static identity and firmware
// information at the time of the scrape. Each field is a pointer so a
// single unparseable or missing field doesn't discard its otherwise-
// valid siblings.
type DeviceInfo struct {
	Model           *string
	SoftwareVersion *string
	HardwareVersion *string
	SerialNumber    *string
	BootVersion     *string
	BuildDate       *string
}

// deviceInfoIDElement is the element wrapping the single Instance in
// devmgr_statusmgr_lua.lua's response, confirmed against a live H3600P.
const deviceInfoIDElement = "OBJ_DEVINFO_ID"

const deviceInfoScript = "devmgr_statusmgr_lua.lua"

// deviceInfoViewTag is the menuView tag for the router's device-info
// page, confirmed against a live H3600P. The priming GET also requires
// a Menu3Location=0 query parameter.
const deviceInfoViewTag = "statusMgr"

// buildDateLayout is the VerDate field's timestamp format
// (YYYYMMDDHHMMSS), confirmed against a live H3600P.
const buildDateLayout = "20060102150405"

// GetDeviceInfo fetches the router's model, firmware/hardware/boot
// versions, serial number, and firmware build date. It only returns an
// error when the page itself couldn't be fetched or parsed; a malformed
// individual field is logged and left nil on the returned DeviceInfo
// rather than failing the whole fetch.
func (c *Client) GetDeviceInfo(ctx context.Context) (*DeviceInfo, error) {
	if _, err := c.get(ctx, fmt.Sprintf("?_type=menuView&_tag=%s&Menu3Location=0&_=%d", deviceInfoViewTag, c.nextGUID())); err != nil {
		return nil, fmt.Errorf("setting up device info context: %w", err)
	}

	body, err := c.get(ctx, fmt.Sprintf("?_type=menuData&_tag=%s&_=%d", deviceInfoScript, c.nextGUID()))
	if err != nil {
		return nil, fmt.Errorf("fetching device info: %w", err)
	}

	params, err := parseSingleInstance(body, deviceInfoIDElement)
	if err != nil {
		return nil, err
	}

	info := deviceInfoFromParams(params)
	slog.Debug("fetched device info",
		"model_available", info.Model != nil,
		"software_version_available", info.SoftwareVersion != nil,
		"hardware_version_available", info.HardwareVersion != nil,
		"serial_number_available", info.SerialNumber != nil,
		"boot_version_available", info.BootVersion != nil,
		"build_date_available", info.BuildDate != nil,
	)
	return info, nil
}

func deviceInfoFromParams(params map[string]string) *DeviceInfo {
	info := &DeviceInfo{}
	info.Model = requiredString(params, "ModelName")
	info.SoftwareVersion = requiredString(params, "SoftwareVer")
	info.HardwareVersion = requiredString(params, "HardwareVer")
	info.SerialNumber = requiredString(params, "SerialNumber")
	info.BootVersion = requiredString(params, "BootVer")

	if raw, ok := params["VerDate"]; !ok {
		slog.Warn("parsing device info field failed", "field", "VerDate", "error", "missing")
	} else if t, err := time.Parse(buildDateLayout, raw); err != nil {
		slog.Warn("parsing device info field failed", "field", "VerDate", "error", err)
		info.BuildDate = &raw
	} else {
		formatted := t.Format(time.RFC3339)
		info.BuildDate = &formatted
	}

	return info
}

func requiredString(params map[string]string, key string) *string {
	raw, ok := params[key]
	if !ok {
		slog.Warn("parsing device info field failed", "field", key, "error", "missing")
		return nil
	}
	return &raw
}
