package handlers

import (
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
	"github.com/cwnelson/fangorn/internal/receipts"
)

const (
	// maxReceiptBytes caps one photo. The app downscales before uploading, so a
	// real upload is a fraction of this; anything bigger did not come from it.
	maxReceiptBytes = 5 << 20

	// inlineBudget is how long an upload waits for its receipt to be read before
	// answering "still working". It is measured from the start of the request,
	// so a slow upload leaves less of it, and it sits well inside the server's
	// 15s WriteTimeout.
	inlineBudget = 10 * time.Second
)

// acceptedImageTypes are what the browser's re-encode produces. HEIC and
// anything else is refused rather than stored: the model and every browser that
// shows it back need one of these.
var acceptedImageTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true,
}

// ReceiptHandler serves receipt uploads and review. It is separate from
// LedgerHandler because it also needs the receipts processor.
type ReceiptHandler struct {
	svc         *ledger.Service
	proc        *receipts.Processor
	householdID int
}

func NewReceiptHandler(svc *ledger.Service, proc *receipts.Processor, householdID int) *ReceiptHandler {
	return &ReceiptHandler{svc: svc, proc: proc, householdID: householdID}
}

func (h *ReceiptHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/receipts", h.List)
	mux.HandleFunc("POST /api/receipts", h.Upload)
	mux.HandleFunc("GET /api/receipts/{id}", h.Get)
	mux.HandleFunc("GET /api/receipts/{id}/image", h.Image)
	mux.HandleFunc("POST /api/receipts/{id}/post", h.Post)
	mux.HandleFunc("POST /api/receipts/{id}/retry", h.Retry)
	mux.HandleFunc("DELETE /api/receipts/{id}", h.Delete)
}

type uploadResponse struct {
	Receipt models.Receipt `json:"receipt"`
	// Duplicate means this exact photo was uploaded before, and Receipt is that
	// earlier upload.
	Duplicate bool `json:"duplicate"`
	// Enabled is false when automatic reading is turned off, so the app can say
	// the receipt is waiting for a person rather than being read.
	Enabled bool `json:"enabled"`
}

// Upload stores a photo and tries to read it before answering. It answers 201
// when the receipt was finished (posted or held for review) and 202 when it is
// still being read, in which case the app polls GET /api/receipts/{id}.
func (h *ReceiptHandler) Upload(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	img, ok := readReceiptImage(w, r)
	if !ok {
		return
	}
	res, err := h.ingest(r, h.householdID, img, start)
	if errors.Is(err, errNotAnImage) {
		writeError(w, http.StatusBadRequest, "That file isn't a JPEG, PNG or WebP image")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	status := http.StatusCreated
	switch {
	case res.Duplicate:
		status = http.StatusOK
	case res.Receipt.Status == models.ReceiptPending || res.Receipt.Status == models.ReceiptProcessing:
		status = http.StatusAccepted
	}
	writeJSON(w, status, res)
}

var errNotAnImage = errors.New("not a JPEG, PNG or WebP image")

// ingest stores a photo for a household and gives it until inlineBudget after
// start to be read. The app's upload and the iPhone Shortcut both come through
// here, so a receipt behaves the same whichever way it arrived.
func (h *ReceiptHandler) ingest(r *http.Request, householdID int, img []byte, start time.Time) (uploadResponse, error) {
	mediaType := http.DetectContentType(img)
	if !acceptedImageTypes[mediaType] {
		return uploadResponse{}, errNotAnImage
	}
	sum := sha256.Sum256(img)

	rec, created, err := h.svc.CreateReceipt(r.Context(), householdID, ledger.NewReceipt{
		Image: img, MediaType: mediaType, SHA256: sum[:],
	})
	if err != nil {
		return uploadResponse{}, err
	}
	if !created {
		return uploadResponse{Receipt: rec, Duplicate: true, Enabled: h.proc.Enabled()}, nil
	}

	if done := h.proc.Start(householdID, rec.ID); done != nil {
		wait := time.NewTimer(time.Until(start.Add(inlineBudget)))
		select {
		case <-done:
		case <-wait.C:
		case <-r.Context().Done():
		}
		wait.Stop()
	}

	rec, err = h.svc.GetReceipt(r.Context(), householdID, rec.ID)
	if err != nil {
		return uploadResponse{}, err
	}
	return uploadResponse{Receipt: rec, Enabled: h.proc.Enabled()}, nil
}

// readReceiptImage reads the single "image" part of a multipart upload into
// memory. It deliberately does not use ParseMultipartForm, which spills large
// parts to temp files — the container has no writable scratch space, and there
// is no reason for a receipt to touch disk.
func readReceiptImage(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	// A little headroom over the image cap for the multipart framing.
	r.Body = http.MaxBytesReader(w, r.Body, maxReceiptBytes+64<<10)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "Expected a multipart upload with an image field")
		return nil, false
	}

	var img []byte
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			uploadError(w, err)
			return nil, false
		}
		if part.FormName() != "image" {
			writeError(w, http.StatusBadRequest, "Unexpected upload field "+part.FormName())
			return nil, false
		}
		if img != nil {
			writeError(w, http.StatusBadRequest, "Upload one image at a time")
			return nil, false
		}
		img, err = io.ReadAll(io.LimitReader(part, maxReceiptBytes+1))
		if err != nil {
			uploadError(w, err)
			return nil, false
		}
		if len(img) > maxReceiptBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "That photo is too large")
			return nil, false
		}
	}
	if len(img) == 0 {
		writeError(w, http.StatusBadRequest, "No image in the upload")
		return nil, false
	}
	return img, true
}

func uploadError(w http.ResponseWriter, err error) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, "That photo is too large")
		return
	}
	writeError(w, http.StatusBadRequest, "Could not read the upload")
}

func (h *ReceiptHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "", models.ReceiptPending, models.ReceiptProcessing, models.ReceiptNeedsReview, models.ReceiptPosted:
	default:
		writeError(w, http.StatusBadRequest, "Invalid status")
		return
	}
	list, err := h.svc.ListReceipts(r.Context(), h.householdID, status, queryInt(r, "limit"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *ReceiptHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	rec, err := h.svc.GetReceipt(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (h *ReceiptHandler) Image(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	img, mediaType, err := h.svc.ReceiptImage(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The bytes behind an id never change, but they are a family's receipts:
	// cache in this browser only.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	w.Write(img)
}

// Post turns a receipt held for review into a transaction from what the person
// confirmed.
func (h *ReceiptHandler) Post(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	var in ledger.TransactionInput
	if !decode(w, r, &in) {
		return
	}
	txn, err := h.svc.PostHeldReceipt(r.Context(), h.householdID, id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, txn)
}

// Retry sends a held receipt back to be read again and starts reading it. It
// answers straight away; the app polls for the result.
func (h *ReceiptHandler) Retry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	rec, err := h.svc.RetryReceipt(r.Context(), h.householdID, id)
	if err != nil {
		fail(w, err)
		return
	}
	h.proc.Start(h.householdID, id)
	writeJSON(w, http.StatusOK, rec)
}

func (h *ReceiptHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteReceipt(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
