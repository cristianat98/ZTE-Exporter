package zteclient

import (
	"context"
	"net/http"
	"testing"
)

const wanConnectedFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<ParaName>ConnectionStatus</ParaName>
	<ParaValue>Connected</ParaValue>
	<ParaName>WANUptime</ParaName>
	<ParaValue>7200</ParaValue>
	<ParaName>LeaseTimeRemain</ParaName>
	<ParaValue>43200</ParaValue>
</ajax_response_xml_root>`

func wanStatusFixtureWith(status string) string {
	return `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<ParaName>ConnectionStatus</ParaName>
	<ParaValue>` + status + `</ParaValue>
	<ParaName>WANUptime</ParaName>
	<ParaValue>0</ParaValue>
	<ParaName>LeaseTimeRemain</ParaName>
	<ParaValue>0</ParaValue>
</ajax_response_xml_root>`
}

func TestWANStatusFromParamsConnected(t *testing.T) {
	params, err := parseFlatParams([]byte(wanConnectedFixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := wanStatusFromParams(params)
	if status.Connected == nil || !*status.Connected {
		t.Error("expected Connected to be true")
	}
	if status.UptimeSeconds == nil || *status.UptimeSeconds != 7200 {
		t.Errorf("unexpected UptimeSeconds: %v", status.UptimeSeconds)
	}
	if status.LeaseRemainingSeconds == nil || *status.LeaseRemainingSeconds != 43200 {
		t.Errorf("unexpected LeaseRemainingSeconds: %v", status.LeaseRemainingSeconds)
	}
}

func TestWANStatusFromParamsConnecting(t *testing.T) {
	params, err := parseFlatParams([]byte(wanStatusFixtureWith("Connecting")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := wanStatusFromParams(params)
	if status.Connected == nil || *status.Connected {
		t.Error("expected Connected to be false for Connecting status")
	}
}

func TestWANStatusFromParamsDisconnected(t *testing.T) {
	params, err := parseFlatParams([]byte(wanStatusFixtureWith("Disconnected")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := wanStatusFromParams(params)
	if status.Connected == nil || *status.Connected {
		t.Error("expected Connected to be false for Disconnected status")
	}
}

func TestWANStatusFromParamsMissingConnectionStatusLeavesOthersIntact(t *testing.T) {
	params := map[string]string{
		"WANUptime":       "7200",
		"LeaseTimeRemain": "43200",
	}

	status := wanStatusFromParams(params)
	if status.Connected != nil {
		t.Error("expected Connected to be nil when ConnectionStatus is missing")
	}
	if status.UptimeSeconds == nil || *status.UptimeSeconds != 7200 {
		t.Errorf("expected UptimeSeconds to still be populated, got %v", status.UptimeSeconds)
	}
	if status.LeaseRemainingSeconds == nil || *status.LeaseRemainingSeconds != 43200 {
		t.Errorf("expected LeaseRemainingSeconds to still be populated, got %v", status.LeaseRemainingSeconds)
	}
}

func TestGetWANStatusSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "menuView" && query.Get("_tag") == "wanStatus":
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && query.Get("_tag") == wanStatusScript:
			_, _ = w.Write([]byte(wanConnectedFixture))
		default:
			http.NotFound(w, r)
		}
	})
	c := newTestClient(t, mux)

	status, err := c.GetWANStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Connected == nil || !*status.Connected {
		t.Error("expected Connected to be true")
	}
}

func TestGetWANStatusMenuDataFails(t *testing.T) {
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

	if _, err := c.GetWANStatus(context.Background()); err == nil {
		t.Fatal("expected an error when the menuData request fails")
	}
}

func TestGetWANStatusInvalidXML(t *testing.T) {
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

	if _, err := c.GetWANStatus(context.Background()); err == nil {
		t.Fatal("expected a parse error for invalid XML")
	}
}
