package ledger_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/testdb"
)

// fixture is one test's service plus its own household.
type fixture struct {
	t   *testing.T
	ctx context.Context
	svc *ledger.Service
	hh  int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testdb.Open(t)
	return &fixture{t: t, ctx: context.Background(), svc: ledger.New(db), hh: testdb.Household(t, db, "America/Denver")}
}

func (f *fixture) account(name, typ string, starting float64) models.Account {
	f.t.Helper()
	a, err := f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
		Name: name, Type: typ, StartingBalance: starting, StartingBalanceDate: "2026-01-01",
	})
	if err != nil {
		f.t.Fatalf("CreateAccount(%s): %v", name, err)
	}
	return a
}

func (f *fixture) category(name, kind string) models.Category {
	f.t.Helper()
	c, err := f.svc.CreateCategory(f.ctx, f.hh, ledger.CategoryInput{Name: name, Kind: kind})
	if err != nil {
		f.t.Fatalf("CreateCategory(%s): %v", name, err)
	}
	return c
}

func (f *fixture) txn(accountID int, kind, date string, amount float64, categoryID *int) models.Transaction {
	f.t.Helper()
	tx, err := f.svc.CreateTransaction(f.ctx, f.hh, ledger.TransactionInput{
		AccountID: accountID, Kind: kind, Date: date, Amount: amount,
		Description: kind + " " + date, CategoryID: categoryID,
	})
	if err != nil {
		f.t.Fatalf("CreateTransaction: %v", err)
	}
	return tx
}

func (f *fixture) transfer(from, to int, amount float64, date string) models.Transfer {
	f.t.Helper()
	tr, err := f.svc.CreateTransfer(f.ctx, f.hh, ledger.TransferInput{
		FromAccountID: from, ToAccountID: to, Amount: amount, Date: date,
	})
	if err != nil {
		f.t.Fatalf("CreateTransfer: %v", err)
	}
	return tr
}

func (f *fixture) balance(id int) float64 {
	f.t.Helper()
	a, err := f.svc.GetAccount(f.ctx, f.hh, id)
	if err != nil {
		f.t.Fatalf("GetAccount(%d): %v", id, err)
	}
	return a.Balance
}

func (f *fixture) dashboard(from, to string) ledger.Dashboard {
	f.t.Helper()
	d, err := f.svc.Dashboard(f.ctx, f.hh, from, to)
	if err != nil {
		f.t.Fatalf("Dashboard: %v", err)
	}
	return d
}

func money(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Errorf("%s = %.2f, want %.2f", what, got, want)
	}
}

func wantInvalid(t *testing.T, err error) {
	t.Helper()
	var inv ledger.ErrInvalid
	if !errors.As(err, &inv) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestNetWorthCountsLiabilitiesAsDebt(t *testing.T) {
	f := newFixture(t)
	f.account("Checking", models.AccountChecking, 2500)
	f.account("Visa", models.AccountCreditCard, -400)

	d := f.dashboard("2026-01-01", "2026-01-31")
	money(t, "total assets", d.TotalAssets, 2500)
	money(t, "total liabilities", d.TotalLiabilities, 400)
	money(t, "net worth", d.NetWorth, 2100)
}

func TestTransactionSignComesFromKind(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	visa := f.account("Visa", models.AccountCreditCard, 0)

	// Callers send magnitudes; a stray minus sign must not flip an expense into income.
	groceries := f.txn(checking.ID, models.KindExpense, "2026-01-05", 84.20, nil)
	paycheck := f.txn(checking.ID, models.KindIncome, "2026-01-06", -500, nil)
	f.txn(visa.ID, models.KindExpense, "2026-01-07", 30, nil)

	money(t, "expense amount", groceries.Amount, -84.20)
	money(t, "income amount", paycheck.Amount, 500)
	money(t, "checking balance", f.balance(checking.ID), 1415.80)
	// A card purchase takes a liability further negative.
	money(t, "visa balance", f.balance(visa.ID), -30)
}

func TestTransactionValidation(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 0)

	cases := map[string]ledger.TransactionInput{
		"zero amount":   {AccountID: checking.ID, Kind: models.KindExpense, Date: "2026-01-01", Amount: 0, Description: "x"},
		"transfer kind": {AccountID: checking.ID, Kind: models.KindTransfer, Date: "2026-01-01", Amount: 5, Description: "x"},
		"bad date":      {AccountID: checking.ID, Kind: models.KindExpense, Date: "01/02/2026", Amount: 5, Description: "x"},
		"no desc":       {AccountID: checking.ID, Kind: models.KindExpense, Date: "2026-01-01", Amount: 5, Description: "  "},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.svc.CreateTransaction(f.ctx, f.hh, in)
			wantInvalid(t, err)
		})
	}
}

