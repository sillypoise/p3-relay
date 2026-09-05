package main

import (
	"github.com/sillypoise/p3-relay/internal/webui"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Static delivery must never replace operator authorization or health routing.
func TestStaticSitePreservesAPIBoundary(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("Relay dashboard"), 0600); err != nil {
		t.Fatal(err)
	}
	site, err := webui.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := site.Close(); err != nil {
			t.Error(err)
		}
	}()
	handler := new_handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	handler.Handle("/", site)
	for path, status := range map[string]int{"/": 200, "/events": 200, "/health": 200, "/v1/events": 401, "/v1/sandbox/events": 401} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatalf("route %s status %d want %d", path, w.Code, status)
		}
	}
}

func TestHealthGetReturnsReadyResponse(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()

	new_handler(http.NotFoundHandler()).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q, want application/json", recorder.Header().Get("Content-Type"))
	}
	if recorder.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q, want health response", recorder.Body.String())
	}
}

func TestHealthRejectsUnsupportedMethod(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/health", nil)
	recorder := httptest.NewRecorder()

	new_handler(http.NotFoundHandler()).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
	body, error_value := io.ReadAll(recorder.Result().Body)
	if error_value != nil {
		t.Fatalf("read response body: %v", error_value)
	}
	if string(body) != "Method Not Allowed\n" {
		t.Fatalf("body = %q, want method error", string(body))
	}
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	recorder := httptest.NewRecorder()

	new_handler(http.NotFoundHandler()).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
