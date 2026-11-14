package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"alphahack/internal/queries"
)

type PharmacyService struct {
	store PharmacyStore
}

func NewPharmacyService(store PharmacyStore) *PharmacyService {
	return &PharmacyService{store: store}
}

// ---- Branches & Inventory --------------------------------------------------

func (s *PharmacyService) ListBranches(ctx context.Context, page Page) ([]queries.Branch, error) {
	page = page.Normalize()
	list, err := s.store.ListBranches(ctx, page.Limit, page.Offset)
	if err != nil {
		return nil, FromStore(err, "branch")
	}
	if list == nil {
		list = []queries.Branch{}
	}
	return list, nil
}

func (s *PharmacyService) GetBranch(ctx context.Context, id string) (queries.Branch, error) {
	id = clean(id)
	if id == "" {
		return queries.Branch{}, NotFound("branch")
	}
	b, err := s.store.GetBranch(ctx, id)
	if err != nil {
		return queries.Branch{}, FromStore(err, "branch")
	}
	return b, nil
}

func (s *PharmacyService) ListBranchInventory(ctx context.Context, branchID string, page Page) ([]queries.InventoryBatch, error) {
	b, err := s.GetBranch(ctx, branchID)
	if err != nil {
		return nil, err
	}
	page = page.Normalize()
	list, err := s.store.ListBranchInventory(ctx, b.ID, "", page.Limit, page.Offset)
	if err != nil {
		return nil, FromStore(err, "inventory")
	}
	if list == nil {
		list = []queries.InventoryBatch{}
	}
	return list, nil
}

// ---- Catalog & Cross-Branch Availability -----------------------------------

func (s *PharmacyService) SearchMedications(ctx context.Context, query string, page Page) ([]queries.Medication, error) {
	page = page.Normalize()
	q := clean(query)
	var (
		list []queries.Medication
		err  error
	)
	if q == "" {
		list, err = s.store.ListMedications(ctx, page.Limit, page.Offset)
	} else {
		list, err = s.store.SearchMedications(ctx, q, page.Limit, page.Offset)
	}
	if err != nil {
		return nil, FromStore(err, "medication")
	}
	if list == nil {
		list = []queries.Medication{}
	}
	return list, nil
}

func (s *PharmacyService) GetMedicationDetail(ctx context.Context, id string) (MedicationDetail, error) {
	id = clean(id)
	if id == "" {
		return MedicationDetail{}, NotFound("medication")
	}
	med, err := s.store.GetMedication(ctx, id)
	if err != nil {
		return MedicationDetail{}, FromStore(err, "medication")
	}
	branches, err := s.store.ListBranches(ctx, MaxLimit, 0)
	if err != nil {
		return MedicationDetail{}, FromStore(err, "branch")
	}

	now := time.Now()
	summaries := make([]BranchStockSummary, 0, len(branches))
	for _, b := range branches {
		batches, err := s.store.ListBranchInventory(ctx, b.ID, med.ID, MaxLimit, 0)
		if err != nil {
			return MedicationDetail{}, FromStore(err, "inventory")
		}
		var availTotal int
		var earliest *time.Time
		for _, batch := range batches {
			avail := batch.QuantityOnHand - batch.QuantityReserved
			if !batch.IsRecalled && batch.ExpiryDate.After(now) && avail > 0 {
				availTotal += avail
				exp := batch.ExpiryDate
				if earliest == nil || exp.Before(*earliest) {
					earliest = &exp
				}
			}
		}
		if availTotal == 0 {
			earliest = nil
		}
		summaries = append(summaries, BranchStockSummary{
			BranchID:          b.ID,
			BranchName:        b.Name,
			City:              b.City,
			AvailableQuantity: availTotal,
			EarliestExpiry:    earliest,
		})
	}

	return MedicationDetail{
		Medication:         med,
		BranchAvailability: summaries,
	}, nil
}

// ---- Customers & Prescriptions ---------------------------------------------

