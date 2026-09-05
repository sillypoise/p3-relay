package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sillypoise/p3-relay/internal/delivery"
	"github.com/sillypoise/p3-relay/internal/identifier"
)

func (store *Store) Claim(context_value context.Context, now time.Time, lease time.Duration) (*delivery.ClaimedEvent, error) {
	claim_id, error_value := identifier.NewUUID()
	if error_value != nil {
		return nil, fmt.Errorf("create claim identifier: %w", error_value)
	}
	const query = `
		WITH candidate AS (
			SELECT id, state FROM p3_relay.events
			WHERE ((
				state IN ('pending', 'retry_scheduled') AND next_attempt_at <= $1
			) OR (state = 'delivering' AND lease_expires_at <= $1))
            AND (sandbox_id IS NULL OR EXISTS (
                SELECT FROM p3_relay.sandbox_sessions s
                WHERE s.id=sandbox_id AND s.expires_at>$1))
			ORDER BY next_attempt_at, created_at
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE p3_relay.events AS event
		SET state = 'delivering',
			attempt_count = CASE WHEN candidate.state = 'delivering'
				THEN event.attempt_count ELSE event.attempt_count + 1 END,
			lease_expires_at = $2, claim_id = $3
		FROM candidate WHERE event.id = candidate.id
		RETURNING event.id, event.body, event.destination_url,
			event.replay_count, event.attempt_count, event.created_at, event.sandbox_id IS NOT NULL`
	claimed := &delivery.ClaimedEvent{ClaimID: claim_id}
	error_value = store.pool.QueryRow(context_value, query, now, now.Add(lease), claim_id).Scan(
		&claimed.ID,
		&claimed.Body,
		&claimed.DestinationURL,
		&claimed.ReplayNumber,
		&claimed.AttemptNumber,
		&claimed.CreatedAt,
		&claimed.Sandbox,
	)
	if errors.Is(error_value, pgx.ErrNoRows) {
		return nil, nil
	}
	if error_value != nil {
		return nil, fmt.Errorf("claim delivery: %w", error_value)
	}
	return claimed, nil
}

func (store *Store) Record(context_value context.Context, claimed *delivery.ClaimedEvent, attempt *delivery.Attempt) error {
	if claimed == nil || attempt == nil {
		panic("claimed event and attempt are required")
	}
	transaction, error_value := store.pool.Begin(context_value)
	if error_value != nil {
		return fmt.Errorf("begin attempt transaction: %w", error_value)
	}
	defer func() { _ = transaction.Rollback(context_value) }()

	state := record_state(claimed, attempt)
	command, error_value := transaction.Exec(context_value, `
		UPDATE p3_relay.events SET state = $1, next_attempt_at = $2,
			lease_expires_at = NULL, claim_id = NULL,
			delivered_at = CASE WHEN $1 = 'delivered' THEN $3 ELSE delivered_at END
		WHERE id = $4 AND state = 'delivering' AND claim_id = $5`,
		state, attempt.NextAttemptAt, attempt.FinishedAt, claimed.ID, claimed.ClaimID)
	if error_value != nil {
		return fmt.Errorf("complete claimed event: %w", error_value)
	}
	if command.RowsAffected() != 1 {
		return errors.New("delivery claim expired or was replaced")
	}
	_, error_value = transaction.Exec(context_value, `
		INSERT INTO p3_relay.delivery_attempts (
			id, event_id, replay_number, attempt_number, started_at, finished_at, outcome,
			status_code, response_excerpt, error_code
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		attempt.ID, claimed.ID, claimed.ReplayNumber, claimed.AttemptNumber,
		attempt.StartedAt, attempt.FinishedAt, attempt.Outcome,
		attempt.StatusCode, attempt.ResponseExcerpt, attempt.ErrorCode)
	if error_value != nil {
		return fmt.Errorf("insert delivery attempt: %w", error_value)
	}
	if error_value = transaction.Commit(context_value); error_value != nil {
		return fmt.Errorf("commit delivery attempt: %w", error_value)
	}
	return nil
}

func record_state(claimed *delivery.ClaimedEvent, attempt *delivery.Attempt) string {
	if attempt.Outcome == "delivered" {
		return "delivered"
	}
	if attempt.Outcome == "terminal_http" {
		return "dead_lettered"
	}
	if !CanRetryAt(claimed.AttemptNumber, claimed.CreatedAt, attempt.FinishedAt) {
		return "dead_lettered"
	}
	return "retry_scheduled"
}

func CanRetryAt(attempt uint8, created_at time.Time, now time.Time) bool {
	return delivery.CanRetry(attempt) && now.Sub(created_at) < 24*time.Hour
}
