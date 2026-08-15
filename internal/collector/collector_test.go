package collector

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/cristianat98/zte-exporter/internal/config"
)

const (
	collectorTestPassword      = "correct-password"
	collectorTestLoginTok      = "deadbeef"
	collectorLANDevsScript     = "accessdev_landevs_lua.lua"
	collectorWLANDevsScript    = "accessdev_ssiddev_lua.lua"
	collectorDeviceInfoScript  = "devmgr_statusmgr_lua.lua"
	collectorEthIfaceScript    = "eth_interface_status_lua.lua"
	collectorWANScript         = "wan_internet_lua.lua"
	collectorLANTrafficScript  = "eth_lanstatus_lua.lua"
	collectorWLANTrafficScript = "wlan_status_lua.lua"
)

// collectorDeviceInfoFixture mirrors a live H3600P
// devmgr_statusmgr_lua.lua response (router identity/firmware info).
const collectorDeviceInfoFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<OBJ_DEVINFO_ID>
		<Instance>
			<ParaName>ModelName</ParaName>
			<ParaValue>H3600P V9.0</ParaValue>
			<ParaName>SoftwareVer</ParaName>
			<ParaValue>V9.0.0P5_DIGI</ParaValue>
			<ParaName>HardwareVer</ParaName>
			<ParaValue>V9.0.0</ParaValue>
			<ParaName>SerialNumber</ParaName>
			<ParaValue>ZTEYH93R8J08158</ParaValue>
			<ParaName>BootVer</ParaName>
			<ParaValue>V1.0.0</ParaValue>
			<ParaName>VerDate</ParaName>
			<ParaValue>20240329222419</ParaValue>
		</Instance>
	</OBJ_DEVINFO_ID>
</ajax_response_xml_root>`

// collectorWANFixture includes LeaseTimeRemain to exercise the (currently
// unconfirmed) DHCP-lease path; a live PPPoE connection won't have it.
const collectorWANFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<ID_WAN_COMFIG>
		<Instance>
			<ParaName>ConnStatus</ParaName>
			<ParaValue>Connected</ParaValue>
			<ParaName>UpTime</ParaName>
			<ParaValue>2000</ParaValue>
			<ParaName>LeaseTimeRemain</ParaName>
			<ParaValue>3000</ParaValue>
		</Instance>
	</ID_WAN_COMFIG>
</ajax_response_xml_root>`

// collectorDeviceInfoPartialFixture has only ModelName populated,
// exercising per-field degradation (empty label values, not a missing
// metric) within an otherwise-successful device info fetch.
const collectorDeviceInfoPartialFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<OBJ_DEVINFO_ID>
		<Instance>
			<ParaName>ModelName</ParaName>
			<ParaValue>H3600P V9.0</ParaValue>
		</Instance>
	</OBJ_DEVINFO_ID>
</ajax_response_xml_root>`

// collectorWANMissingLeaseFixture mirrors a live PPPoE connection: a
// valid ConnStatus/UpTime but no LeaseTimeRemain field at all, exercising
// per-field degradation within an otherwise-successful WAN status fetch.
const collectorWANMissingLeaseFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<ID_WAN_COMFIG>
		<Instance>
			<ParaName>ConnStatus</ParaName>
			<ParaValue>Connected</ParaValue>
			<ParaName>UpTime</ParaName>
			<ParaValue>2000</ParaValue>
		</Instance>
	</ID_WAN_COMFIG>
</ajax_response_xml_root>`

// collectorEthIfaceFixture mirrors a live H3600P
// eth_interface_status_lua.lua response, carrying the WAN interface's
// byte counters.
const collectorEthIfaceFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<OBJ_ETH_ID>
		<Instance>
			<ParaName>BytesReceived</ParaName>
			<ParaValue>500</ParaValue>
			<ParaName>BytesSent</ParaName>
			<ParaValue>200</ParaValue>
		</Instance>
	</OBJ_ETH_ID>
</ajax_response_xml_root>`

// collectorWLANFixture mirrors a live H3600P response: WLAN devices are
// returned under the same OBJ_ACCESSDEV_ID element as LAN.
const collectorWLANFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<OBJ_ACCESSDEV_ID>
		<Instance>
			<ParaName>MACAddress</ParaName>
			<ParaValue>AA:BB:CC:DD:EE:02</ParaValue>
		</Instance>
	</OBJ_ACCESSDEV_ID>
</ajax_response_xml_root>`

