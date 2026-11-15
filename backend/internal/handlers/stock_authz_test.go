package handlers

// Attack cases for the staff stock update and the reference upload/write routes.

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

// p3aServer mounts the handlers with stock and reference services whose stores
// are nil: every attack below must be refused before any store call.
func p3aServer(t *testing.T) http.Handler {
	t.Helper()
	enq := services.NewEnquiryService(enqAuthStore{}, services.EnquiryConfig{AgentToken: "test-agent-token"})
	mux := http.NewServeMux()
	New(Deps{
		Enquiry:   enq,
		Stock:     services.NewStockService(nil, nil),
		Reference: services.NewReferenceService(nil, t.TempDir()),
	}).Register(mux)
	return httpx.NewRouter(mux)
}

func p3aDo(t *testing.T, h http.Handler, method, path, authz, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func p3aWant(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status || enqAuthErrorCode(t, rec) != code {
		t.Fatalf("status %d (want %d %s) body %s", rec.Code, status, code, rec.Body.String())
	}
}

func TestStockAttackOtherBranch(t *testing.T) {
	// staff-tok belongs to branch-1.
	rec := p3aDo(t, p3aServer(t), http.MethodPut, "/api/v1/branches/branch-2/inventory/med-1", "Bearer staff-tok",
		"application/json", []byte(`{"quantity_on_hand":5}`))
	p3aWant(t, rec, http.StatusForbidden, "forbidden")
}

func TestStockAttackAgentCredential(t *testing.T) {
	rec := p3aDo(t, p3aServer(t), http.MethodPut, "/api/v1/branches/branch-1/inventory/med-1", "Agent test-agent-token",
		"application/json", []byte(`{"quantity_on_hand":5}`))
	p3aWant(t, rec, http.StatusForbidden, "agent_forbidden")
}

func TestStockAttackBadQuantity(t *testing.T) {
	h := p3aServer(t)
	for body, code := range map[string]string{
		`{"quantity_on_hand":-1}`:       "validation_failed",
		`{"quantity_on_hand":10000001}`: "validation_failed",
		`{}`:                            "validation_failed",
		`{"quantity_on_hand":"5"}`:      "bad_request",
		`{"quantity_on_hand":1.5}`:      "bad_request",
		`{"quantity_on_hand":5,"quantity_reserved":0}`: "bad_request",
	} {
		t.Run(body, func(t *testing.T) {
			rec := p3aDo(t, h, http.MethodPut, "/api/v1/branches/branch-1/inventory/med-1", "Bearer staff-tok", "application/json", []byte(body))
			p3aWant(t, rec, http.StatusBadRequest, code)
		})
	}
}

func TestStockAttackNoCredential(t *testing.T) {
	rec := p3aDo(t, p3aServer(t), http.MethodPut, "/api/v1/branches/branch-1/inventory/med-1", "", "application/json", []byte(`{"quantity_on_hand":5}`))
	p3aWant(t, rec, http.StatusUnauthorized, "unauthorized")
}

func TestReferenceAttackAgentOnStaffRoutes(t *testing.T) {
	h := p3aServer(t)
	for _, r := range []struct{ method, path, ct, body string }{
		{http.MethodPost, "/api/v1/reference/products", "application/json", `{"code":"6280000000999"}`},
		{http.MethodPost, "/api/v1/reference/documents", "multipart/form-data; boundary=x", "--x--"},
		{http.MethodGet, "/api/v1/reference/documents", "", ""},
	} {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			rec := p3aDo(t, h, r.method, r.path, "Agent test-agent-token", r.ct, []byte(r.body))
			p3aWant(t, rec, http.StatusForbidden, "agent_forbidden")
		})
	}
}

func multipartBody(t *testing.T, field, filename, partType string, data []byte) (string, []byte) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
	if partType != "" {
		hdr.Set("Content-Type", partType)
	}
	w, err := mw.CreatePart(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return mw.FormDataContentType(), b.Bytes()
}

func TestUploadAttackWrongTypeOrSize(t *testing.T) {
	h := p3aServer(t)
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	cases := map[string]struct {
		field, name, partType string
		data                  []byte
	}{
		"html posing as pdf":    {"document", "x.pdf", "application/pdf", []byte("<html><script>alert(1)</script></html>")},
		"exe":                   {"document", "x.exe", "", append([]byte("MZ"), make([]byte, 64)...)},
		"png declared as pdf":   {"document", "x.pdf", "application/pdf", png},
		"plain text":            {"document", "x.txt", "text/plain", []byte("hello")},
		"svg (script-capable)":  {"document", "x.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`)},
		"empty file":            {"document", "x.pdf", "application/pdf", nil},
		"wrong field name":      {"file", "x.png", "image/png", png},
		"oversized (10 MB + 1)": {"document", "big.pdf", "application/pdf", append([]byte("%PDF-1.4\n"), make([]byte, services.MaxReferenceDocumentBytes)...)},
		"path traversal name":   {"document", "../../etc/passwd", "text/plain", []byte("root:x:0:0")},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ct, body := multipartBody(t, c.field, c.name, c.partType, c.data)
			rec := p3aDo(t, h, http.MethodPost, "/api/v1/reference/documents", "Bearer staff-tok", ct, body)
			p3aWant(t, rec, http.StatusBadRequest, "validation_failed")
		})
	}
	t.Run("not multipart", func(t *testing.T) {
		rec := p3aDo(t, h, http.MethodPost, "/api/v1/reference/documents", "Bearer staff-tok", "application/json", []byte(`{}`))
		p3aWant(t, rec, http.StatusBadRequest, "validation_failed")
	})
	t.Run("body far over the limit", func(t *testing.T) {
		big := strings.Repeat("A", services.MaxReferenceDocumentBytes+(2<<20))
		ct, body := multipartBody(t, "document", "big.pdf", "application/pdf", []byte("%PDF-1.4\n"+big))
		rec := p3aDo(t, h, http.MethodPost, "/api/v1/reference/documents", "Bearer staff-tok", ct, body)
		p3aWant(t, rec, http.StatusBadRequest, "validation_failed")
	})
}
