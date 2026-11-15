package handlers

import (
	"net/http"
	"testing"
)

// amLevel is the auth level a route requires, as listed in the route table.
type amLevel string

const (
	amStaff  amLevel = "staff"
	amEither amLevel = "either"
	amAgent  amLevel = "agent"
)

// amRoute is one row of the auth matrix.
type amRoute struct {
	method string
	path   string
	level  amLevel
}

func (r amRoute) name() string { return r.method + " " + r.path }

// amRoutes is the full route table: 9 staff, 11 either, 6 agent.
var amRoutes = []amRoute{
	// staff
	{http.MethodGet, "/api/v1/me", amStaff},
	{http.MethodPost, "/api/v1/enquiries", amStaff},
	{http.MethodGet, "/api/v1/enquiries", amStaff},
	{http.MethodGet, "/api/v1/inbox", amStaff},
	{http.MethodPost, "/api/v1/targets/t1/reply", amStaff},
	{http.MethodPost, "/api/v1/enquiries/e1/clarify", amStaff},
	{http.MethodPost, "/api/v1/enquiries/e1/targets/approve", amStaff},
	{http.MethodGet, "/api/v1/stats", amStaff},
	{http.MethodGet, "/api/v1/enquiries/e1/stream", amStaff},

	// either
	{http.MethodGet, "/api/v1/enquiries/e1", amEither},
	{http.MethodPost, "/api/v1/enquiries/e1/medication", amEither},
	{http.MethodGet, "/api/v1/enquiries/e1/candidates", amEither},
	{http.MethodPost, "/api/v1/enquiries/e1/targets", amEither},
	{http.MethodPost, "/api/v1/enquiries/e1/close", amEither},
	{http.MethodPost, "/api/v1/enquiries/e1/send", amEither},
	{http.MethodPost, "/api/v1/messages/m1/cancel", amEither},
	{http.MethodPost, "/api/v1/targets/t1/follow-up", amEither},
	{http.MethodPost, "/api/v1/targets/t1/no-response", amEither},
	{http.MethodPost, "/api/v1/enquiries/e1/notify", amEither},
	{http.MethodPost, "/api/v1/events/ev1/undo", amEither},

	// agent
	{http.MethodGet, "/api/v1/agent/wakeups", amAgent},
	{http.MethodPost, "/api/v1/agent/runs", amAgent},
	{http.MethodPatch, "/api/v1/agent/runs/r1", amAgent},
	{http.MethodPost, "/api/v1/agent/runs/r1/tool-calls", amAgent},
	{http.MethodPost, "/api/v1/enquiries/e1/wait", amAgent},
	{http.MethodPost, "/api/v1/enquiries/e1/clarification-request", amAgent},
}

// amRoutesAt returns the routes of one level, in table order.
func amRoutesAt(level amLevel) []amRoute {
	var out []amRoute
	for _, r := range amRoutes {
		if r.level == level {
			out = append(out, r)
		}
	}
	return out
}

func TestAuthMatrixNoCredential(t *testing.T) {
	h := enqAuthServer(t, "test-agent-token")
	for _, r := range amRoutes {
		t.Run(r.name(), func(t *testing.T) {
			rec := enqAuthRequest(t, h, r.method, r.path, "")
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d; body %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			if code := enqAuthErrorCode(t, rec); code != "unauthorized" {
				t.Errorf("error code = %q, want %q; body %s", code, "unauthorized", rec.Body.String())
			}
		})
	}
}

func TestAuthMatrixUnknownBearer(t *testing.T) {
	h := enqAuthServer(t, "test-agent-token")
	for _, r := range amRoutes {
		t.Run(r.name(), func(t *testing.T) {
			rec := enqAuthRequest(t, h, r.method, r.path, "Bearer nope")
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d; body %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			if code := enqAuthErrorCode(t, rec); code != "unauthorized" {
				t.Errorf("error code = %q, want %q; body %s", code, "unauthorized", rec.Body.String())
			}
		})
	}
}

