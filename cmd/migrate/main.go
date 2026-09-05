package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"os"
	"time"
)

func main() {
	if err := migrate(); err != nil {
		slog.Error("migration failed")
		os.Exit(1)
	}
}

func migrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if os.Getenv("RELAY_DATABASE_URL") == "" {
		return fmt.Errorf("database URL required")
	}
	conn, err := pgx.Connect(ctx, os.Getenv("RELAY_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			slog.Error("migration connection close failed")
		}
	}()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			slog.Error("migration rollback failed")
		}
	}()
	// Serialize migrations without touching another project's schemas.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(330052)"); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT to_regclass('p3_relay.schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return err
	}
	var version int32
	if exists {
		if err = tx.QueryRow(ctx, "SELECT COALESCE(max(version),0) FROM p3_relay.schema_migrations").Scan(&version); err != nil {
			return err
		}
	}
	paths := [...]string{"migrations/001_initial.sql", "migrations/002_sandbox.sql"}
	if version > int32(len(paths)) {
		return fmt.Errorf("unsupported migration version")
	}
	for index, path := range paths {
		if int32(index) < version {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(content)); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	slog.Info("migrations applied", "version", len(paths))
	return nil
}
