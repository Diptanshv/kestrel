.PHONY: up down logs migrate-up migrate-down run test vet

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f db

migrate-up:
	migrate -path migrations -database "$$DATABASE_URL" up

migrate-down:
	migrate -path migrations -database "$$DATABASE_URL" down 1

run:
	set -a && [ -f .env ] && . ./.env && set +a && go run ./cmd/server

test:
	go test ./...

vet:
	go vet ./...
sqlc:
	sqlc generate