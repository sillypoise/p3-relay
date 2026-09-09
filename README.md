# Relay

Relay is an independent portfolio project demonstrating reliable webhook delivery and operational
visibility. It accepts signed events, persists them, delivers them to an HTTPS endpoint, retries
recoverable failures, and exposes delivery and dead-letter state for inspection and replay.

Relay is a product concept, not client work or a production service. Public demonstrations will
identify seeded traffic and simulated destinations explicitly.

## Project status

Phase 4, the operator dashboard, is complete. Relay now exposes authenticated operational reads and
responsive overview, event list, event detail, attempt timeline, replay, and endpoint configuration
views. Phase 5 adds an isolated visitor sandbox with signed expiring cookies, transactional quotas,
controlled worker simulations, replay, and bounded cleanup. Visitor routes are disabled by default;
see the sandbox contract for HTTPS configuration and rollout requirements.

Phase 6 adds optional SQS notifications with PostgreSQL reconciliation for lost or duplicated hints.
Application integration is tested locally. Phase 7 deployment preparation is in progress: AWS access
is verified and container packaging is updated. OpenTofu foundations and mocked security tests are
added, including scoped IAM and digest-gated task definitions. The state bootstrap and infrastructure
foundations are now applied. The OCI build and smoke checks pass; no public application is running.

## Documentation

- [Product brief](docs/product-brief.md)
- [Delivery contract](docs/delivery-contract.md)
- [Dashboard contract](docs/dashboard-contract.md)
- [Sandbox contract](docs/sandbox-contract.md)
- [SQS notification contract](docs/notification-contract.md)
- [Deployment preparation](docs/deployment-plan.md)
- [AWS-to-Railway connection evaluation](docs/database-connection-evaluation.md)
- [Deployment verification scope](docs/deployment-verification.md)
- [Infrastructure workflow](infra/README.md)
- [AWS cost estimate](docs/cost-estimate.md)
- [Phase log](docs/phase-log.md)

## Planned stack

- Go API and delivery worker
- React, TypeScript, and TanStack
- PostgreSQL
- Optional AWS SQS notifications; ECS deployment remains planned
- OpenTofu for project-owned infrastructure
- Podman for local OCI container workflows

## Local development

Requirements are Go 1.25, Node.js 24, pnpm 10, just, Podman, and OpenTofu 1.11.x.

```text
just install
just database-create
just database-start
just develop
```

`database-create` is a one-time local setup step. `just develop` supervises the API, worker,
simulated receiver, and Vite frontend together. Open the dashboard at `http://127.0.0.1:5173`.
Common commands are discoverable with `just`; run the complete non-mutating validation set with
`just check`. Phase 5 additionally provides `just sandbox-integration` for an empty disposable test
database and `just browser-test` for mobile/desktop Chromium checks; see the sandbox contract.

Copy `.env.example` to `.env` only when local overrides are needed. The local values are development
credentials and must not be reused in deployment.

## Shared PostgreSQL boundary

Deployment will use a Railway PostgreSQL instance shared by portfolio projects. Every Relay table,
index, sequence, migration, and query must be explicitly scoped to the `p3_relay` schema. Relay must
not create or modify objects in another project schema or rely on the connection's default
`search_path`.
