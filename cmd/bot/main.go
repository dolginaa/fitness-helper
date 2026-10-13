package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/adolgina/fitness-helper/internal/charts"
	"github.com/adolgina/fitness-helper/internal/chat"
	"github.com/adolgina/fitness-helper/internal/config"
	"github.com/adolgina/fitness-helper/internal/handlers"
	"github.com/adolgina/fitness-helper/internal/repository/postgres"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
	"github.com/adolgina/fitness-helper/internal/telegram/botapi"
	"github.com/adolgina/fitness-helper/internal/telegram/mtproto"
	"github.com/adolgina/fitness-helper/internal/telegramhttp"
	"github.com/adolgina/fitness-helper/internal/usecase"
	"github.com/adolgina/fitness-helper/internal/webapp"
	"github.com/adolgina/fitness-helper/pkg/logger"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// transport is a way to reach Telegram: it sends messages and delivers updates.
type transport interface {
	chat.Messenger
	// Run delivers updates to handle until ctx is cancelled; ready is called once the bot can send.
	Run(ctx context.Context, handle chat.Handler, ready func(username string)) error
}

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	log := logger.New(cfg.LogLevel)

	// Dates are shown and parsed in the user's time zone.
	if cfg.Timezone != "" {
		loc, err := time.LoadLocation(cfg.Timezone)
		if err != nil {
			log.Error("timezone", "tz", cfg.Timezone, "err", err)
			os.Exit(1)
		}
		time.Local = loc
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	q := sqlcgen.New(pool)
	svc := usecase.NewService(
		postgres.NewCatalog(q),
		postgres.NewWorkout(pool, q),
		postgres.NewSession(q),
		postgres.NewAnalytics(q),
		postgres.NewBackup(pool, q),
		postgres.NewSettings(q),
	)
	chartRenderer := charts.New()

	tg, err := newTransport(cfg, log)
	if err != nil {
		log.Error("telegram", "transport", cfg.TelegramTransport, "err", err)
		os.Exit(1)
	}
	bot := handlers.NewBot(svc, log, tg, chartRenderer, cfg.WebAppURL)

	failed := make(chan error, 2)
	if cfg.WebAppAddr != "" {
		web := webapp.New(svc, chartRenderer, log, webapp.Config{
			Addr:      cfg.WebAppAddr,
			BotToken:  cfg.BotToken,
			DevUserID: cfg.WebAppDevUserID,
		})
		if cfg.WebAppDevUserID != 0 {
			log.Warn("mini app is open without Telegram authorization (WEBAPP_DEV_USER_ID)", "user", cfg.WebAppDevUserID)
		}
		log.Info("mini app", "addr", cfg.WebAppAddr, "url", cfg.WebAppURL)
		go func() {
			if err := web.Run(ctx); err != nil {
				failed <- fmt.Errorf("mini app: %w", err)
			}
		}()
	}

	go func() {
		err := tg.Run(ctx, bot.HandleUpdate, func(username string) {
			log.Info("authorized", "user", username, "transport", cfg.TelegramTransport)
			if err := bot.Setup(ctx); err != nil {
				log.Warn("bot setup", "err", err)
			}
			bot.RunWeeklyBackup(ctx)
		})
		if err != nil {
			failed <- fmt.Errorf("telegram: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown")
	case err := <-failed:
		log.Error("stopped", "err", err)
		os.Exit(1)
	}
}

func newTransport(cfg *config.Config, log *slog.Logger) (transport, error) {
	if cfg.TelegramTransport == config.TransportMTProto {
		proxy, err := mtproto.ParseProxy(cfg.TelegramProxy)
		if err != nil {
			return nil, err
		}
		client, err := mtproto.New(mtproto.Config{
			APIID:       cfg.TelegramAPIID,
			APIHash:     cfg.TelegramAPIHash,
			Token:       cfg.BotToken,
			SessionFile: cfg.TelegramSessionFile,
			Proxy:       proxy,
		}, log)
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	proxy, err := telegramhttp.ParseProxy(cfg.TelegramProxy)
	if err != nil {
		return nil, err
	}
	client, err := botapi.New(cfg.BotToken, telegramhttp.NewClient(proxy), log)
	if err != nil {
		return nil, err
	}
	return client, nil
}