// collectorLANTrafficFixture is a live eth_lanstatus_lua.lua response
// captured from an H3600P: 2 physical LAN ports (LAN1 up, LAN2 no link).
const collectorLANTrafficFixture = `<ajax_response_xml_root><IF_ERRORPARAM>SUCC</IF_ERRORPARAM><IF_ERRORTYPE>SUCC</IF_ERRORTYPE><IF_ERRORSTR>SUCC</IF_ERRORSTR><IF_ERRORID>0</IF_ERRORID><OBJ_ETH_ID><Instance><ParaName>_InstID</ParaName><ParaValue>IGD.LD1.ETH1</ParaValue><ParaName>AliasName</ParaName><ParaValue>LAN1</ParaValue><ParaName>LinkSpeed</ParaName><ParaValue>1000</ParaValue><ParaName>LinkDuplex</ParaName><ParaValue>Full</ParaValue><ParaName>Status</ParaName><ParaValue>Up</ParaValue><ParaName>BytesReceived</ParaName><ParaValue>13036552880</ParaValue><ParaName>MACAddress</ParaName><ParaValue>f4:fc:49:72:b8:10</ParaValue><ParaName>BytesSent</ParaName><ParaValue>124765590890</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>IGD.LD1.ETH2</ParaValue><ParaName>AliasName</ParaName><ParaValue>LAN2</ParaValue><ParaName>LinkSpeed</ParaName><ParaValue>10</ParaValue><ParaName>LinkDuplex</ParaName><ParaValue>Half</ParaValue><ParaName>Status</ParaName><ParaValue>NoLink</ParaValue><ParaName>BytesReceived</ParaName><ParaValue>0</ParaValue><ParaName>MACAddress</ParaName><ParaValue>f4:fc:49:72:b8:10</ParaValue><ParaName>BytesSent</ParaName><ParaValue>0</ParaValue></Instance></OBJ_ETH_ID></ajax_response_xml_root>`

// collectorWLANTrafficFixture is a minimal 2-slot wlan_status_lua.lua
// fixture: both SSID slots enabled, one per radio/band, enough to exercise
// the collector-level wiring (exhaustive parsing coverage lives in
// wlantraffic_test.go).
const collectorWLANTrafficFixture = `<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR><OBJ_WLANAP_ID><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.AP1</ParaValue><ParaName>Enable</ParaName><ParaValue>1</ParaValue><ParaName>ESSID</ParaName><ParaValue>MySSID-24</ParaValue><ParaName>WLANViewName</ParaName><ParaValue>DEV.WIFI.RD1</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.AP5</ParaValue><ParaName>Enable</ParaName><ParaValue>1</ParaValue><ParaName>ESSID</ParaName><ParaValue>MySSID-5G</ParaValue><ParaName>WLANViewName</ParaName><ParaValue>DEV.WIFI.RD2</ParaValue></Instance></OBJ_WLANAP_ID><OBJ_WLANCONFIGDRV_ID><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.AP1</ParaValue><ParaName>WLANViewName</ParaName><ParaValue>DEV.WIFI.RD1</ParaValue><ParaName>TotalBytesReceived</ParaName><ParaValue>1000</ParaValue><ParaName>TotalBytesSent</ParaName><ParaValue>2000</ParaValue><ParaName>TotalPacketsReceived</ParaName><ParaValue>10</ParaValue><ParaName>TotalPacketsSent</ParaName><ParaValue>20</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.AP5</ParaValue><ParaName>WLANViewName</ParaName><ParaValue>DEV.WIFI.RD2</ParaValue><ParaName>TotalBytesReceived</ParaName><ParaValue>3000</ParaValue><ParaName>TotalBytesSent</ParaName><ParaValue>4000</ParaValue><ParaName>TotalPacketsReceived</ParaName><ParaValue>30</ParaValue><ParaName>TotalPacketsSent</ParaName><ParaValue>40</ParaValue></Instance></OBJ_WLANCONFIGDRV_ID><OBJ_WLANSETTING_ID><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.RD1</ParaValue><ParaName>Band</ParaName><ParaValue>2.4GHz</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.RD2</ParaValue><ParaName>Band</ParaName><ParaValue>5GHz</ParaValue></Instance></OBJ_WLANSETTING_ID></ajax_response_xml_root>`

