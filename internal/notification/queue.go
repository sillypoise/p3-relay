// Package notification treats SQS messages as hints, never as delivery state or authority.
package notification

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/sillypoise/p3-relay/internal/identifier"
)

const (
	BatchSize               = 10
	VisibilitySeconds int32 = 300
	WaitSeconds       int32 = 10
	maximumBodyBytes        = 1024
)

var ErrQueue = errors.New("notification queue unavailable")

// Client is the small AWS SDK seam used for deterministic failure tests.
type Client interface {
	SendMessage(context.Context, *sqs.SendMessageInput, ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	ReceiveMessage(context.Context, *sqs.ReceiveMessageInput, ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessageBatch(context.Context, *sqs.DeleteMessageBatchInput, ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error)
	GetQueueAttributes(context.Context, *sqs.GetQueueAttributesInput, ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

type Queue struct {
	client Client
	url    string
}
type Batch struct {
	handles [BatchSize]string
	count   uint8
}
type message struct {
	Version uint8  `json:"version"`
	EventID string `json:"event_id"`
}

func New(client Client, queueURL string) *Queue {
	if client == nil {
		panic("notification client required")
	}
	if queueURL == "" {
		panic("notification queue URL required")
	}
	return &Queue{client: client, url: queueURL}
}

// Publish runs only after PostgreSQL commit. A failed hint never rolls back durable acceptance.
func (queue *Queue) Publish(ctx context.Context, eventID string) error {
	if identifier.ValidUUID(eventID) == false {
		return ErrQueue
	}
	body, err := json.Marshal(message{Version: 1, EventID: eventID})
	if err != nil {
		return ErrQueue
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, err = queue.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl: aws.String(queue.url), MessageBody: aws.String(string(body)), DelaySeconds: 0,
	})
	if err != nil {
		return ErrQueue
	}
	return nil
}

func (queue *Queue) Receive(ctx context.Context) (Batch, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	result, err := queue.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(queue.url), MaxNumberOfMessages: BatchSize,
		VisibilityTimeout: VisibilitySeconds, WaitTimeSeconds: WaitSeconds,
	})
	if err != nil || result == nil {
		return Batch{}, ErrQueue
	}
	if len(result.Messages) > BatchSize {
		return Batch{}, ErrQueue
	}
	var batch Batch
	for _, item := range result.Messages {
		if validMessage(item) == false {
			// Poison messages are left for SQS redrive; never log their body or receipt handle.
			slog.Warn("notification rejected", "code", "invalid_message")
			continue
		}
		batch.handles[batch.count] = *item.ReceiptHandle
		batch.count++
	}
	return batch, nil
}

func (queue *Queue) Acknowledge(ctx context.Context, batch *Batch) error {
	if batch == nil {
		panic("notification batch required")
	}
	if batch.count > BatchSize {
		panic("notification batch exceeds bound")
	}
	if batch.count == 0 {
		return nil
	}
	var entries [BatchSize]types.DeleteMessageBatchRequestEntry
	for index := uint8(0); index < batch.count; index++ {
		entries[index] = types.DeleteMessageBatchRequestEntry{
			Id: aws.String(strconv.FormatUint(uint64(index), 10)), ReceiptHandle: &batch.handles[index]}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := queue.client.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{
		QueueUrl: aws.String(queue.url), Entries: entries[:batch.count],
	})
	if err != nil || result == nil {
		return ErrQueue
	}
	if len(result.Failed) > 0 || len(result.Successful) != int(batch.count) {
		return ErrQueue
	}
	return nil
}

func validMessage(item types.Message) bool {
	if item.Body == nil || item.ReceiptHandle == nil {
		return false
	}
	if len(*item.Body) > maximumBodyBytes || len(*item.ReceiptHandle) == 0 {
		return false
	}
	if len(*item.ReceiptHandle) > 2048 {
		return false
	}
	var value message
	if err := json.Unmarshal([]byte(*item.Body), &value); err != nil {
		return false
	}
	return value.Version == 1 && identifier.ValidUUID(value.EventID)
}
