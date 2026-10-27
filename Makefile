DB_URL      ?= postgres://taskbot:taskbot@127.0.0.1:5432/taskbot?sslmode=disable
TEST_DB_URL ?= postgres://taskbot:taskbot@127.0.0.1:5432/taskbot_test?sslmode=disable

.PHONY: run build test test-db vet fmt db-up db-down db-reset db-psql

run:
	go run ./cmd/bot

build:
	go build -o bin/bot ./cmd/bot

test:
	go test -race ./...

# Интеграционные тесты репозитория: нужен поднятый Postgres.
test-db:
	TEST_DATABASE_URL="$(TEST_DB_URL)" go test -race -count=1 ./internal/storage/postgres

vet:
	go vet ./...

fmt:
	gofmt -l -w .

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-reset:
	docker compose down -v
	docker compose up -d postgres

db-psql:
	docker compose exec postgres psql -U taskbot -d taskbot
