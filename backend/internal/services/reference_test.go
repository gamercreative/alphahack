package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alphahack/internal/queries"
)

type refStoreFake struct {
	listParams queries.ListGovProductsParams
	createErr  error
	docErr     error
	docs       []queries.CreateReferenceDocumentParams
}

func (f *refStoreFake) ListGovProducts(ctx context.Context, p queries.ListGovProductsParams) ([]queries.GovProduct, error) {
	f.listParams = p
	return []queries.GovProduct{}, nil
}

func (f *refStoreFake) CreateGovProduct(ctx context.Context, p queries.CreateGovProductParams) (queries.GovProduct, error) {
	if f.createErr != nil {
		return queries.GovProduct{}, f.createErr
	}
	return queries.GovProduct{Code: p.Code, Source: p.Source, DocumentID: p.DocumentID, DosageForm: p.DosageForm}, nil
}

func (f *refStoreFake) CreateReferenceDocument(ctx context.Context, p queries.CreateReferenceDocumentParams) (queries.ReferenceDocument, error) {
	if f.docErr != nil {
		return queries.ReferenceDocument{}, f.docErr
	}
	f.docs = append(f.docs, p)
	return queries.ReferenceDocument{ID: p.ID, Name: p.Name, ContentType: p.ContentType, SizeBytes: p.SizeBytes, SHA256: p.SHA256, StoragePath: p.StoragePath}, nil
}

func (f *refStoreFake) ListReferenceDocuments(ctx context.Context, limit, offset int) ([]queries.ReferenceDocument, error) {
	return []queries.ReferenceDocument{}, nil
}

var refStaff = Actor{Type: queries.ActorStaff, ID: "s-a", BranchID: "b-a"}
var refAgent = Actor{Type: queries.ActorAgent, ID: "wassit"}

func refCode(err error) string {
	var se *Error
	if errors.As(err, &se) {
		return se.Code
	}
	return ""
}

func TestListProductsEscapesWildcards(t *testing.T) {
	f := &refStoreFake{}
	_, err := NewReferenceService(f, t.TempDir()).ListProducts(context.Background(),
		ReferenceFilter{Q: " 50%_off\\ ", Ingredient: " Ibuprofen "}, Page{})
	if err != nil {
		t.Fatal(err)
	}
	if f.listParams.Q != `50\%\_off\\` || f.listParams.Ingredient != "Ibuprofen" || f.listParams.Limit != DefaultLimit {
		t.Fatalf("params = %+v", f.listParams)
	}
}

func validProduct() CreateGovProductInput {
	price := int64(1200)
	return CreateGovProductInput{Code: "6280000000099", RegNo: "R-1", BrandName: "Augmentin",
		Ingredient: "Amoxicillin/Clavulanate", Strength: "1000mg", DosageForm: "Tablet", OfficialPriceCents: &price}
}

func TestCreateProduct(t *testing.T) {
	neg := int64(-1)
	blank := "  "
	cases := []struct {
		name  string
		actor Actor
		edit  func(*CreateGovProductInput)
		err   error
		want  string
	}{
		{"ok, default source, form lowercased, blank document ignored", refStaff, func(in *CreateGovProductInput) { in.DocumentID = &blank }, nil, ""},
		{"agent forbidden", refAgent, nil, nil, CodeAgentForbidden},
		{"code not 13 digits", refStaff, func(in *CreateGovProductInput) { in.Code = "62800" }, nil, CodeInvalid},
		{"code with letters", refStaff, func(in *CreateGovProductInput) { in.Code = "628000000000A" }, nil, CodeInvalid},
		{"missing brand", refStaff, func(in *CreateGovProductInput) { in.BrandName = " " }, nil, CodeInvalid},
		{"missing price", refStaff, func(in *CreateGovProductInput) { in.OfficialPriceCents = nil }, nil, CodeInvalid},
		{"negative price", refStaff, func(in *CreateGovProductInput) { in.OfficialPriceCents = &neg }, nil, CodeInvalid},
		{"duplicate", refStaff, nil, queries.ErrDuplicate, CodeConflict},
		{"unknown document", refStaff, nil, queries.ErrBadReference, CodeInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validProduct()
			if tc.edit != nil {
				tc.edit(&in)
			}
			g, err := NewReferenceService(&refStoreFake{createErr: tc.err}, t.TempDir()).CreateProduct(context.Background(), tc.actor, in)
			if got := refCode(err); got != tc.want {
				t.Fatalf("code = %q, want %q (err %v)", got, tc.want, err)
			}
			if tc.want == "" && (g.Source != SourceStaffEntry || g.DocumentID != nil || g.DosageForm != "tablet") {
				t.Fatalf("row = %+v", g)
			}
		})
	}
}

