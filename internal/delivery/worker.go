package delivery

import (
	"context"
	"fmt"
	"time"
)

type ClaimedEvent struct {
	ID             string
	ClaimID        string
	Body           []byte
	DestinationURL string
	ReplayNumber   uint8
	AttemptNumber  uint8
	CreatedAt      time.Time
}

type Attempt struct {
	ID              string
	StartedAt       time.Time
	FinishedAt      time.Time
	Outcome         string
	StatusCode      *uint16
	ResponseExcerpt []byte
	ErrorCode       *string
	NextAttemptAt   time.Time
}

type QueueStore interface {
	Claim(context.Context, time.Time, time.Duration) (*ClaimedEvent, error)
	Record(context.Context, *ClaimedEvent, *Attempt) error
}

type Sender interface {
	Send(context.Context, *ClaimedEvent) *Attempt
}

type Worker struct {
	store        QueueStore
	sender       Sender
	lease_length time.Duration
}

func NewWorker(store QueueStore, sender Sender, lease_length time.Duration) *Worker {
	if store == nil {
		panic("queue store is required")
	}
	if sender == nil {
		panic("delivery sender is required")
	}
	if lease_length <= 0 {
		panic("positive lease length is required")
	}
	return &Worker{store: store, sender: sender, lease_length: lease_length}
}

func (worker *Worker) RunOnce(context_value context.Context, now time.Time) (bool, error) {
	claimed, error_value := worker.store.Claim(context_value, now, worker.lease_length)
	if error_value != nil {
		return false, fmt.Errorf("claim event: %w", error_value)
	}
	if claimed == nil {
		return false, nil
	}
	attempt := worker.sender.Send(context_value, claimed)
	if attempt == nil {
		panic("delivery sender returned a nil attempt")
	}
	if error_value = worker.store.Record(context_value, claimed, attempt); error_value != nil {
		return false, fmt.Errorf("record attempt: %w", error_value)
	}
	return true, nil
}
