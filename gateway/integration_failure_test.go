//go:build gatewayintegration

package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// A trusted backend CA does not excuse a hostname mismatch on that separate TLS connection.
func TestBackendHostname(t *testing.T) {
	fixture := startFixture(t, "wrong-backend.test")
	value := fixture.clientConfiguration(t)
	value.ConnectTimeout = 8 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connection, err := pgx.ConnectConfig(ctx, value)
	if connection != nil {
		defer closeConnection(t, connection)
	}
	if err == nil {
		err = connection.Ping(ctx)
	}
	if err == nil {
		t.Fatal("incorrect backend hostname accepted")
	}
	if ctx.Err() != nil {
		t.Fatal("test deadline masked backend hostname failure")
	}
	logs := podmanCommand(t, "logs", fixture.gateway)
	if !strings.Contains(logs, "not present in server certificate") {
		t.Fatal("hostname rejection not observed")
	}
}

// Readiness only checks the disposable local server's listener, not certificate trust.
func (fixture *gatewayFixture) waitDatabase(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for attempt := uint32(0); attempt < 30; attempt++ {
		err := exec.CommandContext(ctx, "podman", "exec", fixture.database, "pg_isready",
			"--host=127.0.0.1", "--username=postgres", "--dbname=railway", "--timeout=1").Run()
		if err == nil {
			return
		}
		var failure *exec.ExitError
		if !errors.As(err, &failure) || failure.ExitCode() > 2 {
			t.Fatal("database readiness operation failed", err)
		}
		select {
		case <-ctx.Done():
			t.Fatal("database readiness deadline exceeded")
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatal("database readiness attempts exhausted")
}

// The frontend can validate successfully while backend trust is wrong. No SQL may pass that hop.
func (fixture *gatewayFixture) rejectBackendCA(t *testing.T) {
	t.Helper()
	// Copy test descriptors, not ownership: only the additional gateway is acquired here.
	failed := *fixture
	failed.gateway += "-wrong-ca"
	configuration := *fixture.configuration
	configuration.backendCA, _ = testIdentity(t, "unrelated-ca.test")
	failed.configuration = &configuration
	t.Cleanup(func() { podmanCommand(t, "rm", "--force", "--ignore", failed.gateway) })
	failed.startGateway(t)
	value := failed.clientConfiguration(t)
	value.ConnectTimeout = 8 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connection, err := pgx.ConnectConfig(ctx, value)
	if connection != nil {
		defer closeConnection(t, connection)
	}
	if err == nil {
		err = connection.Ping(ctx)
	}
	if err == nil {
		t.Fatal("untrusted backend accepted")
	}
	if ctx.Err() != nil {
		t.Fatal("test deadline masked backend verification failure")
	}
	logs := podmanCommand(t, "logs", failed.gateway)
	if !strings.Contains(logs, "certificate verify failed") {
		t.Fatal("backend certificate rejection was not observed")
	}
}

// Force ENOSPC after partial startup writes in an isolated 8 KiB tmpfs. Check cleanup before
// container teardown so an empty unmounted filesystem cannot masquerade as successful cleanup.
func (fixture *gatewayFixture) checkStartupFailure(t *testing.T) {
	t.Helper()
	failed := *fixture
	failed.gateway += "-full"
	t.Cleanup(func() { podmanCommand(t, "rm", "--force", "--ignore", failed.gateway) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	command := failed.gatewayCommand(ctx, 8192)
	command.Args = append(command.Args, "sh", "-ec",
		"set +e; gateway-launch; status=$?; set -e; test \"$status\" -eq 1; "+
			"test -z \"$(find /dev/shm -mindepth 1 -maxdepth 1)\"")
	if err := command.Run(); err != nil {
		t.Fatal("failure fixture launch failed", err)
	}
	if strings.TrimSpace(podmanCommand(t, "wait", failed.gateway)) != "0" {
		t.Fatal("partial startup cleanup failed")
	}
	logs := podmanCommand(t, "logs", failed.gateway)
	if !strings.Contains(logs, "private runtime file creation failed") {
		t.Fatal("expected startup operating failure not observed")
	}
	assertCredentialsAbsent(t, failed.configuration, logs)
}

func assertCredentialsAbsent(t *testing.T, value *configuration, logs string) {
	t.Helper()
	for _, secret := range []string{
		value.privateKey, value.runtimePassword, value.migrationPassword,
	} {
		if strings.Contains(logs, secret) {
			t.Fatal("logs exposed credential material")
		}
	}
}
