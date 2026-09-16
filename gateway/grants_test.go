//go:build gatewayintegration

package main

import (
	"os"
	"testing"
)

// Check the grant boundary on a disposable database: absent/wrong migration state,
// wrong caller, repeated application, permitted DML, and excluded DDL/metadata/destructive work.
func TestRuntimeGrants(t *testing.T) {
	name := bootstrapFixture(t)
	bootstrap, err := os.ReadFile("../ops/database-bootstrap.sql")
	if err != nil {
		t.Fatal("bootstrap source unavailable", err)
	}
	bootstrapQuery(t, name, bootstrapQueryOptions{statement: string(bootstrap)})
	content, err := os.ReadFile("../ops/runtime-grants.sql")
	if err != nil {
		t.Fatal("grant source unavailable", err)
	}
	script := "SET ROLE p3_relay_migrator;" + string(content)
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: script, failure: "does not exist",
	})
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "SET ROLE p3_relay_migrator; BEGIN;" + bootstrapMigrationSQL(t) + "COMMIT;",
	})
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: string(content), failure: "reviewed database and schema owner",
	})
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "SET ROLE p3_relay_migrator; " +
			"INSERT INTO p3_relay.schema_migrations(version) VALUES(3);",
	})
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: script, failure: "migration versions one and two",
	})
	before := bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "SELECT has_table_privilege('p3_relay_runtime','p3_relay.events','SELECT');",
	})
	if before != "f" {
		t.Fatal("rejected grants changed runtime authority")
	}
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "SET ROLE p3_relay_migrator; " +
			"DELETE FROM p3_relay.schema_migrations WHERE version=3;",
	})
	bootstrapQuery(t, name, bootstrapQueryOptions{statement: script})
	bootstrapQuery(t, name, bootstrapQueryOptions{statement: script})
	grantsCheckRuntimeSQL(t, name)
}

func grantsCheckRuntimeSQL(t *testing.T, name string) {
	t.Helper()
	bootstrapQuery(t, name, bootstrapQueryOptions{statement: `SET ROLE p3_relay_runtime; BEGIN;
SELECT * FROM p3_relay.events;
SELECT * FROM p3_relay.delivery_attempts;
SELECT singleton FROM p3_relay.sandbox_budget;
UPDATE p3_relay.sandbox_budget SET admissions=admissions WHERE false;
INSERT INTO p3_relay.sandbox_sessions(id,expires_at)
VALUES('00000000000000000000000000000000',now()+interval '30 minutes');
UPDATE p3_relay.sandbox_sessions SET event_count=1;
DELETE FROM p3_relay.sandbox_sessions;
ROLLBACK;`})
	for _, statement := range []string{
		"CREATE TABLE p3_relay.forbidden(id int);",
		"SELECT * FROM p3_relay.schema_migrations;",
		"UPDATE p3_relay.delivery_attempts SET attempt_number=1 WHERE false;",
		"TRUNCATE p3_relay.events;",
		"INSERT INTO p3_relay.sandbox_budget VALUES(true,now(),0,0);",
		"DELETE FROM p3_relay.sandbox_budget WHERE false;",
	} {
		bootstrapQuery(t, name, bootstrapQueryOptions{
			statement: "SET ROLE p3_relay_runtime;" + statement, failure: "permission denied",
		})
	}
}
