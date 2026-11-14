package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	httpx "alphahack/internal/http"
	"alphahack/internal/queries"
	"alphahack/internal/services"
)

type pharmacyStoreFake struct {
	queries.Querier
	err error
}

var _ services.PharmacyStore = (*pharmacyStoreFake)(nil)

func (f *pharmacyStoreFake) sampleTime() time.Time {
	return time.Now().UTC().Add(30 * 24 * time.Hour)
}

func (f *pharmacyStoreFake) sampleBranch() queries.Branch {
	return queries.Branch{
		ID:               "br_1",
		Name:             "Downtown Pharmacy",
		City:             "Austin",
		Address:          "100 Congress Ave",
		Phone:            "+15550101",
		SupportsDelivery: true,
		CreatedAt:        agentFixedTime,
	}
}

func (f *pharmacyStoreFake) sampleMedication() queries.Medication {
	return queries.Medication{
		ID:                   "med_1",
		BrandName:            "Panadol",
		ActiveIngredient:     "Paracetamol",
		TherapeuticClass:     "Analgesic",
		DosageForm:           queries.DosageFormTablet,
		Strength:             "500mg",
		RxRequired:           false,
		Controlled:           false,
		ChronicEligible:      true,
		UnitPriceCents:       500,
		ContraindicationTags: json.RawMessage(`[]`),
		Description:          "Pain reliever",
		CreatedAt:            agentFixedTime,
	}
}

func (f *pharmacyStoreFake) sampleBatch() queries.InventoryBatch {
	return queries.InventoryBatch{
		ID:                "batch_1",
		BranchID:          "br_1",
		MedicationID:      "med_1",
		BatchNumber:       "LOT-001",
		ExpiryDate:        f.sampleTime(),
		QuantityOnHand:    100,
		QuantityReserved:  0,
		QuantityAvailable: 100,
		IsRecalled:        false,
		UpdatedAt:         agentFixedTime,
	}
}

func (f *pharmacyStoreFake) sampleCustomer() queries.Customer {
	return queries.Customer{
		ID:                "cust_1",
		FullName:          "Jane Doe",
		Phone:             "+15550100",
		Email:             "jane@example.com",
		Address:           "123 Main St",
		Allergies:         json.RawMessage(`[]`),
		ChronicConditions: json.RawMessage(`[]`),
		CreatedAt:         agentFixedTime,
		UpdatedAt:         agentFixedTime,
	}
}

func (f *pharmacyStoreFake) samplePrescription() queries.Prescription {
	return queries.Prescription{
		ID:                 "rx_1",
		CustomerID:         "cust_1",
		MedicationID:       "med_1",
		DoctorName:         "Dr. Smith",
		DosageInstructions: "1 tablet daily",
		AllowedRefills:     3,
		RemainingRefills:   3,
		Status:             queries.PrescriptionStatusVerified,
		Notes:              "Take with water",
		ExpiresAt:          f.sampleTime(),
		CreatedAt:          agentFixedTime,
	}
}

func (f *pharmacyStoreFake) sampleCart() queries.Cart {
	return queries.Cart{
		ID:         "cart_1",
		CustomerID: "cust_1",
		BranchID:   "br_1",
		Status:     queries.CartStatusActive,
		CreatedAt:  agentFixedTime,
		UpdatedAt:  agentFixedTime,
	}
}

func (f *pharmacyStoreFake) sampleCartItem() queries.CartItem {
	return queries.CartItem{
		ID:             "ci_1",
		CartID:         "cart_1",
		MedicationID:   "med_1",
		Quantity:       1,
		UnitPriceCents: 500,
		CreatedAt:      agentFixedTime,
	}
}

