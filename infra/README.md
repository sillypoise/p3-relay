# Relay infrastructure foundations

Status: state bootstrap and 31 main resources applied, including networking and budget alerts.
No API/worker service is running.
See [deployment preparation](../docs/deployment-plan.md) for runtime decisions and cost.

## Ownership and boundaries

- `bootstrap/` owns only Relay's private, encrypted, versioned S3 state bucket.
- This directory owns the image registry, notification queue/DLQ, seven-day runtime log group,
  runtime/migration secret **containers**, scoped IAM roles, networking, and budget notifications.
  Task definitions and the Express service are explicitly gated. No secret versions are managed.
- Project resources use `Project=p3-relay` where supported. AWS owns shared service-linked roles;
  they are retained outside project teardown. No Railway objects are changed.
- OpenTofu 1.11.x and AWS provider 6.63.0 are required. Both dependency locks are committed.
- The provider checks the explicitly supplied account ID and uses `us-east-1`, with bounded retries.

## Preparation and validation

```sh
just infrastructure-init
just infrastructure-format
just infrastructure-validate
just infrastructure-test
```

The 28 tests use mocked providers: they make no AWS calls. They check queue bounds, encryption,
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
The bootstrap is now applied. A transient S3 versioning conflict was recovered by refreshing the
plan and applying only the missing configuration. Do not replace or destroy a bucket to recover
from `OperationAborted`; inspect state and review a new plan first.

Regional STS and container registry connectivity have recovered. Use normal regional endpoints;
the temporary global STS workaround is no longer needed. TLS and account validation remain enabled.

Bootstrap state stays local, ignored by Git, and is created with owner-only file permissions. Keep
it in controlled storage: losing it requires deliberate import of existing state-bucket resources,
not creating replacements. It contains infrastructure metadata, never application secret values.

## Main infrastructure plan

For a new checkout connecting to the existing, reviewed state bucket:

1. Copy `backend.hcl.example` to `backend.hcl`; set the created bucket name. Never add credentials.
2. Run `aws-run sp just infrastructure-connect` to initialize encrypted S3 state and native locking.
3. Run `aws-run sp just infrastructure-plan`; review `relay.tfplan` before confirming apply.
4. Run `aws-run sp just infrastructure-apply` only for the reviewed plan.

The main foundation apply succeeded, and its follow-up plan reported no changes. Never run
concurrent applies. Main state uses S3 locking and versioning; bucket access denies non-TLS transport.
State, local configuration, and saved plans are ignored by Git and excluded from container inputs.

## Task registration and credential separation

`runtime_image_digest` defaults to empty, registering no task definitions. Set it only to a reviewed
`sha256:` digest present in Relay's ECR repository. Task registration does not start compute.
The shared runtime definition allocates 256 CPU units and 512 MiB total, split between `Main` (API)
and `Worker`. Both are essential, non-root, read-only, and drop Linux capabilities. Capacity
under load remains unverified. The OCI build and API-only startup/static-serving smoke passed;
see [verification scope](../docs/deployment-verification.md). Logging is explicitly blocking to avoid
silent buffer loss; a CloudWatch outage can stall processes. This tradeoff needs live testing.
`sandbox_origin` defaults to empty and must later be
the exact verified HTTPS hostname, without credentials, a path, or trailing slash.

Secret JSON fields, populated through the controlled channel rather than OpenTofu:

- `p3-relay/runtime`: `database_url`, `database_ca`, `source_key`, `ingress_secret`, `operator_token`,
  `destination_url`, `sandbox_key`, and `delivery_secret`.
- `p3-relay/migration`: `database_url` and `database_ca` for a separately scoped migration identity.

The API receives only its required fields; the worker receives only database/delivery credentials.
The one-off migration task receives only its separate database URL/CA and has no AWS task role.
All three containers set `RELAY_DATABASE_CA_REQUIRED=true`. The URL must use `sslmode=verify-full`
and the gateway alias `p3_relay`; `database_ca` contains its public trust anchor, not a private key.
See the [database TLS contract](../docs/database-tls-contract.md). Populate the newly required CA
field before registering these task definitions. Existing secret versions are still empty; this is
an initial controlled cutover, not a migration of a running deployment.
Execution roles can pull this repository, write this log group, and read their own secret container.
The runtime task role has only four source-queue actions. Co-located processes share that IAM role;
this is not process-level IAM isolation. See the [contract delta](../docs/notification-contract.md).

Runtime database credentials must lack DDL privileges and access to other portfolio schemas.
Migration credentials must be restricted to Relay's schema operations. Verify database server
identity and encrypted transport before launching either task; task-definition tests do not verify
opaque secret contents or PostgreSQL grants. No database roles or migrations have been applied here.

