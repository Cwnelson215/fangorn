package config

import "os"

type Config struct {
	Port          string
	TellerAppID   string
	TellerEnv     string
	TellerCertPath string
	TellerKeyPath  string
	DBHost        string
	DBPort        string
	DBName        string
	DBUser        string
	DBPassword    string
	EncryptionKey string
	DBSSLMode     string
	AppPassword   string
}

func Load() *Config {
	return &Config{
		Port:           getEnv("PORT", "3000"),
		TellerAppID:    os.Getenv("TELLER_APP_ID"),
		TellerEnv:      getEnv("TELLER_ENV", "sandbox"),
		TellerCertPath: os.Getenv("TELLER_CERT_PATH"),
		TellerKeyPath:  os.Getenv("TELLER_KEY_PATH"),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBName:         getEnv("DB_NAME", "fangorn"),
		DBUser:         getEnv("DB_USER", "postgres"),
		DBPassword:     os.Getenv("DB_PASSWORD"),
		EncryptionKey:  os.Getenv("ENCRYPTION_KEY"),
		DBSSLMode:      getEnv("DB_SSLMODE", "require"),
		AppPassword:    os.Getenv("APP_PASSWORD"),
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
