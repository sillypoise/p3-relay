package notification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

const testURL = "https://sqs.us-east-1.amazonaws.com/123456789012/relay"
const testID = "5a9c38c7-e229-4dad-a702-b03780ba69a7"
const testBody = `{"version":1,"event_id":"` + testID + `"}`

type clientStub struct {
	sent                                 *sqs.SendMessageInput
	receive                              *sqs.ReceiveMessageInput
	deleted                              *sqs.DeleteMessageBatchInput
	messages                             []types.Message
	sendError, receiveError, deleteError error
	failedDelete                         bool
	attributes                           map[string]string
}

func (client *clientStub) SendMessage(ctx context.Context, input *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	client.sent = input
	return &sqs.SendMessageOutput{}, client.sendError
}
func (client *clientStub) ReceiveMessage(ctx context.Context, input *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	client.receive = input
	return &sqs.ReceiveMessageOutput{Messages: client.messages}, client.receiveError
}
func (client *clientStub) DeleteMessageBatch(ctx context.Context, input *sqs.DeleteMessageBatchInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
	client.deleted = input
	result := &sqs.DeleteMessageBatchOutput{}
	if client.failedDelete {
		result.Failed = []types.BatchResultErrorEntry{{Id: aws.String("0")}}
	} else {
		for _, entry := range input.Entries {
			result.Successful = append(result.Successful, types.DeleteMessageBatchResultEntry{Id: entry.Id})
		}
	}
	return result, client.deleteError
}
func (client *clientStub) GetQueueAttributes(context.Context, *sqs.GetQueueAttributesInput, ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	return &sqs.GetQueueAttributesOutput{Attributes: client.attributes}, nil
}

// Verify messages contain only the version and identifier, never credentials or webhook bodies.
func TestPublishBoundary(t *testing.T) {
	stub := &clientStub{}
	queue := New(stub, testURL)
	if err := queue.Publish(context.Background(), testID); err != nil {
		t.Fatal(err)
	}
	if *stub.sent.MessageBody != testBody || *stub.sent.QueueUrl != testURL {
		t.Fatal("incorrect notification envelope")
	}
	stub.sent = nil
	if err := queue.Publish(context.Background(), "malformed"); err == nil || stub.sent != nil {
		t.Fatal("invalid identifier reached SQS")
	}
	stub.sendError = errors.New("internal AWS credential detail")
	if err := queue.Publish(context.Background(), testID); err != ErrQueue {
		t.Fatal("SQS detail escaped bounded error")
	}
}

