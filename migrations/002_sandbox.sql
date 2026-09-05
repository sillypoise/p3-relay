CREATE TABLE p3_relay.sandbox_sessions (
    id text PRIMARY KEY CHECK (id ~ '^[0-9a-f]{32}$'),
    expires_at timestamptz NOT NULL,
    event_count smallint NOT NULL DEFAULT 0 CHECK (event_count BETWEEN 0 AND 20)
);
CREATE INDEX sandbox_sessions_expiry_idx ON p3_relay.sandbox_sessions (expires_at);
ALTER TABLE p3_relay.events ADD COLUMN sandbox_id text
    REFERENCES p3_relay.sandbox_sessions(id);
CREATE INDEX events_sandbox_idx ON p3_relay.events (sandbox_id);
CREATE TABLE p3_relay.sandbox_budget (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    window_start timestamptz NOT NULL,
    sessions integer NOT NULL CHECK (sessions BETWEEN 0 AND 100),
    admissions integer NOT NULL CHECK (admissions BETWEEN 0 AND 1000)
);
INSERT INTO p3_relay.sandbox_budget VALUES (
    true, date_trunc('hour', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC', 0, 0
);
INSERT INTO p3_relay.schema_migrations (version) VALUES (2);
