package receipts

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/vision"
)

var today = time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)

var household = ledger.ReceiptContext{
	Accounts: []ledger.ReceiptAccount{
		{ID: 1, Type: models.AccountChecking, Mask: "1111"},
		{ID: 2, Type: models.AccountCreditCard, Mask: " 4821 "},
		{ID: 3, Type: models.AccountCash},
		{ID: 4, Type: models.AccountSavings, Mask: "12a4"},
	},
	Categories: []ledger.ReceiptCategory{{ID: 10, Name: "Groceries"}, {ID: 11, Name: "Dining Out"}},
}

func str(s string) *string   { return &s }
func num(f float64) *float64 { return &f }

// good is a receipt that should post by itself. Each case below breaks one thing.
func good() vision.Extraction {
	return vision.Extraction{
		IsReceipt: true, Merchant: str("King Soopers"), PurchaseDate: str("2026-09-19"),
		Currency: str("USD"), TransactionType: "purchase",
		Subtotal: num(79.12), Tax: num(5.08), Total: num(84.2),
		Tender: "card", CardLast4: str("4821"), Category: str("groceries"),
		LineItems: []vision.LineItem{{Description: "MILK", Amount: 4.29}},
	}
}

func TestDecidePosts(t *testing.T) {
	d := Decide(good(), household, today)
	if len(d.Reasons) != 0 || d.Post == nil {
		t.Fatalf("want a post, got reasons %v", d.Reasons)
	}
	p := d.Post
	if p.AccountID != 2 || *p.CategoryID != 10 || p.Amount != 84.2 || p.Date != "2026-09-19" ||
		p.Kind != models.KindExpense || p.Description != "King Soopers" {
		t.Errorf("post = %+v", p)
	}
}

func TestDecideHolds(t *testing.T) {
	cases := map[string]struct {
		edit func(*vision.Extraction)
		want string
	}{
		"not a receipt":      {func(x *vision.Extraction) { x.IsReceipt = false }, models.ReasonNotAReceipt},
		"no total":           {func(x *vision.Extraction) { x.Total = nil }, models.ReasonMissingTotal},
		"zero total":         {func(x *vision.Extraction) { x.Total = num(0.001) }, models.ReasonMissingTotal},
		"return":             {func(x *vision.Extraction) { x.TransactionType = "return" }, models.ReasonLooksLikeReturn},
		"negative total":     {func(x *vision.Extraction) { x.Total = num(-84.2) }, models.ReasonLooksLikeReturn},
		"canadian":           {func(x *vision.Extraction) { x.Currency = str("cad") }, models.ReasonNonUSD},
		"no date":            {func(x *vision.Extraction) { x.PurchaseDate = nil }, models.ReasonMissingDate},
		"garbage date":       {func(x *vision.Extraction) { x.PurchaseDate = str("19/09/26") }, models.ReasonMissingDate},
		"future date":        {func(x *vision.Extraction) { x.PurchaseDate = str("2026-09-22") }, models.ReasonDateOutOfRange},
		"misread year":       {func(x *vision.Extraction) { x.PurchaseDate = str("2016-09-19") }, models.ReasonDateOutOfRange},
		"totals disagree":    {func(x *vision.Extraction) { x.Total = num(94.2) }, models.ReasonTotalsDisagree},
		"unknown category":   {func(x *vision.Extraction) { x.Category = str("Groceries & Stuff") }, models.ReasonNoCategoryMatch},
		"no category":        {func(x *vision.Extraction) { x.Category = nil }, models.ReasonNoCategoryMatch},
		"unknown card":       {func(x *vision.Extraction) { x.CardLast4 = str("9999") }, models.ReasonAccountUnresolved},
		"card without last4": {func(x *vision.Extraction) { x.CardLast4 = nil }, models.ReasonAccountUnresolved},
		"malformed last4":    {func(x *vision.Extraction) { x.CardLast4 = str("12a4") }, models.ReasonAccountUnresolved},
		"other tender":       {func(x *vision.Extraction) { x.Tender = "other" }, models.ReasonAccountUnresolved},
		"invented tender":    {func(x *vision.Extraction) { x.Tender = "crypto" }, models.ReasonAccountUnresolved},
	}
	for name, c := range cases {
		x := good()
		c.edit(&x)
		d := Decide(x, household, today)
		if d.Post != nil {
			t.Errorf("%s: posted, want held for %s", name, c.want)
			continue
		}
		if !slices.Contains(d.Reasons, c.want) {
			t.Errorf("%s: reasons = %v, want %s", name, d.Reasons, c.want)
		}
	}
}

// The oldest date that still posts is exactly 60 days back; today posts too.
func TestDecideDateBounds(t *testing.T) {
	for date, posts := range map[string]bool{
		"2026-09-21": true, "2026-07-23": true, "2026-07-22": false,
	} {
		x := good()
		x.PurchaseDate = &date
		if got := Decide(x, household, today).Post != nil; got != posts {
			t.Errorf("%s: posted = %v, want %v", date, got, posts)
		}
	}
}

