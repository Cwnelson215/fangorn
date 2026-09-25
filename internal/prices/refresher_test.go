package prices

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/portfolio"
	"github.com/cwnelson/fangorn/internal/quotes"
	"github.com/cwnelson/fangorn/internal/testdb"
)

// fakeProvider serves prices from a map and counts calls.
type fakeProvider struct {
	mu     sync.Mutex
	prices map[string]float64
	fail   map[string]error
	calls  int
	// historyFrom records each History call as "SYMBOL YYYY-MM-DD".
	historyFrom []string
	// yields are published fund yields in percent; yieldCalls counts lookups.
	yields     map[string]float64
	yieldCalls int
}

func (p *fakeProvider) Yield(_ context.Context, symbol string) (float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.yieldCalls++
	if err := p.fail[symbol]; err != nil {
		return 0, err
	}
	y, ok := p.yields[symbol]
	if !ok {
		return 0, quotes.ErrNotFound
	}
	return y, nil
}

func (p *fakeProvider) yieldCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.yieldCalls
}

func (p *fakeProvider) Quote(ctx context.Context, symbol string) (quotes.Quote, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if err := p.fail[symbol]; err != nil {
		return quotes.Quote{}, err
	}
	price, ok := p.prices[symbol]
	if !ok {
		return quotes.Quote{}, quotes.ErrNotFound
	}
	return quotes.Quote{
		Symbol: symbol, Name: symbol + " Index Fund", QuoteType: "ETF", Currency: "USD",
		Price: price, PreviousClose: price - 1, PriceTime: time.Now(),
		Closes: []quotes.Close{{Date: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), Price: price - 1}},
	}, nil
}

func (p *fakeProvider) History(_ context.Context, symbol string, from time.Time) ([]quotes.Close, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.historyFrom = append(p.historyFrom, symbol+" "+from.Format(models.DateOnly))
	if err := p.fail[symbol]; err != nil {
		return nil, err
	}
	return []quotes.Close{{Date: from, Price: p.prices[symbol]}}, nil
}

func (p *fakeProvider) Search(context.Context, string) ([]quotes.Match, error) { return nil, nil }