func (s *PharmacyService) ListCustomers(ctx context.Context, page Page) ([]queries.Customer, error) {
	page = page.Normalize()
	list, err := s.store.ListCustomers(ctx, page.Limit, page.Offset)
	if err != nil {
		return nil, FromStore(err, "customer")
	}
	if list == nil {
		list = []queries.Customer{}
	}
	return list, nil
}

func (s *PharmacyService) GetCustomer(ctx context.Context, id string) (queries.Customer, error) {
	id = clean(id)
	if id == "" {
		return queries.Customer{}, NotFound("customer")
	}
	c, err := s.store.GetCustomer(ctx, id)
	if err != nil {
		return queries.Customer{}, FromStore(err, "customer")
	}
	return c, nil
}

func (s *PharmacyService) UpdateCustomer(ctx context.Context, id string, in UpdateCustomerInput) (queries.Customer, error) {
	cur, err := s.GetCustomer(ctx, id)
	if err != nil {
		return queries.Customer{}, err
	}
	if clean(in.FullName) != "" {
		cur.FullName = clean(in.FullName)
	}
	if clean(cur.FullName) == "" {
		return queries.Customer{}, Invalid("full_name is required")
	}
	if utf8.RuneCountInString(cur.FullName) > MaxNameLen {
		return queries.Customer{}, Invalid("full_name must be at most %d characters", MaxNameLen)
	}
	if in.Email != "" {
		cur.Email = clean(in.Email)
	}
	if in.Address != "" {
		cur.Address = clean(in.Address)
	}
	if in.Allergies != nil {
		raw, err := json.Marshal(in.Allergies)
		if err != nil {
			return queries.Customer{}, Invalid("invalid allergies")
		}
		cur.Allergies = raw
	}
	if len(cur.Allergies) == 0 {
		cur.Allergies = json.RawMessage("[]")
	}
	if in.ChronicConditions != nil {
		raw, err := json.Marshal(in.ChronicConditions)
		if err != nil {
			return queries.Customer{}, Invalid("invalid chronic_conditions")
		}
		cur.ChronicConditions = raw
	}
	if len(cur.ChronicConditions) == 0 {
		cur.ChronicConditions = json.RawMessage("[]")
	}

	updated, err := s.store.UpdateCustomerProfile(ctx, queries.UpdateCustomerProfileParams{
		ID:                cur.ID,
		FullName:          cur.FullName,
		Email:             cur.Email,
		Address:           cur.Address,
		Allergies:         cur.Allergies,
		ChronicConditions: cur.ChronicConditions,
	})
	if err != nil {
		return queries.Customer{}, FromStore(err, "customer")
	}
	return updated, nil
}

func (s *PharmacyService) ListCustomerPrescriptions(ctx context.Context, customerID string, page Page) ([]queries.Prescription, error) {
	c, err := s.GetCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	page = page.Normalize()
	list, err := s.store.ListCustomerPrescriptions(ctx, c.ID, "", page.Limit, page.Offset)
	if err != nil {
		return nil, FromStore(err, "prescription")
	}
	if list == nil {
		list = []queries.Prescription{}
	}
	return list, nil
}

func (s *PharmacyService) CreatePrescription(ctx context.Context, customerID string, in CreatePrescriptionInput) (queries.Prescription, error) {
	c, err := s.GetCustomer(ctx, customerID)
	if err != nil {
		return queries.Prescription{}, err
	}
	medID := clean(in.MedicationID)
	med, err := s.store.GetMedication(ctx, medID)
	if err != nil {
		return queries.Prescription{}, FromStore(err, "medication")
	}
	doctorName := clean(in.DoctorName)
	if doctorName == "" {
		return queries.Prescription{}, Invalid("doctor_name is required")
	}
	if in.AllowedRefills <= 0 {
		return queries.Prescription{}, Invalid("allowed_refills must be greater than 0")
	}
	expiresAt := in.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().AddDate(0, 6, 0)
	}
	if !expiresAt.After(time.Now()) {
		return queries.Prescription{}, Invalid("expires_at must be in the future")
	}

	rx, err := s.store.CreatePrescription(ctx, queries.CreatePrescriptionParams{
		CustomerID:         c.ID,
		MedicationID:       med.ID,
		DoctorName:         doctorName,
		DosageInstructions: clean(in.DosageInstructions),
		AllowedRefills:     in.AllowedRefills,
		Status:             queries.PrescriptionStatusPending,
		Notes:              clean(in.Notes),
		ExpiresAt:          expiresAt,
	})
	if err != nil {
		return queries.Prescription{}, FromStore(err, "prescription")
	}
	return rx, nil
}

