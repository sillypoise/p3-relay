package event

import "context"

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

type Store interface {
	Accept(context.Context, *Receipt) (Accepted, error)
	Replay(context.Context, string, string) error
}

type ConflictError struct{}

func (ConflictError) Error() string {
	return "event identifier conflicts with an existing body"
}

type NotFoundError struct{}

func (NotFoundError) Error() string { return "event was not found" }

type InvalidStateError struct{}

func (InvalidStateError) Error() string { return "event is not dead-lettered" }
