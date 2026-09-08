set dotenv-load := true
set positional-arguments := false

postgres_container := "p3-relay-postgres"
postgres_image := "docker.io/library/postgres:18.3-alpine"

_default:
    @just --list

install:
    go mod download
    pnpm --dir web install --frozen-lockfile
    just infrastructure-init

api-develop:
    go run ./cmd/api

worker-develop:
    go run ./cmd/worker

receiver-develop:
    go run ./cmd/receiver

web-develop:
    pnpm --dir web dev

develop:
    #!/usr/bin/env bash
    set -euo pipefail
    processes=()
    cleanup() {
        trap - EXIT INT TERM
        kill "${processes[@]}" 2>/dev/null || true
        wait "${processes[@]}" 2>/dev/null || true
    }
    trap cleanup EXIT INT TERM
    go build -o /tmp/p3-relay-api ./cmd/api
    go build -o /tmp/p3-relay-worker ./cmd/worker
    go build -o /tmp/p3-relay-receiver ./cmd/receiver
    /tmp/p3-relay-api & processes+=("$!")
    /tmp/p3-relay-worker & processes+=("$!")
    /tmp/p3-relay-receiver & processes+=("$!")
    (cd web && exec node_modules/.bin/vite) & processes+=("$!")
    wait -n "${processes[@]}"

format: infrastructure-format
    gofmt -w cmd internal
    pnpm --dir web format

format-check:
    test -z "$(gofmt -l cmd internal)"
    pnpm --dir web format-check

lint:
    go vet ./...
    pnpm --dir web lint

typecheck:
    pnpm --dir web typecheck

test: infrastructure-test
    go test -race -cover ./...
    pnpm --dir web test

build:
    go build -o /tmp/p3-relay-api ./cmd/api
    go build -o /tmp/p3-relay-worker ./cmd/worker
    go build -o /tmp/p3-relay-receiver ./cmd/receiver
    pnpm --dir web build

check: format-check lint typecheck test build infrastructure-validate

# Requires an empty disposable PostgreSQL database named p3_relay_test; never use Railway.
sandbox-integration:
    go test -race -tags integration ./internal/sandbox -run TestSandboxIntegration -count=1

# Install the pinned Chromium runtime for browser validation.
browser-install:
    pnpm --dir web exec playwright install chromium

# Exercise visitor loading, errors, quotas, and expiry at mobile and desktop sizes.
browser-test:
    pnpm --dir web test-browser

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

# Run through aws-run sp. Publish a clean Git revision, never persistent local AWS credentials.
container-publish:
    #!/usr/bin/env bash
    set -euo pipefail
    git diff --quiet
    git diff --cached --quiet
    test -z "$(git ls-files --others --exclude-standard)"
    revision=$(git rev-parse --verify HEAD)
    repository=$(tofu -chdir=infra output -raw repository_url)
    account=$(aws sts get-caller-identity --region us-east-1 --query Account --output text)
    test "$repository" = "$account.dkr.ecr.us-east-1.amazonaws.com/p3-relay"
    podman build --label "org.opencontainers.image.revision=$revision" \
        --tag "$repository:$revision" --file Containerfile .
    directory=$(mktemp --directory "${XDG_RUNTIME_DIR:?}/p3-relay-publish.XXXXXX")
    trap 'rm --recursive --force "$directory"' EXIT
    aws ecr get-login-password --region us-east-1 | podman login \
        --authfile "$directory/auth.json" --username AWS --password-stdin "${repository%%/*}"
    podman push --retry=1 --authfile "$directory/auth.json" \
        --digestfile "$directory/digest" "$repository:$revision"
    # Podman's digest file need not end with a newline.
    digest=$(< "$directory/digest")
    [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]
    printf 'Published %s@%s from revision %s\n' "$repository" "$digest" "$revision"

# Initialize providers for offline validation; no cloud resources or state bucket are created.
infrastructure-init:
    tofu -chdir=infra init -backend=false -input=false
    tofu -chdir=infra/bootstrap init -backend=false -input=false

# Format project-owned OpenTofu configuration.
infrastructure-format:
    tofu fmt -recursive infra

# Validate both the state bootstrap and the application foundations.
infrastructure-validate:
    tofu fmt -check -recursive infra
    tofu -chdir=infra validate
    tofu -chdir=infra/bootstrap validate

# Mocked provider tests include invalid inputs and notification security boundaries.
infrastructure-test:
    tofu -chdir=infra test
    # OpenTofu 1.11 can report mocked cleanup errors with a zero exit status.
    test ! -e infra/errored_test.tfstate
    tofu -chdir=infra/bootstrap test
    test ! -e infra/bootstrap/errored_test.tfstate

# Run via aws-run sp with TF_VAR_aws_account_id set to the verified account.
infrastructure-bootstrap-plan:
    umask 077; tofu -chdir=infra/bootstrap plan -input=false -out=bootstrap.tfplan

[confirm("Apply the reviewed state-bucket bootstrap plan?")]
infrastructure-bootstrap-apply:
    umask 077; tofu -chdir=infra/bootstrap apply -input=false bootstrap.tfplan

# Requires the approved state bucket and a local infra/backend.hcl, containing no credentials.
infrastructure-connect:
    tofu -chdir=infra init -input=false -backend-config=backend.hcl

# Produce a saved plan for review, using the approved AWS wrapper.
infrastructure-plan:
    umask 077; tofu -chdir=infra plan -input=false -out=relay.tfplan

[confirm("Apply the reviewed Relay infrastructure plan?")]
infrastructure-apply:
    umask 077; tofu -chdir=infra apply -input=false relay.tfplan

# Destruction is always previewed separately and never runs from check.
infrastructure-destroy-plan:
    umask 077; tofu -chdir=infra plan -destroy -input=false -out=destroy.tfplan

[confirm("Apply the reviewed destruction plan for Relay infrastructure?")]
infrastructure-destroy:
    umask 077; tofu -chdir=infra apply -input=false destroy.tfplan