// ---- Carts & Deterministic Safety Gates ------------------------------------

func (s *PharmacyService) CreateOrGetActiveCart(ctx context.Context, in CreateCartInput) (CartDetail, error) {
	custID := clean(in.CustomerID)
	if custID == "" {
		return CartDetail{}, Invalid("customer_id is required")
	}
	branchID := clean(in.BranchID)
	if branchID == "" {
		return CartDetail{}, Invalid("branch_id is required")
	}
	if _, err := s.GetCustomer(ctx, custID); err != nil {
		return CartDetail{}, err
	}
	if _, err := s.GetBranch(ctx, branchID); err != nil {
		return CartDetail{}, err
	}

	cart, err := s.store.GetActiveCustomerCart(ctx, custID)
	if err == nil {
		if cart.BranchID == branchID {
			return s.buildCartDetail(ctx, cart)
		}
		if _, err := s.store.UpdateCartStatus(ctx, cart.ID, queries.CartStatusAbandoned); err != nil {
			return CartDetail{}, FromStore(err, "cart")
		}
		cart, err = s.store.CreateCart(ctx, queries.CreateCartParams{
			CustomerID: custID,
			BranchID:   branchID,
		})
		if err != nil {
			return CartDetail{}, FromStore(err, "cart")
		}
		return s.buildCartDetail(ctx, cart)
	}
	if !errors.Is(err, queries.ErrNotFound) {
		return CartDetail{}, FromStore(err, "cart")
	}

	cart, err = s.store.CreateCart(ctx, queries.CreateCartParams{
		CustomerID: custID,
		BranchID:   branchID,
	})
	if err != nil {
		return CartDetail{}, FromStore(err, "cart")
	}
	return s.buildCartDetail(ctx, cart)
}

func (s *PharmacyService) GetCartDetail(ctx context.Context, cartID string) (CartDetail, error) {
	cartID = clean(cartID)
	if cartID == "" {
		return CartDetail{}, NotFound("cart")
	}
	cart, err := s.store.GetCart(ctx, cartID)
	if err != nil {
		return CartDetail{}, FromStore(err, "cart")
	}
	return s.buildCartDetail(ctx, cart)
}

