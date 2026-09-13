# Database TLS configuration

Owner: Relay maintainer. Applies to API, worker and migration runner.
Status: implemented and tested locally; no running ECS deployment uses it yet.

## Inputs and behavior

- `RELAY_DATABASE_URL` remains a pgx connection string. Gateway connections must use
  `sslmode=verify-full`, the assigned public gateway hostname/port, and database alias `p3_relay`.
- `RELAY_DATABASE_CA` contains one or two PEM CA certificates, at most 32 KiB total. Certificates
  must currently be valid and permit certificate signing (an omitted X.509 key-usage extension is
  unrestricted). Private keys, malformed blocks, extraneous text and excess certificates fail.
- `RELAY_DATABASE_CA_REQUIRED` accepts only `true`, `false`, or an unset/empty value. `true` rejects
  missing CA material. Every ECS container explicitly sets it to `true`; it is not a secret.
- An unset CA with an unset/false requirement preserves existing local pgx behavior. A nonempty CA
  always activates strict checking regardless of that flag. Local plaintext development is not an
  acceptable deployment override.

Configured trust requires hostname verification, TLS 1.2 or newer, and no alternate/fallback
connection targets. A stricter existing TLS minimum is retained. Only the supplied CA pool is trusted;
public system roots are not implicitly added. Hostname verification remains enabled. The loader
clones the TLS configuration only after all validation succeeds; rejection leaves it unchanged.

The application performs no CA-file writes and requires no writable mount for this configuration.
Parsing occurs once at startup: two bounded certificates, no network calls, and at most 32 KiB of
PEM input per process plus standard-library decoding/pool overhead. No throughput claim is made.
Pool connection limits and session/prepared-statement behavior are unchanged.

## Failures

Invalid requirement values, missing required CA, insecure TLS modes, fallback targets, incompatible
protocol bounds, malformed/oversized bundles, invalid CA authority and expired/future certificates
are startup errors. No connection is attempted through those invalid configurations. Error classes
are fixed text and do not contain the supplied connection string or PEM material.

A valid startup configuration is not proof of connectivity: runtime TLS still rejects an incorrect
server CA/hostname or expired identity. Applications continue to handle network failures and unknown
transaction outcomes through their existing error/recovery paths. A canceled operation does not
necessarily undo a committed write.

## Compatibility and rollout

This is additive for local consumers with no CA input. ECS secret JSON now requires `database_ca` in
both separately scoped runtime/migration secrets. Supply that public anchor and a verify-full URL
before registering the new task definitions; older application images do not implement this loader.
No running task or populated secret version is being migrated at this checkpoint. Use a newly built
and reviewed image for first activation; do not combine old images with the new trust expectations.

Rotation sequence:

1. Generate a new gateway identity through vetted tooling; distribute its public CA alongside the
   still-valid old CA in both client secrets. Keep private keys out of clients and IaC state.
2. Restart client tasks so their in-memory trust includes both anchors; retain the old gateway.
3. Deploy the gateway's new identity and verify new SQL connections with the intended hostname.
4. Remove old trust and restart clients. Verify the new identity still works and old trust fails.

Complete rotation before either bundled certificate expires; an expired member rejects the bundle.
Replacing an environment value does not update an existing process. Existing TLS sessions also do
not automatically end when trust changes: restart/drain affected connections explicitly. Operator
expiry alerting and the exact Railway/ECS rollout procedure remain live release gates.

## Verification

`just check` exercises valid modes, insecure alternatives, missing/invalid configuration, exact
certificate count/byte/time boundaries, CA authority restrictions and failure-state immutability.
`just gateway-container-test` uses this same application loader against real disposable PgBouncer and
PostgreSQL containers. It checks both TLS hops and old/new anchor overlap followed by old-anchor
retirement. Separate local ports represent old/new identities; this is not a live Railway rollout.

No shared Railway schemas, roles, deployment settings or credentials are changed by these tests.
