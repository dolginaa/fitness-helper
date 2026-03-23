# Fitness Helper — Telegram-бот трекинга тренировок

Бот на **Go 1.22+** с чистой архитектурой: `domain` → `usecase` → `repository` (PostgreSQL, sqlc/pgx) → `handlers` / `keyboards`. Состояние диалога хранится в таблице `user_workout_state` (JSONB).

## Требования

- Go 1.22+
- Docker и Docker Compose (для PostgreSQL и миграций)
- [sqlc](https://docs.sqlc.dev/en/latest/overview/install.html) — только если меняете SQL-запросы (`make sqlc`)

## Быстрый старт (локально)

1. Скопируйте окружение и задайте токен бота от [@BotFather](https://t.me/BotFather). Файл `.env` подхватывается при старте (`godotenv`).

   ```bash
   cp .env.example .env
   # Отредактируйте .env: BOT_TOKEN, при необходимости DATABASE_URL
   ```

2. Поднимите Postgres и примените миграции:

   ```bash
   make docker-up
   make migrate-up
   ```

3. Запустите бота на хосте (БД на `localhost:5432`):

   ```bash
   make run
   ```

Команды бота: `/start`, `/startnew`, `/history`, `/last`. Кнопка «Новая тренировка» дублирует `/startnew`.

### Ошибка `TLS handshake timeout` / нет связи с Telegram

Обычно это сеть: блокировка `api.telegram.org`, корпоративный файрвол, нестабильный Wi‑Fi. Проверьте `curl -I https://api.telegram.org`. Попробуйте другую сеть или VPN. При необходимости задайте прокси: `HTTPS_PROXY=https://...` (и при необходимости `NO_PROXY=localhost,127.0.0.1` для Postgres). Токен бота в логах теперь маскируется; при утечке токена перевыпустите его в BotFather (`/revoke`).

## Make-цели

| Цель          | Описание                                      |
|---------------|-----------------------------------------------|
| `make dev`    | Postgres + миграции + `go mod tidy`           |
| `make run`    | `go run ./cmd/bot`                            |
| `make test`   | `go test ./...`                               |
| `make build`  | Сборка бинаря в `bin/bot`                     |
| `make sqlc`   | Генерация кода из `sql/queries` (нужен sqlc) |
| `make migrate-up` / `migrate-down` | Миграции через Docker (compose profile `migrate`) |

## Запуск в Docker Compose (бот + БД)

После первого `make migrate-up` (схема и seed в БД):

```bash
cp .env.example .env
# Укажите BOT_TOKEN в .env
docker compose up -d --build bot
```

Сервис `bot` подключается к `postgres` по внутреннему URL; `DATABASE_URL` в compose переопределён для контейнера.

## Продакшен на VPS

1. Установите Docker (или Go + systemd).
2. Склонируйте репозиторий, создайте `.env` с `BOT_TOKEN` и при необходимости `DATABASE_URL`.
3. Выполните миграции, затем поднимите сервисы: `docker compose up -d --build bot`.
4. Альтернатива без Docker для бота: соберите `make build`, перенесите `bin/bot` на сервер, задайте переменные окружения и запустите unit в systemd с `Restart=always`.

Переменные: `BOT_TOKEN` (обязательно), `DATABASE_URL` (обязательно при локальном запуске), `LOG_LEVEL` (`debug`, `info`, `warn`, `error`).

## Разработка SQL

После правок в `sql/queries/*.sql` или `sql/schema.sql` выполните `make sqlc`. Миграции в `migrations/` должны соответствовать схеме для sqlc (`sql/schema.sql`).

## Структура

- `cmd/bot` — точка входа, конфиг (Viper), пул БД, polling Telegram, graceful shutdown.
- `internal/domain` — сущности и порты репозиториев.
- `internal/usecase` — сценарии тренировки и `/last`.
- `internal/repository/postgres` — реализация портов; `internal/repository/sqlcgen` — код sqlc.
- `internal/handlers`, `internal/keyboards` — Telegram.
- `migrations` — схема и seed упражнений.
- `pkg/logger` — JSON slog.

## Тесты

```bash
make test
```

Юнит-тесты use case с in-memory заглушками; в `handlers` — smoke-тест callback без `Message`.
