.PHONY: up down logs migrate-up migrate-down run test test-db vet sqlc web-install web-dev web-build

# The suite TRUNCATEs users CASCADE between tests, which would wipe the dev
# database. Point it at a separate one unless the caller already chose a target.
TEST_DATABASE_URL ?= postgres://pulse:pulse@127.0.0.1:5433/pulse_test?sslmode=disable

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f db

migrate-up:
	set -a && [ -f .env ] && . ./.env && set +a && migrate -path migrations -database "$$DATABASE_URL" up

migrate-down:
	set -a && [ -f .env ] && . ./.env && set +a && migrate -path migrations -database "$$DATABASE_URL" down 1

run:
	set -a && [ -f .env ] && . ./.env && set +a && go run ./cmd/server

# Creates the test database if it is missing. Migrations are applied by the
# test harness itself, so this only has to make the database exist.
test-db:
	@docker compose exec -T db psql -U pulse -d postgres -tAc \
		"SELECT 1 FROM pg_database WHERE datname='pulse_test'" | grep -q 1 \
		|| docker compose exec -T db psql -U pulse -d postgres -c "CREATE DATABASE pulse_test"

test: test-db
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test ./...

vet:
	go vet ./...
sqlc:
	sqlc generate
web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build