func TestDecideCash(t *testing.T) {
	x := good()
	x.Tender, x.CardLast4 = "cash", nil
	d := Decide(x, household, today)
	if d.Post == nil || d.Post.AccountID != 3 {
		t.Fatalf("one cash account should match: %+v %v", d.Post, d.Reasons)
	}

	two := household
	two.Accounts = append(slices.Clone(household.Accounts), ledger.ReceiptAccount{ID: 5, Type: models.AccountCash})
	if d := Decide(x, two, today); d.Post != nil {
		t.Error("two cash accounts is a guess; want held")
	}
}

// Two accounts sharing a last four is ambiguous, and ambiguity is never
// resolved by picking one.
func TestDecideAmbiguousCard(t *testing.T) {
	two := household
	two.Accounts = append(slices.Clone(household.Accounts), ledger.ReceiptAccount{ID: 6, Type: models.AccountChecking, Mask: "4821"})
	if d := Decide(good(), two, today); d.Post != nil || !slices.Contains(d.Reasons, models.ReasonAccountUnresolved) {
		t.Errorf("want held as account_unresolved, got %v", d.Reasons)
	}
}

func TestDecideTip(t *testing.T) {
	x := good()
	x.Tip, x.Total = num(15), num(99.2)
	if d := Decide(x, household, today); d.Post == nil || d.Post.Amount != 99.2 {
		t.Errorf("a tip within the total should post at the total: %v", d.Reasons)
	}
}

// Model output is untrusted text: it is cleaned and bounded before storage.
func TestDecideCleans(t *testing.T) {
	x := good()
	x.Merchant = str("King\x00 Soopers\n\t#123 " + strings.Repeat("x", 300))
	items := make([]vision.LineItem, 150)
	for i := range items {
		items[i] = vision.LineItem{Description: strings.Repeat("y", 500), Amount: 1.005}
	}
	x.LineItems = items
	x.Total = num(84.199999)

	d := Decide(x, household, today)
	m := *d.Fields.Merchant
	if strings.ContainsAny(m, "\x00\n\t") || len([]rune(m)) > maxMerchant || !strings.HasPrefix(m, "King Soopers #123") {
		t.Errorf("merchant = %q", m)
	}
	if *d.Fields.Total != 84.2 {
		t.Errorf("total = %v, want rounded to cents", *d.Fields.Total)
	}
	var stored []vision.LineItem
	if err := json.Unmarshal(d.Fields.LineItems, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != maxLineItems || len([]rune(stored[0].Description)) > maxDescription {
		t.Errorf("line items not bounded: %d items, first %d runes", len(stored), len([]rune(stored[0].Description)))
	}
}

func TestDecideNoMerchant(t *testing.T) {
	x := good()
	x.Merchant = str("   ")
	if d := Decide(x, household, today); d.Post == nil || d.Post.Description != "Receipt" || d.Post.Merchant != nil {
		t.Errorf("want description Receipt and no merchant, got %+v", d.Post)
	}
}

// A category that names its account wins over the card on the receipt, and
// fills in an account the receipt couldn't resolve.
func TestDecideCategoryAccountWins(t *testing.T) {
	rc := household
	visa := 2
	rc.Categories = []ledger.ReceiptCategory{{ID: 12, Name: "Gas", AccountID: &visa}}

	x := good()
	x.Category = str("gas")
	x.CardLast4 = str("1111") // the checking account's debit card
	d := Decide(x, rc, today)
	if d.Post == nil || d.Post.AccountID != visa {
		t.Fatalf("want a post to the Visa, got %+v reasons %v", d.Post, d.Reasons)
	}

	x.CardLast4 = str("9999")
	if d := Decide(x, rc, today); d.Post == nil || d.Post.AccountID != visa {
		t.Errorf("unknown card: want a post to the Visa, got %+v reasons %v", d.Post, d.Reasons)
	}

	// Held for another reason, the review form still starts on the Visa.
	x.PurchaseDate = nil
	if d := Decide(x, rc, today); d.AccountID == nil || *d.AccountID != visa {
		t.Errorf("held receipt account = %v, want the Visa", d.AccountID)
	}
}

// A checking account with two debit cards matches a receipt from either.
func TestDecideMatchesAnyCardOnAnAccount(t *testing.T) {
	rc := household
	rc.Accounts = []ledger.ReceiptAccount{
		{ID: 1, Type: models.AccountChecking, Mask: "1111, 2222"},
		{ID: 2, Type: models.AccountCreditCard, Mask: "4821"},
		{ID: 4, Type: models.AccountSavings, Mask: "12a4, 3333x"},
	}
	for _, last4 := range []string{"1111", "2222"} {
		x := good()
		x.CardLast4 = str(last4)
		if d := Decide(x, rc, today); d.Post == nil || d.Post.AccountID != 1 {
			t.Errorf("card %s: want checking, got %+v reasons %v", last4, d.Post, d.Reasons)
		}
	}
	// A mangled mask matches nothing rather than a piece of itself.
	x := good()
	x.CardLast4 = str("3333")
	if d := Decide(x, rc, today); d.Post != nil {
		t.Errorf("malformed mask matched: %+v", d.Post)
	}
}
