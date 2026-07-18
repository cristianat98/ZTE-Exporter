package zteclient

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	status, err := wanStatusFromParams(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Connected {
		t.Error("expected Connected to be true")
	}
	if status.UptimeSeconds != 7200 {
		t.Errorf("unexpected UptimeSeconds: %d", status.UptimeSeconds)
	}
	if status.LeaseRemainingSeconds != 43200 {
		t.Errorf("unexpected LeaseRemainingSeconds: %d", status.LeaseRemainingSeconds)
	}
}

func TestWANStatusFromParamsConnecting(t *testing.T) {
	params, err := parseFlatParams([]byte(wanStatusFixtureWith("Connecting")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status, err := wanStatusFromParams(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Connected {
		t.Error("expected Connected to be false for Connecting status")
	}
}

func TestWANStatusFromParamsDisconnected(t *testing.T) {
	params, err := parseFlatParams([]byte(wanStatusFixtureWith("Disconnected")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status, err := wanStatusFromParams(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Connected {
		t.Error("expected Connected to be false for Disconnected status")
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
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := NewClient("placeholder", "admin", "secret", 0)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	c.baseURL = srv.URL

	status, err := c.GetWANStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Connected {
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
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := NewClient("placeholder", "admin", "secret", 0)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	c.baseURL = srv.URL

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
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := NewClient("placeholder", "admin", "secret", 0)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	c.baseURL = srv.URL

	if _, err := c.GetWANStatus(context.Background()); err == nil {
		t.Fatal("expected a parse error for invalid XML")
	}
}
