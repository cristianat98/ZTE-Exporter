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
	collectorTestPassword   = "correct-password"
	collectorTestLoginTok   = "deadbeef"
	collectorLANDevsScript  = "accessdev_landevs_lua.lua"
	collectorWLANDevsScript = "accessdev_ssiddev_lua.lua"
	collectorHealthScript   = "devmgr_statusmgr_lua.lua"
	collectorWANScript      = "wan_internetstatus_lua.lua"
)

const collectorHealthFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<ParaName>CPUUsage</ParaName>
	<ParaValue>10</ParaValue>
	<ParaName>MemTotal</ParaName>
	<ParaValue>1000</ParaValue>
	<ParaName>MemFree</ParaName>
	<ParaValue>400</ParaValue>
	<ParaName>SysUpTime</ParaName>
	<ParaValue>1000</ParaValue>
</ajax_response_xml_root>`

const collectorWANFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<ParaName>ConnectionStatus</ParaName>
	<ParaValue>Connected</ParaValue>
	<ParaName>WANUptime</ParaName>
	<ParaValue>2000</ParaValue>
	<ParaName>LeaseTimeRemain</ParaName>
	<ParaValue>3000</ParaValue>
</ajax_response_xml_root>`

const collectorWLANFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<OBJ_SSIDDEV_ID>
		<Instance>
			<ParaName>MACAddress</ParaName>
			<ParaValue>AA:BB:CC:DD:EE:02</ParaValue>
		</Instance>
	</OBJ_SSIDDEV_ID>
</ajax_response_xml_root>`

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
		case query.Get("_type") == "menuData" && tag == collectorHealthScript:
			_, _ = w.Write([]byte(collectorHealthFixture))
		case query.Get("_type") == "menuData" && tag == collectorWANScript:
			_, _ = w.Write([]byte(collectorWANFixture))
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

func hasMetric(metrics []prometheus.Metric, name string) bool {
	marker := fqNameMarker(name)
	for _, m := range metrics {
		if strings.Contains(m.Desc().String(), marker) {
			return true
		}
	}
	return false
}

func collectMetrics(c *Collector) []prometheus.Metric {
	ch := make(chan prometheus.Metric, 20)
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
	if cpu := gaugeValue(t, metrics, "zte_cpu_usage_percent"); cpu != 10 {
		t.Errorf("expected zte_cpu_usage_percent=10, got %v", cpu)
	}
	if used := gaugeValue(t, metrics, "zte_memory_used_bytes"); used != 600 {
		t.Errorf("expected zte_memory_used_bytes=600, got %v", used)
	}
	if total := gaugeValue(t, metrics, "zte_memory_total_bytes"); total != 1000 {
		t.Errorf("expected zte_memory_total_bytes=1000, got %v", total)
	}
	if uptime := gaugeValue(t, metrics, "zte_uptime_seconds"); uptime != 1000 {
		t.Errorf("expected zte_uptime_seconds=1000, got %v", uptime)
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

func TestCollectHealthFailureDegradesIndependently(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword, map[string]bool{collectorHealthScript: true})
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 1 {
		t.Errorf("expected zte_up=1, got %v", up)
	}
	if hasMetric(metrics, "zte_cpu_usage_percent") {
		t.Error("expected zte_cpu_usage_percent to be omitted")
	}
	if hasMetric(metrics, "zte_uptime_seconds") {
		t.Error("expected zte_uptime_seconds to be omitted")
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
	if cpu := gaugeValue(t, metrics, "zte_cpu_usage_percent"); cpu != 10 {
		t.Errorf("expected zte_cpu_usage_percent=10, got %v", cpu)
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
	if count != 11 {
		t.Errorf("expected 11 described metrics, got %d", count)
	}
}
