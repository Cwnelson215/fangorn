// Package receipts turns uploaded receipt photos into transactions.
//
// The work is called from two places, like the price refresher: the upload
// request, which tries to finish while the phone waits, and the scheduler, which
// finishes whatever the upload could not. A model call can take longer than the
// server's 15s write timeout, and a restart can cut a call off midway, so
// neither caller alone is enough. The ledger's claim-and-fence makes it safe for
// both to try: at most one of them pays for a model call per claim, and at most
// one transaction is ever written per receipt.
package receipts

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/vision"
)

const (
	// Lease is how long a claim holds a receipt before another caller may take
	// it over. It must outlast callTimeout, or a slow call that eventually
	// succeeds would find its claim already taken.
	Lease = 3 * time.Minute

	// callTimeout bounds one model call.
	callTimeout = 90 * time.Second

	// maxFailures is how many passing failures (overload, rate limit, network) a
	// receipt gets before it is handed to a person instead.
	maxFailures = 5

	// inlineSlots bounds the model calls started by upload requests. Past that,
	// uploads are stored and left for the scheduler.
	inlineSlots = 4
)

type Processor struct {
	svc *ledger.Service
	ext vision.Extractor
	now func() time.Time

	// base is the parent of every inline run. Cancelling it at shutdown makes
	// in-flight calls give their claims back instead of being killed mid-write.
	base       context.Context
	cancelBase context.CancelFunc
	sem        chan struct{}
	wg         sync.WaitGroup
}

// New builds a processor. A nil extractor turns reading off: photos are still
// stored, and every receipt waits for a person.
func New(svc *ledger.Service, ext vision.Extractor) *Processor {
	base, cancel := context.WithCancel(context.Background())
	return &Processor{
		svc: svc, ext: ext, now: time.Now,
		base: base, cancelBase: cancel, sem: make(chan struct{}, inlineSlots),
	}
}

// Enabled reports whether receipts are read automatically.
func (p *Processor) Enabled() bool { return p != nil && p.ext != nil }

// Process reads and resolves one receipt if it can claim it. It returns nil
// when there was nothing to do (someone else has it, or it is finished), and a
// context error when ctx ended before it could finish — in which case the claim
// has been handed back.
func (p *Processor) Process(ctx context.Context, householdID, receiptID int) error {
	claim, ok, err := p.svc.ClaimReceipt(ctx, householdID, receiptID, Lease)
	if err != nil || !ok {
		return err
	}

	if p.ext == nil {
		return p.finish(ctx, householdID, claim, ledger.ReceiptOutcome{
			Reasons: []string{models.ReasonExtractionDisabled},
		})
	}

	household, err := p.svc.GetHousehold(ctx, householdID)
	if err != nil {
		return p.giveBack(ctx, householdID, claim, err)
	}
	rc, err := p.svc.ReceiptContext(ctx, householdID)
	if err != nil {
		return p.giveBack(ctx, householdID, claim, err)
	}
	today := household.Today()
	names := make([]string, len(rc.Categories))
	for i, c := range rc.Categories {
		names[i] = c.Name
	}

	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	x, meta, err := p.ext.Extract(callCtx, claim.Image, claim.MediaType,
		vision.Hints{Today: today.Format(models.DateOnly), Categories: names})
	cancel()
	if err != nil {
		return p.failed(ctx, householdID, claim, err)
	}
	log.Printf("receipts: read receipt %d with %s (%d in / %d out tokens)",
		receiptID, meta.Model, meta.InputTokens, meta.OutputTokens)

	d := Decide(x, rc, today)
	if meta.Model != "" {
		d.Fields.Model = &meta.Model
	}
	if d.Post != nil {
		dup, err := p.svc.SimilarExpenseExists(ctx, householdID, d.Post.AccountID, d.Post.Date, d.Post.Amount)
		if err != nil {
			return p.giveBack(ctx, householdID, claim, err)
		}
		if dup {
			d.Reasons = append(d.Reasons, models.ReasonPossibleDuplicate)
			d.Post = nil
		}
	}

	return p.finish(ctx, householdID, claim, ledger.ReceiptOutcome{
		Fields: &d.Fields, AccountID: d.AccountID, CategoryID: d.CategoryID,
		Reasons: d.Reasons, Post: d.Post,
	})
}

func (p *Processor) finish(ctx context.Context, householdID int, claim ledger.Claim, out ledger.ReceiptOutcome) error {
	err := p.svc.FinishReceipt(context.WithoutCancel(ctx), householdID, claim, out)
	if errors.Is(err, ledger.ErrLostClaim) {
		// Re-claimed after our lease ran out, or deleted. The other side owns it.
		log.Printf("receipts: receipt %d changed hands before it could be finished", claim.ReceiptID)
		return nil
	}
	return err
}

// failed records a failed model call.
//
// A call that failed because ctx ended says nothing about the receipt — the
// server is shutting down, or the scheduler ran out of budget — so it is handed
// back uncounted, the same rule the price refresher uses for its backoff.
func (p *Processor) failed(ctx context.Context, householdID int, claim ledger.Claim, err error) error {
	if ctx.Err() != nil {
		return p.giveBack(ctx, householdID, claim, ctx.Err())
	}
	log.Printf("receipts: receipt %d: %v", claim.ReceiptID, err)

	hold := ""
	switch {
	case errors.Is(err, vision.ErrUnreadable):
		hold = models.ReasonUnreadable
	case claim.Failures+1 >= maxFailures:
		hold = models.ReasonExtractionFailed
	}
	// 2, 4, 8, 16 minutes: long enough to ride out an overload, short enough that
	// a receipt is still read the same hour.
	retryAfter := p.now().Add(time.Duration(math.Pow(2, float64(claim.Failures+1))) * time.Minute)

	rerr := p.svc.RecordReceiptFailure(context.WithoutCancel(ctx), householdID, claim, err.Error(), hold, retryAfter)
	if rerr != nil && !errors.Is(rerr, ledger.ErrLostClaim) {
		return fmt.Errorf("recording failure for receipt %d: %w", claim.ReceiptID, rerr)
	}
	return nil
}

// giveBack releases a claim after something other than the model went wrong,
// and returns cause.
func (p *Processor) giveBack(ctx context.Context, householdID int, claim ledger.Claim, cause error) error {
	if err := p.svc.ReleaseReceipt(context.WithoutCancel(ctx), householdID, claim); err != nil {
		log.Printf("receipts: releasing receipt %d: %v", claim.ReceiptID, err)
	}
	return cause
}

// Start processes a receipt in the background for an upload request, and
// returns a channel closed when it is done. It returns nil when every inline
// slot is busy; the receipt is then left for the scheduler.
//
// The run is detached from the request on purpose. If the phone stops waiting,
// the model call it already paid for should still land.
func (p *Processor) Start(householdID, receiptID int) <-chan struct{} {
	select {
	case p.sem <- struct{}{}:
	default:
		return nil
	}
	done := make(chan struct{})
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer close(done)
		defer func() { <-p.sem }()

		ctx, cancel := context.WithTimeout(p.base, Lease-30*time.Second)
		defer cancel()
		if err := p.Process(ctx, householdID, receiptID); err != nil && ctx.Err() == nil {
			log.Printf("receipts: processing receipt %d: %v", receiptID, err)
		}
	}()
	return done
}

// Wait blocks until background runs finish or ctx ends. If ctx ends first, the
// runs are cancelled and given a moment to hand their claims back, so the
// scheduler can pick them up on the next boot.
func (p *Processor) Wait(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-ctx.Done():
	}
	p.cancelBase()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}