func (s *PharmacyService) AddCartItem(ctx context.Context, cartID string, in AddCartItemInput) (CartDetail, error) {
	cartID = clean(cartID)
	if cartID == "" {
		return CartDetail{}, NotFound("cart")
	}
	cart, err := s.store.GetCart(ctx, cartID)
	if err != nil {
		return CartDetail{}, FromStore(err, "cart")
	}
	if cart.Status != queries.CartStatusActive {
		return CartDetail{}, Conflict("cart is not active")
	}
	if in.Quantity <= 0 {
		return CartDetail{}, Invalid("quantity must be greater than 0")
	}
	medID := clean(in.MedicationID)
	if medID == "" {
		return CartDetail{}, Invalid("medication_id is required")
	}

	customer, err := s.store.GetCustomer(ctx, cart.CustomerID)
	if err != nil {
		return CartDetail{}, FromStore(err, "customer")
	}
	medication, err := s.store.GetMedication(ctx, medID)
	if err != nil {
		return CartDetail{}, FromStore(err, "medication")
	}

	if err := checkMedicationSafetyGates(customer, medication); err != nil {
		return CartDetail{}, err
	}

	var rxIDPtr *string
	if medication.RxRequired {
		if in.PrescriptionID == nil || clean(*in.PrescriptionID) == "" {
			return CartDetail{}, Conflict("rx_required: valid verified prescription with remaining refills required")
		}
		rxID := clean(*in.PrescriptionID)
		rx, err := s.store.GetPrescription(ctx, rxID)
		if err != nil ||
			rx.CustomerID != cart.CustomerID ||
			rx.MedicationID != medication.ID ||
			rx.Status != queries.PrescriptionStatusVerified ||
			rx.RemainingRefills <= 0 ||
			!rx.ExpiresAt.After(time.Now()) {
			return CartDetail{}, Conflict("rx_required: valid verified prescription with remaining refills required")
		}
		rxIDPtr = &rx.ID
	} else if in.PrescriptionID != nil && clean(*in.PrescriptionID) != "" {
		rxID := clean(*in.PrescriptionID)
		rxIDPtr = &rxID
	}

	batches, err := s.store.ListBranchInventory(ctx, cart.BranchID, medication.ID, MaxLimit, 0)
	if err != nil {
		return CartDetail{}, FromStore(err, "inventory")
	}
	now := time.Now()
	hasStock := false
	for _, b := range batches {
		avail := b.QuantityOnHand - b.QuantityReserved
		if !b.IsRecalled && b.ExpiryDate.After(now) && avail >= in.Quantity {
			hasStock = true
			break
		}
	}
	if !hasStock {
		return CartDetail{}, Conflict("insufficient_stock: medication unavailable at branch")
	}

	_, err = s.store.UpsertCartItem(ctx, queries.UpsertCartItemParams{
		CartID:         cart.ID,
		MedicationID:   medication.ID,
		PrescriptionID: rxIDPtr,
		Quantity:       in.Quantity,
		UnitPriceCents: medication.UnitPriceCents,
	})
	if err != nil {
		return CartDetail{}, FromStore(err, "cart item")
	}

	return s.buildCartDetail(ctx, cart)
}

func (s *PharmacyService) RemoveCartItem(ctx context.Context, cartID, medicationID string) (CartDetail, error) {
	cartID = clean(cartID)
	if cartID == "" {
		return CartDetail{}, NotFound("cart")
	}
	cart, err := s.store.GetCart(ctx, cartID)
	if err != nil {
		return CartDetail{}, FromStore(err, "cart")
	}
	if cart.Status != queries.CartStatusActive {
		return CartDetail{}, Conflict("cart is not active")
	}
	medicationID = clean(medicationID)
	if err := s.store.DeleteCartItem(ctx, cartID, medicationID); err != nil {
		return CartDetail{}, FromStore(err, "cart item")
	}
	return s.buildCartDetail(ctx, cart)
}

func (s *PharmacyService) buildCartDetail(ctx context.Context, cart queries.Cart) (CartDetail, error) {
	items, err := s.store.ListCartItems(ctx, cart.ID)
	if err != nil {
		return CartDetail{}, FromStore(err, "cart")
	}
	if items == nil {
		items = []queries.CartItem{}
	}
	var subtotal int64
	for _, it := range items {
		subtotal += int64(it.Quantity) * it.UnitPriceCents
	}
	return CartDetail{
		Cart:          cart,
		Items:         items,
		SubtotalCents: subtotal,
	}, nil
}

func parseNormalizedTags(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var tags []string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		norm := strings.ToLower(strings.TrimSpace(t))
		if norm != "" {
			out = append(out, norm)
		}
	}
	return out, nil
}

