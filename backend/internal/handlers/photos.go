package handlers

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	httpx "alphahack/internal/http"
	"alphahack/internal/services"
)

// registerPhotos mounts package photos (CONTRACT rule 10). Upload: staff at the
// target branch; agent reading: agent only; review: reviewer staff; file: the
// service decides (target or requester staff, reviewer, agent, ticket client).
func (h *Handlers) registerPhotos(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/targets/{id}/photos", h.auth(staffOnly, h.uploadPhoto))
	mux.HandleFunc("POST /api/v1/photos/{id}/agent-reading", h.auth(agentOnly, h.agentReadingPhoto))
	mux.HandleFunc("POST /api/v1/photos/{id}/review", h.auth(staffOnly, h.reviewPhoto))
	mux.HandleFunc("GET /api/v1/photos/{id}/file", h.auth(anyActor, h.photoFile))
}

func (h *Handlers) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	const help = `send multipart/form-data with the image in a field named "photo" (JPEG or PNG, max 5 MB) plus fields "code" and "expiry_month"`
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "multipart/form-data" {
		h.fail(w, r, services.Invalid("%s", help))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, services.MaxPhotoBytes+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		h.fail(w, r, services.Invalid("%s", help))
		return
	}
	in := services.UploadPhotoInput{}
	gotPhoto := false
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			h.fail(w, r, photoReadError(err, help))
			return
		}
		switch part.FormName() {
		case "photo":
			data, err := io.ReadAll(io.LimitReader(part, services.MaxPhotoBytes+1))
			if err != nil {
				_ = part.Close()
				h.fail(w, r, photoReadError(err, help))
				return
			}
			in.Data, in.DeclaredType, in.SniffedType, gotPhoto = data, part.Header.Get("Content-Type"), http.DetectContentType(data), true
		case "code", "expiry_month":
			v, err := io.ReadAll(io.LimitReader(part, 256))
			if err != nil {
				_ = part.Close()
				h.fail(w, r, photoReadError(err, help))
				return
			}
			if part.FormName() == "code" {
				in.Code = string(v)
			} else {
				in.ExpiryMonth = string(v)
			}
		}
		_ = part.Close()
	}
	if !gotPhoto {
		h.fail(w, r, services.Invalid(`no field named "photo"; %s`, help))
		return
	}
	res, err := h.enq.UploadPhoto(r.Context(), actor(r), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

func photoReadError(err error, help string) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || strings.Contains(err.Error(), "request body too large") {
		return services.Invalid("the upload is larger than 5 MB; upload a smaller photo")
	}
	return services.Invalid("could not read the upload; %s", help)
}

func (h *Handlers) agentReadingPhoto(w http.ResponseWriter, r *http.Request) {
	var in services.AgentReadingInput
	if !h.decode(w, r, &in) {
		return
	}
	res, err := h.enq.RecordAgentReading(r.Context(), actor(r), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handlers) reviewPhoto(w http.ResponseWriter, r *http.Request) {
	var in services.ReviewPhotoInput
	if !h.decode(w, r, &in) {
		return
	}
	res, err := h.enq.ReviewPhoto(r.Context(), actor(r), r.PathValue("id"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handlers) photoFile(w http.ResponseWriter, r *http.Request) {
	f, err := h.enq.GetPhotoFile(r.Context(), actor(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(f.Data)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f.Data)
}
