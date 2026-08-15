package zteclient

import (
	"context"
	"net/http"
	"testing"
)

// lanTrafficFixture is the live eth_lanstatus_lua.lua response captured
// from an H3600P: 3 physical LAN ports, one Up (LAN1) and two NoLink
// (LAN2, LAN3), plus an OBJ_WANLAN_ID section that is out of scope for
// LAN traffic parsing and must be ignored.
const lanTrafficFixture = `<ajax_response_xml_root><IF_ERRORPARAM>SUCC</IF_ERRORPARAM><IF_ERRORTYPE>SUCC</IF_ERRORTYPE><IF_ERRORSTR>SUCC</IF_ERRORSTR><IF_ERRORID>0</IF_ERRORID><OBJ_ETH_ID><Instance><ParaName>_InstID</ParaName><ParaValue>IGD.LD1.ETH1</ParaValue><ParaName>AliasName</ParaName><ParaValue>LAN1</ParaValue><ParaName>LinkSpeed</ParaName><ParaValue>1000</ParaValue><ParaName>LinkDuplex</ParaName><ParaValue>Full</ParaValue><ParaName>Status</ParaName><ParaValue>Up</ParaValue><ParaName>BytesReceived</ParaName><ParaValue>13036552880</ParaValue><ParaName>MACAddress</ParaName><ParaValue>f4:fc:49:72:b8:10</ParaValue><ParaName>BytesSent</ParaName><ParaValue>124765590890</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>IGD.LD1.ETH2</ParaValue><ParaName>AliasName</ParaName><ParaValue>LAN2</ParaValue><ParaName>LinkSpeed</ParaName><ParaValue>10</ParaValue><ParaName>LinkDuplex</ParaName><ParaValue>Half</ParaValue><ParaName>Status</ParaName><ParaValue>NoLink</ParaValue><ParaName>BytesReceived</ParaName><ParaValue>0</ParaValue><ParaName>MACAddress</ParaName><ParaValue>f4:fc:49:72:b8:10</ParaValue><ParaName>BytesSent</ParaName><ParaValue>0</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>IGD.LD1.ETH3</ParaValue><ParaName>AliasName</ParaName><ParaValue>LAN3</ParaValue><ParaName>LinkSpeed</ParaName><ParaValue>10</ParaValue><ParaName>LinkDuplex</ParaName><ParaValue>Half</ParaValue><ParaName>Status</ParaName><ParaValue>NoLink</ParaValue><ParaName>BytesReceived</ParaName><ParaValue>0</ParaValue><ParaName>MACAddress</ParaName><ParaValue>f4:fc:49:72:b8:10</ParaValue><ParaName>BytesSent</ParaName><ParaValue>0</ParaValue></Instance></OBJ_ETH_ID><OBJ_WANLAN_ID><Instance><ParaName>_InstID</ParaName><ParaValue>IGD.LD1.ETH1</ParaValue><ParaName>IPAddress</ParaName><ParaValue>192.168.1.1</ParaValue><ParaName>IPv6Addr</ParaName><ParaValue>fe80::1</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>IGD.LD1.ETH2</ParaValue><ParaName>IPAddress</ParaName><ParaValue>192.168.1.1</ParaValue><ParaName>IPv6Addr</ParaName><ParaValue>fe80::1</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>IGD.LD1.ETH3</ParaValue><ParaName>IPAddress</ParaName><ParaValue>192.168.1.1</ParaValue><ParaName>IPv6Addr</ParaName><ParaValue>fe80::1</ParaValue></Instance></OBJ_WANLAN_ID></ajax_response_xml_root>`

func TestParseLANTrafficSuccess(t *testing.T) {
	ports, err := parseLANTraffic([]byte(lanTrafficFixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(ports))
	}

	first := ports[0]
	if first.Port != "LAN1" {
		t.Errorf("unexpected port label: %s", first.Port)
	}
	if first.BytesReceived == nil || *first.BytesReceived != 13036552880 {
		t.Errorf("unexpected BytesReceived: %v", first.BytesReceived)
	}
	if first.BytesSent == nil || *first.BytesSent != 124765590890 {
		t.Errorf("unexpected BytesSent: %v", first.BytesSent)
	}

	second := ports[1]
	if second.Port != "LAN2" {
		t.Errorf("unexpected port label: %s", second.Port)
	}
	if second.BytesReceived == nil || *second.BytesReceived != 0 {
		t.Errorf("unexpected BytesReceived: %v", second.BytesReceived)
	}
	if second.BytesSent == nil || *second.BytesSent != 0 {
		t.Errorf("unexpected BytesSent: %v", second.BytesSent)
	}

	third := ports[2]
	if third.Port != "LAN3" {
		t.Errorf("unexpected port label: %s", third.Port)
	}
}