func TestAuthMatrixWrongAgentToken(t *testing.T) {
	h := enqAuthServer(t, "test-agent-token")
	for _, r := range amRoutes {
		t.Run(r.name(), func(t *testing.T) {
			rec := enqAuthRequest(t, h, r.method, r.path, "Agent nope")
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d; body %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			if code := enqAuthErrorCode(t, rec); code != "unauthorized" {
				t.Errorf("error code = %q, want %q; body %s", code, "unauthorized", rec.Body.String())
			}
		})
	}
}

func TestAuthMatrixBasicScheme(t *testing.T) {
	h := enqAuthServer(t, "test-agent-token")
	for _, r := range amRoutes {
		t.Run(r.name(), func(t *testing.T) {
			rec := enqAuthRequest(t, h, r.method, r.path, "Basic abc")
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d; body %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			if code := enqAuthErrorCode(t, rec); code != "unauthorized" {
				t.Errorf("error code = %q, want %q; body %s", code, "unauthorized", rec.Body.String())
			}
		})
	}
}

func TestAuthMatrixAgentOnStaffOnlyRoutes(t *testing.T) {
	h := enqAuthServer(t, "test-agent-token")
	for _, r := range amRoutesAt(amStaff) {
		t.Run(r.name(), func(t *testing.T) {
			rec := enqAuthRequest(t, h, r.method, r.path, "Agent test-agent-token")
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d; body %s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
			if code := enqAuthErrorCode(t, rec); code != "agent_forbidden" {
				t.Errorf("error code = %q, want %q; body %s", code, "agent_forbidden", rec.Body.String())
			}
		})
	}
}

func TestAuthMatrixStaffOnAgentOnlyRoutes(t *testing.T) {
	h := enqAuthServer(t, "test-agent-token")
	for _, r := range amRoutesAt(amAgent) {
		t.Run(r.name(), func(t *testing.T) {
			rec := enqAuthRequest(t, h, r.method, r.path, "Bearer staff-tok")
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d; body %s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
			if code := enqAuthErrorCode(t, rec); code != "forbidden" {
				t.Errorf("error code = %q, want %q; body %s", code, "forbidden", rec.Body.String())
			}
		})
	}
}

func TestAuthMatrixAccessTokenQueryIsStaffOnly(t *testing.T) {
	h := enqAuthServer(t, "test-agent-token")
	for _, r := range amRoutesAt(amAgent) {
		t.Run(r.name(), func(t *testing.T) {
			rec := enqAuthRequest(t, h, r.method, r.path+"?access_token=test-agent-token", "")
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d; body %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			if code := enqAuthErrorCode(t, rec); code != "unauthorized" {
				t.Errorf("error code = %q, want %q; body %s", code, "unauthorized", rec.Body.String())
			}
		})
	}
}

func TestAuthMatrixAgentDisabledWhenTokenUnset(t *testing.T) {
	h := enqAuthServer(t, "")
	for _, r := range amRoutes {
		t.Run(r.name(), func(t *testing.T) {
			for _, authz := range []string{"Agent test-agent-token", "Agent "} {
				rec := enqAuthRequest(t, h, r.method, r.path, authz)
				if rec.Code != http.StatusUnauthorized {
					t.Errorf("header %q: status = %d, want %d; body %s", authz, rec.Code, http.StatusUnauthorized, rec.Body.String())
				}
				if code := enqAuthErrorCode(t, rec); code != "unauthorized" {
					t.Errorf("header %q: error code = %q, want %q; body %s", authz, code, "unauthorized", rec.Body.String())
				}
			}
		})
	}
}

func TestAuthMatrixRouteTableIsComplete(t *testing.T) {
	if got := len(amRoutes); got != 26 {
		t.Fatalf("route table has %d rows, want 26", got)
	}
	want := map[amLevel]int{amStaff: 9, amEither: 11, amAgent: 6}
	for level, n := range want {
		if got := len(amRoutesAt(level)); got != n {
			t.Errorf("level %q has %d routes, want %d", level, got, n)
		}
	}
}
