# Database bootstrap

Owner: Relay maintainer. This is the credential-free, one-time stage before migrations and gateway
activation. `database-bootstrap.sql` requires an existing authorized administrator connected to the
`railway` database. It creates only `p3_relay`, `p3_relay_runtime` and `p3_relay_migrator`.

## Contract

- Both roles start **NOLOGIN**, without passwords or elevated role flags. Connection limits are
  24 runtime / one migrator. The migrator owns Relay DDL; runtime receives schema usage only.
- Statements and lock waits are bounded to five seconds within one transaction. Existing names,
  a wrong database, insufficient authority, or unsafe inherited shared grants abort the operation.
  Do not adopt existing objects or revoke another project's grants to make this script succeed.
- Success does not mean migrations, table grants, authentication or TLS are ready. Separate private
  credential provisioning must precede LOGIN, and runtime table privileges must be explicit after
  migrations. Do not give runtime schema creation or schema-migration write access.
- First-deployment contract: rerunning against existing roles is an error, not an idempotent repair.
  Future changes require an explicit migration/review rather than editing an applied bootstrap.

Run with `psql --no-psqlrc --set=ON_ERROR_STOP=1 --file=ops/database-bootstrap.sql` only through the
reviewed administrator channel. No passwords belong in this file, command arguments or logs. The
Railway compatibility exception and exact target are recorded in `.railway/README.md`.

An interactive PTY is not equivalent to a batch exit contract: the tested `psql --file=-` session
remained open after an error despite `ON_ERROR_STOP`. Private SQL must use explicit error detection,
transaction outcome checks and session completion; exit zero after `\quit` is not SQL success.

## Private role activation

The 2026-09-15 activation used the existing project-scoped administrator channel, not a copied
administrator credential. Runtime/migration passwords came from their separate AWS secret versions.
The consumer validated exact role/host/port/database/TLS parameters, distinct 64-character lowercase
hex passwords and the reviewed gateway CA fingerprint before use.

Before changing credentials, the same transaction acquired advisory lock 330052 and required both
exact role names to remain NOLOGIN, without passwords or elevated/inherited authority. This is not
an overwrite/rotation routine: a failed precondition requires investigation. Password encryption was
explicitly SCRAM-SHA-256. Password validity ends at the reviewed certificate expiry,
2026-12-13T01:24:40Z; existing sessions are not forcibly revoked at that time.

Input echo and history were disabled, and SQL logging suppression was scoped to the administrator
session only. No private input was written to source, arguments or diagnostics. Successful preparation
was checked before COMMIT; authoritative post-commit flags were then checked separately. Do not
retry an uncertain commit or treat interactive process exit as transaction success.

Verification first used non-secret credentials with rollback and deliberate SQL-error rejection.
After activation, both real roles authenticated using private-hostname verify-full TLS (minimum 1.2).
Wrong-password and wrong-hostname connection attempts failed; the prior connection remained usable.
A later activity query found zero Relay sessions. This is private database evidence, not gateway/ECS
acceptance. Future password rotation must coordinate database, gateway and client credentials and
drain/restart existing connections; that live rotation procedure remains unverified.

## Runtime table grants

After the ECS migration reports version 2, run `runtime-grants.sql` through the gateway as
`p3_relay_migrator`. It validates database, caller, exact migration versions and excluded runtime
DDL/migration-metadata authority before granting in one bounded transaction:

- Events and sandbox sessions: SELECT, INSERT, UPDATE, DELETE.
- Delivery attempts: SELECT, INSERT, DELETE (attempt records are not updated).
- Sandbox budget: SELECT, UPDATE only (the singleton is migration-owned).

No TRUNCATE, REFERENCES, TRIGGER, migration-table access, future-table default privileges or
cross-schema grants are added. Repeated application preserves these explicit grants. Existing
unexpected authority is not a repair path; stop and investigate. `TestRuntimeGrants` verifies
missing/wrong migration state, wrong caller, rollback, repeated application and allowed/denied SQL
against a disposable local database as part of `just gateway-container-test`.

## Runtime revision retention and rollout probes

Owner: Relay maintainer. Runtime task definitions use `skip_destroy=true`: retiring a definition
before Express completes rollback can leave CloudFormation unable to restore its previous revision.
This retains definition metadata, not running compute. After successful rollout and bake completion,
retain current and previous known-good ACTIVE runtime revisions. Before another rollout, review and
explicitly deregister older unused revisions if necessary; never deregister a revision referenced by
running tasks, an unfinished deployment or its rollback target. Stop additional rollouts if that
cleanup/reconciliation cannot be established. Include retained definitions in reviewed teardown.

Express's managed rollback alarm includes expected application 4xx/5xx responses. Do not run negative
or synthetic-failure probes during deployment or its bake window, and do not disable the alarm to
make tests pass. Verify the alarm has naturally recovered before starting a rollout. Test failure
paths after stability and allow their metric window to clear before another deployment.

## First live migration checkpoint

On 2026-09-16, a reviewed two-addition OpenTofu plan registered runtime/migration task definitions
using the scanned application digest. `deploy_service` stayed false. A controlled one-off ECS
`run-task` call selected the migration definition, owned public subnet and migration security group,
Fargate 1.4.0, one task, public IP and a recorded idempotency token. Secret values were neither task
overrides nor command arguments. Task execution is an imperative operation; its definition, IAM,
networking and log destination remain under the canonical infrastructure recipes.

Task `a9844a2405934331924d805e51e7c9ba` stopped with exit 0 and logged migration version 2. Runtime
grants then passed live allowed/denied checks. Future runs must use a reviewed registered revision,
record their task ARN/token, bound the wait and inspect exit status/logs plus authoritative database
state. A timed-out waiter does not cancel a task or establish rollback; inspect/stop only the owned
task if needed and reconcile before retrying. Never start the runtime service as a migration shortcut.

## Initial migration compatibility

`001_initial.sql` now creates the schema only when it is absent. This preserves fresh local
administrator installs while allowing the restricted schema owner to use the existing production
schema without database-wide CREATE permission. Already-applied migration versions and table
shapes do not change. First production migration requires a rebuilt/scanned application image;
the previously published `0968f9b` image still contains the incompatible unconditional statement.

## Local validation

`just gateway-container-test` includes `TestDatabaseBootstrap`. It runs a network-isolated disposable
PostgreSQL container without host ports or Railway credentials. Tests cover successful role bounds,
wrong database, denied administration, duplicate names and rollback for inherited PUBLIC schema,
table and security-definer access. Rejections check that no roles/schema persist. Migration tests
also cover fresh administrator installation, restricted ownership of an existing schema, and
rejection when a restricted role lacks a pre-created schema.

Run only this test with:

```sh
go test -tags gatewayintegration ./gateway -run TestDatabaseBootstrap -count=1 -timeout=60s
```
