package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

const migration_timeout = 30 * time.Second

func main() {
	database_url := os.Getenv("RELAY_DATABASE_URL")
	if database_url == "" {
		slog.Error("RELAY_DATABASE_URL is required")
		os.Exit(1)
	}

	migration, error_value := os.ReadFile("migrations/001_initial.sql")
	if error_value != nil {
		slog.Error("read migration", "error", error_value)
		os.Exit(1)
	}

	context_value, cancel := context.WithTimeout(context.Background(), migration_timeout)
	defer cancel()

	connection, error_value := pgx.Connect(context_value, database_url)
	if error_value != nil {
		slog.Error("connect to PostgreSQL", "error", error_value)
		os.Exit(1)
	}
	defer func() {
		if close_error := connection.Close(context.Background()); close_error != nil {
			slog.Error("close PostgreSQL connection", "error", close_error)
		}
	}()

	applied, error_value := migration_applied(context_value, connection)
	if error_value != nil {
		slog.Error("check migration version", "error", error_value)
		os.Exit(1)
	}
	if applied {
		slog.Info("database migration already applied", "version", 1)
		return
	}

	if _, error_value = connection.Exec(context_value, string(migration)); error_value != nil {
		slog.Error("apply database migration", "error", error_value)
		os.Exit(1)
	}
	slog.Info("database migration applied", "version", 1)
}

func migration_applied(context_value context.Context, connection *pgx.Conn) (bool, error) {
	const relation_query = `SELECT to_regclass('p3_relay.schema_migrations') IS NOT NULL`
	var relation_exists bool
	error_value := connection.QueryRow(context_value, relation_query).Scan(&relation_exists)
	if error_value != nil {
		return false, error_value
	}
	if !relation_exists {
		return false, nil
	}

	const version_query = `SELECT EXISTS (
        SELECT FROM p3_relay.schema_migrations WHERE version = 1
    )`
	var applied bool
	error_value = connection.QueryRow(context_value, version_query).Scan(&applied)
	return applied, error_value
}
