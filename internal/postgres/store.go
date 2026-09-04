package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sillypoise/p3-relay/internal/event"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	if pool == nil {
		panic("PostgreSQL pool is required")
	}
	return &Store{pool: pool}
}

func (store *Store) Accept(context_value context.Context, receipt *event.Receipt) (event.Accepted, error) {
	if receipt == nil {
		panic("receipt is required")
	}
	const insert_query = `
		INSERT INTO p3_relay.events (
			id, source_key, external_event_id, body, body_sha256, destination_url, state
		) VALUES ($1, $2, $3, $4, $5, $6, 'pending')
		ON CONFLICT (source_key, external_event_id) DO NOTHING
		RETURNING id`
	var accepted_id string
	error_value := store.pool.QueryRow(
		context_value,
		insert_query,
		receipt.ID,
		receipt.SourceKey,
		receipt.ExternalEventID,
		receipt.Body,
		receipt.BodySHA256,
		receipt.DestinationURL,
	).Scan(&accepted_id)
	if error_value == nil {
		return event.Accepted{ID: accepted_id, Duplicate: false}, nil
	}
	if !errors.Is(error_value, pgx.ErrNoRows) {
		return event.Accepted{}, fmt.Errorf("insert event: %w", error_value)
	}
	return store.accept_existing(context_value, receipt)
}

func (store *Store) accept_existing(context_value context.Context, receipt *event.Receipt) (event.Accepted, error) {
	const select_query = `
		SELECT id, body_sha256
		FROM p3_relay.events
		WHERE source_key = $1 AND external_event_id = $2`
	var accepted_id string
	var body_sha256 []byte
	error_value := store.pool.QueryRow(
		context_value,
		select_query,
		receipt.SourceKey,
		receipt.ExternalEventID,
	).Scan(&accepted_id, &body_sha256)
	if error_value != nil {
		return event.Accepted{}, fmt.Errorf("read existing event: %w", error_value)
	}
	if !bytes.Equal(body_sha256, receipt.BodySHA256) {
		return event.Accepted{}, event.ConflictError{}
	}
	return event.Accepted{ID: accepted_id, Duplicate: true}, nil
}
