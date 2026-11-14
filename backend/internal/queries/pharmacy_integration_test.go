package queries

import (
	"context"
	"errors"
	"testing"
)

// Pharmacy integration tests run against the deterministic seed in
// db/migrations/0003_pharmacy_seed.up.sql. Every write happens inside InTx and
// is rolled back with errRollback, so the seed stays untouched.

var errRollback = errors.New("rollback")

func inRollbackTx(t *testing.T, q *Queries, fn func(tx Querier)) {
	t.Helper()
	err := q.InTx(context.Background(), func(tx Querier) error {
		fn(tx)
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("InTx: %v", err)
	}
}

func TestIntegrationPharmacyCatalog(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()

	branches, err := q.ListBranches(ctx, 50, 0)
	if err != nil || len(branches) < 3 {
		t.Fatalf("ListBranches = %d rows, %v; want >= 3", len(branches), err)
	}
	if _, err := q.GetBranch(ctx, "branch-nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBranch(missing) err = %v, want ErrNotFound", err)
	}

	found, err := q.SearchMedications(ctx, "paracetamol", 10, 0)
	if err != nil || len(found) != 1 || found[0].ID != "med-panadol-advance" {
		t.Fatalf("SearchMedications(paracetamol) = %+v, %v", found, err)
	}
	none, err := q.SearchMedications(ctx, "zzz-no-such-drug", 10, 0)
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("SearchMedications(no match) = %#v, %v; want empty non-nil slice", none, err)
	}
	m, err := q.GetMedication(ctx, "med-brufen-400")
	if err != nil || m.RxRequired || string(m.ContraindicationTags) != `["nsaid", "peptic_ulcer", "asthma"]` {
		t.Fatalf("GetMedication = %+v, %v", m, err)
	}
}

func TestIntegrationPharmacyFEFO(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()

	// FEFO: the 2027-03 Panadol batch beats the 2028-01 one.
	inRollbackTx(t, q, func(tx Querier) {
		b, err := tx.GetSelectFefoBatchForUpdate(ctx, "branch-byblos", "med-panadol-advance", 5)
		if err != nil || b.ID != "batch-byblos-panadol-a" {
			t.Errorf("FEFO panadol = %s, %v; want batch-byblos-panadol-a", b.ID, err)
		}
		// Too many for the earliest batch alone: falls through to the later one.
		b, err = tx.GetSelectFefoBatchForUpdate(ctx, "branch-byblos", "med-panadol-advance", 50)
		if err != nil || b.ID != "batch-byblos-panadol-b" {
			t.Errorf("FEFO panadol x50 = %s, %v; want batch-byblos-panadol-b", b.ID, err)
		}
		// Recalled Ventolin batch is skipped even though it expires first.
		b, err = tx.GetSelectFefoBatchForUpdate(ctx, "branch-byblos", "med-ventolin-inh", 1)
		if err != nil || b.ID != "batch-byblos-ventolin-b" {
			t.Errorf("FEFO ventolin = %s, %v; want batch-byblos-ventolin-b", b.ID, err)
		}
		// Only an expired Lipitor batch in Byblos.
		if _, err := tx.GetSelectFefoBatchForUpdate(ctx, "branch-byblos", "med-lipitor-20", 1); !errors.Is(err, ErrNotFound) {
			t.Errorf("FEFO expired lipitor err = %v, want ErrNotFound", err)
		}
		// Concor: out of stock in Byblos, 25 in Hamra.
		if _, err := tx.GetSelectFefoBatchForUpdate(ctx, "branch-byblos", "med-concor-5", 1); !errors.Is(err, ErrNotFound) {
			t.Errorf("FEFO concor byblos err = %v, want ErrNotFound", err)
		}
		b, err = tx.GetSelectFefoBatchForUpdate(ctx, "branch-hamra", "med-concor-5", 25)
		if err != nil || b.QuantityAvailable != 25 {
			t.Errorf("FEFO concor hamra = %+v, %v; want 25 available", b, err)
		}
	})

	inv, err := q.ListBranchInventory(ctx, "branch-byblos", "", 200, 0)
	if err != nil {
		t.Fatalf("ListBranchInventory: %v", err)
	}
	for i, b := range inv {
		if b.IsRecalled || b.QuantityAvailable <= 0 || b.ID == "batch-byblos-lipitor-x" {
			t.Errorf("ListBranchInventory returned unsellable batch %+v", b)
		}
		if i > 0 && inv[i-1].ExpiryDate.After(b.ExpiryDate) {
			t.Errorf("ListBranchInventory not ordered by expiry at %d", i)
		}
	}
}

func TestIntegrationPharmacyStockLifecycle(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()
	const batch = "batch-hamra-concor-a"

	inRollbackTx(t, q, func(tx Querier) {
		b, err := tx.ReserveBatchStock(ctx, batch, 5)
		if err != nil || b.QuantityReserved != 5 || b.QuantityAvailable != 20 {
			t.Fatalf("Reserve = %+v, %v", b, err)
		}
		if _, err := tx.ReserveBatchStock(ctx, batch, 21); !errors.Is(err, ErrNotFound) {
			t.Errorf("over-reserve err = %v, want ErrNotFound", err)
		}
		b, err = tx.CommitBatchStock(ctx, batch, 3)
		if err != nil || b.QuantityOnHand != 22 || b.QuantityReserved != 2 {
			t.Fatalf("Commit = %+v, %v", b, err)
		}
		b, err = tx.ReleaseBatchStock(ctx, batch, 2)
		if err != nil || b.QuantityReserved != 0 || b.QuantityAvailable != 22 {
			t.Fatalf("Release = %+v, %v", b, err)
		}
		if _, err := tx.ReleaseBatchStock(ctx, batch, 1); !errors.Is(err, ErrNotFound) {
			t.Errorf("over-release err = %v, want ErrNotFound", err)
		}
		if _, err := tx.ReserveBatchStock(ctx, "batch-byblos-ventolin-r", 1); !errors.Is(err, ErrNotFound) {
			t.Errorf("reserve recalled err = %v, want ErrNotFound", err)
		}
	})
}

