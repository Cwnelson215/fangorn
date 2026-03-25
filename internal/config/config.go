package config

import "os"

type Config struct {
	Port          string
	PlaidEnv      string
	DBHost        string
	DBPort        string
	DBName        string
	DBUser        string
	DBPassword    string
	PlaidClientID string
	PlaidSecret   string
	EncryptionKey string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "3000"),
		PlaidEnv:      getEnv("PLAID_ENV", "sandbox"),
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnv("DB_PORT", "5432"),
		DBName:        getEnv("DB_NAME", "fangorn"),
		DBUser:        getEnv("DB_USER", "postgres"),
		DBPassword:    os.Getenv("DB_PASSWORD"),
		PlaidClientID: os.Getenv("PLAID_CLIENT_ID"),
		PlaidSecret:   os.Getenv("PLAID_SECRET"),
		EncryptionKey: os.Getenv("ENCRYPTION_KEY"),
	}
}

func (c *Config) DSN() string {
	return "host=" + c.DBHost +
		" port=" + c.DBPort +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" sslmode=require"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
