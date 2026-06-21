// Package zteclient implements an HTTP client for the ZTE H3600P router's
// authenticated web UI, following the challenge/login flow and XML/JSON
// response formats observed on that router's admin interface.
package zteclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// Client talks to a single ZTE H3600P router over its web UI.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
	guid       int64
}

// NewClient creates a Client for the router reachable at host (e.g.
// "192.168.1.1"). The H3600P web UI is served over HTTPS with a
// self-signed certificate, so TLS verification is intentionally skipped.
func NewClient(host, username, password string, timeout time.Duration) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	return &Client{
		baseURL:  fmt.Sprintf("https://%s", host),
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: insecureTransport(),
			Jar:       jar,
		},
		guid: time.Now().UnixMilli(),
	}, nil
}

func (c *Client) nextGUID() int64 {
	return atomic.AddInt64(&c.guid, 1)
}

type ajaxResponse struct {
	XMLName  xml.Name `xml:"ajax_response_xml_root"`
	ErrorStr string   `xml:"IF_ERRORSTR"`
	Text     string   `xml:",chardata"`
}

func (r ajaxResponse) checkError() error {
	if r.ErrorStr != "" && r.ErrorStr != "SUCC" && r.ErrorStr != "SUCCESS" && r.ErrorStr != "OK" {
		return fmt.Errorf("router error: %s", r.ErrorStr)
	}
	return nil
}

type sessionTokenResponse struct {
	SessToken   string `json:"sess_token"`
	LockingTime int    `json:"lockingTime"`
}

type loginEntryResponse struct {
	LoginNeedRefresh int    `json:"login_need_refresh"`
	LockingTime      int    `json:"lockingTime"`
	LoginErrMsg      string `json:"loginErrMsg"`
}

// Login authenticates against the router. It must be called before any
// data-fetching method. Each call performs a fresh login.
func (c *Client) Login(ctx context.Context) error {
	slog.Debug("logging in", "host", c.baseURL, "username", c.username)

	sessionToken, err := c.getSessionToken(ctx)
	if err != nil {
		return fmt.Errorf("getting session token: %w", err)
	}

	loginToken, err := c.getLoginToken(ctx)
	if err != nil {
		return fmt.Errorf("getting login token: %w", err)
	}

	hash := sha256.Sum256([]byte(c.password + loginToken))
	passwordHash := hex.EncodeToString(hash[:])

	form := url.Values{
		"action":        {"login"},
		"Password":      {passwordHash},
		"Username":      {c.username},
		"_sessionTOKEN": {sessionToken},
	}

	body, err := c.post(ctx, "?_type=loginData&_tag=login_entry", form)
	if err != nil {
		return fmt.Errorf("posting login entry: %w", err)
	}

	var loginResp loginEntryResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return fmt.Errorf("parsing login entry response: %w", err)
	}

	if loginResp.LockingTime == -1 {
		return fmt.Errorf("router is locked: %s", loginResp.LoginErrMsg)
	}
	if loginResp.LockingTime > 0 {
		return fmt.Errorf("router is locked for %d seconds: too many login attempts", loginResp.LockingTime)
	}
	if loginResp.LoginErrMsg != "" && strings.Contains(strings.ToLower(loginResp.LoginErrMsg), "password") {
		return fmt.Errorf("login denied: %s", loginResp.LoginErrMsg)
	}

	slog.Debug("login succeeded", "host", c.baseURL)
	return nil
}

func (c *Client) getSessionToken(ctx context.Context) (string, error) {
	body, err := c.get(ctx, "?_type=loginData&_tag=login_entry")
	if err != nil {
		return "", err
	}

	var resp sessionTokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parsing session token response: %w", err)
	}
	if resp.LockingTime != 0 {
		return "", fmt.Errorf("router is locked for %d seconds", resp.LockingTime)
	}
	if resp.SessToken == "" {
		return "", fmt.Errorf("empty session token in response")
	}
	return resp.SessToken, nil
}

func (c *Client) getLoginToken(ctx context.Context) (string, error) {
	path := fmt.Sprintf("?_type=loginData&_tag=login_token&_=%d", c.nextGUID())
	body, err := c.get(ctx, path)
	if err != nil {
		return "", err
	}

	var resp ajaxResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parsing login token XML: %w", err)
	}
	if err := resp.checkError(); err != nil {
		return "", err
	}
	token := strings.TrimSpace(resp.Text)
	if token == "" {
		return "", fmt.Errorf("empty login token in response")
	}
	return token, nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/"+path, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *Client) post(ctx context.Context, path string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/"+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	slog.Debug("sending request", "method", req.Method, "url", req.URL.String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	slog.Debug("received response", "method", req.Method, "url", req.URL.String(), "status", resp.StatusCode, "bytes", len(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return body, nil
}