// Duplicates and ordering do not alter the envelope. Poison messages remain for redrive.
func TestReceiveAndDeleteBoundaries(t *testing.T) {
	stub := &clientStub{messages: []types.Message{
		{Body: aws.String(testBody), ReceiptHandle: aws.String("first")},
		{Body: aws.String(testBody), ReceiptHandle: aws.String("duplicate")},
		{Body: aws.String(`{"version":2,"event_id":"` + testID + `"}`), ReceiptHandle: aws.String("future")},
		{Body: aws.String(strings.Repeat("x", maximumBodyBytes+1)), ReceiptHandle: aws.String("oversize")},
		{Body: aws.String(testBody)},
	}}
	queue := New(stub, testURL)
	batch, err := queue.Receive(context.Background())
	if err != nil || batch.count != 2 {
		t.Fatalf("batch count %d: %v", batch.count, err)
	}
	if stub.receive.MaxNumberOfMessages != 10 || stub.receive.WaitTimeSeconds != 10 || stub.receive.VisibilityTimeout != 300 {
		t.Fatal("unbounded receive options")
	}
	if err = queue.Acknowledge(context.Background(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(stub.deleted.Entries) != 2 || *stub.deleted.Entries[1].ReceiptHandle != "duplicate" {
		t.Fatal("poison message acknowledged")
	}
	stub.failedDelete = true
	if err = queue.Acknowledge(context.Background(), &batch); err != ErrQueue {
		t.Fatal("partial deletion failure ignored")
	}
	stub.receiveError = errors.New("unavailable")
	if _, err = queue.Receive(context.Background()); err != ErrQueue {
		t.Fatal("receive failure ignored")
	}
	stub.receiveError = nil
	stub.messages = make([]types.Message, 11)
	if _, err = queue.Receive(context.Background()); err != ErrQueue {
		t.Fatal("oversized batch accepted")
	}
}

type runnerStub struct {
	calls     uint8
	remaining uint8
	err       error
}

func (runner *runnerStub) RunOnce(context.Context, time.Time) (bool, error) {
	runner.calls++
	if runner.err != nil {
		return false, runner.err
	}
	if runner.remaining == 0 {
		return false, nil
	}
	runner.remaining--
	return true, nil
}

func TestReconciliationSurvivesLostNotifications(t *testing.T) {
	for _, receiveError := range []error{nil, ErrQueue} {
		stub := &clientStub{receiveError: receiveError}
		runner := &runnerStub{remaining: 3}
		err := Cycle(context.Background(), runner, New(stub, testURL))
		if err != receiveError || runner.remaining != 0 || runner.calls != 4 {
			t.Fatalf("reconciliation failed: %v", err)
		}
	}
	runner := &runnerStub{remaining: 11}
	if err := Cycle(context.Background(), runner, nil); err != nil || runner.calls != 10 {
		t.Fatal("work batch bound violated")
	}
}

// A later event hint followed by an older one only triggers reconciliation of current state.
func TestOutOfOrderHintsAreOnlyWakeups(t *testing.T) {
	stub := &clientStub{messages: []types.Message{
		{Body: aws.String(strings.Replace(testBody, testID, "ffffffff-ffff-4fff-afff-ffffffffffff", 1)), ReceiptHandle: aws.String("later")},
		{Body: aws.String(testBody), ReceiptHandle: aws.String("older")},
	}}
	runner := &runnerStub{remaining: 0}
	if err := Cycle(context.Background(), runner, New(stub, testURL)); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 || stub.deleted == nil || len(stub.deleted.Entries) != 2 {
		t.Fatal("stale/out-of-order hints must not synthesize work")
	}
}

func TestWorkerFailurePreservesNotification(t *testing.T) {
	stub := &clientStub{messages: []types.Message{{Body: aws.String(testBody), ReceiptHandle: aws.String("handle")}}}
	runner := &runnerStub{err: errors.New("database unavailable")}
	if err := Cycle(context.Background(), runner, New(stub, testURL)); err == nil {
		t.Fatal("worker error lost")
	}
	if stub.deleted != nil {
		t.Fatal("notification deleted before durable processing")
	}
	runner.err = nil
	if err := Cycle(context.Background(), runner, New(stub, testURL)); err != nil {
		t.Fatal(err)
	}
	if stub.deleted == nil {
		t.Fatal("recovered/empty database did not acknowledge stale hint")
	}
}

func TestQueueConfigurationFailsClosed(t *testing.T) {
	if queue, err := Open(context.Background(), "", ""); queue != nil || err != nil {
		t.Fatal("local mode unavailable")
	}
	for _, url := range []string{"", strings.Replace(testURL, "https:", "http:", 1), testURL + ".fifo", testURL + "?token=secret", "https://evil.test/123456789012/relay"} {
		if validURL(url, "us-east-1") {
			t.Fatalf("invalid queue URL accepted: %s", url)
		}
	}
	attrs := map[string]string{"VisibilityTimeout": "300", "MaximumMessageSize": "1024",
		"ReceiveMessageWaitTimeSeconds": "10", "MessageRetentionPeriod": "86400", "SqsManagedSseEnabled": "true",
		"RedrivePolicy": `{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:123456789012:relay-dlq","maxReceiveCount":5}`}
	stub := &clientStub{attributes: attrs}
	queue := New(stub, testURL)
	if err := queue.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"VisibilityTimeout", "MaximumMessageSize", "ReceiveMessageWaitTimeSeconds", "MessageRetentionPeriod", "SqsManagedSseEnabled", "RedrivePolicy"} {
		value := attrs[name]
		delete(attrs, name)
		if err := queue.Check(context.Background()); err == nil {
			t.Fatalf("missing %s accepted", name)
		}
		attrs[name] = value
	}
	attrs["RedrivePolicy"] = `{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:123456789012:relay","maxReceiveCount":"5"}`
	if err := queue.Check(context.Background()); err == nil {
		t.Fatal("self-redrive accepted")
	}
}
