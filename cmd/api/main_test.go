package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthGetReturnsReadyResponse(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()

	new_handler().ServeHTTP(recorder, request)

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

	new_handler().ServeHTTP(recorder, request)

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

	new_handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
