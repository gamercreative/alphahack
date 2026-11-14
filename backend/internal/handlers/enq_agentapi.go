package handlers

import (
	"net/http"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

// registerAgentAPI mounts the Wassit agent routes (agent credential only).
func (h *Handlers) registerAgentAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/agent/wakeups", h.auth(agentOnly, h.agentWakeups))
	mux.HandleFunc("POST /api/v1/agent/runs", h.auth(agentOnly, h.agentStartRun))
	mux.HandleFunc("PATCH /api/v1/agent/runs/{id}", h.auth(agentOnly, h.agentFinishRun))
	mux.HandleFunc("POST /api/v1/agent/runs/{id}/tool-calls", h.auth(agentOnly, h.agentRecordToolCall))
	mux.HandleFunc("POST /api/v1/enquiries/{id}/wait", h.auth(agentOnly, h.agentWait))
}

func (h *Handlers) agentWakeups(w http.ResponseWriter, r *http.Request) {
	items, err := h.enq.Wakeups(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handlers) agentStartRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EnquiryID string `json:"enquiry_id"`
		Trigger   string `json:"trigger"`
		Model     string `json:"model"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	run, err := h.enq.StartRun(r.Context(), body.EnquiryID, body.Trigger, body.Model)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, run)
}

func (h *Handlers) agentFinishRun(w http.ResponseWriter, r *http.Request) {
	var in services.FinishRunInput
	if !h.decode(w, r, &in) {
		return
	}
	run, err := h.enq.FinishRun(r.Context(), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, run)
}

func (h *Handlers) agentRecordToolCall(w http.ResponseWriter, r *http.Request) {
	var in services.ToolCallInput
	if !h.decode(w, r, &in) {
		return
	}
	call, err := h.enq.RecordToolCall(r.Context(), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, call)
}

func (h *Handlers) agentWait(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Minutes int `json:"minutes"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.enq.Wait(r.Context(), actor(r), r.PathValue("id"), body.Minutes)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}
