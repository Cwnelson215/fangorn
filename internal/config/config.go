package config

import (
	"fmt"
	"os"
	"strings"
)

type GmailAccount struct {
	Name          string   // label for logging (e.g. "personal", "work")
	ClientID      string
	ClientSecret  string
	RefreshToken  string
	SenderFilters []string
}

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
	GmailEnabled      bool
	GmailAccounts     []GmailAccount
	GmailPollInterval string
}

func Load() *Config {
	// Load Gmail accounts: supports GMAIL_1_*, GMAIL_2_*, etc.
	// Also supports legacy single-account GMAIL_CLIENT_ID for backwards compat.
	var gmailAccounts []GmailAccount

	for i := 1; i <= 10; i++ {
		prefix := fmt.Sprintf("GMAIL_%d_", i)
		clientID := os.Getenv(prefix + "CLIENT_ID")
		if clientID == "" {
			break
		}
		acct := GmailAccount{
			Name:         getEnv(prefix+"NAME", fmt.Sprintf("account-%d", i)),
			ClientID:     clientID,
			ClientSecret: os.Getenv(prefix + "CLIENT_SECRET"),
			RefreshToken: os.Getenv(prefix + "REFRESH_TOKEN"),
		}
		if sf := os.Getenv(prefix + "SENDER_FILTERS"); sf != "" {
			for _, s := range strings.Split(sf, ",") {
				if t := strings.TrimSpace(s); t != "" {
					acct.SenderFilters = append(acct.SenderFilters, t)
				}
			}
		}
		gmailAccounts = append(gmailAccounts, acct)
	}

	// Legacy single-account fallback
	if len(gmailAccounts) == 0 {
		if clientID := os.Getenv("GMAIL_CLIENT_ID"); clientID != "" {
			acct := GmailAccount{
				Name:         "default",
				ClientID:     clientID,
				ClientSecret: os.Getenv("GMAIL_CLIENT_SECRET"),
				RefreshToken: os.Getenv("GMAIL_REFRESH_TOKEN"),
			}
			if sf := os.Getenv("GMAIL_SENDER_FILTERS"); sf != "" {
				for _, s := range strings.Split(sf, ",") {
					if t := strings.TrimSpace(s); t != "" {
						acct.SenderFilters = append(acct.SenderFilters, t)
					}
				}
			}
			gmailAccounts = append(gmailAccounts, acct)
		}
	}

	return &Config{
		Port:              getEnv("PORT", "3000"),
		TellerAppID:       os.Getenv("TELLER_APP_ID"),
		TellerEnv:         getEnv("TELLER_ENV", "sandbox"),
		TellerCertPath:    os.Getenv("TELLER_CERT_PATH"),
		TellerKeyPath:     os.Getenv("TELLER_KEY_PATH"),
		TellerEnabled:     getEnv("TELLER_ENABLED", "false") == "true",
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnv("DB_PORT", "5432"),
		DBName:            getEnv("DB_NAME", "fangorn"),
		DBUser:            getEnv("DB_USER", "postgres"),
		DBPassword:        os.Getenv("DB_PASSWORD"),
		EncryptionKey:     os.Getenv("ENCRYPTION_KEY"),
		DBSSLMode:         getEnv("DB_SSLMODE", "require"),
		AppPassword:       os.Getenv("APP_PASSWORD"),
		GmailEnabled:      getEnv("GMAIL_ENABLED", "false") == "true",
		GmailAccounts:     gmailAccounts,
		GmailPollInterval: getEnv("GMAIL_POLL_INTERVAL", "5m"),
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
