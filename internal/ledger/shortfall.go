package ledger

import (
	"context"
	"fmt"
	"math"

	"github.com/lib/pq"

	"github.com/cwnelson/fangorn/internal/models"
)

// incomeAccountMonth is what went through the income account in one month.
type incomeAccountMonth struct {
	// Income is the income received into it.
	Income float64
	// Out is everything else that left it, net of what came back (transfers in,
	// refunds). Savings transfers are in here too.
	Out float64
	// ToGoals is the part of Out moved to savings goals' accounts. Only money
	// going out counts: pulling money back from savings already lowers that
	// goal's month by itself, so offsetting it here too would count it twice.
	ToGoals float64
}

// savingsShortfall is how much of the month's planned savings was spent
// instead of saved: whatever left the income account, other than the savings
// themselves, beyond the income there was to plan on less the savings planned.
// It can't be more than was planned.
func savingsShortfall(m incomeAccountMonth, planOn, planned float64) float64 {
	spent := m.Out - m.ToGoals
	over := spent - (planOn - planned)
	return round2(math.Max(0, math.Min(planned, over)))
}

// splitShortfall charges a shortfall to the savings lines in proportion to
// their monthly amounts. The last line takes the rounding remainder, so the
// shares add up to the total exactly.
func splitShortfall(total float64, lines []models.SavingsLine) {
	var planned float64
	for _, l := range lines {
		planned += l.Monthly
	}
	if total <= 0 || planned <= 0 {
		return
	}
	left := total
	for i := range lines {
		if i == len(lines)-1 {
			lines[i].Overspent = round2(left)
			return
		}
		share := round2(total * lines[i].Monthly / planned)
		lines[i].Overspent = share
		left -= share
	}
}

// savingsShortfallFor fills in each savings line's share of what was spent
// from savings this month. It needs an income account to measure against; with
// none chosen there's nothing to say.
//
// The income to plan on is what was received — but for the current or a future
// month, the expected income when that's higher, since rent goes out on the 1st
// and the paycheck lands on the 15th. A past month has had its paychecks.
func (s *Service) savingsShortfallFor(ctx context.Context, householdID int, monthStart string, current bool,
	expected float64, lines []models.SavingsLine) (float64, error) {
	settings, err := s.GetSettings(ctx, householdID)
	if err != nil || settings.IncomeAccountID == nil || len(lines) == 0 {
		return 0, err
	}
	incomeAccount := *settings.IncomeAccountID

	var goalAccounts []int64
	var planned float64
	for _, l := range lines {
		planned += l.Monthly
		if l.AccountID != nil && *l.AccountID != incomeAccount {
			goalAccounts = append(goalAccounts, int64(*l.AccountID))
		}
	}

	var m incomeAccountMonth
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(t.amount) FILTER (WHERE t.kind = 'income'), 0),
		        COALESCE(-SUM(t.amount) FILTER (WHERE t.kind <> 'income'), 0),
		        COALESCE(-SUM(t.amount) FILTER (
		          WHERE t.kind = 'transfer' AND t.amount < 0 AND EXISTS (
		            SELECT 1 FROM transactions o
		            WHERE o.transfer_group_id = t.transfer_group_id AND o.id <> t.id
		              AND o.account_id = ANY($4))), 0)
		 FROM transactions t
		 WHERE t.household_id = $1 AND t.account_id = $2
		   AND t.date >= $3::date AND t.date < ($3::date + INTERVAL '1 month')`,
		householdID, incomeAccount, monthStart, pq.Array(goalAccounts),
	).Scan(&m.Income, &m.Out, &m.ToGoals)
	if err != nil {
		return 0, fmt.Errorf("totalling the income account: %w", err)
	}

	planOn := m.Income
	if current && expected > planOn {
		planOn = expected
	}
	total := savingsShortfall(m, planOn, planned)
	splitShortfall(total, lines)
	return total, nil
}
