-- Owner: Relay maintainer. Run as the restricted schema owner after migration version 2.
-- Deliberately name tables and actions: no default privileges or schema-wide future grants.
BEGIN;
SET LOCAL statement_timeout = '5s';
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(330052);
DO $relay_grants$
BEGIN
    IF current_database() <> 'railway' OR current_user <> 'p3_relay_migrator' THEN
        RAISE EXCEPTION 'Runtime grants require the reviewed database and schema owner';
    END IF;
    -- Two expected versions plus one overflow row suffice to reject unsupported migration state.
    IF (SELECT array_agg(version ORDER BY version) FROM (
        SELECT version FROM p3_relay.schema_migrations ORDER BY version LIMIT 3
    ) AS versions) IS DISTINCT FROM ARRAY[1, 2] THEN
        RAISE EXCEPTION 'Runtime grants require migration versions one and two';
    END IF;
    IF has_schema_privilege('p3_relay_runtime', 'p3_relay', 'CREATE') THEN
        RAISE EXCEPTION 'Runtime must not have schema CREATE authority';
    END IF;
    IF has_table_privilege('p3_relay_runtime', 'p3_relay.schema_migrations',
        'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER') THEN
        RAISE EXCEPTION 'Runtime must not have migration metadata authority';
    END IF;
END
$relay_grants$;

GRANT SELECT, INSERT, UPDATE, DELETE ON p3_relay.events TO p3_relay_runtime;
GRANT SELECT, INSERT, DELETE ON p3_relay.delivery_attempts TO p3_relay_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON p3_relay.sandbox_sessions TO p3_relay_runtime;
GRANT SELECT, UPDATE ON p3_relay.sandbox_budget TO p3_relay_runtime;
COMMIT;