func TestTransferIsExcludedFromIncomeAndExpenses(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	savings := f.account("Savings", models.AccountSavings, 0)
	food := f.category("Food", models.KindExpense)

	f.txn(checking.ID, models.KindIncome, "2026-01-02", 300, nil)
	f.txn(checking.ID, models.KindExpense, "2026-01-03", 50, &food.ID)
	before := f.dashboard("2026-01-01", "2026-01-31")

	f.transfer(checking.ID, savings.ID, 200, "2026-01-04")
	after := f.dashboard("2026-01-01", "2026-01-31")

	money(t, "checking balance", f.balance(checking.ID), 1050)
	money(t, "savings balance", f.balance(savings.ID), 200)
	money(t, "income", after.Income, before.Income)
	money(t, "expenses", after.Expenses, before.Expenses)
	money(t, "net worth", after.NetWorth, before.NetWorth)
	if len(after.Categories) != 1 || after.Categories[0].CategoryName != "Food" {
		t.Errorf("category breakdown = %+v, want only Food", after.Categories)
	}

	legs, err := f.svc.ListTransactions(f.ctx, f.hh, ledger.TransactionFilter{Kind: models.KindTransfer})
	if err != nil {
		t.Fatal(err)
	}
	if len(legs) != 2 {
		t.Fatalf("got %d transfer legs, want 2", len(legs))
	}
	if legs[0].TransferGroupID == nil || legs[1].TransferGroupID == nil ||
		*legs[0].TransferGroupID != *legs[1].TransferGroupID {
		t.Errorf("transfer legs do not share a group id: %v, %v", legs[0].TransferGroupID, legs[1].TransferGroupID)
	}
	money(t, "sum of legs", legs[0].Amount+legs[1].Amount, 0)
}

func TestWeeklySpendingCoversEveryWeek(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	savings := f.account("Savings", models.AccountSavings, 0)

	// 2026-03-11 is a Wednesday, so its week starts Monday 2026-03-09.
	f.txn(checking.ID, models.KindExpense, "2026-03-09", 40, nil)
	f.txn(checking.ID, models.KindExpense, "2026-03-11", 10, nil)
	f.txn(checking.ID, models.KindExpense, "2026-03-12", 500, nil) // after `to`
	f.txn(checking.ID, models.KindIncome, "2026-03-10", 900, nil)
	f.transfer(checking.ID, savings.ID, 300, "2026-03-10")
	f.txn(checking.ID, models.KindExpense, "2026-02-17", 25, nil)

	weeks := f.dashboard("2026-03-01", "2026-03-11").WeeklySpending
	if len(weeks) != 12 {
		t.Fatalf("got %d weeks, want 12: %+v", len(weeks), weeks)
	}
	if weeks[0].Week != "2025-12-22" || weeks[11].Week != "2026-03-09" {
		t.Errorf("weeks run %s..%s, want 2025-12-22..2026-03-09", weeks[0].Week, weeks[11].Week)
	}
	want := map[string]float64{"2026-02-16": 25, "2026-03-09": 50}
	for _, w := range weeks {
		money(t, "spend in week of "+w.Week, w.Amount, want[w.Week])
	}
}

