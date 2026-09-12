package config

import (
	"strings"
	"testing"
)

// The bug this guards against: lib/pq skips whitespace after `=` when parsing a
// keyword/value DSN, so an unquoted empty value consumes the next token.
// `password= dbname=fangorn` parsed as password="dbname=fangorn" with no dbname,
// and libpq then connected to a database named after the user instead. Quoting
// every value makes an empty one unambiguous.
func TestDSNQuotesEmptyPassword(t *testing.T) {
	cfg := &Config{
		DBHost:     "127.0.0.1",
		DBPort:     "5432",
		DBUser:     "postgres",
		DBPassword: "",
		DBName:     "fangorn",
		DBSSLMode:  "disable",
	}

	dsn := cfg.DSN()

	if !strings.Contains(dsn, "password=''") {
		t.Errorf("empty password must be quoted, got: %s", dsn)
	}
	if !strings.Contains(dsn, "dbname='fangorn'") {
		t.Errorf("dbname must survive an empty password, got: %s", dsn)
	}
}

func TestDSNEscapesQuotesAndBackslashes(t *testing.T) {
	cfg := &Config{
		DBHost:     "db.internal",
		DBPort:     "5432",
		DBUser:     "app",
		DBPassword: `pa'ss\word`,
		DBName:     "fangorn",
		DBSSLMode:  "require",
	}

	dsn := cfg.DSN()

	if !strings.Contains(dsn, `password='pa\'ss\\word'`) {
		t.Errorf("special characters must be escaped, got: %s", dsn)
	}
	// An unescaped quote would terminate the value early and leave the rest of
	// the string to be misparsed as further keywords.
	if !strings.Contains(dsn, "dbname='fangorn'") {
		t.Errorf("dbname must survive a password containing a quote, got: %s", dsn)
	}
}

func TestDSNIncludesEveryField(t *testing.T) {
	cfg := &Config{
		DBHost:     "h",
		DBPort:     "1",
		DBUser:     "u",
		DBPassword: "p",
		DBName:     "n",
		DBSSLMode:  "s",
	}

	want := "host='h' port='1' user='u' password='p' dbname='n' sslmode='s'"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}
