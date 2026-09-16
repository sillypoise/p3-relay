// Package demoreceiver provides a stateless, authenticated synthetic HTTPS destination.
package demoreceiver

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/sillypoise/p3-relay/internal/identifier"
	"github.com/sillypoise/p3-relay/internal/signature"
)

const (
	bodyBytesMax = 256 * 1024
	// Four requests bound retained raw payloads to roughly 1 MiB, excluding allocator overhead.
	bodyBytesInFlightMax = 1024 * 1024
	timestampWindow      = 300 * time.Second
)

type Handler struct {
	secret []byte
	slots  chan struct{}
	now    func() time.Time
}

func New(secret []byte) (*Handler, error) {
	if len(secret) < 16 || len(secret) > 256 {
		return nil, errors.New("invalid demo receiver key length")
	}
	return &Handler{
		secret: append([]byte(nil), secret...),
		slots:  make(chan struct{}, bodyBytesInFlightMax/bodyBytesMax), now: time.Now,
	}, nil
}

func (handler *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Cache-Control", "no-store")
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", http.MethodPost)
		reject(response, http.StatusMethodNotAllowed)
		return
	}
	if request.URL.RawQuery != "" || request.Header.Get("Content-Type") != "application/json" {
		reject(response, http.StatusBadRequest)
		return
	}
	if !handler.validHeaders(request) {
		reject(response, http.StatusUnauthorized)
		return
	}
	select {
	case handler.slots <- struct{}{}:
		defer func() { <-handler.slots }()
	default:
		reject(response, http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, bodyBytesMax+1))
	if err != nil {
		reject(response, http.StatusBadRequest)
		return
	}
	if len(body) > bodyBytesMax {
		reject(response, http.StatusRequestEntityTooLarge)
		return
	}
	if !signature.Valid(handler.secret, request.Header.Get("X-Relay-Timestamp"), body,
		request.Header.Get("X-Relay-Signature")) {
		reject(response, http.StatusUnauthorized)
		return
	}
	status := scenarioStatus(body, request.Header.Get("X-Relay-Attempt"))
	// Only fixed classifications are logged; never bodies, signatures, keys or request identifiers.
	slog.Info("demo receiver authenticated request", "status", status)
	response.WriteHeader(status)
}

func (handler *Handler) validHeaders(request *http.Request) bool {
	for _, name := range [...]string{
		"X-Relay-Timestamp", "X-Relay-Signature", "X-Relay-Attempt", "X-Relay-Event-Id",
	} {
		if len(request.Header.Values(name)) != 1 {
			return false
		}
	}
	seconds, err := strconv.ParseInt(request.Header.Get("X-Relay-Timestamp"), 10, 64)
	if err != nil {
		return false
	}
	difference := handler.now().Sub(time.Unix(seconds, 0))
	if difference < -timestampWindow || difference > timestampWindow {
		return false
	}
	attempt, err := strconv.ParseUint(request.Header.Get("X-Relay-Attempt"), 10, 8)
	if err != nil || attempt < 1 || attempt > 8 ||
		strconv.FormatUint(attempt, 10) != request.Header.Get("X-Relay-Attempt") {
		return false
	}
	return identifier.ValidUUID(request.Header.Get("X-Relay-Event-Id"))
}

func scenarioStatus(body []byte, attempt string) int {
	var value struct {
		Scenario string `json:"scenario"`
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return http.StatusBadRequest
	}
	switch value.Scenario {
	case "success":
		return http.StatusNoContent
	case "temporary_failure":
		// Attempt is unsigned fixture metadata, not authority for a business operation.
		if attempt == "1" || attempt == "2" {
			return http.StatusServiceUnavailable
		}
		return http.StatusNoContent
	case "permanent_failure":
		return http.StatusUnprocessableEntity
	default:
		return http.StatusBadRequest
	}
}

func reject(response http.ResponseWriter, status int) {
	slog.Warn("demo receiver rejected request", "status", status)
	response.WriteHeader(status)
}
