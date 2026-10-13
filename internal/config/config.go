package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

const (
	TransportBotAPI  = "botapi"
	TransportMTProto = "mtproto"
)

type Config struct {
	BotToken    string `mapstructure:"BOT_TOKEN"`
	DatabaseURL string `mapstructure:"DATABASE_URL"`
	LogLevel    string `mapstructure:"LOG_LEVEL"`
	// Timezone is an IANA name such as Europe/Moscow; empty means the system time zone.
	Timezone string `mapstructure:"TZ"`

	// TelegramTransport is how the bot reaches Telegram: the HTTP Bot API or MTProto.
	TelegramTransport string `mapstructure:"TELEGRAM_TRANSPORT"`
	// TelegramProxy is socks5://, http:// or https:// for the Bot API
	// and an MTProto proxy link (https://t.me/proxy?...) for MTProto.
	TelegramProxy string `mapstructure:"TELEGRAM_PROXY"`
	// TelegramAPIID and TelegramAPIHash identify the application for MTProto (my.telegram.org/apps).
	TelegramAPIID   int    `mapstructure:"TELEGRAM_API_ID"`
	TelegramAPIHash string `mapstructure:"TELEGRAM_API_HASH"`
	// TelegramSessionFile keeps the MTProto authorization between restarts.
	TelegramSessionFile string `mapstructure:"TELEGRAM_SESSION_FILE"`

	// WebAppAddr is the address the Mini App is served on (":8080"); empty turns it off.
	WebAppAddr string `mapstructure:"WEBAPP_ADDR"`
	// WebAppURL is the public HTTPS address of the Mini App that Telegram opens.
	WebAppURL string `mapstructure:"WEBAPP_URL"`
	// WebAppDevUserID lets a plain browser open the Mini App as this user. Local runs only.
	WebAppDevUserID int64 `mapstructure:"WEBAPP_DEV_USER_ID"`
}

func Load() (*Config, error) {
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("TELEGRAM_TRANSPORT", TransportBotAPI)
	v.SetDefault("TELEGRAM_SESSION_FILE", "data/telegram-session.json")

	for _, key := range []string{
		"BOT_TOKEN", "DATABASE_URL", "LOG_LEVEL", "TZ",
		"TELEGRAM_TRANSPORT", "TELEGRAM_PROXY", "TELEGRAM_API_ID", "TELEGRAM_API_HASH", "TELEGRAM_SESSION_FILE",
		"WEBAPP_ADDR", "WEBAPP_URL", "WEBAPP_DEV_USER_ID",
	} {
		_ = v.BindEnv(key)
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if c.BotToken == "" {
		return fmt.Errorf("BOT_TOKEN is required")
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	switch c.TelegramTransport {
	case TransportBotAPI:
	case TransportMTProto:
		if c.TelegramAPIID == 0 || c.TelegramAPIHash == "" {
			return fmt.Errorf("TELEGRAM_TRANSPORT=mtproto needs TELEGRAM_API_ID and TELEGRAM_API_HASH from https://my.telegram.org/apps")
		}
	default:
		return fmt.Errorf("TELEGRAM_TRANSPORT must be %q or %q", TransportBotAPI, TransportMTProto)
	}
	if c.WebAppURL != "" {
		if c.WebAppAddr == "" {
			return fmt.Errorf("WEBAPP_URL is set but WEBAPP_ADDR is not: the Mini App is not served")
		}
		// Telegram opens Mini Apps only over HTTPS.
		if u, err := url.Parse(c.WebAppURL); err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("WEBAPP_URL must be a public https:// address")
		}
		if c.WebAppDevUserID != 0 {
			return fmt.Errorf("WEBAPP_DEV_USER_ID lets anyone in without Telegram: do not set it together with WEBAPP_URL")
		}
	}
	return nil
}
