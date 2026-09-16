package demoreceiver

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sillypoise/p3-relay/internal/signature"
)

var receiverTestKey = []byte("independent-demo-receiver-test-key")

// Exercise every fixture/attempt combination, including repetition: this receiver has no state
// or business effects to roll back, and does not claim exactly-once handling or durable receipt.
func TestScenarios(t *testing.T) {
	handler := testHandler(t)
	for _, scenario := range []string{"success", "temporary_failure", "permanent_failure"} {
		for attempt := uint8(1); attempt <= 8; attempt++ {
			status := http.StatusNoContent
			if scenario == "temporary_failure" && attempt < 3 {
				status = http.StatusServiceUnavailable
			}
			if scenario == "permanent_failure" {
				status = http.StatusUnprocessableEntity
			}
			for repeat := uint8(0); repeat < 2; repeat++ {
				request := testRequest(`{"scenario":"` + scenario + `"}`)
				request.Header.Set("X-Relay-Attempt", strconv.FormatUint(uint64(attempt), 10))
				checkResponse(t, handler, request, status)
			}
		}
	}
}

// Reject credential/header confusion, timestamp overflow, body/schema errors and reader failures.
// Failures must release admission slots and return neither payloads nor authentication material.
func TestRejectedRequests(t *testing.T) {
	cases := []struct {
		name   string
		status int
		mutate func(*http.Request)
	}{
		{"method", 405, func(r *http.Request) { r.Method = "GET" }},
		{"media", 400, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }},
		{"query", 400, func(r *http.Request) { r.URL.RawQuery = "scenario=success" }},
		{"unsigned", 401, func(r *http.Request) { r.Header.Del("X-Relay-Signature") }},
		{"signature", 401, func(r *http.Request) { r.Header.Set("X-Relay-Signature", "v1=00") }},
		{"duplicate", 401, func(r *http.Request) { r.Header.Add("X-Relay-Timestamp", "0") }},
		{"time syntax", 401, func(r *http.Request) { r.Header.Set("X-Relay-Timestamp", "no") }},
		{"overflow", 401, func(r *http.Request) {
			r.Header.Set("X-Relay-Timestamp", "9223372036854775808")
		}},
		{"identifier", 401, func(r *http.Request) { r.Header.Set("X-Relay-Event-Id", "bad") }},
		{"zero attempt", 401, func(r *http.Request) { r.Header.Set("X-Relay-Attempt", "0") }},
		{"ninth attempt", 401, func(r *http.Request) { r.Header.Set("X-Relay-Attempt", "9") }},
		{"padded attempt", 401, func(r *http.Request) { r.Header.Set("X-Relay-Attempt", "01") }},
		{"read failure", 400, func(r *http.Request) { r.Body = io.NopCloser(failedReader{}) }},
		{"changed body", 401, func(r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(`{"scenario":"permanent_failure"}`))
		}},
	}
	for _, entry := range cases {
		t.Run(entry.name, func(t *testing.T) {
			handler := testHandler(t)
			request := testRequest(`{"scenario":"success"}`)
			entry.mutate(request)
			checkResponse(t, handler, request, entry.status)
			if len(handler.slots) != 0 {
				t.Fatal("rejected request retained admission slot")
			}
			checkResponse(t, handler, testRequest(`{"scenario":"success"}`), 204)
		})
	}
	for _, body := range []string{"", "{", "null", `{"scenario":"unknown"}`, "{} {}"} {
		checkResponse(t, testHandler(t), testRequest(body), 400)
	}
}

// Include/exclude both timestamp endpoints and the exact payload bound, then saturate admission
// without spawning workers. Releasing a held slot must restore availability without extra state.
func TestBoundsAndOverload(t *testing.T) {
	handler := testHandler(t)
	for _, seconds := range []int64{-301, -300, 300, 301} {
		request := testRequest(`{"scenario":"success"}`)
		timestamp := strconv.FormatInt(handler.now().Unix()+seconds, 10)
		request.Header.Set("X-Relay-Timestamp", timestamp)
		request.Header.Set("X-Relay-Signature",
			signature.Create(receiverTestKey, timestamp, []byte(`{"scenario":"success"}`)))
		status := 204
		if seconds < -300 || seconds > 300 {
			status = 401
		}
		checkResponse(t, handler, request, status)
	}
	body := `{"scenario":"success"}`
	body += strings.Repeat(" ", bodyBytesMax-len(body))
	checkResponse(t, handler, testRequest(body), 204)
	checkResponse(t, handler, testRequest(body+" "), 413)
	for index := 0; index < cap(handler.slots); index++ {
		handler.slots <- struct{}{}
	}
	checkResponse(t, handler, testRequest(`{"scenario":"success"}`), 503)
	<-handler.slots
	checkResponse(t, handler, testRequest(`{"scenario":"success"}`), 204)
	for index := 0; index < cap(handler.slots)-1; index++ {
		<-handler.slots
	}
	for _, length := range []uint16{0, 15, 16, 256, 257} {
		_, err := New([]byte(strings.Repeat("k", int(length))))
		if (err == nil) != (length >= 16 && length <= 256) {
			t.Fatal("incorrect secret length boundary")
		}
	}
}

func testHandler(t *testing.T) *Handler {
	t.Helper()
	handler, err := New(receiverTestKey)
	if err != nil {
		t.Fatal(err)
	}
	handler.now = func() time.Time { return time.Unix(1800000000, 0) }
	return handler
}

func testRequest(body string) *http.Request {
	request := httptest.NewRequest("POST", "/v1/demo-receiver", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Relay-Timestamp", "1800000000")
	request.Header.Set("X-Relay-Event-Id", "5a9c38c7-e229-4dad-a702-b03780ba69a7")
	request.Header.Set("X-Relay-Attempt", "1")
	request.Header.Set("X-Relay-Signature",
		signature.Create(receiverTestKey, "1800000000", []byte(body)))
	return request
}

func checkResponse(t *testing.T, handler *Handler, request *http.Request, status int) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("status %d; expected %d", response.Code, status)
	}
	if response.Body.Len() != 0 {
		t.Fatal("receiver echoed a body or diagnostic")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("receiver response could be cached")
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) {
	return 0, errors.New("private reader failure detail")
}
