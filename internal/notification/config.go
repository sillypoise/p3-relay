package notification

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

var regionPattern = regexp.MustCompile(`^[a-z]{2}-[a-z]+-[1-9]$`)
var pathPattern = regexp.MustCompile(`^/[0-9]{12}/[A-Za-z0-9_-]{1,80}$`)

// Open uses the AWS credential chain (ECS task roles in deployment), not project-stored keys.
// Empty URL and region deliberately select local polling. Partial configuration is an error.
func Open(ctx context.Context, queueURL string, region string) (*Queue, error) {
	if queueURL == "" && region == "" {
		return nil, nil
	}
	if validURL(queueURL, region) == false {
		return nil, ErrQueue
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext, TLSHandshakeTimeout: 3 * time.Second,
		ResponseHeaderTimeout: 12 * time.Second, IdleConnTimeout: 60 * time.Second,
		MaxIdleConns: 4, MaxIdleConnsPerHost: 4, MaxConnsPerHost: 4,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region),
		config.WithHTTPClient(client), config.WithRetryMaxAttempts(1))
	if err != nil {
		return nil, ErrQueue
	}
	sdk := sqs.NewFromConfig(cfg, func(options *sqs.Options) {
		// Pin the regional endpoint so ambient endpoint overrides cannot receive credentials.
		options.BaseEndpoint = aws.String("https://sqs." + region + ".amazonaws.com")
		options.ClientLogMode = 0
		options.DisableMessageChecksumValidation = false
	})
	queue := New(sdk, queueURL)
	if err = queue.Check(ctx); err != nil {
		return nil, err
	}
	return queue, nil
}

func validURL(raw string, region string) bool {
	if regionPattern.MatchString(region) == false {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" && parsed.Host == "sqs."+region+".amazonaws.com" &&
		parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" &&
		parsed.RawPath == "" && pathPattern.MatchString(parsed.Path)
}

// Check verifies bounded transport retries and encryption before enabling the configured queue.
func (queue *Queue) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := queue.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(queue.url), AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameAll},
	})
	if err != nil || output == nil {
		return ErrQueue
	}
	attrs := output.Attributes
	if attrs["VisibilityTimeout"] != "300" || attrs["MaximumMessageSize"] != "1024" {
		return ErrQueue
	}
	if attrs["ReceiveMessageWaitTimeSeconds"] != "10" || attrs["MessageRetentionPeriod"] != "86400" {
		return ErrQueue
	}
	if attrs["SqsManagedSseEnabled"] != "true" || attrs["FifoQueue"] == "true" {
		return ErrQueue
	}
	var redrive struct {
		Target string      `json:"deadLetterTargetArn"`
		Count  json.Number `json:"maxReceiveCount"`
	}
	if json.Unmarshal([]byte(attrs["RedrivePolicy"]), &redrive) != nil {
		return ErrQueue
	}
	if redrive.Count.String() != "5" {
		return ErrQueue
	}
	// The DLQ must be a distinct queue in the same account and region.
	parsed, err := url.Parse(queue.url)
	if err != nil {
		return ErrQueue
	}
	path := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if len(path) != 2 {
		return ErrQueue
	}
	region := strings.Split(parsed.Host, ".")[1]
	prefix := "arn:aws:sqs:" + region + ":" + path[0] + ":"
	if strings.HasPrefix(redrive.Target, prefix) == false {
		return ErrQueue
	}
	name := strings.TrimPrefix(redrive.Target, prefix)
	if name == path[1] || pathPattern.MatchString("/"+path[0]+"/"+name) == false {
		return ErrQueue
	}
	return nil
}
