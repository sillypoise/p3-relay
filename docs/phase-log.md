# Phase Log

This log records completed work and explicitly marked in-progress phases.

## Phase 1 — Product and delivery contracts

- Defined Relay's problem, users, vertical capability proof, screens, boundaries, and completion
  criteria.
- Defined receipt authentication, idempotency, delivery signing, retry classification, bounded
  timing, state transitions, stable errors, authorization, and network-safety requirements.
- Chose an isolated, quota-limited public sandbox with controlled simulated receivers; retained a
  read-only deployment as the fail-closed fallback.
- Recorded deferred and rejected scope, including AWS until after the local flow, arbitrary
  anonymous destinations, broad multi-tenancy, Kubernetes, and generalized queue infrastructure.
- Added the initial project README and documentation links.

Validation: Documentation was reviewed against the shared portfolio strategy, shipping,
infrastructure, and command standards. No executable implementation exists in this phase.

## Phase 2 — Repository foundation

- Added a Go module with a bounded HTTP server, health endpoint, and success, unsupported-method,
  and unknown-route tests.
- Added a strict React and TypeScript Vite foundation with formatting, linting, type checking, a
  component test, and a production build.
- Added pinned frontend dependencies, a pnpm lockfile, local environment documentation, a Podman
  `Containerfile`, and explicit PostgreSQL container lifecycle recipes.
- Added the root `justfile` as the canonical interface for install, development, formatting,
  validation, build, container, and database tasks.
- Recorded `p3_relay` as the mandatory schema for the shared Railway PostgreSQL instance.

Validation: `just check` passed, including formatting, static analysis, strict type checking, Go
race and boundary tests, frontend tests, and production builds. The Podman OCI image built and its
health endpoint passed a container smoke test. An initial base-image pull timeout was traced to
intermittently unresponsive Docker Hub registry addresses rather than the build definition.

## Phase 3 — Local backend vertical slice

- Added the `p3_relay` schema, repeat-safe migration command, events, delivery attempts, bounded
  states, leases, replay generations, and queue indexes.
- Implemented signed receipt, validation before persistence, body-digest idempotency, and stable
  conflict and unavailable errors.
- Implemented transactional worker claims, signed outbound delivery, attempt recording, bounded
  deterministic retries, lease recovery, dead-letter transitions, and authorized replay.
- Added fail-closed public-address resolution, HTTPS enforcement, redirect refusal, strict network
  timeouts, and an explicit local-only private-address override.
- Added deterministic success, temporary-failure, and permanent-failure receiver scenarios.

Validation: `just check` passed. Migration application and repeat execution passed against an
isolated PostgreSQL 17 cluster. A signed event was accepted, claimed, and delivered to the simulator
with HTTP 204. It was recorded as `delivered` with one durable attempt. Invalid receipt,
authorization, network-address, response-class, exhaustion, and state-transition paths are covered
by tests.

## Phase 4 — Operator dashboard

- Added authenticated, source-scoped overview, event list, event detail, attempt history, and
  endpoint configuration read APIs with bounded event results.
- Added a React and TanStack dashboard with overview metrics, event navigation, attempt timelines,
  dead-letter replay, and read-only endpoint configuration.
- Added explicit authorization, loading, empty, invalid-filter, request-failure, and replay-failure
  states.
- Added a local operator-token entry boundary that retains the token only for the browser tab; no
  operator credential is compiled into the frontend.
- Added responsive navigation and data layouts for narrow and desktop viewports.
- Corrected local service supervision to use bounded child processes instead of an unsupported
  `just --parallel` option.

Validation: `just check` passed, including authenticated and denied API paths, strict TypeScript,
frontend authorization rendering, and the production frontend build. Automated browser screenshot
comparison is not configured; final visual review at representative viewports remains a shipping
check.

## Phase 5 — Public sandbox controls

- Added signed 30-minute visitor cookies, persistent session ownership, exact-origin mutation
  checks, and separate visitor routes that cannot use operator authority.
- Added migration 002 in `p3_relay`, serialized global quotas, per-session event limits, two-replay
  limits, expired-session claim exclusion, and bounded cleanup that preserves active leases.
- Added visitor event creation, list/detail, replay, quota feedback, and session-expiry recovery.
- Kept PostgreSQL receipt and worker retries real; receiver responses are explicitly simulated
  in-process and cannot make external network requests.
- Documented default-disabled enablement, HTTPS/key requirements, controlled migration rollout,
  cleanup, and test commands in `docs/sandbox-contract.md`.
- Extended Go formatting checks to include `internal/` and added pinned Chromium browser tests.

