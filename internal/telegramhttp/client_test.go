package telegramhttp

import (
	"errors"
	"net/http"
	"testing"
)

func TestParseProxy(t *testing.T) {
	t.Parallel()
	if u, err := ParseProxy(""); u != nil || err != nil {
		t.Fatalf("empty address means no proxy, got %v, %v", u, err)
	}
	for raw, want := range map[string]string{
		"socks5://user:pass@127.0.0.1:1080": "socks5://user:pass@127.0.0.1:1080",
		"socks5h://proxy.example.com:1080":  "socks5://proxy.example.com:1080",
		"http://10.0.0.1:3128":              "http://10.0.0.1:3128",
	} {
		u, err := ParseProxy(raw)
		if err != nil || u.String() != want {
			t.Fatalf("%q: got %v, %v", raw, u, err)
		}
	}
	for _, raw := range []string{"127.0.0.1:1080", "ftp://host:21", "socks5://"} {
		if _, err := ParseProxy(raw); err == nil {
			t.Fatalf("%q must be rejected", raw)
		}
	}
	for _, raw := range []string{"https://t.me/proxy?server=1.2.3.4&port=443&secret=ee00", "tg://proxy?server=1.2.3.4&port=443&secret=ee00"} {
		if _, err := ParseProxy(raw); !errors.Is(err, errMTProtoProxy) {
			t.Fatalf("%q: an MTProto proxy must be explained, got %v", raw, err)
		}
	}
}

func TestNewClientUsesProxy(t *testing.T) {
	t.Parallel()
	proxy, _ := ParseProxy("socks5://127.0.0.1:1080")
	tr := NewClient(proxy).Transport.(*http.Transport)
	req, _ := http.NewRequest(http.MethodGet, "https://api.telegram.org/", nil)
	got, err := tr.Proxy(req)
	if err != nil || got.String() != "socks5://127.0.0.1:1080" {
		t.Fatalf("requests must go through the proxy, got %v, %v", got, err)
	}
}
