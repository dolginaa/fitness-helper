package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	BotToken    string `mapstructure:"BOT_TOKEN"`
	DatabaseURL string `mapstructure:"DATABASE_URL"`
	LogLevel    string `mapstructure:"LOG_LEVEL"`
}

func Load() (*Config, error) {
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("LOG_LEVEL", "info")

	_ = v.BindEnv("BOT_TOKEN")
	_ = v.BindEnv("DATABASE_URL")
	_ = v.BindEnv("LOG_LEVEL")

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if c.BotToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is required")
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	return &c, nil
}
