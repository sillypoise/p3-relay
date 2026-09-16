# Initial runtime capacity check

Owner: Relay maintainer. Measured 2026-09-16 against application source
`a3dd9b79ffe6fcb91d00077acc05d030d7483c88` built with `just container-build`.

## Method and limits

A disposable, internal Podman network contained PostgreSQL 18.3-alpine and separate API/worker
containers. It did not connect to Railway or AWS. The API published only a random loopback port.
Each application container had a 256 MiB hard limit, 0.125 CPU quota, `GOMEMLIMIT=160MiB`, read-only
rootfs, dropped capabilities and no-new-privileges. PostgreSQL had its own 256 MiB/one-CPU limit.
Application CPU quotas sum to the proposed 0.25-vCPU task; separate hard quotas are not identical to
Fargate's shared CPU weights. All fixture containers and the network were removed after inspection.

Reproduction procedure:

1. Build the current image using `just container-build`. Use a fresh internal Podman network and
   PostgreSQL database, not a portfolio database. Run the packaged migrator against that fixture.
2. Start the image's API/worker with the limits above. Use only synthetic source/signing/operator
   keys. Set the API destination to the fixture API's `/v1/demo-receiver`; set the receiver and worker
   delivery keys identically. Disable SQS and allow private HTTP **only inside this isolated fixture**.
3. Send 20 distinct signed receipts with four concurrent clients and 15-second client timeouts.
   Each raw JSON body is exactly 262144 bytes: `scenario:success` plus a string `padding` field.
   Sign timestamp-dot-raw-body per delivery contract; every receipt must return 202.
4. Read the packaged landing page. Poll the disposable database for at most 90 iterations/one-second
   pauses, requiring 20 delivered events and 20 delivered attempts. Inspect cgroup v2
   `memory.current`, `memory.peak`, `memory.max` and container running/OOM state before teardown.
5. Reject sizing if either peak reaches the 160 MiB planning threshold or any request/delivery fails.
   Always remove only fixture resources, including on failure.

This was a short memory/admission check, **not a throughput benchmark or production capacity rating**.
The run used local HTTP/direct PostgreSQL without SQS, gateway, public TLS or UI browser allocation.
The separate receiver TLS test covers signatures/HTTP classifications, not that missing overhead.

## Observations

| Item | Observed result |
| --- | --- |
| API current / peak | 8,376,320 / 11,169,792 bytes |
| Worker current / peak | 7,213,056 / 7,528,448 bytes |
| Per-container hard limit | 268,435,456 bytes |
| Signed receipt batch elapsed | 0.590 seconds |
| Elapsed through delivered-state check | 4.760 seconds |
| Persisted events / delivered attempts | 20 / 20 |
| Container outcome | Both running; neither OOM-killed |

Decision: admit the proposed initial low-traffic 0.25-vCPU/512-MiB task with two 256-MiB containers.
Confidence: medium; the local margin supports an initial deployment, not sustained AWS capacity.
Recheck actual ECS memory/CPU, public TLS/SQS behavior and failure recovery after launch. Unexpected
resource growth or memory pressure invalidates this sizing decision; do not silently scale replicas.
