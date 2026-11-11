SERVICE ?= chat

-include .env
ifneq ($(filter run migrate-up migrate-down,$(MAKECMDGOALS)),)
-include services/$(SERVICE)/.env
endif
export

GOOSE_DRIVER ?= postgres
GOOSE_DBSTRING ?= $(DATABASE_URL)
GOOSE_MIGRATION_DIR ?= services/$(SERVICE)/migrations

chat auth notifications:
	$(MAKE) run SERVICE=$@

run:
	cd services/$(SERVICE) && go run ./cmd/$(SERVICE)

build:
	go build ./...

vet:
	go vet ./...

proto:
	protoc --go_out=. --go_opt=module=chatflow --go-grpc_out=. --go-grpc_opt=module=chatflow proto/auth_service/v1/auth_service.proto

test:
	go test -race ./...

postgres:
	docker compose up -d postgres_chat postgres_auth postgres_notifications

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down -v

migrate-up:
	go tool goose -dir "$(GOOSE_MIGRATION_DIR)" up

migrate-down:
	go tool goose -dir "$(GOOSE_MIGRATION_DIR)" down

.PHONY: chat auth notifications run build vet proto test postgres compose-up compose-down migrate-up migrate-down