// newFakeRouter builds a router that succeeds at login and every data
// fetch, except the menuData tags listed in failTags, which return a
// 500. failTags may be nil to make everything succeed.
func newFakeRouter(t *testing.T, password string, failTags map[string]bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		tag := query.Get("_tag")

		switch {
		case query.Get("_type") == "loginData" && tag == "login_entry" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"sess_token":"abc123","lockingTime":0}`))
		case query.Get("_type") == "loginData" && tag == "login_token":
			_, _ = w.Write([]byte(`<ajax_response_xml_root>` + collectorTestLoginTok + `</ajax_response_xml_root>`))
		case query.Get("_type") == "loginData" && tag == "login_entry" && r.Method == http.MethodPost:
			buf, _ := url.ParseQuery(readBody(t, r))
			hash := sha256.Sum256([]byte(password + collectorTestLoginTok))
			expected := hex.EncodeToString(hash[:])
			if buf.Get("Password") == expected {
				_, _ = w.Write([]byte(`{"login_need_refresh":0,"lockingTime":0,"loginErrMsg":""}`))
			} else {
				_, _ = w.Write([]byte(`{"login_need_refresh":0,"lockingTime":0,"loginErrMsg":"Wrong username or password"}`))
			}
		case query.Get("_type") == "menuView":
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && failTags[tag]:
			http.Error(w, "boom", http.StatusInternalServerError)
		case query.Get("_type") == "menuData" && tag == collectorLANDevsScript:
			_, _ = w.Write([]byte(`<ajax_response_xml_root>
				<IF_ERRORSTR>SUCC</IF_ERRORSTR>
				<OBJ_ACCESSDEV_ID>
					<Instance>
						<ParaName>MACAddress</ParaName>
						<ParaValue>AA:BB:CC:DD:EE:01</ParaValue>
					</Instance>
				</OBJ_ACCESSDEV_ID>
			</ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && tag == collectorWLANDevsScript:
			_, _ = w.Write([]byte(collectorWLANFixture))
		case query.Get("_type") == "menuData" && tag == collectorDeviceInfoScript:
			_, _ = w.Write([]byte(collectorDeviceInfoFixture))
		case query.Get("_type") == "menuData" && tag == collectorEthIfaceScript:
			_, _ = w.Write([]byte(collectorEthIfaceFixture))
		case query.Get("_type") == "menuData" && tag == collectorWANScript:
			_, _ = w.Write([]byte(collectorWANFixture))
		case query.Get("_type") == "menuData" && tag == collectorLANTrafficScript:
			_, _ = w.Write([]byte(collectorLANTrafficFixture))
		case query.Get("_type") == "menuData" && tag == collectorWLANTrafficScript:
			_, _ = w.Write([]byte(collectorWLANTrafficFixture))
		default:
			http.NotFound(w, r)
		}
	})
	return httptest.NewTLSServer(mux)
}

func readBody(t *testing.T, r *http.Request) string {
	t.Helper()
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading request body: %v", err)
	}
	return string(buf)
}

func cfgForServer(srv *httptest.Server, password string) *config.Config {
	return &config.Config{
		Host:          strings.TrimPrefix(srv.URL, "https://"),
		Username:      "admin",
		Password:      password,
		ScrapeTimeout: 5 * time.Second,
	}
}

// fqNameMarker matches a Desc's exact fqName field, avoiding false
// positives from substrings that happen to appear in another metric's
// help text (e.g. "zte_wan_uptime_seconds"'s help text mentions
// "zte_uptime_seconds").
func fqNameMarker(name string) string {
	return `fqName: "` + name + `"`
}

func gaugeValue(t *testing.T, metrics []prometheus.Metric, name string) float64 {
	t.Helper()
	marker := fqNameMarker(name)
	for _, m := range metrics {
		if !strings.Contains(m.Desc().String(), marker) {
			continue
		}
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("writing metric: %v", err)
		}
		return pb.GetGauge().GetValue()
	}
	t.Fatalf("metric %s not collected", name)
	return 0
}