func TestParseLANTrafficLabelFallback(t *testing.T) {
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

	ports, err := parseLANTraffic([]byte(fixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(ports))
	}
	if ports[0].Port != "IGD.LD1.ETH1" {
		t.Errorf("expected fallback to _InstID, got %q", ports[0].Port)
	}
	if ports[0].BytesReceived == nil || *ports[0].BytesReceived != 100 {
		t.Errorf("unexpected BytesReceived: %v", ports[0].BytesReceived)
	}
}

func TestParseLANTrafficFieldDegrade(t *testing.T) {
	const fixture = `<ajax_response_xml_root>
		<IF_ERRORSTR>SUCC</IF_ERRORSTR>
		<OBJ_ETH_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>IGD.LD1.ETH1</ParaValue>
				<ParaName>AliasName</ParaName>
				<ParaValue>LAN1</ParaValue>
				<ParaName>BytesReceived</ParaName>
				<ParaValue>100</ParaValue>
				<ParaName>BytesSent</ParaName>
				<ParaValue>not-a-number</ParaValue>
			</Instance>
		</OBJ_ETH_ID>
	</ajax_response_xml_root>`

	ports, err := parseLANTraffic([]byte(fixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(ports))
	}
	if ports[0].Port != "LAN1" {
		t.Errorf("unexpected port label: %s", ports[0].Port)
	}
	if ports[0].BytesReceived == nil || *ports[0].BytesReceived != 100 {
		t.Errorf("unexpected BytesReceived: %v", ports[0].BytesReceived)
	}
	if ports[0].BytesSent != nil {
		t.Errorf("expected BytesSent to be nil, got %v", *ports[0].BytesSent)
	}
}

func TestParseLANTrafficDuplicatePortSkipped(t *testing.T) {
	const fixture = `<ajax_response_xml_root>
		<IF_ERRORSTR>SUCC</IF_ERRORSTR>
		<OBJ_ETH_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>IGD.LD1.ETH1</ParaValue>
				<ParaName>AliasName</ParaName>
				<ParaValue>LAN1</ParaValue>
				<ParaName>BytesReceived</ParaName>
				<ParaValue>100</ParaValue>
				<ParaName>BytesSent</ParaName>
				<ParaValue>200</ParaValue>
			</Instance>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>IGD.LD1.ETH2</ParaValue>
				<ParaName>AliasName</ParaName>
				<ParaValue>LAN1</ParaValue>
				<ParaName>BytesReceived</ParaName>
				<ParaValue>999</ParaValue>
				<ParaName>BytesSent</ParaName>
				<ParaValue>999</ParaValue>
			</Instance>
		</OBJ_ETH_ID>
	</ajax_response_xml_root>`

	ports, err := parseLANTraffic([]byte(fixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 1 {
		t.Fatalf("expected the duplicate-labeled port to be skipped, got %d ports", len(ports))
	}
	if ports[0].BytesReceived == nil || *ports[0].BytesReceived != 100 {
		t.Errorf("expected the first LAN1 instance to win, got %v", ports[0].BytesReceived)
	}
}

func TestParseLANTrafficInvalidXML(t *testing.T) {
	_, err := parseLANTraffic([]byte("not xml"))
	if err == nil {
		t.Fatal("expected an error for invalid XML")
	}
}

func TestGetLANTrafficSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "menuView" && query.Get("_tag") == localNetStatusTag:
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && query.Get("_tag") == lanTrafficScript:
			_, _ = w.Write([]byte(lanTrafficFixture))
		default:
			http.NotFound(w, r)
		}
	})
	c := newTestClient(t, mux)

	ports, err := c.GetLANTraffic(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(ports))
	}
}

func TestGetLANTrafficMenuViewFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := newTestClient(t, mux)

	if _, err := c.GetLANTraffic(context.Background()); err == nil {
		t.Fatal("expected an error when the menuView request fails")
	}
}

func TestGetLANTrafficMenuDataFails(t *testing.T) {
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

	if _, err := c.GetLANTraffic(context.Background()); err == nil {
		t.Fatal("expected an error when the menuData request fails")
	}
}

func TestGetLANTrafficInvalidXML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "menuView" && query.Get("_tag") == localNetStatusTag:
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && query.Get("_tag") == lanTrafficScript:
			_, _ = w.Write([]byte("not xml"))
		default:
			http.NotFound(w, r)
		}
	})
	c := newTestClient(t, mux)

	if _, err := c.GetLANTraffic(context.Background()); err == nil {
		t.Fatal("expected an error for invalid XML response")
	}
}
