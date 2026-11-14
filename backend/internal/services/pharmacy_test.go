package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"alphahack/internal/queries"
)

type fakePharmacyStore struct {
	queries.Querier

	branches      map[string]queries.Branch
	medications   map[string]queries.Medication
	batches       map[string]queries.InventoryBatch
	customers     map[string]queries.Customer
	prescriptions map[string]queries.Prescription
	carts         map[string]queries.Cart
	cartItems     map[string][]queries.CartItem
	orders        map[string]queries.Order
	orderItems    map[string][]queries.OrderItem
	refills       map[string]queries.RefillSubscription

	seq int
}

func newSeededFakePharmacyStore() *fakePharmacyStore {
	now := time.Now().UTC()
	futureEarly := now.AddDate(0, 3, 0)
	futureLate := now.AddDate(0, 9, 0)
	past := now.AddDate(0, -1, 0)

	f := &fakePharmacyStore{
		branches: map[string]queries.Branch{
			"branch-hamra": {
				ID:               "branch-hamra",
				Name:             "Hamra Flagship",
				City:             "Beirut",
				Address:          "Hamra Main Street",
				Phone:            "+961-1-340000",
				SupportsDelivery: true,
				CreatedAt:        now,
			},
			"branch-byblos": {
				ID:               "branch-byblos",
				Name:             "Byblos Old Souk",
				City:             "Jbeil",
				Address:          "Voie Romaine",
				Phone:            "+961-9-540000",
				SupportsDelivery: false,
				CreatedAt:        now,
			},
		},
		medications: map[string]queries.Medication{
			"med-panadol-500": {
				ID:                   "med-panadol-500",
				BrandName:            "Panadol Advance",
				ActiveIngredient:     "Paracetamol",
				TherapeuticClass:     "Analgesic",
				DosageForm:           queries.DosageFormTablet,
				Strength:             "500mg",
				RxRequired:           false,
				Controlled:           false,
				ChronicEligible:      false,
				UnitPriceCents:       450,
				ContraindicationTags: json.RawMessage(`["severe_hepatic_impairment"]`),
			},
			"med-augmentin-1g": {
				ID:                   "med-augmentin-1g",
				BrandName:            "Augmentin",
				ActiveIngredient:     "Amoxicillin / Clavulanic Acid",
				TherapeuticClass:     "Antibiotic",
				DosageForm:           queries.DosageFormTablet,
				Strength:             "1g",
				RxRequired:           true,
				Controlled:           false,
				ChronicEligible:      false,
				UnitPriceCents:       1850,
				ContraindicationTags: json.RawMessage(`["penicillin", "beta_lactam"]`),
			},
			"med-brufen-400": {
				ID:                   "med-brufen-400",
				BrandName:            "Brufen",
				ActiveIngredient:     "Ibuprofen",
				TherapeuticClass:     "NSAID",
				DosageForm:           queries.DosageFormTablet,
				Strength:             "400mg",
				RxRequired:           false,
				Controlled:           false,
				ChronicEligible:      false,
				UnitPriceCents:       600,
				ContraindicationTags: json.RawMessage(`["nsaid", "asthma", "peptic_ulcer"]`),
			},
			"med-concor-5": {
				ID:                   "med-concor-5",
				BrandName:            "Concor",
				ActiveIngredient:     "Bisoprolol",
				TherapeuticClass:     "Beta Blocker",
				DosageForm:           queries.DosageFormTablet,
				Strength:             "5mg",
				RxRequired:           true,
				Controlled:           false,
				ChronicEligible:      true,
				UnitPriceCents:       1200,
				ContraindicationTags: json.RawMessage(`["severe_bradycardia"]`),
			},
			"med-lexotanil-3": {
				ID:                   "med-lexotanil-3",
				BrandName:            "Lexotanil",
				ActiveIngredient:     "Bromazepam",
				TherapeuticClass:     "Benzodiazepine",
				DosageForm:           queries.DosageFormTablet,
				Strength:             "3mg",
				RxRequired:           true,
				Controlled:           true,
				ChronicEligible:      false,
				UnitPriceCents:       1500,
				ContraindicationTags: json.RawMessage(`["respiratory_depression"]`),
			},
		},
		batches: map[string]queries.InventoryBatch{
			// Concor in Hamra: 10 early + 15 late = 25 available, plus 1 recalled and 1 expired (excluded)
			"batch-concor-hamra-early": {
				ID:                "batch-concor-hamra-early",
				BranchID:          "branch-hamra",
				MedicationID:      "med-concor-5",
				BatchNumber:       "CON-H-01",
				ExpiryDate:        futureEarly,
				QuantityOnHand:    12,
				QuantityReserved:  2,
				QuantityAvailable: 10,
				IsRecalled:        false,
			},
			"batch-concor-hamra-late": {
				ID:                "batch-concor-hamra-late",
				BranchID:          "branch-hamra",
				MedicationID:      "med-concor-5",
				BatchNumber:       "CON-H-02",
				ExpiryDate:        futureLate,
				QuantityOnHand:    15,
				QuantityReserved:  0,
				QuantityAvailable: 15,
				IsRecalled:        false,
			},
			"batch-concor-hamra-recalled": {
				ID:                "batch-concor-hamra-recalled",
				BranchID:          "branch-hamra",
				MedicationID:      "med-concor-5",
				BatchNumber:       "CON-H-REC",
				ExpiryDate:        futureLate,
				QuantityOnHand:    50,
				QuantityReserved:  0,
				QuantityAvailable: 50,
				IsRecalled:        true,
				RecallReason:      "Packaging defect",
			},
			"batch-concor-hamra-expired": {
				ID:                "batch-concor-hamra-expired",
				BranchID:          "branch-hamra",
				MedicationID:      "med-concor-5",
				BatchNumber:       "CON-H-EXP",
				ExpiryDate:        past,
				QuantityOnHand:    30,
				QuantityReserved:  0,
				QuantityAvailable: 30,
				IsRecalled:        false,
			},
			// Concor in Byblos: 0 available (only expired/recalled or 0 on hand)
			"batch-concor-byblos-empty": {
				ID:                "batch-concor-byblos-empty",
				BranchID:          "branch-byblos",
				MedicationID:      "med-concor-5",
				BatchNumber:       "CON-B-01",
				ExpiryDate:        futureEarly,
				QuantityOnHand:    5,
				QuantityReserved:  5,
				QuantityAvailable: 0,
				IsRecalled:        false,
			},
			// Panadol in Hamra: 2 batches for FEFO test
			"batch-panadol-hamra-late": {
				ID:                "batch-panadol-hamra-late",
				BranchID:          "branch-hamra",
				MedicationID:      "med-panadol-500",
				BatchNumber:       "PAN-H-LATE",
				ExpiryDate:        futureLate,
				QuantityOnHand:    40,
				QuantityReserved:  0,
				QuantityAvailable: 40,
				IsRecalled:        false,
			},
			"batch-panadol-hamra-early": {
				ID:                "batch-panadol-hamra-early",
				BranchID:          "branch-hamra",
				MedicationID:      "med-panadol-500",
				BatchNumber:       "PAN-H-EARLY",
				ExpiryDate:        futureEarly,
				QuantityOnHand:    20,
				QuantityReserved:  0,
				QuantityAvailable: 20,
				IsRecalled:        false,
			},
			// Augmentin in Hamra
			"batch-augmentin-hamra": {
				ID:                "batch-augmentin-hamra",
				BranchID:          "branch-hamra",
				MedicationID:      "med-augmentin-1g",
				BatchNumber:       "AUG-H-01",
				ExpiryDate:        futureLate,
				QuantityOnHand:    20,
				QuantityReserved:  0,
				QuantityAvailable: 20,
				IsRecalled:        false,
			},
			// Brufen in Hamra
			"batch-brufen-hamra": {
				ID:                "batch-brufen-hamra",
				BranchID:          "branch-hamra",
				MedicationID:      "med-brufen-400",
				BatchNumber:       "BRU-H-01",
				ExpiryDate:        futureLate,
				QuantityOnHand:    20,
				QuantityReserved:  0,
				QuantityAvailable: 20,
				IsRecalled:        false,
			},
			// Lexotanil in Hamra
			"batch-lexotanil-hamra": {
				ID:                "batch-lexotanil-hamra",
				BranchID:          "branch-hamra",
				MedicationID:      "med-lexotanil-3",
				BatchNumber:       "LEX-H-01",
				ExpiryDate:        futureLate,
				QuantityOnHand:    10,
				QuantityReserved:  0,
				QuantityAvailable: 10,
				IsRecalled:        false,
			},
		},
		customers: map[string]queries.Customer{
			"cust-nadia": {
				ID:                "cust-nadia",
				FullName:          "Nadia Khoury",
				Phone:             "+961-3-111222",
				Email:             "nadia@example.com",
				Address:           "Bliss Street, Beirut",
				Allergies:         json.RawMessage(`["Penicillin"]`),
				ChronicConditions: json.RawMessage(`["hypertension"]`),
				CreatedAt:         now,
				UpdatedAt:         now,
			},
			"cust-karim": {
				ID:                "cust-karim",
				FullName:          "Karim Haddad",
				Phone:             "+961-3-333444",
				Email:             "karim@example.com",
				Address:           "Achrafieh, Beirut",
				Allergies:         json.RawMessage(`["NSAID"]`),
				ChronicConditions: json.RawMessage(`["Asthma"]`),
				CreatedAt:         now,
				UpdatedAt:         now,
			},
		},
		prescriptions: map[string]queries.Prescription{
			"rx-nadia-concor-verified": {
				ID:                 "rx-nadia-concor-verified",
				CustomerID:         "cust-nadia",
				MedicationID:       "med-concor-5",
				DoctorName:         "Dr. Samir Nassar",
				DosageInstructions: "1 tablet daily",
				AllowedRefills:     6,
				RemainingRefills:   3,
				Status:             queries.PrescriptionStatusVerified,
				ExpiresAt:          futureLate,
				CreatedAt:          now,
			},
			"rx-nadia-concor-pending": {
				ID:                 "rx-nadia-concor-pending",
				CustomerID:         "cust-nadia",
				MedicationID:       "med-concor-5",
				DoctorName:         "Dr. Samir Nassar",
				DosageInstructions: "1 tablet daily",
				AllowedRefills:     3,
				RemainingRefills:   3,
				Status:             queries.PrescriptionStatusPending,
				ExpiresAt:          futureLate,
				CreatedAt:          now,
			},
			"rx-nadia-concor-exhausted": {
				ID:                 "rx-nadia-concor-exhausted",
				CustomerID:         "cust-nadia",
				MedicationID:       "med-concor-5",
				DoctorName:         "Dr. Samir Nassar",
				DosageInstructions: "1 tablet daily",
				AllowedRefills:     3,
				RemainingRefills:   0,
				Status:             queries.PrescriptionStatusVerified,
				ExpiresAt:          futureLate,
				CreatedAt:          now,
			},
			"rx-karim-augmentin-verified": {
				ID:                 "rx-karim-augmentin-verified",
				CustomerID:         "cust-karim",
				MedicationID:       "med-augmentin-1g",
				DoctorName:         "Dr. Rania Saab",
				DosageInstructions: "1 tablet twice daily",
				AllowedRefills:     1,
				RemainingRefills:   1,
				Status:             queries.PrescriptionStatusVerified,
				ExpiresAt:          futureLate,
				CreatedAt:          now,
			},
		},
		carts:      make(map[string]queries.Cart),
		cartItems:  make(map[string][]queries.CartItem),
		orders:     make(map[string]queries.Order),
		orderItems: make(map[string][]queries.OrderItem),
		refills:    make(map[string]queries.RefillSubscription),
	}
	return f
}