func counterValue(t *testing.T, metrics []prometheus.Metric, name string) float64 {
	t.Helper()
	marker := fqNameMarker(name)
	for _, m := range metrics {
		if !strings.Contains(m.Desc().String(), marker) {
			continue
		}
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("writing metric: %v", err)
		}
		return pb.GetCounter().GetValue()
	}
	t.Fatalf("metric %s not collected", name)
	return 0
}

func labelValue(t *testing.T, metrics []prometheus.Metric, metricName, labelName string) string {
	t.Helper()
	marker := fqNameMarker(metricName)
	for _, m := range metrics {
		if !strings.Contains(m.Desc().String(), marker) {
			continue
		}
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("writing metric: %v", err)
		}
		for _, l := range pb.GetLabel() {
			if l.GetName() == labelName {
				return l.GetValue()
			}
		}
		t.Fatalf("label %s not found on metric %s", labelName, metricName)
	}
	t.Fatalf("metric %s not collected", metricName)
	return ""
}

func hasMetric(metrics []prometheus.Metric, name string) bool {
	marker := fqNameMarker(name)
	for _, m := range metrics {
		if strings.Contains(m.Desc().String(), marker) {
			return true
		}
	}
	return false
}

// findMetric returns the single collected metric with the given fqName
// whose labels match every entry in wantLabels (a subset match, so
// callers only need to specify the labels that disambiguate the series
// they want among several sharing the same metric name).
func findMetric(t *testing.T, metrics []prometheus.Metric, name string, wantLabels map[string]string) *dto.Metric {
	t.Helper()
	marker := fqNameMarker(name)
	for _, m := range metrics {
		if !strings.Contains(m.Desc().String(), marker) {
			continue
		}
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("writing metric: %v", err)
		}
		match := true
		for k, v := range wantLabels {
			found := false
			for _, l := range pb.GetLabel() {
				if l.GetName() == k && l.GetValue() == v {
					found = true
					break
				}
			}
			if !found {
				match = false
				break
			}
		}
		if match {
			return &pb
		}
	}
	t.Fatalf("metric %s with labels %v not collected", name, wantLabels)
	return nil
}

func collectMetrics(c *Collector) []prometheus.Metric {
	ch := make(chan prometheus.Metric, 64)
	c.Collect(ch)
	close(ch)

	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}
	return metrics
}