func checkMedicationSafetyGates(customer queries.Customer, medication queries.Medication) error {
	if medication.Controlled {
		return Conflict("controlled_substance: requires in-person pharmacist dispensing")
	}
	medTags, err := parseNormalizedTags(medication.ContraindicationTags)
	if err != nil {
		return Conflict("safety_data_unreadable: medication contraindication tags are not a list of strings")
	}
	if len(medTags) == 0 {
		return nil
	}
	allergyTags, err := parseNormalizedTags(customer.Allergies)
	if err != nil {
		return Conflict("safety_data_unreadable: customer allergy or condition tags are not a list of strings")
	}
	conditionTags, err := parseNormalizedTags(customer.ChronicConditions)
	if err != nil {
		return Conflict("safety_data_unreadable: customer allergy or condition tags are not a list of strings")
	}
	custTags := append(allergyTags, conditionTags...)
	custSet := make(map[string]bool, len(custTags))
	for _, t := range custTags {
		custSet[t] = true
	}
	for _, mt := range medTags {
		if custSet[mt] {
			return Conflict("contraindication_detected: medication conflicts with customer profile tag " + mt)
		}
	}
	return nil
}

// ---- Atomic FEFO Checkout & Orders -----------------------------------------

func (s *PharmacyService) CheckoutCart(ctx context.Context, cartID string, in CheckoutCartInput) (OrderDetail, error) {
	ft := clean(in.FulfillmentType)
	if ft != queries.FulfillmentPickup && ft != queries.FulfillmentDelivery {
		return OrderDetail{}, Invalid("fulfillment_type must be pickup or delivery")
	}
	cartID = clean(cartID)
	if cartID == "" {
		return OrderDetail{}, NotFound("cart")
	}
	cart, err := s.store.GetCart(ctx, cartID)
	if err != nil {
		return OrderDetail{}, FromStore(err, "cart")
	}
	branch, err := s.store.GetBranch(ctx, cart.BranchID)
	if err != nil {
		return OrderDetail{}, FromStore(err, "branch")
	}
	if ft == queries.FulfillmentDelivery && !branch.SupportsDelivery {
		return OrderDetail{}, Conflict("branch does not support delivery")
	}
	customer, err := s.store.GetCustomer(ctx, cart.CustomerID)
	if err != nil {
		return OrderDetail{}, FromStore(err, "customer")
	}
	addr := clean(in.DeliveryAddress)
	if ft == queries.FulfillmentDelivery && addr == "" {
		addr = clean(customer.Address)
	}
	if ft == queries.FulfillmentDelivery && addr == "" {
		return OrderDetail{}, Invalid("delivery_address is required for delivery orders")
	}

	var (
		createdOrder      queries.Order
		createdOrderItems []queries.OrderItem
	)

	err = s.store.InTx(ctx, func(q queries.Querier) error {
		txCart, err := q.GetCart(ctx, cartID)
		if err != nil {
			return FromStore(err, "cart")
		}
		if txCart.Status != queries.CartStatusActive {
			return Conflict("cart is not active")
		}
		items, err := q.ListCartItems(ctx, cartID)
		if err != nil {
			return FromStore(err, "cart")
		}
		if len(items) == 0 {
			return Conflict("cart is empty")
		}

		type resolvedLine struct {
			item    queries.CartItem
			batchID string
		}
		resolved := make([]resolvedLine, 0, len(items))
		var totalCents int64

		for _, item := range items {
			med, err := q.GetMedication(ctx, item.MedicationID)
			if err != nil {
				return FromStore(err, "medication")
			}
			if err := checkMedicationSafetyGates(customer, med); err != nil {
				return err
			}
			if med.RxRequired {
				if item.PrescriptionID == nil || clean(*item.PrescriptionID) == "" {
					return Conflict("rx_required: prescription invalid or out of refills")
				}
				rxID := clean(*item.PrescriptionID)
				rx, err := q.GetPrescription(ctx, rxID)
				if err != nil {
					if errors.Is(err, queries.ErrNotFound) {
						return Conflict("rx_required: prescription invalid or out of refills")
					}
					return FromStore(err, "prescription")
				}
				if rx.CustomerID != txCart.CustomerID || rx.MedicationID != med.ID {
					return Conflict("rx_required: prescription invalid or out of refills")
				}
				if _, err := q.DecrementPrescriptionRefill(ctx, rxID); err != nil {
					if errors.Is(err, queries.ErrNotFound) {
						return Conflict("rx_required: prescription invalid or out of refills")
					}
					return FromStore(err, "prescription")
				}
			}

			batch, err := q.GetSelectFefoBatchForUpdate(ctx, txCart.BranchID, item.MedicationID, item.Quantity)
			if err != nil {
				if errors.Is(err, queries.ErrNotFound) {
					return Conflict("insufficient_stock: medication unavailable at branch")
				}
				return FromStore(err, "inventory")
			}
			if _, err := q.ReserveBatchStock(ctx, batch.ID, item.Quantity); err != nil {
				if errors.Is(err, queries.ErrNotFound) {
					return Conflict("insufficient_stock: medication unavailable at branch")
				}
				return FromStore(err, "inventory")
			}
			if _, err := q.CommitBatchStock(ctx, batch.ID, item.Quantity); err != nil {
				if errors.Is(err, queries.ErrNotFound) {
					return Conflict("insufficient_stock: medication unavailable at branch")
				}
				return FromStore(err, "inventory")
			}

			totalCents += int64(item.Quantity) * item.UnitPriceCents
			resolved = append(resolved, resolvedLine{
				item:    item,
				batchID: batch.ID,
			})
		}

		cartIDCopy := txCart.ID
		createdOrder, err = q.CreateOrder(ctx, queries.CreateOrderParams{
			CustomerID:      txCart.CustomerID,
			BranchID:        txCart.BranchID,
			CartID:          &cartIDCopy,
			Status:          queries.OrderStatusConfirmed,
			FulfillmentType: ft,
			DeliveryAddress: addr,
			TotalCents:      totalCents,
		})
		if err != nil {
			return FromStore(err, "order")
		}

		createdOrderItems = make([]queries.OrderItem, 0, len(resolved))
		for _, rl := range resolved {
			oi, err := q.CreateOrderItem(ctx, queries.CreateOrderItemParams{
				OrderID:        createdOrder.ID,
				MedicationID:   rl.item.MedicationID,
				BatchID:        rl.batchID,
				Quantity:       rl.item.Quantity,
				UnitPriceCents: rl.item.UnitPriceCents,
			})
			if err != nil {
				return FromStore(err, "order item")
			}
			createdOrderItems = append(createdOrderItems, oi)
		}

		if _, err := q.UpdateCartStatus(ctx, txCart.ID, queries.CartStatusCheckedOut); err != nil {
			return FromStore(err, "cart")
		}
		return nil
	})
	if err != nil {
		var svcErr *Error
		if errors.As(err, &svcErr) {
			return OrderDetail{}, err
		}
		return OrderDetail{}, FromStore(err, "order")
	}

	return OrderDetail{
		Order: createdOrder,
		Items: createdOrderItems,
	}, nil
}

