package demoreceiver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sillypoise/p3-relay/internal/delivery"
)

// Exercise the real sender against a local TLS receiver: signed bytes, retry classifications,
// eventual success, and wrong-key rejection. This does not measure AWS networking or persistence.
func TestHTTPSDelivery(t *testing.T) {
	handler, err := New(receiverTestKey)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewTLSServer(handler)
	defer server.Close()

	client := server.Client()
	client.Timeout = 5 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	sender := delivery.NewHTTPSender(client, receiverTestKey, false)
	claimed := &delivery.ClaimedEvent{
		ID: "5a9c38c7-e229-4dad-a702-b03780ba69a7", DestinationURL: server.URL,
		Body: []byte(`{"scenario":"temporary_failure"}`),
	}
	for attempt := uint8(1); attempt <= 3; attempt++ {
		claimed.AttemptNumber = attempt
		result := sender.Send(context.Background(), claimed)
		want := "retryable_http"
		if attempt == 3 {
			want = "delivered"
		}
		if result.Outcome != want || result.ErrorCode != nil {
			t.Fatalf("attempt %d: unexpected classification", attempt)
		}
		if len(result.ResponseExcerpt) != 0 {
			t.Fatal("receiver reflected request data")
		}
	}
	wrongSender := delivery.NewHTTPSender(client, []byte("wrong-delivery-key"), false)
	result := wrongSender.Send(context.Background(), claimed)
	if result.StatusCode == nil || *result.StatusCode != http.StatusUnauthorized {
		t.Fatal("wrong delivery key accepted")
	}
}
