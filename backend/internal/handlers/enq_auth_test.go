package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	httpx "alphahack/internal/http"
	"alphahack/internal/queries"
	"alphahack/internal/services"
)

// enqAuthStore is a fake session store. Only GetStaffBySessionToken is
// implemented; any other Querier method would panic on the nil embedded
// interface, which is the desired failure mode for an unexpected call.
type enqAuthStore struct {
	queries.Querier
}

func (s enqAuthStore) GetStaffBySessionToken(_ context.Context, tokenHash string) (queries.Staff, error) {
	if tokenHash == enqAuthSHA256Hex("staff-tok") {
		return queries.Staff{ID: "staff-1", BranchID: "branch-1"}, nil
	}
	return queries.Staff{}, queries.ErrNotFound
}

func enqAuthSHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// enqAuthServer mounts the handlers with an enquiry service built the way the
// server builds it, using the given agent token config.
func enqAuthServer(t *testing.T, agentToken string) http.Handler {
	t.Helper()
	svc := services.NewEnquiryService(enqAuthStore{}, services.EnquiryConfig{AgentToken: agentToken})
	mux := http.NewServeMux()
	New(Deps{Enquiry: svc}).Register(mux)
	return httpx.NewRouter(mux)
}

// enqAuthRequest sends one request. authorization is set as the Authorization
// header only when non-empty, so "no header" can be expressed.
func enqAuthRequest(t *testing.T, h http.Handler, method, target, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func enqAuthErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not JSON: %v: %s", err, rec.Body.String())
	}
	return body.Error.Code
}

func TestEnquiryAuthMatrix(t *testing.T) {
	const agentToken = "test-agent-token"

	tests := []struct {
		name          string
		emptyConfig   bool // build the service with AgentToken ""
		method        string
		target        string
		authorization string
		wantStatus    int
		wantCode      string
	}{
		{
			name:       "missing_credential_on_staff_route",
			method:     http.MethodGet,
			target:     "/api/v1/me",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthorized",
		},
		{
			name:          "wrong_agent_token",
			method:        http.MethodGet,
			target:        "/api/v1/agent/wakeups",
			authorization: "Agent nope",
			wantStatus:    http.StatusUnauthorized,
			wantCode:      "unauthorized",
		},
		{
			name:          "agent_token_on_staff_only_route",
			method:        http.MethodGet,
			target:        "/api/v1/me",
			authorization: "Agent test-agent-token",
			wantStatus:    http.StatusForbidden,
			wantCode:      "agent_forbidden",
		},
		{
			name:          "agent_token_on_staff_reply_route",
			method:        http.MethodPost,
			target:        "/api/v1/targets/t1/reply",
			authorization: "Agent test-agent-token",
			wantStatus:    http.StatusForbidden,
			wantCode:      "agent_forbidden",
		},
		{
			name:          "staff_token_on_agent_only_route",
			method:        http.MethodGet,
			target:        "/api/v1/agent/wakeups",
			authorization: "Bearer staff-tok",
			wantStatus:    http.StatusForbidden,
			wantCode:      "forbidden",
		},
		{
			name:          "unknown_bearer_token",
			method:        http.MethodGet,
			target:        "/api/v1/agent/wakeups",
			authorization: "Bearer nope",
			wantStatus:    http.StatusUnauthorized,
			wantCode:      "unauthorized",
		},
		{
			name:       "access_token_query_is_staff_not_agent",
			method:     http.MethodGet,
			target:     "/api/v1/agent/wakeups?access_token=staff-tok",
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
		{
			name:          "empty_agent_token_config_disables_agent",
			emptyConfig:   true,
			method:        http.MethodGet,
			target:        "/api/v1/agent/wakeups",
			authorization: "Agent ",
			wantStatus:    http.StatusUnauthorized,
			wantCode:      "unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgToken := agentToken
			if tt.emptyConfig {
				cfgToken = ""
			}
			h := enqAuthServer(t, cfgToken)
			rec := enqAuthRequest(t, h, tt.method, tt.target, tt.authorization)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if code := enqAuthErrorCode(t, rec); code != tt.wantCode {
				t.Errorf("error code = %q, want %q; body %s", code, tt.wantCode, rec.Body.String())
			}
		})
	}
}