func (f *fakePharmacyStore) nextID(prefix string) string {
	f.seq++
	return fmt.Sprintf("%s-%d", prefix, f.seq)
}

func (f *fakePharmacyStore) InTx(ctx context.Context, fn func(q queries.Querier) error) error {
	return fn(f)
}

func (f *fakePharmacyStore) ListBranches(ctx context.Context, limit, offset int) ([]queries.Branch, error) {
	out := make([]queries.Branch, 0, len(f.branches))
	for _, b := range f.branches {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (f *fakePharmacyStore) GetBranch(ctx context.Context, id string) (queries.Branch, error) {
	b, ok := f.branches[id]
	if !ok {
		return queries.Branch{}, queries.ErrNotFound
	}
	return b, nil
}

func (f *fakePharmacyStore) ListMedications(ctx context.Context, limit, offset int) ([]queries.Medication, error) {
	out := make([]queries.Medication, 0, len(f.medications))
	for _, m := range f.medications {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BrandName < out[j].BrandName })
	return out, nil
}

func (f *fakePharmacyStore) SearchMedications(ctx context.Context, query string, limit, offset int) ([]queries.Medication, error) {
	q := strings.ToLower(query)
	out := []queries.Medication{}
	for _, m := range f.medications {
		if strings.Contains(strings.ToLower(m.BrandName), q) ||
			strings.Contains(strings.ToLower(m.ActiveIngredient), q) ||
			strings.Contains(strings.ToLower(m.TherapeuticClass), q) {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BrandName < out[j].BrandName })
	return out, nil
}

func (f *fakePharmacyStore) GetMedication(ctx context.Context, id string) (queries.Medication, error) {
	m, ok := f.medications[id]
	if !ok {
		return queries.Medication{}, queries.ErrNotFound
	}
	return m, nil
}

func (f *fakePharmacyStore) ListBranchInventory(ctx context.Context, branchID, medicationID string, limit, offset int) ([]queries.InventoryBatch, error) {
	out := []queries.InventoryBatch{}
	for _, b := range f.batches {
		if b.BranchID != branchID {
			continue
		}
		if medicationID != "" && b.MedicationID != medicationID {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ExpiryDate.Equal(out[j].ExpiryDate) {
			return out[i].ExpiryDate.Before(out[j].ExpiryDate)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (f *fakePharmacyStore) GetSelectFefoBatchForUpdate(ctx context.Context, branchID, medicationID string, quantity int) (queries.InventoryBatch, error) {
	now := time.Now()
	var candidates []queries.InventoryBatch
	for _, b := range f.batches {
		avail := b.QuantityOnHand - b.QuantityReserved
		if b.BranchID == branchID && b.MedicationID == medicationID && !b.IsRecalled && b.ExpiryDate.After(now) && avail >= quantity {
			candidates = append(candidates, b)
		}
	}
	if len(candidates) == 0 {
		return queries.InventoryBatch{}, queries.ErrNotFound
	}
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].ExpiryDate.Equal(candidates[j].ExpiryDate) {
			return candidates[i].ExpiryDate.Before(candidates[j].ExpiryDate)
		}
		return candidates[i].ID < candidates[j].ID
	})
	return candidates[0], nil
}

func (f *fakePharmacyStore) ReserveBatchStock(ctx context.Context, batchID string, quantity int) (queries.InventoryBatch, error) {
	b, ok := f.batches[batchID]
	if !ok || b.IsRecalled || !b.ExpiryDate.After(time.Now()) || (b.QuantityOnHand-b.QuantityReserved) < quantity {
		return queries.InventoryBatch{}, queries.ErrNotFound
	}
	b.QuantityReserved += quantity
	b.QuantityAvailable = b.QuantityOnHand - b.QuantityReserved
	f.batches[batchID] = b
	return b, nil
}

func (f *fakePharmacyStore) CommitBatchStock(ctx context.Context, batchID string, quantity int) (queries.InventoryBatch, error) {
	b, ok := f.batches[batchID]
	if !ok || b.QuantityReserved < quantity || b.QuantityOnHand < quantity {
		return queries.InventoryBatch{}, queries.ErrNotFound
	}
	b.QuantityOnHand -= quantity
	b.QuantityReserved -= quantity
	b.QuantityAvailable = b.QuantityOnHand - b.QuantityReserved
	f.batches[batchID] = b
	return b, nil
}

func (f *fakePharmacyStore) ReleaseBatchStock(ctx context.Context, batchID string, quantity int) (queries.InventoryBatch, error) {
	b, ok := f.batches[batchID]
	if !ok || b.QuantityReserved < quantity {
		return queries.InventoryBatch{}, queries.ErrNotFound
	}
	b.QuantityReserved -= quantity
	b.QuantityAvailable = b.QuantityOnHand - b.QuantityReserved
	f.batches[batchID] = b
	return b, nil
}

func (f *fakePharmacyStore) GetCustomer(ctx context.Context, id string) (queries.Customer, error) {
	c, ok := f.customers[id]
	if !ok {
		return queries.Customer{}, queries.ErrNotFound
	}
	return c, nil
}

func (f *fakePharmacyStore) ListCustomers(ctx context.Context, limit, offset int) ([]queries.Customer, error) {
	out := make([]queries.Customer, 0, len(f.customers))
	for _, c := range f.customers {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullName < out[j].FullName })
	return out, nil
}

func (f *fakePharmacyStore) UpdateCustomerProfile(ctx context.Context, p queries.UpdateCustomerProfileParams) (queries.Customer, error) {
	c, ok := f.customers[p.ID]
	if !ok {
		return queries.Customer{}, queries.ErrNotFound
	}
	c.FullName = p.FullName
	c.Email = p.Email
	c.Address = p.Address
	c.Allergies = p.Allergies
	c.ChronicConditions = p.ChronicConditions
	c.UpdatedAt = time.Now().UTC()
	f.customers[p.ID] = c
	return c, nil
}

func (f *fakePharmacyStore) ListCustomerPrescriptions(ctx context.Context, customerID, status string, limit, offset int) ([]queries.Prescription, error) {
	out := []queries.Prescription{}
	for _, rx := range f.prescriptions {
		if rx.CustomerID == customerID && (status == "" || rx.Status == status) {
			out = append(out, rx)
		}
	}
	return out, nil
}

func (f *fakePharmacyStore) GetPrescription(ctx context.Context, id string) (queries.Prescription, error) {
	rx, ok := f.prescriptions[id]
	if !ok {
		return queries.Prescription{}, queries.ErrNotFound
	}
	return rx, nil
}

func (f *fakePharmacyStore) CreatePrescription(ctx context.Context, p queries.CreatePrescriptionParams) (queries.Prescription, error) {
	rx := queries.Prescription{
		ID:                 f.nextID("rx"),
		CustomerID:         p.CustomerID,
		MedicationID:       p.MedicationID,
		DoctorName:         p.DoctorName,
		DosageInstructions: p.DosageInstructions,
		AllowedRefills:     p.AllowedRefills,
		RemainingRefills:   p.AllowedRefills,
		Status:             p.Status,
		Notes:              p.Notes,
		ExpiresAt:          p.ExpiresAt,
		CreatedAt:          time.Now().UTC(),
	}
	f.prescriptions[rx.ID] = rx
	return rx, nil
}

func (f *fakePharmacyStore) DecrementPrescriptionRefill(ctx context.Context, id string) (queries.Prescription, error) {
	rx, ok := f.prescriptions[id]
	if !ok || rx.Status != queries.PrescriptionStatusVerified || rx.RemainingRefills <= 0 || !rx.ExpiresAt.After(time.Now()) {
		return queries.Prescription{}, queries.ErrNotFound
	}
	rx.RemainingRefills--
	f.prescriptions[id] = rx
	return rx, nil
}

func (f *fakePharmacyStore) GetCart(ctx context.Context, id string) (queries.Cart, error) {
	c, ok := f.carts[id]
	if !ok {
		return queries.Cart{}, queries.ErrNotFound
	}
	return c, nil
}

func (f *fakePharmacyStore) GetActiveCustomerCart(ctx context.Context, customerID string) (queries.Cart, error) {
	for _, c := range f.carts {
		if c.CustomerID == customerID && c.Status == queries.CartStatusActive {
			return c, nil
		}
	}
	return queries.Cart{}, queries.ErrNotFound
}

func (f *fakePharmacyStore) CreateCart(ctx context.Context, p queries.CreateCartParams) (queries.Cart, error) {
	now := time.Now().UTC()
	c := queries.Cart{
		ID:         f.nextID("cart"),
		CustomerID: p.CustomerID,
		BranchID:   p.BranchID,
		Status:     queries.CartStatusActive,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	f.carts[c.ID] = c
	return c, nil
}

func (f *fakePharmacyStore) UpdateCartStatus(ctx context.Context, id, status string) (queries.Cart, error) {
	c, ok := f.carts[id]
	if !ok {
		return queries.Cart{}, queries.ErrNotFound
	}
	c.Status = status
	c.UpdatedAt = time.Now().UTC()
	f.carts[id] = c
	return c, nil
}

func (f *fakePharmacyStore) ListCartItems(ctx context.Context, cartID string) ([]queries.CartItem, error) {
	items := f.cartItems[cartID]
	if items == nil {
		return []queries.CartItem{}, nil
	}
	out := make([]queries.CartItem, len(items))
	copy(out, items)
	return out, nil
}

func (f *fakePharmacyStore) UpsertCartItem(ctx context.Context, p queries.UpsertCartItemParams) (queries.CartItem, error) {
	items := f.cartItems[p.CartID]
	for i, it := range items {
		if it.MedicationID == p.MedicationID {
			it.PrescriptionID = p.PrescriptionID
			it.Quantity = p.Quantity
			it.UnitPriceCents = p.UnitPriceCents
			items[i] = it
			f.cartItems[p.CartID] = items
			return it, nil
		}
	}
	it := queries.CartItem{
		ID:             f.nextID("citem"),
		CartID:         p.CartID,
		MedicationID:   p.MedicationID,
		PrescriptionID: p.PrescriptionID,
		Quantity:       p.Quantity,
		UnitPriceCents: p.UnitPriceCents,
		CreatedAt:      time.Now().UTC(),
	}
	f.cartItems[p.CartID] = append(items, it)
	return it, nil
}

func (f *fakePharmacyStore) DeleteCartItem(ctx context.Context, cartID, medicationID string) error {
	items := f.cartItems[cartID]
	for i, it := range items {
		if it.MedicationID == medicationID {
			f.cartItems[cartID] = append(items[:i], items[i+1:]...)
			return nil
		}
	}
	return queries.ErrNotFound
}

func (f *fakePharmacyStore) CreateOrder(ctx context.Context, p queries.CreateOrderParams) (queries.Order, error) {
	now := time.Now().UTC()
	o := queries.Order{
		ID:              f.nextID("order"),
		CustomerID:      p.CustomerID,
		BranchID:        p.BranchID,
		CartID:          p.CartID,
		Status:          p.Status,
		FulfillmentType: p.FulfillmentType,
		DeliveryAddress: p.DeliveryAddress,
		TotalCents:      p.TotalCents,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	f.orders[o.ID] = o
	return o, nil
}

func (f *fakePharmacyStore) GetOrder(ctx context.Context, id string) (queries.Order, error) {
	o, ok := f.orders[id]
	if !ok {
		return queries.Order{}, queries.ErrNotFound
	}
	return o, nil
}

func (f *fakePharmacyStore) ListCustomerOrders(ctx context.Context, customerID string, limit, offset int) ([]queries.Order, error) {
	out := []queries.Order{}
	for _, o := range f.orders {
		if o.CustomerID == customerID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *fakePharmacyStore) CreateOrderItem(ctx context.Context, p queries.CreateOrderItemParams) (queries.OrderItem, error) {
	oi := queries.OrderItem{
		ID:             f.nextID("oitem"),
		OrderID:        p.OrderID,
		MedicationID:   p.MedicationID,
		BatchID:        p.BatchID,
		Quantity:       p.Quantity,
		UnitPriceCents: p.UnitPriceCents,
		CreatedAt:      time.Now().UTC(),
	}
	f.orderItems[p.OrderID] = append(f.orderItems[p.OrderID], oi)
	return oi, nil
}

func (f *fakePharmacyStore) ListOrderItems(ctx context.Context, orderID string) ([]queries.OrderItem, error) {
	items := f.orderItems[orderID]
	if items == nil {
		return []queries.OrderItem{}, nil
	}
	return items, nil
}

func (f *fakePharmacyStore) ListCustomerRefillSubscriptions(ctx context.Context, customerID string) ([]queries.RefillSubscription, error) {
	out := []queries.RefillSubscription{}
	for _, r := range f.refills {
		if r.CustomerID == customerID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakePharmacyStore) CreateRefillSubscription(ctx context.Context, p queries.CreateRefillSubscriptionParams) (queries.RefillSubscription, error) {
	r := queries.RefillSubscription{
		ID:             f.nextID("refill"),
		CustomerID:     p.CustomerID,
		MedicationID:   p.MedicationID,
		BranchID:       p.BranchID,
		PrescriptionID: p.PrescriptionID,
		IntervalDays:   p.IntervalDays,
		NextRefillAt:   p.NextRefillAt,
		Status:         queries.RefillStatusActive,
		CreatedAt:      time.Now().UTC(),
	}
	f.refills[r.ID] = r
	return r, nil
}

func (f *fakePharmacyStore) UpdateRefillSubscriptionStatus(ctx context.Context, id, status string) (queries.RefillSubscription, error) {
	r, ok := f.refills[id]
	if !ok {
		return queries.RefillSubscription{}, queries.ErrNotFound
	}
	r.Status = status
	f.refills[id] = r
	return r, nil
}

func assertServiceErrorCode(t *testing.T, err error, wantCode string, wantMsgSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %q, got nil", wantCode)
	}
	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *services.Error, got %T (%v)", err, err)
	}
	if svcErr.Code != wantCode {
		t.Fatalf("expected error code %q, got %q (message: %q)", wantCode, svcErr.Code, svcErr.Message)
	}
	if wantMsgSubstr != "" && !strings.Contains(svcErr.Message, wantMsgSubstr) {
		t.Fatalf("expected error message containing %q, got %q", wantMsgSubstr, svcErr.Message)
	}
}

func TestGetMedicationDetail_CrossBranchAvailability(t *testing.T) {
	ctx := context.Background()
	store := newSeededFakePharmacyStore()
	svc := NewPharmacyService(store)

	detail, err := svc.GetMedicationDetail(ctx, "med-concor-5")
	if err != nil {
		t.Fatalf("GetMedicationDetail unexpected error: %v", err)
	}
	if detail.Medication.ID != "med-concor-5" {
		t.Fatalf("expected medication ID med-concor-5, got %s", detail.Medication.ID)
	}
	if len(detail.BranchAvailability) != 2 {
		t.Fatalf("expected 2 branch summaries, got %d", len(detail.BranchAvailability))
	}

	byBranch := make(map[string]BranchStockSummary)
	for _, s := range detail.BranchAvailability {
		byBranch[s.BranchID] = s
	}

	byblos := byBranch["branch-byblos"]
	if byblos.AvailableQuantity != 0 {
		t.Errorf("expected 0 available in Byblos, got %d", byblos.AvailableQuantity)
	}
	if byblos.EarliestExpiry != nil {
		t.Errorf("expected nil EarliestExpiry in Byblos when stock is 0, got %v", byblos.EarliestExpiry)
	}

	hamra := byBranch["branch-hamra"]
	// 10 from early + 15 from late = 25 (recalled 50 and expired 30 excluded)
	if hamra.AvailableQuantity != 25 {
		t.Errorf("expected 25 available in Hamra, got %d", hamra.AvailableQuantity)
	}
	if hamra.EarliestExpiry == nil {
		t.Fatalf("expected non-nil EarliestExpiry in Hamra")
	}
	wantEarly := store.batches["batch-concor-hamra-early"].ExpiryDate
	if !hamra.EarliestExpiry.Equal(wantEarly) {
		t.Errorf("expected EarliestExpiry %v, got %v", wantEarly, *hamra.EarliestExpiry)
	}
}

func TestAddCartItem_SafetyGates(t *testing.T) {
	ctx := context.Background()

	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name          string
		customerID    string
		branchID      string
		input         AddCartItemInput
		wantErrCode   string
		wantErrSubstr string
	}{
		{
			name:       "penicillin allergy blocks Augmentin for Nadia",
			customerID: "cust-nadia",
			branchID:   "branch-hamra",
			input: AddCartItemInput{
				MedicationID: "med-augmentin-1g",
				Quantity:     1,
			},
			wantErrCode:   CodeConflict,
			wantErrSubstr: "contraindication_detected: medication conflicts with customer profile tag penicillin",
		},
		{
			name:       "NSAID/asthma contraindication blocks Brufen for Karim",
			customerID: "cust-karim",
			branchID:   "branch-hamra",
			input: AddCartItemInput{
				MedicationID: "med-brufen-400",
				Quantity:     1,
			},
			wantErrCode:   CodeConflict,
			wantErrSubstr: "contraindication_detected: medication conflicts with customer profile tag",
		},
		{
			name:       "controlled substance blocks Lexotanil",
			customerID: "cust-nadia",
			branchID:   "branch-hamra",
			input: AddCartItemInput{
				MedicationID: "med-lexotanil-3",
				Quantity:     1,
			},
			wantErrCode:   CodeConflict,
			wantErrSubstr: "controlled_substance: requires in-person pharmacist dispensing",
		},
		{
			name:       "RxRequired missing prescription blocked",
			customerID: "cust-nadia",
			branchID:   "branch-hamra",
			input: AddCartItemInput{
				MedicationID: "med-concor-5",
				Quantity:     1,
			},
			wantErrCode:   CodeConflict,
			wantErrSubstr: "rx_required: valid verified prescription with remaining refills required",
		},
		{
			name:       "RxRequired pending prescription blocked",
			customerID: "cust-nadia",
			branchID:   "branch-hamra",
			input: AddCartItemInput{
				MedicationID:   "med-concor-5",
				PrescriptionID: strPtr("rx-nadia-concor-pending"),
				Quantity:       1,
			},
			wantErrCode:   CodeConflict,
			wantErrSubstr: "rx_required: valid verified prescription with remaining refills required",
		},
		{
			name:       "RxRequired exhausted prescription blocked",
			customerID: "cust-nadia",
			branchID:   "branch-hamra",
			input: AddCartItemInput{
				MedicationID:   "med-concor-5",
				PrescriptionID: strPtr("rx-nadia-concor-exhausted"),
				Quantity:       1,
			},
			wantErrCode:   CodeConflict,
			wantErrSubstr: "rx_required: valid verified prescription with remaining refills required",
		},
		{
			name:       "insufficient stock at branch blocked",
			customerID: "cust-nadia",
			branchID:   "branch-byblos",
			input: AddCartItemInput{
				MedicationID:   "med-concor-5",
				PrescriptionID: strPtr("rx-nadia-concor-verified"),
				Quantity:       1,
			},
			wantErrCode:   CodeConflict,
			wantErrSubstr: "insufficient_stock: medication unavailable at branch",
		},
		{
			name:       "valid Rx and sufficient stock succeeds",
			customerID: "cust-nadia",
			branchID:   "branch-hamra",
			input: AddCartItemInput{
				MedicationID:   "med-concor-5",
				PrescriptionID: strPtr("rx-nadia-concor-verified"),
				Quantity:       2,
			},
			wantErrCode: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newSeededFakePharmacyStore()
			svc := NewPharmacyService(store)

			cartDetail, err := svc.CreateOrGetActiveCart(ctx, CreateCartInput{
				CustomerID: tc.customerID,
				BranchID:   tc.branchID,
			})
			if err != nil {
				t.Fatalf("CreateOrGetActiveCart failed: %v", err)
			}

			got, err := svc.AddCartItem(ctx, cartDetail.Cart.ID, tc.input)
			if tc.wantErrCode != "" {
				assertServiceErrorCode(t, err, tc.wantErrCode, tc.wantErrSubstr)
				return
			}
			if err != nil {
				t.Fatalf("AddCartItem unexpected error: %v", err)
			}
			if len(got.Items) != 1 {
				t.Fatalf("expected 1 cart item, got %d", len(got.Items))
			}
			wantSubtotal := int64(tc.input.Quantity) * store.medications[tc.input.MedicationID].UnitPriceCents
			if got.SubtotalCents != wantSubtotal {
				t.Errorf("expected subtotal %d, got %d", wantSubtotal, got.SubtotalCents)
			}
		})
	}
}

func TestCheckoutCart_AtomicFEFOAndRefillDecrement(t *testing.T) {
	ctx := context.Background()
	store := newSeededFakePharmacyStore()
	svc := NewPharmacyService(store)

	cartDetail, err := svc.CreateOrGetActiveCart(ctx, CreateCartInput{
		CustomerID: "cust-nadia",
		BranchID:   "branch-hamra",
	})
	if err != nil {
		t.Fatalf("CreateOrGetActiveCart failed: %v", err)
	}

	rxID := "rx-nadia-concor-verified"
	// Add 2 Concor (Rx required) + 3 Panadol (OTC)
	if _, err := svc.AddCartItem(ctx, cartDetail.Cart.ID, AddCartItemInput{
		MedicationID:   "med-concor-5",
		PrescriptionID: &rxID,
		Quantity:       2,
	}); err != nil {
		t.Fatalf("AddCartItem Concor failed: %v", err)
	}
	if _, err := svc.AddCartItem(ctx, cartDetail.Cart.ID, AddCartItemInput{
		MedicationID: "med-panadol-500",
		Quantity:     3,
	}); err != nil {
		t.Fatalf("AddCartItem Panadol failed: %v", err)
	}

	orderDetail, err := svc.CheckoutCart(ctx, cartDetail.Cart.ID, CheckoutCartInput{
		FulfillmentType: queries.FulfillmentDelivery,
	})
	if err != nil {
		t.Fatalf("CheckoutCart failed: %v", err)
	}

	if orderDetail.Order.Status != queries.OrderStatusConfirmed {
		t.Errorf("expected order status %q, got %q", queries.OrderStatusConfirmed, orderDetail.Order.Status)
	}
	// Default customer address used when DeliveryAddress omitted
	if orderDetail.Order.DeliveryAddress != "Bliss Street, Beirut" {
		t.Errorf("expected delivery address %q, got %q", "Bliss Street, Beirut", orderDetail.Order.DeliveryAddress)
	}
	wantTotal := int64(2*1200 + 3*450)
	if orderDetail.Order.TotalCents != wantTotal {
		t.Errorf("expected total_cents %d, got %d", wantTotal, orderDetail.Order.TotalCents)
	}
	if len(orderDetail.Items) != 2 {
		t.Fatalf("expected 2 order items, got %d", len(orderDetail.Items))
	}

	// Verify earliest valid FEFO batches were selected and committed
	itemByMed := make(map[string]queries.OrderItem)
	for _, oi := range orderDetail.Items {
		itemByMed[oi.MedicationID] = oi
	}
	if itemByMed["med-concor-5"].BatchID != "batch-concor-hamra-early" {
		t.Errorf("expected FEFO batch batch-concor-hamra-early for Concor, got %s", itemByMed["med-concor-5"].BatchID)
	}
	if itemByMed["med-panadol-500"].BatchID != "batch-panadol-hamra-early" {
		t.Errorf("expected FEFO batch batch-panadol-hamra-early for Panadol, got %s", itemByMed["med-panadol-500"].BatchID)
	}

	// Verify stock deducted on early batches (ReserveBatchStock + CommitBatchStock)
	concorBatch := store.batches["batch-concor-hamra-early"]
	// Started at on_hand=12, reserved=2 -> after committing 2: on_hand=10, reserved=2, available=8
	if concorBatch.QuantityOnHand != 10 || concorBatch.QuantityReserved != 2 {
		t.Errorf("unexpected Concor batch stock after checkout: on_hand=%d reserved=%d", concorBatch.QuantityOnHand, concorBatch.QuantityReserved)
	}
	panadolBatch := store.batches["batch-panadol-hamra-early"]
	// Started at on_hand=20, reserved=0 -> after committing 3: on_hand=17, reserved=0
	if panadolBatch.QuantityOnHand != 17 || panadolBatch.QuantityReserved != 0 {
		t.Errorf("unexpected Panadol batch stock after checkout: on_hand=%d reserved=%d", panadolBatch.QuantityOnHand, panadolBatch.QuantityReserved)
	}

	// Verify Rx refill decremented from 3 to 2
	updatedRx := store.prescriptions[rxID]
	if updatedRx.RemainingRefills != 2 {
		t.Errorf("expected remaining_refills 2, got %d", updatedRx.RemainingRefills)
	}

	// Verify cart marked checked_out
	updatedCart := store.carts[cartDetail.Cart.ID]
	if updatedCart.Status != queries.CartStatusCheckedOut {
		t.Errorf("expected cart status %q, got %q", queries.CartStatusCheckedOut, updatedCart.Status)
	}
}

func TestCreateRefill_ValidationAndEligibility(t *testing.T) {
	ctx := context.Background()
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name          string
		customerID    string
		input         CreateRefillInput
		wantErrCode   string
		wantErrSubstr string
	}{
		{
			name:       "non-chronic medication rejected with 409",
			customerID: "cust-nadia",
			input: CreateRefillInput{
				MedicationID: "med-panadol-500",
				BranchID:     "branch-hamra",
				IntervalDays: 30,
			},
			wantErrCode:   CodeConflict,
			wantErrSubstr: "medication is not eligible for chronic refill subscriptions",
		},
		{
			name:       "interval_days < 7 rejected with 400",
			customerID: "cust-nadia",
			input: CreateRefillInput{
				MedicationID:   "med-concor-5",
				BranchID:       "branch-hamra",
				PrescriptionID: strPtr("rx-nadia-concor-verified"),
				IntervalDays:   5,
			},
			wantErrCode:   CodeInvalid,
			wantErrSubstr: "interval_days must be between 7 and 90",
		},
		{
			name:       "interval_days > 90 rejected with 400",
			customerID: "cust-nadia",
			input: CreateRefillInput{
				MedicationID:   "med-concor-5",
				BranchID:       "branch-hamra",
				PrescriptionID: strPtr("rx-nadia-concor-verified"),
				IntervalDays:   95,
			},
			wantErrCode:   CodeInvalid,
			wantErrSubstr: "interval_days must be between 7 and 90",
		},
		{
			name:       "chronic Rx medication missing verified prescription rejected with 409",
			customerID: "cust-nadia",
			input: CreateRefillInput{
				MedicationID: "med-concor-5",
				BranchID:     "branch-hamra",
				IntervalDays: 30,
			},
			wantErrCode:   CodeConflict,
			wantErrSubstr: "rx_required: valid verified prescription with remaining refills required",
		},
		{
			name:       "valid chronic Rx refill subscription created",
			customerID: "cust-nadia",
			input: CreateRefillInput{
				MedicationID:   "med-concor-5",
				BranchID:       "branch-hamra",
				PrescriptionID: strPtr("rx-nadia-concor-verified"),
				IntervalDays:   28,
			},
			wantErrCode: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newSeededFakePharmacyStore()
			svc := NewPharmacyService(store)

			sub, err := svc.CreateRefill(ctx, tc.customerID, tc.input)
			if tc.wantErrCode != "" {
				assertServiceErrorCode(t, err, tc.wantErrCode, tc.wantErrSubstr)
				return
			}
			if err != nil {
				t.Fatalf("CreateRefill unexpected error: %v", err)
			}
			if sub.Status != queries.RefillStatusActive {
				t.Errorf("expected refill status %q, got %q", queries.RefillStatusActive, sub.Status)
			}
			if sub.IntervalDays != tc.input.IntervalDays {
				t.Errorf("expected interval_days %d, got %d", tc.input.IntervalDays, sub.IntervalDays)
			}
		})
	}
}

func TestCustomerAndCartLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newSeededFakePharmacyStore()
	svc := NewPharmacyService(store)

	// 1. UpdateCustomer profile
	updatedCust, err := svc.UpdateCustomer(ctx, "cust-nadia", UpdateCustomerInput{
		FullName:          "Nadia M. Khoury",
		Address:           "Verdun, Beirut",
		Allergies:         []string{"penicillin", "sulfa"},
		ChronicConditions: []string{"hypertension"},
	})
	if err != nil {
		t.Fatalf("UpdateCustomer failed: %v", err)
	}
	if updatedCust.FullName != "Nadia M. Khoury" || updatedCust.Address != "Verdun, Beirut" {
		t.Errorf("unexpected updated customer fields: %+v", updatedCust)
	}

	// 2. CreatePrescription defaults to pending & 6 months expiry
	rx, err := svc.CreatePrescription(ctx, "cust-nadia", CreatePrescriptionInput{
		MedicationID:       "med-concor-5",
		DoctorName:         "Dr. Fadi",
		DosageInstructions: "1 daily",
		AllowedRefills:     4,
	})
	if err != nil {
		t.Fatalf("CreatePrescription failed: %v", err)
	}
	if rx.Status != queries.PrescriptionStatusPending {
		t.Errorf("expected pending status, got %q", rx.Status)
	}

	// 3. Switching active cart branch abandons previous cart
	cart1, err := svc.CreateOrGetActiveCart(ctx, CreateCartInput{
		CustomerID: "cust-nadia",
		BranchID:   "branch-hamra",
	})
	if err != nil {
		t.Fatalf("CreateOrGetActiveCart 1 failed: %v", err)
	}
	cart2, err := svc.CreateOrGetActiveCart(ctx, CreateCartInput{
		CustomerID: "cust-nadia",
		BranchID:   "branch-byblos",
	})
	if err != nil {
		t.Fatalf("CreateOrGetActiveCart 2 failed: %v", err)
	}
	if cart1.Cart.ID == cart2.Cart.ID {
		t.Errorf("expected new cart when switching branches")
	}
	if store.carts[cart1.Cart.ID].Status != queries.CartStatusAbandoned {
		t.Errorf("expected old cart to be abandoned, got %q", store.carts[cart1.Cart.ID].Status)
	}

	// 4. Delivery at branch that does not support delivery (Byblos) returns 409
	_, err = svc.CheckoutCart(ctx, cart2.Cart.ID, CheckoutCartInput{
		FulfillmentType: queries.FulfillmentDelivery,
	})
	assertServiceErrorCode(t, err, CodeConflict, "branch does not support delivery")
}
