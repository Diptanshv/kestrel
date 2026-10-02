package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port            string
	DatabaseURL     string
	MigrationsPath  string
}

func Load() (Config, error) {
	c := Config{
		Port:           envOr("PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		MigrationsPath: envOr("MIGRATIONS_PATH", "migrations"),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}