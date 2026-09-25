package ledger

import "testing"

func TestAccountInputNormalizeTrimsInstitution(t *testing.T) {
	str := func(s string) *string { return &s }

	for _, tc := range []struct {
		in   *string
		want *string
	}{
		{nil, nil},
		{str(""), nil},
		{str("   "), nil},
		{str(" Gesa "), str("Gesa")},
	} {
		// The mask is normalized as a list of cards; TestAccountCardsAreNormalized
		// covers it.
		in := AccountInput{Name: "Checking", Type: "checking", StartingBalanceDate: "2026-01-01",
			InstitutionName: tc.in}
		if err := in.normalize(); err != nil {
			t.Fatalf("normalize: %v", err)
		}
		if got := in.InstitutionName; (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("institution_name from %v = %v; want %v", deref(tc.in), deref(got), deref(tc.want))
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return "\"" + *s + "\""
}
