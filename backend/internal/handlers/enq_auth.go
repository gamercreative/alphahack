package handlers

import (
	"net/http"

	httpx "alphahack/internal/http"
)

// registerAuth mounts the Wassit staff login and the "who am I" route.
// Login is public (no auth wrapper); /api/v1/me is staff only.
func (h *Handlers) registerAuth(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("GET /api/v1/me", h.auth(staffOnly, h.me))
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Pin   string `json:"pin"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	res, err := h.enq.Login(r.Context(), in.Email, in.Pin)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	res, err := h.enq.Me(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
