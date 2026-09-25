package quotes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Yahoo reads the unofficial Yahoo Finance endpoints.
//
// It uses the per-symbol chart endpoint rather than the batch v7 quote endpoint,
// which now demands a crumb and cookie. Two quirks, both found the hard way:
//
//   - A full browser User-Agent string got a 429 while the bare "Mozilla/5.0"
//     was served normally. If quotes start failing with 429s, look here first.
//   - An unknown symbol is a 404 with a JSON error body, which is how "no such
//     symbol" is told apart from an outage.
type Yahoo struct {
	client  *http.Client
	baseURL string

	// Fund yields come from quoteSummary, which — unlike chart — wants a session
	// cookie and a "crumb" token issued against it. Both are fetched on first use
	// and kept until Yahoo rejects the crumb.
	cookieURL string
	crumbMu   sync.Mutex
	crumb     string
}

const (
	yahooBaseURL   = "https://query1.finance.yahoo.com"
	yahooCookieURL = "https://fc.yahoo.com"
	yahooUserAgent = "Mozilla/5.0"
)

func NewYahoo() *Yahoo {
	jar, _ := cookiejar.New(nil) // never fails without options
	return &Yahoo{
		client:    &http.Client{Timeout: 10 * time.Second, Jar: jar},
		baseURL:   yahooBaseURL,
		cookieURL: yahooCookieURL,
	}
}

// newYahooAt points the client at a test server.
func newYahooAt(baseURL string) *Yahoo {
	y := NewYahoo()
	y.baseURL = baseURL
	y.cookieURL = baseURL + "/cookie"
	return y
}

// holdableTypes are the search results worth offering. Yahoo also returns
// indexes (^GSPC), futures and currencies, none of which can sit in an account.
//
// The search endpoint spells money market funds MONEY_MARKET while the chart
// endpoint says MONEYMARKET; searchType folds the first into the second, so
// SPAXX isn't filtered out and both paths agree on what is stored.
var holdableTypes = map[string]bool{
	"EQUITY": true, "ETF": true, "MUTUALFUND": true, "MONEYMARKET": true, "CRYPTOCURRENCY": true,
}

type chartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol                     string   `json:"symbol"`
				Currency                   string   `json:"currency"`
				InstrumentType             string   `json:"instrumentType"`
				FullExchangeName           string   `json:"fullExchangeName"`
				ExchangeTimezoneName       string   `json:"exchangeTimezoneName"`
				LongName                   string   `json:"longName"`
				ShortName                  string   `json:"shortName"`
				RegularMarketPrice         *float64 `json:"regularMarketPrice"`
				RegularMarketTime          int64    `json:"regularMarketTime"`
				RegularMarketChangePercent *float64 `json:"regularMarketChangePercent"`
				PreviousClose              *float64 `json:"previousClose"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []chartQuote `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func (y *Yahoo) Quote(ctx context.Context, symbol string) (Quote, error) {
	q := url.Values{"range": {"5d"}, "interval": {"1d"}}
	res, err := y.chart(ctx, symbol, q)
	if err != nil {
		return Quote{}, err
	}
	r := res.Chart.Result[0]
	m := r.Meta
	if m.RegularMarketPrice == nil {
		return Quote{}, fmt.Errorf("%w: yahoo: %s has no price", ErrUnavailable, symbol)
	}

	quote := Quote{
		Symbol:    strings.ToUpper(m.Symbol),
		Name:      firstNonEmpty(m.LongName, m.ShortName, m.Symbol),
		QuoteType: m.InstrumentType,
		Currency:  firstNonEmpty(m.Currency, "USD"),
		Exchange:  m.FullExchangeName,
		Price:     *m.RegularMarketPrice,
		PriceTime: time.Unix(m.RegularMarketTime, 0).UTC(),
		Closes:    closes(r.Timestamp, r.Indicators.Quote, m.ExchangeTimezoneName),
	}
	quote.PreviousClose = previousClose(quote.Price, m.RegularMarketChangePercent, m.PreviousClose, quote.Closes)
	return quote, nil
}

