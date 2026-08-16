package zteclient

import (
	"context"
	"net/http"
	"testing"
)

// wlanTrafficFixture mirrors a live H3600P wlan_status_lua.lua response,
// trimmed to 3 of the real 8 AP slots: AP1 enabled/2.4GHz, AP2
// disabled/2.4GHz (zero counters), AP5 enabled/5GHz. This subset already
// exercises enabled+disabled+both-radios.
const wlanTrafficFixture = `<ajax_response_xml_root><IF_ERRORPARAM>SUCC</IF_ERRORPARAM><IF_ERRORTYPE>SUCC</IF_ERRORTYPE><IF_ERRORSTR>SUCC</IF_ERRORSTR><IF_ERRORID>0</IF_ERRORID><OBJ_WLANAP_ID><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.AP1</ParaValue><ParaName>Enable</ParaName><ParaValue>1</ParaValue><ParaName>Alias</ParaName><ParaValue>SSID1</ParaValue><ParaName>ESSID</ParaName><ParaValue>DIGIFIBRA-5UN9</ParaValue><ParaName>WPA3EncryptType</ParaName><ParaValue>AESEncryption</ParaValue><ParaName>WPA3AuthMode</ParaName><ParaValue>SAEAuthentication</ParaValue><ParaName>WPAAuthMode</ParaName><ParaValue>PSKAuthentication</ParaValue><ParaName>WLANViewName</ParaName><ParaValue>DEV.WIFI.RD1</ParaValue><ParaName>11iEncryptType</ParaName><ParaValue>AESEncryption</ParaValue><ParaName>WPAEncryptType</ParaName><ParaValue>AESEncryption</ParaValue><ParaName>BeaconType</ParaName><ParaValue>11iandWPA3</ParaValue><ParaName>11iAuthMode</ParaName><ParaValue>PSKAuthentication</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.AP2</ParaValue><ParaName>Enable</ParaName><ParaValue>0</ParaValue><ParaName>Alias</ParaName><ParaValue>SSID2</ParaValue><ParaName>ESSID</ParaName><ParaValue>SSID2</ParaValue><ParaName>WPA3EncryptType</ParaName><ParaValue>AESEncryption</ParaValue><ParaName>WPA3AuthMode</ParaName><ParaValue>SAEAuthentication</ParaValue><ParaName>WPAAuthMode</ParaName><ParaValue>PSKAuthentication</ParaValue><ParaName>WLANViewName</ParaName><ParaValue>DEV.WIFI.RD1</ParaValue><ParaName>11iEncryptType</ParaName><ParaValue>AESEncryption</ParaValue><ParaName>WPAEncryptType</ParaName><ParaValue>AESEncryption</ParaValue><ParaName>BeaconType</ParaName><ParaValue>11i</ParaValue><ParaName>11iAuthMode</ParaName><ParaValue>PSKAuthentication</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.AP5</ParaValue><ParaName>Enable</ParaName><ParaValue>1</ParaValue><ParaName>Alias</ParaName><ParaValue>SSID5</ParaValue><ParaName>ESSID</ParaName><ParaValue>DIGIFIBRA-PLUS-5UN9</ParaValue><ParaName>WPA3EncryptType</ParaName><ParaValue>AESEncryption</ParaValue><ParaName>WPA3AuthMode</ParaName><ParaValue>SAEAuthentication</ParaValue><ParaName>WPAAuthMode</ParaName><ParaValue>PSKAuthentication</ParaValue><ParaName>WLANViewName</ParaName><ParaValue>DEV.WIFI.RD2</ParaValue><ParaName>11iEncryptType</ParaName><ParaValue>AESEncryption</ParaValue><ParaName>WPAEncryptType</ParaName><ParaValue>AESEncryption</ParaValue><ParaName>BeaconType</ParaName><ParaValue>11iandWPA3</ParaValue><ParaName>11iAuthMode</ParaName><ParaValue>PSKAuthentication</ParaValue></Instance></OBJ_WLANAP_ID><OBJ_WLANCONFIGDRV_ID><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.AP1</ParaValue><ParaName>TotalBytesSent</ParaName><ParaValue>2235396767</ParaValue><ParaName>Bssid</ParaName><ParaValue>f4:fc:49:72:b8:10</ParaValue><ParaName>RealRF</ParaName><ParaValue>1</ParaValue><ParaName>WLANViewName</ParaName><ParaValue>DEV.WIFI.RD1</ParaValue><ParaName>ChannelInUsed</ParaName><ParaValue>7</ParaValue><ParaName>TotalPacketsSent</ParaName><ParaValue>1850327</ParaValue><ParaName>TotalPacketsReceived</ParaName><ParaValue>581840</ParaValue><ParaName>TotalBytesReceived</ParaName><ParaValue>128343400</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.AP2</ParaValue><ParaName>TotalBytesSent</ParaName><ParaValue>0</ParaValue><ParaName>Bssid</ParaName><ParaValue>f6:fc:49:12:b8:10</ParaValue><ParaName>RealRF</ParaName><ParaValue>1</ParaValue><ParaName>WLANViewName</ParaName><ParaValue>DEV.WIFI.RD1</ParaValue><ParaName>ChannelInUsed</ParaName><ParaValue>7</ParaValue><ParaName>TotalPacketsSent</ParaName><ParaValue>0</ParaValue><ParaName>TotalPacketsReceived</ParaName><ParaValue>0</ParaValue><ParaName>TotalBytesReceived</ParaName><ParaValue>0</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.AP5</ParaValue><ParaName>TotalBytesSent</ParaName><ParaValue>69667980577</ParaValue><ParaName>Bssid</ParaName><ParaValue>f4:fc:49:72:b8:11</ParaValue><ParaName>RealRF</ParaName><ParaValue>1</ParaValue><ParaName>WLANViewName</ParaName><ParaValue>DEV.WIFI.RD2</ParaValue><ParaName>ChannelInUsed</ParaName><ParaValue>56</ParaValue><ParaName>TotalPacketsSent</ParaName><ParaValue>56313196</ParaValue><ParaName>TotalPacketsReceived</ParaName><ParaValue>12538992</ParaValue><ParaName>TotalBytesReceived</ParaName><ParaValue>2975426141</ParaValue></Instance></OBJ_WLANCONFIGDRV_ID><OBJ_WLANSETTING_ID><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.RD1</ParaValue><ParaName>Band</ParaName><ParaValue>2.4GHz</ParaValue></Instance><Instance><ParaName>_InstID</ParaName><ParaValue>DEV.WIFI.RD2</ParaValue><ParaName>Band</ParaName><ParaValue>5GHz</ParaValue></Instance></OBJ_WLANSETTING_ID></ajax_response_xml_root>`

