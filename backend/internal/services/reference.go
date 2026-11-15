package services

// Government reference: the official product list (read by anyone, the agent
// included, as information only) and the source documents staff upload.
// Equivalence decisions never happen here.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"alphahack/internal/queries"
)

// MaxReferenceDocumentBytes is the upload limit for one reference document.
const MaxReferenceDocumentBytes = 10 << 20

// SourceStaffEntry is the source of a product row added by staff without one.
const SourceStaffEntry = "Staff entry"

// referenceTypes are the accepted document types and their file extensions.
var referenceTypes = map[string]string{
	"application/pdf": ".pdf",
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
}

var govCodeRe = regexp.MustCompile(`^[0-9]{13}$`)

type ReferenceStore interface {
	ListGovProducts(ctx context.Context, p queries.ListGovProductsParams) ([]queries.GovProduct, error)
	CreateGovProduct(ctx context.Context, p queries.CreateGovProductParams) (queries.GovProduct, error)
	CreateReferenceDocument(ctx context.Context, p queries.CreateReferenceDocumentParams) (queries.ReferenceDocument, error)
	ListReferenceDocuments(ctx context.Context, limit, offset int) ([]queries.ReferenceDocument, error)
}

type ReferenceService struct {
	store     ReferenceStore
	uploadDir string
}

// NewReferenceService stores uploads under uploadDir/reference ("" = ./uploads).
func NewReferenceService(store ReferenceStore, uploadDir string) *ReferenceService {
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	return &ReferenceService{store: store, uploadDir: uploadDir}
}

// ReferenceFilter: empty fields mean no filter. Ingredient, Strength and Form
// are case-insensitive exact matches; Q is a substring of brand or ingredient.
type ReferenceFilter struct {
	Q          string
	Ingredient string
	Strength   string
	Form       string
}

func (s *ReferenceService) ListProducts(ctx context.Context, f ReferenceFilter, p Page) ([]queries.GovProduct, error) {
	p = p.Normalize()
	for _, v := range []string{f.Q, f.Ingredient, f.Strength, f.Form} {
		if utf8.RuneCountInString(v) > MaxNameLen {
			return nil, Invalid("search filters must be at most %d characters", MaxNameLen)
		}
	}
	list, err := s.store.ListGovProducts(ctx, queries.ListGovProductsParams{
		Q: escapeLike(clean(f.Q)), Ingredient: clean(f.Ingredient), Strength: clean(f.Strength),
		DosageForm: clean(f.Form), Limit: p.Limit, Offset: p.Offset,
	})
	if err != nil {
		return nil, FromStore(err, "reference product")
	}
	return list, nil
}

// escapeLike escapes the ILIKE wildcards so q is matched literally (ESCAPE '\').
func escapeLike(q string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
}

type CreateGovProductInput struct {
	Code               string  `json:"code"`
	RegNo              string  `json:"reg_no"`
	BrandName          string  `json:"brand_name"`
	Ingredient         string  `json:"ingredient"`
	Strength           string  `json:"strength"`
	DosageForm         string  `json:"dosage_form"`
	OfficialPriceCents *int64  `json:"official_price_cents"`
	Source             string  `json:"source"`      // optional, default "Staff entry"
	DocumentID         *string `json:"document_id"` // optional, an uploaded reference document
}

// CreateProduct adds one reference row (staff only).
func (s *ReferenceService) CreateProduct(ctx context.Context, a Actor, in CreateGovProductInput) (queries.GovProduct, error) {
	if a.IsAgent() {
		return queries.GovProduct{}, AgentForbidden()
	}
	p := queries.CreateGovProductParams{
		Code: clean(in.Code), RegNo: clean(in.RegNo), BrandName: clean(in.BrandName),
		Ingredient: clean(in.Ingredient), Strength: clean(in.Strength),
		DosageForm: strings.ToLower(clean(in.DosageForm)), Source: clean(in.Source),
	}
	if !govCodeRe.MatchString(p.Code) {
		return queries.GovProduct{}, Invalid("code must be exactly 13 digits, e.g. 6280000000001")
	}
	for _, f := range []struct{ name, v string }{
		{"reg_no", p.RegNo}, {"brand_name", p.BrandName}, {"ingredient", p.Ingredient},
		{"strength", p.Strength}, {"dosage_form", p.DosageForm},
	} {
		if n := utf8.RuneCountInString(f.v); n < 1 || n > MaxNameLen {
			return queries.GovProduct{}, Invalid("%s is required (1 to %d characters)", f.name, MaxNameLen)
		}
	}
	if in.OfficialPriceCents == nil || *in.OfficialPriceCents < 0 {
		return queries.GovProduct{}, Invalid("official_price_cents is required: a whole number of cents, 0 or more")
	}
	p.OfficialPriceCents = *in.OfficialPriceCents
	if p.Source == "" {
		p.Source = SourceStaffEntry
	}
	if utf8.RuneCountInString(p.Source) > MaxNameLen {
		return queries.GovProduct{}, Invalid("source must be at most %d characters", MaxNameLen)
	}
	if in.DocumentID != nil {
		if id := clean(*in.DocumentID); id != "" {
			p.DocumentID = &id
		}
	}
	g, err := s.store.CreateGovProduct(ctx, p)
	switch {
	case errors.Is(err, queries.ErrDuplicate):
		return g, Conflict("a reference product with this code, or with the same brand, ingredient, strength and form, already exists")
	case errors.Is(err, queries.ErrBadReference):
		return g, Invalid("document_id is not an uploaded reference document; upload it with POST /api/v1/reference/documents first or leave document_id out")
	case err != nil:
		return g, FromStore(err, "reference product")
	}
	return g, nil
}

