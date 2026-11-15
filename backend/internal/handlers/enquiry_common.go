package handlers

// Wassit auth wrapper and shared helpers (owned by the lead, see CONTRACT.md).
// Every enquiry route is wrapped with one access level:
//
//	mux.HandleFunc("POST /api/v1/targets/{id}/reply", h.auth(staffOnly, h.replyTarget))
//
// Inside a handler, actor(r) returns the authenticated services.Actor.

import (
	"context"
	"net/http"

	"alphahack/internal/services"
)

type access int

const (
	staffOnly     access = iota // human pharmacists only; agent credential -> 403 agent_forbidden
	agentOK                     // staff or agent
	agentOnly                   // agent only; staff token -> 403 forbidden
	clientOnly                  // DOSYN clients only; staff -> 403 forbidden, agent -> 403 agent_forbidden
	staffOrClient               // humans: staff or client; agent -> 403 agent_forbidden
	anyActor                    // staff, agent or client; the service decides who may see what
)

// A client token on any staff or agent route gets 403 forbidden.

type actorKey struct{}

// auth authenticates the Authorization header ("Bearer <staff token>" or
// "Agent <AGENT_TOKEN>") via EnquiryService.Authenticate and enforces level.
func (h *Handlers) auth(level access, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if authz == "" && r.URL.Query().Get("access_token") != "" {
			authz = "Bearer " + r.URL.Query().Get("access_token") // EventSource cannot set headers
		}
		a, err := h.enq.Authenticate(r.Context(), authz)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		switch {
		case (level == staffOnly || level == clientOnly || level == staffOrClient) && a.IsAgent():
			h.fail(w, r, services.AgentForbidden())
			return
		case level == agentOnly && !a.IsAgent():
			h.fail(w, r, services.Forbidden("this route is for the Wassit agent only"))
			return
		case level == clientOnly && !a.IsClient():
			h.fail(w, r, services.Forbidden("this route is for DOSYN clients; log in with POST /api/v1/client/auth/login"))
			return
		case (level == staffOnly || level == agentOK) && a.IsClient():
			h.fail(w, r, services.Forbidden("this route is for pharmacists; client tokens are not accepted here"))
			return
		}
		fn(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, a)))
	}
}

// actor returns the caller set by auth. Only valid inside an auth-wrapped handler.
func actor(r *http.Request) services.Actor {
	a, _ := r.Context().Value(actorKey{}).(services.Actor)
	return a
}
