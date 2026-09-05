package delivery

import "time"

// Sandbox scenarios execute in-process, never through the outbound network client.
// Receipt, queueing, retry timing and attempt persistence still use the real worker.
func sandbox_attempt(claimed *ClaimedEvent, now time.Time) *Attempt {
	var status uint16 = 422
	switch claimed.DestinationURL {
	case "relay-simulator://success":
		status = 204
	case "relay-simulator://temporary_failure":
		status = 204
		if claimed.AttemptNumber < 3 {
			status = 503
		}
	case "relay-simulator://permanent_failure":
		status = 422
	}
	outcome := HTTPOutcome(status)
	return &Attempt{ID: claimed.ClaimID, StartedAt: now, FinishedAt: now,
		Outcome: outcome, StatusCode: &status, ResponseExcerpt: []byte("Simulated receiver response; no network request."),
		NextAttemptAt: next_attempt_at(claimed, now, outcome)}
}
