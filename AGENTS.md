# Repo Agent Context

<!-- BEGIN MANAGED GUIDE HEADER -->
This repository uses the pi guide system.

## Guide Activation Contract

Active guides for this repository are defined in:

- `.pi/guides.json` — canonical machine-readable guide selection
- installed pi guide package extension — resolves and injects active guides into the system prompt

The pi guide package can be made available either:

- globally from `~/.pi/agent/settings.json`, or
- repo-locally from `.pi/settings.json`

Repo-local `AGENTS.md` supplements the guide system with repository-specific context.
It does not define the canonical active guide set.

## Authoring Rules for This File

Use this file for:

- repository architecture facts
- build, test, and validation commands
- local workflow expectations
- repository-specific constraints
- durable notes that help future tasks in this repo

Do not use this file for:

- reusable cross-repo guide content
- large generic policy documents
- secrets, tokens, or credentials
- machine-readable guide selection state

If a rule should apply across multiple repositories, promote it into the guide package instead of only documenting it here.
<!-- END MANAGED GUIDE HEADER -->

## Repo-Specific Context

<!-- BEGIN REPO CONTEXT -->
- Purpose: Demonstrate durable webhook receipt, bounded delivery retries, operational visibility,
  and safe recovery as an independent portfolio project.
- Primary languages: Go for the API and worker; TypeScript and React for the web application; SQL
  for PostgreSQL; HCL for OpenTofu.
- Key directories: `cmd/` contains Go entry points, `web/` contains the React application, `docs/`
  contains product and delivery contracts, and future `infra/` will contain OpenTofu.
- Architectural constraints: PostgreSQL is authoritative. Optional AWS SQS hints prompt bounded
  database reconciliation; queue messages never authorize delivery. Objects live in `p3_relay`
  because the Railway PostgreSQL instance is shared with other portfolio projects.
<!-- END REPO CONTEXT -->

## Build / Test / Validation

- Install: `just install`
- Build: `just build`
- Test: `just test`
- Lint: `just lint`
- Typecheck: `just typecheck`
- Validation: `just check`; `just sandbox-integration` additionally exercises PostgreSQL, sandbox,
  and notification-loss recovery against an empty disposable `p3_relay_test` database.
- Run one test: `go test ./cmd/api -run TestName` or
  `pnpm --dir web test -- --run src/App.test.tsx`

## Local Workflow Notes

- Preferred commands: Use the root `justfile`; use Podman rather than Docker.
- AWS access: In interactive Zsh, use `aws-run sp aws <arguments>`; the wrapper takes the complete
  command. Region `us-east-1` was verified. Do not print or persist credentials.
- Deployment constraint: AWS budget ceiling is USD 50/month; review estimates before applying.
  See `docs/deployment-plan.md` for current topology and unresolved deployment decisions.
- Safe-to-edit areas: Application code and project documentation within this repository.
- Areas requiring extra care: Delivery state transitions, outbound network validation, credentials,
  shared-database schema qualification, and public sandbox isolation.
- Review expectations: Verify valid, invalid, boundary, authorization, failure, and recovery paths.

## Repository-Specific Constraints

- Compatibility expectations: Treat `docs/delivery-contract.md` as the initial external contract.
- Migration / rollout constraints: Qualify every migration and query with `p3_relay`; do not alter
  other schemas on the shared Railway database.
- Performance constraints: All retries, batches, payloads, response excerpts, and leases are bounded
  by the delivery contract.
- Security / privacy constraints: Prevent SSRF, keep secrets out of telemetry and errors, fail
  closed on authorization failures, and isolate public visitor resources.