func (s *PharmacyService) ListCustomerOrders(ctx context.Context, customerID string, page Page) ([]queries.Order, error) {
	c, err := s.GetCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	page = page.Normalize()
	list, err := s.store.ListCustomerOrders(ctx, c.ID, page.Limit, page.Offset)
	if err != nil {
		return nil, FromStore(err, "order")
	}
	if list == nil {
		list = []queries.Order{}
	}
	return list, nil
}

func (s *PharmacyService) GetOrderDetail(ctx context.Context, orderID string) (OrderDetail, error) {
	orderID = clean(orderID)
	if orderID == "" {
		return OrderDetail{}, NotFound("order")
	}
	order, err := s.store.GetOrder(ctx, orderID)
	if err != nil {
		return OrderDetail{}, FromStore(err, "order")
	}
	items, err := s.store.ListOrderItems(ctx, order.ID)
	if err != nil {
		return OrderDetail{}, FromStore(err, "order")
	}
	if items == nil {
		items = []queries.OrderItem{}
	}
	return OrderDetail{
		Order: order,
		Items: items,
	}, nil
}

// ---- Refill Subscriptions --------------------------------------------------

func (s *PharmacyService) ListCustomerRefills(ctx context.Context, customerID string, page Page) ([]queries.RefillSubscription, error) {
	c, err := s.GetCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	list, err := s.store.ListCustomerRefillSubscriptions(ctx, c.ID)
	if err != nil {
		return nil, FromStore(err, "refill subscription")
	}
	if list == nil {
		list = []queries.RefillSubscription{}
	}
	return list, nil
}

