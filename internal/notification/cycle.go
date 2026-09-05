package notification

import (
	"context"
	"log/slog"
	"time"
)

type Runner interface {
	RunOnce(context.Context, time.Time) (bool, error)
}
type Consumer interface {
	Receive(context.Context) (Batch, error)
	Acknowledge(context.Context, *Batch) error
}

// Cycle reconciles current database state even if the notification was lost or receive failed.
// Hints never choose an event, supply a destination, or bypass a PostgreSQL claim predicate.
func Cycle(ctx context.Context, runner Runner, queueOrPollingOnly Consumer) error {
	if runner == nil {
		panic("delivery runner required")
	}
	var batch Batch
	var receiveError error
	if queueOrPollingOnly != nil {
		batch, receiveError = queueOrPollingOnly.Receive(ctx)
	}
	if receiveError != nil {
		slog.Error("notification receive failed; reconciling database")
	}
	for index := uint8(0); index < BatchSize; index++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		attemptContext, cancel := context.WithTimeout(ctx, 20*time.Second)
		processed, err := runner.RunOnce(attemptContext, time.Now())
		cancel()
		// Preserve handles when any database/worker operation fails. SQS may redeliver safely.
		if err != nil {
			return err
		}
		if processed == false {
			break
		}
	}
	if queueOrPollingOnly != nil && receiveError == nil {
		if err := queueOrPollingOnly.Acknowledge(ctx, &batch); err != nil {
			slog.Error("notification acknowledgement failed; hints may redeliver")
			return err
		}
	}
	return receiveError
}
