# Deployment preparation

Owner: Relay repository maintainer.
Status: Phase 7 in progress. Infrastructure foundations are applied; no public application runs yet.

## Confirmed constraints

- AWS command wrapper: `aws-run sp aws <arguments>` in an interactive Zsh shell. The wrapper takes
  the whole command, so `aws-run sp sts ...` is not valid. STS identity verification succeeded.
- Configured region: `us-east-1`.
- Operator budget ceiling: USD 50/month for AWS; minimize spend rather than spending to the limit.
- PostgreSQL remains on shared Railway, with every Relay object in `p3_relay`. No RDS instance.
- The operator approved a temporary provider-generated HTTPS address; no domain purchase or custom
  DNS is required for this phase. The sandbox still needs an exact configured HTTPS Origin.

## Runtime design and remaining gates

Prefer ECS Express Mode: it supplies a generated HTTPS URL, TLS load balancer, and Fargate service.
Use one 0.25-vCPU/512-MiB task with separate API and worker containers sharing the image; serve the
frontend from the API. Set both minimum and maximum task counts to one. Avoid RDS, NAT gateways,
a separate frontend service, and extra steady-state tasks. Measure memory before approving the size.

Confidence: medium for the overall fit; generated HTTPS and custom task definitions are documented,
but runtime capacity, networking, IAM, database TLS, and the full cost envelope remain unverified.
Sharing a task reduces baseline cost but couples restarts, capacity, and the task's IAM permissions.
PostgreSQL leases recover interrupted work. Rollouts may briefly run an additional task.

App Runner was considered and rejected: AWS stopped accepting new customers on April 30, 2026 and
recommends ECS Express Mode instead. Two ordinary ECS services would leave insufficient headroom at
the estimated baseline; an ordinary ALB alone does not supply our required trusted HTTPS hostname.

The ECS API and CloudFormation support a custom task definition with sidecars (primary container
named `Main`). AWS provider 6.63.0 does not expose that property on its native Express resource.
Narrow admission: use a single-resource CloudFormation stack managed by OpenTofu for that resource,
rather than adding a process supervisor or another provider. The wrapper is now implemented but
not applied; AWS's live CloudFormation schema confirms custom task-definition support.
Owner: repository maintainer; revisit when the native provider supports `taskDefinitionArn`.

See [cost estimate](cost-estimate.md) for verified regional rates and assumptions. The modeled base
is USD 36.39/month; one average load-balancer capacity unit brings it to USD 42.23 **before** other
usage charges and taxes. Budget alerts are not hard caps. Full cost review remains an apply gate.

## Infrastructure preparation

[Infrastructure foundations](../infra/README.md) define encrypted state storage, immutable image
releases, encrypted SQS/DLQ, scoped IAM roles, bounded logs, and separate runtime/migration
secret metadata. Digest-gated runtime and migration task definitions are implemented; no ECS service
or running tasks were created. Nineteen mocked tests now pass, covering invalid and boundary paths.

Regional connectivity recovered. Reviewed and applied the five-resource state bootstrap and the
16-resource foundation plan, with no changes/deletions to existing resources. A transient S3
versioning conflict recovered through a fresh plan and a one-resource follow-up apply. Main state
now uses encrypted, versioned S3 storage and native locking; a drift plan reports no changes.

Live checks verified state protection, anonymous state denial, queue/DLQ attributes and policies,
and 14 resource-specific IAM simulation decisions. These do not prove ECS runtime enforcement or
actual redrive. See [verification scope](deployment-verification.md). Express service/control-plane
IAM and networking are now defined; their reviewed preparation plan has 14 additions and no compute.
It remains unapplied: the missing account-level ECS service-linked role needs bootstrap approval.
Secrets, migrations, budget alerts, capacity measurement, and live recovery remain open.
No Railway objects have been changed.

## Container packaging

`just container-build` builds `localhost/p3-relay-api:development` using Podman. The image includes:

- `/usr/local/bin/relay-api` (default command).
- `/usr/local/bin/relay-worker` (override command for the worker container).
- `/usr/local/bin/relay-migrate` (explicit one-off migration task, never automatic at API startup).
- `/app/migrations` and the compiled frontend under `/app/web`.
- Trusted CA certificates and a non-root UID/GID of 10001.

The build uses locked Go/pnpm dependencies in separate stages. `.containerignore` allowlists source
inputs and excludes local environment files, Git history, dependency folders, and test artifacts.
Runtime image files are root-owned and readable by the application user. The intended runtime
filesystem is read-only; secrets and database URLs arrive through runtime environment injection.

`RELAY_WEB_DIRECTORY=/app/web` is set in the image. Outside the image, omitting it preserves the
separate Vite development workflow. A configured missing dashboard build stops API startup.

The API serves GET/HEAD for `/`, `/events`, `/endpoint`, UUID event detail routes, and generated
`/assets/` files. Unknown paths return 404; unsupported methods return 405; listings are disabled,
and traversal/symlink escapes cannot read outside the opened web root. HTML uses `no-store` and a
same-origin Content Security Policy. API routes and authorization remain separate from SPA routing.

Validation: `just check`, the full OCI build, and a read-only/non-root container smoke test passed.
The smoke verified packaged files, missing-configuration rejection, static routes, security headers,
and API authorization denial. It used synthetic configuration without a working database, so full
API/worker capacity and delivery still require verification.
