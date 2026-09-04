# Relay Product Brief

## Status and ownership

- Status: Normative Phase 1 product contract.
- Owner: Relay repository maintainer.
- Compatibility: Initial contract; no previous consumers exist.

## Problem

Teams that send webhooks need to know that accepted events are delivered despite temporary receiver
failures. A basic HTTP request does not provide durable receipt, bounded retries, delivery history,
or a safe recovery path.

## Capability proof

Relay will prove one complete flow:

```text
signed event receipt
→ validation and durable persistence
→ asynchronous delivery
→ recorded attempt
→ success, bounded retry, or dead-letter state
→ inspection and authorized replay
```

The proof is the working flow and its deterministic failure cases, not dashboard statistics alone.

## Users

- An integrating system sends signed webhook events to Relay.
- An operator configures a destination, inspects attempts, and replays dead-lettered events.
- A portfolio visitor uses an isolated, quota-limited sandbox backed by clearly labeled simulated
  destinations.

A portfolio visitor is not an operator and cannot configure arbitrary destination URLs or access
another visitor's resources.

## Required product behavior

Relay must:

1. Validate authentication, identifiers, timestamps, content type, and body size before persistence.
2. Persist each accepted event before acknowledging it.
3. Deduplicate retries from senders without creating duplicate deliveries.
4. Deliver the original JSON body to the configured destination with a Relay signature.
5. Record each bounded delivery attempt and its outcome.
6. Retry only recoverable failures according to the delivery contract.
7. Move exhausted or expired events to a visible dead-letter state.
8. Permit resource-scoped, authorized replay of dead-lettered events.
9. Expose overview, event list, event detail, and endpoint configuration views.
10. Make loading, empty, invalid-input, denied, failure, and recovery states visible.

## Public demo boundary

The public demo will provide each visitor an isolated sandbox with a signed, secure session cookie.
Sandbox resources will expire, and event creation will have fixed per-session and service-wide
limits. Visitors can select controlled receiver scenarios: immediate success, temporary failure,
and permanent failure. The UI and documentation must label these receivers and seeded data as
simulated.

Anonymous visitors cannot supply destination URLs, invoke operator endpoints, reset global data, or
replay another session's events. If safe isolation and quotas are not ready, the deployment must
fall back to a read-only public view rather than expose shared mutation controls.

## Screens

1. **Overview:** Delivery state totals and recent activity.
2. **Events:** Bounded event list with state and endpoint filters.
3. **Event detail:** Metadata, attempt timeline, bounded response excerpts, and replay action.
4. **Endpoint:** Destination status and secret-rotation controls; secrets are never redisplayed.

## Out of scope

- Production-scale multi-tenancy, billing, teams, or role administration.
- Arbitrary anonymous destination URLs.
- Payload transformation and webhook schema design.
- Ordering guarantees between distinct events.
- Exactly-once network delivery.
- Kubernetes, multi-region operation, Kafka, and generalized queue plugins.
- Claims of production traffic, customer outcomes, or unmeasured performance.

## Acceptance criteria

The first shipped release is complete when:

- The vertical flow works locally and in the public deployment.
- Tests demonstrate valid, invalid, boundary, authorization, retry, dead-letter, and recovery paths.
- Concurrent or duplicate processing cannot create an invalid state transition.
- Public visitors cannot cross session boundaries or select arbitrary network destinations.
- The four screens are usable at declared mobile and desktop widths.
- `just check` passes without ignored failures.
- OpenTofu formatting and validation pass, and deployment uses a reviewed plan.
- Setup, operation, rollback, teardown, simulation, and evidence limitations are documented.

## Significant decisions

- **Decision:** PostgreSQL remains the source of truth; SQS later carries delivery notifications.
  **Reason:** At-least-once queue delivery must not determine authoritative event state.
- **Decision:** All database objects use the explicitly qualified `p3_relay` schema.
  **Reason:** The Railway PostgreSQL instance is shared by independent portfolio projects.
- **Decision:** Prove a PostgreSQL-backed local worker before adding AWS.
  **Reason:** This isolates delivery correctness from deployment complexity.
- **Decision:** Use controlled receivers for anonymous visitors.
  **Reason:** Arbitrary destinations would turn the demo into an SSRF and traffic-proxy surface.
- **Alternative considered:** An operator-only interactive deployment with a public recording. This
  is safer and simpler, but provides weaker hands-on evidence; it remains the fallback if sandbox
  controls cannot be verified.
