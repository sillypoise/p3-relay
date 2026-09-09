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
settings/lifecycle gaps above. Populate private keys/passwords through the
CLI's stdin/sealed-variable mechanisms, never IaC values or command arguments. Inspect raw variable
command output only in a controlled consumer: it can contain credentials.

## Evidence references

- Official release: https://github.com/railwayapp/cli/releases/tag/v5.49.6
- Provider transport: https://github.com/terraform-community-providers/terraform-provider-railway/blob/v0.6.2/internal/provider/client.go
- Provider authentication: https://github.com/terraform-community-providers/terraform-provider-railway/blob/v0.6.2/docs/index.md
- Native partial ownership and pinned plans: https://docs.railway.com/infrastructure-as-code
- Connection design: [evaluation](../docs/database-connection-evaluation.md).