func TestCollectSuccess(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, nil)
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 1 {
		t.Errorf("expected zte_up=1, got %v", up)
	}
	if lan := gaugeValue(t, metrics, "zte_lan_connected_devices"); lan != 1 {
		t.Errorf("expected zte_lan_connected_devices=1, got %v", lan)
	}
	if wlan := gaugeValue(t, metrics, "zte_wlan_connected_devices"); wlan != 1 {
		t.Errorf("expected zte_wlan_connected_devices=1, got %v", wlan)
	}
	if info := gaugeValue(t, metrics, "zte_router_info"); info != 1 {
		t.Errorf("expected zte_router_info=1, got %v", info)
	}
	if model := labelValue(t, metrics, "zte_router_info", "model"); model != "H3600P V9.0" {
		t.Errorf("expected model=H3600P V9.0, got %v", model)
	}
	if sw := labelValue(t, metrics, "zte_router_info", "software_version"); sw != "V9.0.0P5_DIGI" {
		t.Errorf("expected software_version=V9.0.0P5_DIGI, got %v", sw)
	}
	if serial := labelValue(t, metrics, "zte_router_info", "serial_number"); serial != "ZTEYH93R8J08158" {
		t.Errorf("expected serial_number=ZTEYH93R8J08158, got %v", serial)
	}
	if wanConnected := gaugeValue(t, metrics, "zte_wan_connected"); wanConnected != 1 {
		t.Errorf("expected zte_wan_connected=1, got %v", wanConnected)
	}
	if wanUptime := gaugeValue(t, metrics, "zte_wan_uptime_seconds"); wanUptime != 2000 {
		t.Errorf("expected zte_wan_uptime_seconds=2000, got %v", wanUptime)
	}
	if lease := gaugeValue(t, metrics, "zte_wan_lease_remaining_seconds"); lease != 3000 {
		t.Errorf("expected zte_wan_lease_remaining_seconds=3000, got %v", lease)
	}
	if received := counterValue(t, metrics, "zte_wan_received_bytes_total"); received != 500 {
		t.Errorf("expected zte_wan_received_bytes_total=500, got %v", received)
	}
	if sent := counterValue(t, metrics, "zte_wan_sent_bytes_total"); sent != 200 {
		t.Errorf("expected zte_wan_sent_bytes_total=200, got %v", sent)
	}

	lan1Received := findMetric(t, metrics, "zte_lan_received_bytes_total", map[string]string{"port": "LAN1"})
	if v := lan1Received.GetCounter().GetValue(); v != 13036552880 {
		t.Errorf("expected LAN1 zte_lan_received_bytes_total=13036552880, got %v", v)
	}
	lan1Sent := findMetric(t, metrics, "zte_lan_sent_bytes_total", map[string]string{"port": "LAN1"})
	if v := lan1Sent.GetCounter().GetValue(); v != 124765590890 {
		t.Errorf("expected LAN1 zte_lan_sent_bytes_total=124765590890, got %v", v)
	}
	lan2Received := findMetric(t, metrics, "zte_lan_received_bytes_total", map[string]string{"port": "LAN2"})
	if v := lan2Received.GetCounter().GetValue(); v != 0 {
		t.Errorf("expected LAN2 zte_lan_received_bytes_total=0, got %v", v)
	}

	ap1Received := findMetric(t, metrics, "zte_wlan_received_bytes_total", map[string]string{"ap": "DEV.WIFI.AP1", "essid": "MySSID-24", "band": "2.4GHz"})
	if v := ap1Received.GetCounter().GetValue(); v != 1000 {
		t.Errorf("expected AP1 zte_wlan_received_bytes_total=1000, got %v", v)
	}
	ap1Sent := findMetric(t, metrics, "zte_wlan_sent_bytes_total", map[string]string{"ap": "DEV.WIFI.AP1", "essid": "MySSID-24", "band": "2.4GHz"})
	if v := ap1Sent.GetCounter().GetValue(); v != 2000 {
		t.Errorf("expected AP1 zte_wlan_sent_bytes_total=2000, got %v", v)
	}
	ap1PktsReceived := findMetric(t, metrics, "zte_wlan_received_packets_total", map[string]string{"ap": "DEV.WIFI.AP1", "essid": "MySSID-24", "band": "2.4GHz"})
	if v := ap1PktsReceived.GetCounter().GetValue(); v != 10 {
		t.Errorf("expected AP1 zte_wlan_received_packets_total=10, got %v", v)
	}
	ap1PktsSent := findMetric(t, metrics, "zte_wlan_sent_packets_total", map[string]string{"ap": "DEV.WIFI.AP1", "essid": "MySSID-24", "band": "2.4GHz"})
	if v := ap1PktsSent.GetCounter().GetValue(); v != 20 {
		t.Errorf("expected AP1 zte_wlan_sent_packets_total=20, got %v", v)
	}
	ap5Received := findMetric(t, metrics, "zte_wlan_received_bytes_total", map[string]string{"ap": "DEV.WIFI.AP5", "essid": "MySSID-5G", "band": "5GHz"})
	if v := ap5Received.GetCounter().GetValue(); v != 3000 {
		t.Errorf("expected AP5 zte_wlan_received_bytes_total=3000, got %v", v)
	}
}

