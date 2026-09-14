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
