package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	httpx "alphahack/internal/http"
	"alphahack/internal/queries"
	"alphahack/internal/services"
)

type refHandlerStore struct {
	docs []queries.CreateReferenceDocumentParams
}

func (s *refHandlerStore) ListGovProducts(context.Context, queries.ListGovProductsParams) ([]queries.GovProduct, error) {
	return []queries.GovProduct{{Code: "6280000000001", BrandName: "Augmentin"}}, nil
}

func (s *refHandlerStore) CreateGovProduct(_ context.Context, p queries.CreateGovProductParams) (queries.GovProduct, error) {
	return queries.GovProduct{Code: p.Code}, nil
}

func (s *refHandlerStore) CreateReferenceDocument(_ context.Context, p queries.CreateReferenceDocumentParams) (queries.ReferenceDocument, error) {
	s.docs = append(s.docs, p)
	return queries.ReferenceDocument{ID: p.ID, Name: p.Name, ContentType: p.ContentType, SizeBytes: p.SizeBytes, StoragePath: p.StoragePath}, nil
}

func (s *refHandlerStore) ListReferenceDocuments(context.Context, int, int) ([]queries.ReferenceDocument, error) {
	return []queries.ReferenceDocument{}, nil
}

type stockHandlerStore struct{ queries.Querier }

func (stockHandlerStore) InTx(_ context.Context, fn func(q queries.Querier) error) error {
	return fn(stockHandlerStore{})
}

func (stockHandlerStore) GetMedication(_ context.Context, id string) (queries.Medication, error) {
	return queries.Medication{ID: id}, nil
}

func (stockHandlerStore) GetStockBatchForUpdate(context.Context, string, string) (queries.InventoryBatch, error) {
	return queries.InventoryBatch{ID: "batch-1", BranchID: "branch-1", MedicationID: "m1", QuantityOnHand: 3}, nil
}

func (stockHandlerStore) SetBatchQuantityOnHand(_ context.Context, id string, qty int) (queries.InventoryBatch, error) {
	return queries.InventoryBatch{ID: id, BranchID: "branch-1", MedicationID: "m1", QuantityOnHand: qty, QuantityAvailable: qty}, nil
}

func refServer(t *testing.T, store *refHandlerStore) http.Handler {
	t.Helper()
	enq := services.NewEnquiryService(enqAuthStore{}, services.EnquiryConfig{AgentToken: "agent-tok"})
	mux := http.NewServeMux()
	New(Deps{
		Enquiry:   enq,
		Reference: services.NewReferenceService(store, t.TempDir()),
		Stock:     services.NewStockService(stockHandlerStore{}, nil),
	}).Register(mux)
	return httpx.NewRouter(mux)
}

