package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/sillypoise/p3-relay/internal/event"
	"github.com/sillypoise/p3-relay/internal/identifier"
	"github.com/sillypoise/p3-relay/internal/postgres"
)

type Handler struct {
	Store  *Store
	Reads  *postgres.Store
	Key    []byte
	Origin string
}

func (h *Handler) Valid() bool {
	parsed, err := url.Parse(h.Origin)
	if err != nil {
		return false
	}
	return len(h.Key) == 32 && parsed.Scheme == "https" && parsed.Host != "" &&
		parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if h.Valid() == false {
		replyError(w, 503, "sandbox_unavailable")
		return
	}
	// Reject ambient/operator authority; this boundary accepts only its own cookie.
	if r.Header.Get("Authorization") != "" {
		replyError(w, 401, "invalid_session")
		return
	}
	if r.Method == http.MethodPost {
		if r.Header.Get("Origin") != h.Origin {
			replyError(w, 403, "invalid_origin")
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			replyError(w, 400, "invalid_request")
			return
		}
	}
	if r.URL.Path == "/v1/sandbox/session" && r.Method == http.MethodPost {
		h.create(w, r)
		return
	}
	cookies := r.CookiesNamed(cookieName)
	if len(cookies) != 1 {
		replyError(w, 401, "invalid_session")
		return
	}
	id, err := Validate(h.Key, cookies[0].Value, time.Now())
	if err != nil {
		replyError(w, 401, "invalid_session")
		return
	}
	if err = h.Store.Active(ctx, id); err != nil {
		h.failure(w, err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/sandbox/events", func(w http.ResponseWriter, r *http.Request) { h.list(w, r, id) })
	mux.HandleFunc("POST /v1/sandbox/events", func(w http.ResponseWriter, r *http.Request) { h.submit(w, r, id) })
	mux.HandleFunc("GET /v1/sandbox/events/{id}", func(w http.ResponseWriter, r *http.Request) { h.detail(w, r, id) })
	mux.HandleFunc("POST /v1/sandbox/events/{id}/replay", func(w http.ResponseWriter, r *http.Request) { h.replay(w, r, id) })
	mux.ServeHTTP(w, r)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var empty struct{}
	if decode(w, r, &empty) == false {
		return
	}
	// A valid existing cookie cannot mint a fresh quota via repeated clicks.
	if cookies := r.CookiesNamed(cookieName); len(cookies) > 0 {
		if len(cookies) != 1 {
			replyError(w, 401, "invalid_session")
			return
		}
		if id, err := Validate(h.Key, cookies[0].Value, time.Now()); err == nil {
			if err = h.Store.Active(r.Context(), id); err == nil {
				reply(w, 200, map[string]bool{"active": true})
				return
			}
			if errors.Is(err, ErrSession) == false {
				h.failure(w, err)
				return
			}
		}
	}
	if err := h.Store.Cleanup(r.Context()); err != nil {
		h.failure(w, err)
		return
	}
	cookie, id, err := Issue(h.Key, time.Now())
	if err != nil {
		h.failure(w, err)
		return
	}
	if err = h.Store.Create(r.Context(), id, cookie.Expires); err != nil {
		h.failure(w, err)
		return
	}
	slog.Info("sandbox session admitted")
	http.SetCookie(w, &cookie)
	reply(w, 201, map[string]bool{"active": true})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, id string) {
	values, err := h.Reads.List(r.Context(), "sandbox:"+id, "", 20)
	if err != nil {
		h.failure(w, err)
		return
	}
	reply(w, 200, map[string]any{"events": values})
}
func (h *Handler) detail(w http.ResponseWriter, r *http.Request, id string) {
	if identifier.ValidUUID(r.PathValue("id")) == false {
		replyError(w, 400, "invalid_request")
		return
	}
	value, err := h.Reads.Detail(r.Context(), r.PathValue("id"), "sandbox:"+id)
	if err != nil {
		h.failure(w, err)
		return
	}
	reply(w, 200, value)
}
func (h *Handler) submit(w http.ResponseWriter, r *http.Request, id string) {
	var input struct {
		Scenario string `json:"scenario"`
	}
	if decode(w, r, &input) == false {
		return
	}
	if ValidScenario(input.Scenario) == false {
		replyError(w, 400, "invalid_request")
		return
	}
	eventID, err := h.Store.Submit(r.Context(), id, input.Scenario)
	if err != nil {
		h.failure(w, err)
		return
	}
	slog.Info("sandbox event admitted", "event_id", eventID)
	reply(w, 202, map[string]string{"event_id": eventID, "status": "pending"})
}
func (h *Handler) replay(w http.ResponseWriter, r *http.Request, id string) {
	var empty struct{}
	if decode(w, r, &empty) == false {
		return
	}
	if identifier.ValidUUID(r.PathValue("id")) == false {
		replyError(w, 400, "invalid_request")
		return
	}
	if err := h.Store.Replay(r.Context(), id, r.PathValue("id")); err != nil {
		h.failure(w, err)
		return
	}
	slog.Info("sandbox replay admitted", "event_id", r.PathValue("id"))
	reply(w, 202, map[string]string{"status": "pending"})
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		replyError(w, 400, "invalid_request")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		replyError(w, 400, "invalid_request")
		return false
	}
	return true
}
func (h *Handler) failure(w http.ResponseWriter, err error) {
	var missing event.NotFoundError
	switch {
	case errors.Is(err, ErrSession):
		replyError(w, 401, "invalid_session")
	case errors.Is(err, ErrQuota):
		replyError(w, 429, "quota_exceeded")
	case errors.Is(err, ErrMissing), errors.As(err, &missing):
		replyError(w, 404, "event_not_found")
	case errors.Is(err, ErrState):
		replyError(w, 409, "invalid_event_state")
	default:
		slog.Error("sandbox persistence unavailable")
		replyError(w, 503, "sandbox_unavailable")
	}
}
func replyError(w http.ResponseWriter, status int, code string) {
	requestID, err := identifier.NewUUID()
	if err != nil {
		requestID = "unavailable"
	}
	slog.Info("sandbox request denied", "status", status, "code", code, "request_id", requestID)
	reply(w, status, map[string]any{"error": map[string]string{"code": code, "message": code, "request_id": requestID}})
}
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Warn("sandbox response write failed")
	}
}
