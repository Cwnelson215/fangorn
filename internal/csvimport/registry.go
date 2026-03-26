package csvimport

import (
	"fmt"
	"sort"
)

var parsers = map[string]BankParser{
	"chase_credit":   &ChaseCreditParser{},
	"chase_checking": &ChaseCheckingParser{},
	"bofa":           &BofAParser{},
}

// GetParser returns the parser for the given bank name.
func GetParser(bankName string) (BankParser, error) {
	p, ok := parsers[bankName]
	if !ok {
		return nil, fmt.Errorf("unsupported bank format: %s", bankName)
	}
	return p, nil
}

// SupportedBanks returns the list of supported bank format identifiers.
func SupportedBanks() []string {
	names := make([]string, 0, len(parsers))
	for k := range parsers {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
