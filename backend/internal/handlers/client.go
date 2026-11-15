package handlers

import (
	"net/http"
	"time"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

// registerClient mounts the DOSYN client routes: login (public), me and tickets
// (client tokens only; staff -> 403 forbidden, agent -> 403 agent_forbidden).
func (h *Handlers) registerClient(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/client/auth/login", h.clientLogin)
	mux.HandleFunc("GET /api/v1/client/me", h.auth(clientOnly, h.clientMe))
	mux.HandleFunc("POST /api/v1/client/tickets", h.auth(clientOnly, h.createTicket))
	mux.HandleFunc("GET /api/v1/client/tickets", h.auth(clientOnly, h.listTickets))
	mux.HandleFunc("GET /api/v1/client/tickets/{id}", h.auth(clientOnly, h.getTicket))
	mux.HandleFunc("GET /api/v1/client/tickets/{id}/stream", h.auth(clientOnly, h.streamTicket))
	mux.HandleFunc("POST /api/v1/client/tickets/{id}/reserve", h.auth(clientOnly, h.reserveTicket))
}

func (h *Handlers) clientLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone string `json:"phone"`
		Pin   string `json:"pin"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	res, err := h.enq.ClientLogin(r.Context(), in.Phone, in.Pin)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handlers) clientMe(w http.ResponseWriter, r *http.Request) {
	res, err := h.enq.ClientMe(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handlers) createTicket(w http.ResponseWriter, r *http.Request) {
	var in services.CreateTicketInput
	if !h.decode(w, r, &in) {
		return
	}
	t, err := h.enq.CreateTicket(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

func (h *Handlers) listTickets(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	items, err := h.enq.ListTickets(r.Context(), actor(r), r.URL.Query().Get("status"), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) getTicket(w http.ResponseWriter, r *http.Request) {
	d, err := h.enq.GetTicket(r.Context(), actor(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handlers) reserveTicket(w http.ResponseWriter, r *http.Request) {
	var in services.ReserveInput
	if !h.decode(w, r, &in) {
		return
	}
	t, err := h.enq.Reserve(r.Context(), actor(r), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

// streamTicket sends every thread row, then each new row (by id) once a second.
// The computed best_offer row gets a new id whenever its content changes.
func (h *Handlers) streamTicket(w http.ResponseWriter, r *http.Request) {
	a, id := actor(r), r.PathValue("id")
	d, err := h.enq.GetTicket(r.Context(), a, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	sse := httpx.NewSSE(w)
	sent := map[string]bool{}
	send := func(rows []services.ThreadRow) bool {
		for _, row := range rows {
			if sent[row.ID] {
				continue
			}
			if err := sse.Send("ticket_event", row); err != nil {
				return false
			}
			sent[row.ID] = true
		}
		return true
	}
	if !send(d.Thread) {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			d, err := h.enq.GetTicket(r.Context(), a, id)
			if err != nil {
				_, body := h.errorBody(r, err)
				_ = sse.Send("error", body)
				return
			}
			if !send(d.Thread) {
				return
			}
		}
	}
}

// registerSchedules mounts client medicine schedules (client tokens only).
func (h *Handlers) registerSchedules(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/client/schedules", h.auth(clientOnly, h.listSchedules))
	mux.HandleFunc("POST /api/v1/client/schedules", h.auth(clientOnly, h.createSchedule))
	mux.HandleFunc("PATCH /api/v1/client/schedules/{id}", h.auth(clientOnly, h.updateSchedule))
	mux.HandleFunc("POST /api/v1/client/schedules/{id}/run", h.auth(clientOnly, h.runSchedule))
}

func (h *Handlers) listSchedules(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	items, err := h.enq.ListSchedules(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) createSchedule(w http.ResponseWriter, r *http.Request) {
	var in services.CreateScheduleInput
	if !h.decode(w, r, &in) {
		return
	}
	v, err := h.enq.CreateSchedule(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}

func (h *Handlers) updateSchedule(w http.ResponseWriter, r *http.Request) {
	var in services.UpdateScheduleInput
	if !h.decode(w, r, &in) {
		return
	}
	v, err := h.enq.UpdateSchedule(r.Context(), actor(r), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

func (h *Handlers) runSchedule(w http.ResponseWriter, r *http.Request) {
	t, err := h.enq.RunScheduleNow(r.Context(), actor(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}
