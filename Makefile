up:
	docker compose up --build -d
.PHONY: up

down:
	docker compose down -v --remove-orphans
.PHONY: down

logs:
	docker compose logs -f
.PHONY: logs

ps:
	docker compose ps
.PHONY: ps

build:
	go build ./...
.PHONY: build

test:
	go test -v -race ./...
.PHONY: test

lint:
	golangci-lint run
.PHONY: lint

fmt:
	golangci-lint fmt
.PHONY: fmt

generate:
	go generate ./...
.PHONY: generate

tidy:
	go mod tidy
.PHONY: tidy

migrate-up:
	docker compose --profile migrations run --rm migrator
.PHONY: migrate-up

migrate-down:
	docker compose --profile migrations run --rm migrator down 1
.PHONY: migrate-down

migrate-down-all:
	docker compose --profile migrations run --rm migrator down -all
.PHONY: migrate-down-all

migrate-create:
	docker compose --profile migrations run --rm --entrypoint migrate migrator create -ext sql -dir /migrations -seq $(name)
.PHONY: migrate-create

migrate-version:
	docker compose --profile migrations run --rm migrator version
.PHONY: migrate-version
