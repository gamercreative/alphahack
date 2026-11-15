package services

// Staff stock update: a pharmacist sets the on-hand quantity of one medicine at
// their own branch. The agent never can (CONTRACT rule 2: no tool marks stock).

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alphahack/internal/queries"
)

const (
	// MaxStockQuantity caps one quantity_on_hand value.
	MaxStockQuantity = 1_000_000
	// StaffUpdateBatchNumber is the lot of a batch created by a stock update.
	StaffUpdateBatchNumber = "STAFF-UPDATE"
	// staffBatchMonths is the shelf life given to such a batch.
	staffBatchMonths = 18
)

type StockStore interface {
	InTx(ctx context.Context, fn func(q queries.Querier) error) error
	GetMedication(ctx context.Context, id string) (queries.Medication, error)
}

type StockService struct {
	store StockStore
	now   func() time.Time
}

// NewStockService: now nil = time.Now.
func NewStockService(store StockStore, now func() time.Time) *StockService {
	if now == nil {
		now = time.Now
	}
	return &StockService{store: store, now: now}
}

type SetStockInput struct {
	QuantityOnHand *int `json:"quantity_on_hand"`
}

// SetStock sets quantity_on_hand on the soonest-expiring unexpired, unrecalled
// batch of medicationID at branchID, or creates a STAFF-UPDATE batch when there
// is none. Staff at that branch only.
func (s *StockService) SetStock(ctx context.Context, a Actor, branchID, medicationID string, in SetStockInput) (queries.InventoryBatch, error) {
	if a.IsAgent() {
		return queries.InventoryBatch{}, AgentForbidden()
	}
	if a.BranchID == "" || a.BranchID != branchID {
		return queries.InventoryBatch{}, Forbidden("you can only update stock for your own pharmacy")
	}
	if in.QuantityOnHand == nil {
		return queries.InventoryBatch{}, Invalid("quantity_on_hand is required: a whole number of 0 or more")
	}
	qty := *in.QuantityOnHand
	if qty < 0 || qty > MaxStockQuantity {
		return queries.InventoryBatch{}, Invalid("quantity_on_hand must be between 0 and %d", MaxStockQuantity)
	}
	if _, err := s.store.GetMedication(ctx, medicationID); err != nil {
		return queries.InventoryBatch{}, FromStore(err, "medication")
	}

	var out queries.InventoryBatch
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		b, err := q.GetStockBatchForUpdate(ctx, branchID, medicationID)
		if errors.Is(err, queries.ErrNotFound) {
			out, err = q.CreateInventoryBatch(ctx, queries.CreateInventoryBatchParams{
				BranchID: branchID, MedicationID: medicationID, BatchNumber: StaffUpdateBatchNumber,
				ExpiryDate: s.now().UTC().AddDate(0, staffBatchMonths, 0), QuantityOnHand: qty,
			})
			if errors.Is(err, queries.ErrBadReference) {
				return NotFound("branch")
			}
			return FromStore(err, "inventory")
		}
		if err != nil {
			return FromStore(err, "inventory")
		}
		if b.QuantityReserved > qty {
			return Conflict(fmt.Sprintf("this batch has %d units reserved; quantity_on_hand cannot be set below that", b.QuantityReserved))
		}
		out, err = q.SetBatchQuantityOnHand(ctx, b.ID, qty)
		return FromStore(err, "inventory")
	})
	if err != nil {
		return queries.InventoryBatch{}, err
	}
	return out, nil
}
