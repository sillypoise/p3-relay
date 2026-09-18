# AWS cost estimate — deployment review input

Owner: repository maintainer. Status: reviewed low-traffic initial deployment envelope, not a bill
or spending cap. Rates refreshed 2026-09-16 from regional catalogs published 2026-09-11 (CloudWatch:
2026-09-15). Initial provisioning must be checked against the resource assumptions below.

## Assumptions and calculation

Use `us-east-1`, Linux/x86 Fargate, 730 hours/month, one task containing API and worker, one ALB,
and three in-use public IPv4 addresses (two ALB addresses plus one task address). Existing Railway
charges are outside this AWS estimate. No free-tier credits, discounts, or committed spend assumed.

| Item | Verified unit price | Modeled monthly cost |
| --- | --- | --- |
| Fargate CPU, 0.25 vCPU | $0.04048/vCPU-hour | $7.39 |
| Fargate memory, 0.5 GB | $0.004445/GB-hour | $1.62 |
| Application Load Balancer | $0.0225/hour | $16.43 |
| Three in-use public IPv4 addresses | $0.005/address-hour | $10.95 |
| **Baseline** | Calculated before rounding | **$36.39** |
| One average ALB capacity unit | $0.008/LCU-hour | $5.84 |
| **Baseline plus one average LCU** | | **$42.23** |

Confidence: high for these catalog rates and arithmetic; medium for sizing and address-count
assumptions until the runtime plan and actual deployment are checked. ECS Express Mode has no
additional service fee. Its underlying resources are charged normally.

### Ancillary envelope and decision

| Item and monthly workload assumption | Allowance |
| --- | --- |
| Two Secrets Manager secrets at $0.40 each, plus 1,000 reads at $0.05/10,000 | $0.805 |
| 500,000 standard SQS requests at $0.40/million, without free-tier credit | $0.20 |
| 0.5 GB log ingestion at $0.50/GB and 0.12 GB retained at $0.03/GB-month | $0.254 |
| 1 GB private ECR storage at $0.10/GB-month | $0.10 |
| State storage/versions and low-volume S3 GET/PUT operations | $0.03 |
| External/cross-AZ transfer reserve, not a verified transfer quote | $0.30 |
| Up to 24 extra task/address hours for rollouts and migrations | $0.42 |
| Existing expiry alarm/channel allowance | $0.25 |
| Two standard Express/autoscaling alarm metrics at $0.10 each | $0.20 |
| Managed rollback alarm: six input metrics at $0.10/metric-month | $0.60 |
| **Ancillary envelope** | **$3.159** |

The low-traffic scenario assumes **0.25 average LCU**, yielding approximately **$41.01/month** before
local taxes. A planning reserve of 20% for tax/uncertainty gives **$49.21**, under the $50 target;
20% is a reserve, not a claim about this account's tax rate. The one-average-LCU stress scenario is
**$45.39 before tax**, and would exceed $50 with that reserve. These scenarios are not hard caps.

Live inspection confirmed one two-container 0.25-vCPU/512-MiB task, one internet-facing ALB across
two subnets, a task public IPv4 address and no owned NAT gateway. Express also created a six-input
rollback alarm, adding $0.60 to the earlier envelope; ALB IPv4 billing remains a modeled assumption.

Confidence: high for refreshed catalog rates/arithmetic; medium for the initial usage envelope.
Admission: proceed with one task and generated HTTPS, then inspect generated resources and usage.
Re-evaluate if sustained LCU exceeds 0.25, monthly logs exceed 0.5 GB, images exceed 1 GB, rollouts
exceed 24 task-hours or observed tax/other account charges consume the reserve. The signed synthetic
receiver shares the API; no extra compute is needed. Do not enable Container Insights, paid
dashboards, NAT or extra replicas without a new cost review.

Cost Explorer returned an estimated September 1–16 account-wide unblended cost of **$0.1855** on
2026-09-16. It is delayed month-to-date evidence, not a projection or final invoice. The $50 budget
alerts above $35/$45/$50 actual and $50 forecast cost. Operator-confirmed SNS tests do not establish
delivery of a future AWS Budgets email. Public request abuse, address allocation or retained
resources can invalidate this envelope.

Do not apply the runtime plan until it confirms one steady-state task, no NAT gateway, the intended
IPv4/ALB allocation, and adequate remaining headroom. Recheck rates and measured memory before
apply, after changing topology, or if the load/retention assumptions change. Limit task scaling to
one; budget alerts supplement resource limits but cannot guarantee a hard billing ceiling.

## Expiry reminder increment

The deployed expiry channel adds three one-time reminder jobs, one SNS email subscription and one
standard CloudWatch alarm, without application compute, Lambda or a polling process. Allow **$0.25
per month** of the remaining headroom for this small channel. This is a conservative planning
allowance, not a measured bill. The standard alarm rate was refreshed at $0.10/month; request/email
volume remains an assumption in this allowance. At the configured three reminders and maximum
three retries each, up to 12 publish attempts carry less than 12 KiB of public metadata. Notification
latency and mailbox receipt are not guaranteed by this cost estimate.

## Reviewer-visible sources

- [Fargate regional catalog][ecs-prices], usage types `USE1-Fargate-vCPU-Hours:perCPU` and
  `USE1-Fargate-GB-Hours`.
- [Load balancer regional catalog][elb-prices], application load-balancer `LoadBalancerUsage`
  and `LCUUsage` (not Outposts, reserved capacity, or trust stores).
- [VPC regional catalog][vpc-prices], `USE1-PublicIPv4:InUseAddress`.
- [ECS Express Mode overview][express], generated HTTPS, underlying-resource pricing, availability.
- [ECS create API][create], custom task definitions and primary-container requirements.
- Refreshed ancillary catalogs: [Secrets Manager][secrets-prices], [SQS][sqs-prices],
  [CloudWatch][logs-prices], [ECR][ecr-prices], [S3][state-prices]. Standard S3 rates checked:
  $0.023/GB-month, $0.005/1,000 PUT and $0.004/10,000 GET.
- [App Runner notice](https://aws.amazon.com/apprunner/), closed to new customers April 30, 2026.

[ecs-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonECS/20260911124425/us-east-1/index.json
[elb-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AWSELB/20260911124544/us-east-1/index.json
[vpc-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonVPC/20260911124513/us-east-1/index.json
[secrets-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AWSSecretsManager/20260911124610/us-east-1/index.json
[sqs-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AWSQueueService/20260911124607/us-east-1/index.json
[logs-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonCloudWatch/20260915152028/us-east-1/index.json
[ecr-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonECR/20260911124425/us-east-1/index.json
[state-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonS3/20260911124507/us-east-1/index.json
[express]:
  https://docs.aws.amazon.com/AmazonECS/latest/developerguide/express-service-overview.html
[create]:
  https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_CreateExpressGatewayService.html
