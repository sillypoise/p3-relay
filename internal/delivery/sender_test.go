package delivery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sillypoise/p3-relay/internal/signature"
)

func TestHTTPSenderSignsAndClassifiesDelivery(t *testing.T) {
	secret := []byte("delivery-test-secret")
	receiver := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body := []byte(`{"kind":"test"}`)
		timestamp := request.Header.Get("X-Relay-Timestamp")
		if !signature.Valid(secret, timestamp, body, request.Header.Get("X-Relay-Signature")) {
			t.Error("delivery signature is invalid")
		}
		response.WriteHeader(http.StatusServiceUnavailable)
		_, _ = response.Write([]byte("retry later"))
	}))
	defer receiver.Close()

	sender := NewHTTPSender(receiver.Client(), secret, true)
	fixed_time := time.Unix(1_757_023_200, 0)
	sender.now = func() time.Time { return fixed_time }
	attempt := sender.Send(context.Background(), &ClaimedEvent{
		ID: "event-1", ClaimID: "claim-1", Body: []byte(`{"kind":"test"}`),
		DestinationURL: receiver.URL, AttemptNumber: 1, CreatedAt: fixed_time,
	})

	if attempt.Outcome != "retryable_http" {
		t.Fatalf("outcome = %q, want retryable_http", attempt.Outcome)
	}
	if attempt.StatusCode == nil || *attempt.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %v, want %d", attempt.StatusCode, http.StatusServiceUnavailable)
	}
	if string(attempt.ResponseExcerpt) != "retry later" {
		t.Fatalf("excerpt = %q, want retry later", attempt.ResponseExcerpt)
	}
}

func TestHTTPSenderRecordsNetworkFailure(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Millisecond}
	sender := NewHTTPSender(client, []byte("delivery-test-secret"), false)
	attempt := sender.Send(context.Background(), &ClaimedEvent{
		ID: "event-1", ClaimID: "claim-1", Body: []byte(`{}`),
		DestinationURL: "://invalid", AttemptNumber: 8, CreatedAt: time.Now(),
	})
	if attempt.Outcome != "network_error" {
		t.Fatalf("outcome = %q, want network_error", attempt.Outcome)
	}
	if attempt.ErrorCode == nil {
		t.Fatal("network error code is missing")
	}
}
