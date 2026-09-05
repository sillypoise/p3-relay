package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sillypoise/p3-relay/internal/event"
	"github.com/sillypoise/p3-relay/internal/identifier"
	"github.com/sillypoise/p3-relay/internal/signature"
)

const (
	maximum_body_bytes = 256 * 1024
	timestamp_window   = 5 * time.Minute
)

type API struct {
	store           event.Store
	source_key      string
	ingress_secret  []byte
	destination_url string
	operator_token  []byte
	now             func() time.Time
}

func New(
	store event.Store,
	source_key string,
	ingress_secret []byte,
	destination_url string,
	operator_token []byte,
) *API {
	if store == nil {
		panic("event store is required")
	}
	if source_key == "" {
		panic("source key is required")
	}
	if len(ingress_secret) < 16 {
		panic("ingress secret must contain at least 16 bytes")
	}
	if destination_url == "" {
		panic("destination URL is required")
	}
	if len(operator_token) < 16 {
		panic("operator token must contain at least 16 bytes")
	}
	return &API{
		store: store, source_key: source_key, ingress_secret: ingress_secret,
		destination_url: destination_url, operator_token: operator_token, now: time.Now,
	}
}

func (api *API) Handler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /v1/sources/{source_key}/events", api.events_post)
	handler.HandleFunc("GET /v1/overview", api.overview_get)
	handler.HandleFunc("GET /v1/events", api.events_get)
	handler.HandleFunc("GET /v1/events/{event_id}", api.event_get)
	handler.HandleFunc("POST /v1/events/{event_id}/replay", api.events_replay_post)
	handler.HandleFunc("GET /v1/endpoint", api.endpoint_get)
	return handler
}

func (api *API) events_post(response http.ResponseWriter, request *http.Request) {
	if request.PathValue("source_key") != api.source_key {
		write_error(response, http.StatusUnauthorized, "invalid_signature", "Request authentication failed.")
		return
	}
	body, valid := read_body(response, request)
	if !valid {
		return
	}
	timestamp, valid := api.valid_timestamp(request.Header.Get("X-Relay-Timestamp"))
	if !valid {
		write_error(response, http.StatusBadRequest, "invalid_request", "Request timestamp is invalid.")
		return
	}
	if !signature.Valid(api.ingress_secret, timestamp, body, request.Header.Get("X-Relay-Signature")) {
		write_error(response, http.StatusUnauthorized, "invalid_signature", "Request authentication failed.")
		return
	}
	external_id := request.Header.Get("X-Relay-Event-Id")
	if !valid_event_id(external_id) {
		write_error(response, http.StatusBadRequest, "invalid_request", "Event identifier is invalid.")
		return
	}
	api.accept(response, request, external_id, body)
}

func (api *API) accept(response http.ResponseWriter, request *http.Request, external_id string, body []byte) {
	id, error_value := identifier.NewUUID()
	if error_value != nil {
		write_error(response, http.StatusServiceUnavailable, "receipt_unavailable", "Event receipt is unavailable.")
		return
	}
	body_digest := sha256.Sum256(body)
	accepted, error_value := api.store.Accept(request.Context(), &event.Receipt{
		ID: id, SourceKey: api.source_key, ExternalEventID: external_id, Body: body,
		BodySHA256: body_digest[:], DestinationURL: api.destination_url,
	})
	if error_value != nil {
		var conflict event.ConflictError
		if errors.As(error_value, &conflict) {
			write_error(response, http.StatusConflict, "idempotency_conflict", "Event identifier conflicts with an existing event.")
			return
		}
		write_error(response, http.StatusServiceUnavailable, "receipt_unavailable", "Event receipt is unavailable.")
		return
	}
	write_json(response, http.StatusAccepted, map[string]any{
		"event_id": accepted.ID, "status": "pending", "duplicate": accepted.Duplicate,
	})
}

func (api *API) overview_get(response http.ResponseWriter, request *http.Request) {
	if !api.require_authorization(response, request) {
		return
	}
	overview, error_value := api.store.Overview(request.Context(), api.source_key)
	if error_value != nil {
		write_error(response, http.StatusServiceUnavailable, "read_unavailable", "Overview is unavailable.")
		return
	}
	write_json(response, http.StatusOK, overview)
}

func (api *API) events_get(response http.ResponseWriter, request *http.Request) {
	if !api.require_authorization(response, request) {
		return
	}
	state := request.URL.Query().Get("state")
	if state != "" && !valid_state(state) {
		write_error(response, http.StatusBadRequest, "invalid_request", "Event state is invalid.")
		return
	}
	events, error_value := api.store.List(request.Context(), api.source_key, state, 50)
	if error_value != nil {
		write_error(response, http.StatusServiceUnavailable, "read_unavailable", "Events are unavailable.")
		return
	}
	write_json(response, http.StatusOK, map[string]any{"events": events})
}

