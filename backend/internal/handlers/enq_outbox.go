package handlers

import (
	"net/http"

	httpx "alphahack/internal/http"
)

// registerOutbox mounts the Wassit outbox routes (send, cancel, follow-up, no-response, notify, undo).
// Staff and the agent may both call these; canAct in the service enforces the requester side.
func (h *Handlers) registerOutbox(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/enquiries/{id}/send", h.auth(agentOK, h.sendEnquiry))
	mux.HandleFunc("POST /api/v1/messages/{id}/cancel", h.auth(agentOK, h.cancelMessage))
	mux.HandleFunc("POST /api/v1/targets/{id}/follow-up", h.auth(agentOK, h.followUpTarget))
	mux.HandleFunc("POST /api/v1/targets/{id}/no-response", h.auth(agentOK, h.markTargetNoResponse))
	mux.HandleFunc("POST /api/v1/enquiries/{id}/notify", h.auth(agentOK, h.notifyRequester))
	mux.HandleFunc("POST /api/v1/events/{id}/undo", h.auth(agentOK, h.undoEvent))
}

func (h *Handlers) sendEnquiry(w http.ResponseWriter, r *http.Request) {
	msgs, err := h.enq.Send(r.Context(), actor(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": msgs})
}

func (h *Handlers) cancelMessage(w http.ResponseWriter, r *http.Request) {
	msg, err := h.enq.CancelMessage(r.Context(), actor(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, msg)
}

func (h *Handlers) followUpTarget(w http.ResponseWriter, r *http.Request) {
	msg, err := h.enq.FollowUp(r.Context(), actor(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, msg)
}

func (h *Handlers) markTargetNoResponse(w http.ResponseWriter, r *http.Request) {
	t, err := h.enq.MarkNoResponse(r.Context(), actor(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handlers) notifyRequester(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body string `json:"body"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	msg, err := h.enq.Notify(r.Context(), actor(r), r.PathValue("id"), in.Body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, msg)
}

func (h *Handlers) undoEvent(w http.ResponseWriter, r *http.Request) {
	ev, err := h.enq.Undo(r.Context(), actor(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, ev)
}
