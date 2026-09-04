# Relay Delivery Contract

## Contract metadata

- Owner: Relay repository maintainer.
- Status: Normative initial contract.
- Version: `v1`.
- Compatibility: No previous consumers. Future additive fields must preserve existing behavior;
  removals or semantic changes require a versioned cutover and migration note.
- Database namespace: Every object and query must explicitly use the `p3_relay` schema on the shared
  Railway PostgreSQL instance.

Examples in this document are illustrative. Statements using **must** define required behavior.

## Event receipt boundary

A source submits an event with `POST /v1/sources/{source_key}/events`.

Required request properties:

- `Content-Type` must be `application/json`.
- The raw body must contain one valid JSON value and be between 1 byte and 256 KiB inclusive.
- `X-Relay-Event-Id` must contain 1–128 printable ASCII characters.
- `X-Relay-Timestamp` must be Unix seconds within 300 seconds of server time.
- `X-Relay-Signature` must be `v1=<lowercase hex HMAC-SHA256>` over
  `<timestamp>.<raw-request-body>` using the source's ingress secret.

Relay must validate all properties before persistence. Signature comparison must be constant-time.
The source key and secret are credentials and must not appear in logs or error bodies.

On first acceptance, Relay returns HTTP `202` with:

```json
{"event_id":"evt_...","status":"pending","duplicate":false}
```

The unique idempotency key is `(source_id, X-Relay-Event-Id)`. Repeating the same key and body
returns `202` with the original Relay event ID and `duplicate: true`. Reusing the key with a
different body returns `409 idempotency_conflict`. Relay compares a stored SHA-256 body digest to
detect conflict.

A `202` response guarantees durable persistence, not successful destination delivery. Relay must not
acknowledge an event if persistence fails.

## Stable receipt errors

Errors use this bounded shape:

```json
{
  "error": {
    "code": "invalid_signature",
    "message": "Request authentication failed.",
    "request_id": "req_..."
  }
}
```

The stable `code` values are:

| HTTP | Code | Meaning |
| --- | --- | --- |
| 400 | `invalid_request` | Headers, JSON, identifier, or timestamp syntax is invalid. |
| 401 | `invalid_signature` | Source credentials or signature validation failed. |
| 404 | `source_not_found` | No enabled source is available. |
| 409 | `idempotency_conflict` | An event ID was reused with a different body. |
| 413 | `payload_too_large` | The body exceeds 256 KiB. |
| 415 | `unsupported_media_type` | The content type is not JSON. |
| 429 | `rate_limited` | A bounded source or sandbox quota was exceeded. |
| 503 | `receipt_unavailable` | Durable receipt could not be completed. |

Error messages must not include secrets, SQL errors, stack traces, network addresses, or signature
comparison details. Unknown source keys and bad signatures may share externally indistinguishable
responses in deployment to limit credential discovery.

## Delivery request boundary

Relay sends the original raw JSON body with `POST` to the endpoint's configured HTTPS URL. Redirects
must not be followed. The request contains:

- `Content-Type: application/json`
- `User-Agent: Relay/1`
- `X-Relay-Event-Id: <stable Relay event ID>`
- `X-Relay-Attempt: <one-based attempt number>`
- `X-Relay-Timestamp: <Unix seconds>`
- `X-Relay-Signature: v1=<HMAC-SHA256>`

The delivery signature covers `<timestamp>.<raw-request-body>` using the endpoint's delivery secret.
The event ID remains stable across attempts. Receivers must use it for idempotency because network
exactly-once delivery is not promised.

## Network safety and limits

Before saving an operator-configured destination and before each connection, Relay must enforce:

- HTTPS in deployed environments.
- No loopback, private, link-local, multicast, unspecified, or cloud metadata destination.
- DNS resolution to allowed addresses only, including every address considered for connection.
- No redirects.
- A 3-second connection timeout and 10-second total request timeout.
- At most 4 KiB of response body retained as an attempt excerpt.

Sandbox visitors can select only project-owned receiver scenarios and cannot submit a URL.
Destination validation failures must fail closed without a network request.

## Attempt classification

A response from `200` through `299` completes the event as `delivered`.

The following outcomes are recoverable and scheduled for retry:

- connection failure or timeout;
- HTTP `408` or `429`;
- HTTP `500` through `599`.

Other HTTP responses are terminal and move the event directly to `dead_lettered`. Exhausting the
attempt limit or exceeding event age also moves it to `dead_lettered`. Every attempted request must
produce one durable attempt record, including failures without an HTTP response.

## Bounded retry schedule

An event has at most eight attempts. Nominal delays before attempts 2–8 are:

```text
5 seconds, 30 seconds, 2 minutes, 10 minutes, 30 minutes, 2 hours, 6 hours
```

A deterministic offset in the inclusive range of minus 20% through plus 20% must be derived from the
event ID and attempt number. Retry timing must never precede the previous attempt. An event older
than 24 hours must not start another automatic attempt.

The initial implementation does not honor `Retry-After`; adding it would change timing semantics and
requires a documented contract revision.

## State contract

Allowed automatic transitions are:

```text
pending → delivering
delivering → delivered
delivering → retry_scheduled
retry_scheduled → delivering
delivering → dead_lettered
retry_scheduled → dead_lettered
```

An authorized replay creates `dead_lettered → pending`, resets the automatic-attempt budget, and
preserves previous attempt history. An event permits at most 16 replays. Replay is rejected for all
other states and after replay exhaustion. State claims have a
30-second lease; recovery after an expired lease must preserve the eight-attempt bound and may
produce a duplicate network request. Database transitions must use compare-and-set conditions so a
stale worker cannot overwrite newer state.

SQS integration must assume duplicate and out-of-order messages. A queue message is only a prompt to
claim current PostgreSQL state; it grants no authority to force a transition.

## Authorization contract

Receipt authentication is source-scoped. Dashboard reads, endpoint mutation, secret rotation, and
replay require explicit action- and resource-scoped authorization. Secrets are returned only once at
creation or rotation and are otherwise write-only.

A sandbox session can read and replay only its own events. It cannot rotate operator secrets,
configure URLs, or invoke administrative reset operations. Missing, expired, malformed, or
unverifiable authorization must deny the operation.

## Required contract tests

Tests must cover:

- minimum, maximum, empty, malformed, and oversized bodies;
- valid, malformed, stale, future, and incorrect signatures;
- duplicate IDs with identical and different bodies;
- persistence failure before acknowledgement;
- each success, retryable, and terminal response class;
- connection timeout, response truncation, redirect refusal, and prohibited destination addresses;
- attempts 1 and 8, retry-age expiry, lease expiry, stale workers, and concurrent claims;
- replay from allowed and disallowed states;
- absent, expired, cross-resource, and insufficient authorization;
- duplicate and out-of-order queue notifications.
