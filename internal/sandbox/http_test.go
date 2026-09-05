package sandbox

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Invalid trust-boundary requests must return before touching the nil database dependencies.
func TestHTTPBoundaryFailsClosed(t *testing.T) {
	h := &Handler{Origin: "https://relay.test", Key: []byte(strings.Repeat("k", 32))}
	tests := []struct {
		method, path, origin, authorization, cookie string
		status                                      int
	}{
		{"POST", "/v1/sandbox/session", "", "", "", 403},
		{"POST", "/v1/sandbox/session", "https://evil.test", "", "", 403},
		{"POST", "/v1/sandbox/session", h.Origin, "Bearer operator-token", "", 401},
		{"GET", "/v1/sandbox/events", "", "", "", 401},
		{"GET", "/v1/sandbox/events", "", "", "__Host-relay_sandbox=forged", 401},
		{"GET", "/v1/sandbox/events", "", "", "__Host-relay_sandbox=a; __Host-relay_sandbox=b", 401},
	}
	for _, tc := range tests {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", tc.authorization)
		r.Header.Set("Cookie", tc.cookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("status %d want %d", w.Code, tc.status)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private response may be cached")
		}
	}
}

func TestConfigurationRejectsInsecureOrigins(t *testing.T) {
	for _, origin := range []string{"", "http://relay.test", "https://relay.test/path",
		"https://user:password@relay.test", "https://relay.test?x=y", "https://relay.test#fragment"} {
		h := &Handler{Origin: origin, Key: []byte(strings.Repeat("k", 32))}
		if h.Valid() {
			t.Fatalf("invalid origin accepted: %s", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/sandbox/events", nil))
		if w.Code != 503 {
			t.Fatal("invalid configuration did not fail closed")
		}
	}
}
