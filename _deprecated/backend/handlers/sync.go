package handlers

import (
	"net/http"

	"github.com/cwnelson/fangorn/internal/services"
)

type SyncHandler struct {
	sync *services.SyncService
}

func NewSyncHandler(sync *services.SyncService) *SyncHandler {
	return &SyncHandler{sync: sync}
}

func (h *SyncHandler) Sync(w http.ResponseWriter, r *http.Request) {
	if err := h.sync.SyncAll(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Sync failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}
