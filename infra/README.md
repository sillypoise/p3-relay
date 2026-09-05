# Relay infrastructure foundations

Status: prepared and locally tested; not applied. These configurations do not yet deploy the API
or worker. See [deployment preparation](../docs/deployment-plan.md) for runtime decisions and cost.

## Ownership and boundaries

- `bootstrap/` owns only Relay's private, encrypted, versioned S3 state bucket.
- This directory owns the image registry, notification queue/DLQ, seven-day runtime log group,
  runtime/migration secret **containers**, scoped IAM roles, and optional task definitions.
  It creates no secret versions, ECS service, or running tasks.
- All resources are project-owned and tagged `Project=p3-relay`. No Railway objects are changed.
- OpenTofu 1.11.x and AWS provider 6.63.0 are required. Both dependency locks are committed.
- The provider checks the explicitly supplied account ID and uses `us-east-1`, with bounded retries.

## Preparation and validation

```sh
just infrastructure-init
just infrastructure-format
just infrastructure-validate
just infrastructure-test
```

The 16 tests use mocked providers: they make no AWS calls. They check queue bounds, encryption,
redrive, state protection, IAM scope, and invalid account/image/origin inputs.
`just check` includes these checks;
`just install` initializes providers without contacting an AWS state backend.

## State bootstrap and plan review

Copy `terraform.tfvars.example` to `terraform.tfvars` and replace its sample account ID with the
identity verified through the approved AWS wrapper. For bootstrap, supply the same non-secret ID
through `TF_VAR_aws_account_id`, or create `bootstrap/terraform.tfvars` with that value.

Run authenticated recipes inside interactive Zsh through the approved wrapper:

```sh
aws-run sp just infrastructure-bootstrap-plan
aws-run sp just infrastructure-bootstrap-apply
```

Review the saved plan before confirming apply. The observed initial plan contains five additions:
S3 bucket, public-access block, encryption, versioning, and TLS-only bucket policy; zero changes and
zero deletions. The bucket has `prevent_destroy` and does not allow forced deletion of its contents.
The plan has **not** been applied. The latest refresh failed because both regional and global STS
endpoints timed out. Regenerate and review a successful plan before apply; do not reuse old plans.

The regional STS endpoint timed out during local planning. This process-scoped workaround succeeded:

```sh
aws-run sp env AWS_ENDPOINT_URL_STS=https://sts.amazonaws.com just infrastructure-bootstrap-plan
```

It uses AWS's official global STS endpoint, preserves TLS certificate verification, and does not
change deployment region or credentials. Owner: repository maintainer. Retry the regional default
at the next deployment session; remove this workaround when connectivity recovers. Do not disable
TLS checks or credential/account validation. No TLS algorithm override is retained.

Bootstrap state stays local, ignored by Git, and is created with owner-only file permissions. Keep
it in controlled storage: losing it requires deliberate import of existing state-bucket resources,
not creating replacements. It contains infrastructure metadata, never application secret values.

## Main infrastructure plan

After the reviewed bootstrap is applied:

1. Copy `backend.hcl.example` to `backend.hcl`; set the created bucket name. Never add credentials.
2. Run `aws-run sp just infrastructure-connect` to initialize encrypted S3 state and native locking.
3. Run `aws-run sp just infrastructure-plan`; review `relay.tfplan` before confirming apply.
4. Run `aws-run sp just infrastructure-apply` only for the reviewed plan.

Use the scoped STS override if the regional endpoint is still unreachable. Never run concurrent
applies. Main state uses S3 locking and versioning; bucket access denies non-TLS transport. State,
local configuration, and saved plans are ignored by Git and excluded from container build inputs.

## Task registration and credential separation

`runtime_image_digest` defaults to empty, registering no task definitions. Set it only to a reviewed
`sha256:` digest present in Relay's ECR repository. Task registration does not start compute.
The shared runtime definition allocates 256 CPU units and 512 MiB total, split between `Main` (API)
and `Worker`. Both are essential, non-root, read-only, and drop Linux capabilities. Capacity
and actual image execution remain unverified. Log mode is explicitly blocking to avoid silent buffer
loss; a CloudWatch outage can stall processes, so this availability tradeoff needs live testing.
`sandbox_origin` defaults to empty and must later be
the exact verified HTTPS hostname, without credentials, a path, or trailing slash.

Secret JSON fields, populated through the controlled channel rather than OpenTofu:

- `p3-relay/runtime`: `database_url`, `source_key`, `ingress_secret`, `operator_token`,
  `destination_url`, `sandbox_key`, and `delivery_secret`.
- `p3-relay/migration`: `database_url` for a separately scoped migration identity.

The API receives only its required fields; the worker receives only database/delivery credentials.
The one-off migration task receives only its separate database URL and has no AWS task role.
Execution roles can pull this repository, write this log group, and read their own secret container.
The runtime task role has only four source-queue actions. Co-located processes share that IAM role;
this is not process-level IAM isolation. See the [contract delta](../docs/notification-contract.md).

Runtime database credentials must lack DDL privileges and access to other portfolio schemas.
Migration credentials must be restricted to Relay's schema operations. Verify database server
identity and encrypted transport before launching either task; task-definition tests do not verify
opaque secret contents or PostgreSQL grants. No database roles or migrations have been applied here.

## Secrets, rollout, rollback, and teardown

Secret versions must be populated through a controlled secret-storage channel, not OpenTofu input
variables or command arguments. This step is not implemented yet. Owner: repository maintainer;
track version metadata during rollout to detect drift without retrieving values into logs/state.
No runtime should start until verified database TLS and runtime secret configuration are checked.

Express Mode service/control-plane IAM and networking, migration execution, live health checks,
budget alerts, and release/rollback automation remain pending. Tagged images are immutable and
retained for rollback; only untagged image artifacts expire automatically. Review storage growth when releasing.
Task definition updates deregister replaced revisions: roll back by registering a new revision with
an approved previous image digest, not by assuming a deregistered ARN can be deployed.

To remove the main foundations intentionally, first run `infrastructure-destroy-plan` through the
wrapper, review it, then confirm `infrastructure-destroy`. A nonempty ECR repository refuses
forced deletion; image removal needs separate approval. Secrets use a 30-day recovery window.
The protected bootstrap bucket is not part of main teardown. Never destroy or restore state merely
to undo an application release, and never alter other projects' Railway schemas.
