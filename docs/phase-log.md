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
race and boundary tests, frontend tests, and production builds. The OCI smoke build was attempted but
could not be verified because Docker Hub timed out during the base-image pull. Re-check it when
registry access is available.
