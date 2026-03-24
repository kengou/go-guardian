package tools

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// httpClientTimeout is the total timeout for a single HTTP request.
	httpClientTimeout = 30 * time.Second

	// maxResponseBody caps bytes read from any HTTP response (64 MiB).
	maxResponseBody = 64 << 20
)

// secureHTTPClient is the single hardened HTTP client for all outbound API calls.
//   - 30-second total timeout (fixes FINDING-01)
//   - Never follows redirects — returns 3xx as-is (fixes FINDING-02)
//   - TLS 1.2 minimum (fixes FINDING-05)
var secureHTTPClient = &http.Client{
	Timeout: httpClientTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Return the redirect response directly. This prevents credential
		// leakage: custom headers (apiKey, Authorization) are NOT forwarded
		// to the redirect target.
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   5,
	},
}

// requireHTTPS validates that rawURL uses the https:// scheme.
// Loopback URLs (http://127.0.0.1, http://localhost) are allowed
// for test servers spun up by httptest.NewServer.
func requireHTTPS(rawURL string) error {
	if strings.HasPrefix(rawURL, "https://") {
		return nil
	}
	if strings.HasPrefix(rawURL, "http://127.0.0.1") ||
		strings.HasPrefix(rawURL, "http://localhost") {
		return nil
	}
	return fmt.Errorf("URL scheme must be https, got: %s", rawURL)
}

// limitedBody wraps a response body with io.LimitReader (fixes FINDING-09).
// Callers must still close the returned ReadCloser.
func limitedBody(body io.ReadCloser) io.ReadCloser {
	return &limitedReadCloser{
		Reader: io.LimitReader(body, maxResponseBody),
		Closer: body,
	}
}

type limitedReadCloser struct {
	io.Reader
	io.Closer
}
