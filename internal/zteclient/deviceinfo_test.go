package zteclient

import (
	"context"
	"net/http"
	"testing"
)

// deviceInfoFixture mirrors a live H3600P devmgr_statusmgr_lua.lua
// response: a single Instance nested under OBJ_DEVINFO_ID.
const deviceInfoFixture = `<ajax_response_xml_root>
	<IF_ERRORSTR>SUCC</IF_ERRORSTR>
	<OBJ_DEVINFO_ID>
		<Instance>
			<ParaName>VerDate</ParaName>
			<ParaValue>20240329222419</ParaValue>
			<ParaName>SoftwareVer</ParaName>
			<ParaValue>V9.0.0P5_DIGI</ParaValue>
			<ParaName>ModelName</ParaName>
			<ParaValue>H3600P V9.0</ParaValue>
			<ParaName>HardwareVer</ParaName>
			<ParaValue>V9.0.0</ParaValue>
			<ParaName>SerialNumber</ParaName>
			<ParaValue>ZTEYH93R8J08158</ParaValue>
			<ParaName>BootVer</ParaName>
			<ParaValue>V1.0.0</ParaValue>
		</Instance>
	</OBJ_DEVINFO_ID>
</ajax_response_xml_root>`

func TestDeviceInfoFromParams(t *testing.T) {
	params, err := parseSingleInstance([]byte(deviceInfoFixture), deviceInfoIDElement)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info := deviceInfoFromParams(params)
	if info.Model == nil || *info.Model != "H3600P V9.0" {
		t.Errorf("unexpected Model: %v", info.Model)
	}
	if info.SoftwareVersion == nil || *info.SoftwareVersion != "V9.0.0P5_DIGI" {
		t.Errorf("unexpected SoftwareVersion: %v", info.SoftwareVersion)
	}
	if info.HardwareVersion == nil || *info.HardwareVersion != "V9.0.0" {
		t.Errorf("unexpected HardwareVersion: %v", info.HardwareVersion)
	}
	if info.SerialNumber == nil || *info.SerialNumber != "ZTEYH93R8J08158" {
		t.Errorf("unexpected SerialNumber: %v", info.SerialNumber)
	}
	if info.BootVersion == nil || *info.BootVersion != "V1.0.0" {
		t.Errorf("unexpected BootVersion: %v", info.BootVersion)
	}
	if info.BuildDate == nil || *info.BuildDate != "2024-03-29T22:24:19Z" {
		t.Errorf("unexpected BuildDate: %v", info.BuildDate)
	}
}

func TestDeviceInfoFromParamsMissingFieldLeavesOthersIntact(t *testing.T) {
	params := map[string]string{"ModelName": "H3600P V9.0"}

	info := deviceInfoFromParams(params)
	if info.Model == nil || *info.Model != "H3600P V9.0" {
		t.Errorf("expected Model to still be populated, got %v", info.Model)
	}
	if info.SoftwareVersion != nil {
		t.Errorf("expected SoftwareVersion to be nil, got %v", *info.SoftwareVersion)
	}
	if info.BuildDate != nil {
		t.Errorf("expected BuildDate to be nil, got %v", *info.BuildDate)
	}
}

func TestDeviceInfoFromParamsUnparseableBuildDateFallsBackToRaw(t *testing.T) {
	params := map[string]string{"VerDate": "not-a-timestamp"}

	info := deviceInfoFromParams(params)
	if info.BuildDate == nil || *info.BuildDate != "not-a-timestamp" {
		t.Errorf("expected BuildDate to fall back to the raw value, got %v", info.BuildDate)
	}
}

func TestGetDeviceInfoSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "menuView" && query.Get("_tag") == deviceInfoViewTag && query.Get("Menu3Location") == "0":
			_, _ = w.Write([]byte(`<ajax_response_xml_root><IF_ERRORSTR>SUCC</IF_ERRORSTR></ajax_response_xml_root>`))
		case query.Get("_type") == "menuData" && query.Get("_tag") == deviceInfoScript:
			_, _ = w.Write([]byte(deviceInfoFixture))
		default:
			http.NotFound(w, r)
		}
	})
	c := newTestClient(t, mux)

	info, err := c.GetDeviceInfo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Model == nil || *info.Model != "H3600P V9.0" {
		t.Errorf("unexpected Model: %v", info.Model)
	}
}

func TestGetDeviceInfoMenuViewFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := newTestClient(t, mux)

	if _, err := c.GetDeviceInfo(context.Background()); err == nil {
		t.Fatal("expected an error when the menuView request fails")
	}
}

func TestGetDeviceInfoMenuDataFails(t *testing.T) {
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

	if _, err := c.GetDeviceInfo(context.Background()); err == nil {
		t.Fatal("expected an error when the menuData request fails")
	}
}

func TestGetDeviceInfoInvalidXML(t *testing.T) {
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

	if _, err := c.GetDeviceInfo(context.Background()); err == nil {
		t.Fatal("expected a parse error for invalid XML")
	}
}