func (s *PharmacyService) CreateRefill(ctx context.Context, customerID string, in CreateRefillInput) (queries.RefillSubscription, error) {
	c, err := s.GetCustomer(ctx, customerID)
	if err != nil {
		return queries.RefillSubscription{}, err
	}
	b, err := s.GetBranch(ctx, in.BranchID)
	if err != nil {
		return queries.RefillSubscription{}, err
	}
	medID := clean(in.MedicationID)
	if medID == "" {
		return queries.RefillSubscription{}, NotFound("medication")
	}
	med, err := s.store.GetMedication(ctx, medID)
	if err != nil {
		return queries.RefillSubscription{}, FromStore(err, "medication")
	}
	if !med.ChronicEligible {
		return queries.RefillSubscription{}, Conflict("medication is not eligible for chronic refill subscriptions")
	}
	if in.IntervalDays < 7 || in.IntervalDays > 90 {
		return queries.RefillSubscription{}, Invalid("interval_days must be between 7 and 90")
	}

	var rxIDPtr *string
	if med.RxRequired {
		if in.PrescriptionID == nil || clean(*in.PrescriptionID) == "" {
			return queries.RefillSubscription{}, Conflict("rx_required: valid verified prescription with remaining refills required")
		}
		rxID := clean(*in.PrescriptionID)
		rx, err := s.store.GetPrescription(ctx, rxID)
		if err != nil ||
			rx.CustomerID != c.ID ||
			rx.MedicationID != med.ID ||
			rx.Status != queries.PrescriptionStatusVerified ||
			rx.RemainingRefills <= 0 ||
			!rx.ExpiresAt.After(time.Now()) {
			return queries.RefillSubscription{}, Conflict("rx_required: valid verified prescription with remaining refills required")
		}
		rxIDPtr = &rx.ID
	} else if in.PrescriptionID != nil && clean(*in.PrescriptionID) != "" {
		rxID := clean(*in.PrescriptionID)
		rxIDPtr = &rxID
	}

	nextAt := time.Now().UTC().AddDate(0, 0, in.IntervalDays)
	if in.NextRefillAt != nil && !in.NextRefillAt.IsZero() {
		nextAt = *in.NextRefillAt
	}

	sub, err := s.store.CreateRefillSubscription(ctx, queries.CreateRefillSubscriptionParams{
		CustomerID:     c.ID,
		MedicationID:   med.ID,
		BranchID:       b.ID,
		PrescriptionID: rxIDPtr,
		IntervalDays:   in.IntervalDays,
		NextRefillAt:   nextAt,
	})
	if err != nil {
		return queries.RefillSubscription{}, FromStore(err, "refill subscription")
	}
	return sub, nil
}

func (s *PharmacyService) UpdateRefillStatus(ctx context.Context, refillID string, in UpdateRefillStatusInput) (queries.RefillSubscription, error) {
	refillID = clean(refillID)
	if refillID == "" {
		return queries.RefillSubscription{}, NotFound("refill subscription")
	}
	st := clean(in.Status)
	if st != queries.RefillStatusActive && st != queries.RefillStatusPaused && st != queries.RefillStatusCancelled {
		return queries.RefillSubscription{}, Invalid("status must be active, paused, or cancelled")
	}
	sub, err := s.store.UpdateRefillSubscriptionStatus(ctx, refillID, st)
	if err != nil {
		return queries.RefillSubscription{}, FromStore(err, "refill subscription")
	}
	return sub, nil
}
