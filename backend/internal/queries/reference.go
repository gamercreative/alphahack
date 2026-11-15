package queries

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Government reference (gov_products, reference_documents; migration 0007) and
// the staff stock update (inventory_batches).

var (
	// ErrDuplicate: a unique constraint rejected the write (PostgreSQL 23505).
	ErrDuplicate = errors.New("queries: duplicate")
	// ErrBadReference: a foreign key rejected the write (PostgreSQL 23503).
	ErrBadReference = errors.New("queries: referenced row does not exist")
)

var (
	qListGovProducts         = mustSQL("list_gov_products")
	qCreateGovProduct        = mustSQL("create_gov_product")
	qCreateReferenceDocument = mustSQL("create_reference_document")
	qListReferenceDocuments  = mustSQL("list_reference_documents")
	qGetStockBatchForUpdate  = mustSQL("get_stock_batch_for_update")
	qSetBatchQuantityOnHand  = mustSQL("set_batch_quantity_on_hand")
	qCreateInventoryBatch    = mustSQL("create_inventory_batch")
)

// constraint maps unique / foreign-key violations to the sentinels above.
func constraint(op string, err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return ErrDuplicate
		case "23503":
			return ErrBadReference
		}
	}
	return wrap(op, err)
}

func scanGovProduct(r rowScanner) (GovProduct, error) {
	var g GovProduct
	err := r.Scan(&g.Code, &g.RegNo, &g.BrandName, &g.Ingredient, &g.Strength, &g.DosageForm,
		&g.OfficialPriceCents, &g.Source, &g.DocumentID, &g.CreatedAt)
	return g, err
}

func scanReferenceDocument(r rowScanner) (ReferenceDocument, error) {
	var d ReferenceDocument
	err := r.Scan(&d.ID, &d.Name, &d.ContentType, &d.SizeBytes, &d.SHA256,
		&d.UploadedByStaffID, &d.StoragePath, &d.CreatedAt)
	return d, err
}

func (q *Queries) ListGovProducts(ctx context.Context, p ListGovProductsParams) ([]GovProduct, error) {
	rows, err := q.db.Query(ctx, qListGovProducts, p.Q, p.Ingredient, p.Strength, p.DosageForm, p.Limit, p.Offset)
	if err != nil {
		return nil, wrap("list gov products", err)
	}
	return collect(rows, "list gov products", scanGovProduct)
}

func (q *Queries) CreateGovProduct(ctx context.Context, p CreateGovProductParams) (GovProduct, error) {
	g, err := scanGovProduct(q.db.QueryRow(ctx, qCreateGovProduct,
		p.Code, p.RegNo, p.BrandName, p.Ingredient, p.Strength, p.DosageForm,
		p.OfficialPriceCents, p.Source, p.DocumentID))
	if err != nil {
		return g, constraint("create gov product", err)
	}
	return g, nil
}

func (q *Queries) CreateReferenceDocument(ctx context.Context, p CreateReferenceDocumentParams) (ReferenceDocument, error) {
	d, err := scanReferenceDocument(q.db.QueryRow(ctx, qCreateReferenceDocument,
		p.ID, p.Name, p.ContentType, p.SizeBytes, p.SHA256, p.UploadedByStaffID, p.StoragePath))
	if err != nil {
		return d, constraint("create reference document", err)
	}
	return d, nil
}

func (q *Queries) ListReferenceDocuments(ctx context.Context, limit, offset int) ([]ReferenceDocument, error) {
	rows, err := q.db.Query(ctx, qListReferenceDocuments, limit, offset)
	if err != nil {
		return nil, wrap("list reference documents", err)
	}
	return collect(rows, "list reference documents", scanReferenceDocument)
}

func (q *Queries) GetStockBatchForUpdate(ctx context.Context, branchID, medicationID string) (InventoryBatch, error) {
	b, err := scanInventoryBatch(q.db.QueryRow(ctx, qGetStockBatchForUpdate, branchID, medicationID))
	return b, wrap("get stock batch for update", err)
}

func (q *Queries) SetBatchQuantityOnHand(ctx context.Context, batchID string, quantity int) (InventoryBatch, error) {
	b, err := scanInventoryBatch(q.db.QueryRow(ctx, qSetBatchQuantityOnHand, batchID, quantity))
	return b, wrap("set batch quantity on hand", err)
}

func (q *Queries) CreateInventoryBatch(ctx context.Context, p CreateInventoryBatchParams) (InventoryBatch, error) {
	b, err := scanInventoryBatch(q.db.QueryRow(ctx, qCreateInventoryBatch,
		p.BranchID, p.MedicationID, p.BatchNumber, p.ExpiryDate, p.QuantityOnHand))
	if err != nil {
		return b, constraint("create inventory batch", err)
	}
	return b, nil
}
