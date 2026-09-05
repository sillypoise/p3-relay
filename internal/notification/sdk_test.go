package notification

import (
	"context"
	"crypto/md5" // SQS's response checksum protocol uses MD5; this is not an authentication primitive.
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Exercise the real SDK's serialization and signing against a local server, never live AWS.
func TestSDKPublishesMinimalEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("SDK did not sign request")
		}
		if r.Header.Get("X-Amz-Target") != "AmazonSQS.SendMessage" {
			t.Error("unexpected SQS action")
		}
		var input struct {
			MessageBody  string
			QueueUrl     string
			DelaySeconds int32
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if input.MessageBody != testBody || input.DelaySeconds != 0 {
			t.Error("unexpected notification body")
		}
		checksum := md5.Sum([]byte(input.MessageBody))
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		if err := json.NewEncoder(w).Encode(map[string]string{"MessageId": testID, "MD5OfMessageBody": hex.EncodeToString(checksum[:])}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	sdk := sqs.New(sqs.Options{Region: "us-east-1", BaseEndpoint: aws.String(server.URL),
		HTTPClient: server.Client(), RetryMaxAttempts: 1, Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "test-key", SecretAccessKey: "synthetic-test-secret", Source: "test"}, nil
		})})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := New(sdk, server.URL+"/123456789012/relay").Publish(ctx, testID); err != nil {
		t.Fatal(err)
	}
}