func TestCollectLANTrafficFailureDegradesIndependently(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, map[string]bool{collectorLANTrafficScript: true})
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 1 {
		t.Errorf("expected zte_up=1, got %v", up)
	}
	if hasMetric(metrics, "zte_lan_received_bytes_total") {
		t.Error("expected zte_lan_received_bytes_total to be omitted")
	}
	if hasMetric(metrics, "zte_lan_sent_bytes_total") {
		t.Error("expected zte_lan_sent_bytes_total to be omitted")
	}
	if !hasMetric(metrics, "zte_wlan_received_bytes_total") {
		t.Error("expected zte_wlan_received_bytes_total to still be present")
	}
	if lan := gaugeValue(t, metrics, "zte_lan_connected_devices"); lan != 1 {
		t.Errorf("expected zte_lan_connected_devices=1 (unaffected), got %v", lan)
	}
	if wlan := gaugeValue(t, metrics, "zte_wlan_connected_devices"); wlan != 1 {
		t.Errorf("expected zte_wlan_connected_devices=1 (unaffected), got %v", wlan)
	}
	if info := gaugeValue(t, metrics, "zte_router_info"); info != 1 {
		t.Errorf("expected zte_router_info=1 (unaffected), got %v", info)
	}
	if wanConnected := gaugeValue(t, metrics, "zte_wan_connected"); wanConnected != 1 {
		t.Errorf("expected zte_wan_connected=1 (unaffected), got %v", wanConnected)
	}
}

func TestCollectWLANTrafficFailureDegradesIndependently(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, map[string]bool{collectorWLANTrafficScript: true})
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 1 {
		t.Errorf("expected zte_up=1, got %v", up)
	}
	if hasMetric(metrics, "zte_wlan_received_bytes_total") {
		t.Error("expected zte_wlan_received_bytes_total to be omitted")
	}
	if hasMetric(metrics, "zte_wlan_sent_bytes_total") {
		t.Error("expected zte_wlan_sent_bytes_total to be omitted")
	}
	if hasMetric(metrics, "zte_wlan_received_packets_total") {
		t.Error("expected zte_wlan_received_packets_total to be omitted")
	}
	if hasMetric(metrics, "zte_wlan_sent_packets_total") {
		t.Error("expected zte_wlan_sent_packets_total to be omitted")
	}
	if !hasMetric(metrics, "zte_lan_received_bytes_total") {
		t.Error("expected zte_lan_received_bytes_total to still be present")
	}
	if lan := gaugeValue(t, metrics, "zte_lan_connected_devices"); lan != 1 {
		t.Errorf("expected zte_lan_connected_devices=1 (unaffected), got %v", lan)
	}
	if wlan := gaugeValue(t, metrics, "zte_wlan_connected_devices"); wlan != 1 {
		t.Errorf("expected zte_wlan_connected_devices=1 (unaffected), got %v", wlan)
	}
	if info := gaugeValue(t, metrics, "zte_router_info"); info != 1 {
		t.Errorf("expected zte_router_info=1 (unaffected), got %v", info)
	}
	if wanConnected := gaugeValue(t, metrics, "zte_wan_connected"); wanConnected != 1 {
		t.Errorf("expected zte_wan_connected=1 (unaffected), got %v", wanConnected)
	}
}

func TestCollectLANAndWLANTrafficBothFailDegradeIndependently(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, map[string]bool{
		collectorLANTrafficScript:  true,
		collectorWLANTrafficScript: true,
	})
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 1 {
		t.Errorf("expected zte_up=1, got %v", up)
	}
	if hasMetric(metrics, "zte_lan_received_bytes_total") {
		t.Error("expected zte_lan_received_bytes_total to be omitted")
	}
	if hasMetric(metrics, "zte_lan_sent_bytes_total") {
		t.Error("expected zte_lan_sent_bytes_total to be omitted")
	}
	if hasMetric(metrics, "zte_wlan_received_bytes_total") {
		t.Error("expected zte_wlan_received_bytes_total to be omitted")
	}
	if hasMetric(metrics, "zte_wlan_sent_bytes_total") {
		t.Error("expected zte_wlan_sent_bytes_total to be omitted")
	}
	if hasMetric(metrics, "zte_wlan_received_packets_total") {
		t.Error("expected zte_wlan_received_packets_total to be omitted")
	}
	if hasMetric(metrics, "zte_wlan_sent_packets_total") {
		t.Error("expected zte_wlan_sent_packets_total to be omitted")
	}
	if lan := gaugeValue(t, metrics, "zte_lan_connected_devices"); lan != 1 {
		t.Errorf("expected zte_lan_connected_devices=1 (unaffected), got %v", lan)
	}
	if wlan := gaugeValue(t, metrics, "zte_wlan_connected_devices"); wlan != 1 {
		t.Errorf("expected zte_wlan_connected_devices=1 (unaffected), got %v", wlan)
	}
	if info := gaugeValue(t, metrics, "zte_router_info"); info != 1 {
		t.Errorf("expected zte_router_info=1 (unaffected), got %v", info)
	}
	if wanConnected := gaugeValue(t, metrics, "zte_wan_connected"); wanConnected != 1 {
		t.Errorf("expected zte_wan_connected=1 (unaffected), got %v", wanConnected)
	}
}

