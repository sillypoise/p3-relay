set dotenv-load := true
set positional-arguments := false

postgres_container := "p3-relay-postgres"
postgres_image := "docker.io/library/postgres:18.3-alpine"

_default:
    @just --list

install:
    go mod download
    pnpm --dir web install --frozen-lockfile

api-develop:
    go run ./cmd/api

worker-develop:
    go run ./cmd/worker

receiver-develop:
    go run ./cmd/receiver

web-develop:
    pnpm --dir web dev

develop:
    just --parallel api-develop worker-develop receiver-develop web-develop

format:
    gofmt -w cmd
    pnpm --dir web format

format-check:
    test -z "$(gofmt -l cmd)"
    pnpm --dir web format-check

lint:
    go vet ./...
    pnpm --dir web lint

typecheck:
    pnpm --dir web typecheck

test:
    go test -race -cover ./...
    pnpm --dir web test

build:
    go build -o /tmp/p3-relay-api ./cmd/api
    go build -o /tmp/p3-relay-worker ./cmd/worker
    go build -o /tmp/p3-relay-receiver ./cmd/receiver
    pnpm --dir web build

check: format-check lint typecheck test build

# Creation is separate so repeated starts never replace or mutate the database container.
database-create:
    podman create --name {{postgres_container}} \
        --publish 127.0.0.1:5432:5432 \
        --env POSTGRES_DB=relay \
        --env POSTGRES_USER=relay \
        --env POSTGRES_PASSWORD=relay_local \
        --volume p3-relay-postgres-data:/var/lib/postgresql/data \
        {{postgres_image}}

database-start:
    podman start {{postgres_container}}

database-migrate:
    go run ./cmd/migrate

database-stop:
    podman stop --time 10 {{postgres_container}}

database-logs:
    podman logs --tail 100 {{postgres_container}}

# Removing the container preserves its named data volume.
database-remove:
    podman rm {{postgres_container}}

container-build:
    podman build --tag localhost/p3-relay-api:development --file Containerfile .
