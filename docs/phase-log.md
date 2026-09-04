# Phase Log

This log records completed project phases. Entries describe verified work rather than planned work.

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
isolated PostgreSQL 17 cluster. A signed event was accepted, claimed, delivered to the simulator with
HTTP 204, and recorded as `delivered` with one durable attempt. Invalid receipt, authorization,
network-address, response-class, exhaustion, and state-transition paths are covered by tests.
