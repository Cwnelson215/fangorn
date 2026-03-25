package handlers

import "net/http"

type ConfigHandler struct {
	tellerAppID string
}

func NewConfigHandler(tellerAppID string) *ConfigHandler {
	return &ConfigHandler{tellerAppID: tellerAppID}
}

func (h *ConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"teller_app_id": h.tellerAppID,
	})
}
