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

## Local validation

`just gateway-container-test` includes `TestDatabaseBootstrap`. It runs a network-isolated disposable
PostgreSQL container without host ports or Railway credentials. Tests cover successful role bounds,
wrong database, denied administration, duplicate names and rollback for inherited PUBLIC schema,
table and security-definer access. Rejections check that no roles/schema persist.

Run only this test with:

```sh
go test -tags gatewayintegration ./gateway -run TestDatabaseBootstrap -count=1 -timeout=60s
```
