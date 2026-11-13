package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Diptanshv/kestrel/internal/batch"
	"github.com/Diptanshv/kestrel/internal/config"
	"github.com/Diptanshv/kestrel/internal/enrich"
	"github.com/Diptanshv/kestrel/internal/httpapi"
	"github.com/Diptanshv/kestrel/internal/store"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver for database/sql
)

const shutdownTimeout = 10 * time.Second

func main() {
	cfg, err := config.Load() // loading the configurations from the config package that we created in other file config.go
	if err != nil {
		log.Fatal(err)
	}

	logger := slog.New( // creating a new logger
		slog.NewJSONHandler( // creating a new json handler that formats the logs in json format
			os.Stdout, // writing the logs to the standard output
			&slog.HandlerOptions{ // setting the handler options of the logger
				Level: slog.LevelInfo, // setting the level of the logger to info
			},
		),
	)
	slog.SetDefault(logger) // setting the default logger to the logger we created

	if err := runMigrations(cfg.DatabaseURL, cfg.MigrationsPath); err != nil { // running the database migrations using the config we got above
		logger.Error("migrations failed", "err", err)
		os.Exit(1)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL) // Creating a database handle using the pgx PostgreSQL driver and this connection string
	if err != nil {
		logger.Error("db open failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }() // closing the database connection when the main function is done (It essentially is like closing the connection after the main function is done)

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second) // creating a new context with a timeout of 5 seconds
	defer pingCancel()                                                              // canceling the context when the main context is done
	if err := db.PingContext(pingCtx); err != nil {
		logger.Error("db ping failed", "err", err)
		os.Exit(1)
	}
	logger.Info("connected to database")

	queries := store.New(db)

	geo, err := enrich.OpenGeo(cfg.GeoIPDBPath)
	if err != nil {
		logger.Error("geoip open failed", "path", cfg.GeoIPDBPath, "err", err)
		os.Exit(1)
	}
	defer func() { _ = geo.Close() }()
	if cfg.GeoIPDBPath == "" {
		logger.Warn("geoip disabled; country will be empty", "hint", "set GEOIP_DB_PATH")
	}

	enricher := enrich.New(enrich.NewSaltStore(queries), geo)

	batcher := batch.New(batch.CopyWriter{DB: db}, logger, batch.Config{
		BufferSize:    cfg.IngestBufferSize,
		BatchSize:     cfg.IngestBatchSize,
		FlushInterval: cfg.IngestFlushInterval,
	})
	batcher.Start()

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Log:     logger,
			Queries: queries,
			Config:  cfg,
			Enrich:  enricher,
			Sink:    batcher,
		}),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("http listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server failed", "err", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("shutdown signal received", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	// Order matters: stop the HTTP server first so no handler is still
	// enqueueing, then drain the buffer into Postgres.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
	if err := batcher.Close(shutdownCtx); err != nil {
		logger.Error("batcher drain failed", "err", err, "pending", batcher.Depth())
		os.Exit(1)
	}
	logger.Info("server stopped")
}

func runMigrations(databaseURL, migrationsPath string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}

	sourceURL := fmt.Sprintf("file://%s", migrationsPath)
	if _, err := os.Stat(migrationsPath); err != nil {
		return fmt.Errorf("migrations path %q: %w", migrationsPath, err)
	}

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
