package zteclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const fakeLoginToken = "deadbeef"

func newTestServer(t *testing.T, password string, expectLoginSuccess bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "loginData" && query.Get("_tag") == "login_entry" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"sess_token":"abc123","lockingTime":0}`))
		case query.Get("_type") == "loginData" && query.Get("_tag") == "login_token":
			_, _ = w.Write([]byte(`<ajax_response_xml_root>` + fakeLoginToken + `</ajax_response_xml_root>`))
		case query.Get("_type") == "loginData" && query.Get("_tag") == "login_entry" && r.Method == http.MethodPost:
			body, _ := url.ParseQuery(readBody(t, r))
			hash := sha256.Sum256([]byte(password + fakeLoginToken))
			expected := hex.EncodeToString(hash[:])
			if expectLoginSuccess && body.Get("Password") == expected {
				_, _ = w.Write([]byte(`{"login_need_refresh":0,"lockingTime":0,"loginErrMsg":""}`))
			} else {
				_, _ = w.Write([]byte(`{"login_need_refresh":0,"lockingTime":0,"loginErrMsg":"Wrong username or password"}`))
			}
		default:
			http.NotFound(w, r)
		}
	})
	return httptest.NewServer(mux)
}

func readBody(t *testing.T, r *http.Request) string {
	t.Helper()
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading request body: %v", err)
	}
	return string(buf)
}

func newClientForServer(t *testing.T, srv *httptest.Server, password string) *Client {
	t.Helper()
	c, err := NewClient("placeholder", "admin", password, 5*time.Second)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	c.baseURL = srv.URL
	return c
}

func TestLoginSuccess(t *testing.T) {
	srv := newTestServer(t, "correct-password", true)
	defer srv.Close()

	c := newClientForServer(t, srv, "correct-password")
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("expected login to succeed, got error: %v", err)
	}
}

func TestLoginSucceedsWithBooleanLoginNeedRefresh(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("_type") == "loginData" && query.Get("_tag") == "login_entry" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"sess_token":"abc123","lockingTime":0}`))
		case query.Get("_type") == "loginData" && query.Get("_tag") == "login_token":
			_, _ = w.Write([]byte(`<ajax_response_xml_root>` + fakeLoginToken + `</ajax_response_xml_root>`))
		case query.Get("_type") == "loginData" && query.Get("_tag") == "login_entry" && r.Method == http.MethodPost:
			// Some firmware versions send a JSON boolean here instead of 0/1.
			_, _ = w.Write([]byte(`{"login_need_refresh":true,"sess_token":"def456","loginErrType":""}`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newClientForServer(t, srv, "correct-password")
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("expected login to succeed, got error: %v", err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	srv := newTestServer(t, "correct-password", true)
	defer srv.Close()

	c := newClientForServer(t, srv, "wrong-password")
	err := c.Login(context.Background())
	if err == nil {
		t.Fatal("expected login to fail with wrong password")
	}
	if !strings.Contains(err.Error(), "password") {
		t.Errorf("expected error to mention password, got: %v", err)
	}
}
