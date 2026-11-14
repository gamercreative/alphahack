package handlers

import (
	"net/http"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

// registerReplies mounts the Wassit inbox, reply, clarification and approval routes.
func (h *Handlers) registerReplies(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/inbox", h.auth(staffOnly, h.listInbox))
	mux.HandleFunc("POST /api/v1/targets/{id}/reply", h.auth(staffOnly, h.replyTarget))
	mux.HandleFunc("POST /api/v1/enquiries/{id}/clarification-request", h.auth(agentOnly, h.requestClarification))
	mux.HandleFunc("POST /api/v1/enquiries/{id}/clarify", h.auth(staffOnly, h.clarifyEnquiry))
	mux.HandleFunc("POST /api/v1/enquiries/{id}/targets/approve", h.auth(staffOnly, h.approveTargets))
}

func (h *Handlers) listInbox(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	items, err := h.enq.Inbox(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) replyTarget(w http.ResponseWriter, r *http.Request) {
	var in services.ReplyInput
	if !h.decode(w, r, &in) {
		return
	}
	t, err := h.enq.Reply(r.Context(), actor(r), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handlers) requestClarification(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Question string `json:"question"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.enq.RequestClarification(r.Context(), actor(r), r.PathValue("id"), body.Question)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}

func (h *Handlers) clarifyEnquiry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Answer string `json:"answer"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.enq.Clarify(r.Context(), actor(r), r.PathValue("id"), body.Answer)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}

func (h *Handlers) approveTargets(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetIDs []string `json:"target_ids"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	targets, err := h.enq.ApproveTargets(r.Context(), actor(r), r.PathValue("id"), body.TargetIDs)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": targets})
}
