package config

import (
	"os"
	"strings"
)

type Config struct {
	Port           string
	TellerAppID    string
	TellerEnv      string
	TellerCertPath string
	TellerKeyPath  string
	TellerEnabled  bool
	DBHost         string
	DBPort         string
	DBName         string
	DBUser         string
	DBPassword     string
	EncryptionKey  string
	DBSSLMode      string
	AppPassword    string
	// Gmail watcher
	GmailEnabled       bool
	GmailClientID      string
	GmailClientSecret  string
	GmailRefreshToken  string
	GmailSenderFilters []string
	GmailPollInterval  string
}

func Load() *Config {
	senderFilters := []string{}
	if sf := os.Getenv("GMAIL_SENDER_FILTERS"); sf != "" {
		for _, s := range strings.Split(sf, ",") {
			if t := strings.TrimSpace(s); t != "" {
				senderFilters = append(senderFilters, t)
			}
		}
	}

	return &Config{
		Port:               getEnv("PORT", "3000"),
		TellerAppID:        os.Getenv("TELLER_APP_ID"),
		TellerEnv:          getEnv("TELLER_ENV", "sandbox"),
		TellerCertPath:     os.Getenv("TELLER_CERT_PATH"),
		TellerKeyPath:      os.Getenv("TELLER_KEY_PATH"),
		TellerEnabled:      getEnv("TELLER_ENABLED", "false") == "true",
		DBHost:             getEnv("DB_HOST", "localhost"),
		DBPort:             getEnv("DB_PORT", "5432"),
		DBName:             getEnv("DB_NAME", "fangorn"),
		DBUser:             getEnv("DB_USER", "postgres"),
		DBPassword:         os.Getenv("DB_PASSWORD"),
		EncryptionKey:      os.Getenv("ENCRYPTION_KEY"),
		DBSSLMode:          getEnv("DB_SSLMODE", "require"),
		AppPassword:        os.Getenv("APP_PASSWORD"),
		GmailEnabled:       getEnv("GMAIL_ENABLED", "false") == "true",
		GmailClientID:      os.Getenv("GMAIL_CLIENT_ID"),
		GmailClientSecret:  os.Getenv("GMAIL_CLIENT_SECRET"),
		GmailRefreshToken:  os.Getenv("GMAIL_REFRESH_TOKEN"),
		GmailSenderFilters: senderFilters,
		GmailPollInterval:  getEnv("GMAIL_POLL_INTERVAL", "5m"),
	}
}

func (c *Config) DSN() string {
	return "host=" + c.DBHost +
		" port=" + c.DBPort +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" sslmode=" + c.DBSSLMode
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
