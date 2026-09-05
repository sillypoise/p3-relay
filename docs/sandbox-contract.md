# Public sandbox contract

Owner: Relay repository maintainer.
Status: Implemented Phase 5 contract; disabled until deployment configuration is supplied.
Compatibility: Additive to operator APIs. Visitor credentials never authorize operator routes.

## Identity and authorization

- Use a distinct 32-byte signing key, supplied through runtime configuration, never frontend code.
- Issue a random 128-bit visitor identifier in an HMAC-SHA256 signed cookie.
- Cookie: `__Host-relay_sandbox`, Secure, HttpOnly, SameSite=Strict, Path=/, no Domain.
- Expire after 30 minutes without sliding renewal. Reject at the exact expiry boundary.
- Persist ownership before issuing a cookie. Signature validation alone grants no resource access.
- Verify database session existence and expiry on every read and mutation; fail closed on errors.
- Rotating the signing key invalidates existing sessions; previous keys are not accepted.
- POST requests require an exact configured Origin and JSON content type. Forwarded headers do not
  determine the allowed origin. Browser access requires HTTPS; cookie controls are never weakened.
- Visitor requests carrying an Authorization header are rejected. Operator routes continue to
  require their own bearer token. The `sandbox:` source namespace is reserved server-side.

## API boundary

All routes are under `/v1/sandbox`. Responses include `Cache-Control: no-store`.

- `POST /session`, body `{}`: persist a session and set its cookie; return `201 {"active":true}`.
  A valid existing session returns `200` without renewing expiry or resetting quotas.
- `GET /events`: return `{ "events": [...] }` with up to 20 session-owned events.
- `POST /events`, body `{ "scenario": "success" }`: generate synthetic JSON server-side; return
  `202` with `event_id` and `pending` status after durable receipt. Unknown fields are rejected.
- `GET /events/{id}`: read only a session-owned event; missing and foreign events both return `404`.
- `POST /events/{id}/replay`, body `{}`: replay only an owned dead-lettered event within its quota;
  return `202 {"status":"pending"}`.

Accepted scenarios: `success`, `temporary_failure`, `permanent_failure`. POST bodies are limited to
1 KiB and one JSON value. No URL, payload, source key, or ownership override is accepted.

Stable failures use the existing error envelope (`code`, `message`, `request_id`):
`400 invalid_request`, `401 invalid_session`, `403 invalid_origin`, `404 event_not_found`,
`409 invalid_event_state`, `429 quota_exceeded`, `503 sandbox_unavailable`.
Errors and audit messages exclude tokens and internal database/network details.

## Resource bounds and lifecycle

PostgreSQL transactions serialize admission across replicas:

- 20 events per session, each with less than 1 KiB of generated JSON.
- Two replays per event, preserving prior generations and consuming the global admission budget.
- 100 new sessions and 1,000 event/replay admissions per UTC hour across all API replicas.
- At most 1,000 retained sessions; admission fails if cleanup falls behind.
- Clearing cookies never resets the global budget. There is no public reset operation.
- Expired sessions cannot access their resources, and their events are excluded from worker claims.
- Cleanup deletes attempts, events, then empty sessions in batches of at most 100 events, skipping
  active leases. It runs before new session admission; the worker checks its one-minute schedule
  between bounded delivery cycles.
- Hourly quota counters survive cleanup. A missing counter row fails closed.
- Requests have a five-second database deadline; failed transactions do not consume partial quotas.

The payload allowance is at most 20 MiB. Attempt excerpts have a conservative upper bound of
20,000 events × 24 attempts × 4 KiB ≈ 1.8 GiB, so cleanup and admission capacity are required.
Actual simulator excerpts are much shorter. These are bounds, not performance benchmarks.

## Simulation boundary and tradeoff

Public receiver responses execute **inside the worker without network access**. PostgreSQL receipt,
claiming, retry schedules, dead-letter transitions, replay, and attempt persistence are real.
Immediate success returns simulated 204; temporary failure returns simulated 503 twice then 204;
permanent failure returns simulated 422. UI and attempts explicitly identify simulation.

Decision: admit an in-process receiver instead of a public HTTP receiver for visitor traffic. It
removes anonymous outbound traffic and private-network exceptions, but does not demonstrate HTTP
signing or transport behavior. The separate operator/local receiver flow demonstrates those paths.
The database-owned sandbox marker selects simulation; request bodies cannot select the transport.

## Migration and rollout

Migration 002 adds sandbox tables and nullable ownership in `p3_relay`, preserving operator events.
Stop API and worker processes, run `just database-migrate`, then start the new binaries together.
Do not run old workers alongside sandbox traffic: they lack expiry handling.
Migration re-execution is safe and serialized with a project-specific advisory lock.

Omit `RELAY_SANDBOX_ORIGIN` to leave visitor routes disabled. To enable, configure an exact HTTPS
origin without a trailing slash and a fresh random 32-byte key encoded as 64 hex characters in
`RELAY_SANDBOX_KEY`. Supply both through approved runtime environment configuration, never frontend
assets or committed files. Serve the UI and API under that same origin through HTTPS termination.
The Vite HTTP development URL alone is not a supported live sandbox origin.

Disable visitor routes by clearing the origin and restarting the API; the worker continues expiry
cleanup. Do not roll the database back or delete other project schemas. Key rotation also requires
an API restart and expires all existing visitor credentials. No live Railway changes are part of
Phase 5 verification.

## Reproducible verification

- `just check`: formatting, unit tests with race detection, lint, type checking, builds.
- `just sandbox-integration`: requires `RELAY_TEST_DATABASE_URL` pointing to an **empty disposable**
  database named `p3_relay_test`. It refuses a populated Relay schema; never use shared Railway.
- `just browser-install`, then `just browser-test`: Chromium UI tests at 390×844 and 1440×900.

Integration tests cover TLS cookie round-trips, two-visitor isolation, injection, origin rejection,
concurrent session quotas, global quotas, retry persistence, stale-worker recovery, replay history,
expiry, active-lease cleanup, and database failure. Browser tests use mocked API responses for
empty, quota, expiry, unavailable states, and layout boundaries. They do not independently prove
database isolation or a deployed HTTPS configuration.
