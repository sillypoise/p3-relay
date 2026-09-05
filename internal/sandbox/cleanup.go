package sandbox

import "context"

// Cleanup removes at most 100 expired events per invocation, never active delivery leases.
// Admission invokes it before checking retained-session capacity; quotas outlive deleted sessions.
func (store *Store) Cleanup(ctx context.Context) error {
	tx, err := store.admission(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	rows, err := tx.Query(ctx, `SELECT e.id FROM p3_relay.events e
 JOIN p3_relay.sandbox_sessions s ON e.sandbox_id=s.id
 WHERE s.expires_at<=clock_timestamp()
 AND (e.lease_expires_at IS NULL OR e.lease_expires_at<=clock_timestamp())
 ORDER BY e.id LIMIT 100 FOR UPDATE OF e SKIP LOCKED`)
	if err != nil {
		return err
	}
	ids := make([]string, 0, 100)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM p3_relay.delivery_attempts WHERE event_id=ANY($1::uuid[])`, ids); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM p3_relay.events WHERE id=ANY($1::uuid[])`, ids); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM p3_relay.sandbox_sessions WHERE id IN (
 SELECT s.id FROM p3_relay.sandbox_sessions s WHERE expires_at<=clock_timestamp()
 AND NOT EXISTS(SELECT FROM p3_relay.events e WHERE e.sandbox_id=s.id) LIMIT 100)`)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
