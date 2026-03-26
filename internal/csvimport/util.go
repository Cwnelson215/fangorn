package csvimport

import (
	"fmt"
	"strings"
	"time"
)

// mapColumns returns a map of header name -> column index.
func mapColumns(header []string) map[string]int {
	m := make(map[string]int)
	for i, h := range header {
		m[strings.TrimSpace(h)] = i
	}
	return m
}

// parseDate tries common date formats and returns YYYY-MM-DD.
func parseDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	formats := []string{
		"01/02/2006",
		"1/2/2006",
		"2006-01-02",
		"01-02-2006",
		"Jan 2, 2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("cannot parse date: %s", s)
}