Validation: `just check` passed. `just sandbox-integration` passed with race detection against an
isolated PostgreSQL 17 cluster, including TLS cookie round-trip, two-visitor isolation, concurrent
quota admission, stale-worker recovery, retry/replay history, expiry, lease-aware cleanup, and
unavailable database handling. Migration application and repeat execution passed. Four Chromium
UI tests passed at 390×844 and 1440×900 using explicitly mocked API fixtures.

Deployment remains pending. No shared Railway resources were modified; public routes remain
disabled unless the operator supplies an HTTPS origin and a separate sandbox signing key.

## Phase 6 — SQS notification integration

- Added a pinned AWS SDK adapter with minimal versioned messages, bounded publication, long polling,
  batched acknowledgement, startup queue-policy checks, and sanitized failure events.
- Added post-commit notifications to operator and sandbox receipt/replay. Publication failure does
  not change durable acceptance; PostgreSQL reconciliation recovers lost hints without an outbox.
- Kept database state authoritative for duplicates, reordered/stale hints, retries, and leases.
  Malformed notifications remain eligible for SQS redrive, not delivery replay.
- Added bounded worker reconciliation, cancellation, and retirement of events at the existing
  24-hour automatic-delivery boundary.
- Documented queue/DLQ requirements, least-privilege IAM, enablement, rollback, limits, and the
  deliberate polling tradeoff in `docs/notification-contract.md`.

Validation: `just check` and `just sandbox-integration` passed with race detection. Tests exercise
real SDK serialization/signing against a local server, queue/envelope rejection, duplicate hints,
receive/partial-delete failures, post-commit publication visibility, notification-loss recovery,
idempotent receipt, replay, and exact delivery-age expiry against isolated PostgreSQL 17.

Application integration is complete. No live AWS calls or resource changes were made. Queue/DLQ
provisioning, IAM enforcement, redrive verification, and measured operations remain Phase 7.

## Phase 7 — Deployment preparation (in progress)

- Verified AWS identity through `aws-run sp aws ...` in interactive Zsh; confirmed `us-east-1`.
- Recorded the USD 50/month AWS ceiling and a candidate shared-task topology, pending a cost review
  and public hostname selection. No infrastructure was provisioned.
- Updated the multi-stage Containerfile for locked Go/pnpm builds, API/worker/migration binaries,
  compiled frontend assets, CA certificates, and non-root execution.
- Added a build-context allowlist and same-origin static serving with deep links, no directory
  listings, root-contained file access, and security headers.

Validation: `just check` passed, covering missing builds, traversal, symlinks, and API routing.
A native production-bundle smoke test verified HTML, JavaScript, deep links, CSP, and denied API
access. OCI build verification remains blocked by Docker Hub TLS timeouts;
current-price lookup initially timed out. Deployment remains open.

### Infrastructure foundations follow-up

- The operator approved a generated HTTPS hostname, without buying a domain. Selected ECS Express
  Mode for further runtime work; App Runner's current notice closes it to new customers.
- Retrieved regional price catalogs: modeled baseline USD 36.39/month, or USD 42.23 with one average
  ALB capacity unit, before other usage and taxes. Full cost approval remains pending.
- Added OpenTofu 1.11/AWS provider 6.63 foundations: protected encrypted state, immutable ECR,
  encrypted SQS/DLQ with source-only redrive, short log retention, and secret metadata.
- Added canonical infrastructure recipes, committed provider locks, and five mocked tests. Updated
  `just install` and `just check` so infrastructure validation is part of the normal workflow.
- Generated a live bootstrap plan: five additions, no changes/deletions; did not apply. Regional STS
  timed out; AWS's global STS endpoint worked through the approved wrapper with TLS checks intact.

Validation: `just check` passed with five mocked infrastructure tests. A simulated mock-cleanup
failure was correctly rejected by the recipe. Mocked tests do not prove
live IAM or queue behavior. Docker Hub and public ECR pulls still timed out. Runtime/IAM/networking
configuration, secret setup, migrations, budget alerts, apply, and live recovery proof remain open.

### Task definitions and scoped IAM

- Added digest-gated Fargate task definitions: one bounded API/worker task and a separate one-off
  migration task. Empty image configuration registers neither; no service or running tasks exist.
- Added source-queue-only runtime IAM and separate execution roles for runtime/migration secrets.
  Recorded the shared task-role tradeoff in the notification contract; wire semantics are unchanged.
- Separated migration/runtime secret references and gave the migration task no AWS application
  role. Database grants and TLS still require live verification.
- Added tests for credential separation, task limits, immutable image input, malformed origins, and
  exact DNS length boundaries. Sixteen infrastructure tests now pass under mocked providers.
