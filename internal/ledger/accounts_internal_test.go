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
		in := AccountInput{Name: "Checking", Type: "checking", StartingBalanceDate: "2026-01-01",
			InstitutionName: tc.in, Mask: tc.in}
		if err := in.normalize(); err != nil {
			t.Fatalf("normalize: %v", err)
		}
		for field, got := range map[string]*string{"institution_name": in.InstitutionName, "mask": in.Mask} {
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Errorf("%s from %v = %v; want %v", field, deref(tc.in), deref(got), deref(tc.want))
			}
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return "\"" + *s + "\""
}