// TestCollectLANTrafficPortLabelFallback exercises the client-layer
// AliasName/_InstID label fallback (KTD3) end-to-end through the
// collector: a LAN port fixture missing AliasName must still produce a
// series, labeled with the raw _InstID.
func TestCollectLANTrafficPortLabelFallback(t *testing.T) {
	const fixture = `<ajax_response_xml_root>
		<IF_ERRORSTR>SUCC</IF_ERRORSTR>
		<OBJ_ETH_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>IGD.LD1.ETH1</ParaValue>
				<ParaName>BytesReceived</ParaName>
				<ParaValue>100</ParaValue>
				<ParaName>BytesSent</ParaName>
				<ParaValue>200</ParaValue>
			</Instance>
		</OBJ_ETH_ID>
	</ajax_response_xml_root>`

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		tag := query.Get("_tag")
		switch {
		case query.Get("_type") == "loginData" && tag == "login_entry" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"sess_token":"abc123","lockingTime":0}`))
		case query.Get("_type") == "loginData" && tag == "login_token":
			_, _ = w.Write([]byte(`<ajax_response_xml_root>` + collectorTestLoginTok + `</ajax_response_xml_root>`))
		case query.Get("_type") == "loginData" && tag == "login_entry" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"login_need_refresh":0,"lockingTime":0,"loginErrMsg":""}`))
		case query.Get("_type") == "menuView":
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && tag == collectorLANTrafficScript:
			_, _ = w.Write([]byte(fixture))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	received := findMetric(t, metrics, "zte_lan_received_bytes_total", map[string]string{"port": "IGD.LD1.ETH1"})
	if v := received.GetCounter().GetValue(); v != 100 {
		t.Errorf("expected fallback-labeled port received=100, got %v", v)
	}
}

func TestCollectScrapeFailure(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, nil)
	defer srv.Close()

	c := New(cfgForServer(srv, "wrong-password"))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 0 {
		t.Errorf("expected zte_up=0 on scrape failure, got %v", up)
	}
	if len(metrics) != 1 {
		t.Errorf("expected only zte_up to be collected on login failure, got %d metrics", len(metrics))
	}
}

func TestCollectLANFailureDegradesIndependently(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, map[string]bool{collectorLANDevsScript: true})
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 1 {
		t.Errorf("expected zte_up=1, got %v", up)
	}
	if hasMetric(metrics, "zte_lan_connected_devices") {
		t.Error("expected zte_lan_connected_devices to be omitted")
	}
	if wlan := gaugeValue(t, metrics, "zte_wlan_connected_devices"); wlan != 1 {
		t.Errorf("expected zte_wlan_connected_devices=1, got %v", wlan)
	}
}

func TestCollectWLANFailureDegradesIndependently(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, map[string]bool{collectorWLANDevsScript: true})
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 1 {
		t.Errorf("expected zte_up=1, got %v", up)
	}
	if lan := gaugeValue(t, metrics, "zte_lan_connected_devices"); lan != 1 {
		t.Errorf("expected zte_lan_connected_devices=1, got %v", lan)
	}
	if hasMetric(metrics, "zte_wlan_connected_devices") {
		t.Error("expected zte_wlan_connected_devices to be omitted")
	}
}

func TestCollectDeviceInfoFailureDegradesIndependently(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, map[string]bool{collectorDeviceInfoScript: true})
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 1 {
		t.Errorf("expected zte_up=1, got %v", up)
	}
	if hasMetric(metrics, "zte_router_info") {
		t.Error("expected zte_router_info to be omitted")
	}
	if wanConnected := gaugeValue(t, metrics, "zte_wan_connected"); wanConnected != 1 {
		t.Errorf("expected zte_wan_connected=1, got %v", wanConnected)
	}
}

