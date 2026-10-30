package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/cwnelson/fangorn/internal/assistant"
	"github.com/cwnelson/fangorn/internal/ledger"
	"github.com/cwnelson/fangorn/internal/models"
)

// The assistant's instructions and files: what a claude.ai Project would hold,
// kept here so Ask has it with every question.

// knowledge loads what a chat started with.
func (h *ChatHandler) knowledge(ctx context.Context, cc *models.ChatContext) (assistant.Knowledge, error) {
	var kn assistant.Knowledge
	if cc == nil {
		return kn, nil
	}
	kn.Instructions = cc.Instructions
	files, err := h.svc.ContextFiles(ctx, h.householdID, cc.FileIDs)
	if err != nil {
		return kn, err
	}
	for _, f := range files {
		kn.Files = append(kn.Files, assistant.File{Name: f.Name, MediaType: f.MediaType, Data: f.Data})
	}
	return kn, nil
}

func (h *ChatHandler) GetContext(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.GetAssistantContext(r.Context(), h.householdID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *ChatHandler) SetInstructions(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Instructions string `json:"instructions"`
	}
	if !decode(w, r, &in) {
		return
	}
	text, err := h.svc.SetAssistantInstructions(r.Context(), h.householdID, in.Instructions)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"instructions": text})
}

// textExtensions are the files taken as plain text, provided they are UTF-8.
var textExtensions = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".csv": true, ".tsv": true, ".json": true,
}

const unsupportedFile = "Attach a PDF or a plain text file (.txt, .md, .csv). Save a Word document or spreadsheet as PDF or CSV first."

// AddFile takes one multipart "file" part, read in memory like a receipt.
func (h *ChatHandler) AddFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, ledger.MaxAssistantFileBytes+64<<10)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "Expected a multipart upload with a file field")
		return
	}
	var name string
	var data []byte
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			fileUploadError(w, err)
			return
		}
		if part.FormName() != "file" || data != nil {
			writeError(w, http.StatusBadRequest, "Upload one file at a time")
			return
		}
		name = part.FileName()
		data, err = io.ReadAll(io.LimitReader(part, ledger.MaxAssistantFileBytes+1))
		if err != nil {
			fileUploadError(w, err)
			return
		}
		if len(data) > ledger.MaxAssistantFileBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "That file is too large")
			return
		}
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "No file in the upload")
		return
	}

	// Decided from the bytes, not what the browser claims.
	var mediaType string
	switch {
	case strings.HasPrefix(http.DetectContentType(data), "application/pdf"):
		mediaType = "application/pdf"
	case textExtensions[strings.ToLower(filepath.Ext(name))] && utf8.Valid(data):
		mediaType = "text/plain"
	default:
		writeError(w, http.StatusBadRequest, unsupportedFile)
		return
	}

	f, err := h.svc.AddAssistantFile(r.Context(), h.householdID, filepath.Base(name), mediaType, data)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func fileUploadError(w http.ResponseWriter, err error) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, "That file is too large")
		return
	}
	writeError(w, http.StatusBadRequest, "Could not read the upload")
}

func (h *ChatHandler) RemoveFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.RemoveAssistantFile(r.Context(), h.householdID, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