## Image publication

Commit and review a clean working tree, then run `aws-run sp just container-publish` from interactive
Zsh. The recipe checks the account against the state-owned ECR repository, builds the committed
revision with an OCI revision label, and publishes an immutable commit tag. It prints the resulting
manifest digest. AWS environment credentials are never copied into the image or registry auth file.
The generated ECR login credential is piped to Podman and kept only in a private temporary directory
under `XDG_RUNTIME_DIR`, removed on exit. Do not enable shell tracing for this operation.

If publication fails, inspect the immutable tag before retrying; a failed client request does not
prove that nothing was uploaded. Never overwrite/delete a release just to make a retry succeed.
Scan and review the published image before setting `runtime_image_digest`.

## Express Mode preparation and activation

The applied preparation plan adds dedicated two-AZ public networking, an outbound-only migration
security group, a Fargate cluster, and control-plane roles. It created no NAT gateway or compute.
The migration group permits outbound TCP because Railway assigns its proxy port; its destination
and TLS identity must be validated before running migrations. No inbound migration rule exists.

`deploy_service` defaults to false. Setting it true creates the single-resource CloudFormation stack
and starts billable compute/load balancing, so do so only after secret, database, migration, image,
and budget checks pass. Changing it back to false destroys the service: review that destructive plan
explicitly. Data remains in PostgreSQL, but availability and the generated hostname may change.

The live CloudFormation type schema confirms support for `TaskDefinitionArn`; the pinned native
provider does not expose it. The stack sets minimum/maximum tasks to one and health checks to
`/health`. It deliberately omits extra security groups: Express creates HTTPS load-balancer ingress
and ALB-only ingress to the task. TLS terminates at the ALB; the last hop is HTTP within the private
VPC address space, not end-to-end TLS. Inspect generated rules before declaring the demo ready.

AWS's Express Mode defaults are documented in
[created resources](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/express-service-work.html).
The infrastructure role uses AWS's service-managed Express policy; the CloudFormation role is scoped
to this service and its runtime roles. Trust-policy compatibility and live control-plane permissions
still need validation. Service name, cluster, infrastructure role and service tags are create-only
properties: changing them needs a replacement/cutover review, not an ordinary rolling-update claim.

The operator approved standard service-linked-role bootstrap. Cluster creation automatically
created `AWSServiceRoleForECS`, verified through IAM. Other approved service-linked roles can be
created when their services need them; do not provision unused roles preemptively or delete shared
roles during project teardown. Full recurring-cost review remains an activation gate.

## Budget alerts and private configuration

`budget_alert_email` is a sensitive, optional ASCII mailbox input. Configure it only in the ignored,
owner-readable `terraform.tfvars`, never in Git. The selected address is held in protected plans and
encrypted state; the sensitive marker redacts output, not stored values. Mocked tests override the
operator address with an empty value or reserved test addresses.

The deployed `p3-relay-aws-monthly-guardrail` budget watches total account costs without tag filters,
so Express-managed/untagged charges are not missed. It changes no existing budgets. It includes tax
and support, excludes credits/refunds, and sends notifications above $35/$45/$50 actual monthly cost
and forecast monthly cost above $50. Forecasts need sufficient billing history. These are alerts,
not a hard cap or automatic shutdown; inbox delivery remains unverified.

Compatibility delta: first service activation now requires a nonempty alert mailbox and successful
budget creation. No service existed before this prerequisite; no running workload needs migration.

## Secrets, rollout, rollback, and teardown

Secret versions must be populated through a controlled secret-storage channel, not OpenTofu input
variables or command arguments. This step is not implemented yet. Owner: repository maintainer;
track version metadata during rollout to detect drift without retrieving values into logs/state.
No runtime should start until verified database TLS and runtime secret configuration are checked.

Network/control-plane preparation and budget alerts are applied; the Express service is not.
Migration execution, live health checks, email delivery, and full release/rollback verification
remain pending. Tagged images are immutable and
retained for rollback; only untagged image artifacts expire automatically. Review storage on release.
Task definition updates deregister replaced revisions: roll back by registering a new revision with
an approved previous image digest, not by assuming a deregistered ARN can be deployed.

To remove the main foundations intentionally, first run `infrastructure-destroy-plan` through the
wrapper, review it, then confirm `infrastructure-destroy`. A nonempty ECR repository refuses
forced deletion; image removal needs separate approval. Secrets use a 30-day recovery window.
The protected bootstrap bucket is not part of main teardown. Never destroy or restore state merely
to undo an application release, and never alter other projects' Railway schemas.
