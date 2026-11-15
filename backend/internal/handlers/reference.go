package handlers

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

// registerReference mounts the government reference. The product list is
// public (the agent reads it as information); writes and documents are staff only.
func (h *Handlers) registerReference(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/reference/products", h.listReferenceProducts)
	mux.HandleFunc("POST /api/v1/reference/products", h.auth(staffOnly, h.createReferenceProduct))
	mux.HandleFunc("POST /api/v1/reference/documents", h.auth(staffOnly, h.uploadReferenceDocument))
	mux.HandleFunc("GET /api/v1/reference/documents", h.auth(staffOnly, h.listReferenceDocuments))
}

func (h *Handlers) listReferenceProducts(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	items, err := h.reference.ListProducts(r.Context(), services.ReferenceFilter{
		Q: q.Get("q"), Ingredient: q.Get("ingredient"), Strength: q.Get("strength"), Form: q.Get("form"),
	}, p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}

func (h *Handlers) createReferenceProduct(w http.ResponseWriter, r *http.Request) {
	var in services.CreateGovProductInput
	if !h.decode(w, r, &in) {
		return
	}
	g, err := h.reference.CreateProduct(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, g)
}

// uploadReferenceDocument reads the multipart field "document" (max 10 MB),
// sniffs its content type and hands both types to the service, which decides.
func (h *Handlers) uploadReferenceDocument(w http.ResponseWriter, r *http.Request) {
	const fieldHelp = `send multipart/form-data with the file in a field named "document" (PDF, PNG or JPEG, max 10 MB)`
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "multipart/form-data" {
		h.fail(w, r, services.Invalid("%s", fieldHelp))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, services.MaxReferenceDocumentBytes+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		h.fail(w, r, services.Invalid("%s", fieldHelp))
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			h.fail(w, r, services.Invalid(`no field named "document"; %s`, fieldHelp))
			return
		}
		if err != nil {
			h.fail(w, r, uploadReadError(err, fieldHelp))
			return
		}
		if part.FormName() != "document" {
			_ = part.Close()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, services.MaxReferenceDocumentBytes+1))
		_ = part.Close()
		if err != nil {
			h.fail(w, r, uploadReadError(err, fieldHelp))
			return
		}
		sniffed := http.DetectContentType(data)
		doc, err := h.reference.UploadDocument(r.Context(), actor(r), services.UploadDocumentInput{
			Name: part.FileName(), DeclaredType: part.Header.Get("Content-Type"),
			SniffedType: sniffed, Data: data,
		})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, doc)
		return
	}
}

func uploadReadError(err error, help string) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || strings.Contains(err.Error(), "request body too large") {
		return services.Invalid("the upload is larger than 10 MB; upload a smaller or compressed file")
	}
	return services.Invalid("could not read the upload; %s", help)
}

func (h *Handlers) listReferenceDocuments(w http.ResponseWriter, r *http.Request) {
	p, ok := h.page(w, r)
	if !ok {
		return
	}
	items, err := h.reference.ListDocuments(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeList(w, items, p)
}
