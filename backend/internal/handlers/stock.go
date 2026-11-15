package handlers

import (
	"net/http"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

// registerStock mounts the staff stock update. Staff only (rule 2: the agent
// never marks stock); the service also checks it is the staff's own branch.
func (h *Handlers) registerStock(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/v1/branches/{id}/inventory/{medication_id}", h.auth(staffOnly, h.setStock))
}

func (h *Handlers) setStock(w http.ResponseWriter, r *http.Request) {
	var in services.SetStockInput
	if !h.decode(w, r, &in) {
		return
	}
	b, err := h.stock.SetStock(r.Context(), actor(r), r.PathValue("id"), r.PathValue("medication_id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, b)
}
