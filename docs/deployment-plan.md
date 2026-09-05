# Deployment preparation

Owner: Relay repository maintainer.
Status: Phase 7 in progress. No infrastructure has been provisioned.

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
rather than adding a process supervisor or another provider. This wrapper is not implemented yet.
Owner: repository maintainer; revisit when the native provider supports `taskDefinitionArn`.

See [cost estimate](cost-estimate.md) for verified regional rates and assumptions. The modeled base
is USD 36.39/month; one average load-balancer capacity unit brings it to USD 42.23 **before** other
usage charges and taxes. Budget alerts are not hard caps. Full cost review remains an apply gate.

## Infrastructure preparation

[Infrastructure foundations](../infra/README.md) define encrypted state storage, immutable image
releases, encrypted SQS/DLQ, scoped IAM roles, bounded logs, and separate runtime/migration
secret metadata. Digest-gated runtime and migration task definitions are implemented; no ECS service
or running tasks are created. Sixteen mocked tests pass, covering invalid and boundary paths.
These are not live IAM, queue, redrive, or deployment evidence.

A live state-bootstrap plan was generated: five additions, zero changes, zero deletions. Nothing
was applied. The regional STS endpoint timed out; AWS's official global STS endpoint worked with
certificate verification unchanged. On the latest retry both endpoints timed out, so no apply was
attempted. A new successful plan is required. The workaround and teardown protections are documented
in the infrastructure README. Express service/control-plane IAM, networking, secrets, migrations,
budget notifications, capacity measurement, and live failure/recovery verification remain open.

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

Validation: `just check` and a native smoke test against the production frontend build passed,
including deep links, JavaScript assets, security headers, and unauthorized API rejection. Full OCI
build verification is pending because Docker Hub timed out while pulling the Node base image.
