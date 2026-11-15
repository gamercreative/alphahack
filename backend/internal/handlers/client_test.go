package handlers

import (
	"context"
	"net/http"
	"testing"

	httpx "alphahack/internal/http"
	"alphahack/internal/queries"
	"alphahack/internal/services"
)

// clientAuthStore knows one staff token and one client token; any other
// Querier call panics (nil embedded interface), so routes are only exercised
// up to the auth wrapper.
type clientAuthStore struct{ queries.Querier }

func (clientAuthStore) GetStaffBySessionToken(_ context.Context, h string) (queries.Staff, error) {
	if h == enqAuthSHA256Hex("staff-tok") {
		return queries.Staff{ID: "staff-1", BranchID: "branch-1"}, nil
	}
	return queries.Staff{}, queries.ErrNotFound
}

func (clientAuthStore) GetClientBySessionToken(_ context.Context, h string) (queries.Client, error) {
	if h == enqAuthSHA256Hex("c_client-tok") {
		return queries.Client{ID: "client-1", DisplayName: "Maya"}, nil
	}
	return queries.Client{}, queries.ErrNotFound
}

func TestClientAuthLevels(t *testing.T) {
	svc := services.NewEnquiryService(clientAuthStore{}, services.EnquiryConfig{AgentToken: "agent-tok"})
	mux := http.NewServeMux()
	New(Deps{Enquiry: svc}).Register(mux)
	h := httpx.NewRouter(mux)

	const client, staff, agent = "Bearer c_client-tok", "Bearer staff-tok", "Agent agent-tok"
	cases := []struct {
		name, method, path, auth string
		status                   int
		code                     string
	}{
		{"client on staff route", "GET", "/api/v1/me", client, 403, "forbidden"},
		{"client on staff+agent route", "GET", "/api/v1/enquiries/e1", client, 403, "forbidden"},
		{"client on agent route", "GET", "/api/v1/agent/wakeups", client, 403, "forbidden"},
		{"client on reply", "POST", "/api/v1/targets/t1/reply", client, 403, "forbidden"},
		{"client on photo upload", "POST", "/api/v1/targets/t1/photos", client, 403, "forbidden"},
		{"client on photo review", "POST", "/api/v1/photos/p1/review", client, 403, "forbidden"},
		{"client on agent reading", "POST", "/api/v1/photos/p1/agent-reading", client, 403, "forbidden"},
		{"staff on client route", "GET", "/api/v1/client/tickets", staff, 403, "forbidden"},
		{"staff on client schedules", "POST", "/api/v1/client/schedules", staff, 403, "forbidden"},
		{"agent on client route", "GET", "/api/v1/client/tickets/x", agent, 403, "agent_forbidden"},
		{"agent on client reserve", "POST", "/api/v1/client/tickets/x/reserve", agent, 403, "agent_forbidden"},
		{"agent on photo upload", "POST", "/api/v1/targets/t1/photos", agent, 403, "agent_forbidden"},
		{"staff on agent reading", "POST", "/api/v1/photos/p1/agent-reading", staff, 403, "forbidden"},
		{"unknown client token", "GET", "/api/v1/client/me", "Bearer c_nope", 401, "unauthorized"},
		{"no credential", "GET", "/api/v1/client/tickets", "", 401, "unauthorized"},
		{"client token as access_token on staff stream", "GET", "/api/v1/enquiries/e1/stream?access_token=c_client-tok", "", 403, "forbidden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := enqAuthRequest(t, h, tc.method, tc.path, tc.auth)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			if got := enqAuthErrorCode(t, rec); got != tc.code {
				t.Fatalf("code = %q, want %q", got, tc.code)
			}
		})
	}
}
