package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sillypoise/p3-relay/internal/httpapi"
	"github.com/sillypoise/p3-relay/internal/postgres"
)

const (
	header_timeout  = 5 * time.Second
	idle_timeout    = 60 * time.Second
	request_timeout = 15 * time.Second
)

func main() {
	configuration := load_configuration()
	pool_configuration, error_value := pgxpool.ParseConfig(configuration.database_url)
	if error_value != nil {
		slog.Error("parse PostgreSQL configuration")
		os.Exit(1)
	}
	pool_configuration.MaxConns = 8
	pool_configuration.MinConns = 1
	pool_configuration.MaxConnLifetime = 30 * time.Minute
	pool_configuration.MaxConnIdleTime = 5 * time.Minute

	pool, error_value := pgxpool.NewWithConfig(context.Background(), pool_configuration)
	if error_value != nil {
		slog.Error("create PostgreSQL pool")
		os.Exit(1)
	}
	defer pool.Close()

	store := postgres.NewStore(pool)
	event_api := httpapi.New(
		store,
		configuration.source_key,
		[]byte(configuration.ingress_secret),
		configuration.destination_url,
	)
	server := new_server(configuration.address, new_handler(event_api.Handler()))

	slog.Info("starting Relay API", "address", configuration.address)
	error_value = server.ListenAndServe()
	if errors.Is(error_value, http.ErrServerClosed) {
		return
	}
	if error_value != nil {
		slog.Error("Relay API stopped", "error", error_value)
		os.Exit(1)
	}
}

type configuration struct {
	address         string
	database_url    string
	source_key      string
	ingress_secret  string
	destination_url string
}

func load_configuration() configuration {
	value := configuration{
		address:         os.Getenv("RELAY_HTTP_ADDRESS"),
		database_url:    os.Getenv("RELAY_DATABASE_URL"),
		source_key:      os.Getenv("RELAY_SOURCE_KEY"),
		ingress_secret:  os.Getenv("RELAY_INGRESS_SECRET"),
		destination_url: os.Getenv("RELAY_DESTINATION_URL"),
	}
	if value.address == "" {
		value.address = ":8080"
	}
	if value.database_url == "" || value.source_key == "" {
		slog.Error("RELAY_DATABASE_URL and RELAY_SOURCE_KEY are required")
		os.Exit(1)
	}
	if len(value.ingress_secret) < 16 || value.destination_url == "" {
		slog.Error("RELAY_INGRESS_SECRET and RELAY_DESTINATION_URL are required")
		os.Exit(1)
	}
	return value
}

func new_server(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: address, Handler: handler, ReadHeaderTimeout: header_timeout,
		ReadTimeout: request_timeout, WriteTimeout: request_timeout,
		IdleTimeout: idle_timeout, MaxHeaderBytes: 16 * 1024,
	}
}

func new_handler(events_handler http.Handler) http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /health", health_get)
	handler.Handle("/v1/", events_handler)
	return handler
}

func health_get(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_, error_value := response.Write([]byte("{\"status\":\"ok\"}\n"))
	if error_value != nil {
		slog.Warn("health response write failed", "error", error_value)
	}
}
