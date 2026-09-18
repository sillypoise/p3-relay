# Live deployment checkpoint

Owner: Relay maintainer. Status: public application and initial vertical-flow acceptance verified.
This is an independent portfolio demo, not client work or a production reliability certification.

Public URL: https://p3-aa9ae9b0d2e745dc832970fc5b60883c.ecs.us-east-1.on.aws

## Current deployment and evidence

- Runtime revision `p3-relay-runtime:3`: one running 0.25-vCPU/512-MiB task, two containers,
  completed deployment, zero pending tasks. One internet-facing ALB across two owned subnets;
  task public IPv4 allocation verified and no NAT gateway in the owned VPC.
- Image: `sha256:66225c1cdbb00452412271624c7ed286a0952520171a38abe772ae5826cc5030`, source
  `a3dd9b79ffe6fcb91d00077acc05d030d7483c88`. ECR package scan completed with empty findings.
- Public DNS addresses and trusted HTTPS health checked before destination configuration.
  The destination-only Secrets Manager update preserved other values, round-tripped AWSPENDING,
  then conditionally promoted it against the original current version. Protected tmpfs staging was
  removed. No credential values entered source, command arguments, plans or diagnostic output.
- Exact sandbox origin is configured. Browser credentials remain visitor-scoped; operator tokens
  are not distributed to visitors. PostgreSQL still owns receipt, attempts, retries and replay.
- `just check` passed. A fresh AWS plan reports no changes, subject to the explicitly documented
  immutable creation-metadata exception below. Railway's separate known planner limitations remain.

Live operator evidence, using synthetic JSON and the authenticated HTTPS receiver:

| Scenario | Event ID | Observed result |
| --- | --- | --- |
| Success | `9319d499-b316-4e6e-902e-f1bbdc0d5a34` | Delivered, HTTP 204 |
| Temporary failure | `1a9d3b11-0f10-43e5-abb7-fa16129d175a` | Persisted HTTP 503, 503, 204 attempts |
| Permanent failure | `0c50386a-bfff-483d-a965-4a06d1fd3b0f` | HTTP 422, dead letter, authorized replay with prior history preserved |

Identical receipt returned the original event with `duplicate:true`; conflicting body returned 409.
The receiver verifies delivery HMAC, but its outcomes are synthetic and share the API failure domain.

Two visitor sessions verified Secure/HttpOnly/SameSite=Strict host-only cookies. An owned event
`82ec7c3f-90f7-45b3-9c24-3ce72ff9ba04` reached delivered through the explicitly labeled in-process
simulation. The other visitor received 404 for it; visitor access to operator reads returned 401;
wrong Origin returned 403 and URL injection returned 400. Session expiry/cleanup owns these fixtures;
there is no cross-visitor reset. This is API-level live evidence, not a live browser layout test.

## Recovery record and operating constraints

Initial Express creation produced a running service but CloudFormation failed while inspecting its
parent service ARN. Both `DescribeServiceDeployments` and `DescribeServiceRevisions` needed that
resource scope in addition to descendant ARNs. Policies were narrowed to Relay's exact service;
resource-specific IAM simulation allowed Relay and denied an unrelated service. A failed custom-policy
simulation did not stop an early shell chain as intended; subsequent recovery chains used `set -e`.
Do not present that failed simulation as a passed prerequisite.

A second create failed with AlreadyExists. CloudControl readback, not the failed stack's logical
DELETE_COMPLETE marker, established that the service still existed. Only the empty failed stack
record (no physical resource binding) was removed. An import-only change set associated the existing
service with the new stack, using temporary Retain protection and omitting Outputs as required by
CloudFormation import. OpenTofu's stale failed-stack binding was replaced with the imported stack:

`arn:aws:cloudformation:us-east-1:397483721549:stack/p3-relay-runtime/ba45c2a0-b175-11f1-a762-0affefe8ab71`

A reviewed in-place update restored Outputs and ordinary lifecycle ownership. Import cannot recover
OnFailure or set an existing stack's creation timeout. Only `on_failure` and `timeout_in_minutes` are
ignored on existing stacks to prevent destructive replacement; bounded creation defaults remain for
future stacks. Owner: Relay maintainer; review by 2026-12-08 or next deliberate stack replacement.
Provider waits remain bounded to 40 minutes. Timeout is not cancellation or proof of rollback.

The first origin rollout was interrupted locally and rejected by the managed error-rate alarm.
Its two 20% error datapoints coincided with intentional bootstrap 503 probes: strong evidence of a
probe-induced rollback, not proof of a revision-3 crash. CloudFormation rollback then repeatedly
returned ECS 500. Revision 2 was still serving but its task definition had been deregistered; that
is a confirmed lifecycle defect, not a proven explanation of the generic ECS 500.

Before skipping only the failed Service rollback step, live revision, health path, scaling, subnets
and infrastructure role were checked against the rollback template. The interrupted apply had also
registered active revision 3 without persisting its binding. It was imported, not recreated. Runtime
revision retention now preserves rollback definitions; see [operations](../ops/README.md).
A fresh revision-3-only rollout succeeded with the alarm enabled and naturally OK. Failure probes
were run only after completion. Expected 4xx/5xx tests must not overlap future rollout/bake windows.

Stale locks from interrupted local waiters were removed only after checking no local OpenTofu
process remained and reconciling AWS state. Do not replay saved plans after uncertain outcomes.

## Remaining work

- Live browser/mobile visual review, screenshots and portfolio walkthrough.
- Sustained ECS/gateway capacity, full rotation/draining and crash-recovery exercises.
- Separate SQS redrive and lost-notification live evidence; completed deliveries alone do not prove it.
- Actual billing/ALB LCU and log-volume checks against the revised cost envelope (about $41.01 before
  tax under low-traffic assumptions, $49.21 with a 20% reserve, not a spending cap).
- Future AWS Budgets email delivery remains distinct from operator-confirmed SNS test emails.
