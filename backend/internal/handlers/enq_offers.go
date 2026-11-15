package handlers

import (
	"net/http"
	"strconv"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

// registerOffers mounts structured offers and simulated-call speech.
func (h *Handlers) registerOffers(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/targets/{id}/offers", h.auth(agentOK, h.recordOffer))
	mux.HandleFunc("GET /api/v1/enquiries/{id}/offers", h.auth(agentOK, h.listOffers))
	mux.HandleFunc("POST /api/v1/speech", h.auth(agentOK, h.speak))
}

func (h *Handlers) recordOffer(w http.ResponseWriter, r *http.Request) {
	var in services.OfferInput
	if !h.decode(w, r, &in) {
		return
	}
	o, err := h.enq.RecordOffer(r.Context(), actor(r), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, o)
}

func (h *Handlers) listOffers(w http.ResponseWriter, r *http.Request) {
	offers, err := h.enq.ListOffers(r.Context(), actor(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": offers})
}

// speak answers with the audio bytes (audio/wav or audio/mpeg), not JSON.
func (h *Handlers) speak(w http.ResponseWriter, r *http.Request) {
	var in services.SpeechInput
	if !h.decode(w, r, &in) {
		return
	}
	a, err := h.enq.Speak(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(a.Data)))
	w.Header().Set("X-Speech-Voice", a.Voice)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(a.Data)
}
