package delivery

import (
	"context"
	"errors"
	"testing"
	"time"
)

type queue_stub struct {
	claimed      *ClaimedEvent
	claim_error  error
	record_error error
	recorded     *Attempt
}

func (queue *queue_stub) Claim(context.Context, time.Time, time.Duration) (*ClaimedEvent, error) {
	return queue.claimed, queue.claim_error
}

func (queue *queue_stub) Record(_ context.Context, _ *ClaimedEvent, attempt *Attempt) error {
	queue.recorded = attempt
	return queue.record_error
}

type sender_stub struct{ attempt *Attempt }

func (sender sender_stub) Send(context.Context, *ClaimedEvent) *Attempt { return sender.attempt }

func TestWorkerRunOnceHandlesEmptyQueue(t *testing.T) {
	queue := &queue_stub{}
	worker := NewWorker(queue, sender_stub{attempt: &Attempt{}}, 30*time.Second)
	processed, error_value := worker.RunOnce(context.Background(), time.Now())
	if error_value != nil || processed {
		t.Fatalf("processed = %v, error = %v; want false and nil", processed, error_value)
	}
}

func TestWorkerRunOnceRecordsAttempt(t *testing.T) {
	attempt := &Attempt{Outcome: "delivered"}
	queue := &queue_stub{claimed: &ClaimedEvent{ID: "event-1"}}
	worker := NewWorker(queue, sender_stub{attempt: attempt}, 30*time.Second)
	processed, error_value := worker.RunOnce(context.Background(), time.Now())
	if error_value != nil || !processed {
		t.Fatalf("processed = %v, error = %v; want true and nil", processed, error_value)
	}
	if queue.recorded != attempt {
		t.Fatal("delivery attempt was not recorded")
	}
}

func TestWorkerRunOncePropagatesStoreErrors(t *testing.T) {
	queue := &queue_stub{claim_error: errors.New("unavailable")}
	worker := NewWorker(queue, sender_stub{attempt: &Attempt{}}, 30*time.Second)
	if _, error_value := worker.RunOnce(context.Background(), time.Now()); error_value == nil {
		t.Fatal("claim failure was ignored")
	}
}
