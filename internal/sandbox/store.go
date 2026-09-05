package sandbox

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sillypoise/p3-relay/internal/identifier"
)

var ErrQuota = errors.New("sandbox quota exceeded")
var ErrMissing = errors.New("sandbox event missing")
var ErrState = errors.New("sandbox event cannot be replayed")

type Store struct{ Pool *pgxpool.Pool }

func rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
		slog.Error("sandbox rollback failed")
	}
}

// One row serializes admission across replicas; the transaction also owns session counters.
func (store *Store) admission(ctx context.Context) (pgx.Tx, error) {
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	// Read the wall clock only after acquiring the lock: transaction start times can arrive
	// out of order across an hourly boundary. A missing budget row must fail closed.
	var singleton bool
	err = tx.QueryRow(ctx, `SELECT singleton FROM p3_relay.sandbox_budget WHERE singleton FOR UPDATE`).Scan(&singleton)
	if err != nil {
		rollback(ctx, tx)
		return nil, err
	}
	_, err = tx.Exec(ctx, `WITH clock AS MATERIALIZED (
 SELECT date_trunc('hour',clock_timestamp() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS hour
 ) UPDATE p3_relay.sandbox_budget SET
 sessions = CASE WHEN window_start < clock.hour THEN 0 ELSE sessions END,
 admissions = CASE WHEN window_start < clock.hour THEN 0 ELSE admissions END,
 window_start = GREATEST(window_start,clock.hour) FROM clock WHERE singleton`)
	if err != nil {
		rollback(ctx, tx)
		return nil, err
	}
	return tx, nil
}

func (store *Store) Create(ctx context.Context, id string, expiry time.Time) error {
	tx, err := store.admission(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	command, err := tx.Exec(ctx, `UPDATE p3_relay.sandbox_budget SET sessions=sessions+1
 WHERE singleton AND sessions<100 AND (SELECT count(*) FROM p3_relay.sandbox_sessions)<1000`)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrQuota
	}
	if _, err = tx.Exec(ctx, `INSERT INTO p3_relay.sandbox_sessions(id,expires_at) VALUES($1,$2)`, id, expiry); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (store *Store) Active(ctx context.Context, id string) error {
	var active bool
	err := store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT FROM p3_relay.sandbox_sessions
 WHERE id=$1 AND expires_at>clock_timestamp())`, id).Scan(&active)
	if err != nil {
		return err
	}
	if active == false {
		return ErrSession
	}
	return nil
}

func (store *Store) Submit(ctx context.Context, session string, scenario string) (string, error) {
	if ValidScenario(scenario) == false {
		return "", ErrState
	}
	id, err := identifier.NewUUID()
	if err != nil {
		return "", err
	}
	tx, err := store.admission(ctx)
	if err != nil {
		return "", err
	}
	defer rollback(ctx, tx)
	command, err := tx.Exec(ctx, `UPDATE p3_relay.sandbox_sessions SET event_count=event_count+1
 WHERE id=$1 AND expires_at>clock_timestamp() AND event_count<20`, session)
	if err != nil {
		return "", err
	}
	if command.RowsAffected() != 1 {
		var active bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM p3_relay.sandbox_sessions
  WHERE id=$1 AND expires_at>clock_timestamp())`, session).Scan(&active)
		if err != nil {
			return "", err
		}
		if active == false {
			return "", ErrSession
		}
		return "", ErrQuota
	}
	if err = charge(ctx, tx); err != nil {
		return "", err
	}
	body := []byte(fmt.Sprintf(`{"simulated":true,"scenario":%q}`, scenario))
	digest := sha256.Sum256(body)
	_, err = tx.Exec(ctx, `INSERT INTO p3_relay.events
 (id,source_key,external_event_id,body,body_sha256,destination_url,state,sandbox_id)
 VALUES($1,$2,$3,$4,$5,$6,'pending',$7)`, id, "sandbox:"+session,
		"simulated-"+scenario+"-"+id, body, digest[:], "relay-simulator://"+scenario, session)
	if err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func charge(ctx context.Context, tx pgx.Tx) error {
	command, err := tx.Exec(ctx, `UPDATE p3_relay.sandbox_budget SET admissions=admissions+1
 WHERE singleton AND admissions<1000`)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrQuota
	}
	return nil
}

func (store *Store) Replay(ctx context.Context, session string, id string) error {
	tx, err := store.admission(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	var state string
	var replays int16
	err = tx.QueryRow(ctx, `SELECT e.state,e.replay_count FROM p3_relay.events e
 JOIN p3_relay.sandbox_sessions s ON e.sandbox_id=s.id
 WHERE e.id=$1 AND s.id=$2 AND s.expires_at>clock_timestamp() FOR UPDATE OF e,s`, id, session).Scan(&state, &replays)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMissing
	}
	if err != nil {
		return err
	}
	if state != "dead_lettered" {
		return ErrState
	}
	if replays >= 2 {
		return ErrQuota
	}
	if err = charge(ctx, tx); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE p3_relay.events SET state='pending',attempt_count=0,
 replay_count=replay_count+1,next_attempt_at=clock_timestamp(),lease_expires_at=NULL,claim_id=NULL
 WHERE id=$1 AND sandbox_id=$2`, id, session)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ValidScenario(value string) bool {
	switch value {
	case "success", "temporary_failure", "permanent_failure":
		return true
	}
	return false
}
