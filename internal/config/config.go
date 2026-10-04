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

	// Timezone buckets the stats queries and labels the dashboard. Hourly and
	// daily buckets both follow it, so a "day" is a local day rather than a
	// UTC one. IANA name; "UTC" restores the original behaviour.
	Timezone   string
	EnableDemo bool
}

func Load() (Config, error) {
	c := Config{
		Port:           envOr("PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		MigrationsPath: envOr("MIGRATIONS_PATH", "migrations"),
		SessionSecret:  os.Getenv("SESSION_SECRET"),
		CookieSecure:   envOr("COOKIE_SECURE", "false") == "true",
		SessionTTL:     envOrDuration("SESSION_TTL", 30*24*time.Hour),
		Timezone:       envOr("TIMEZONE", "Asia/Kolkata"),
		EnableDemo:     envOr("ENABLE_DEMO", "false") == "true",
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
