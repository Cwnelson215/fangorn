package receipts

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/vision"
)

// maxAge is how far back a receipt's date may be and still post on its own. A
// receipt from months ago is either a shoebox being worked through — worth a
// look — or a misread year.
const maxAge = 60 * 24 * time.Hour

const (
	maxMerchant    = 100
	maxDescription = 200
	maxLineItems   = 100
)

var (
	last4Pattern    = regexp.MustCompile(`^\d{4}$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Decision is what to do with one extraction.
type Decision struct {
	Fields     ledger.ReceiptFields
	AccountID  *int
	CategoryID *int
	// Reasons is empty exactly when Post is set.
	Reasons []string
	Post    *ledger.TransactionInput
}

// Decide checks an extraction against the household and decides whether it can
// post by itself. It is pure — no database, no clock — so every rule here is
// covered by a table test.
//
// Nothing in x is trusted. It is the model's reading of a photo that anyone could
// have written anything on, so every value is cleaned, bounded, and matched
// exactly against what the household already has. A category or account is only
// ever chosen from the household's own lists, never created or guessed at.
func Decide(x vision.Extraction, rc ledger.ReceiptContext, today time.Time) Decision {
	var d Decision
	f := &d.Fields
	hold := func(reason string) { d.Reasons = append(d.Reasons, reason) }
	var categoryAccount *int

	f.Merchant = cleanPtr(x.Merchant, maxMerchant)
	f.CategorySuggested = cleanPtr(x.Category, maxMerchant)

	if x.Currency != nil {
		c := strings.ToUpper(strings.TrimSpace(*x.Currency))
		if currencyPattern.MatchString(c) {
			f.Currency = &c
		}
	}

	txnType := "purchase"
	if x.TransactionType == "return" {
		txnType = "return"
	}
	f.TxnType = &txnType

	tender := "unknown"
	switch x.Tender {
	case "card", "cash", "other":
		tender = x.Tender
	}
	f.Tender = &tender

	if x.CardLast4 != nil {
		if l4 := strings.TrimSpace(*x.CardLast4); last4Pattern.MatchString(l4) {
			f.CardLast4 = &l4
		}
	}

	f.Subtotal = cents(x.Subtotal)
	f.Tax = cents(x.Tax)
	f.Tip = cents(x.Tip)
	f.Total = cents(x.Total)
	f.LineItems = lineItems(x.LineItems)

	if !x.IsReceipt {
		hold(models.ReasonNotAReceipt)
	}

	// A negative total is a return however it was labelled. The stored total is
	// the magnitude either way; the kind carries the direction.
	isReturn := txnType == "return"
	if f.Total != nil && *f.Total < 0 {
		isReturn = true
		abs := -*f.Total
		f.Total = &abs
	}
	if f.Total == nil || *f.Total < 0.01 {
		hold(models.ReasonMissingTotal)
	}
	if isReturn {
		hold(models.ReasonLooksLikeReturn)
	}

	if f.Currency != nil && *f.Currency != "USD" {
		hold(models.ReasonNonUSD)
	}

	var date time.Time
	if x.PurchaseDate != nil {
		if t, err := models.ParseDate(strings.TrimSpace(*x.PurchaseDate)); err == nil {
			date = t
			s := t.Format(models.DateOnly)
			f.PurchasedOn = &s
		}
	}
	switch {
	case f.PurchasedOn == nil:
		hold(models.ReasonMissingDate)
	case date.After(today) || date.Before(today.Add(-maxAge)):
		hold(models.ReasonDateOutOfRange)
	}

	if f.Subtotal != nil && f.Tax != nil && f.Total != nil {
		sum := *f.Subtotal + *f.Tax
		if f.Tip != nil {
			sum += *f.Tip
		}
		if math.Abs(sum-*f.Total) > 0.02 {
			hold(models.ReasonTotalsDisagree)
		}
	}

	if f.CategorySuggested != nil {
		want := strings.ToLower(*f.CategorySuggested)
		for _, c := range rc.Categories {
			if strings.ToLower(strings.TrimSpace(c.Name)) == want {
				id := c.ID
				d.CategoryID = &id
				categoryAccount = c.AccountID
				break
			}
		}
	}
	if d.CategoryID == nil {
		hold(models.ReasonNoCategoryMatch)
	}

	// A category that names its account wins over the card on the receipt:
	// the household has said where that spending goes.
	if categoryAccount != nil {
		id := *categoryAccount
		d.AccountID = &id
	} else {
		d.AccountID = matchAccount(tender, f.CardLast4, rc.Accounts)
	}
	if d.AccountID == nil {
		hold(models.ReasonAccountUnresolved)
	}

	if len(d.Reasons) > 0 {
		return d
	}
	desc := "Receipt"
	if f.Merchant != nil {
		desc = *f.Merchant
	}
	d.Post = &ledger.TransactionInput{
		AccountID:   *d.AccountID,
		Date:        *f.PurchasedOn,
		Amount:      *f.Total,
		Kind:        models.KindExpense,
		Description: desc,
		Merchant:    f.Merchant,
		CategoryID:  d.CategoryID,
	}
	return d
}

// matchAccount finds the one account a receipt was paid from, or nil. A card
// matches on its last four digits; cash matches the household's cash account.
// Anything short of exactly one candidate is nil — posting to the wrong account
// is worse than asking.
func matchAccount(tender string, last4 *string, accounts []ledger.ReceiptAccount) *int {
	var found []int
	switch {
	case tender == "card" && last4 != nil:
		for _, a := range accounts {
			if m := strings.TrimSpace(a.Mask); last4Pattern.MatchString(m) && m == *last4 {
				found = append(found, a.ID)
			}
		}
	case tender == "cash":
		for _, a := range accounts {
			if a.Type == models.AccountCash {
				found = append(found, a.ID)
			}
		}
	}
	if len(found) != 1 {
		return nil
	}
	return &found[0]
}

func lineItems(items []vision.LineItem) []byte {
	if len(items) > maxLineItems {
		items = items[:maxLineItems]
	}
	out := make([]vision.LineItem, 0, len(items))
	for _, it := range items {
		out = append(out, vision.LineItem{
			Description: clean(it.Description, maxDescription),
			Quantity:    it.Quantity,
			Amount:      math.Round(it.Amount*100) / 100,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
}

func cents(v *float64) *float64 {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return nil
	}
	r := math.Round(*v*100) / 100
	return &r
}

// clean strips control characters, collapses runs of whitespace and truncates
// to max runes.
func clean(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max]))
	}
	return s
}

func cleanPtr(s *string, max int) *string {
	if s == nil {
		return nil
	}
	c := clean(*s, max)
	if c == "" {
		return nil
	}
	return &c
}