var pdfBytes = []byte("%PDF-1.4\n1 0 obj << >> endobj\ntrailer << >>\n%%EOF\n")

func TestUploadDocument(t *testing.T) {
	cases := []struct {
		name  string
		actor Actor
		in    UploadDocumentInput
		want  string
	}{
		{"pdf ok", refStaff, UploadDocumentInput{Name: "../../etc/MoPH list.pdf", DeclaredType: "application/pdf", SniffedType: "application/pdf", Data: pdfBytes}, ""},
		{"octet-stream declared uses sniffed", refStaff, UploadDocumentInput{Name: "x", DeclaredType: "application/octet-stream", SniffedType: "application/pdf", Data: pdfBytes}, ""},
		{"jpg alias", refStaff, UploadDocumentInput{Name: "p.jpg", DeclaredType: "image/jpg", SniffedType: "image/jpeg", Data: []byte{0xff, 0xd8, 0xff}}, ""},
		{"agent forbidden", refAgent, UploadDocumentInput{SniffedType: "application/pdf", Data: pdfBytes}, CodeAgentForbidden},
		{"empty", refStaff, UploadDocumentInput{SniffedType: "text/plain; charset=utf-8"}, CodeInvalid},
		{"too big", refStaff, UploadDocumentInput{SniffedType: "application/pdf", Data: make([]byte, MaxReferenceDocumentBytes+1)}, CodeInvalid},
		{"html content rejected", refStaff, UploadDocumentInput{DeclaredType: "application/pdf", SniffedType: "text/html; charset=utf-8", Data: []byte("<html>")}, CodeInvalid},
		{"declared png but pdf content", refStaff, UploadDocumentInput{DeclaredType: "image/png", SniffedType: "application/pdf", Data: pdfBytes}, CodeInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			f := &refStoreFake{}
			doc, err := NewReferenceService(f, dir).UploadDocument(context.Background(), tc.actor, tc.in)
			if got := refCode(err); got != tc.want {
				t.Fatalf("code = %q, want %q (err %v)", got, tc.want, err)
			}
			files, _ := filepath.Glob(filepath.Join(dir, "reference", "*"))
			if tc.want != "" {
				if len(files) != 0 || len(f.docs) != 0 {
					t.Fatalf("rejected upload left files %v / rows %d", files, len(f.docs))
				}
				return
			}
			if len(files) != 1 || files[0] != doc.StoragePath || !strings.HasPrefix(filepath.Base(doc.StoragePath), doc.ID) {
				t.Fatalf("files = %v, doc = %+v", files, doc)
			}
			got, _ := os.ReadFile(files[0])
			sum := sha256.Sum256(tc.in.Data)
			if string(got) != string(tc.in.Data) || doc.SHA256 != hex.EncodeToString(sum[:]) || doc.SizeBytes != int64(len(tc.in.Data)) {
				t.Fatalf("stored file / sha / size mismatch: %+v", doc)
			}
			if strings.ContainsAny(doc.Name, "/\\") || doc.Name == "" {
				t.Fatalf("name not sanitised: %q", doc.Name)
			}
			if f.docs[0].UploadedByStaffID != "s-a" {
				t.Fatalf("uploader = %q", f.docs[0].UploadedByStaffID)
			}
		})
	}
}

func TestUploadDocumentRemovesFileWhenInsertFails(t *testing.T) {
	dir := t.TempDir()
	f := &refStoreFake{docErr: errors.New("db down")}
	_, err := NewReferenceService(f, dir).UploadDocument(context.Background(), refStaff,
		UploadDocumentInput{Name: "a.pdf", SniffedType: "application/pdf", Data: pdfBytes})
	if refCode(err) != CodeInternal {
		t.Fatalf("err = %v", err)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "reference", "*")); len(files) != 0 {
		t.Fatalf("orphan files: %v", files)
	}
}