func TestTransferLegsMoveTogether(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	savings := f.account("Savings", models.AccountSavings, 0)
	brokerage := f.account("Brokerage", models.AccountInvestment, 0)
	tr := f.transfer(checking.ID, savings.ID, 200, "2026-01-04")

	legs, err := f.svc.ListTransactions(f.ctx, f.hh, ledger.TransactionFilter{AccountID: savings.ID})
	if err != nil || len(legs) != 1 {
		t.Fatalf("savings legs = %v, %v", legs, err)
	}
	leg := legs[0]

	t.Run("a single leg cannot be edited", func(t *testing.T) {
		_, err := f.svc.UpdateTransaction(f.ctx, f.hh, leg.ID, ledger.TransactionInput{
			AccountID: savings.ID, Kind: models.KindIncome, Date: leg.Date, Amount: 999, Description: "hack",
		})
		wantInvalid(t, err)
	})

	t.Run("updating the transfer rewrites both legs", func(t *testing.T) {
		updated, err := f.svc.UpdateTransfer(f.ctx, f.hh, tr.GroupID, ledger.TransferInput{
			FromAccountID: checking.ID, ToAccountID: brokerage.ID, Amount: 150, Date: "2026-01-05",
		})
		if err != nil {
			t.Fatal(err)
		}
		if updated.GroupID != tr.GroupID || updated.ToAccountID != brokerage.ID {
			t.Errorf("updated transfer = %+v", updated)
		}
		money(t, "checking", f.balance(checking.ID), 850)
		money(t, "savings", f.balance(savings.ID), 0)
		money(t, "brokerage", f.balance(brokerage.ID), 150)
	})

	t.Run("deleting one leg deletes both", func(t *testing.T) {
		legs, err := f.svc.ListTransactions(f.ctx, f.hh, ledger.TransactionFilter{AccountID: brokerage.ID})
		if err != nil || len(legs) != 1 {
			t.Fatalf("brokerage legs = %v, %v", legs, err)
		}
		if err := f.svc.DeleteTransaction(f.ctx, f.hh, legs[0].ID); err != nil {
			t.Fatal(err)
		}
		money(t, "checking", f.balance(checking.ID), 1000)
		money(t, "brokerage", f.balance(brokerage.ID), 0)
		if _, err := f.svc.GetTransfer(f.ctx, f.hh, tr.GroupID); !errors.Is(err, ledger.ErrNotFound) {
			t.Errorf("GetTransfer after delete: %v, want ErrNotFound", err)
		}
	})
}

func TestTransferValidation(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 0)

	_, err := f.svc.CreateTransfer(f.ctx, f.hh, ledger.TransferInput{
		FromAccountID: checking.ID, ToAccountID: checking.ID, Amount: 10, Date: "2026-01-01",
	})
	wantInvalid(t, err)
}

func TestRegisterRunningBalance(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 100)
	savings := f.account("Savings", models.AccountSavings, 0)

	f.txn(checking.ID, models.KindIncome, "2026-01-01", 50, nil)
	f.txn(checking.ID, models.KindExpense, "2026-01-03", 30, nil)
	f.transfer(checking.ID, savings.ID, 20, "2026-01-02")

	reg, err := f.svc.Register(f.ctx, f.hh, checking.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Newest first; each row's running balance includes itself and everything older.
	want := []struct {
		date    string
		running float64
	}{{"2026-01-03", 100}, {"2026-01-02", 130}, {"2026-01-01", 150}}
	if len(reg) != len(want) {
		t.Fatalf("register has %d rows, want %d", len(reg), len(want))
	}
	for i, w := range want {
		if reg[i].Date != w.date || reg[i].RunningBalance == nil {
			t.Fatalf("row %d = %s (running %v), want %s", i, reg[i].Date, reg[i].RunningBalance, w.date)
		}
		money(t, "running balance on "+w.date, *reg[i].RunningBalance, w.running)
	}
}

