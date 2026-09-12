package csvimport

import (
	"fmt"
	"sort"
	"sync"
)

var (
	parsers = map[string]BankParser{}
	mu      sync.RWMutex
)

// RegisterParser registers a parser under the given bank name.
func RegisterParser(name string, parser BankParser) {
	mu.Lock()
	defer mu.Unlock()
	parsers[name] = parser
}

// GetParser returns the parser for the given bank name.
func GetParser(bankName string) (BankParser, error) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := parsers[bankName]
	if !ok {
		return nil, fmt.Errorf("unsupported bank format: %s", bankName)
	}
	return p, nil
}

// SupportedBanks returns the list of supported bank format identifiers.
func SupportedBanks() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(parsers))
	for k := range parsers {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