func (f *pharmacyStoreFake) sampleOrder() queries.Order {
	cartID := "cart_1"
	return queries.Order{
		ID:              "ord_1",
		CustomerID:      "cust_1",
		BranchID:        "br_1",
		CartID:          &cartID,
		Status:          queries.OrderStatusConfirmed,
		FulfillmentType: queries.FulfillmentPickup,
		DeliveryAddress: "123 Main St",
		TotalCents:      500,
		CreatedAt:       agentFixedTime,
		UpdatedAt:       agentFixedTime,
	}
}

func (f *pharmacyStoreFake) sampleOrderItem() queries.OrderItem {
	return queries.OrderItem{
		ID:             "oi_1",
		OrderID:        "ord_1",
		MedicationID:   "med_1",
		BatchID:        "batch_1",
		Quantity:       1,
		UnitPriceCents: 500,
		CreatedAt:      agentFixedTime,
	}
}

func (f *pharmacyStoreFake) sampleRefill() queries.RefillSubscription {
	return queries.RefillSubscription{
		ID:           "ref_1",
		CustomerID:   "cust_1",
		MedicationID: "med_1",
		BranchID:     "br_1",
		IntervalDays: 30,
		NextRefillAt: f.sampleTime(),
		Status:       queries.RefillStatusActive,
		CreatedAt:    agentFixedTime,
	}
}

func (f *pharmacyStoreFake) InTx(_ context.Context, fn func(q queries.Querier) error) error {
	if f.err != nil {
		return f.err
	}
	return fn(f)
}

func (f *pharmacyStoreFake) ListBranches(_ context.Context, _, _ int) ([]queries.Branch, error) {
	return []queries.Branch{f.sampleBranch()}, f.err
}

func (f *pharmacyStoreFake) GetBranch(_ context.Context, _ string) (queries.Branch, error) {
	return f.sampleBranch(), f.err
}

func (f *pharmacyStoreFake) ListMedications(_ context.Context, _, _ int) ([]queries.Medication, error) {
	return []queries.Medication{f.sampleMedication()}, f.err
}

func (f *pharmacyStoreFake) SearchMedications(_ context.Context, _ string, _, _ int) ([]queries.Medication, error) {
	return []queries.Medication{f.sampleMedication()}, f.err
}

func (f *pharmacyStoreFake) GetMedication(_ context.Context, _ string) (queries.Medication, error) {
	return f.sampleMedication(), f.err
}

func (f *pharmacyStoreFake) ListBranchInventory(_ context.Context, _, _ string, _, _ int) ([]queries.InventoryBatch, error) {
	return []queries.InventoryBatch{f.sampleBatch()}, f.err
}

func (f *pharmacyStoreFake) GetSelectFefoBatchForUpdate(_ context.Context, _, _ string, _ int) (queries.InventoryBatch, error) {
	return f.sampleBatch(), f.err
}

func (f *pharmacyStoreFake) ReserveBatchStock(_ context.Context, _ string, _ int) (queries.InventoryBatch, error) {
	return f.sampleBatch(), f.err
}

func (f *pharmacyStoreFake) CommitBatchStock(_ context.Context, _ string, _ int) (queries.InventoryBatch, error) {
	return f.sampleBatch(), f.err
}

func (f *pharmacyStoreFake) ReleaseBatchStock(_ context.Context, _ string, _ int) (queries.InventoryBatch, error) {
	return f.sampleBatch(), f.err
}

func (f *pharmacyStoreFake) GetCustomer(_ context.Context, _ string) (queries.Customer, error) {
	return f.sampleCustomer(), f.err
}

func (f *pharmacyStoreFake) ListCustomers(_ context.Context, _, _ int) ([]queries.Customer, error) {
	return []queries.Customer{f.sampleCustomer()}, f.err
}

func (f *pharmacyStoreFake) UpdateCustomerProfile(_ context.Context, _ queries.UpdateCustomerProfileParams) (queries.Customer, error) {
	return f.sampleCustomer(), f.err
}

func (f *pharmacyStoreFake) ListCustomerPrescriptions(_ context.Context, _, _ string, _, _ int) ([]queries.Prescription, error) {
	return []queries.Prescription{f.samplePrescription()}, f.err
}