func TestParseWLANTrafficSuccess(t *testing.T) {
	traffic, err := parseWLANTraffic([]byte(wlanTrafficFixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(traffic) != 3 {
		t.Fatalf("expected 3 SSID slots, got %d", len(traffic))
	}

	ap1 := traffic[0]
	if ap1.APID != "DEV.WIFI.AP1" {
		t.Errorf("unexpected APID: %s", ap1.APID)
	}
	if ap1.ESSID != "DIGIFIBRA-5UN9" {
		t.Errorf("unexpected ESSID: %s", ap1.ESSID)
	}
	if ap1.Band != "2.4GHz" {
		t.Errorf("unexpected Band: %s", ap1.Band)
	}
	if ap1.BytesReceived == nil || *ap1.BytesReceived != 128343400 {
		t.Errorf("unexpected BytesReceived: %v", ap1.BytesReceived)
	}
	if ap1.BytesSent == nil || *ap1.BytesSent != 2235396767 {
		t.Errorf("unexpected BytesSent: %v", ap1.BytesSent)
	}
	if ap1.PacketsReceived == nil || *ap1.PacketsReceived != 581840 {
		t.Errorf("unexpected PacketsReceived: %v", ap1.PacketsReceived)
	}
	if ap1.PacketsSent == nil || *ap1.PacketsSent != 1850327 {
		t.Errorf("unexpected PacketsSent: %v", ap1.PacketsSent)
	}

	ap2 := traffic[1]
	if ap2.APID != "DEV.WIFI.AP2" {
		t.Errorf("unexpected APID: %s", ap2.APID)
	}
	if ap2.ESSID != "SSID2" {
		t.Errorf("unexpected ESSID: %s", ap2.ESSID)
	}
	if ap2.Band != "2.4GHz" {
		t.Errorf("unexpected Band: %s", ap2.Band)
	}
	if ap2.BytesReceived == nil || *ap2.BytesReceived != 0 {
		t.Errorf("unexpected BytesReceived: %v", ap2.BytesReceived)
	}
	if ap2.BytesSent == nil || *ap2.BytesSent != 0 {
		t.Errorf("unexpected BytesSent: %v", ap2.BytesSent)
	}
	if ap2.PacketsReceived == nil || *ap2.PacketsReceived != 0 {
		t.Errorf("unexpected PacketsReceived: %v", ap2.PacketsReceived)
	}
	if ap2.PacketsSent == nil || *ap2.PacketsSent != 0 {
		t.Errorf("unexpected PacketsSent: %v", ap2.PacketsSent)
	}

	ap5 := traffic[2]
	if ap5.APID != "DEV.WIFI.AP5" {
		t.Errorf("unexpected APID: %s", ap5.APID)
	}
	if ap5.ESSID != "DIGIFIBRA-PLUS-5UN9" {
		t.Errorf("unexpected ESSID: %s", ap5.ESSID)
	}
	if ap5.Band != "5GHz" {
		t.Errorf("unexpected Band: %s", ap5.Band)
	}
	if ap5.BytesReceived == nil || *ap5.BytesReceived != 2975426141 {
		t.Errorf("unexpected BytesReceived: %v", ap5.BytesReceived)
	}
	if ap5.BytesSent == nil || *ap5.BytesSent != 69667980577 {
		t.Errorf("unexpected BytesSent: %v", ap5.BytesSent)
	}
	if ap5.PacketsReceived == nil || *ap5.PacketsReceived != 12538992 {
		t.Errorf("unexpected PacketsReceived: %v", ap5.PacketsReceived)
	}
	if ap5.PacketsSent == nil || *ap5.PacketsSent != 56313196 {
		t.Errorf("unexpected PacketsSent: %v", ap5.PacketsSent)
	}
}

func TestParseWLANTrafficESSIDJoinMiss(t *testing.T) {
	const fixture = `<ajax_response_xml_root>
		<IF_ERRORSTR>SUCC</IF_ERRORSTR>
		<OBJ_WLANAP_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.AP9</ParaValue>
				<ParaName>ESSID</ParaName>
				<ParaValue>SomeOtherSSID</ParaValue>
				<ParaName>WLANViewName</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
			</Instance>
		</OBJ_WLANAP_ID>
		<OBJ_WLANCONFIGDRV_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.AP1</ParaValue>
				<ParaName>WLANViewName</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
				<ParaName>TotalBytesReceived</ParaName>
				<ParaValue>100</ParaValue>
				<ParaName>TotalBytesSent</ParaName>
				<ParaValue>200</ParaValue>
				<ParaName>TotalPacketsReceived</ParaName>
				<ParaValue>10</ParaValue>
				<ParaName>TotalPacketsSent</ParaName>
				<ParaValue>20</ParaValue>
			</Instance>
		</OBJ_WLANCONFIGDRV_ID>
		<OBJ_WLANSETTING_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
				<ParaName>Band</ParaName>
				<ParaValue>2.4GHz</ParaValue>
			</Instance>
		</OBJ_WLANSETTING_ID>
	</ajax_response_xml_root>`

	traffic, err := parseWLANTraffic([]byte(fixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(traffic) != 1 {
		t.Fatalf("expected 1 SSID slot, got %d", len(traffic))
	}
	if traffic[0].ESSID != "" {
		t.Errorf("expected empty ESSID on join miss, got %q", traffic[0].ESSID)
	}
	if traffic[0].Band != "2.4GHz" {
		t.Errorf("expected Band to still resolve, got %q", traffic[0].Band)
	}
	if traffic[0].BytesReceived == nil || *traffic[0].BytesReceived != 100 {
		t.Errorf("unexpected BytesReceived: %v", traffic[0].BytesReceived)
	}
}

func TestParseWLANTrafficBandJoinMiss(t *testing.T) {
	const fixture = `<ajax_response_xml_root>
		<IF_ERRORSTR>SUCC</IF_ERRORSTR>
		<OBJ_WLANAP_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.AP1</ParaValue>
				<ParaName>ESSID</ParaName>
				<ParaValue>MySSID</ParaValue>
				<ParaName>WLANViewName</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
			</Instance>
		</OBJ_WLANAP_ID>
		<OBJ_WLANCONFIGDRV_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.AP1</ParaValue>
				<ParaName>WLANViewName</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
				<ParaName>TotalBytesReceived</ParaName>
				<ParaValue>100</ParaValue>
				<ParaName>TotalBytesSent</ParaName>
				<ParaValue>200</ParaValue>
				<ParaName>TotalPacketsReceived</ParaName>
				<ParaValue>10</ParaValue>
				<ParaName>TotalPacketsSent</ParaName>
				<ParaValue>20</ParaValue>
			</Instance>
		</OBJ_WLANCONFIGDRV_ID>
		<OBJ_WLANSETTING_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.RD9</ParaValue>
				<ParaName>Band</ParaName>
				<ParaValue>2.4GHz</ParaValue>
			</Instance>
		</OBJ_WLANSETTING_ID>
	</ajax_response_xml_root>`

	traffic, err := parseWLANTraffic([]byte(fixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(traffic) != 1 {
		t.Fatalf("expected 1 SSID slot, got %d", len(traffic))
	}
	if traffic[0].ESSID != "MySSID" {
		t.Errorf("expected ESSID to still resolve, got %q", traffic[0].ESSID)
	}
	if traffic[0].Band != "" {
		t.Errorf("expected empty Band on join miss, got %q", traffic[0].Band)
	}
}

func TestParseWLANTrafficDuplicateAPIDSkipped(t *testing.T) {
	const fixture = `<ajax_response_xml_root>
		<IF_ERRORSTR>SUCC</IF_ERRORSTR>
		<OBJ_WLANAP_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.AP1</ParaValue>
				<ParaName>ESSID</ParaName>
				<ParaValue>MySSID</ParaValue>
				<ParaName>WLANViewName</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
			</Instance>
		</OBJ_WLANAP_ID>
		<OBJ_WLANCONFIGDRV_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.AP1</ParaValue>
				<ParaName>WLANViewName</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
				<ParaName>TotalBytesReceived</ParaName>
				<ParaValue>100</ParaValue>
				<ParaName>TotalBytesSent</ParaName>
				<ParaValue>200</ParaValue>
				<ParaName>TotalPacketsReceived</ParaName>
				<ParaValue>10</ParaValue>
				<ParaName>TotalPacketsSent</ParaName>
				<ParaValue>20</ParaValue>
			</Instance>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.AP1</ParaValue>
				<ParaName>WLANViewName</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
				<ParaName>TotalBytesReceived</ParaName>
				<ParaValue>999</ParaValue>
				<ParaName>TotalBytesSent</ParaName>
				<ParaValue>999</ParaValue>
				<ParaName>TotalPacketsReceived</ParaName>
				<ParaValue>999</ParaValue>
				<ParaName>TotalPacketsSent</ParaName>
				<ParaValue>999</ParaValue>
			</Instance>
		</OBJ_WLANCONFIGDRV_ID>
		<OBJ_WLANSETTING_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
				<ParaName>Band</ParaName>
				<ParaValue>2.4GHz</ParaValue>
			</Instance>
		</OBJ_WLANSETTING_ID>
	</ajax_response_xml_root>`

	traffic, err := parseWLANTraffic([]byte(fixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(traffic) != 1 {
		t.Fatalf("expected the duplicate-APID slot to be skipped, got %d slots", len(traffic))
	}
	if traffic[0].BytesReceived == nil || *traffic[0].BytesReceived != 100 {
		t.Errorf("expected the first AP1 instance to win, got %v", traffic[0].BytesReceived)
	}
}

func TestParseWLANTrafficFieldDegrade(t *testing.T) {
	const fixture = `<ajax_response_xml_root>
		<IF_ERRORSTR>SUCC</IF_ERRORSTR>
		<OBJ_WLANAP_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.AP1</ParaValue>
				<ParaName>ESSID</ParaName>
				<ParaValue>MySSID</ParaValue>
				<ParaName>WLANViewName</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
			</Instance>
		</OBJ_WLANAP_ID>
		<OBJ_WLANCONFIGDRV_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.AP1</ParaValue>
				<ParaName>WLANViewName</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
				<ParaName>TotalBytesReceived</ParaName>
				<ParaValue>100</ParaValue>
				<ParaName>TotalBytesSent</ParaName>
				<ParaValue>200</ParaValue>
				<ParaName>TotalPacketsReceived</ParaName>
				<ParaValue>10</ParaValue>
				<ParaName>TotalPacketsSent</ParaName>
				<ParaValue>not-a-number</ParaValue>
			</Instance>
		</OBJ_WLANCONFIGDRV_ID>
		<OBJ_WLANSETTING_ID>
			<Instance>
				<ParaName>_InstID</ParaName>
				<ParaValue>DEV.WIFI.RD1</ParaValue>
				<ParaName>Band</ParaName>
				<ParaValue>2.4GHz</ParaValue>
			</Instance>
		</OBJ_WLANSETTING_ID>
	</ajax_response_xml_root>`

	traffic, err := parseWLANTraffic([]byte(fixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(traffic) != 1 {
		t.Fatalf("expected 1 SSID slot, got %d", len(traffic))
	}
	entry := traffic[0]
	if entry.ESSID != "MySSID" {
		t.Errorf("unexpected ESSID: %s", entry.ESSID)
	}
	if entry.Band != "2.4GHz" {
		t.Errorf("unexpected Band: %s", entry.Band)
	}
	if entry.BytesReceived == nil || *entry.BytesReceived != 100 {
		t.Errorf("unexpected BytesReceived: %v", entry.BytesReceived)
	}
	if entry.BytesSent == nil || *entry.BytesSent != 200 {
		t.Errorf("unexpected BytesSent: %v", entry.BytesSent)
	}
	if entry.PacketsReceived == nil || *entry.PacketsReceived != 10 {
		t.Errorf("unexpected PacketsReceived: %v", entry.PacketsReceived)
	}
	if entry.PacketsSent != nil {
		t.Errorf("expected PacketsSent to be nil, got %v", *entry.PacketsSent)
	}
}

func TestParseWLANTrafficInvalidXML(t *testing.T) {
	_, err := parseWLANTraffic([]byte("not xml"))
	if err == nil {
		t.Fatal("expected an error for invalid XML")
	}
}

func TestGetWLANTrafficSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "menuView" && query.Get("_tag") == localNetStatusTag:
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && query.Get("_tag") == wlanTrafficScript:
			_, _ = w.Write([]byte(wlanTrafficFixture))
		default:
			http.NotFound(w, r)
		}
	})
	c := newTestClient(t, mux)

	traffic, err := c.GetWLANTraffic(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(traffic) != 3 {
		t.Fatalf("expected 3 SSID slots, got %d", len(traffic))
	}
}

func TestGetWLANTrafficMenuViewFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := newTestClient(t, mux)

	if _, err := c.GetWLANTraffic(context.Background()); err == nil {
		t.Fatal("expected an error when the menuView request fails")
	}
}

func TestGetWLANTrafficMenuDataFails(t *testing.T) {
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

	if _, err := c.GetWLANTraffic(context.Background()); err == nil {
		t.Fatal("expected an error when the menuData request fails")
	}
}

func TestGetWLANTrafficInvalidXML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "menuView" && query.Get("_tag") == localNetStatusTag:
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && query.Get("_tag") == wlanTrafficScript:
			_, _ = w.Write([]byte("not xml"))
		default:
			http.NotFound(w, r)
		}
	})
	c := newTestClient(t, mux)

	if _, err := c.GetWLANTraffic(context.Background()); err == nil {
		t.Fatal("expected an error for invalid XML response")
	}
}
