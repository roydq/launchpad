.PHONY: build test lint migrate-up run-api run-worker

BIN_DIR := bin
DATABASE_URL ?= file:launchpad.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/launchpad-api ./cmd/api
	go build -o $(BIN_DIR)/launchpad-worker ./cmd/worker
	go build -o $(BIN_DIR)/launchpad ./cmd/launchpad

test:
	go test ./...

lint:
	golangci-lint run ./... 2>/dev/null || go vet ./...

migrate-up:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/migrate

run-api: build
	LAUNCHPAD_DATABASE_URL="$(DATABASE_URL)" LAUNCHPAD_AUTO_MIGRATE=true $(BIN_DIR)/launchpad-api

run-worker: build
	LAUNCHPAD_DATABASE_URL="$(DATABASE_URL)" $(BIN_DIR)/launchpad-worker