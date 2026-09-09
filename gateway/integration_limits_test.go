//go:build gatewayintegration

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Occupy the exact runtime server limit, check explicit overload, then release and recover.
func (fixture *gatewayFixture) checkExhaustion(t *testing.T) {
	t.Helper()
	connections := make([]*pgx.Conn, 0, 24)
	defer func() {
		for _, connection := range connections {
			closeConnection(t, connection)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for index := uint32(0); index < 24; index++ {
		connection := fixture.connect(t)
		connections = append(connections, connection)
		if err := connection.Ping(ctx); err != nil {
			t.Fatal("in-budget connection failed", err)
		}
	}
	t.Log("Gateway memory with 24 held runtime sessions:",
		strings.TrimSpace(podmanCommand(t, "stats", "--no-stream", "--format",
			"{{.MemUsage}}", fixture.gateway)))
	configuration := fixture.clientConfiguration(t)
	configuration.ConnectTimeout = 8 * time.Second

	extra, err := pgx.ConnectConfig(ctx, configuration)
	if extra != nil {
		defer closeConnection(t, extra)
	}

	if err == nil {
		err = extra.Ping(ctx)
	}
	if err == nil {
		t.Fatal("runtime server connection limit exceeded")
	}
	var failure *pgconn.PgError
	if !errors.As(err, &failure) || failure.Message != "query_wait_timeout" {
		t.Fatal("overload did not produce the pooler's bounded failure", err)
	}
	if ctx.Err() != nil {
		t.Fatal("test deadline masked pooler overload")
	}
	closeConnection(t, connections[0])
	recovered := fixture.connect(t)
	defer closeConnection(t, recovered)

	if err := recovered.Ping(ctx); err != nil {
		t.Fatal("released capacity did not recover", err)
	}
}

// Interrupt backend and gateway independently. A fresh wait budget must distinguish real failure
// from an expired test context; restarting the container must not retain old credential files.
func (fixture *gatewayFixture) checkRecovery(t *testing.T) {
	t.Helper()
	connection := fixture.connect(t)
	defer closeConnection(t, connection)

	podmanCommand(t, "stop", "--time=3", fixture.database)
	assertConnectionFailed(t, connection)
	podmanCommand(t, "start", fixture.database)
	recovered := fixture.connect(t)
	defer closeConnection(t, recovered)

	podmanCommand(t, "kill", "--signal=KILL", fixture.gateway)
	assertConnectionFailed(t, recovered)
	podmanCommand(t, "start", fixture.gateway)
	restarted := fixture.connect(t)
	defer closeConnection(t, restarted)

	directories := podmanCommand(t, "exec", fixture.gateway, "find", "/dev/shm",
		"-mindepth", "1", "-maxdepth", "1", "-type", "d")
	if len(strings.Fields(directories)) != 1 {
		t.Fatal("stale startup directories retained")
	}
	podmanCommand(t, "exec", fixture.gateway, "sh", "-ec",
		"if tr '\\000' '\\n' </proc/1/environ | grep -q '^GATEWAY_'; then exit 1; fi")
	logs := podmanCommand(t, "logs", fixture.gateway)
	if !strings.Contains(logs, "login attempt") {
		t.Fatal("connection audit record missing")
	}
	assertCredentialsAbsent(t, fixture.configuration, logs)
}

func assertConnectionFailed(t *testing.T, connection *pgx.Conn) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := connection.Ping(ctx); err == nil {
		t.Fatal("interrupted connection reported success")
	}
	if ctx.Err() != nil {
		t.Fatal("test deadline masked the interruption")
	}
}
