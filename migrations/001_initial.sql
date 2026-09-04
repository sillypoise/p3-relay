CREATE SCHEMA IF NOT EXISTS p3_relay;

CREATE TABLE p3_relay.schema_migrations (
    version integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE p3_relay.events (
    id uuid PRIMARY KEY,
    source_key text NOT NULL CHECK (length(source_key) BETWEEN 1 AND 128),
    external_event_id text NOT NULL CHECK (length(external_event_id) BETWEEN 1 AND 128),
    body bytea NOT NULL CHECK (octet_length(body) BETWEEN 1 AND 262144),
    body_sha256 bytea NOT NULL CHECK (octet_length(body_sha256) = 32),
    destination_url text NOT NULL CHECK (length(destination_url) BETWEEN 1 AND 2048),
    state text NOT NULL CHECK (state IN (
        'pending', 'delivering', 'delivered', 'retry_scheduled', 'dead_lettered'
    )),
    attempt_count smallint NOT NULL DEFAULT 0 CHECK (attempt_count BETWEEN 0 AND 8),
    replay_count smallint NOT NULL DEFAULT 0 CHECK (replay_count BETWEEN 0 AND 16),
    next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    lease_expires_at timestamptz,
    claim_id uuid,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    delivered_at timestamptz,
    UNIQUE (source_key, external_event_id),
    CHECK (state <> 'delivered' OR delivered_at IS NOT NULL),
    CHECK ((state = 'delivering') = (claim_id IS NOT NULL)),
    CHECK ((state = 'delivering') = (lease_expires_at IS NOT NULL))
);

CREATE INDEX events_delivery_claim_idx
    ON p3_relay.events (next_attempt_at, created_at)
    WHERE state IN ('pending', 'retry_scheduled');

CREATE TABLE p3_relay.delivery_attempts (
    id uuid PRIMARY KEY,
    event_id uuid NOT NULL REFERENCES p3_relay.events (id) ON DELETE RESTRICT,
    replay_number smallint NOT NULL CHECK (replay_number BETWEEN 0 AND 16),
    attempt_number smallint NOT NULL CHECK (attempt_number BETWEEN 1 AND 8),
    started_at timestamptz NOT NULL,
    finished_at timestamptz NOT NULL CHECK (finished_at >= started_at),
    outcome text NOT NULL CHECK (outcome IN (
        'delivered', 'retryable_http', 'terminal_http', 'network_error'
    )),
    status_code integer CHECK (status_code BETWEEN 100 AND 599),
    response_excerpt bytea NOT NULL CHECK (octet_length(response_excerpt) <= 4096),
    error_code text CHECK (error_code IS NULL OR length(error_code) BETWEEN 1 AND 64),
    UNIQUE (event_id, replay_number, attempt_number),
    CHECK (
        (status_code IS NOT NULL AND error_code IS NULL)
        OR (status_code IS NULL AND error_code IS NOT NULL)
    )
);

CREATE INDEX delivery_attempts_event_idx
    ON p3_relay.delivery_attempts (event_id, replay_number, attempt_number);

INSERT INTO p3_relay.schema_migrations (version) VALUES (1);
