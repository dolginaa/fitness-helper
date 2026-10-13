.PHONY: dev docker-up docker-down migrate-up migrate-down sqlc run test test-integration webapp-preview exercises-report exercises-dedupe build tidy

DATABASE_URL ?= postgres://fitness:fitness@localhost:5432/fitness?sslmode=disable

# Поднять Postgres, применить миграции (docker compose profile migrate), go mod tidy
dev: docker-up migrate-up tidy
	@echo "Ready. Set BOT_TOKEN in .env and run: make run (or: docker compose up -d bot)"

docker-up:
	docker-compose up -d postgres

docker-down:
	docker-compose down

migrate-up:
	docker-compose --profile migrate run --rm migrate

migrate-down:
	docker-compose --profile migrate run --rm migrate -path=/migrations -database=postgres://fitness:fitness@postgres:5432/fitness?sslmode=disable down 1

sqlc:
	sqlc generate

run:
	go run ./cmd/bot

test:
	go test ./...

# Тесты репозиториев на настоящем Postgres (во временной схеме, данные не трогает)
test-integration:
	TEST_DATABASE_URL=$(DATABASE_URL) go test -count=1 ./internal/repository/postgres/

# Mini App в браузере без бота и базы: данные из export.json, в памяти (ничего не сохраняется)
webapp-preview:
	@echo "Откройте http://127.0.0.1:8099/  (остановить: Ctrl+C)"
	@WEBAPP_PREVIEW=127.0.0.1:8099 CHART_EXPORT=$(CURDIR)/export.json go test ./internal/webapp -run TestPreview -timeout 0 -count=1

# Чистка списка упражнений. report и dedupe без APPLY=1 ничего не меняют.
exercises-report:
	go run ./cmd/exercises report

exercises-dedupe:
	go run ./cmd/exercises dedupe $(if $(APPLY),-apply)

build:
	go build -o bin/bot ./cmd/bot

tidy:
	go mod tidy