func TestHouseholdsCannotSeeEachOther(t *testing.T) {
	f := newFixture(t)
	other := &fixture{t: t, ctx: f.ctx, svc: f.svc, hh: testdb.Household(t, testdb.Open(t), "America/Denver")}

	theirs := other.account("Their Checking", models.AccountChecking, 500)
	theirCategory := other.category("Theirs", models.KindExpense)
	mine := f.account("My Checking", models.AccountChecking, 0)

	if _, err := f.svc.GetAccount(f.ctx, f.hh, theirs.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("GetAccount across households: %v, want ErrNotFound", err)
	}
	_, err := f.svc.CreateTransaction(f.ctx, f.hh, ledger.TransactionInput{
		AccountID: theirs.ID, Kind: models.KindExpense, Date: "2026-01-01", Amount: 1, Description: "x",
	})
	wantInvalid(t, err)
	_, err = f.svc.CreateTransaction(f.ctx, f.hh, ledger.TransactionInput{
		AccountID: mine.ID, Kind: models.KindExpense, Date: "2026-01-01", Amount: 1, Description: "x",
		CategoryID: &theirCategory.ID,
	})
	wantInvalid(t, err)
	_, err = f.svc.CreateTransfer(f.ctx, f.hh, ledger.TransferInput{
		FromAccountID: mine.ID, ToAccountID: theirs.ID, Amount: 1, Date: "2026-01-01",
	})
	wantInvalid(t, err)
	if err := f.svc.DeleteAccount(f.ctx, f.hh, theirs.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("DeleteAccount across households: %v, want ErrNotFound", err)
	}
	money(t, "their balance", other.balance(theirs.ID), 500)
}

