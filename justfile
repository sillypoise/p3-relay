set dotenv-load := true
set positional-arguments := false

postgres_container := "p3-relay-postgres"
postgres_image := "docker.io/library/postgres:18.3-alpine"

_default:
    @just --list

install:
    go mod download
    pnpm --dir web install --frozen-lockfile
    pnpm --dir .railway install --frozen-lockfile --ignore-scripts
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
    gofmt -w cmd internal gateway
    pnpm --dir web format
    web/node_modules/.bin/oxfmt .railway/*.ts

format-check:
    test -z "$(gofmt -l cmd internal gateway)"
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
    go build -o /tmp/p3-relay-gateway ./gateway
    pnpm --dir web build

check: format-check lint typecheck test build infrastructure-validate gateway-check

# Local gateway ownership checks; no Railway authentication or API calls.
gateway-check:
    web/node_modules/.bin/oxfmt --check .railway/*.ts
    web/node_modules/.bin/oxlint --deny-warnings .railway/*.ts
    pnpm --dir web exec tsc --project ../.railway/tsconfig.json
    node --test .railway/railway.test.ts

# Gateway has its own allowlisted context; no operator credentials enter the image.
gateway-container-build:
    podman build --file gateway/Containerfile --tag localhost/p3-relay-gateway:development gateway

# Disposable local PostgreSQL and gateway only; never contacts Railway or uses a database URL.
gateway-container-test: gateway-container-build
    go vet -tags gatewayintegration ./gateway
    go test -race -tags gatewayintegration ./gateway -count=1 -timeout=3m -v

# Pin the official Linux/x86_64 CLI locally; 4.11 uses removed Railway API fields.
gateway-tool-install:
    #!/usr/bin/env bash
    set -euo pipefail
    test "$(uname --kernel-name --machine)" = "Linux x86_64"
    directory=$(mktemp --directory)
    trap 'rm --recursive --force "$directory"' EXIT
    base=https://github.com/railwayapp/cli/releases/download/v5.49.6
    curl --fail --location --silent --show-error --connect-timeout 5 --max-time 90 \
        --max-filesize 7766908 \
        "$base/railway-v5.49.6-x86_64-unknown-linux-musl.tar.gz" --output "$directory/cli.tar.gz"
    digest=39c89cc07203331392ae5d65f3a21bc49a7219cefdb69948245aca638a33856e
    printf '%s  %s\n' "$digest" "$directory/cli.tar.gz" | sha256sum --check
    tar --extract --file "$directory/cli.tar.gz" --directory "$directory" \
        --no-same-owner --no-same-permissions railway
    mkdir --parents .tools
    install --mode=0755 "$directory/railway" .tools/railway

# Native Railway IaC exception: the community provider cannot use project-scoped authentication.
gateway-plan:
    # SDK 3.11 checks $_ as the CLI path; unset the shell's inherited just path, not the check.
    umask 077; env --unset=_ PATH="$PWD/.tools:$PATH" \
        railway config plan --out .railway/gateway-plan.json

[confirm("Apply only the reviewed Relay gateway plan? No destructive changes are allowed.")]
gateway-apply:
    env --unset=_ PATH="$PWD/.tools:$PATH" \
        railway config apply --plan .railway/gateway-plan.json --yes

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
container-publish $component="application":
    #!/usr/bin/env bash
    set -euo pipefail
    case "$component" in
        application) context=.; definition=Containerfile; prefix= ;;
        gateway) context=gateway; definition=gateway/Containerfile; prefix=gateway- ;;
        *) printf 'Expected application or gateway component\n' >&2; exit 2 ;;
    esac
    git diff --quiet
    git diff --cached --quiet
    test -z "$(git ls-files --others --exclude-standard)"
    revision=$(git rev-parse --verify HEAD)
    tag="$prefix$revision"
    repository=$(tofu -chdir=infra output -raw repository_url)
    account=$(aws sts get-caller-identity --region us-east-1 --query Account --output text)
    test "$repository" = "$account.dkr.ecr.us-east-1.amazonaws.com/p3-relay"
    # Refresh runtime security packages rather than reusing cached package-install layers.
    podman build --pull=always --no-cache --label "org.opencontainers.image.revision=$revision" \
        --tag "$repository:$tag" --file "$definition" "$context"
    directory=$(mktemp --directory "${XDG_RUNTIME_DIR:?}/p3-relay-publish.XXXXXX")
    trap 'rm --recursive --force "$directory"' EXIT
    aws ecr get-login-password --region us-east-1 | podman login \
        --authfile "$directory/auth.json" --username AWS --password-stdin "${repository%%/*}"
    podman push --retry=1 --authfile "$directory/auth.json" \
        --digestfile "$directory/digest" "$repository:$tag"
    # Podman's digest file need not end with a newline.
    digest=$(< "$directory/digest")
    [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]
    printf 'Published %s@%s from revision %s\n' "$repository" "$digest" "$revision"

# Scan-matched public distribution; run through aws-run sp after private gateway publication.
gateway-container-release $revision $digest:
    #!/usr/bin/env bash
    set -euo pipefail
    [[ "$revision" =~ ^[0-9a-f]{40}$ ]]
    [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]
    private=$(tofu -chdir=infra output -raw repository_url)
    public=$(tofu -chdir=infra output -raw gateway_repository_url)
    account=$(aws sts get-caller-identity --region us-east-1 --query Account --output text)
    test "$private" = "$account.dkr.ecr.us-east-1.amazonaws.com/p3-relay"
    [[ "$public" =~ ^public\.ecr\.aws/[a-z0-9]+/p3-relay-gateway$ ]]
    actual=$(aws ecr-public describe-repositories --region us-east-1 \
        --repository-names p3-relay-gateway --query 'repositories[0].repositoryUri' --output text)
    test "$actual" = "$public"
    actual=$(aws ecr describe-images --region us-east-1 --repository-name p3-relay \
        --image-ids "imageTag=gateway-$revision" --query 'imageDetails[0].imageDigest' --output text)
    test "$actual" = "$digest"
    aws ecr wait image-scan-complete --region us-east-1 --repository-name p3-relay \
        --image-id "imageDigest=$digest"
    scan_check='imageScanStatus.status == `"COMPLETE"` && '
    scan_check+='imageScanFindings.findingSeverityCounts == `{}`'
    clean=$(aws ecr describe-image-scan-findings --region us-east-1 --repository-name p3-relay \
        --image-id "imageDigest=$digest" --output text --query "$scan_check")
    test "$clean" = True
    directory=$(mktemp --directory "${XDG_RUNTIME_DIR:?}/p3-relay-release.XXXXXX")
    trap 'rm --recursive --force "$directory"' EXIT
    # Public tags lack immutability: reject existing tags, serialize releases, and consume digests.
    if aws ecr-public describe-images --region us-east-1 --repository-name p3-relay-gateway \
        --image-ids "imageTag=$revision" > "$directory/tag.json" 2> "$directory/tag-error"; then
        printf 'Public tag already exists; inspect its digest before any retry\n' >&2; exit 1
    else
        grep --quiet ImageNotFoundException "$directory/tag-error"
    fi
    # Preserve the workstation's pull policy: use the existing build only if its config matches.
    config_digest=$(aws ecr batch-get-image --region us-east-1 --repository-name p3-relay \
        --image-ids "imageDigest=$digest" --query 'images[0].imageManifest' --output text | \
        node -p 'JSON.parse(require("node:fs").readFileSync(0,"utf8")).config.digest')
    local_id=$(podman image inspect --format=json "$private:gateway-$revision" | \
        node -p 'JSON.parse(require("node:fs").readFileSync(0,"utf8"))[0].Id')
    test "$config_digest" = "sha256:$local_id"
    aws ecr-public get-login-password --region us-east-1 | podman login \
        --authfile "$directory/auth.json" --username AWS --password-stdin public.ecr.aws
    podman push --retry=1 --authfile "$directory/auth.json" --digestfile "$directory/digest" \
        "$private:gateway-$revision" "docker://$public:$revision"
    published=$(< "$directory/digest")
    test "$published" = "$digest"
    actual=$(aws ecr-public describe-images --region us-east-1 --repository-name p3-relay-gateway \
        --image-ids "imageTag=$revision" --query 'imageDetails[0].imageDigest' --output text)
    test "$actual" = "$digest"
    printf 'Published scan-matched gateway %s@%s\n' "$public" "$digest"

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
    # Isolate mocked defaults from private operator configuration; test runs set their own inputs.
    tofu -chdir=infra test -var=budget_alert_email= -var=gateway_certificate_expires_at=
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
