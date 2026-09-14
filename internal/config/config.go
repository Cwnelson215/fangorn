package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port        string
	DBHost      string
	DBPort      string
	DBName      string
	DBUser      string
	DBPassword  string
	DBSSLMode   string
	AppPassword string

	// SchedulerInterval controls how often the recurring/snapshot scheduler ticks.
	SchedulerInterval time.Duration
	// SchedulerHorizonDays is how far past today occurrences are materialized so the
	// UI can show upcoming charges without posting them.
	SchedulerHorizonDays int

	// QuotesProvider picks where security prices come from: "yahoo" (default,
	// no key needed) or "none" to turn price fetching off.
	QuotesProvider string
	// QuotesMarketTTL is how stale a stock or ETF price may get during market
	// hours before it is fetched again.
	QuotesMarketTTL time.Duration
}

func Load() *Config {
	return &Config{
		Port:                 getEnv("PORT", "3000"),
		DBHost:               getEnv("DB_HOST", "localhost"),
		DBPort:               getEnv("DB_PORT", "5432"),
		DBName:               getEnv("DB_NAME", "fangorn"),
		DBUser:               getEnv("DB_USER", "postgres"),
		DBPassword:           os.Getenv("DB_PASSWORD"),
		DBSSLMode:            getEnv("DB_SSLMODE", "require"),
		AppPassword:          os.Getenv("APP_PASSWORD"),
		SchedulerInterval:    getEnvDuration("SCHEDULER_INTERVAL", 5*time.Minute),
		SchedulerHorizonDays: getEnvInt("SCHEDULER_HORIZON_DAYS", 60),
		QuotesProvider:       getEnv("QUOTES_PROVIDER", "yahoo"),
		QuotesMarketTTL:      getEnvDuration("QUOTES_MARKET_TTL", time.Minute),
	}
}

// DSN builds a libpq keyword/value connection string.
//
// Every value is single-quoted, which is not cosmetic. lib/pq's parser skips
// whitespace after `=`, so an unquoted empty value swallows the following token:
// `password= dbname=fangorn` parses as password="dbname=fangorn" with no dbname
// at all, and libpq then silently falls back to connecting to a database named
// after the user. That is a very confusing way to end up querying the wrong
// database, and an empty password is normal against a trust-auth local Postgres.
func (c *Config) DSN() string {
	parts := [][2]string{
		{"host", c.DBHost},
		{"port", c.DBPort},
		{"user", c.DBUser},
		{"password", c.DBPassword},
		{"dbname", c.DBName},
		{"sslmode", c.DBSSLMode},
	}

	var b strings.Builder
	for i, kv := range parts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(kv[0])
		b.WriteString("='")
		// Backslashes and quotes have to be escaped inside a quoted value.
		b.WriteString(dsnEscape.Replace(kv[1]))
		b.WriteByte('\'')
	}
	return b.String()
}

var dsnEscape = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("config: %s=%q is not a valid duration, using %s", key, v, fallback)
		return fallback
	}
	return d
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		log.Printf("config: %s=%q is not a positive integer, using %d", key, v, fallback)
		return fallback
	}
	return n
}