func TestIntegrationPharmacyCustomersAndRx(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()

	c, err := q.GetCustomer(ctx, "cust-nadia")
	if err != nil || string(c.Allergies) != `["penicillin"]` {
		t.Fatalf("GetCustomer = %+v, %v", c, err)
	}
	verified, err := q.ListCustomerPrescriptions(ctx, "cust-nadia", PrescriptionStatusVerified, 50, 0)
	if err != nil || len(verified) != 2 {
		t.Fatalf("verified rx = %d, %v; want 2", len(verified), err)
	}
	subs, err := q.ListCustomerRefillSubscriptions(ctx, "cust-nadia")
	if err != nil || len(subs) != 1 || subs[0].Status != RefillStatusActive {
		t.Fatalf("refills = %+v, %v", subs, err)
	}

	inRollbackTx(t, q, func(tx Querier) {
		rx, err := tx.DecrementPrescriptionRefill(ctx, "rx-nadia-glucophage")
		if err != nil || rx.RemainingRefills != 4 {
			t.Errorf("DecrementPrescriptionRefill = %+v, %v", rx, err)
		}
		// Pending prescriptions cannot be used.
		if _, err := tx.DecrementPrescriptionRefill(ctx, "rx-karim-augmentin"); !errors.Is(err, ErrNotFound) {
			t.Errorf("decrement pending err = %v, want ErrNotFound", err)
		}
		up, err := tx.UpdateCustomerProfile(ctx, UpdateCustomerProfileParams{
			ID: "cust-rami", FullName: "Rami Saad", Email: "rami@example.com", Address: "Byblos",
		})
		if err != nil || string(up.Allergies) != "[]" {
			t.Errorf("UpdateCustomerProfile = %+v, %v", up, err)
		}
		sub, err := tx.UpdateRefillSubscriptionStatus(ctx, "refill-nadia-glucophage", RefillStatusPaused)
		if err != nil || sub.Status != RefillStatusPaused {
			t.Errorf("UpdateRefillSubscriptionStatus = %+v, %v", sub, err)
		}
	})
}

func TestIntegrationPharmacyCartToOrder(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()

	cart, err := q.GetActiveCustomerCart(ctx, "cust-rami")
	if err != nil || cart.ID != "cart-rami-1" {
		t.Fatalf("GetActiveCustomerCart = %+v, %v", cart, err)
	}
	if _, err := q.GetActiveCustomerCart(ctx, "cust-karim"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no active cart err = %v, want ErrNotFound", err)
	}

	inRollbackTx(t, q, func(tx Querier) {
		it, err := tx.UpsertCartItem(ctx, UpsertCartItemParams{
			CartID: cart.ID, MedicationID: "med-panadol-advance", Quantity: 3, UnitPriceCents: 450,
		})
		if err != nil || it.ID != "cart-item-rami-panadol" || it.Quantity != 3 {
			t.Fatalf("UpsertCartItem (update) = %+v, %v", it, err)
		}
		if err := tx.DeleteCartItem(ctx, cart.ID, "med-nexium-20"); err != nil {
			t.Fatalf("DeleteCartItem: %v", err)
		}
		if err := tx.DeleteCartItem(ctx, cart.ID, "med-nexium-20"); !errors.Is(err, ErrNotFound) {
			t.Errorf("DeleteCartItem twice err = %v, want ErrNotFound", err)
		}
		items, err := tx.ListCartItems(ctx, cart.ID)
		if err != nil || len(items) != 1 {
			t.Fatalf("ListCartItems = %+v, %v", items, err)
		}

		cartID := cart.ID
		o, err := tx.CreateOrder(ctx, CreateOrderParams{
			CustomerID: "cust-rami", BranchID: "branch-byblos", CartID: &cartID,
			Status: OrderStatusConfirmed, FulfillmentType: FulfillmentPickup, TotalCents: 1350,
		})
		if err != nil || o.ID == "" {
			t.Fatalf("CreateOrder = %+v, %v", o, err)
		}
		oi, err := tx.CreateOrderItem(ctx, CreateOrderItemParams{
			OrderID: o.ID, MedicationID: "med-panadol-advance", BatchID: "batch-byblos-panadol-a",
			Quantity: 3, UnitPriceCents: 450,
		})
		if err != nil || oi.OrderID != o.ID {
			t.Fatalf("CreateOrderItem = %+v, %v", oi, err)
		}
		if got, err := tx.ListOrderItems(ctx, o.ID); err != nil || len(got) != 1 {
			t.Fatalf("ListOrderItems = %+v, %v", got, err)
		}
		if c, err := tx.UpdateCartStatus(ctx, cart.ID, CartStatusCheckedOut); err != nil || c.Status != CartStatusCheckedOut {
			t.Fatalf("UpdateCartStatus = %+v, %v", c, err)
		}
		orders, err := tx.ListCustomerOrders(ctx, "cust-rami", 10, 0)
		if err != nil || len(orders) != 1 {
			t.Fatalf("ListCustomerOrders = %+v, %v", orders, err)
		}
	})

	empty, err := q.ListOrderItems(ctx, "order-does-not-exist")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("ListOrderItems(missing) = %#v, %v; want empty non-nil slice", empty, err)
	}
}
