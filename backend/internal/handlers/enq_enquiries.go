package handlers

import (
	"math"
	"net/http"
	"strconv"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

// registerEnquiries mounts the enquiry routes (see CONTRACT.md, Endpoints).
func (h *Handlers) registerEnquiries(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/enquiries", h.auth(staffOnly, h.createEnquiry))
	mux.HandleFunc("GET /api/v1/enquiries", h.auth(staffOnly, h.listEnquiries))
	mux.HandleFunc("GET /api/v1/enquiries/{id}", h.auth(agentOK, h.getEnquiryDetail))
	mux.HandleFunc("POST /api/v1/enquiries/{id}/medication", h.auth(agentOK, h.setEnquiryMedication))
	mux.HandleFunc("GET /api/v1/enquiries/{id}/candidates", h.auth(agentOK, h.listEnquiryCandidates))
	mux.HandleFunc("POST /api/v1/enquiries/{id}/targets", h.auth(agentOK, h.addEnquiryTargets))
	mux.HandleFunc("POST /api/v1/enquiries/{id}/close", h.auth(agentOK, h.closeEnquiry))
}

func (h *Handlers) createEnquiry(w http.ResponseWriter, r *http.Request) {
	var in services.CreateEnquiryInput
	if !h.decode(w, r, &in) {
		return
	}
	e, err := h.enq.CreateEnquiry(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, e)
}

func (h *Handlers) listEnquiries(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	items, err := h.enq.ListEnquiries(r.Context(), actor(r), q.Get("scope"), q.Get("status"), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) getEnquiryDetail(w http.ResponseWriter, r *http.Request) {
	d, err := h.enq.GetEnquiryDetail(r.Context(), actor(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handlers) setEnquiryMedication(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MedicationID string `json:"medication_id"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	e, err := h.enq.SetMedication(r.Context(), actor(r), r.PathValue("id"), in.MedicationID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}

func (h *Handlers) listEnquiryCandidates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			httpx.Error(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "limit must be a non-negative integer")
			return
		}
		limit = n
	}
	radius := 0.0
	if raw := q.Get("radius_km"); raw != "" {
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
			httpx.Error(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "radius_km must be a non-negative number")
			return
		}
		radius = f
	}
	items, err := h.enq.Candidates(r.Context(), actor(r), r.PathValue("id"), limit, radius)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handlers) addEnquiryTargets(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BranchIDs []string `json:"branch_ids"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	targets, err := h.enq.AddTargets(r.Context(), actor(r), r.PathValue("id"), in.BranchIDs)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": targets})
}

func (h *Handlers) closeEnquiry(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Outcome  string  `json:"outcome"`
		BranchID *string `json:"branch_id"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	e, err := h.enq.CloseEnquiry(r.Context(), actor(r), r.PathValue("id"), in.Outcome, in.BranchID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}
