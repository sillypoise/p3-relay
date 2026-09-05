package notification

import (
	"context"
	"log/slog"
)

type Publisher interface {
	Publish(context.Context, string) error
}

// AfterCommit preserves durable receipt semantics when best-effort notification fails.
// A nil publisher selects PostgreSQL-only polling; it is not an authorization bypass.
func AfterCommit(ctx context.Context, publisherOrPollingOnly Publisher, eventID string) {
	if publisherOrPollingOnly == nil {
		return
	}
	if err := publisherOrPollingOnly.Publish(ctx, eventID); err != nil {
		slog.Error("notification publish failed; database reconciliation required", "event_id", eventID)
	}
}