// previousClose works out what the day's change is measured from.
//
// The change percent is preferred because it is right for both kinds of fund:
// a mutual fund's latest NAV can be published the next morning, dated after the
// day it belongs to, so "the close before today" would compare the NAV with
// itself. Deriving it from the percent sidesteps the dating question entirely.
// chartPreviousClose is deliberately ignored — it is the close before the start
// of the requested range, not before today.
func previousClose(price float64, changePct, prev *float64, recent []Close) float64 {
	if changePct != nil && *changePct > -100 {
		return price / (1 + *changePct/100)
	}
	if prev != nil {
		return *prev
	}
	if n := len(recent); n >= 2 {
		return recent[n-2].Price
	}
	return price
}

func (y *Yahoo) History(ctx context.Context, symbol string, from time.Time) ([]Close, error) {
	q := url.Values{
		"period1":  {strconv.FormatInt(from.Unix(), 10)},
		"period2":  {strconv.FormatInt(time.Now().Unix(), 10)},
		"interval": {"1d"},
	}
	res, err := y.chart(ctx, symbol, q)
	if err != nil {
		return nil, err
	}
	r := res.Chart.Result[0]
	return closes(r.Timestamp, r.Indicators.Quote, r.Meta.ExchangeTimezoneName), nil
}

func (y *Yahoo) chart(ctx context.Context, symbol string, q url.Values) (chartResponse, error) {
	var res chartResponse
	status, err := y.get(ctx, "/v8/finance/chart/"+url.PathEscape(strings.ToUpper(symbol)), q, &res)
	if status == http.StatusNotFound || (res.Chart.Error != nil && res.Chart.Error.Code == "Not Found") {
		return res, ErrNotFound
	}
	if err != nil {
		return res, err
	}
	if len(res.Chart.Result) == 0 {
		return res, ErrNotFound
	}
	return res, nil
}

type chartQuote struct {
	Close []*float64 `json:"close"`
}

type searchResponse struct {
	Quotes []struct {
		Symbol    string `json:"symbol"`
		ShortName string `json:"shortname"`
		LongName  string `json:"longname"`
		QuoteType string `json:"quoteType"`
		ExchDisp  string `json:"exchDisp"`
	} `json:"quotes"`
}

func (y *Yahoo) Search(ctx context.Context, query string) ([]Match, error) {
	q := url.Values{"q": {query}, "quotesCount": {"10"}, "newsCount": {"0"}}
	var res searchResponse
	if _, err := y.get(ctx, "/v1/finance/search", q, &res); err != nil {
		return nil, err
	}
	out := []Match{}
	for _, r := range res.Quotes {
		qt := searchType(r.QuoteType)
		if !holdableTypes[qt] {
			continue
		}
		out = append(out, Match{
			Symbol:    strings.ToUpper(r.Symbol),
			Name:      firstNonEmpty(r.LongName, r.ShortName, r.Symbol),
			QuoteType: qt,
			Exchange:  r.ExchDisp,
		})
	}
	return out, nil
}

func searchType(t string) string {
	if t == "MONEY_MARKET" {
		return "MONEYMARKET"
	}
	return t
}

// get performs the request and decodes a JSON body into dst. It returns the
// status code even on failure, so a 404 can be recognised as "unknown symbol".
func (y *Yahoo) get(ctx context.Context, path string, q url.Values, dst any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", yahooUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := y.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: yahoo: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("%w: yahoo: reading response: %v", ErrUnavailable, err)
	}
	// Decode even on a 404: the chart endpoint explains itself in the body.
	jsonErr := json.Unmarshal(body, dst)
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("%w: yahoo: status %d: %s", ErrUnavailable, resp.StatusCode, snippet(body))
	}
	if jsonErr != nil {
		return resp.StatusCode, fmt.Errorf("%w: yahoo: decoding response: %v", ErrUnavailable, jsonErr)
	}
	return resp.StatusCode, nil
}

