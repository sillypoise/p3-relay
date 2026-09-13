# Relay PostgreSQL gateway

Owner: Relay maintainer. Status: locally verified candidate, not deployed to Railway.
This is PgBouncer 1.25.1 with a small Go startup loader, not a custom database protocol implementation.
It is separate from the API/worker image and has no shared PostgreSQL administrator credential.

Admitted complexity: the standard PgBouncer protocol implementation plus a startup loader to validate
and materialize injected secrets without shell interpolation. The loader reuses Go's standard
library. A bespoke proxy, shared administrator authentication and a general provisioning framework
are not needed.

## Boundary contract

Clients connect using database alias `p3_relay`; PgBouncer maps that alias to database `railway` at
`postgres.railway.internal:5432`. The only admitted roles are `p3_relay_runtime` and
`p3_relay_migrator`. Session pooling preserves pgx session/prepared-statement behavior. Schema grants
remain PostgreSQL's responsibility; the gateway is not an SQL authorization filter.

Required runtime inputs, supplied through controlled secret injection rather than IaC values:

| Variable | Contract |
| --- | --- |
| `GATEWAY_HOSTNAME` | Lowercase certificate hostname, at most 253 bytes. |
| `GATEWAY_CERTIFICATE` | One self-signed CA/server certificate matching that hostname. Clients must receive its public trust anchor separately. |
| `GATEWAY_PRIVATE_KEY` | One matching, unencrypted PKCS8 PEM private key. Generate using vetted cryptographic tooling. |
| `GATEWAY_BACKEND_CA` | One valid CA certificate obtained through authenticated Railway access, not an unauthenticated connection. |
| `GATEWAY_RUNTIME_PASSWORD` | 32 cryptographically random bytes encoded as 64 lowercase hexadecimal characters. |
| `GATEWAY_MIGRATION_PASSWORD` | Same format, but a different independently generated password. |

Each PEM input is limited to 16 KiB. Startup checks format, key pairing, certificate identity and
validity before creating files. Its validity interval is `NotBefore <= now < NotAfter`.
Passwords must match separately provisioned database roles; format validation cannot prove entropy.
No real credentials or private keys belong in this directory or a container build context.

Startup writes five owner-only files into a new private `/dev/shm` directory. Failed preparation
removes partial files. Successful `exec` replaces Go with PgBouncer and removes all gateway secret
variables from the process environment. PgBouncer retains access to the private tmpfs files until
container teardown. The lifecycle requires one launch per container; Podman container restart was
verified to discard old startup files. Railway's equivalent lifecycle still needs verification.

Invalid startup inputs and operating failures exit 1 with fixed, value-free error classifications.
The listener accepts PostgreSQL TLS 1.2/1.3 only. Backend TLS verifies both its CA and private hostname;
wrong CA/name failures do not fall back to plaintext. No public PgBouncer administration/stats role
is configured. PgBouncer errors are bounded by its configured login/query waiting budgets; overload
can return `query_wait_timeout` and disconnect the client. Consumers must handle connection loss and
unknown transaction outcomes rather than assume cancellation rolled back a completed write.
Connection attempts and pooler failures remain logged; query/payload/debug logging is not enabled.
Tests check that startup and native pooler diagnostics omit the injected credential material.

This introduces a new database connection endpoint/alias, not a migration of existing clients.
Integration Hub and shared PostgreSQL networking/settings must remain unchanged. Live rollout
requires separate Relay roles/schema, verified application CA loading, and a reviewed gateway release.

## Resource and lifecycle budgets

- 32 frontend connections; 25 backend connections for the one database mapping.
- Runtime: 24 backend / 30 frontend connections; migrator: 1 backend / 2 frontend connections.
- API uses eight database connections and worker uses four. Two overlapping runtime tasks consume
  up to 24 backend connections, leaving one for an explicitly scheduled migration.
- Session pooling, no reserve pool or minimum idle pool. Waits: login/connect/query queue five seconds;
  query and idle-transaction limits 30 seconds; idle client timeout 300 seconds.
- Protocol packets are at most 1 MiB, above the 256 KiB event-body contract with metadata headroom.
- Three bounded PEMs plus config/auth files use under 64 KiB before filesystem page overhead. At most
  32 maximum-sized protocol buffers would consume 32 MiB, excluding TLS/library overhead. The gateway
  container limit is 128 MiB / 0.25 CPU; this is a budget sketch, not a bound on library allocations.
- A single local Podman sample reported `5.763MB / 134.2MB` with 24 held runtime sessions issuing
  small queries. This is neither peak RSS nor production sizing, latency, or throughput evidence.

Certificates/private keys are owned by the maintainer. Before activation, establish expiry alerts
and test rotation: stage new public trust alongside old trust in clients, deploy the new gateway
identity, verify connections, then retire old trust. Password rotation must update the corresponding
PostgreSQL role, private gateway auth input and application secret, with an explicit interruption
window. Restart the gateway to end old sessions; merely changing a password does not revoke existing
connections. The [application trust loader](../docs/database-tls-contract.md) and two-anchor overlap/retirement
are tested locally. This rollout/rotation procedure is not yet implemented or verified on Railway.

## Local commands and evidence

```sh
just gateway-container-build
just gateway-container-test
just check
```

The integration recipe uses a fresh local Podman network and disposable PostgreSQL container.
It never reads a database URL from the environment or connects to Railway. Fixtures create a fake
neighbor schema solely to prove permission denial. Credentials/certificates are test-only and the
context allowlist excludes all tests and generated inputs from the image. Normal completion removes
owned containers, volumes and networks. If the test process is killed or hits its outer timeout,
inspect remaining `p3-gateway-test-*` resources before removing only confirmed test fixtures; do not
prune unrelated Podman resources.

Executed checks:

- Valid runtime reads and migration DDL; rejected runtime DDL and cross-schema reads.
- Verified frontend TLS and observed backend TLS through `pg_stat_ssl`.
- Accepted TLS 1.2 and rejected obsolete TLS/plaintext, wrong frontend CA/name/password,
  administrator login, wrong database alias,
  PgBouncer admin access, untrusted backend CA and a trusted backend certificate with the wrong name.
- Exactly 24 held runtime sessions accepted; an extra runtime query exhausted the five-second wait
  rather than gaining another backend connection. Releasing capacity allowed recovery.
- Backend interruption/restart and gateway kill/restart, with fresh SQL probes for recovery.
  Authentication success alone is insufficient because PgBouncer can cache its welcome response.
- Forced 8 KiB tmpfs exhaustion after partial writes; startup exited 1, files were removed before
  teardown, and diagnostic output did not contain the fixture credentials.
- PEM/password limits, invalid/mismatched keys, and certificate validity boundary instants.
- The same trust loader used by API/worker/migrator, with old/new CA overlap, successful queries
  against both local identities, and rejection of the retired identity after old trust is removed.

The development build may reuse layers. Release builds must pull fresh bases and disable cached
package layers (`podman build --pull=always --no-cache`), then scan the resulting image.

Remaining release gates: fresh image vulnerability scanning, live certificate/secret provisioning,
real database grants, live resource/lifecycle checks, effective Railway deployment settings and
resolution of the [native planner gap](../.railway/README.md). These tests do not prove crash recovery
of arbitrary in-flight database transactions or public AWS-to-Railway connectivity. Repository-wide
assertion density is unmeasured; these checks do not establish SAF-05 compliance.
