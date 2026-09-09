# AWS-to-Railway database connection evaluation

Owner: Relay repository maintainer. Status: read-only evaluation; no topology approved or deployed.
Scope: retain AWS ECS for Relay and the existing shared Railway PostgreSQL database.

## Findings and confidence

High confidence, based on authenticated inspection and the official documentation linked below:

- Railway documents external PostgreSQL access through a public TCP proxy. This exposes a connection
  endpoint, not unauthenticated tables. Database roles and grants still control access.
- Railway's private network is an environment-scoped WireGuard mesh. AWS tasks do not automatically
  participate in it. The inspected documentation does not offer a native AWS VPC attachment.
- The current database has no configured public URL. Its certificate validates against its private
  CA for `postgres.railway.internal`, not a newly assigned public proxy hostname.
- Its catch-all HBA rule accepts SCRAM authentication with or without PostgreSQL TLS. A read-only
  `pg_stat_activity`/`pg_stat_ssl` snapshot found two TCP sessions using `postgres` without PostgreSQL
  TLS. This does not identify their applications or prove private traffic is unencrypted: Railway
  encrypts private-network traffic with WireGuard. A global TLS requirement needs consumer review.
- Schema prefixes organize objects; restricted roles enforce access. Neither prevents connection
  attempts against a publicly reachable listener or isolates shared database CPU/memory/I/O.

## Options

| Option | Benefits | Tradeoffs / disposition |
| --- | --- | --- |
| Public TCP proxy on PostgreSQL | Fewest new components; documented Railway feature. | Changes the shared listener's exposure. Requires verified client TLS and a shared authentication/hardening review. Do not enable unchanged. |
| Relay-only PgBouncer service with its own TCP proxy | Keeps PostgreSQL private; can require TLS and admit only Relay roles without changing other applications' connection paths. | Extra service, credentials, certificates, patching, and a new availability dependency. Recommended if ECS remains required. |
| Private overlay/tunnel between AWS and Railway | Avoids a public database-protocol endpoint. | Adds routing/proxy agents, tunnel credentials, reconnection and availability work. Defer at this scale. |
| Run Relay beside PostgreSQL on Railway | Simplest network arrangement; uses existing encrypted private networking. | Changes the chosen ECS portfolio demonstration and deployment plan. Alternative only with an explicit architecture decision. |

Public PostgreSQL access is not inherently invalid. The gateway recommendation follows from this
shared instance's current configuration and the requirement to avoid disrupting its other users,
not from a claim that every public database port exposes its contents.

## Narrow gateway design, if approved

```text
AWS API / worker / one-off migrator
  -> verified TLS -> Railway TCP proxy -> Relay-only PgBouncer
  -> verified backend TLS over Railway private networking -> shared PostgreSQL
  -> p3_relay schema, using distinct restricted runtime/migration roles
```

Use standard PgBouncer, not a custom database protocol proxy. Its documented controls support:

- `client_tls_sslmode=require`, plus a certificate/key and client-side CA/hostname verification.
  Encryption-only modes are not an identity check. TCP proxying must not be mistaken for Railway's
  automatic HTTPS certificate feature. Plan explicit certificate provisioning, expiry and rotation.
- `server_tls_sslmode=verify-full` and the authenticated database CA for the private backend name.
- An explicit database mapping and Relay-only authentication entries; no wildcard database mapping,
  forced shared administrator user, broad authentication lookup, or public administrative console.
- Session pooling initially, preserving existing pgx sessions/prepared statements and migration
  behavior. Bound client/server connections and waits against the API/worker pools and deployment
  overlap; reject overload rather than exhausting the shared server.

The gateway restricts the entry point, not SQL privileges. Runtime and migration grants still need
negative cross-schema/DDL tests. PostgreSQL remains a shared availability and resource dependency.
Gateway correctness, failover behavior and actual resource use are unverified until tested.

## Incremental cost sketch

Railway's current service rates are $10/GB-month RAM, $20/vCPU-month CPU, and $0.05/GB public egress.
Assume one always-running gateway, average RAM of 0.064–0.128 GB, average CPU of 0.01–0.05 vCPU,
and 10 GB/month public egress for low-traffic demonstration use:

- RAM: $0.64–$1.28/month.
- CPU: $0.20–$1.00/month.
- Egress: $0.50/month.
- Total: **$1.34–$2.78/month additional Railway usage**, before credits, taxes or plan changes.

These are estimates, not measurements or caps. Direct TCP access would also incur public egress;
the modeled extra gateway compute portion is $0.84–$2.28/month. No new volume is assumed. Existing
Railway subscription charges and AWS runtime/transfer costs are separate. Measure handshake load,
memory and database traffic before accepting the estimate; revisit for higher traffic or replicas.

## Provisioning and verification gates

The community Railway Terraform provider documents service and TCP-proxy resources. This is evidence
of resource coverage, not a tested deployment path. Its service docs still refer to legacy Railway
configuration files, while current Railway docs describe a newer IaC system. Check the pinned
provider's API compatibility and resource-limit support before adopting it. Keep ownership scoped to
the Relay gateway: no project-wide import/apply that takes over the existing database or application.
Use OpenTofu where supported and document any required exception rather than silently replacing it.

Before activation, verify valid TLS and role access; rejection of non-TLS, wrong CA/name, invalid
credentials and non-Relay roles; denied cross-schema access and runtime DDL; connection exhaustion,
gateway/backend interruption and recovery; and secret exclusion from logs, images and IaC state.
Keep the shared administrator password out of both AWS tasks and the gateway.

Recommendation confidence: **medium**. The mechanisms and prices are documented, but provider
compatibility, workload sizing and live behavior still need implementation-time verification.
No infrastructure or database mutations were made for this evaluation.

## Sources

- [Railway PostgreSQL](https://docs.railway.com/guides/postgresql), external access and egress.
- [Railway TCP proxy](https://docs.railway.com/networking/tcp-proxy), raw TCP and assigned host/port.
- [Railway private networking](https://docs.railway.com/networking/private-networking/how-it-works),
  WireGuard encryption and environment boundaries.
- [Railway pricing](https://docs.railway.com/reference/pricing/plans), service resource unit rates.
- [PostgreSQL HBA](https://www.postgresql.org/docs/current/auth-pg-hba-conf.html), matching and TLS.
- [PgBouncer configuration](https://www.pgbouncer.org/config.html), TLS, authentication and bounds.
- [Community Railway provider](https://github.com/terraform-community-providers/terraform-provider-railway/tree/master/docs/resources),
  service and TCP-proxy resources.
- [Railway IaC](https://docs.railway.com/infrastructure-as-code), current configuration mechanism.
