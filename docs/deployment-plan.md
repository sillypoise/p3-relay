# Deployment preparation

Owner: Relay repository maintainer.
Status: Phase 7 in progress. No infrastructure has been provisioned.

## Confirmed constraints

- AWS command wrapper: `aws-run sp aws <arguments>` in an interactive Zsh shell. The wrapper takes
  the whole command, so `aws-run sp sts ...` is not valid. STS identity verification succeeded.
- Configured region: `us-east-1`.
- Operator budget ceiling: USD 50/month for AWS; minimize spend rather than spending to the limit.
- PostgreSQL remains on shared Railway, with every Relay object in `p3_relay`. No RDS instance.
- Public hostname is still undecided. A hostname means an address such as `relay.example.com` under
  a domain owned by the operator; it is needed for DNS, HTTPS, and the sandbox's exact Origin check.

## Candidate topology, not yet approved for apply

Prefer one small ECS Fargate task with separate API and worker containers, using the same image.
Serve compiled frontend assets from the Go API so the browser and API share one HTTPS origin.
Use an HTTPS load balancer, SQS with a notification DLQ, narrowly scoped task roles, and short log
retention. Avoid RDS, NAT gateways, a separate frontend service, and autoscaling for this demo.

Tradeoff: sharing a task lowers baseline cost but couples API/worker restarts and resource capacity.
PostgreSQL leases recover unfinished delivery work after restart. Two independent ECS services
remain an alternative only if the reviewed estimate leaves adequate budget headroom.

Cost verification is pending: the AWS Pricing endpoint timed out, including a bounded retry with
one attempt and explicit connection/read timeouts. Do not treat the candidate topology as a current
AWS quote or guarantee it fits the budget. Prepare a regional estimate with headroom for IPv4,
load-balancer usage, logs, images, secrets, SQS, and transfer before any apply. Budget alerts do not
constitute a hard billing cap; variable usage and taxes may affect the final bill.

OpenTofu must define project-owned resources and produce a reviewed plan before apply. Runtime
secrets must be referenced from controlled secret storage, never baked into images or stored in
OpenTofu variables/state. State storage, IAM, HTTPS, database TLS, rollback, and teardown still need
implementation and verification after hostname and cost decisions.

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