- Retried state-bootstrap planning: both regional and global STS timed out. Did not apply; no cloud
  resources or shared Railway objects were modified. A fresh successful plan is required.

Remaining: restore AWS/registry connectivity, validate the OCI image and memory sizing, finish the
Express service/control-plane IAM and network configuration, configure secrets/budget alerts, then
review plans, apply, migrate, and verify live failure/recovery paths.

### AWS foundation applied and OCI verified

- Regional STS and registry connectivity recovered; no endpoint or TLS bypass was needed.
- Reviewed and applied the five-resource state bootstrap. An S3 versioning conflict caused partial
  failure; a fresh plan identified only the missing step, and its follow-up apply succeeded.
- Connected the main S3 backend and applied 16 foundation resources with no changes/deletions.
  A subsequent drift plan reported no changes. No ECS service, compute, or Railway changes occurred.
- Verified actual state encryption/versioning/public-access blocks and anonymous HTTP 403 denial.
  Verified live source/DLQ attributes, TLS-denial policies, and 14 IAM simulation decisions.
- `just container-build` and an API container smoke passed: non-root, read-only, dropped capabilities,
  packaged files, missing-config rejection, static routes, and denied operator access.
- `just check` passed with 16 mocked infrastructure tests. The image is not published yet, and the
  smoke used synthetic configuration without a working database; no full capacity claim is made.

See `docs/deployment-verification.md` for scope and limitations. Remaining: image publication/scan,
Express service/network/control-plane IAM, database TLS/roles, secrets, migrations, budget alerts
and complete cost review, generated HTTPS, and live delivery/retry/redrive/recovery.

### Express Mode configuration and publication workflow

- Confirmed custom task-definition support in AWS's live CloudFormation resource schema.
- Added dedicated two-AZ public networking, outbound-only migration access, a Fargate cluster,
  scoped CloudFormation control-plane permissions, and AWS's Express infrastructure role.
- Implemented the one-resource service stack behind `deploy_service=false`, with exactly one
  steady-state task, explicit health checks, and bounded rollback/wait settings.
- Reviewed a live preparation plan: 14 additions, zero changes/deletions, no service or compute.
  Did not apply: `AWSServiceRoleForECS` is absent and account-level bootstrap needs confirmation.
- Added a clean-revision ECR publication recipe with account checks and ephemeral registry login.
- Nineteen mocked infrastructure tests pass, including the disabled service and missing-image gates.

Remaining inputs: approval for standard AWS service-linked roles and a budget alert destination.
Secret/database/migration checks and live service verification still precede activation.

### Image security gate

- Published revision `b0769d5` to ECR. Its scan reported two critical, seven high, and one medium
  OpenSSL inventory finding. The image is blocked and has never been activated.
- Updated the runtime libraries from OpenSSL 3.5.7-r0 to 3.5.8-r0, verified in the rebuilt image.
  Release builds now refresh base images and avoid cached package-install layers.
- Published replacement revision `efb875e`; ECR basic scanning completed with no reported findings.
  Confirmed ephemeral registry authentication directories were cleaned up. The earlier image remains
  blocked. See the verification record for both digests and the scanner's limited scope.
- Application activation still awaits account-level service-linked-role approval, budget alert
  configuration, database/secrets/migration checks, and live deployment verification.


### Approved network bootstrap and budget alerts

- The operator approved standard AWS service-linked-role creation and supplied a private alert
  destination. Neither the address nor operator configuration is committed.
- Reviewed and applied 15 additions: networking, empty Fargate cluster, control-plane roles, and
  a $50 monthly account-wide notification budget. No existing resources were changed or deleted.
- EC2 throttled one subnet creation. Confirmed the failed subnet was absent, reviewed a recovery
  plan containing only that subnet and two route associations, and applied it successfully.
- Verified an empty active cluster, automatic ECS service-linked-role creation, four budget
  notification thresholds, and a final no-change drift plan. Inbox delivery remains unverified.
- Added budget/activation checks covering valid configuration, disabled defaults, invalid mailboxes,
  local-part boundaries, and the missing-budget error path. Full recurring-cost review and
  database/secrets/migration checks still block public service activation.


### Read-only Railway inspection

- Verified authenticated CLI/SSH access to the existing shared PostgreSQL service. Read-only SQL
  confirmed TLS is enabled and that Relay's schema/runtime/migration roles are absent.
- The configured database endpoint is private-only. The server certificate validates for its private
  hostname against the deployment CA; ECS connectivity has not been verified.
