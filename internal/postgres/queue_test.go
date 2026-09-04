package postgres

import (
	"testing"
	"time"

	"github.com/sillypoise/p3-relay/internal/delivery"
)

func TestRecordStateTransitions(t *testing.T) {
	now := time.Unix(1_757_023_200, 0)
	test_cases := []struct {
		name    string
		claimed *delivery.ClaimedEvent
		attempt *delivery.Attempt
		state   string
	}{
		{name: "success", claimed: claimed_at(now, 1), attempt: attempt_at(now, "delivered"), state: "delivered"},
		{name: "terminal", claimed: claimed_at(now, 1), attempt: attempt_at(now, "terminal_http"), state: "dead_lettered"},
		{name: "retry", claimed: claimed_at(now, 1), attempt: attempt_at(now, "network_error"), state: "retry_scheduled"},
		{name: "attempt exhaustion", claimed: claimed_at(now, 8), attempt: attempt_at(now, "network_error"), state: "dead_lettered"},
		{name: "age exhaustion", claimed: claimed_at(now.Add(-24*time.Hour), 1), attempt: attempt_at(now, "network_error"), state: "dead_lettered"},
	}
	for _, test_case := range test_cases {
		t.Run(test_case.name, func(t *testing.T) {
			if actual := record_state(test_case.claimed, test_case.attempt); actual != test_case.state {
				t.Fatalf("state = %q, want %q", actual, test_case.state)
			}
		})
	}
}

func claimed_at(created_at time.Time, attempt uint8) *delivery.ClaimedEvent {
	return &delivery.ClaimedEvent{CreatedAt: created_at, AttemptNumber: attempt}
}

func attempt_at(finished_at time.Time, outcome string) *delivery.Attempt {
	return &delivery.Attempt{FinishedAt: finished_at, Outcome: outcome}
}