// closes pairs bar timestamps with their closes, dropping nulls — the bar for a
// trading day still in progress, or a mutual fund's day before its NAV is out.
// Each bar is dated by its day in the exchange's own timezone.
func closes(timestamps []int64, quote []chartQuote, tz string) []Close {
	if len(quote) == 0 {
		return nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc, _ = time.LoadLocation("America/New_York")
	}
	out := []Close{}
	for i, ts := range timestamps {
		if i >= len(quote[0].Close) || quote[0].Close[i] == nil {
			continue
		}
		y, m, d := time.Unix(ts, 0).In(loc).Date()
		out = append(out, Close{Date: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Price: *quote[0].Close[i]})
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// errBadCrumb is quoteSummary refusing the crumb: the session expired, so the
// crumb is fetched again once.
var errBadCrumb = errors.New("yahoo: crumb rejected")

// Yield returns a fund's published yield in percent — SPAXX's 7-day yield, for
// example. A symbol Yahoo has no yield for is ErrNotFound.
func (y *Yahoo) Yield(ctx context.Context, symbol string) (float64, error) {
	pct, err := y.yield(ctx, symbol)
	if errors.Is(err, errBadCrumb) {
		y.crumbMu.Lock()
		y.crumb = ""
		y.crumbMu.Unlock()
		pct, err = y.yield(ctx, symbol)
	}
	return pct, err
}

type quoteSummaryResponse struct {
	QuoteSummary struct {
		Result []struct {
			SummaryDetail struct {
				Yield struct {
					Raw *float64 `json:"raw"`
				} `json:"yield"`
			} `json:"summaryDetail"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"quoteSummary"`
	Finance struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	} `json:"finance"`
}

func (y *Yahoo) yield(ctx context.Context, symbol string) (float64, error) {
	crumb, err := y.sessionCrumb(ctx)
	if err != nil {
		return 0, err
	}
	q := url.Values{"modules": {"summaryDetail"}, "crumb": {crumb}}
	var res quoteSummaryResponse
	status, err := y.get(ctx, "/v10/finance/quoteSummary/"+url.PathEscape(strings.ToUpper(symbol)), q, &res)
	if status == http.StatusUnauthorized ||
		(res.Finance.Error != nil && res.Finance.Error.Code == "Unauthorized") {
		return 0, errBadCrumb
	}
	if status == http.StatusNotFound {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if len(res.QuoteSummary.Result) == 0 {
		return 0, ErrNotFound
	}
	raw := res.QuoteSummary.Result[0].SummaryDetail.Yield.Raw
	if raw == nil || *raw <= 0 || *raw >= 1 {
		return 0, fmt.Errorf("%w: yahoo publishes no yield for %s", ErrNotFound, symbol)
	}
	// Yahoo gives a fraction (0.0333); keep the three decimals a fund quotes.
	return math.Round(*raw*100*1000) / 1000, nil
}

// sessionCrumb returns the cached crumb, or starts a session for one: the
// cookie endpoint sets the session cookie (its own status doesn't matter), and
// getcrumb issues a crumb bound to that cookie.
func (y *Yahoo) sessionCrumb(ctx context.Context) (string, error) {
	y.crumbMu.Lock()
	defer y.crumbMu.Unlock()
	if y.crumb != "" {
		return y.crumb, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.cookieURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", yahooUserAgent)
	resp, err := y.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: yahoo: starting a session: %v", ErrUnavailable, err)
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, y.baseURL+"/v1/test/getcrumb", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", yahooUserAgent)
	resp, err = y.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: yahoo: fetching a crumb: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	crumb := strings.TrimSpace(string(body))
	if resp.StatusCode >= 400 || crumb == "" || strings.ContainsAny(crumb, "{<") {
		return "", fmt.Errorf("%w: yahoo: no crumb (status %d)", ErrUnavailable, resp.StatusCode)
	}
	y.crumb = crumb
	return crumb, nil
}