- HBA allows password-authenticated non-TLS connections. Stopped before exposing a public port,
  changing shared TLS settings, creating roles, populating secrets, or launching application compute.
- Recorded the network decision in the deployment plan: a Relay-only TLS gateway versus a reviewed
  public proxy/shared-service hardening change. Gateway costs and provider support remain unchecked.
- An initial SSH SQL inspection invoked a pager and timed out. Subsequent inspections explicitly
  disabled paging and bounded SQL/idle session time; no SQL mutations were issued.


### Approved gateway preparation

- The operator approved the Relay-only gateway. Community provider v0.6.2 uses account-token
  authentication, while available access is project-scoped. CLI 4.11 also failed against a removed
  API field. Documented a narrow, maintainer-owned native Railway IaC exception with a review date.
- Installed official CLI 5.49.6 locally after release-checksum verification and pinned SDK 3.11.0.
  No system tool was replaced and no broader credential was requested.
- Added the permanent `p3-relay-gateway` partial. A live pinned plan created only the empty gateway
  service and port 6432 proxy, with explicit CPU/memory/replica/restart/drain settings.
- Verified no gateway deployment exists, existing PostgreSQL/Integration Hub deployment IDs stayed
  unchanged, and PostgreSQL's public TCP-proxy list is still empty. No SQL mutations were issued.
- The follow-up native plan reports three default-normalization differences; this is a documented
  tooling gap, not a zero-drift result. Do not blindly reapply or ignore other changes.
- `just check` passed, including 28 OpenTofu tests and four offline gateway ownership/target tests.
  The pinned-plan SDK version guard needed an inherited shell-path correction, not a bypass.
- Gateway TLS/authentication/bootstrap, database roles, bounded connection/failure tests and the
  running gateway image remain pending. No public Relay application is running yet.

### Local gateway image and boundary verification

- Added a separate PgBouncer 1.25.1 image and bounded Go startup loader. Client TLS is required;
  backend TLS verifies the Railway private hostname and supplied CA. Only Relay's two database
  roles and explicit database alias are admitted. No administrator credential is included.
- Private startup files are owner-only tmpfs files. Failed preparation cleans them up; successful
  exec removes secret variables from PgBouncer's environment. The build context excludes fixtures.
- Added disposable Podman tests for actual TLS/authentication/SQL permissions, runtime connection
  exhaustion, backend and gateway interruption/recovery, and partial-write cleanup under ENOSPC.
  Certificate/key/password/size/expiry boundary tests run in the normal Go suite.
- A readiness test initially treated cached authentication as sufficient. Corrected it to require
  a SQL roundtrip and use fresh failure deadlines, avoiding false recovery/interruption evidence.
- Local tests reject a trusted backend certificate with the wrong hostname as well as an unrelated
  CA. A first assertion expected different PgBouncer wording; the observed diagnostic was
  `not present in server certificate`, and the test now checks that actual failure classification.
- The race-enabled integration suite passed. A single small-query sample with 24 held sessions
  reported 5.763 MB of gateway memory, not peak load or production capacity.
- No cloud deployment, SQL mutation on Railway, or production secret provisioning occurred in this
  step. The reserved gateway remains empty. Release scanning, real grants/identities, application
  trust loading, rotation, native settings validation and live acceptance remain required.
- The [gateway contract](../gateway/README.md) records inputs, failures, resource assumptions,
  maintainer-owned credential lifecycle and remaining gates.

### Application gateway trust and rotation preparation

- Added one shared database TLS loader for API, worker and migration connections. Configured CA
  trust requires hostname verification, TLS 1.2+, no fallback targets and bounded valid CA bundles.
  Rejected inputs do not mutate the connection configuration or disclose supplied values.
- ECS explicitly requires CA material in all three containers and references separate `database_ca`
  JSON fields in runtime/migration secrets. Local configurations without CA retain their previous
  behavior. This is a controlled first-deployment change; no populated secrets or running tasks
  are migrated.
- The real local gateway tests now use the application loader. They exercise old/new CA overlap,
  successful SQL with both identities, and rejection of the old identity after trust retirement.
  Separate local ports model identity transitions; actual Railway/ECS rollout remains unverified.
- `just check` and the race-enabled gateway container suite passed, including byte/count/time
  boundaries, insecure-mode rejection, unchanged failure state and scoped task-secret assertions.
- Authenticated AWS planning reported no changes: the empty image gate still leaves task definitions
  unregistered. No infrastructure apply was needed.
- Production roles, certificates, secrets, expiry alerts and live activation remain pending. See the
  [database TLS contract](database-tls-contract.md) for the rollout sequence and compatibility delta.