func (f *pharmacyStoreFake) GetPrescription(_ context.Context, _ string) (queries.Prescription, error) {
	return f.samplePrescription(), f.err
}

func (f *pharmacyStoreFake) CreatePrescription(_ context.Context, _ queries.CreatePrescriptionParams) (queries.Prescription, error) {
	return f.samplePrescription(), f.err
}

func (f *pharmacyStoreFake) DecrementPrescriptionRefill(_ context.Context, _ string) (queries.Prescription, error) {
	return f.samplePrescription(), f.err
}

func (f *pharmacyStoreFake) GetCart(_ context.Context, _ string) (queries.Cart, error) {
	return f.sampleCart(), f.err
}

func (f *pharmacyStoreFake) GetActiveCustomerCart(_ context.Context, _ string) (queries.Cart, error) {
	return f.sampleCart(), f.err
}

func (f *pharmacyStoreFake) CreateCart(_ context.Context, _ queries.CreateCartParams) (queries.Cart, error) {
	return f.sampleCart(), f.err
}

func (f *pharmacyStoreFake) UpdateCartStatus(_ context.Context, _, _ string) (queries.Cart, error) {
	return f.sampleCart(), f.err
}

func (f *pharmacyStoreFake) ListCartItems(_ context.Context, _ string) ([]queries.CartItem, error) {
	return []queries.CartItem{f.sampleCartItem()}, f.err
}

func (f *pharmacyStoreFake) UpsertCartItem(_ context.Context, _ queries.UpsertCartItemParams) (queries.CartItem, error) {
	return f.sampleCartItem(), f.err
}

func (f *pharmacyStoreFake) DeleteCartItem(_ context.Context, _, _ string) error {
	return f.err
}

func (f *pharmacyStoreFake) CreateOrder(_ context.Context, _ queries.CreateOrderParams) (queries.Order, error) {
	return f.sampleOrder(), f.err
}

func (f *pharmacyStoreFake) GetOrder(_ context.Context, _ string) (queries.Order, error) {
	return f.sampleOrder(), f.err
}

func (f *pharmacyStoreFake) ListCustomerOrders(_ context.Context, _ string, _, _ int) ([]queries.Order, error) {
	return []queries.Order{f.sampleOrder()}, f.err
}

func (f *pharmacyStoreFake) CreateOrderItem(_ context.Context, _ queries.CreateOrderItemParams) (queries.OrderItem, error) {
	return f.sampleOrderItem(), f.err
}

func (f *pharmacyStoreFake) ListOrderItems(_ context.Context, _ string) ([]queries.OrderItem, error) {
	return []queries.OrderItem{f.sampleOrderItem()}, f.err
}

func (f *pharmacyStoreFake) ListCustomerRefillSubscriptions(_ context.Context, _ string) ([]queries.RefillSubscription, error) {
	return []queries.RefillSubscription{f.sampleRefill()}, f.err
}

func (f *pharmacyStoreFake) CreateRefillSubscription(_ context.Context, _ queries.CreateRefillSubscriptionParams) (queries.RefillSubscription, error) {
	return f.sampleRefill(), f.err
}

func (f *pharmacyStoreFake) UpdateRefillSubscriptionStatus(_ context.Context, _, _ string) (queries.RefillSubscription, error) {
	return f.sampleRefill(), f.err
}

func pharmacyServer(t *testing.T, fake *pharmacyStoreFake) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	New(Deps{Pharmacy: services.NewPharmacyService(fake)}).Register(mux)
	return httpx.NewRouter(mux)
}

