.PHONY: build test test-postgres lint migrate-up run-api run-worker e2e-stub e2e-kind openapi-check example-60s

BIN_DIR := bin
DATABASE_URL ?= file:launchpad.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)
# Optional Postgres integration (CI job test-postgres). Example:
# LAUNCHPAD_TEST_DATABASE_URL=postgres://launchpad:launchpad@localhost:5432/launchpad?sslmode=disable
LAUNCHPAD_TEST_DATABASE_URL ?=

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/launchpad-api ./cmd/api
	go build -o $(BIN_DIR)/launchpad-worker ./cmd/worker
	go build -o $(BIN_DIR)/launchpad ./cmd/launchpad

test:
	go test ./...

# Requires LAUNCHPAD_TEST_DATABASE_URL (skipped when unset).
test-postgres:
	@if [ -z "$(LAUNCHPAD_TEST_DATABASE_URL)" ]; then echo "LAUNCHPAD_TEST_DATABASE_URL required"; exit 1; fi
	LAUNCHPAD_TEST_DATABASE_URL="$(LAUNCHPAD_TEST_DATABASE_URL)" go test ./internal/store/ -run 'TestPostgres' -count=1 -v

openapi-check:
	go test ./internal/api/ -run 'TestOpenAPI|TestHandlersRoutes' -count=1

lint:
	golangci-lint run ./... 2>/dev/null || go vet ./...

migrate-up:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/migrate

run-api: build
	LAUNCHPAD_DATABASE_URL="$(DATABASE_URL)" LAUNCHPAD_AUTO_MIGRATE=true $(BIN_DIR)/launchpad-api

run-worker: build
	LAUNCHPAD_DATABASE_URL="$(DATABASE_URL)" $(BIN_DIR)/launchpad-worker

e2e-stub:
	./scripts/e2e-stub.sh

e2e-kind:
	./scripts/e2e-kind.sh

example-60s:
	./scripts/example-60s-stub.sh