func TestListDocumentsAgentForbidden(t *testing.T) {
	_, err := NewReferenceService(&refStoreFake{}, t.TempDir()).ListDocuments(context.Background(), refAgent, Page{})
	if refCode(err) != CodeAgentForbidden {
		t.Fatalf("err = %v", err)
	}
}

// ---- stock -------------------------------------------------------------------

type stockStoreFake struct {
	queries.Querier
	batch    *queries.InventoryBatch
	created  []queries.CreateInventoryBatchParams
	setQty   int
	setCalls int
}

func (f *stockStoreFake) InTx(ctx context.Context, fn func(q queries.Querier) error) error {
	return fn(f)
}

func (f *stockStoreFake) GetMedication(ctx context.Context, id string) (queries.Medication, error) {
	if id != "m1" {
		return queries.Medication{}, queries.ErrNotFound
	}
	return queries.Medication{ID: "m1"}, nil
}

func (f *stockStoreFake) GetStockBatchForUpdate(ctx context.Context, branchID, medicationID string) (queries.InventoryBatch, error) {
	if f.batch == nil {
		return queries.InventoryBatch{}, queries.ErrNotFound
	}
	return *f.batch, nil
}

func (f *stockStoreFake) SetBatchQuantityOnHand(ctx context.Context, id string, qty int) (queries.InventoryBatch, error) {
	f.setCalls++
	f.setQty = qty
	b := *f.batch
	b.QuantityOnHand = qty
	return b, nil
}

func (f *stockStoreFake) CreateInventoryBatch(ctx context.Context, p queries.CreateInventoryBatchParams) (queries.InventoryBatch, error) {
	f.created = append(f.created, p)
	return queries.InventoryBatch{ID: "new", BranchID: p.BranchID, MedicationID: p.MedicationID,
		BatchNumber: p.BatchNumber, ExpiryDate: p.ExpiryDate, QuantityOnHand: p.QuantityOnHand}, nil
}

func TestSetStock(t *testing.T) {
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	qty := func(n int) SetStockInput { return SetStockInput{QuantityOnHand: &n} }
	existing := func() *queries.InventoryBatch {
		return &queries.InventoryBatch{ID: "b1", BranchID: "b-a", MedicationID: "m1", QuantityOnHand: 9, QuantityReserved: 2}
	}
	cases := []struct {
		name   string
		actor  Actor
		branch string
		med    string
		in     SetStockInput
		batch  *queries.InventoryBatch
		want   string
	}{
		{"updates existing batch", refStaff, "b-a", "m1", qty(5), existing(), ""},
		{"creates a batch when none", refStaff, "b-a", "m1", qty(0), nil, ""},
		{"agent forbidden", refAgent, "b-a", "m1", qty(5), existing(), CodeAgentForbidden},
		{"other branch forbidden", refStaff, "b-c", "m1", qty(5), existing(), CodeForbidden},
		{"unknown medication", refStaff, "b-a", "nope", qty(5), existing(), CodeNotFound},
		{"missing quantity", refStaff, "b-a", "m1", SetStockInput{}, existing(), CodeInvalid},
		{"negative quantity", refStaff, "b-a", "m1", qty(-1), existing(), CodeInvalid},
		{"below reserved", refStaff, "b-a", "m1", qty(1), existing(), CodeConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &stockStoreFake{batch: tc.batch}
			b, err := NewStockService(f, func() time.Time { return now }).SetStock(context.Background(), tc.actor, tc.branch, tc.med, tc.in)
			if got := refCode(err); got != tc.want {
				t.Fatalf("code = %q, want %q (err %v)", got, tc.want, err)
			}
			if tc.want != "" {
				if f.setCalls != 0 || len(f.created) != 0 {
					t.Fatalf("rejected update wrote stock")
				}
				return
			}
			if tc.batch != nil {
				if f.setCalls != 1 || b.QuantityOnHand != *tc.in.QuantityOnHand || len(f.created) != 0 {
					t.Fatalf("batch = %+v, set calls %d", b, f.setCalls)
				}
				return
			}
			if len(f.created) != 1 || f.created[0].BatchNumber != StaffUpdateBatchNumber ||
				!f.created[0].ExpiryDate.Equal(now.AddDate(0, 18, 0)) || f.created[0].BranchID != "b-a" {
				t.Fatalf("created = %+v", f.created)
			}
		})
	}
}
