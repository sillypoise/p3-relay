//go:build gatewayintegration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sillypoise/p3-relay/internal/postgres"
)

// Two local endpoints model old/new identities during deployment, not a Railway rollout. The
// actual application trust loader must accept both during overlap and reject retired old trust.
func (fixture *gatewayFixture) checkTrustRotation(t *testing.T) {
	t.Helper()
	rotated := *fixture
	rotated.gateway += "-rotated"
	identity := *fixture.configuration
	identity.certificate, identity.privateKey = testIdentity(t, "gateway.test")
	rotated.configuration = &identity
	t.Cleanup(func() { podmanCommand(t, "rm", "--force", "--ignore", rotated.gateway) })
	rotated.startGateway(t)
	ready := rotated.connect(t)
	closeConnection(t, ready)

	overlap := fixture.configuration.certificate + identity.certificate
	checkTrustedQuery(t, fixture.clientConfiguration(t), overlap)
	checkTrustedQuery(t, rotated.clientConfiguration(t), overlap)
	checkTrustedQuery(t, rotated.clientConfiguration(t), identity.certificate)

	retired := fixture.clientConfiguration(t)
	if err := postgres.ConfigureTLS(retired, &postgres.TLSOptions{
		CAPEM: identity.certificate, Required: "true",
	}); err != nil {
		t.Fatal("retired trust configuration failed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	connection, err := pgx.ConnectConfig(ctx, retired)
	if err == nil {
		closeConnection(t, connection)
		t.Fatal("retired identity remained trusted")
	}
	if ctx.Err() != nil {
		t.Fatal("deadline masked retired certificate rejection")
	}
}

func checkTrustedQuery(t *testing.T, configuration *pgx.ConnConfig, anchors string) {
	t.Helper()
	if err := postgres.ConfigureTLS(configuration, &postgres.TLSOptions{
		CAPEM: anchors, Required: "true",
	}); err != nil {
		t.Fatal("overlap trust configuration failed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connection, err := pgx.ConnectConfig(ctx, configuration)
	if err != nil {
		t.Fatal("trusted identity connection failed")
	}
	defer closeConnection(t, connection)

	if err := connection.Ping(ctx); err != nil {
		t.Fatal("trusted identity query failed")
	}
}
