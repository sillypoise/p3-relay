package delivery

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type forbiddenTransport struct{ t *testing.T }

func (transport forbiddenTransport) RoundTrip(*http.Request) (*http.Response, error) {
	transport.t.Fatal("sandbox attempted outbound network access")
	return nil, nil
}

// Even a corrupted sandbox destination must never reach the HTTP transport.
func TestSandboxNeverUsesNetwork(t *testing.T) {
	sender := NewHTTPSender(&http.Client{Transport: forbiddenTransport{t}, Timeout: time.Second},
		[]byte("test-secret-at-least-16-bytes"), false)
	cases := []struct {
		destination string
		attempt     uint8
		status      uint16
	}{
		{"relay-simulator://success", 1, 204},
		{"relay-simulator://temporary_failure", 1, 503},
		{"relay-simulator://temporary_failure", 2, 503},
		{"relay-simulator://temporary_failure", 3, 204},
		{"relay-simulator://permanent_failure", 1, 422},
		{"http://169.254.169.254/", 1, 422},
		{"https://example.com/", 1, 422},
	}
	for _, tc := range cases {
		attempt := sender.Send(context.Background(), &ClaimedEvent{Sandbox: true, ClaimID: "test",
			ID: "test", AttemptNumber: tc.attempt, DestinationURL: tc.destination})
		if attempt.StatusCode == nil || *attempt.StatusCode != tc.status {
			t.Fatalf("unexpected outcome: %+v", attempt)
		}
		if len(attempt.ResponseExcerpt) > 4096 {
			t.Fatal("excerpt exceeds bound")
		}
	}
}
