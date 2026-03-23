package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/adolgina/fitness-helper/internal/config"
	"github.com/adolgina/fitness-helper/internal/telegramhttp"
	"github.com/joho/godotenv"
	"github.com/adolgina/fitness-helper/internal/handlers"
	"github.com/adolgina/fitness-helper/internal/repository/postgres"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
	"github.com/adolgina/fitness-helper/internal/usecase"
	"github.com/adolgina/fitness-helper/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	log := logger.New(cfg.LogLevel)

	ctx := context.Background()
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
	)

	bot, err := tgbotapi.NewBotAPIWithClient(cfg.BotToken, tgbotapi.APIEndpoint, telegramhttp.NewClient())
	if err != nil {
		log.Error("telegram bot", "err", telegramhttp.RedactToken(err.Error(), cfg.BotToken))
		os.Exit(1)
	}
	bot.Debug = false
	log.Info("authorized", "user", bot.Self.UserName)

	h := &handlers.Bot{Svc: svc, Log: log}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		select {
		case <-runCtx.Done():
			bot.StopReceivingUpdates()
			log.Info("shutdown")
			return
		case upd, ok := <-updates:
			if !ok {
				return
			}
			if runCtx.Err() != nil {
				return
			}
			h.HandleUpdate(runCtx, bot, upd)
		}
	}
}