func TestPharmacyRoutesMatchAndFormatJSON(t *testing.T) {
	srv := pharmacyServer(t, &pharmacyStoreFake{})
	futureISO := time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339)

	routes := []struct {
		name       string
		method     string
		target     string
		body       string
		wantStatus int
		isList     bool
	}{
		{
			name:       "list branches",
			method:     http.MethodGet,
			target:     "/api/v1/branches?limit=15&offset=5",
			wantStatus: http.StatusOK,
			isList:     true,
		},
		{
			name:       "get branch",
			method:     http.MethodGet,
			target:     "/api/v1/branches/br_1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "list branch inventory",
			method:     http.MethodGet,
			target:     "/api/v1/branches/br_1/inventory?limit=15&offset=5",
			wantStatus: http.StatusOK,
			isList:     true,
		},
		{
			name:       "search medications",
			method:     http.MethodGet,
			target:     "/api/v1/medications?q=panadol&limit=15&offset=5",
			wantStatus: http.StatusOK,
			isList:     true,
		},
		{
			name:       "get medication detail",
			method:     http.MethodGet,
			target:     "/api/v1/medications/med_1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "list customers",
			method:     http.MethodGet,
			target:     "/api/v1/customers?limit=15&offset=5",
			wantStatus: http.StatusOK,
			isList:     true,
		},
		{
			name:       "get customer",
			method:     http.MethodGet,
			target:     "/api/v1/customers/cust_1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "update customer",
			method:     http.MethodPatch,
			target:     "/api/v1/customers/cust_1",
			body:       `{"full_name":"Jane Doe","phone":"+15550100","email":"jane@example.com","address":"123 Main St","allergies":[],"chronic_conditions":[]}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "list customer prescriptions",
			method:     http.MethodGet,
			target:     "/api/v1/customers/cust_1/prescriptions?limit=15&offset=5",
			wantStatus: http.StatusOK,
			isList:     true,
		},
		{
			name:       "create prescription",
			method:     http.MethodPost,
			target:     "/api/v1/customers/cust_1/prescriptions",
			body:       `{"medication_id":"med_1","doctor_name":"Dr. Smith","dosage_instructions":"1 tablet daily","allowed_refills":3,"notes":"Take with water","expires_at":"` + futureISO + `"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "list customer refills",
			method:     http.MethodGet,
			target:     "/api/v1/customers/cust_1/refills?limit=15&offset=5",
			wantStatus: http.StatusOK,
			isList:     true,
		},
		{
			name:       "create refill",
			method:     http.MethodPost,
			target:     "/api/v1/customers/cust_1/refills",
			body:       `{"medication_id":"med_1","branch_id":"br_1","interval_days":30,"next_refill_at":"` + futureISO + `"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "update refill status",
			method:     http.MethodPatch,
			target:     "/api/v1/refills/ref_1",
			body:       `{"status":"paused"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "create cart",
			method:     http.MethodPost,
			target:     "/api/v1/carts",
			body:       `{"customer_id":"cust_1","branch_id":"br_1"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "get cart detail",
			method:     http.MethodGet,
			target:     "/api/v1/carts/cart_1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "add cart item",
			method:     http.MethodPost,
			target:     "/api/v1/carts/cart_1/items",
			body:       `{"medication_id":"med_1","quantity":1}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "remove cart item",
			method:     http.MethodDelete,
			target:     "/api/v1/carts/cart_1/items/med_1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "checkout cart",
			method:     http.MethodPost,
			target:     "/api/v1/carts/cart_1/checkout",
			body:       `{"fulfillment_type":"pickup","delivery_address":"123 Main St"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "list customer orders",
			method:     http.MethodGet,
			target:     "/api/v1/customers/cust_1/orders?limit=15&offset=5",
			wantStatus: http.StatusOK,
			isList:     true,
		},
		{
			name:       "get order detail",
			method:     http.MethodGet,
			target:     "/api/v1/orders/ord_1",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range routes {
		t.Run(tc.name, func(t *testing.T) {
			rec := agentDo(t, srv, tc.method, tc.target, tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var raw map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatalf("response is not a JSON object: %v (%s)", err, rec.Body.String())
			}
			if tc.isList {
				if _, ok := raw["data"]; !ok {
					t.Errorf("list response missing 'data' key: %s", rec.Body.String())
				}
				if gotLimit, _ := raw["limit"].(float64); gotLimit != 15 {
					t.Errorf("limit = %v, want 15", raw["limit"])
				}
				if gotOffset, _ := raw["offset"].(float64); gotOffset != 5 {
					t.Errorf("offset = %v, want 5", raw["offset"])
				}
			}
		})
	}
}

func TestPharmacyBadJSONReturns400(t *testing.T) {
	srv := pharmacyServer(t, &pharmacyStoreFake{})

	endpoints := []struct {
		method string
		target string
	}{
		{http.MethodPatch, "/api/v1/customers/cust_1"},
		{http.MethodPost, "/api/v1/customers/cust_1/prescriptions"},
		{http.MethodPost, "/api/v1/customers/cust_1/refills"},
		{http.MethodPatch, "/api/v1/refills/ref_1"},
		{http.MethodPost, "/api/v1/carts"},
		{http.MethodPost, "/api/v1/carts/cart_1/items"},
		{http.MethodPost, "/api/v1/carts/cart_1/checkout"},
	}

	badBodies := []struct {
		name string
		body string
	}{
		{"malformed json", `{"broken":`},
		{"unknown field", `{"unknown_field_xyz": 123}`},
		{"empty body", ``},
	}

	for _, ep := range endpoints {
		for _, bb := range badBodies {
			t.Run(ep.method+" "+ep.target+" "+bb.name, func(t *testing.T) {
				rec := agentDo(t, srv, ep.method, ep.target, bb.body)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
				}
				code, _ := agentErrorCode(t, rec)
				if code != httpx.CodeBadRequest {
					t.Errorf("code = %q, want %q", code, httpx.CodeBadRequest)
				}
			})
		}
	}
}

func TestPharmacyBadPaginationReturns400(t *testing.T) {
	srv := pharmacyServer(t, &pharmacyStoreFake{})

	listTargets := []string{
		"/api/v1/branches",
		"/api/v1/branches/br_1/inventory",
		"/api/v1/medications",
		"/api/v1/customers",
		"/api/v1/customers/cust_1/prescriptions",
		"/api/v1/customers/cust_1/refills",
		"/api/v1/customers/cust_1/orders",
	}

	badQueries := []string{"?limit=abc", "?offset=-1", "?limit=-5", "?offset=xyz"}

	for _, base := range listTargets {
		for _, q := range badQueries {
			target := base + q
			t.Run(target, func(t *testing.T) {
				rec := agentDo(t, srv, http.MethodGet, target, "")
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
				}
				code, msg := agentErrorCode(t, rec)
				if code != httpx.CodeBadRequest {
					t.Errorf("code = %q, want %q", code, httpx.CodeBadRequest)
				}
				if !strings.Contains(msg, "must be a non-negative integer") {
					t.Errorf("message = %q, want non-negative integer error", msg)
				}
			})
		}
	}
}

func TestPharmacyRouterNotFoundAndMethodNotAllowed(t *testing.T) {
	srv := pharmacyServer(t, &pharmacyStoreFake{})

	t.Run("unknown route returns 404 not_found", func(t *testing.T) {
		rec := agentDo(t, srv, http.MethodGet, "/api/v1/unknown-pharmacy-endpoint", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
		if code, _ := agentErrorCode(t, rec); code != httpx.CodeNotFound {
			t.Errorf("code = %q, want %q", code, httpx.CodeNotFound)
		}
	})

	t.Run("wrong method returns 405 method_not_allowed", func(t *testing.T) {
		rec := agentDo(t, srv, http.MethodDelete, "/api/v1/branches", "")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", rec.Code)
		}
		if code, _ := agentErrorCode(t, rec); code != httpx.CodeMethodNotAllowed {
			t.Errorf("code = %q, want %q", code, httpx.CodeMethodNotAllowed)
		}
	})
}
