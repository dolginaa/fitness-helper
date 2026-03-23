.PHONY: dev docker-up docker-down migrate-up migrate-down sqlc run test build tidy

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

build:
	go build -o bin/bot ./cmd/bot

tidy:
	go mod tidy
