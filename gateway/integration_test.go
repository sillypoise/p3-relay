//go:build gatewayintegration

package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sillypoise/p3-relay/internal/postgres"
)

type gatewayFixture struct {
	network, database, gateway string
	port                       uint16
	configuration              *configuration
}

// Real local containers prove TLS/auth/SQL boundaries and backend recovery, not Railway behavior.
// All names are unique and all persistence is disposable; no environment database URL is used.
func TestGatewayContainer(t *testing.T) {
	fixture := startFixture(t, "postgres.railway.internal")
	connection := fixture.connect(t)
	defer closeConnection(t, connection)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var encrypted bool
	if err := connection.QueryRow(ctx,
		"SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()").Scan(&encrypted); err != nil {
		t.Fatal("backend TLS query failed", err)
	}
	if !encrypted {
		t.Fatal("backend connection is not encrypted")
	}
	for _, query := range []string{
		"CREATE TABLE p3_relay.forbidden (id integer)", "SELECT * FROM p1_private.sentinel",
	} {
		_, err := connection.Exec(ctx, query)
		var failure *pgconn.PgError
		if !errors.As(err, &failure) || failure.Code != "42501" {
			t.Fatal("forbidden SQL did not return insufficient_privilege")
		}
	}
	var value int32
	if err := connection.QueryRow(ctx, "SELECT id FROM p3_relay.allowed").Scan(&value); err != nil {
		t.Fatal("permitted read failed", err)
	}
	if value != 1 {
		t.Fatal("unexpected fixture row")
	}
	fixture.rejectClients(t)
	fixture.checkMigration(t)
	closeConnection(t, connection)
	fixture.checkExhaustion(t)
	fixture.checkRecovery(t)
	fixture.rejectBackendCA(t)
	fixture.checkStartupFailure(t)
	fixture.checkTrustRotation(t)
}

func startFixture(t *testing.T, backendHostname string) *gatewayFixture {
	t.Helper()
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal("fixture ID generation failed")
	}
	prefix := "p3-gateway-test-" + hex.EncodeToString(suffix[:])
	fixture := &gatewayFixture{
		network: prefix, database: prefix + "-database", gateway: prefix + "-gateway",
		configuration: testConfiguration(t),
	}
	// Cleanup is registered before create: a timed-out command can have created its resource.
	t.Cleanup(func() {
		podmanCommand(t, "rm", "--force", "--ignore", "--volumes",
			fixture.gateway, fixture.database)
		podmanCommand(t, "network", "rm", "--force", fixture.network)
	})
	podmanCommand(t, "network", "create", fixture.network)
	fixture.startDatabase(t, backendHostname)
	fixture.waitDatabase(t)
	fixture.startGateway(t)
	return fixture
}

func (fixture *gatewayFixture) startDatabase(t *testing.T, backendHostname string) {
	t.Helper()
	directory := t.TempDir()
	certificate, key := testIdentity(t, backendHostname)
	fixture.configuration.backendCA = certificate
	files := map[string]string{
		"backend.crt": certificate, "backend.key": key,
		"database.env": "POSTGRES_DB=railway\nPOSTGRES_PASSWORD=local-integration-only\n",
		"init.sql": `CREATE ROLE p3_relay_runtime LOGIN PASSWORD '` +
			fixture.configuration.runtimePassword + `';
CREATE ROLE p3_relay_migrator LOGIN PASSWORD '` + fixture.configuration.migrationPassword + `';
CREATE SCHEMA p3_relay AUTHORIZATION p3_relay_migrator;
CREATE TABLE p3_relay.allowed (id integer);
INSERT INTO p3_relay.allowed VALUES (1);
GRANT USAGE ON SCHEMA p3_relay TO p3_relay_runtime;
GRANT SELECT ON p3_relay.allowed TO p3_relay_runtime;
CREATE SCHEMA p1_private;
CREATE TABLE p1_private.sentinel (id integer);`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatal("fixture file creation failed")
		}
	}
	podmanCommand(t, "create", "--name", fixture.database, "--network", fixture.network,
		"--network-alias", "postgres.railway.internal", "--memory=256m", "--cpus=0.5",
		"--env-file", filepath.Join(directory, "database.env"),
		"docker.io/library/postgres:18.3-alpine", "sh", "-ec",
		"chown postgres:postgres /tmp/backend.* /docker-entrypoint-initdb.d/init.sql; "+
			"exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/tmp/backend.crt "+
			"-c ssl_key_file=/tmp/backend.key -c shared_buffers=32MB -c max_connections=40")
	for _, name := range []string{"backend.crt", "backend.key", "init.sql"} {
		target := "/tmp/" + name
		if name == "init.sql" {
			target = "/docker-entrypoint-initdb.d/init.sql"
		}
		podmanCommand(t, "cp", filepath.Join(directory, name), fixture.database+":"+target)
	}
	podmanCommand(t, "start", fixture.database)
}

func (fixture *gatewayFixture) startGateway(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := fixture.gatewayCommand(ctx, 1024*1024)
	if err := command.Run(); err != nil {
		t.Fatal("gateway launch failed", err)
	}
	address := strings.TrimSpace(podmanCommand(t, "port", fixture.gateway, "6432/tcp"))
	port, err := strconv.ParseUint(strings.TrimPrefix(address, "127.0.0.1:"), 10, 16)
	if err != nil || port == 0 {
		t.Fatal("invalid mapped port")
	}
	fixture.port = uint16(port)
}

