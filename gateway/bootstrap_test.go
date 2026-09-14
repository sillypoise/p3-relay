//go:build gatewayintegration

package main

import (
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type bootstrapQueryOptions struct {
	database, statement, failure string
}

type bootstrapGrantFixture struct {
	container, script string
}

// A network-isolated disposable database exercises bounds and rollback after rejected targets,
// duplicate names, denied administration and unsafe PUBLIC grants. No Railway credentials are read.
func TestDatabaseBootstrap(t *testing.T) {
	name := bootstrapFixture(t)
	content, err := os.ReadFile("../ops/database-bootstrap.sql")
	if err != nil {
		t.Fatal("bootstrap source unavailable", err)
	}
	script := string(content)
	bootstrapQuery(t, name, bootstrapQueryOptions{statement: "CREATE DATABASE wrong_target;"})
	bootstrapQuery(t, name, bootstrapQueryOptions{
		database: "wrong_target", statement: script, failure: "requires the railway database",
	})
	bootstrapAbsent(t, name)
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "SET ROLE pg_read_all_data;" + script, failure: "permission denied",
	})
	bootstrapAbsent(t, name)
	bootstrapQuery(t, name, bootstrapQueryOptions{statement: script})
	roles := bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "SELECT rolname,rolcanlogin,rolconnlimit FROM pg_roles " +
			"WHERE rolname IN ('p3_relay_runtime','p3_relay_migrator') ORDER BY rolname;",
	})
	if roles != "p3_relay_migrator|f|1\np3_relay_runtime|f|24" {
		t.Fatal("role admission or connection bounds differ")
	}
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "SET ROLE p3_relay_migrator; BEGIN;" + bootstrapMigrationSQL(t) + "ROLLBACK;",
	})
	bootstrapQuery(t, name, bootstrapQueryOptions{statement: script, failure: "already exists"})
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "DROP SCHEMA p3_relay; DROP ROLE p3_relay_runtime,p3_relay_migrator;",
	})
	bootstrapRejectGrants(t, &bootstrapGrantFixture{container: name, script: script})
}

// Preserve administrator-led fresh local installs while requiring prior bootstrap for scoped roles.
func TestMigrationSchemaPermissions(t *testing.T) {
	name := bootstrapFixture(t)
	script := bootstrapMigrationSQL(t)
	bootstrapQuery(t, name, bootstrapQueryOptions{statement: "BEGIN;" + script + "ROLLBACK;"})
	bootstrapAbsent(t, name)
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "CREATE ROLE p3_relay_migrator NOLOGIN;",
	})
	bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "SET ROLE p3_relay_migrator; BEGIN;" + script,
		failure:   "permission denied for database",
	})
	bootstrapQuery(t, name, bootstrapQueryOptions{statement: "DROP ROLE p3_relay_migrator;"})
	bootstrapAbsent(t, name)
}

func bootstrapMigrationSQL(t *testing.T) string {
	t.Helper()
	var script strings.Builder
	paths := [...]string{"../migrations/001_initial.sql", "../migrations/002_sandbox.sql"}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("migration source unavailable", err)
		}
		script.Write(content)
		script.WriteString("\n")
	}
	return script.String()
}

func bootstrapRejectGrants(t *testing.T, fixture *bootstrapGrantFixture) {
	t.Helper()
	cases := []struct{ setup, cleanup, failure string }{
		{
			setup:   "CREATE SCHEMA neighbor; GRANT CREATE ON SCHEMA neighbor TO PUBLIC;",
			cleanup: "DROP SCHEMA neighbor;", failure: "cross-schema CREATE",
		},
		{
			setup: "CREATE TABLE public.neighbor(id int); " +
				"GRANT SELECT ON public.neighbor TO PUBLIC;",
			cleanup: "DROP TABLE public.neighbor;", failure: "cross-schema table",
		},
		{
			setup: "CREATE FUNCTION public.neighbor() RETURNS int LANGUAGE sql SECURITY DEFINER " +
				"AS $$ SELECT 1 $$;",
			cleanup: "DROP FUNCTION public.neighbor();", failure: "SECURITY DEFINER",
		},
	}
	for _, entry := range cases {
		bootstrapQuery(t, fixture.container, bootstrapQueryOptions{statement: entry.setup})
		bootstrapQuery(t, fixture.container, bootstrapQueryOptions{
			statement: fixture.script, failure: entry.failure,
		})
		bootstrapAbsent(t, fixture.container)
		bootstrapQuery(t, fixture.container, bootstrapQueryOptions{statement: entry.cleanup})
	}
}

func bootstrapFixture(t *testing.T) string {
	t.Helper()
	name := "p3-bootstrap-test-" + strings.ToLower(rand.Text())
	t.Cleanup(func() {
		podmanCommand(t, "rm", "--force", "--ignore", "--volumes", name)
	})
	// Trust is restricted to this throwaway fixture: no network and no published ports.
	podmanCommand(t, "run", "--detach", "--name", name, "--network=none",
		"--memory=256m", "--cpus=1", "--env", "POSTGRES_HOST_AUTH_METHOD=trust",
		"--env", "POSTGRES_DB=railway", "docker.io/library/postgres:18.3-alpine")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for attempt := uint32(0); attempt < 20; attempt++ {
		err := exec.CommandContext(ctx, "podman", "exec", name,
			"pg_isready", "--username=postgres").Run()
		if err == nil {
			return name
		}
		select {
		case <-ctx.Done():
			t.Fatal("bootstrap fixture readiness timed out")
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Fatal("bootstrap fixture readiness exhausted")
	return ""
}

// Copy options deliberately so default selection cannot mutate caller configuration.
func bootstrapQuery(t *testing.T, name string, options bootstrapQueryOptions) string {
	t.Helper()
	if options.database == "" {
		options.database = "railway"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, "podman", "exec", "--interactive", name, "psql",
		"--no-psqlrc", "--quiet", "--tuples-only", "--no-align", "--set=ON_ERROR_STOP=1",
		"--username=postgres", "--dbname="+options.database)
	command.Stdin = strings.NewReader(options.statement)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("test deadline masked bootstrap disposition")
	}
	if options.failure != "" {
		if err == nil || !strings.Contains(string(output), options.failure) {
			t.Fatal("expected bootstrap rejection missing")
		}
	} else if err != nil {
		t.Fatal("bootstrap SQL failed", err)
	}
	return strings.TrimSpace(string(output))
}

func bootstrapAbsent(t *testing.T, name string) {
	t.Helper()
	output := bootstrapQuery(t, name, bootstrapQueryOptions{
		statement: "SELECT (SELECT count(*) FROM pg_roles WHERE rolname LIKE 'p3_relay_%')," +
			"(SELECT count(*) FROM pg_namespace WHERE nspname='p3_relay');",
	})
	if output != "0|0" {
		t.Fatal("rejected bootstrap left persistent roles or schema")
	}
}
