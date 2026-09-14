# Relay gateway ownership and workflow

Owner: Relay repository maintainer. This directory owns only `p3-relay-db-gateway` in the verified
portfolio production environment. `partial = "p3-relay-gateway"` is a permanent ownership identity:
these independent repositories cannot safely share an environment-wide authoring file. Never rename
it or import the whole project. PostgreSQL, its volume, and Integration Hub remain unmanaged here.

## Narrow OpenTofu exception

AWS resources remain under OpenTofu. The community Railway provider v0.6.2 uses an
`Authorization: Bearer` account-token transport; its documented authentication does not support our
project-token pathway. Do not request broader credentials just to use that provider. The official
Railway CLI supports project-scoped authentication and native plan/apply with named partials.

The installed CLI 4.11 failed its scale command against a removed `Region.railwayMetal` field.
Use the project-local official CLI 5.49.6, not the system installation. `just gateway-tool-install`
verifies the Linux/x86_64 musl archive against the SHA-256 published with that GitHub release before
extracting the executable. Node's SDK is pinned to Railway 3.11.0 with a pnpm integrity lockfile;
it is deployment-only, not an application dependency. Existing TypeScript/format/lint tools are reused.

This exception is scoped to the gateway, owned by the maintainer, and reviewed at each gateway
release and no later than 2026-12-08 (90 days after first apply). Retire it through a reviewed ownership migration if an
OpenTofu provider supports project-scoped authentication and the required controls. Never let both
systems own the same service. The native partial is smaller than a custom provider or provisioning
framework and avoids account-wide credentials.

## Commands

- `just install`: install locked dependencies, including the deployment SDK.
- `just gateway-tool-install`: install the pinned CLI locally; no cloud mutations.
- `just gateway-check`: offline ownership, bounds, wrong-target tests, lint and strict typechecking.
- Commit the reviewed `.railway/` files before saving a deployment plan.
- `just gateway-plan`: save `.railway/gateway-plan.json` with owner-only permissions. The artifact
  is ignored and binds the change set to environment state and the source tree. Never use
  `--decrypt-variables` or `--show-values` for these plans.
- Review the saved plan: only the gateway may be created/updated. Stop on another service, database,
  volume, shared variable, or any unexpected deletion.
- `just gateway-apply`: confirm and apply that saved plan. The recipe never grants permission for
  destructive changes. Stale plans must be refreshed and reviewed, not forced through.

Use the existing Railway project-token environment; never copy tokens into files or arguments.
After applying, compare the existing services' deployment IDs and inspect a fresh drift plan.
The SDK's version guard inspects `$_`; under `just` that can name `just` rather than Railway.
The recipes unset that inherited shell value and put the pinned CLI first in `PATH`, preserving
actual executable-version checking. Do not disable the guard or spoof a version value.
Use cryptographic randomness for credentials, not the SDK's deterministic `context.randomString`.

## Closed preparation state

The initial definition reserves one empty service and a TCP proxy to port 6432. It has **no source
image or Git deployment**, variables, volumes, HTTP domain, or database connection. There is no
running gateway yet. Its eventual resource limits are 0.25 CPU / 128 MiB and one steady-state
replica, with three failure restarts, no configured rollout overlap, and 15 seconds of draining.
No overlap trades availability during rollout for cost bounds; connection loss must be tested.
These limits are not measured sizing or a hard spending cap.

## Live preparation checkpoint

The pinned create-only plan was applied on 2026-09-09:

- Gateway service ID: `867d03a3-8ea0-4802-8852-2e4f730fd4a6`.
- Dedicated endpoint: `altaria.proxy.rlwy.net:28318`, forwarding to gateway port 6432.
- No gateway deployment exists. PostgreSQL's TCP-proxy list remains empty.
- Existing PostgreSQL and Integration Hub deployment IDs were unchanged after apply.

Known tooling gap: the next plan reports `source.type` (`null` versus `empty`),
`deploy.restartPolicyType` (`null` versus `ON_FAILURE`), and `deploy.sleepApplication` (`null` versus
`false`). Railway omits these explicit default values on readback. Do not repeatedly apply these
normalization differences or claim zero drift; resolve or explicitly validate effective deployment
settings before runtime activation. Other deployment limits round-tripped in the inspected graph.

The [gateway image and local tests](../gateway/README.md) now cover both TLS hops, Relay-only
credentials, connection exhaustion, startup cleanup and interruption/recovery. This is local evidence,
not live gateway verification. Before deploying: provision real scoped grants and identities, test
certificate rotation and application CA loading, scan the release image, and close the native
settings/lifecycle gaps above. Populate private keys/passwords through a reviewed stdin-based
secret channel, never IaC values or command arguments; verify sealing support separately rather
than assuming it is exposed by the public API. Inspect raw variable
command output only in a controlled consumer: it can contain credentials.

## Gateway release and SSH compatibility checkpoint

The reviewed gateway release is available anonymously at:

`public.ecr.aws/f3e3j6u2/p3-relay-gateway@sha256:c89b062ef8e8cf026925620ad8ee63c5b096b8fed578c719fef993f9628e40b7`

Its private/public manifest digests match. ECR basic scanning completed with empty finding counts;
this does not prove application/code security or live gateway behavior. The source remains empty
until real identities/grants and effective deployment settings are verified.

CLI 5.49.6 project status/API calls succeed, but its native SSH path attempts user-key setup and is
rejected with project-scoped authentication. Do not request an account token to satisfy that tooling
path. Narrow compatibility exception: the existing CLI 4.11.0 was verified for read-only SSH commands
against the exact PostgreSQL service ID `d16e1e40-5489-4891-af9c-643e0a8c7a30`. The exception also
covers the reviewed, credential-free `ops/database-bootstrap.sql` and Relay-scoped SQL privilege
checks after the read-only preflight. That script creates NOLOGIN roles transactionally and does
not authorize arbitrary shared-database changes. Keep all native IaC
commands on the pinned 5.49.6 CLI. Maintainer owns this exception; review each release and by
2026-12-08, retiring it when project-scoped native SSH is supported.

Ordinary piped/command-mode stdin was not verified with the legacy transport. An interactive,
echo-disabled PTY probe succeeded only with explicit readiness handshakes; it used non-secret test
text. This is not yet verification of a private SQL/bootstrap channel. No credentials were sent,
and no production role/schema changes were made. Secret provisioning must still avoid arguments,
echo, command logs and shared-service redeployments. Public API variable-upsert introspection did not
expose a sealing option; do not assume the CLI can seal variables merely because it can list them.

## Evidence references

- Official release: https://github.com/railwayapp/cli/releases/tag/v5.49.6
- Native SSH key setup: https://github.com/railwayapp/cli/blob/v5.49.6/src/commands/ssh/mod.rs
- Provider transport: https://github.com/terraform-community-providers/terraform-provider-railway/blob/v0.6.2/internal/provider/client.go
- Provider authentication: https://github.com/terraform-community-providers/terraform-provider-railway/blob/v0.6.2/docs/index.md
- Native partial ownership and pinned plans: https://docs.railway.com/infrastructure-as-code
- Connection design: [evaluation](../docs/database-connection-evaluation.md).
