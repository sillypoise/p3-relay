# SQS notification contract

Owner: Relay repository maintainer.
Status: Phase 6 application integration; live AWS resources and validation remain for Phase 7.
Version: notification envelope v1. Compatibility: additive; receipt responses remain unchanged.

## Authority and publication

PostgreSQL remains authoritative for receipt, ownership, due time, leases, attempts, and replay.
SQS standard-queue messages are hints to reconcile that state, not commands to deliver a particular
resource. Their only fields are:

```json
{"version":1,"event_id":"5a9c38c7-e229-4dad-a702-b03780ba69a7"}
```

The API publishes after successful database commit for new operator receipts, operator replay,
sandbox receipts, and sandbox replay. Idempotent duplicate receipts do not republish. Payloads,
source keys, destinations, secrets, visitor cookies, and response excerpts never enter SQS.

Publication has a one-second deadline and one SDK attempt. Failure emits a sanitized error event
but does not change a successful durable receipt or replay into a failure. A process can crash
between commit and publication: database reconciliation handles that gap.

Decision: reuse periodic database reconciliation instead of introducing an outbox and another
persistent dispatcher. This meets current recovery requirements with less state, at the cost of
polling and potentially slower recovery after a lost hint. Confidence: high for the locally tested
failure paths; AWS service behavior and operational latency still require deployment verification.

## Consumption, retry, and failure

- Receive at most 10 messages per call; long-poll for 10 seconds with a 12-second request deadline.
- Reject bodies over 1 KiB, unsupported versions, invalid UUIDs, and invalid receipt handles.
- Each cycle independently claims up to 10 due PostgreSQL events, even after receive failure or an
  empty receive. Event IDs inside notifications do not select resources or override state.
- Each claim/delivery/record operation has a 20-second deadline. The cycle has a 220-second cap.
- Delete valid hint handles in one batch only after the database work succeeds or finds no due work.
- A failed worker cycle retains its handles. Failed or partial deletion may redeliver; it cannot
  roll back committed attempts or force completed events into delivery again.
- Poison messages remain unacknowledged for the SQS redrive policy. Their content is never logged.
- Cancellation interrupts long-polling and leaves unfinished work to existing lease recovery.
- Retry timing stays in PostgreSQL, not SQS delay timers. No new hint is required for a scheduled
  retry to become eligible. Reordered, duplicate, stale, or lost hints cannot bypass claim rules.

A cycle may acknowledge more hints than events processed. This is intentional: acknowledgement
means the hint was considered, not that its named event was delivered. Remaining due work is found
on subsequent cycles. SQS visibility is 300 seconds, exceeding the bounded work and delete time.

PostgreSQL expiry retirement now handles up to 10 expired pending/retrying/recoverable-leased events
per claim call. An event at the 24-hour boundary cannot begin a new automatic attempt, even if an
old notification arrives. This fixes enforcement of the existing delivery-age requirement.

Without SQS configuration, the worker reconciles bounded batches on its one-second ticker. With
SQS enabled, idle reconciliation follows each long poll. Sandbox cleanup is checked between cycles
on a one-minute schedule; slow delivery batches can defer it by one bounded cycle. Admission still
fails closed at retained-session capacity, and expired sessions cannot authorize access.

## Queue configuration and IAM

Application startup validates these source-queue attributes before enabling SQS:

| Attribute | Required value |
| --- | --- |
| Queue type | Standard, not FIFO |
| Visibility timeout | 300 seconds |
| Long-poll wait | 10 seconds |
| Maximum message size | 1,024 bytes |
| Retention | 86,400 seconds |
| Encryption | SQS-managed SSE enabled |
| Redrive | Five receives, distinct DLQ in the same region/account |

Phase 7 must provision the DLQ with encryption, 14-day retention, and a redrive-allow policy limited
to this source queue. Notification DLQ entries are not delivery dead letters: the latter are
business delivery outcomes retained in PostgreSQL. Never wire automatic DLQ redrive to event replay.

Minimum runtime permissions, restricted to this queue ARN:

- API: `sqs:GetQueueAttributes`, `sqs:SendMessage`.
- Worker: `sqs:GetQueueAttributes`, `sqs:ReceiveMessage`, `sqs:DeleteMessage`.
- Neither runtime needs queue creation/deletion, purge, policy mutation, or DLQ redrive permissions.

The official AWS SDK is admitted to avoid bespoke SigV4 signing and credential renewal code.
Use ECS task roles in deployment. The AWS SDK credential chain is used; this phase does not copy
local credentials into application configuration. The client pins the commercial regional HTTPS
endpoint, rejects custom queue hosts and redirects, disables proxy overrides and SDK payload logs,
and limits SDK retries to one. Queue errors are sanitized before logging or returning to callers.

## Enablement and rollback

Set `RELAY_SQS_QUEUE_URL` and `RELAY_SQS_REGION` in API and worker environments. The URL must be
`https://sqs.<region>.amazonaws.com/<12-digit-account>/<standard-queue-name>`. Other partitions,
custom endpoints, FIFO queues, URL credentials, and query strings are not supported.

Leave both values empty for PostgreSQL-only operation. Partial or invalid configuration, rejected
AWS access, or invalid queue attributes stop startup; a configured but inaccessible queue is not
silently treated as disabled. Runtime outages retain database reconciliation and emit errors.

No database migration is introduced in Phase 6. Clear both settings and restart API/worker to roll
back to local polling without losing delivery state. During mixed-mode rollout, polling workers
still use the same PostgreSQL claims; old hints may be acknowledged or redriven later without
changing event state. Never purge a queue as a substitute for authorized database replay.

## Costs and evidence

An idle worker long-polling every roughly 10 seconds issues about 8,640 receive requests per day.
Each accepted event/replay adds one send; deletion batches amortize up to 10 handles per request.
A batch contains at most 10 KiB of configured message bodies, plus bounded SQS metadata. Worker CPU
and database operations are linear in the ten-event batch. These are sizing estimates, not measured
throughput, latency, or AWS cost claims.

Verified with `just check` and `just sandbox-integration`:

- Real SDK request signing and serialization against a local HTTP test server (synthetic keys).
- Queue/envelope rejection, duplicate hints, partial deletes, receive failures, and bounded batches.
- Post-commit publication visibility, publish failures preserving receipts/replay, and duplicate
  receipts not republishing.
- Real PostgreSQL delivery after lost hints, repeated reconciliation without duplicate terminal
  attempts, and exact delivery-age expiry.

No live AWS commands were run. Queue deployment, IAM enforcement, redrive behavior, runtime key
refresh, monitoring, and measured latency/cost remain Phase 7 verification items. Monitor sanitized
publish/receive/delete failures, rejected hints, DLQ depth, and overdue database work after rollout.
