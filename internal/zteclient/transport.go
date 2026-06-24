package zteclient

import (
	"crypto/tls"
	"net/http"
)

// insecureTransport returns an http.Transport that skips TLS certificate
// verification. The H3600P serves its web UI over HTTPS with a
// self-signed certificate, so this is required to connect at all.
func insecureTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // router uses a self-signed cert
	}
}