// UploadDocumentInput is one uploaded file. SniffedType is what the content
// looks like (the handler sniffs it); DeclaredType is what the client sent.
type UploadDocumentInput struct {
	Name         string
	DeclaredType string
	SniffedType  string
	Data         []byte
}

// UploadDocument stores a reference document on disk and records it (staff only).
func (s *ReferenceService) UploadDocument(ctx context.Context, a Actor, in UploadDocumentInput) (queries.ReferenceDocument, error) {
	if a.IsAgent() {
		return queries.ReferenceDocument{}, AgentForbidden()
	}
	if len(in.Data) == 0 {
		return queries.ReferenceDocument{}, Invalid("the document is empty; attach a PDF, PNG or JPEG file in the multipart field \"document\"")
	}
	if len(in.Data) > MaxReferenceDocumentBytes {
		return queries.ReferenceDocument{}, Invalid("the document is larger than 10 MB; upload a smaller or compressed file")
	}
	sniffed := mediaType(in.SniffedType)
	ext, ok := referenceTypes[sniffed]
	if !ok {
		return queries.ReferenceDocument{}, Invalid("only PDF, PNG or JPEG documents are accepted; this file's content looks like %s", sniffed)
	}
	if declared := mediaType(in.DeclaredType); declared != "" && declared != "application/octet-stream" && declared != sniffed {
		return queries.ReferenceDocument{}, Invalid("the file is sent as %s but its content is %s; send the real file or fix its content type", declared, sniffed)
	}

	id, err := newUUID()
	if err != nil {
		return queries.ReferenceDocument{}, Internal(err)
	}
	dir := filepath.Join(s.uploadDir, "reference")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return queries.ReferenceDocument{}, Internal(err)
	}
	path := filepath.Join(dir, id+ext)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return queries.ReferenceDocument{}, Internal(err)
	}
	_, werr := f.Write(in.Data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(path)
		return queries.ReferenceDocument{}, Internal(werr)
	}
	sum := sha256.Sum256(in.Data)
	doc, err := s.store.CreateReferenceDocument(ctx, queries.CreateReferenceDocumentParams{
		ID: id, Name: documentName(in.Name, ext), ContentType: sniffed, SizeBytes: int64(len(in.Data)),
		SHA256: hex.EncodeToString(sum[:]), UploadedByStaffID: a.ID, StoragePath: path,
	})
	if err != nil {
		_ = os.Remove(path)
		return queries.ReferenceDocument{}, FromStore(err, "reference document")
	}
	return doc, nil
}

// ListDocuments lists uploaded reference documents (staff only), newest first.
func (s *ReferenceService) ListDocuments(ctx context.Context, a Actor, p Page) ([]queries.ReferenceDocument, error) {
	if a.IsAgent() {
		return nil, AgentForbidden()
	}
	p = p.Normalize()
	list, err := s.store.ListReferenceDocuments(ctx, p.Limit, p.Offset)
	if err != nil {
		return nil, FromStore(err, "reference document")
	}
	return list, nil
}

// mediaType returns the lowercased media type without parameters ("" if unparsable).
func mediaType(ct string) string {
	ct = strings.TrimSpace(ct)
	if ct == "" {
		return ""
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return strings.ToLower(ct)
	}
	if mt == "image/jpg" || mt == "image/pjpeg" {
		mt = "image/jpeg"
	}
	return mt
}

// documentName keeps the base file name without control characters, at most
// MaxNameLen runes; empty becomes "document<ext>".
func documentName(name, ext string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, clean(name))
	if name == "" || name == "." || name == "/" {
		return "document" + ext
	}
	if r := []rune(name); len(r) > MaxNameLen {
		name = string(r[:MaxNameLen])
	}
	return name
}

// newUUID returns a random (version 4) UUID string.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
