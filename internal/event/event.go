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
}

type ConflictError struct{}

func (ConflictError) Error() string {
	return "event identifier conflicts with an existing body"
}
