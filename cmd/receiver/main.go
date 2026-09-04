package main

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /events", events_post)
	server := &http.Server{
		Addr: ":9090", Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024,
	}
	slog.Info("starting simulated receiver", "address", server.Addr)
	error_value := server.ListenAndServe()
	if errors.Is(error_value, http.ErrServerClosed) {
		return
	}
	if error_value != nil {
		slog.Error("simulated receiver stopped", "error", error_value)
		os.Exit(1)
	}
}

func events_post(response http.ResponseWriter, request *http.Request) {
	body, error_value := io.ReadAll(io.LimitReader(request.Body, 256*1024+1))
	if error_value != nil || len(body) > 256*1024 {
		http.Error(response, "invalid body", http.StatusBadRequest)
		return
	}
	attempt, error_value := strconv.ParseUint(request.Header.Get("X-Relay-Attempt"), 10, 8)
	if error_value != nil {
		http.Error(response, "invalid attempt", http.StatusBadRequest)
		return
	}
	scenario := request.URL.Query().Get("scenario")
	if scenario == "temporary_failure" && attempt < 3 {
		http.Error(response, "simulated temporary failure", http.StatusServiceUnavailable)
		return
	}
	if scenario == "permanent_failure" {
		http.Error(response, "simulated permanent failure", http.StatusUnprocessableEntity)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}