func TestDeletingACategoryInUseArchivesIt(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 100)
	unused := f.category("Unused", models.KindExpense)
	used := f.category("Used", models.KindExpense)
	f.txn(checking.ID, models.KindExpense, "2026-01-02", 10, &used.ID)

	if err := f.svc.DeleteCategory(f.ctx, f.hh, unused.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteCategory(f.ctx, f.hh, used.ID); err != nil {
		t.Fatal(err)
	}

	all, err := f.svc.ListCategories(f.ctx, f.hh, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != used.ID || !all[0].Archived {
		t.Fatalf("categories after delete = %+v, want only Used, archived", all)
	}
	txns, err := f.svc.ListTransactions(f.ctx, f.hh, ledger.TransactionFilter{})
	if err != nil || len(txns) != 1 || txns[0].CategoryName == nil || *txns[0].CategoryName != "Used" {
		t.Errorf("transaction lost its category label: %+v, %v", txns, err)
	}

	// The archived name is still taken, so restoring is the only way back to it.
	_, err = f.svc.CreateCategory(f.ctx, f.hh, ledger.CategoryInput{Name: "used", Kind: models.KindExpense})
	wantInvalid(t, err)

	restored, err := f.svc.UnarchiveCategory(f.ctx, f.hh, used.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Archived {
		t.Error("restored category is still archived")
	}
	active, err := f.svc.ListCategories(f.ctx, f.hh, false)
	if err != nil || len(active) != 1 {
		t.Errorf("active categories after restore = %+v, %v", active, err)
	}
}

func TestBudgetSpendIsThatMonthsExpensesOnly(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	savings := f.account("Savings", models.AccountSavings, 0)
	food := f.category("Food", models.KindExpense)

	if _, err := f.svc.SetBudget(f.ctx, f.hh, ledger.BudgetInput{
		CategoryID: food.ID, Amount: 400, EffectiveFrom: "2026-01",
	}); err != nil {
		t.Fatal(err)
	}
	f.txn(checking.ID, models.KindExpense, "2026-02-03", 120, &food.ID)
	f.txn(checking.ID, models.KindExpense, "2026-02-28", 30.50, &food.ID)
	f.txn(checking.ID, models.KindExpense, "2026-01-31", 999, &food.ID) // previous month
	f.txn(checking.ID, models.KindExpense, "2026-03-01", 999, &food.ID) // next month
	f.transfer(checking.ID, savings.ID, 999, "2026-02-10")

	budgets, err := f.svc.ListBudgets(f.ctx, f.hh, "2026-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(budgets) != 1 {
		t.Fatalf("got %d budgets, want 1", len(budgets))
	}
	money(t, "budget amount (carried forward from January)", budgets[0].Amount, 400)
	money(t, "February spend", budgets[0].Spent, 150.50)
}

// budgetAmounts maps category name to budgeted amount for one month.
func (f *fixture) budgetAmounts(month string) map[string]float64 {
	f.t.Helper()
	budgets, err := f.svc.ListBudgets(f.ctx, f.hh, month)
	if err != nil {
		f.t.Fatalf("ListBudgets(%s): %v", month, err)
	}
	out := map[string]float64{}
	for _, b := range budgets {
		out[b.CategoryName] = b.Amount
	}
	return out
}

func (f *fixture) setBudget(categoryID int, amount float64, month string) models.Budget {
	f.t.Helper()
	b, err := f.svc.SetBudget(f.ctx, f.hh, ledger.BudgetInput{
		CategoryID: categoryID, Amount: amount, EffectiveFrom: month,
	})
	if err != nil {
		f.t.Fatalf("SetBudget(%s): %v", month, err)
	}
	return b
}

func TestStopBudgetKeepsEarlierMonths(t *testing.T) {
	f := newFixture(t)
	food := f.category("Food", models.KindExpense)

	f.setBudget(food.ID, 400, "2026-01")
	march := f.setBudget(food.ID, 500, "2026-03")
	f.setBudget(food.ID, 600, "2026-07")

	if err := f.svc.StopBudget(f.ctx, f.hh, march.ID, "2026-03"); err != nil {
		t.Fatal(err)
	}

	// Stopping in March must not resurrect January's $400, and must not erase it
	// from the months it already covered. July's later change goes too.
	money(t, "January", f.budgetAmounts("2026-01")["Food"], 400)
	money(t, "February", f.budgetAmounts("2026-02")["Food"], 400)
	for _, month := range []string{"2026-03", "2026-04", "2026-07", "2026-12"} {
		if got := f.budgetAmounts(month); len(got) != 0 {
			t.Errorf("%s budgets after stop = %v, want none", month, got)
		}
	}

	f.setBudget(food.ID, 450, "2026-05")
	if got := f.budgetAmounts("2026-04"); len(got) != 0 {
		t.Errorf("April after restarting in May = %v, want none", got)
	}
	money(t, "May after restart", f.budgetAmounts("2026-05")["Food"], 450)

	// Re-setting the exact month a budget was stopped in restarts it too.
	f.setBudget(food.ID, 520, "2026-03")
	money(t, "March after restart", f.budgetAmounts("2026-03")["Food"], 520)

	if err := f.svc.StopBudget(f.ctx, f.hh, 999999, "2026-03"); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("stopping a missing budget = %v, want ErrNotFound", err)
	}
}

func TestSetBudgetRejectsNonExpenseCategories(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	salary := f.category("Salary", models.KindIncome)
	old := f.category("Old", models.KindExpense)

	_, err := f.svc.SetBudget(f.ctx, f.hh, ledger.BudgetInput{CategoryID: salary.ID, Amount: 100})
	wantInvalid(t, err)

	f.txn(checking.ID, models.KindExpense, "2026-02-03", 10, &old.ID) // in use, so delete archives
	if err := f.svc.DeleteCategory(f.ctx, f.hh, old.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.SetBudget(f.ctx, f.hh, ledger.BudgetInput{CategoryID: old.ID, Amount: 100})
	wantInvalid(t, err)
}

func TestBudgetsHideArchivedCategories(t *testing.T) {
	f := newFixture(t)
	food := f.category("Food", models.KindExpense)
	f.setBudget(food.ID, 400, "2026-01")

	// The budget itself keeps the category in use, so this archives it.
	if err := f.svc.DeleteCategory(f.ctx, f.hh, food.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.budgetAmounts("2026-02"); len(got) != 0 {
		t.Errorf("budgets with an archived category = %v, want none", got)
	}

	if _, err := f.svc.UnarchiveCategory(f.ctx, f.hh, food.ID); err != nil {
		t.Fatal(err)
	}
	money(t, "budget after unarchive", f.budgetAmounts("2026-02")["Food"], 400)
}

func TestBudgetMonthUnbudgetedSpend(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	savings := f.account("Savings", models.AccountSavings, 0)
	food := f.category("Food", models.KindExpense)
	fun := f.category("Fun", models.KindExpense)
	salary := f.category("Salary", models.KindIncome)

	f.setBudget(food.ID, 400, "2026-01")
	f.txn(checking.ID, models.KindExpense, "2026-02-03", 120, &food.ID)
	f.txn(checking.ID, models.KindExpense, "2026-02-04", 40, &fun.ID)
	f.txn(checking.ID, models.KindExpense, "2026-02-05", 9.99, nil)
	f.txn(checking.ID, models.KindExpense, "2026-03-01", 500, &fun.ID) // next month
	f.txn(checking.ID, models.KindIncome, "2026-02-15", 2000, &salary.ID)
	f.transfer(checking.ID, savings.ID, 300, "2026-02-10")

	bm, err := f.svc.BudgetMonth(f.ctx, f.hh, "2026-02-17")
	if err != nil {
		t.Fatal(err)
	}
	if bm.Month != "2026-02-01" {
		t.Errorf("month = %q, want 2026-02-01", bm.Month)
	}
	if len(bm.Budgets) != 1 {
		t.Fatalf("got %d budgets, want 1", len(bm.Budgets))
	}
	money(t, "food spent", bm.Budgets[0].Spent, 120)
	money(t, "unbudgeted", bm.UnbudgetedSpent, 49.99)
}

func TestBudgetScheduledSpend(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 1000)
	food := f.category("Food", models.KindExpense)
	fun := f.category("Fun", models.KindExpense)

	// Scheduled spend depends on the real "today", so every date here is relative
	// to the fixture household's current month.
	today := ledger.Household{Timezone: "America/Denver"}.Today()
	thisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	prev, next := thisMonth.AddDate(0, -1, 0), thisMonth.AddDate(0, 1, 0)
	ym := func(d time.Time) string { return d.Format("2006-01") }
	on := func(month time.Time, day int) string { return month.AddDate(0, 0, day-1).Format(models.DateOnly) }
	day := func(d int) *int { return &d }

	f.setBudget(food.ID, 400, ym(prev))

	rule := func(in ledger.RuleInput) models.RecurringRule {
		t.Helper()
		in.AccountID = checking.ID
		if in.Kind == "" {
			in.Kind = models.KindExpense
		}
		r, err := f.svc.CreateRule(f.ctx, f.hh, in)
		if err != nil {
			t.Fatalf("CreateRule(%s): %v", in.Name, err)
		}
		return r
	}

	// Back-dated and never posted: its date in this month is still owed.
	rule(ledger.RuleInput{Name: "streaming", CategoryID: &food.ID, Amount: 15,
		Frequency: "monthly", DayOfMonth: day(10), StartDate: on(prev, 1)})
	// Manual rules are commitments too, even though they never auto-post.
	manual := false
	rule(ledger.RuleInput{Name: "csa box", CategoryID: &food.ID, Amount: 3,
		Frequency: "monthly", DayOfMonth: day(1), StartDate: on(prev, 1), AutoPost: &manual})
	// Twice next month, and the 5th has already posted — only the 20th remains.
	twice := rule(ledger.RuleInput{Name: "meal kit", CategoryID: &food.ID, Amount: 50,
		Frequency: "semimonthly", DayOfMonth: day(5), SecondDayOfMonth: day(20), StartDate: on(next, 1)})
	// None of these count against Food.
	paused := rule(ledger.RuleInput{Name: "paused", CategoryID: &food.ID, Amount: 1000,
		Frequency: "monthly", StartDate: on(prev, 1)})
	if err := f.svc.SetRulePaused(f.ctx, f.hh, paused.ID, true); err != nil {
		t.Fatal(err)
	}
	rule(ledger.RuleInput{Name: "other category", CategoryID: &fun.ID, Amount: 7,
		Frequency: "monthly", StartDate: on(prev, 1)})

	var occID int
	if err := f.svc.DB().QueryRow(
		`INSERT INTO recurring_occurrences (rule_id, due_date) VALUES ($1, $2) RETURNING id`,
		twice.ID, on(next, 5)).Scan(&occID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.PostOccurrence(f.ctx, twice, occID, next.AddDate(0, 0, 4)); err != nil {
		t.Fatal(err)
	}

	food1 := func(month time.Time) models.Budget {
		t.Helper()
		budgets, err := f.svc.ListBudgets(f.ctx, f.hh, ym(month))
		if err != nil {
			t.Fatal(err)
		}
		if len(budgets) != 1 {
			t.Fatalf("%s: got %d budgets, want 1", ym(month), len(budgets))
		}
		return budgets[0]
	}

	money(t, "previous month scheduled (past months report none)", food1(prev).Scheduled, 0)
	money(t, "this month scheduled", food1(thisMonth).Scheduled, 15+3)
	nb := food1(next)
	money(t, "next month scheduled", nb.Scheduled, 15+3+50)
	money(t, "next month spent (the posted meal kit)", nb.Spent, 50)
}

func TestPostOccurrenceOnlyPostsOnce(t *testing.T) {
	f := newFixture(t)
	checking := f.account("Checking", models.AccountChecking, 100)
	savings := f.account("Savings", models.AccountSavings, 0)

	for _, kind := range []string{models.KindExpense, models.KindTransfer} {
		t.Run(kind, func(t *testing.T) {
			in := ledger.RuleInput{
				Name: "rule " + kind, Kind: kind, AccountID: checking.ID, Amount: 10,
				Frequency: "monthly", StartDate: "2026-01-15",
			}
			if kind == models.KindTransfer {
				in.ToAccountID = &savings.ID
			}
			rule, err := f.svc.CreateRule(f.ctx, f.hh, in)
			if err != nil {
				t.Fatal(err)
			}

			var occID int
			if err := f.svc.DB().QueryRow(
				`INSERT INTO recurring_occurrences (rule_id, due_date) VALUES ($1, '2026-01-15') RETURNING id`,
				rule.ID).Scan(&occID); err != nil {
				t.Fatal(err)
			}
			due := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

			if err := f.svc.PostOccurrence(f.ctx, rule, occID, due); err != nil {
				t.Fatalf("first post: %v", err)
			}
			if err := f.svc.PostOccurrence(f.ctx, rule, occID, due); !ledger.IsAlreadyPosted(err) {
				t.Fatalf("second post: %v, want already-posted", err)
			}

			var rows int
			if err := f.svc.DB().QueryRow(
				`SELECT COUNT(*) FROM transactions WHERE recurring_rule_id = $1`, rule.ID).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			wantRows := 1
			if kind == models.KindTransfer {
				wantRows = 2
			}
			if rows != wantRows {
				t.Errorf("rule wrote %d transaction rows, want %d", rows, wantRows)
			}
		})
	}
	money(t, "checking", f.balance(checking.ID), 80)
	money(t, "savings", f.balance(savings.ID), 10)
}
