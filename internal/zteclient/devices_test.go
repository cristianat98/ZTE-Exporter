package zteclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

// wlanDevicesFixture mirrors a live H3600P response: WLAN devices are
// returned under the same OBJ_ACCESSDEV_ID element as LAN, not a
// WLAN-specific element name.
const wlanDevicesFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<OBJ_ACCESSDEV_ID>
		<Instance>
			<ParaName>MACAddress</ParaName>
			<ParaValue>AA:BB:CC:DD:EE:03</ParaValue>
			<ParaName>IPAddress</ParaName>
			<ParaValue>192.168.1.20</ParaValue>
			<ParaName>HostName</ParaName>
			<ParaValue>phone</ParaValue>
			<ParaName>Active</ParaName>
			<ParaValue>1</ParaValue>
		</Instance>
	</OBJ_ACCESSDEV_ID>
</ajax_response_xml_root>`

const flatFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<ParaName>CPUUsage</ParaName>
	<ParaValue>12</ParaValue>
	<ParaName>Uptime</ParaName>
	<ParaValue>3600</ParaValue>
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

func TestParseDevicesInvalidXML(t *testing.T) {
	_, err := parseDevices([]byte("not xml"), lanIDElement, "LAN")
	if err == nil {
		t.Fatal("expected an error for invalid XML")
	}
}

func newTestClient(t *testing.T, mux *http.ServeMux) *Client {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := NewClient("placeholder", "admin", "secret", 5*time.Second)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	c.baseURL = srv.URL
	return c
}

func TestGetLANDevicesSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "menuView" && query.Get("_tag") == "localNetStatus":
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && query.Get("_tag") == lanScript:
			_, _ = w.Write([]byte(lanDevicesFixture))
		default:
			http.NotFound(w, r)
		}
	})
	c := newTestClient(t, mux)

	devices, err := c.GetLANDevices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
}

func TestGetLANDevicesMenuViewFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := newTestClient(t, mux)

	if _, err := c.GetLANDevices(context.Background()); err == nil {
		t.Fatal("expected an error when the menuView request fails")
	}
}

func TestGetLANDevicesMenuDataFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("_type") == "menuView" {
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := newTestClient(t, mux)

	if _, err := c.GetLANDevices(context.Background()); err == nil {
		t.Fatal("expected an error when the menuData request fails")
	}
}

func TestGetWLANDevicesSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "menuView" && query.Get("_tag") == "localNetStatus":
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && query.Get("_tag") == wlanScript:
			_, _ = w.Write([]byte(wlanDevicesFixture))
		default:
			http.NotFound(w, r)
		}
	})
	c := newTestClient(t, mux)

	devices, err := c.GetWLANDevices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].NetworkType != "WLAN" {
		t.Errorf("unexpected network type: %s", devices[0].NetworkType)
	}
}

func TestGetWLANDevicesMenuViewFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := newTestClient(t, mux)

	if _, err := c.GetWLANDevices(context.Background()); err == nil {
		t.Fatal("expected an error when the menuView request fails")
	}
}

func TestGetWLANDevicesMenuDataFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("_type") == "menuView" {
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := newTestClient(t, mux)

	if _, err := c.GetWLANDevices(context.Background()); err == nil {
		t.Fatal("expected an error when the menuData request fails")
	}
}

func TestParseFlatParams(t *testing.T) {
	params, err := parseFlatParams([]byte(flatFixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["CPUUsage"] != "12" {
		t.Errorf("unexpected CPUUsage: %s", params["CPUUsage"])
	}
	if params["Uptime"] != "3600" {
		t.Errorf("unexpected Uptime: %s", params["Uptime"])
	}
}

func TestParseFlatParamsReturnsRouterError(t *testing.T) {
	_, err := parseFlatParams([]byte(errorFixture))
	if err == nil {
		t.Fatal("expected an error for a SessionTimeout response")
	}
}

func TestParseFlatParamsInvalidXML(t *testing.T) {
	_, err := parseFlatParams([]byte("not xml"))
	if err == nil {
		t.Fatal("expected an error for invalid XML")
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