func TestCollectWANFailureDegradesIndependently(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, map[string]bool{collectorWANScript: true})
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 1 {
		t.Errorf("expected zte_up=1, got %v", up)
	}
	if hasMetric(metrics, "zte_wan_connected") {
		t.Error("expected zte_wan_connected to be omitted")
	}
	if info := gaugeValue(t, metrics, "zte_router_info"); info != 1 {
		t.Errorf("expected zte_router_info=1, got %v", info)
	}
}

func TestCollectDeviceInfoPartialFieldDegradesIndependently(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		tag := query.Get("_tag")
		switch {
		case query.Get("_type") == "loginData" && tag == "login_entry" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"sess_token":"abc123","lockingTime":0}`))
		case query.Get("_type") == "loginData" && tag == "login_token":
			_, _ = w.Write([]byte(`<ajax_response_xml_root>` + collectorTestLoginTok + `</ajax_response_xml_root>`))
		case query.Get("_type") == "loginData" && tag == "login_entry" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"login_need_refresh":0,"lockingTime":0,"loginErrMsg":""}`))
		case query.Get("_type") == "menuView":
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && tag == collectorDeviceInfoScript:
			_, _ = w.Write([]byte(collectorDeviceInfoPartialFixture))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if info := gaugeValue(t, metrics, "zte_router_info"); info != 1 {
		t.Errorf("expected zte_router_info=1, got %v", info)
	}
	if model := labelValue(t, metrics, "zte_router_info", "model"); model != "H3600P V9.0" {
		t.Errorf("expected model=H3600P V9.0, got %v", model)
	}
	if sw := labelValue(t, metrics, "zte_router_info", "software_version"); sw != "" {
		t.Errorf("expected software_version to be empty (field missing), got %v", sw)
	}
}

func TestCollectWANPartialFieldFailureDegradesIndependently(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		tag := query.Get("_tag")
		switch {
		case query.Get("_type") == "loginData" && tag == "login_entry" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"sess_token":"abc123","lockingTime":0}`))
		case query.Get("_type") == "loginData" && tag == "login_token":
			_, _ = w.Write([]byte(`<ajax_response_xml_root>` + collectorTestLoginTok + `</ajax_response_xml_root>`))
		case query.Get("_type") == "loginData" && tag == "login_entry" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"login_need_refresh":0,"lockingTime":0,"loginErrMsg":""}`))
		case query.Get("_type") == "menuView":
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && tag == collectorEthIfaceScript:
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && tag == collectorWANScript:
			_, _ = w.Write([]byte(collectorWANMissingLeaseFixture))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if wanConnected := gaugeValue(t, metrics, "zte_wan_connected"); wanConnected != 1 {
		t.Errorf("expected zte_wan_connected=1, got %v", wanConnected)
	}
	if wanUptime := gaugeValue(t, metrics, "zte_wan_uptime_seconds"); wanUptime != 2000 {
		t.Errorf("expected zte_wan_uptime_seconds=2000, got %v", wanUptime)
	}
	if hasMetric(metrics, "zte_wan_lease_remaining_seconds") {
		t.Error("expected zte_wan_lease_remaining_seconds to be omitted (LeaseTimeRemain missing)")
	}
}

// TestCollectConcurrentScrapesDoNotRace exercises overlapping Collect
// calls on the same Collector (e.g. two scrapes racing each other),
// which is exactly what `go test -race` would flag if Collect still
// mutated shared prometheus.Gauge fields instead of building metrics
// from immutable Desc + local values.
func TestCollectConcurrentScrapesDoNotRace(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, nil)
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))

	const concurrentScrapes = 10
	done := make(chan struct{})
	for i := 0; i < concurrentScrapes; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			collectMetrics(c)
		}()
	}
	for i := 0; i < concurrentScrapes; i++ {
		<-done
	}
}

func TestDescribe(t *testing.T) {
	c := New(&config.Config{})
	ch := make(chan *prometheus.Desc, 20)
	c.Describe(ch)
	close(ch)

	count := 0
	for range ch {
		count++
	}
	if count != 15 {
		t.Errorf("expected 15 described metrics, got %d", count)
	}
}
