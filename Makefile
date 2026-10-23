-include .env
export

GOOSE_DRIVER ?= postgres
GOOSE_DBSTRING ?= $(DATABASE_URL)
GOOSE_MIGRATION_DIR ?= migrations

chat:
	go run cmd/chat/main.go

test:
	go test -race ./...

postgres:
	docker compose up -d postgres

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down -v

migrate-up:
	go tool goose up

migrate-down:
	go tool goose down

.PHONY: chat test postgres compose-up compose-down migrate-up migrate-down
