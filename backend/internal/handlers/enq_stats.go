package handlers

import (
	"net/http"
	"time"

	httpx "alphahack/internal/http"
)

// registerStats mounts the Wassit stats and enquiry event stream (human pharmacists only).
func (h *Handlers) registerStats(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/stats", h.auth(staffOnly, h.getStats))
	mux.HandleFunc("GET /api/v1/enquiries/{id}/stream", h.auth(staffOnly, h.streamEnquiry))
}

func (h *Handlers) getStats(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	res, err := h.enq.Stats(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// streamEnquiry sends every event of the enquiry, then each new one as it is written.
// Access is checked before the stream starts, so a denied caller gets a normal JSON error.
func (h *Handlers) streamEnquiry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a := actor(r)

	first, err := h.enq.EventsSince(r.Context(), a, id, 0)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	sse := httpx.NewSSE(w)
	var lastSeq int64
	for _, ev := range first {
		if err := sse.Send("enquiry_event", ev); err != nil {
			return
		}
		lastSeq = ev.Seq
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			next, err := h.enq.EventsSince(r.Context(), a, id, lastSeq)
			if err != nil {
				_, body := h.errorBody(r, err)
				_ = sse.Send("error", body)
				return
			}
			for _, ev := range next {
				if err := sse.Send("enquiry_event", ev); err != nil {
					return
				}
				lastSeq = ev.Seq
			}
		}
	}
}
