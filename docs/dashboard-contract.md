# Relay Dashboard Contract

## Contract metadata

- Owner: Relay repository maintainer.
- Status: Normative initial operator-read contract.
- Version: `v1`.
- Compatibility: Initial contract. Additive fields must preserve existing consumers; removals or
  semantic changes require a versioned cutover.

## Authorization

Every dashboard request must provide `Authorization: Bearer <operator-token>`. Authorization is
source- and action-scoped by the server. Missing, malformed, expired, or incorrect credentials
return `401 unauthorized` and must not execute a database operation.

The local frontend asks the operator for this token and retains it only in tab-scoped session
storage. A token must never be compiled into frontend assets or returned by an API.

## Read boundaries

- `GET /v1/overview` returns counts for `pending`, `delivering`, `delivered`, `retrying`, and
  `dead_lettered` events.
- `GET /v1/events` returns at most 50 newest events. Its optional `state` query must be one of the
  delivery contract states.
- `GET /v1/events/{event_id}` returns event metadata and chronological attempts, or
  `404 event_not_found`.
- `GET /v1/endpoint` returns the source key, destination URL, enabled state, and the literal secret
  disposition `write_only`. It must not return secret material.

Read persistence failures return `503 read_unavailable` without database or network details.
Malformed identifiers and filters return `400 invalid_request`.

## Replay boundary

`POST /v1/events/{event_id}/replay` follows the authorization and state rules in the
[delivery contract](./delivery-contract.md). Success returns HTTP `202` with the event ID and
`pending` state.

## Compatibility classification

This contract is additive to the Phase 1 receipt contract and does not change existing receipt or
delivery behavior. The frontend and API are owned by this repository and currently deploy together.

## Required checks

Tests must cover authorized and unauthorized reads, malformed filters and identifiers, empty lists,
missing events, persistence failures, secret omission, permitted replay, and denied replay.
