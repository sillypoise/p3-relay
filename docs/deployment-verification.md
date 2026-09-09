# Stage 7 foundation verification

Owner: Relay repository maintainer. Scope: infrastructure foundation, not a deployed application.
Source revision for the verified OCI build: `ef40c9d`.

## Observed results

- Regional STS authenticated through `aws-run sp` to the expected account. No endpoint override or
  TLS verification bypass was needed.
- Reviewed bootstrap plan: five additions, no changes/deletions. Applied four resources initially;
  S3 rejected concurrent versioning configuration with `OperationAborted` (HTTP 409). A refreshed
  plan showed only the missing versioning resource. That recovery apply succeeded without replacing
  the bucket or weakening its protections.
- Reviewed main plan: 16 additions, no changes/deletions. Applied ECR, SQS/DLQ and policies, scoped
  IAM roles/policies, a seven-day log group, and two empty secret containers. No task definitions,
  ECS services, compute instances, load balancers, or Railway changes were applied.
- A subsequent `just infrastructure-plan` reported **no changes**.
- The actual remote state object has AES256 encryption and a non-null S3 version ID. All four
  public-access blocks are enabled. Unsigned HEAD access to that state object returned HTTP 403.
- Live source queue attributes match the notification contract: standard queue, 300-second
  visibility, 10-second polling, 1,024-byte size, one-day retention, encryption, and five-receive
  redrive. The encrypted DLQ has fourteen-day retention and source-only redrive permission. Both
  queue policies deny insecure transport.
- IAM's simulator checked seven actions across the source queue and DLQ: 14 resource-specific
  decisions. The four runtime hint actions were allowed on the source; purge, delete-queue, and
  policy mutation were denied. All seven actions were denied on the DLQ. This is policy simulation,
  not execution under ECS-issued task credentials. Inspect `ResourceSpecificResults`, not just the
  aggregate decision when combining allowed and denied resources in one simulation request.

## Container checks

`just container-build` now succeeds using Podman and the multi-stage Containerfile. The local image
ID is `9ca996f7a65cb97dca9f1f65411c752370c6d7f9386607be87246aa349969ce5`; this is a local image ID,
not an ECR manifest digest or a published release identifier.

Smoke checks verified the packaged API/worker/migration binaries, compiled frontend, migrations,
CA bundle, UID 10001, read-only filesystem, and dropped capabilities. Each binary rejected missing
configuration with exit status 1. The API served health, a deep link, and its JavaScript asset;
operator access returned 401, and hidden/missing paths returned 404.

The API smoke ran with 256 MiB and 0.125 CPU, matching its proposed container limits, using synthetic
configuration and an intentionally unreachable loopback database. This checks startup and static
serving, **not** working database delivery, peak memory, or combined API/worker capacity. The test
container was stopped and removed. No real credentials were injected into it.

`just check` passed, including 16 mocked infrastructure tests. These local tests remain separate
from the live metadata checks above.

## Image publication and security gate

The clean revision `b0769d5` was published through `just container-publish`, using an ephemeral ECR
login credential. Its manifest digest is:

```text
sha256:53297354f951f292b986f98a3ee5127df0445217f3fbe0c8bb695e57b5dae0aa
```

Release disposition: **blocked, never deployed**. ECR completed its scan with two critical, seven
high, and one medium finding, all attributed to OpenSSL 3.5.7-r0. Critical identifiers were
CVE-2026-75803 and CVE-2026-63073. These are inventory findings, not demonstrated application exploits.

The runtime build now upgrades `libcrypto3` and `libssl3`; local verification found 3.5.8-r0. Release
builds refresh base images and package-install layers rather than reusing stale security packages.
Replacement revision `efb875e` was published with this manifest digest:

```text
sha256:5e48d76746fe5bc51c52037255602df9f437211b81fea0004e1837ee0ae1f998
```

ECR basic scanning completed with an empty finding-count map. The former OpenSSL findings are absent
from this replacement scan. This is not a claim that application code/dependencies have no
vulnerabilities. Temporary registry-auth directories were confirmed removed after publication.
Service activation remains blocked by the other deployment gates, not the resolved OS findings.

## Approved preparation and budget verification

After explicit operator approval, a fresh plan added 15 resources: project networking, an empty
Fargate cluster, control-plane IAM, and one notification-only monthly budget. EC2 rejected one
subnet creation with `503 RequestLimitExceeded`. A read confirmed only the other subnet existed;
a refreshed plan added just the missing subnet and two route associations. Recovery succeeded,
and the subsequent drift plan had no changes. No resources were replaced or deleted.

Live AWS queries confirmed:

- Both selected availability zones were available.
- `AWSServiceRoleForECS` exists after automatic cluster bootstrap. AWS owns this shared role;
  project teardown must retain it. ELB/autoscaling role creation is deferred until needed.
- Cluster `p3-relay` is `ACTIVE`, with zero running/pending tasks and zero services.
- Budget `p3-relay-aws-monthly-guardrail` is USD 50 monthly, with no cost filters.
- Actual notifications have 70/90/100-percent thresholds; forecast notification has 100 percent.
  All four reported `OK`; subscriber queries matched the supplied mailbox for every threshold.
  This verifies configuration, not actual inbox delivery or a spending cap.

The private recipient is excluded from Git and normal plan output. The ignored variable file has
mode 0600; encrypted state and private saved plans contain the configured recipient. Tests use
reserved addresses instead. The main state now owns 31 resources, separate from the five-resource
state bootstrap and AWS-owned service-linked role.

## Local PgBouncer candidate — 2026-09-09

`just gateway-container-test` passed with strict Go vet and race-enabled tests against disposable
Podman containers. The [gateway contract and test matrix](../gateway/README.md) distinguish real local
TLS/authentication/permission/connection-limit/recovery checks from still-unverified live behavior.
Both untrusted backend CA and trusted-but-wrong backend hostname were rejected. Partial startup
writes were removed before container teardown after forced tmpfs exhaustion.

The image runs as UID 10001 with a read-only filesystem and dropped capabilities in these tests.
It uses PgBouncer 1.25.1-r0 and patched libcrypto3/libssl3 3.5.8-r0. This candidate has not been
published or vulnerability-scanned; the earlier application-image ECR scan does not cover it.
A single sample with 24 held small-query sessions reported `5.763MB / 134.2MB` in Podman's memory
output. Do not substitute that observation for peak/resource/cost measurements on Railway.

There were no cloud mutations in this implementation step. Production credentials/certificates,
roles/schema, gateway deployment and application CA loading remain pending. Native Railway default
normalization remains an explicitly recorded gap in [.railway/README.md](../.railway/README.md).

## Repeating checks and remaining gates

Use `just infrastructure-plan` through the approved AWS wrapper for drift checks. Review live
metadata using S3 `head-object`/`get-public-access-block`, SQS `get-queue-attributes`, and IAM
`simulate-principal-policy`, scoped to the resources recorded in Relay's state. Never retrieve
secret values or state bodies into logs for these checks. Repeat `just container-build` and
`just check` for code changes.

Still pending: Express service activation,
verified database TLS and scoped database roles, secret population, migrations, generated HTTPS
origin, budget notification delivery, full cost review, measured capacity, real task-role behavior,
actual SQS redrive, and live delivery/retry/recovery. No public demo is running yet.
