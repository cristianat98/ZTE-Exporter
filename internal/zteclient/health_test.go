package zteclient

import (
	"context"
	"net/http"
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

	health := healthFromParams(params)
	if health.MemoryUsedBytes == nil || health.MemoryTotalBytes == nil {
		t.Fatal("expected memory bytes fields to be populated")
	}
	if *health.MemoryTotalBytes != 134217728 {
		t.Errorf("unexpected MemoryTotalBytes: %d", *health.MemoryTotalBytes)
	}
	if *health.MemoryUsedBytes != 67108864 {
		t.Errorf("unexpected MemoryUsedBytes: %d", *health.MemoryUsedBytes)
	}
	if health.MemoryUsagePercent != nil {
		t.Errorf("expected MemoryUsagePercent to be nil, got %v", *health.MemoryUsagePercent)
	}
	if health.CPUUsagePercent == nil || *health.CPUUsagePercent != 12.5 {
		t.Errorf("unexpected CPUUsagePercent: %v", health.CPUUsagePercent)
	}
	if health.UptimeSeconds == nil || *health.UptimeSeconds != 3600 {
		t.Errorf("unexpected UptimeSeconds: %v", health.UptimeSeconds)
	}
}

func TestHealthFromParamsPercentFallback(t *testing.T) {
	params, err := parseFlatParams([]byte(healthPercentFixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	health := healthFromParams(params)
	if health.MemoryUsedBytes != nil || health.MemoryTotalBytes != nil {
		t.Fatal("expected memory bytes fields to be nil")
	}
	if health.MemoryUsagePercent == nil || *health.MemoryUsagePercent != 50 {
		t.Errorf("unexpected MemoryUsagePercent: %v", health.MemoryUsagePercent)
	}
}

func TestHealthFromParamsMissingFieldLeavesOthersIntact(t *testing.T) {
	params := map[string]string{"CPUUsage": "12.5"}

	health := healthFromParams(params)
	if health.CPUUsagePercent == nil || *health.CPUUsagePercent != 12.5 {
		t.Errorf("expected CPUUsagePercent to still be populated, got %v", health.CPUUsagePercent)
	}
	if health.UptimeSeconds != nil {
		t.Errorf("expected UptimeSeconds to be nil (missing SysUpTime), got %v", *health.UptimeSeconds)
	}
	if health.MemoryUsagePercent != nil || health.MemoryUsedBytes != nil {
		t.Error("expected no memory fields to be populated when neither bytes nor percent are present")
	}
}

func TestHealthFromParamsMemFreeExceedsMemTotalLeavesOthersIntact(t *testing.T) {
	params := map[string]string{
		"CPUUsage":  "12.5",
		"SysUpTime": "3600",
		"MemTotal":  "100",
		"MemFree":   "200",
	}

	health := healthFromParams(params)
	if health.MemoryUsedBytes != nil || health.MemoryTotalBytes != nil {
		t.Error("expected memory bytes fields to be nil when MemFree exceeds MemTotal")
	}
	if health.CPUUsagePercent == nil || *health.CPUUsagePercent != 12.5 {
		t.Errorf("expected CPUUsagePercent to still be populated, got %v", health.CPUUsagePercent)
	}
	if health.UptimeSeconds == nil || *health.UptimeSeconds != 3600 {
		t.Errorf("expected UptimeSeconds to still be populated, got %v", health.UptimeSeconds)
	}
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
	c := newTestClient(t, mux)

	health, err := c.GetHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.MemoryUsedBytes == nil {
		t.Error("expected MemoryUsedBytes to be populated")
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
	c := newTestClient(t, mux)

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
	c := newTestClient(t, mux)

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
	c := newTestClient(t, mux)

	if _, err := c.GetHealth(context.Background()); err == nil {
		t.Fatal("expected a parse error for invalid XML")
	}
}
