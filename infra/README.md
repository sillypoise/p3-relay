# Relay infrastructure foundations

Status: prepared and locally tested; not applied. These configurations do not yet deploy the API
or worker. See [deployment preparation](../docs/deployment-plan.md) for runtime decisions and cost.

## Ownership and boundaries

- `bootstrap/` owns only Relay's private, encrypted, versioned S3 state bucket.
- This directory owns the image registry, notification queue/DLQ, seven-day runtime log group,
  and a Secrets Manager secret **container**, without a secret version or secret values.
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

Tests use mocked providers: they make no AWS calls. They check queue bounds, encryption, redrive,
transport denial, state protection, and invalid account inputs. `just check` includes these checks;
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
The plan has **not** been applied.

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

## Secrets, rollout, rollback, and teardown

Secret versions must be populated through a controlled secret-storage channel, not OpenTofu input
variables or command arguments. This step is not implemented yet. Owner: repository maintainer;
track version metadata during rollout to detect drift without retrieving values into logs/state.
No runtime should start until verified database TLS and runtime secret configuration are checked.

Runtime/IAM/networking definitions, migration execution, live health checks, budget alert delivery,
and release/rollback automation remain pending. Tagged images are immutable and retained for
rollback; only untagged image artifacts expire automatically. Review storage growth when releasing.

To remove the main foundations intentionally, first run `infrastructure-destroy-plan` through the
wrapper, review it, then confirm `infrastructure-destroy`. A nonempty ECR repository refuses
forced deletion; image removal needs separate approval. Secrets use a 30-day recovery window.
The protected bootstrap bucket is not part of main teardown. Never destroy or restore state merely
to undo an application release, and never alter other projects' Railway schemas.
