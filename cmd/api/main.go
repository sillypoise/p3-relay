package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"
)

const (
	header_timeout  = 5 * time.Second
	idle_timeout    = 60 * time.Second
	request_timeout = 15 * time.Second
)

func main() {
	address := os.Getenv("RELAY_HTTP_ADDRESS")
	if address == "" {
		address = ":8080"
	}

	server := &http.Server{
		Addr:              address,
		Handler:           new_handler(),
		ReadHeaderTimeout: header_timeout,
		ReadTimeout:       request_timeout,
		WriteTimeout:      request_timeout,
		IdleTimeout:       idle_timeout,
		MaxHeaderBytes:    16 * 1024,
	}

	slog.Info("starting Relay API", "address", address)
	error_value := server.ListenAndServe()
	if errors.Is(error_value, http.ErrServerClosed) {
		return
	}
	if error_value != nil {
		slog.Error("Relay API stopped", "error", error_value)
		os.Exit(1)
	}
}

func new_handler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /health", health_get)
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
