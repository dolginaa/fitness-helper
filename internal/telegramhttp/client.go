package telegramhttp

import (
	"net"
	"net/http"
	"time"
)

// NewClient returns an http.Client tuned for Telegram Bot API (longer TLS/dial timeouts).
// HTTPS_PROXY / HTTP_PROXY from the environment are still respected (DefaultTransport behavior).
func NewClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSHandshakeTimeout = 45 * time.Second
	tr.ResponseHeaderTimeout = 120 * time.Second
	tr.DialContext = (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	return &http.Client{
		Timeout:   120 * time.Second,
		Transport: tr,
	}
}
