package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Diptanshv/kestrel/internal/config"
	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const defaultDatabaseURL = "postgres://pulse:pulse@127.0.0.1:5433/pulse?sslmode=disable"

// TestConfig returns API config suitable for httptest integration tests.
func TestConfig() config.Config {
	return config.Config{
		SessionSecret: "integration-test-session-secret",
		CookieSecure:  false,
		SessionTTL:    24 * time.Hour,
		Timezone:      "Asia/Kolkata",
	}
}

// Setup returns a migrated database handle and a cleanup function.
// Uses TEST_DATABASE_URL when set, otherwise the local docker-compose default.
func Setup() (*sql.DB, func(), error) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = defaultDatabaseURL
	}

	if err := migrateUp(dsn); err != nil {
		return nil, nil, fmt.Errorf("migrate up: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("sql open: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("db ping: %w", err)
	}

	return db, func() { _ = db.Close() }, nil
}

// OpenPostgres is Setup that terminates the test on failure.
func OpenPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	db, cleanup, err := Setup()
	if err != nil {
		t.Fatal(err)
	}
	return db, cleanup
}

// Reset clears application tables between tests.
func Reset(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, "TRUNCATE users RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatalf("truncate users: %v", err)
	}
}

func migrateUp(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		return err
	}

	migrationsPath, err := migrationsDir()
	if err != nil {
		return err
	}
	sourceURL := fmt.Sprintf("file://%s", migrationsPath)

	m, err := migrate.NewWithDatabaseInstance(sourceURL, "postgres", driver)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func migrationsDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "migrations"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %q", wd)
		}
		dir = parent
	}
}
