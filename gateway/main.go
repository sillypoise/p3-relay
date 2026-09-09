package main

import (
	_ "embed"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

//go:embed pgbouncer.ini
var poolerConfiguration string

func main() {
	if err := launch(); err != nil {
		// Error messages contain only fixed configuration classifications, never input values.
		slog.Error("gateway startup rejected", "reason", err.Error())
		os.Exit(1)
	}
}

func launch() error {
	value := configuration{
		certificate:       os.Getenv("GATEWAY_CERTIFICATE"),
		privateKey:        os.Getenv("GATEWAY_PRIVATE_KEY"),
		backendCA:         os.Getenv("GATEWAY_BACKEND_CA"),
		hostname:          os.Getenv("GATEWAY_HOSTNAME"),
		runtimePassword:   os.Getenv("GATEWAY_RUNTIME_PASSWORD"),
		migrationPassword: os.Getenv("GATEWAY_MIGRATION_PASSWORD"),
	}
	if err := value.validate(time.Now()); err != nil {
		return err
	}

	// One launch per container. The private tmpfs directory is owned by the container lifecycle.
	directory, err := prepareFiles("/dev/shm", &value)
	if err != nil {
		return err
	}
	defer func() {
		// Successful exec replaces this process; only failure returns to this cleanup.
		if err := os.RemoveAll(directory); err != nil {
			slog.Error("gateway startup cleanup failed")
		}
	}()

	if err := os.Chdir(directory); err != nil {
		return errors.New("runtime directory unavailable")
	}
	// Keep keys out of the long-lived process environment. PgBouncer reads private tmpfs files.
	if err := syscall.Exec("/usr/bin/pgbouncer", []string{
		"pgbouncer", "pgbouncer.ini",
	}, []string{"PATH=/usr/bin:/bin"}); err != nil {
		return errors.New("pooler executable unavailable")
	}
	panic("successful exec must not return")
}

func prepareFiles(parent string, value *configuration) (directory string, result error) {
	if value == nil {
		panic("validated gateway configuration required")
	}

	directory, err := os.MkdirTemp(parent, "p3-relay-gateway-")
	if err != nil {
		return "", errors.New("private runtime directory creation failed")
	}
	defer func() {
		if result != nil {
			if err := os.RemoveAll(directory); err != nil {
				result = errors.New("private runtime directory cleanup failed")
			}
		}
	}()

	files := [...]struct{ name, content string }{
		{"pgbouncer.ini", poolerConfiguration},
		{"server.crt", value.certificate},
		{"server.key", value.privateKey},
		{"backend.crt", value.backendCA},
		{"users.txt", "\"p3_relay_runtime\" \"" + value.runtimePassword + "\"\n" +
			"\"p3_relay_migrator\" \"" + value.migrationPassword + "\"\n"},
	}
	for _, file := range files {
		path := filepath.Join(directory, file.name)
		if err := os.WriteFile(path, []byte(file.content), 0o600); err != nil {
			return directory, errors.New("private runtime file creation failed")
		}
	}
	return directory, nil
}
