-- Owner: Relay maintainer. Run once as the existing database administrator.
-- This contains no passwords and leaves both roles unable to log in. Existing names cause rollback,
-- not adoption or credential replacement. Only Relay's new schema and roles may be changed.
BEGIN;
SET LOCAL statement_timeout = '5s';
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(330052);
DO $relay_database$
BEGIN
    IF current_database() <> 'railway' THEN
        RAISE EXCEPTION 'Relay bootstrap requires the railway database';
    END IF;
END
$relay_database$;

CREATE ROLE p3_relay_runtime NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE
    NOREPLICATION NOBYPASSRLS NOINHERIT CONNECTION LIMIT 24;
CREATE ROLE p3_relay_migrator NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE
    NOREPLICATION NOBYPASSRLS NOINHERIT CONNECTION LIMIT 1;
CREATE SCHEMA p3_relay AUTHORIZATION p3_relay_migrator;
REVOKE ALL ON SCHEMA p3_relay FROM PUBLIC;
GRANT USAGE ON SCHEMA p3_relay TO p3_relay_runtime;
ALTER ROLE p3_relay_runtime SET search_path = pg_catalog, p3_relay;
ALTER ROLE p3_relay_migrator SET search_path = pg_catalog, p3_relay;
ALTER ROLE p3_relay_runtime SET statement_timeout = '30s';
ALTER ROLE p3_relay_migrator SET statement_timeout = '30s';
ALTER ROLE p3_relay_runtime SET idle_in_transaction_session_timeout = '30s';
ALTER ROLE p3_relay_migrator SET idle_in_transaction_session_timeout = '30s';

-- PostgreSQL has no per-role DENY overriding PUBLIC grants. Abort if existing shared grants
-- would silently broaden either newly created identity; do not repair other projects here.
DO $relay_bounds$
DECLARE identity_name text;
BEGIN
    FOREACH identity_name IN ARRAY ARRAY['p3_relay_runtime', 'p3_relay_migrator'] LOOP
        IF has_database_privilege(identity_name, current_database(), 'CREATE') THEN
            RAISE EXCEPTION 'Relay role would have database CREATE authority';
        END IF;
        IF EXISTS (
            SELECT 1 FROM pg_namespace n
            WHERE n.nspname !~ '^pg_' AND n.nspname <> 'information_schema'
                AND n.nspname <> 'p3_relay'
                AND has_schema_privilege(identity_name, n.oid, 'CREATE')
        ) THEN
            RAISE EXCEPTION 'Relay role would have cross-schema CREATE authority';
        END IF;
        IF EXISTS (
            SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname !~ '^pg_' AND n.nspname <> 'information_schema'
                AND n.nspname <> 'p3_relay' AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
                AND has_table_privilege(identity_name, c.oid,
                    'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
        ) THEN
            RAISE EXCEPTION 'Relay role would have cross-schema table authority';
        END IF;
        IF EXISTS (
            SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
            WHERE n.nspname !~ '^pg_' AND n.nspname <> 'information_schema'
                AND p.prosecdef AND has_function_privilege(identity_name, p.oid, 'EXECUTE')
        ) THEN
            RAISE EXCEPTION 'Relay role would have application SECURITY DEFINER access';
        END IF;
    END LOOP;
    IF has_schema_privilege('p3_relay_runtime', 'p3_relay', 'CREATE') THEN
        RAISE EXCEPTION 'Runtime role must not own DDL';
    END IF;
    IF NOT has_schema_privilege('p3_relay_migrator', 'p3_relay', 'CREATE') THEN
        RAISE EXCEPTION 'Migrator must own Relay DDL';
    END IF;
END
$relay_bounds$;
COMMIT;
