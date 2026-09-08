# AWS cost estimate — deployment review input

Owner: repository maintainer. Status: preliminary, not an approved full deployment quote.
Evidence collected 2026-09-05 from AWS regional price catalogs published 2026-08-31.

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

The remainder must cover SQS requests, CloudWatch logs/alarms, two Secrets Manager secrets, ECR,
S3 state, transfer, one-off migrations, rollout overlap, and taxes. Their full estimate remains
pending. The operator supplied the private notification destination; the $50 monthly account-wide
budget is deployed, alerting above $35/$45/$50 actual cost and $50 forecast cost. Inbox delivery is
not verified. Keep a low-traffic target around $45 before taxes, but do not claim a $50 hard cap:
traffic, retained images, address allocation, and rollout duration can increase the bill.

Do not apply the runtime plan until it confirms one steady-state task, no NAT gateway, the intended
IPv4/ALB allocation, and adequate remaining headroom. Recheck rates and measured memory before
apply, after changing topology, or if the load/retention assumptions change. Limit task scaling to
one; budget alerts supplement resource limits but cannot guarantee a hard billing ceiling.

## Reviewer-visible sources

- [Fargate regional catalog][ecs-prices], usage types `USE1-Fargate-vCPU-Hours:perCPU` and
  `USE1-Fargate-GB-Hours`.
- [Load balancer regional catalog][elb-prices], application load-balancer `LoadBalancerUsage`
  and `LCUUsage` (not Outposts, reserved capacity, or trust stores).
- [VPC regional catalog][vpc-prices], `USE1-PublicIPv4:InUseAddress`.
- [ECS Express Mode overview][express], generated HTTPS, underlying-resource pricing, availability.
- [ECS create API][create], custom task definitions and primary-container requirements.
- [App Runner notice](https://aws.amazon.com/apprunner/), closed to new customers April 30, 2026.

[ecs-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonECS/20260831092155/us-east-1/index.json
[elb-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AWSELB/20260831092255/us-east-1/index.json
[vpc-prices]:
  https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonVPC/20260831092232/us-east-1/index.json
[express]:
  https://docs.aws.amazon.com/AmazonECS/latest/developerguide/express-service-overview.html
[create]:
  https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_CreateExpressGatewayService.html
