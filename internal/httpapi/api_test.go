package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sillypoise/p3-relay/internal/event"
	"github.com/sillypoise/p3-relay/internal/signature"
)

type store_stub struct {
	accepted event.Accepted
	error    error
	receipt      *event.Receipt
	replay_error error
	replayed_id  string
}

func (store *store_stub) Accept(_ context.Context, receipt *event.Receipt) (event.Accepted, error) {
	store.receipt = receipt
	return store.accepted, store.error
}

func (store *store_stub) Replay(_ context.Context, event_id string, _ string) error {
	store.replayed_id = event_id
	return store.replay_error
}

func TestEventsPostAcceptsSignedEvent(t *testing.T) {
	store := &store_stub{accepted: event.Accepted{ID: "5a9c38c7-e229-4dad-a702-b03780ba69a7"}}
	handler := test_api(store)
	request := signed_request(`{"kind":"invoice.created"}`)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusAccepted, response.Body)
	}
	if store.receipt == nil {
		t.Fatal("accepted event was not persisted")
	}
	if store.receipt.ExternalEventID != "external-1" {
		t.Fatalf("external ID = %q, want external-1", store.receipt.ExternalEventID)
	}
}

func TestEventsPostRejectsInvalidBoundaries(t *testing.T) {
	test_cases := map[string]func(*http.Request){
		"wrong source": func(request *http.Request) { request.URL.Path = "/v1/sources/wrong/events" },
		"wrong media":  func(request *http.Request) { request.Header.Set("Content-Type", "text/plain") },
		"empty body":   func(request *http.Request) { request.Body = http.NoBody },
		"bad json":     func(request *http.Request) { request.Body = io_body(`{"`) },
		"bad timestamp": func(request *http.Request) {
			request.Header.Set("X-Relay-Timestamp", "not-a-time")
		},
		"bad signature": func(request *http.Request) { request.Header.Set("X-Relay-Signature", "v1=00") },
		"empty event ID": func(request *http.Request) {
			request.Header.Set("X-Relay-Event-Id", "")
		},
	}
	for name, mutate := range test_cases {
		t.Run(name, func(t *testing.T) {
			store := &store_stub{}
			request := signed_request(`{"kind":"test"}`)
			mutate(request)
			response := httptest.NewRecorder()
			test_api(store).ServeHTTP(response, request)
			if response.Code < 400 || response.Code > 499 {
				t.Fatalf("status = %d, want bounded client error", response.Code)
			}
			if store.receipt != nil {
				t.Fatal("invalid event reached persistence")
			}
		})
	}
}

func TestEventsPostMapsPersistenceFailures(t *testing.T) {
	test_cases := []struct {
		error_value error
		status      int
	}{
		{error_value: event.ConflictError{}, status: http.StatusConflict},
		{error_value: errors.New("database unavailable"), status: http.StatusServiceUnavailable},
	}
	for _, test_case := range test_cases {
		store := &store_stub{error: test_case.error_value}
		response := httptest.NewRecorder()
		test_api(store).ServeHTTP(response, signed_request(`{"kind":"test"}`))
		if response.Code != test_case.status {
			t.Fatalf("status = %d, want %d", response.Code, test_case.status)
		}
		if strings.Contains(response.Body.String(), "database unavailable") {
			t.Fatal("internal error leaked to response")
		}
	}
}

func TestEventsReplayRequiresAuthorizationAndDeadLetterState(t *testing.T) {
	const event_id = "5a9c38c7-e229-4dad-a702-b03780ba69a7"
	test_cases := []struct {
		name        string
		token       string
		replay_error error
		status      int
	}{
		{name: "missing authorization", status: http.StatusUnauthorized},
		{name: "wrong authorization", token: "Bearer wrong-token-value", status: http.StatusUnauthorized},
		{name: "not found", token: "Bearer local-operator-token", replay_error: event.NotFoundError{}, status: http.StatusNotFound},
		{name: "invalid state", token: "Bearer local-operator-token", replay_error: event.InvalidStateError{}, status: http.StatusConflict},
		{name: "accepted", token: "Bearer local-operator-token", status: http.StatusAccepted},
	}
	for _, test_case := range test_cases {
		t.Run(test_case.name, func(t *testing.T) {
			store := &store_stub{replay_error: test_case.replay_error}
			request := httptest.NewRequest(http.MethodPost, "/v1/events/"+event_id+"/replay", nil)
			request.Header.Set("Authorization", test_case.token)
			response := httptest.NewRecorder()
			test_api(store).ServeHTTP(response, request)
			if response.Code != test_case.status {
				t.Fatalf("status = %d, want %d", response.Code, test_case.status)
			}
			if test_case.status == http.StatusAccepted && store.replayed_id != event_id {
				t.Fatal("authorized replay did not reach persistence")
			}
			if test_case.status == http.StatusUnauthorized && store.replayed_id != "" {
				t.Fatal("unauthorized replay reached persistence")
			}
		})
	}
}

func test_api(store event.Store) http.Handler {
	api := New(
		store,
		"local-source",
		[]byte("local-ingress-secret"),
		"http://127.0.0.1:9090/events",
		[]byte("local-operator-token"),
	)
	api.now = func() time.Time { return time.Unix(1_757_023_200, 0) }
	return api.Handler()
}

func signed_request(body string) *http.Request {
	timestamp := "1757023200"
	request := httptest.NewRequest(http.MethodPost, "/v1/sources/local-source/events", io_body(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Relay-Event-Id", "external-1")
	request.Header.Set("X-Relay-Timestamp", timestamp)
	request.Header.Set("X-Relay-Signature", signature.Create([]byte("local-ingress-secret"), timestamp, []byte(body)))
	return request
}

func io_body(value string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(value))
}
