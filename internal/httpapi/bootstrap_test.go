package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Bootstrap disables only new operator receipts, without weakening authentication or recording
// undeliverable events. Configured destinations preserve the existing acceptance contract.
func TestUnconfiguredDestination(t *testing.T) {
	store := &store_stub{}
	api := New(store, "local-source", []byte("local-ingress-secret"), "",
		[]byte("local-operator-token"))
	api.now = func() time.Time { return time.Unix(1757023200, 0) }
	for _, authenticated := range []bool{false, true} {
		request := signed_request(`{"scenario":"success"}`)
		status := http.StatusServiceUnavailable
		if !authenticated {
			request.Header.Set("X-Relay-Signature", "invalid")
			status = http.StatusUnauthorized
		}
		response := httptest.NewRecorder()
		api.Handler().ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("status %d; want %d", response.Code, status)
		}
		if store.receipt != nil {
			t.Fatal("disabled ingress reached persistence")
		}
	}
	request := httptest.NewRequest("GET", "/v1/endpoint", nil)
	request.Header.Set("Authorization", "Bearer local-operator-token")
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"enabled":false`) {
		t.Fatal("unconfigured endpoint presented as enabled")
	}
}
