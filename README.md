# Relay

Relay is an independent portfolio project demonstrating reliable webhook delivery and operational
visibility. It accepts signed events, persists them, delivers them to an HTTPS endpoint, retries
recoverable failures, and exposes delivery and dead-letter state for inspection and replay.

Relay is a product concept, not client work or a production service. Public demonstrations will
identify seeded traffic and simulated destinations explicitly.

## Project status

Phase 3, the local backend vertical slice, is in progress. Its database schema, migration command,
signed receipt endpoint, idempotent persistence, delivery worker, bounded retry transitions, and
simulated receiver are implemented. Authorized replay, outbound address validation, and database
integration verification remain before the phase is complete.

## Documentation

- [Product brief](docs/product-brief.md)
- [Delivery contract](docs/delivery-contract.md)
- [Phase log](docs/phase-log.md)

## Planned stack

- Go API and delivery worker
- React, TypeScript, and TanStack
- PostgreSQL
- AWS SQS and ECS after the local delivery flow is proven
- OpenTofu for project-owned infrastructure
- Podman for local OCI container workflows

## Local development

Requirements are Go 1.25, Node.js 24, pnpm 10, just, and Podman.

```text
just install
just database-create
just database-start
just develop
```

`database-create` is a one-time local setup step. Common commands are discoverable with `just`.
Run the complete non-mutating validation set with `just check`.

Copy `.env.example` to `.env` only when local overrides are needed. The local values are development
credentials and must not be reused in deployment.

## Shared PostgreSQL boundary

Deployment will use a Railway PostgreSQL instance shared by portfolio projects. Every Relay table,
index, sequence, migration, and query must be explicitly scoped to the `p3_relay` schema. Relay must
not create or modify objects in another project schema or rely on the connection's default
`search_path`.
