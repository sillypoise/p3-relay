package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/sillypoise/p3-relay/internal/event"
)

func (store *Store) Overview(context_value context.Context, source_key string) (event.Overview, error) {
	const query = `
		SELECT
			count(*) FILTER (WHERE state = 'pending'),
			count(*) FILTER (WHERE state = 'delivering'),
			count(*) FILTER (WHERE state = 'delivered'),
			count(*) FILTER (WHERE state = 'retry_scheduled'),
			count(*) FILTER (WHERE state = 'dead_lettered')
		FROM p3_relay.events WHERE source_key = $1`
	var result event.Overview
	error_value := store.pool.QueryRow(context_value, query, source_key).Scan(
		&result.Pending, &result.Delivering, &result.Delivered,
		&result.Retrying, &result.DeadLettered,
	)
	if error_value != nil {
		return event.Overview{}, fmt.Errorf("read event overview: %w", error_value)
	}
	return result, nil
}

func (store *Store) List(context_value context.Context, source_key string, state string, limit uint16) ([]event.Summary, error) {
	const query = `
		SELECT id, external_event_id, state, attempt_count, created_at
		FROM p3_relay.events
		WHERE source_key = $1 AND ($2 = '' OR state = $2)
		ORDER BY created_at DESC, id DESC LIMIT $3`
	rows, error_value := store.pool.Query(context_value, query, source_key, state, limit)
	if error_value != nil {
		return nil, fmt.Errorf("list events: %w", error_value)
	}
	defer rows.Close()
	results := make([]event.Summary, 0, limit)
	for rows.Next() {
		var summary event.Summary
		if error_value = rows.Scan(
			&summary.ID, &summary.ExternalEventID, &summary.State,
			&summary.AttemptCount, &summary.CreatedAt,
		); error_value != nil {
			return nil, fmt.Errorf("scan event summary: %w", error_value)
		}
		results = append(results, summary)
	}
	if error_value = rows.Err(); error_value != nil {
		return nil, fmt.Errorf("iterate event summaries: %w", error_value)
	}
	return results, nil
}

func (store *Store) Detail(context_value context.Context, event_id string, source_key string) (event.Detail, error) {
	const query = `
		SELECT id, external_event_id, state, attempt_count, created_at,
			destination_url, replay_count
		FROM p3_relay.events WHERE id = $1 AND source_key = $2`
	var detail event.Detail
	error_value := store.pool.QueryRow(context_value, query, event_id, source_key).Scan(
		&detail.ID, &detail.ExternalEventID, &detail.State, &detail.AttemptCount,
		&detail.CreatedAt, &detail.DestinationURL, &detail.ReplayCount,
	)
	if errors.Is(error_value, pgx.ErrNoRows) {
		return event.Detail{}, event.NotFoundError{}
	}
	if error_value != nil {
		return event.Detail{}, fmt.Errorf("read event detail: %w", error_value)
	}
	attempts, error_value := store.detail_attempts(context_value, event_id)
	if error_value != nil {
		return event.Detail{}, error_value
	}
	detail.Attempts = attempts
	return detail, nil
}

func (store *Store) detail_attempts(context_value context.Context, event_id string) ([]event.AttemptView, error) {
	const query = `
		SELECT replay_number, attempt_number, outcome, status_code, error_code,
			started_at, finished_at, response_excerpt
		FROM p3_relay.delivery_attempts WHERE event_id = $1
		ORDER BY replay_number, attempt_number`
	rows, error_value := store.pool.Query(context_value, query, event_id)
	if error_value != nil {
		return nil, fmt.Errorf("list delivery attempts: %w", error_value)
	}
	defer rows.Close()
	attempts := make([]event.AttemptView, 0, 8)
	for rows.Next() {
		var attempt event.AttemptView
		var response_excerpt []byte
		error_value = rows.Scan(
			&attempt.ReplayNumber, &attempt.AttemptNumber, &attempt.Outcome,
			&attempt.StatusCode, &attempt.ErrorCode, &attempt.StartedAt,
			&attempt.FinishedAt, &response_excerpt,
		)
		if error_value != nil {
			return nil, fmt.Errorf("scan delivery attempt: %w", error_value)
		}
		attempt.ResponseExcerpt = strings.ToValidUTF8(string(response_excerpt), "�")
		attempts = append(attempts, attempt)
	}
	if error_value = rows.Err(); error_value != nil {
		return nil, fmt.Errorf("iterate delivery attempts: %w", error_value)
	}
	return attempts, nil
}