func (fixture *gatewayFixture) gatewayCommand(
	ctx context.Context, sharedMemoryBytes uint32,
) *exec.Cmd {
	command := exec.CommandContext(ctx, "podman", "run", "--detach", "--name", fixture.gateway,
		"--network", fixture.network, "--publish", "127.0.0.1::6432", "--read-only",
		"--cap-drop=ALL", "--memory=128m", "--cpus=0.25",
		fmt.Sprintf("--shm-size=%db", sharedMemoryBytes),
		"--env", "GATEWAY_CERTIFICATE", "--env", "GATEWAY_PRIVATE_KEY",
		"--env", "GATEWAY_BACKEND_CA", "--env", "GATEWAY_HOSTNAME",
		"--env", "GATEWAY_RUNTIME_PASSWORD", "--env", "GATEWAY_MIGRATION_PASSWORD",
		"localhost/p3-relay-gateway:development")
	value := fixture.configuration
	command.Env = append(os.Environ(), "GATEWAY_CERTIFICATE="+value.certificate,
		"GATEWAY_PRIVATE_KEY="+value.privateKey, "GATEWAY_BACKEND_CA="+value.backendCA,
		"GATEWAY_HOSTNAME="+value.hostname, "GATEWAY_RUNTIME_PASSWORD="+value.runtimePassword,
		"GATEWAY_MIGRATION_PASSWORD="+value.migrationPassword)
	return command
}

func (fixture *gatewayFixture) clientConfiguration(t *testing.T) *pgx.ConnConfig {
	t.Helper()
	value, err := pgx.ParseConfig(
		"host=gateway.test user=p3_relay_runtime dbname=p3_relay sslmode=verify-full")
	if err != nil {
		t.Fatal("fixture connection parsing failed")
	}
	// Dial loopback without weakening verification of the fixture's synthetic DNS identity.
	value.Host = "127.0.0.1"
	if err := postgres.ConfigureTLS(value, &postgres.TLSOptions{
		CAPEM: fixture.configuration.certificate, Required: "true",
	}); err != nil {
		t.Fatal("application trust configuration failed", err)
	}
	value.Port = fixture.port
	value.Password = fixture.configuration.runtimePassword
	value.ConnectTimeout = time.Second
	return value
}

func (fixture *gatewayFixture) connect(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for attempt := uint32(0); attempt < 40; attempt++ {
		connection, err := pgx.ConnectConfig(ctx, fixture.clientConfiguration(t))
		if err == nil {
			if err := connection.Ping(ctx); err == nil {
				return connection
			}
			closeConnection(t, connection)
		}
		select {
		case <-ctx.Done():
			t.Fatal("gateway readiness timed out")
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatal("gateway readiness exhausted")
	return nil
}

func (fixture *gatewayFixture) rejectClients(t *testing.T) {
	t.Helper()
	cases := []struct {
		name   string
		change func(*pgx.ConnConfig)
	}{
		{"plaintext", func(c *pgx.ConnConfig) { c.TLSConfig = nil }},
		{"obsolete TLS", func(c *pgx.ConnConfig) {
			c.TLSConfig.MinVersion = tls.VersionTLS10
			c.TLSConfig.MaxVersion = tls.VersionTLS11
		}},
		{"wrong CA", func(c *pgx.ConnConfig) { c.TLSConfig.RootCAs = x509.NewCertPool() }},
		{"wrong name", func(c *pgx.ConnConfig) { c.TLSConfig.ServerName = "wrong.test" }},
		{"wrong password", func(c *pgx.ConnConfig) { c.Password = "invalid" }},
		{"administrator", func(c *pgx.ConnConfig) {
			c.User = "postgres"
			c.Password = "local-integration-only"
		}},
		{"wrong database", func(c *pgx.ConnConfig) { c.Database = "railway" }},
		{"pooler admin", func(c *pgx.ConnConfig) { c.Database = "pgbouncer" }},
	}
	for _, entry := range cases {
		t.Run(entry.name, func(t *testing.T) {
			value := fixture.clientConfiguration(t)
			entry.change(value)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			connection, err := pgx.ConnectConfig(ctx, value)
			if err == nil {
				closeConnection(t, connection)
				t.Fatal("invalid client accepted")
			}
			if ctx.Err() != nil {
				t.Fatal("rejection timed out rather than failing closed")
			}
		})
	}
}

// Migration login also exercises the minimum accepted client TLS version.
func (fixture *gatewayFixture) checkMigration(t *testing.T) {
	t.Helper()
	value := fixture.clientConfiguration(t)
	value.User = "p3_relay_migrator"
	value.Password = fixture.configuration.migrationPassword
	value.TLSConfig.MaxVersion = tls.VersionTLS12
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := pgx.ConnectConfig(ctx, value)
	if err != nil {
		t.Fatal("migration login failed", err)
	}
	defer closeConnection(t, connection)

	var absent bool
	err = connection.QueryRow(ctx,
		"SELECT to_regclass('p3_relay.forbidden') IS NULL").Scan(&absent)
	if err != nil || !absent {
		t.Fatal("denied DDL left unexpected state")
	}
	_, err = connection.Exec(ctx, "CREATE TABLE p3_relay.migration_allowed (id integer)")
	if err != nil {
		t.Fatal("migration DDL failed", err)
	}
}

func closeConnection(t *testing.T, connection *pgx.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := connection.Close(ctx); err != nil {
		t.Error("test connection close failed")
	}
}

func podmanCommand(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "podman", args...).CombinedOutput()
	if err != nil {
		t.Fatal(fmt.Sprintf("Podman %s failed: %v", args[0], err))
	}
	return string(output)
}
