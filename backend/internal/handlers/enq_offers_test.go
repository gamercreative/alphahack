package handlers

// Attack cases for the offers and speech routes at the HTTP layer.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

func offersServer(t *testing.T, speechOn bool) http.Handler {
	t.Helper()
	cfg := services.EnquiryConfig{AgentToken: "test-agent-token"}
	if speechOn {
		cfg.Speech = services.FakeSpeech()
	}
	svc := services.NewEnquiryService(enqAuthStore{}, cfg)
	mux := http.NewServeMux()
	New(Deps{Enquiry: svc}).Register(mux)
	return httpx.NewRouter(mux)
}

func offersPost(t *testing.T, h http.Handler, path, authorization, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestOffersAttackNoOrBadCredential(t *testing.T) {
	h := offersServer(t, true)
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/targets/t1/offers"},
		{http.MethodGet, "/api/v1/enquiries/e1/offers"},
		{http.MethodPost, "/api/v1/speech"},
	}
	for _, r := range routes {
		for _, authz := range []string{"", "Agent nope", "Bearer nope", "Basic abc"} {
			t.Run(r.method+" "+r.path+" "+authz, func(t *testing.T) {
				rec := enqAuthRequest(t, h, r.method, r.path, authz)
				if rec.Code != http.StatusUnauthorized || enqAuthErrorCode(t, rec) != "unauthorized" {
					t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestOffersAttackAgentSetsDecision(t *testing.T) {
	// decision and decided_by are not input fields: the decoder rejects them
	// before the service runs (enqAuthStore would panic on any store call).
	h := offersServer(t, false)
	for _, field := range []string{`"decision":"matches"`, `"decided_by":"human"`, `"reasons":[]`, `"source_text":"3ande 10"`} {
		t.Run(field, func(t *testing.T) {
			body := `{"in_stock":true,"evidence":{"in_stock":"3ande"},"confidence":0.9,` + field + `}`
			rec := offersPost(t, h, "/api/v1/targets/t1/offers", "Agent test-agent-token", body)
			if rec.Code != http.StatusBadRequest || enqAuthErrorCode(t, rec) != "bad_request" {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestOffersAttackPriceNegativeHTTP(t *testing.T) {
	h := offersServer(t, false)
	rec := offersPost(t, h, "/api/v1/targets/t1/offers", "Agent test-agent-token",
		`{"price_cents":-100,"currency":"USD","evidence":{"price":"12$"},"confidence":0.9}`)
	if rec.Code != http.StatusBadRequest || enqAuthErrorCode(t, rec) != "validation_failed" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestSpeechAttackOversizedTextHTTP(t *testing.T) {
	h := offersServer(t, true)
	body := `{"text":"` + strings.Repeat("ب", 601) + `"}`
	rec := offersPost(t, h, "/api/v1/speech", "Agent test-agent-token", body)
	if rec.Code != http.StatusBadRequest || enqAuthErrorCode(t, rec) != "validation_failed" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestSpeechAttackEmptyTextHTTP(t *testing.T) {
	h := offersServer(t, true)
	for _, body := range []string{`{"text":""}`, `{"text":"   "}`, `{}`} {
		rec := offersPost(t, h, "/api/v1/speech", "Bearer staff-tok", body)
		if rec.Code != http.StatusBadRequest || enqAuthErrorCode(t, rec) != "validation_failed" {
			t.Fatalf("%s: status %d body %s", body, rec.Code, rec.Body.String())
		}
	}
}

func TestSpeechReturnsAudio(t *testing.T) {
	h := offersServer(t, true)
	for _, authz := range []string{"Bearer staff-tok", "Agent test-agent-token"} {
		rec := offersPost(t, h, "/api/v1/speech", authz, `{"text":"مرحبا","speaker":"pharmacy"}`)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "audio/wav" ||
			rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Speech-Voice") != "fake-ar-pharmacy" ||
			!strings.HasPrefix(rec.Body.String(), "RIFF") {
			t.Fatalf("%s: status %d headers %v", authz, rec.Code, rec.Header())
		}
	}
}

func TestSpeechOffIs409(t *testing.T) {
	rec := offersPost(t, offersServer(t, false), "/api/v1/speech", "Agent test-agent-token", `{"text":"hi"}`)
	if rec.Code != http.StatusConflict || enqAuthErrorCode(t, rec) != "conflict" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}
