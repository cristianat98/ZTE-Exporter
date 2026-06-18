package zteclient

import "testing"

const lanDevicesFixture = `<ajax_response_xml_root>
	<IF_ERRORPARAM>SUCC</IF_ERRORPARAM>
	<IF_ERRORTYPE>SUCC</IF_ERRORTYPE>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<IF_ERRORID>0</IF_ERRORID>
	<OBJ_ACCESSDEV_ID>
		<Instance>
			<ParaName>_InstID</ParaName>
			<ParaValue>1</ParaValue>
			<ParaName>MACAddress</ParaName>
			<ParaValue>AA:BB:CC:DD:EE:01</ParaValue>
			<ParaName>IPAddress</ParaName>
			<ParaValue>192.168.1.10</ParaValue>
			<ParaName>HostName</ParaName>
			<ParaValue>laptop</ParaValue>
			<ParaName>Active</ParaName>
			<ParaValue>1</ParaValue>
		</Instance>
		<Instance>
			<ParaName>_InstID</ParaName>
			<ParaValue>2</ParaValue>
			<ParaName>MACAddress</ParaName>
			<ParaValue>AA:BB:CC:DD:EE:02</ParaValue>
			<ParaName>IPAddress</ParaName>
			<ParaValue>192.168.1.11</ParaValue>
			<ParaName>HostName</ParaName>
			<ParaValue>printer</ParaValue>
			<ParaName>Active</ParaName>
			<ParaValue>0</ParaValue>
		</Instance>
	</OBJ_ACCESSDEV_ID>
</ajax_response_xml_root>`

const errorFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SessionTimeout</IF_ERRORSTR>
</ajax_response_xml_root>`

func TestParseDevices(t *testing.T) {
	devices, err := parseDevices([]byte(lanDevicesFixture), lanIDElement, "LAN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	first := devices[0]
	if first.MACAddress != "AA:BB:CC:DD:EE:01" {
		t.Errorf("unexpected MAC: %s", first.MACAddress)
	}
	if first.IPAddress != "192.168.1.10" {
		t.Errorf("unexpected IP: %s", first.IPAddress)
	}
	if first.HostName != "laptop" {
		t.Errorf("unexpected hostname: %s", first.HostName)
	}
	if !first.Active {
		t.Errorf("expected first device to be active")
	}
	if first.NetworkType != "LAN" {
		t.Errorf("unexpected network type: %s", first.NetworkType)
	}

	second := devices[1]
	if second.Active {
		t.Errorf("expected second device to be inactive")
	}
}

func TestParseDevicesSkipsEntriesWithoutMAC(t *testing.T) {
	const fixture = `<ajax_response_xml_root>
		<IF_ERRORSTR>SUCC</IF_ERRORSTR>
		<OBJ_ACCESSDEV_ID>
			<Instance>
				<ParaName>HostName</ParaName>
				<ParaValue>no-mac-device</ParaValue>
			</Instance>
		</OBJ_ACCESSDEV_ID>
	</ajax_response_xml_root>`

	devices, err := parseDevices([]byte(fixture), lanIDElement, "LAN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices, got %d", len(devices))
	}
}

func TestParseDevicesReturnsRouterError(t *testing.T) {
	_, err := parseDevices([]byte(errorFixture), lanIDElement, "LAN")
	if err == nil {
		t.Fatal("expected an error for a SessionTimeout response")
	}
}

func TestParseDevicesNoInstances(t *testing.T) {
	const fixture = `<ajax_response_xml_root>
		<IF_ERRORSTR>SUCC</IF_ERRORSTR>
		<OBJ_ACCESSDEV_ID></OBJ_ACCESSDEV_ID>
	</ajax_response_xml_root>`

	devices, err := parseDevices([]byte(fixture), lanIDElement, "LAN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices, got %d", len(devices))
	}
}
