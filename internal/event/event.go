package event

import (
	"context"
	"time"
)

type Receipt struct {
	ID              string
	SourceKey       string
	ExternalEventID string
	Body            []byte
	BodySHA256      []byte
	DestinationURL  string
}

type Accepted struct {
	ID        string
	Duplicate bool
}

type Overview struct {
	Pending      uint32 `json:"pending"`
	Delivering   uint32 `json:"delivering"`
	Delivered    uint32 `json:"delivered"`
	Retrying     uint32 `json:"retrying"`
	DeadLettered uint32 `json:"dead_lettered"`
}

type Summary struct {
	ID              string    `json:"id"`
	ExternalEventID string    `json:"external_event_id"`
	State           string    `json:"state"`
	AttemptCount    uint8     `json:"attempt_count"`
	CreatedAt       time.Time `json:"created_at"`
}

type AttemptView struct {
	ReplayNumber   uint8     `json:"replay_number"`
	AttemptNumber  uint8     `json:"attempt_number"`
	Outcome        string    `json:"outcome"`
	StatusCode     *uint16   `json:"status_code"`
	ErrorCode      *string   `json:"error_code"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`
	ResponseExcerpt string   `json:"response_excerpt"`
}

type Detail struct {
	Summary
	DestinationURL string        `json:"destination_url"`
	ReplayCount    uint8         `json:"replay_count"`
	Attempts       []AttemptView `json:"attempts"`
}

type Store interface {
	Accept(context.Context, *Receipt) (Accepted, error)
	Replay(context.Context, string, string) error
	Overview(context.Context, string) (Overview, error)
	List(context.Context, string, string, uint16) ([]Summary, error)
	Detail(context.Context, string, string) (Detail, error)
}

type ConflictError struct{}

func (ConflictError) Error() string {
	return "event identifier conflicts with an existing body"
}

type NotFoundError struct{}

func (NotFoundError) Error() string { return "event was not found" }

type InvalidStateError struct{}

func (InvalidStateError) Error() string { return "event is not dead-lettered" }
