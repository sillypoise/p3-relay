package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sillypoise/p3-relay/internal/delivery"
	"github.com/sillypoise/p3-relay/internal/netguard"
	"github.com/sillypoise/p3-relay/internal/postgres"
)

const (
	lease_length  = 30 * time.Second
	poll_interval = time.Second
)

func main() {
	database_url := os.Getenv("RELAY_DATABASE_URL")
	delivery_secret := os.Getenv("RELAY_DELIVERY_SECRET")
	if database_url == "" || len(delivery_secret) < 16 {
		slog.Error("RELAY_DATABASE_URL and RELAY_DELIVERY_SECRET are required")
		os.Exit(1)
	}
	pool_configuration, error_value := pgxpool.ParseConfig(database_url)
	if error_value != nil {
		slog.Error("parse PostgreSQL configuration")
		os.Exit(1)
	}
	pool_configuration.MaxConns = 4
	pool_configuration.MinConns = 1
	pool, error_value := pgxpool.NewWithConfig(context.Background(), pool_configuration)
	if error_value != nil {
		slog.Error("create PostgreSQL pool")
		os.Exit(1)
	}
	defer pool.Close()

	allow_private := os.Getenv("RELAY_ALLOW_PRIVATE_DESTINATIONS") == "true"
	worker := delivery.NewWorker(
		postgres.NewStore(pool),
		delivery.NewHTTPSender(new_http_client(allow_private), []byte(delivery_secret), allow_private),
		lease_length,
	)
	ticker := time.NewTicker(poll_interval)
	defer ticker.Stop()
	for now := range ticker.C {
		processed, run_error := worker.RunOnce(context.Background(), now)
		if run_error != nil {
			slog.Error("delivery cycle failed", "error", run_error)
			continue
		}
		if processed {
			slog.Info("delivery cycle completed")
		}
	}
}

func new_http_client(allow_private bool) *http.Client {
	dialer := netguard.NewDialer(allow_private, 3*time.Second)
	transport := &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext,
		ForceAttemptHTTP2: true, MaxIdleConns: 16, MaxIdleConnsPerHost: 4,
		IdleConnTimeout: 60 * time.Second, TLSHandshakeTimeout: 3 * time.Second,
		ResponseHeaderTimeout: 7 * time.Second, ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
