//go:build integration

package sandbox

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sillypoise/p3-relay/internal/delivery"
	"github.com/sillypoise/p3-relay/internal/event"
	"github.com/sillypoise/p3-relay/internal/identifier"
	"github.com/sillypoise/p3-relay/internal/notification"
	"github.com/sillypoise/p3-relay/internal/postgres"
)

type failedNotifier struct {
	pool  *pgxpool.Pool
	calls atomic.Uint32
	t     *testing.T
}

func (notifier *failedNotifier) Publish(ctx context.Context, id string) error {
	notifier.calls.Add(1)
	var exists bool
	if err := notifier.pool.QueryRow(ctx, `SELECT EXISTS(SELECT FROM p3_relay.events WHERE id=$1)`, id).Scan(&exists); err != nil || !exists {
		notifier.t.Error("notification preceded database commit")
	}
	return notification.ErrQueue
}

type failedConsumer struct{}

func (failedConsumer) Receive(context.Context) (notification.Batch, error) {
	return notification.Batch{}, notification.ErrQueue
}
func (failedConsumer) Acknowledge(context.Context, *notification.Batch) error {
	return errors.New("unexpected acknowledgement")
}

// Called with the disposable database prepared by TestSandboxIntegration.
func verifyNotificationFailures(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *Store,
	reads *postgres.Store, worker *delivery.Worker, session string) {
	t.Helper()
	probe := &failedNotifier{pool: pool, t: t}
	store.Notifications = probe
	reads.Notifications = probe
	id, err := store.Submit(ctx, session, "success")
	if err != nil {
		t.Fatal("notification failure changed durable receipt", err)
	}
	// No notification reaches the worker. Reconciliation still completes the real event.
	if err = notification.Cycle(ctx, worker, failedConsumer{}); err != notification.ErrQueue {
		t.Fatal(err)
	}
	detail, err := reads.Detail(ctx, id, "sandbox:"+session)
	if err != nil || detail.State != "delivered" || len(detail.Attempts) != 1 {
		t.Fatal("lost hint stranded event")
	}
	for i := 0; i < 2; i++ {
		if err = notification.Cycle(ctx, worker, nil); err != nil {
			t.Fatal(err)
		}
	}
	detail, err = reads.Detail(ctx, id, "sandbox:"+session)
	if err != nil || len(detail.Attempts) != 1 {
		t.Fatal("repeated reconciliation duplicated terminal event")
	}
	dead, err := store.Submit(ctx, session, "permanent_failure")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = worker.RunOnce(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = store.Replay(ctx, session, dead); err != nil {
		t.Fatal("notification failure changed replay", err)
	}
	if _, err = worker.RunOnce(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if probe.calls.Load() != 3 {
		t.Fatalf("sandbox post-commit notifications: %d", probe.calls.Load())
	}
	operatorID, err := identifier.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"test":true}`)
	digest := sha256.Sum256(body)
	receipt := &event.Receipt{ID: operatorID, SourceKey: "integration-operator", ExternalEventID: "notify-1",
		Body: body, BodySHA256: digest[:], DestinationURL: "https://example.com"}
	if _, err = reads.Accept(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	duplicate, err := reads.Accept(ctx, receipt)
	if err != nil || !duplicate.Duplicate || probe.calls.Load() != 4 {
		t.Fatal("duplicate receipt re-published")
	}
	if _, err = pool.Exec(ctx, `UPDATE p3_relay.events SET state='dead_lettered' WHERE id=$1`, operatorID); err != nil {
		t.Fatal(err)
	}
	if err = reads.Replay(ctx, operatorID, "integration-operator"); err != nil {
		t.Fatal(err)
	}
	if probe.calls.Load() != 5 {
		t.Fatal("operator replay did not notify after commit")
	}
	// A late or lost notification must not start an automatic attempt at the age boundary.
	clock := time.Now()
	if _, err = pool.Exec(ctx, `UPDATE p3_relay.events SET created_at=$2::timestamptz-interval '24 hours' WHERE id=$1`, operatorID, clock); err != nil {
		t.Fatal(err)
	}
	if claimed, err := reads.Claim(ctx, clock, 30*time.Second); err != nil || claimed != nil {
		t.Fatal("expired event was claimed")
	}
	detail, err = reads.Detail(ctx, operatorID, "integration-operator")
	if err != nil || detail.State != "dead_lettered" || len(detail.Attempts) != 0 {
		t.Fatal("expiry did not retire without a network attempt")
	}

	store.Notifications = nil
	reads.Notifications = nil
}
