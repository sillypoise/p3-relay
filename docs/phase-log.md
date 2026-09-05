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