func (api *API) event_get(response http.ResponseWriter, request *http.Request) {
	if !api.require_authorization(response, request) {
		return
	}
	event_id := request.PathValue("event_id")
	if !identifier.ValidUUID(event_id) {
		write_error(response, http.StatusBadRequest, "invalid_request", "Event identifier is invalid.")
		return
	}
	detail, error_value := api.store.Detail(request.Context(), event_id, api.source_key)
	if error_value == nil {
		write_json(response, http.StatusOK, detail)
		return
	}
	var not_found event.NotFoundError
	if errors.As(error_value, &not_found) {
		write_error(response, http.StatusNotFound, "event_not_found", "Event was not found.")
		return
	}
	write_error(response, http.StatusServiceUnavailable, "read_unavailable", "Event is unavailable.")
}

func (api *API) endpoint_get(response http.ResponseWriter, request *http.Request) {
	if !api.require_authorization(response, request) {
		return
	}
	write_json(response, http.StatusOK, map[string]any{
		"source_key": api.source_key, "destination_url": api.destination_url,
		"enabled": true, "secrets": "write_only",
	})
}

func (api *API) events_replay_post(response http.ResponseWriter, request *http.Request) {
	if !api.authorized(request.Header.Get("Authorization")) {
		write_error(response, http.StatusUnauthorized, "unauthorized", "Authorization is required.")
		return
	}
	event_id := request.PathValue("event_id")
	if !identifier.ValidUUID(event_id) {
		write_error(response, http.StatusBadRequest, "invalid_request", "Event identifier is invalid.")
		return
	}
	error_value := api.store.Replay(request.Context(), event_id, api.source_key)
	if error_value == nil {
		write_json(response, http.StatusAccepted, map[string]string{"event_id": event_id, "status": "pending"})
		return
	}
	var not_found event.NotFoundError
	if errors.As(error_value, &not_found) {
		write_error(response, http.StatusNotFound, "event_not_found", "Event was not found.")
		return
	}
	var invalid_state event.InvalidStateError
	if errors.As(error_value, &invalid_state) {
		write_error(response, http.StatusConflict, "invalid_event_state", "Event cannot be replayed.")
		return
	}
	write_error(response, http.StatusServiceUnavailable, "replay_unavailable", "Replay is unavailable.")
}

func (api *API) require_authorization(response http.ResponseWriter, request *http.Request) bool {
	if api.authorized(request.Header.Get("Authorization")) {
		return true
	}
	write_error(response, http.StatusUnauthorized, "unauthorized", "Authorization is required.")
	return false
}

func (api *API) authorized(value string) bool {
	const prefix = "Bearer "
	if len(value) <= len(prefix) || value[:len(prefix)] != prefix {
		return false
	}
	supplied_digest := sha256.Sum256([]byte(value[len(prefix):]))
	expected_digest := sha256.Sum256(api.operator_token)
	return subtle.ConstantTimeCompare(supplied_digest[:], expected_digest[:]) == 1
}

func (api *API) valid_timestamp(raw string) (string, bool) {
	seconds, error_value := strconv.ParseInt(raw, 10, 64)
	if error_value != nil {
		return "", false
	}
	difference := api.now().Sub(time.Unix(seconds, 0))
	if difference < -timestamp_window {
		return "", false
	}
	if difference > timestamp_window {
		return "", false
	}
	return raw, true
}

func read_body(response http.ResponseWriter, request *http.Request) ([]byte, bool) {
	if request.Header.Get("Content-Type") != "application/json" {
		write_error(response, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content type must be application/json.")
		return nil, false
	}
	request.Body = http.MaxBytesReader(response, request.Body, maximum_body_bytes)
	body, error_value := io.ReadAll(request.Body)
	if error_value != nil {
		write_error(response, http.StatusRequestEntityTooLarge, "payload_too_large", "Request body is too large.")
		return nil, false
	}
	if len(body) == 0 || !json.Valid(body) {
		write_error(response, http.StatusBadRequest, "invalid_request", "Request body must be valid JSON.")
		return nil, false
	}
	return body, true
}

func valid_state(value string) bool {
	states := [...]string{"pending", "delivering", "delivered", "retry_scheduled", "dead_lettered"}
	for _, state := range states {
		if value == state {
			return true
		}
	}
	return false
}

func valid_event_id(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x20 || character > 0x7e {
			return false
		}
	}
	return true
}

func write_error(response http.ResponseWriter, status int, code string, message string) {
	write_json(response, status, map[string]any{
		"error": map[string]string{"code": code, "message": message, "request_id": "unavailable"},
	})
}

func write_json(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
