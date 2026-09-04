package delivery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sillypoise/p3-relay/internal/identifier"
	"github.com/sillypoise/p3-relay/internal/signature"
)

const maximum_response_bytes = 4096

type HTTPSender struct {
	client *http.Client
	secret []byte
	now                 func() time.Time
	allow_insecure_http bool
}

func NewHTTPSender(client *http.Client, secret []byte, allow_insecure_http bool) *HTTPSender {
	if client == nil {
		panic("HTTP client is required")
	}
	if len(secret) < 16 {
		panic("delivery secret must contain at least 16 bytes")
	}
	return &HTTPSender{
		client: client, secret: secret, now: time.Now,
		allow_insecure_http: allow_insecure_http,
	}
}

func (sender *HTTPSender) Send(context_value context.Context, claimed *ClaimedEvent) *Attempt {
	if claimed == nil {
		panic("claimed event is required")
	}
	started_at := sender.now()
	attempt_id, error_value := identifier.NewUUID()
	if error_value != nil {
		return network_attempt("identifier_unavailable", started_at, sender.now(), claimed)
	}
	request, error_value := http.NewRequestWithContext(
		context_value,
		http.MethodPost,
		claimed.DestinationURL,
		bytes.NewReader(claimed.Body),
	)
	if error_value != nil {
		return network_attempt("invalid_destination", started_at, sender.now(), claimed)
	}
	if request.URL.Scheme != "https" && !sender.allow_insecure_http {
		return network_attempt("insecure_destination", started_at, sender.now(), claimed)
	}
	timestamp := strconv.FormatInt(started_at.Unix(), 10)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Relay/1")
	request.Header.Set("X-Relay-Event-Id", claimed.ID)
	request.Header.Set("X-Relay-Attempt", strconv.FormatUint(uint64(claimed.AttemptNumber), 10))
	request.Header.Set("X-Relay-Timestamp", timestamp)
	request.Header.Set("X-Relay-Signature", signature.Create(sender.secret, timestamp, claimed.Body))

	response, error_value := sender.client.Do(request)
	if error_value != nil {
		return network_attempt("request_failed", started_at, sender.now(), claimed)
	}
	defer response.Body.Close()
	excerpt, read_error := io.ReadAll(io.LimitReader(response.Body, maximum_response_bytes))
	if read_error != nil {
		return network_attempt("response_read_failed", started_at, sender.now(), claimed)
	}
	status_code := uint16(response.StatusCode)
	outcome := HTTPOutcome(status_code)
	return &Attempt{
		ID: attempt_id, StartedAt: started_at, FinishedAt: sender.now(), Outcome: outcome,
		StatusCode: &status_code, ResponseExcerpt: excerpt,
		NextAttemptAt: next_attempt_at(claimed, sender.now(), outcome),
	}
}

func network_attempt(code string, started_at time.Time, finished_at time.Time, claimed *ClaimedEvent) *Attempt {
	attempt_id, error_value := identifier.NewUUID()
	if error_value != nil {
		attempt_id = claimed.ClaimID
	}
	return &Attempt{
		ID: attempt_id, StartedAt: started_at, FinishedAt: finished_at, Outcome: "network_error",
		ResponseExcerpt: []byte{}, ErrorCode: &code,
		NextAttemptAt: next_attempt_at(claimed, finished_at, "network_error"),
	}
}

func next_attempt_at(claimed *ClaimedEvent, now time.Time, outcome string) time.Time {
	if outcome == "delivered" || outcome == "terminal_http" {
		return now
	}
	delays := [...]time.Duration{0, 5 * time.Second, 30 * time.Second, 2 * time.Minute,
		10 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}
	index := int(claimed.AttemptNumber)
	if index >= len(delays) {
		index = len(delays) - 1
	}
	delay := delays[index]
	digest := sha256.Sum256([]byte(claimed.ID + ":" + strconv.Itoa(index)))
	per_mille := int64(binary.BigEndian.Uint16(digest[:2])%401) - 200
	offset := time.Duration((int64(delay) * per_mille) / 1000)
	return now.Add(delay + offset)
}
