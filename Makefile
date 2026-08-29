up:
	docker compose up --build -d
.PHONY: up

down:
	docker compose --profile "*" down --remove-orphans
.PHONY: down

clean:
	docker compose --profile "*" down -v --remove-orphans
.PHONY: clean

logs:
	docker compose logs -f
.PHONY: logs

ps:
	docker compose --profile "*" ps -a
.PHONY: ps

build:
	go build ./...
.PHONY: build

test:
	go test -v -race -timeout 15m ./...
.PHONY: test

lint:
	golangci-lint run
.PHONY: lint

fmt:
	golangci-lint fmt
.PHONY: fmt

generate:
	go tool oapi-codegen -config api/openapi/kitchen.cfg.yaml api/openapi/kitchen.yaml
	go tool oapi-codegen -config api/openapi/partner.cfg.yaml api/openapi/partner.yaml
.PHONY: generate

mocks:
	go tool mockery
.PHONY: mocks

tidy:
	go mod tidy
.PHONY: tidy

migrate-up:
	docker compose run --rm migrator
.PHONY: migrate-up

migrate-down:
	docker compose run --rm migrator down 1
.PHONY: migrate-down

migrate-down-all:
	docker compose run --rm migrator down -all
.PHONY: migrate-down-all

migrate-create:
	docker compose run --rm --entrypoint migrate migrator create -ext sql -dir /migrations -seq $(name)
.PHONY: migrate-create

migrate-version:
	docker compose run --rm migrator version
.PHONY: migrate-version

seed:
	docker compose run --rm seeder
.PHONY: seed

demo:
	go run ./cmd/demo
.PHONY: demo

diagrams:
	docker run --rm -v "$(CURDIR)/docs:/data" plantuml/plantuml -tsvg "/data/*.puml"
.PHONY: diagrams