func (p *fakeProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type fixture struct {
	t        *testing.T
	ctx      context.Context
	svc      *ledger.Service
	hh       int
	provider *fakeProvider
	r        *Refresher
	now      time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testdb.Open(t)
	svc := ledger.New(db)
	f := &fixture{
		t: t, ctx: context.Background(), svc: svc, hh: testdb.Household(t, db, "America/Denver"),
		provider: &fakeProvider{prices: map[string]float64{}, fail: map[string]error{}, yields: map[string]float64{}},
		// Saturday: nothing is open, so only quietTTL applies.
		now: et("2026-09-12 12:00"),
	}
	f.r = New(svc, f.provider, time.Minute)
	f.r.now = func() time.Time { return f.now }
	return f
}

// sym gives each test its own symbols: securities are shared across households.
func (f *fixture) sym(base string) string { return fmt.Sprintf("%s%d", base, f.hh) }

func (f *fixture) holding(symbol string, price float64) models.Account {
	f.t.Helper()
	a, err := f.svc.CreateAccount(f.ctx, f.hh, ledger.AccountInput{
		Name: "Brokerage " + symbol, Type: models.AccountInvestment, StartingBalance: 10000, StartingBalanceDate: "2026-01-01",
	})
	if err != nil {
		f.t.Fatal(err)
	}
	_, err = f.svc.CreateTrade(f.ctx, f.hh, a.ID, ledger.TradeInput{
		Symbol: symbol, Side: portfolio.SideBuy, TradeDate: "2026-02-01", Shares: 10, Price: price,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}

func (f *fixture) security(symbol string) models.Security {
	f.t.Helper()
	sec, err := f.svc.GetSecurity(f.ctx, symbol)
	if err != nil {
		f.t.Fatal(err)
	}
	return sec
}

func TestRefreshFetchesOnceUntilStale(t *testing.T) {
	f := newFixture(t)
	voo := f.sym("VOO")
	f.holding(voo, 500)
	f.provider.prices[voo] = 520

	if err := f.r.RefreshStale(f.ctx, []string{voo}); err != nil {
		t.Fatal(err)
	}
	sec := f.security(voo)
	if sec.Price != 520 || sec.PriceTime == nil || sec.Name == nil {
		t.Fatalf("security after refresh = %+v", sec)
	}

	f.now = f.now.Add(5 * time.Minute)
	if err := f.r.RefreshStale(f.ctx, []string{voo}); err != nil {
		t.Fatal(err)
	}
	if n := f.provider.callCount(); n != 1 {
		t.Errorf("provider called %d times, want 1 — the price was still fresh", n)
	}

	f.now = f.now.Add(20 * time.Minute)
	f.provider.prices[voo] = 525
	if err := f.r.RefreshStale(f.ctx, []string{voo}); err != nil {
		t.Fatal(err)
	}
	if n := f.provider.callCount(); n != 2 {
		t.Errorf("provider called %d times, want 2 once stale", n)
	}
	if got := f.security(voo).Price; got != 525 {
		t.Errorf("price = %v, want 525", got)
	}
}

func TestFailureBacksOffAndKeepsLastPrice(t *testing.T) {
	f := newFixture(t)
	gone := f.sym("GONE")
	f.holding(gone, 40) // seeded at 40, never quoted

	f.provider.fail[gone] = fmt.Errorf("%w: status 500", quotes.ErrUnavailable)
	if err := f.r.RefreshStale(f.ctx, []string{gone}); err == nil {
		t.Fatal("want the failure reported")
	}
	sec := f.security(gone)
	if sec.Price != 40 || sec.FetchError == nil {
		t.Fatalf("security after failure = %+v", sec)
	}

	f.now = f.now.Add(time.Minute)
	_ = f.r.RefreshStale(f.ctx, []string{gone})
	if n := f.provider.callCount(); n != 1 {
		t.Errorf("provider called %d times within the backoff, want 1", n)
	}

	f.now = f.now.Add(10 * time.Minute)
	delete(f.provider.fail, gone)
	f.provider.prices[gone] = 41
	if err := f.r.RefreshStale(f.ctx, []string{gone}); err != nil {
		t.Fatal(err)
	}
	if sec := f.security(gone); sec.Price != 41 || sec.FetchError != nil {
		t.Errorf("recovery did not clear the failure: %+v", sec)
	}
}

func TestEnsureSecurity(t *testing.T) {
	f := newFixture(t)

	t.Run("unknown symbol is a user error", func(t *testing.T) {
		err := f.r.EnsureSecurity(f.ctx, f.sym("TYPO"))
		var inv ledger.ErrInvalid
		if !errors.As(err, &inv) {
			t.Fatalf("want ErrInvalid, got %v", err)
		}
	})

	t.Run("an outage lets the trade go ahead", func(t *testing.T) {
		sym := f.sym("SLOW")
		f.provider.fail[sym] = quotes.ErrUnavailable
		if err := f.r.EnsureSecurity(f.ctx, sym); err != nil {
			t.Fatalf("want nil so the trade is seeded, got %v", err)
		}
	})

	t.Run("a known symbol is saved with its quote", func(t *testing.T) {
		sym := f.sym("VTI")
		f.provider.prices[sym] = 300
		if err := f.r.EnsureSecurity(f.ctx, sym); err != nil {
			t.Fatal(err)
		}
		if got := f.security(sym).Price; got != 300 {
			t.Errorf("price = %v", got)
		}
		calls := f.provider.callCount()
		if err := f.r.EnsureSecurity(f.ctx, sym); err != nil {
			t.Fatal(err)
		}
		if f.provider.callCount() != calls {
			t.Error("an existing security should not be re-fetched")
		}
	})
}

func TestRefreshGivesUpWhenOutOfTime(t *testing.T) {
	f := newFixture(t)
	f.r.sem <- struct{}{} // someone else is mid-refresh
	defer func() { <-f.r.sem }()

	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Millisecond)
	defer cancel()
	if err := f.r.RefreshStale(ctx, []string{"VOO"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want a deadline error, got %v", err)
	}
}

func TestNilProviderIsANoOp(t *testing.T) {
	r := New(nil, nil, time.Minute)
	if err := r.RefreshStale(context.Background(), []string{"VOO"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Search(context.Background(), "voo"); !errors.Is(err, quotes.ErrUnavailable) {
		t.Fatalf("search without a provider: %v", err)
	}
}

func TestBackfillHistoryOncePerGap(t *testing.T) {
	f := newFixture(t)
	voo, broken := f.sym("VOO"), f.sym("BROKE")
	a := f.holding(voo, 500) // bought 2026-02-01
	f.provider.prices[voo] = 500
	f.provider.fail[broken] = quotes.ErrUnavailable
	_, err := f.svc.CreateTrade(f.ctx, f.hh, a.ID, ledger.TradeInput{
		Symbol: broken, Side: portfolio.SideOpening, TradeDate: "2026-03-01", Shares: 1, Price: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := f.r.BackfillHistory(f.ctx, f.hh, nil); err == nil {
		t.Fatal("want the failing symbol reported")
	}
	// A week of lead before the first trade.
	if !contains(f.provider.historyFrom, voo+" 2026-01-25") {
		t.Fatalf("history calls = %v", f.provider.historyFrom)
	}

	delete(f.provider.fail, broken)
	calls := len(f.provider.historyFrom)
	if err := f.r.BackfillHistory(f.ctx, f.hh, nil); err != nil {
		t.Fatal(err)
	}
	if got := f.provider.historyFrom[calls:]; len(got) != 1 || got[0] != broken+" 2026-02-22" {
		t.Errorf("second pass should retry only the failed symbol, got %v", got)
	}
	if err := f.r.BackfillHistory(f.ctx, f.hh, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(f.provider.historyFrom); n != calls+1 {
		t.Errorf("a filled history should not be fetched again (%d calls)", n)
	}

	gaps, err := f.svc.HistoryGaps(f.ctx, f.hh, nil)
	if err != nil || len(gaps) != 0 {
		t.Errorf("gaps after backfill = %+v, %v", gaps, err)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
