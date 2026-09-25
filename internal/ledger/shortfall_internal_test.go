package ledger

import (
	"testing"

	"github.com/cwnelson/fangorn/internal/models"
)

func TestSavingsShortfall(t *testing.T) {
	cases := []struct {
		name    string
		m       incomeAccountMonth
		planOn  float64
		planned float64
		want    float64
	}{
		// $6,200 in, $500 planned: $5,700 is safe to spend.
		{"within plan", incomeAccountMonth{Out: 5700 + 500, ToGoals: 500}, 6200, 500, 0},
		{"over by 200", incomeAccountMonth{Out: 5900 + 500, ToGoals: 500}, 6200, 500, 200},
		// Savings not moved yet don't count as spending.
		{"savings not moved yet", incomeAccountMonth{Out: 5900}, 6200, 500, 200},
		{"capped at what was planned", incomeAccountMonth{Out: 9000}, 6200, 500, 500},
		{"nothing planned", incomeAccountMonth{Out: 9000}, 6200, 0, 0},
	}
	for _, c := range cases {
		if got := savingsShortfall(c.m, c.planOn, c.planned); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSplitShortfall(t *testing.T) {
	lines := []models.SavingsLine{{Monthly: 500}, {Monthly: 250}}
	splitShortfall(300, lines)
	if lines[0].Overspent != 200 || lines[1].Overspent != 100 {
		t.Errorf("split = %v / %v, want 200 / 100", lines[0].Overspent, lines[1].Overspent)
	}

	// Thirds don't divide into cents; the shares still add up exactly.
	lines = []models.SavingsLine{{Monthly: 100}, {Monthly: 100}, {Monthly: 100}}
	splitShortfall(100, lines)
	sum := lines[0].Overspent + lines[1].Overspent + lines[2].Overspent
	if round2(sum) != 100 || lines[0].Overspent != 33.33 {
		t.Errorf("thirds = %v, %v, %v", lines[0].Overspent, lines[1].Overspent, lines[2].Overspent)
	}
}
