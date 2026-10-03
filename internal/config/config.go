package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Port           string
	DatabaseURL    string
	MigrationsPath string

	SessionSecret string // HMAC secret for session token hashing
	CookieSecure  bool   // true in prod (HTTPS)
	SessionTTL    time.Duration
}

func Load() (Config, error) {
	c := Config{
		Port:           envOr("PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		MigrationsPath: envOr("MIGRATIONS_PATH", "migrations"),
		SessionSecret:  os.Getenv("SESSION_SECRET"),
		CookieSecure:   envOr("COOKIE_SECURE", "false") == "true",
		SessionTTL:     envOrDuration("SESSION_TTL", 30*24*time.Hour),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}

	if c.SessionSecret == "" {
		return c, fmt.Errorf("SESSION_SECRET is required")
	}

	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return def
		}
		return d
	}
	return def
}
