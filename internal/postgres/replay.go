package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/sillypoise/p3-relay/internal/event"
)

func (store *Store) Replay(context_value context.Context, event_id string, source_key string) error {
	const query = `
		UPDATE p3_relay.events
		SET state = 'pending', attempt_count = 0, replay_count = replay_count + 1,
			next_attempt_at = clock_timestamp(), lease_expires_at = NULL,
			claim_id = NULL, delivered_at = NULL
		WHERE id = $1 AND source_key = $2 AND state = 'dead_lettered'
			AND replay_count < 16`
	command, error_value := store.pool.Exec(context_value, query, event_id, source_key)
	if error_value != nil {
		return fmt.Errorf("replay dead-lettered event: %w", error_value)
	}
	if command.RowsAffected() == 1 {
		return nil
	}

	var state string
	error_value = store.pool.QueryRow(context_value, `
		SELECT state FROM p3_relay.events WHERE id = $1 AND source_key = $2`,
		event_id, source_key).Scan(&state)
	if errors.Is(error_value, pgx.ErrNoRows) {
		return event.NotFoundError{}
	}
	if error_value != nil {
		return fmt.Errorf("read event replay state: %w", error_value)
	}
	return event.InvalidStateError{}
}
