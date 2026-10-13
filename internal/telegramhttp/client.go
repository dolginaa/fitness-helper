package telegramhttp

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// NewClient returns an http.Client tuned for Telegram Bot API (longer TLS/dial timeouts).
// With a proxy every request goes through it; without one HTTPS_PROXY / HTTP_PROXY
// from the environment are still respected (DefaultTransport behavior).
func NewClient(proxy *url.URL) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSHandshakeTimeout = 45 * time.Second
	tr.ResponseHeaderTimeout = 120 * time.Second
	tr.DialContext = (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	if proxy != nil {
		tr.Proxy = http.ProxyURL(proxy)
	}
	return &http.Client{
		Timeout:   120 * time.Second,
		Transport: tr,
	}
}

// ParseProxy reads a proxy address for the Bot API: socks5://user:pass@host:port or http(s)://host:port.
// An empty string means no explicit proxy.
func ParseProxy(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		// url.Parse errors quote the input, which may carry a password.
		return nil, fmt.Errorf("proxy address is not a valid URL")
	}
	if u.Scheme == "tg" || u.Host == "t.me" || u.Host == "telegram.me" {
		return nil, errMTProtoProxy
	}
	if u.Host == "" {
		return nil, fmt.Errorf("proxy address must look like socks5://host:port")
	}
	switch u.Scheme {
	case "http", "https", "socks5":
		return u, nil
	case "socks5h":
		// net/http resolves host names on the proxy side for socks5 as well.
		u.Scheme = "socks5"
		return u, nil
	}
	return nil, fmt.Errorf("unsupported proxy scheme %q: use socks5, http or https", u.Scheme)
}

var errMTProtoProxy = fmt.Errorf("this is an MTProto proxy: the Bot API (HTTPS) cannot go through it; " +
	"use a SOCKS5/HTTP proxy or set TELEGRAM_TRANSPORT=mtproto")
