# Controlled HTTPS demo receiver

Owner: Relay maintainer. Status: implemented, local TLS tests passed; live deployment pending.
Version: initial additive demo route. Existing configured receipt/delivery behavior is unchanged.

## Boundary and purpose

`POST /v1/demo-receiver` is enabled only when `RELAY_DEMO_RECEIVER_SECRET` is supplied to the API.
An invalid nonempty key (outside 16–256 bytes) fails startup. Deployment injects the existing delivery
key into this verifier, separately from the ingress key. No new service, dependency or secret value
is introduced into infrastructure state.

The route checks delivery HMAC-SHA256 using the existing signature library and raw bytes, a timestamp
within 300 seconds inclusive, a UUID event ID and a canonical decimal attempt from 1 through 8.
Duplicate authentication/metadata headers are rejected. Content type must be `application/json`, with
no query parameters. Body size is at most 256 KiB. It accepts a JSON object with a `scenario` string;
other fields are ignored and never returned or logged.

| Scenario | Response |
| --- | --- |
| `success` | 204 |
| `temporary_failure` | 503 for attempts 1–2, then 204 |
| `permanent_failure` | 422 |

These are synthetic outcomes, not a third-party integration. The actual HTTP/TLS transport and
signature checks are real. Event ID/attempt are **unsigned fixture metadata**, not authority to
perform a business operation. Authentication covers timestamp and body only, matching delivery v1.
There are no business effects, stored payloads, deduplication records or exactly-once claims.
Repeating a valid request within the timestamp window produces the same fixture outcome.

Failures return an empty body: 400 invalid input/read failure, 401 failed authentication or metadata,
405 unsupported method, 413 oversized body, 503 admission full. Responses have `Cache-Control:
no-store`. Audit records contain only fixed classifications/statuses, not keys, bodies, signatures
or event identifiers. At most four handlers read/verify payloads concurrently; excess requests fail
immediately without a queue. API header/read/write timeouts also apply. Secrets are copied into an
immutable handler-owned key and released with its process.

## Bootstrap and compatibility

An empty `RELAY_DESTINATION_URL` now starts the API with **new operator receipts disabled** rather
than failing startup. Authenticated receipt requests return the existing `503 receipt_unavailable`
without persistence; authentication failures remain failures. Endpoint inspection reports
`enabled:false`. Reads and existing event processing/replay retain their contracts. This is not a
general worker pause control. Nonempty configured destinations preserve prior behavior.

This controlled configuration cutover permits an initial Express deployment without placeholder URLs.
After its generated HTTPS endpoint is verified, set the destination to that origin plus
`/v1/demo-receiver`, validate its public DNS/TLS, then replace the task to load the new configuration.
Set the exact sandbox origin in the same reviewed rollout. No public application consumers existed
at this cutover. Future removal or semantic changes require a documented deployment cutover.

## Design tradeoffs and resource sketch

Admission decision: reuse the existing API deployment instead of another Lambda, Railway service or
public third-party receiver. Confidence: high that this avoids extra steady-state infrastructure;
capacity remains to be measured. The receiver shares the API's failure domain, so it does not prove
independent receiver availability. Public HTTPS round trips must still be verified after deployment.

Four maximum bodies retain roughly 1 MiB of payload, with allocator/JSON/TLS overhead beyond that;
allow several additional MiB when measuring API memory. Each request performs one bounded HMAC and
one JSON decode. No receiver database or disk I/O occurs. At 1,000 maximum-size requests/month,
request bodies total about 250 MiB; responses have no body. Network latency is dominated by the
AWS HTTPS round trip, not estimated here as a measurement. Keep workload/cost claims separate from
these planning assumptions.

Validation: `just check` covers every scenario/attempt combination, duplicates, wrong keys, modified
bodies, malformed/overflow timestamps, timestamp endpoints, exact/oversized payloads, reader errors,
full admission/recovery and bootstrap receipt rejection. A local TLS test exercises the real sender's
retry/success classifications. Live capacity, logs, HTTPS and persisted attempt evidence remain
activation/acceptance work, not conclusions from local tests.
