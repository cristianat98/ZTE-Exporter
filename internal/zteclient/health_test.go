package zteclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const healthBytesFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<ParaName>CPUUsage</ParaName>
	<ParaValue>12.5</ParaValue>
	<ParaName>MemTotal</ParaName>
	<ParaValue>134217728</ParaValue>
	<ParaName>MemFree</ParaName>
	<ParaValue>67108864</ParaValue>
	<ParaName>SysUpTime</ParaName>
	<ParaValue>3600</ParaValue>
</ajax_response_xml_root>`

const healthPercentFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<ParaName>CPUUsage</ParaName>
	<ParaValue>12.5</ParaValue>
	<ParaName>MemUsage</ParaName>
	<ParaValue>50</ParaValue>
	<ParaName>SysUpTime</ParaName>
	<ParaValue>3600</ParaValue>
</ajax_response_xml_root>`

func TestHealthFromParamsBytes(t *testing.T) {
	params, err := parseFlatParams([]byte(healthBytesFixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	health, err := healthFromParams(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !health.HasMemoryBytes {
		t.Fatal("expected HasMemoryBytes to be true")
	}
	if health.MemoryTotalBytes != 134217728 {
		t.Errorf("unexpected MemoryTotalBytes: %d", health.MemoryTotalBytes)
	}
	if health.MemoryUsedBytes != 67108864 {
		t.Errorf("unexpected MemoryUsedBytes: %d", health.MemoryUsedBytes)
	}
	if health.CPUUsagePercent != 12.5 {
		t.Errorf("unexpected CPUUsagePercent: %v", health.CPUUsagePercent)
	}
	if health.UptimeSeconds != 3600 {
		t.Errorf("unexpected UptimeSeconds: %d", health.UptimeSeconds)
	}
}

func TestHealthFromParamsPercentFallback(t *testing.T) {
	params, err := parseFlatParams([]byte(healthPercentFixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	health, err := healthFromParams(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.HasMemoryBytes {
		t.Fatal("expected HasMemoryBytes to be false")
	}
	if health.MemoryUsagePercent != 50 {
		t.Errorf("unexpected MemoryUsagePercent: %v", health.MemoryUsagePercent)
	}
}

func TestHealthFromParamsMissingField(t *testing.T) {
	params := map[string]string{"CPUUsage": "12.5"}
	if _, err := healthFromParams(params); err == nil {
		t.Fatal("expected an error for missing SysUpTime")
	}
}

func newHealthTestClient(t *testing.T, mux *http.ServeMux) *Client {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := NewClient("placeholder", "admin", "secret", 0)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	c.baseURL = srv.URL
	return c
}

func TestGetHealthSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "menuView" && query.Get("_tag") == "deviceStatus":
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && query.Get("_tag") == healthScript:
			_, _ = w.Write([]byte(healthBytesFixture))
		default:
			http.NotFound(w, r)
		}
	})
	c := newHealthTestClient(t, mux)

	health, err := c.GetHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !health.HasMemoryBytes {
		t.Error("expected HasMemoryBytes to be true")
	}
}

func TestGetHealthMenuDataFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("_type") == "menuView" {
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := newHealthTestClient(t, mux)

	if _, err := c.GetHealth(context.Background()); err == nil {
		t.Fatal("expected an error when the menuData request fails")
	}
}

func TestGetHealthRouterError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("_type") == "menuView" {
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
			return
		}
		_, _ = w.Write([]byte(errorFixture))
	})
	c := newHealthTestClient(t, mux)

	if _, err := c.GetHealth(context.Background()); err == nil {
		t.Fatal("expected an error for a SessionTimeout response")
	}
}

func TestGetHealthInvalidXML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("_type") == "menuView" {
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
			return
		}
		_, _ = w.Write([]byte("not xml"))
	})
	c := newHealthTestClient(t, mux)

	if _, err := c.GetHealth(context.Background()); err == nil {
		t.Fatal("expected a parse error for invalid XML")
	}
}
