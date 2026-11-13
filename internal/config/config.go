package config

import (
	"fmt"
	"os"
	"strconv"
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

	// GeoIPDBPath points at a MaxMind-format country database. Empty disables
	// country lookup rather than failing - local dev should not need a 4 MB blob.
	GeoIPDBPath string

	// Ingest pipeline sizing; see internal/batch.
	IngestBufferSize    int
	IngestBatchSize     int
	IngestFlushInterval time.Duration

	// RollupSecret guards POST /internal/rollup. Empty disables the endpoint;
	// the in-process ticker runs either way.
	RollupSecret   string
	RollupInterval time.Duration

	// TrustedProxyHops is how many reverse proxies sit in front of the API
	// (1 on Render, Fly and similar). 0 ignores X-Forwarded-For entirely.
	TrustedProxyHops int
	// MetricsToken guards GET /metrics. Empty disables the endpoint.
	MetricsToken string

	// Token-bucket limits on POST /api/event, per client IP and per site.
	IngestIPRate    float64
	IngestIPBurst   float64
	IngestSiteRate  float64
	IngestSiteBurst float64
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
		GeoIPDBPath:    os.Getenv("GEOIP_DB_PATH"),

		IngestBufferSize:    envOrInt("INGEST_BUFFER_SIZE", 10_000),
		IngestBatchSize:     envOrInt("INGEST_BATCH_SIZE", 500),
		IngestFlushInterval: envOrDuration("INGEST_FLUSH_INTERVAL", 2*time.Second),

		RollupSecret:   os.Getenv("ROLLUP_SECRET"),
		RollupInterval: envOrDuration("ROLLUP_INTERVAL", 15*time.Minute),

		TrustedProxyHops: envOrIntZero("TRUSTED_PROXY_HOPS", 0),
		MetricsToken:     os.Getenv("METRICS_TOKEN"),

		IngestIPRate:    envOrFloat("INGEST_IP_RATE", 10),
		IngestIPBurst:   envOrFloat("INGEST_IP_BURST", 50),
		IngestSiteRate:  envOrFloat("INGEST_SITE_RATE", 200),
		IngestSiteBurst: envOrFloat("INGEST_SITE_BURST", 1000),
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

func envOrInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return def
		}
		return n
	}
	return def
}

// envOrIntZero is envOrInt for settings where 0 is meaningful.
func envOrIntZero(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return def
		}
		return n
	}
	return def
}

func envOrFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			return def
		}
		return f
	}
	return def
}
