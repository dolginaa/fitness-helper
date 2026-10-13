package mtproto

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Proxy is an MTProto proxy (MTProxy), the same kind Telegram apps connect through.
type Proxy struct {
	Addr   string
	Secret []byte
}

// ParseProxy reads a proxy link as shared by Telegram apps:
// https://t.me/proxy?server=HOST&port=PORT&secret=SECRET or tg://proxy?...
// An empty string means a direct connection.
func ParseProxy(raw string) (*Proxy, error) {
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		// url.Parse errors quote the input, which carries the proxy secret.
		return nil, errors.New("proxy link is not a valid URL")
	}
	isLink := (u.Scheme == "tg" && u.Host == "proxy") ||
		((u.Host == "t.me" || u.Host == "telegram.me") && strings.Trim(u.Path, "/") == "proxy")
	if !isLink {
		return nil, errors.New("the MTProto transport needs an MTProto proxy link " +
			"(https://t.me/proxy?server=...&port=...&secret=...); SOCKS5/HTTP proxies work with TELEGRAM_TRANSPORT=botapi")
	}
	q := u.Query()
	server, port := q.Get("server"), q.Get("port")
	if server == "" || port == "" {
		return nil, errors.New("proxy link must have server and port")
	}
	secret, err := decodeSecret(q.Get("secret"))
	if err != nil {
		return nil, err
	}
	return &Proxy{Addr: net.JoinHostPort(server, port), Secret: secret}, nil
}

// decodeSecret accepts both forms apps use: hex and URL-safe base64.
func decodeSecret(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New("proxy link must have a secret")
	}
	if b, err := hex.DecodeString(s); err == nil {
		return b, nil
	}
	s = strings.TrimRight(s, "=")
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.RawStdEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("proxy secret is neither hex nor base64")
}
