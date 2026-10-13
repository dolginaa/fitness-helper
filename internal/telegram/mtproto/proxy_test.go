package mtproto

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/adolgina/fitness-helper/internal/chat"
)

// A FakeTLS secret: the "ee" tag, 16 key bytes and the cloak host "www.google.com".
const testSecret = "ee000102030405060708090a0b0c0d0e0f7777772e676f6f676c652e636f6d"

func TestParseProxy(t *testing.T) {
	t.Parallel()
	if p, err := ParseProxy(""); p != nil || err != nil {
		t.Fatalf("empty link means a direct connection, got %v, %v", p, err)
	}
	for _, link := range []string{
		"https://t.me/proxy?server=203.0.113.7&port=443&secret=" + testSecret,
		"tg://proxy?server=203.0.113.7&port=443&secret=" + testSecret,
	} {
		p, err := ParseProxy(link)
		if err != nil {
			t.Fatalf("%q: %v", link, err)
		}
		if p.Addr != "203.0.113.7:443" {
			t.Fatalf("%q: unexpected address %q", link, p.Addr)
		}
		if len(p.Secret) != 31 || p.Secret[0] != 0xee || !bytes.HasSuffix(p.Secret, []byte("www.google.com")) {
			t.Fatalf("%q: unexpected secret %x", link, p.Secret)
		}
	}
	// Apps also share secrets as URL-safe base64.
	p, err := ParseProxy("https://t.me/proxy?server=proxy.example.com&port=8443&secret=7gABAgMEBQYHCAkKCwwNDg8")
	if err != nil || p.Addr != "proxy.example.com:8443" || len(p.Secret) != 17 {
		t.Fatalf("base64 secret: %+v, %v", p, err)
	}
}

func TestParseProxyRejects(t *testing.T) {
	t.Parallel()
	for _, link := range []string{
		"socks5://127.0.0.1:1080",
		"https://t.me/proxy?server=203.0.113.7&secret=" + testSecret,
		"https://t.me/proxy?server=203.0.113.7&port=443",
		"https://t.me/proxy?server=203.0.113.7&port=443&secret=not a secret!",
	} {
		_, err := ParseProxy(link)
		if err == nil {
			t.Fatalf("%q must be rejected", link)
		}
		if strings.Contains(err.Error(), testSecret) {
			t.Fatalf("the error leaks the secret: %v", err)
		}
	}
}

func TestMarkup(t *testing.T) {
	t.Parallel()
	m, ok := markup(chat.Keyboard{
		{{Text: "Жим ногами", Data: "ex:16"}},
		{{Text: "Назад", Data: "next"}, {Text: "Приложение", WebAppURL: "https://fit.example.com/"}},
	}).(*tg.ReplyInlineMarkup)
	if !ok || len(m.Rows) != 2 || len(m.Rows[1].Buttons) != 2 {
		t.Fatalf("unexpected markup: %+v", m)
	}
	cb, ok := m.Rows[0].Buttons[0].Type.(*tg.InlineButtonTypeCallback)
	if !ok || string(cb.Data) != "ex:16" || m.Rows[0].Buttons[0].Text != "Жим ногами" {
		t.Fatalf("unexpected callback button: %+v", m.Rows[0].Buttons[0])
	}
	web, ok := m.Rows[1].Buttons[1].Type.(*tg.InlineButtonTypeWebView)
	if !ok || web.URL != "https://fit.example.com/" {
		t.Fatalf("unexpected Mini App button: %+v", m.Rows[1].Buttons[1])
	}
}
