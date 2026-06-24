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
	collectorTestPassword  = "correct-password"
	collectorTestLoginTok  = "deadbeef"
	collectorLANDevsScript = "accessdev_landevs_lua.lua"
)

func newFakeRouter(t *testing.T, password string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "loginData" && query.Get("_tag") == "login_entry" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"sess_token":"abc123","lockingTime":0}`))
		case query.Get("_type") == "loginData" && query.Get("_tag") == "login_token":
			_, _ = w.Write([]byte(`<ajax_response_xml_root>` + collectorTestLoginTok + `</ajax_response_xml_root>`))
		case query.Get("_type") == "loginData" && query.Get("_tag") == "login_entry" && r.Method == http.MethodPost:
			buf, _ := url.ParseQuery(readBody(t, r))
			hash := sha256.Sum256([]byte(password + collectorTestLoginTok))
			expected := hex.EncodeToString(hash[:])
			if buf.Get("Password") == expected {
				_, _ = w.Write([]byte(`{"login_need_refresh":0,"lockingTime":0,"loginErrMsg":""}`))
			} else {
				_, _ = w.Write([]byte(`{"login_need_refresh":0,"lockingTime":0,"loginErrMsg":"Wrong username or password"}`))
			}
		case query.Get("_type") == "menuView" && query.Get("_tag") == "localNetStatus":
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && query.Get("_tag") == collectorLANDevsScript:
			_, _ = w.Write([]byte(`<ajax_response_xml_root>
				<IF_ERRORSTR>SUCC</IF_ERRORSTR>
				<OBJ_ACCESSDEV_ID>
					<Instance>
						<ParaName>MACAddress</ParaName>
						<ParaValue>AA:BB:CC:DD:EE:01</ParaValue>
					</Instance>
				</OBJ_ACCESSDEV_ID>
			</ajax_response_xml_root>`))
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

func gaugeValue(t *testing.T, metrics []prometheus.Metric, name string) float64 {
	t.Helper()
	for _, m := range metrics {
		if !strings.Contains(m.Desc().String(), name) {
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

func collectMetrics(c *Collector) []prometheus.Metric {
	ch := make(chan prometheus.Metric, 10)
	c.Collect(ch)
	close(ch)

	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}
	return metrics
}

func TestCollectSuccess(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword)
	defer srv.Close()

	c := New(cfgForServer(srv, collectorTestPassword))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 1 {
		t.Errorf("expected zte_up=1, got %v", up)
	}
	if lan := gaugeValue(t, metrics, "zte_lan_connected_devices"); lan != 1 {
		t.Errorf("expected zte_lan_connected_devices=1, got %v", lan)
	}
}

func TestCollectScrapeFailure(t *testing.T) {
	srv := newFakeRouter(t, collectorTestPassword)
	defer srv.Close()

	c := New(cfgForServer(srv, "wrong-password"))
	metrics := collectMetrics(c)

	if up := gaugeValue(t, metrics, "zte_up"); up != 0 {
		t.Errorf("expected zte_up=0 on scrape failure, got %v", up)
	}
}

func TestDescribe(t *testing.T) {
	c := New(&config.Config{})
	ch := make(chan *prometheus.Desc, 10)
	c.Describe(ch)
	close(ch)

	count := 0
	for range ch {
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 described metrics, got %d", count)
	}
}