func multipartBody(t *testing.T, field, filename, contentType string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
	hdr.Set("Content-Type", contentType)
	pw, err := mw.CreatePart(hdr)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = pw.Write(data)
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestReferenceRoutes(t *testing.T) {
	pdf := []byte("%PDF-1.4\n%%EOF\n")
	cases := []struct {
		name       string
		method     string
		target     string
		auth       string
		body       func() (*bytes.Buffer, string)
		wantStatus int
		wantCode   string
	}{
		{"products public", http.MethodGet, "/api/v1/reference/products?ingredient=amoxicillin/clavulanate&limit=5", "", nil, 200, ""},
		{"products agent ok", http.MethodGet, "/api/v1/reference/products?q=aug", "Agent agent-tok", nil, 200, ""},
		{"bad limit", http.MethodGet, "/api/v1/reference/products?limit=-1", "", nil, 400, "bad_request"},
		{"create product agent forbidden", http.MethodPost, "/api/v1/reference/products", "Agent agent-tok",
			func() (*bytes.Buffer, string) { return bytes.NewBufferString(`{}`), "application/json" }, 403, "agent_forbidden"},
		{"create product no credential", http.MethodPost, "/api/v1/reference/products", "",
			func() (*bytes.Buffer, string) { return bytes.NewBufferString(`{}`), "application/json" }, 401, "unauthorized"},
		{"create product staff", http.MethodPost, "/api/v1/reference/products", "Bearer staff-tok",
			func() (*bytes.Buffer, string) {
				return bytes.NewBufferString(`{"code":"6280000000099","reg_no":"R","brand_name":"B","ingredient":"I","strength":"1mg","dosage_form":"tablet","official_price_cents":1}`), "application/json"
			}, 201, ""},
		{"upload pdf", http.MethodPost, "/api/v1/reference/documents", "Bearer staff-tok",
			func() (*bytes.Buffer, string) {
				return multipartBody(t, "document", "list.pdf", "application/pdf", pdf)
			}, 201, ""},
		{"upload type mismatch", http.MethodPost, "/api/v1/reference/documents", "Bearer staff-tok",
			func() (*bytes.Buffer, string) { return multipartBody(t, "document", "list.png", "image/png", pdf) }, 400, "validation_failed"},
		{"upload html rejected", http.MethodPost, "/api/v1/reference/documents", "Bearer staff-tok",
			func() (*bytes.Buffer, string) {
				return multipartBody(t, "document", "x.pdf", "application/pdf", []byte("<html><script>alert(1)</script>"))
			}, 400, "validation_failed"},
		{"upload wrong field", http.MethodPost, "/api/v1/reference/documents", "Bearer staff-tok",
			func() (*bytes.Buffer, string) { return multipartBody(t, "file", "list.pdf", "application/pdf", pdf) }, 400, "validation_failed"},
		{"upload not multipart", http.MethodPost, "/api/v1/reference/documents", "Bearer staff-tok",
			func() (*bytes.Buffer, string) { return bytes.NewBufferString(`{}`), "application/json" }, 400, "validation_failed"},
		{"upload agent forbidden", http.MethodPost, "/api/v1/reference/documents", "Agent agent-tok",
			func() (*bytes.Buffer, string) {
				return multipartBody(t, "document", "list.pdf", "application/pdf", pdf)
			}, 403, "agent_forbidden"},
		{"list documents agent forbidden", http.MethodGet, "/api/v1/reference/documents", "Agent agent-tok", nil, 403, "agent_forbidden"},
		{"list documents staff", http.MethodGet, "/api/v1/reference/documents", "Bearer staff-tok", nil, 200, ""},
		{"stock own branch", http.MethodPut, "/api/v1/branches/branch-1/inventory/m1", "Bearer staff-tok",
			func() (*bytes.Buffer, string) {
				return bytes.NewBufferString(`{"quantity_on_hand":7}`), "application/json"
			}, 200, ""},
		{"stock other branch", http.MethodPut, "/api/v1/branches/branch-2/inventory/m1", "Bearer staff-tok",
			func() (*bytes.Buffer, string) {
				return bytes.NewBufferString(`{"quantity_on_hand":7}`), "application/json"
			}, 403, "forbidden"},
		{"stock agent forbidden", http.MethodPut, "/api/v1/branches/branch-1/inventory/m1", "Agent agent-tok",
			func() (*bytes.Buffer, string) {
				return bytes.NewBufferString(`{"quantity_on_hand":7}`), "application/json"
			}, 403, "agent_forbidden"},
		{"stock negative", http.MethodPut, "/api/v1/branches/branch-1/inventory/m1", "Bearer staff-tok",
			func() (*bytes.Buffer, string) {
				return bytes.NewBufferString(`{"quantity_on_hand":-2}`), "application/json"
			}, 400, "validation_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &refHandlerStore{}
			h := refServer(t, store)
			var req *http.Request
			if tc.body != nil {
				body, ct := tc.body()
				req = httptest.NewRequest(tc.method, tc.target, body)
				req.Header.Set("Content-Type", ct)
			} else {
				req = httptest.NewRequest(tc.method, tc.target, nil)
			}
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantCode != "" {
				if got := enqAuthErrorCode(t, rec); got != tc.wantCode {
					t.Fatalf("code = %q, want %q", got, tc.wantCode)
				}
			}
			if tc.name == "upload pdf" {
				var doc map[string]any
				_ = json.Unmarshal(rec.Body.Bytes(), &doc)
				if doc["content_type"] != "application/pdf" || doc["name"] != "list.pdf" {
					t.Fatalf("doc = %v", doc)
				}
				if strings.Contains(rec.Body.String(), "storage_path") || strings.Contains(rec.Body.String(), "uploaded_by") {
					t.Fatalf("server-side fields leaked: %s", rec.Body.String())
				}
			}
			if tc.name == "stock own branch" && !strings.Contains(rec.Body.String(), `"quantity_on_hand":7`) {
				t.Fatalf("body = %s", rec.Body.String())
			}
		})
	}
}
