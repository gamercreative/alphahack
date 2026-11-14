package queries

import (
	"context"
	"encoding/json"
)

// B2C pharmacy data access: branches, catalog, inventory batches, customers,
// prescriptions, carts, orders and refill subscriptions.

var (
	qListBranches = mustSQL("list_branches")
	qGetBranch    = mustSQL("get_branch")

	qListMedications   = mustSQL("list_medications")
	qSearchMedications = mustSQL("search_medications")
	qGetMedication     = mustSQL("get_medication")

	qListBranchInventory         = mustSQL("list_branch_inventory")
	qGetSelectFefoBatchForUpdate = mustSQL("get_select_fefo_batch_for_update")
	qReserveBatchStock           = mustSQL("reserve_batch_stock")
	qCommitBatchStock            = mustSQL("commit_batch_stock")
	qReleaseBatchStock           = mustSQL("release_batch_stock")

	qGetCustomer           = mustSQL("get_customer")
	qListCustomers         = mustSQL("list_customers")
	qUpdateCustomerProfile = mustSQL("update_customer_profile")

	qListCustomerPrescriptions   = mustSQL("list_customer_prescriptions")
	qGetPrescription             = mustSQL("get_prescription")
	qCreatePrescription          = mustSQL("create_prescription")
	qDecrementPrescriptionRefill = mustSQL("decrement_prescription_refill")

	qGetCart               = mustSQL("get_cart")
	qGetActiveCustomerCart = mustSQL("get_active_customer_cart")
	qCreateCart            = mustSQL("create_cart")
	qUpdateCartStatus      = mustSQL("update_cart_status")
	qListCartItems         = mustSQL("list_cart_items")
	qUpsertCartItem        = mustSQL("upsert_cart_item")
	qDeleteCartItem        = mustSQL("delete_cart_item")

	qCreateOrder        = mustSQL("create_order")
	qGetOrder           = mustSQL("get_order")
	qListCustomerOrders = mustSQL("list_customer_orders")
	qCreateOrderItem    = mustSQL("create_order_item")
	qListOrderItems     = mustSQL("list_order_items")

	qListCustomerRefillSubscriptions = mustSQL("list_customer_refill_subscriptions")
	qCreateRefillSubscription        = mustSQL("create_refill_subscription")
	qUpdateRefillSubscriptionStatus  = mustSQL("update_refill_subscription_status")
)

// emptyJSONArray is stored when a JSON list parameter is empty.
var emptyJSONArray = []byte("[]")

// jsonList returns raw as bytes for a jsonb parameter, defaulting to [].
func jsonList(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return emptyJSONArray
	}
	return []byte(raw)
}

// ---- scanners (column order matches the SQL files) ---------------------------

func scanBranch(r rowScanner) (Branch, error) {
	var b Branch
	err := r.Scan(&b.ID, &b.Name, &b.City, &b.Address, &b.Phone, &b.SupportsDelivery, &b.CreatedAt)
	return b, err
}

func scanMedication(r rowScanner) (Medication, error) {
	var m Medication
	var tags []byte
	err := r.Scan(
		&m.ID, &m.BrandName, &m.ActiveIngredient, &m.TherapeuticClass, &m.DosageForm, &m.Strength,
		&m.RxRequired, &m.Controlled, &m.ChronicEligible, &m.UnitPriceCents, &tags, &m.Description, &m.CreatedAt,
	)
	m.ContraindicationTags = json.RawMessage(tags)
	return m, err
}

func scanInventoryBatch(r rowScanner) (InventoryBatch, error) {
	var b InventoryBatch
	err := r.Scan(
		&b.ID, &b.BranchID, &b.MedicationID, &b.BatchNumber, &b.ExpiryDate,
		&b.QuantityOnHand, &b.QuantityReserved, &b.IsRecalled, &b.RecallReason, &b.UpdatedAt,
	)
	b.QuantityAvailable = b.QuantityOnHand - b.QuantityReserved
	return b, err
}

func scanCustomer(r rowScanner) (Customer, error) {
	var c Customer
	var allergies, chronic []byte
	err := r.Scan(&c.ID, &c.FullName, &c.Phone, &c.Email, &c.Address, &allergies, &chronic, &c.CreatedAt, &c.UpdatedAt)
	c.Allergies = json.RawMessage(allergies)
	c.ChronicConditions = json.RawMessage(chronic)
	return c, err
}

func scanPrescription(r rowScanner) (Prescription, error) {
	var p Prescription
	err := r.Scan(
		&p.ID, &p.CustomerID, &p.MedicationID, &p.DoctorName, &p.DosageInstructions,
		&p.AllowedRefills, &p.RemainingRefills, &p.Status, &p.Notes, &p.ExpiresAt, &p.CreatedAt,
	)
	return p, err
}

func scanCart(r rowScanner) (Cart, error) {
	var c Cart
	err := r.Scan(&c.ID, &c.CustomerID, &c.BranchID, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func scanCartItem(r rowScanner) (CartItem, error) {
	var it CartItem
	err := r.Scan(&it.ID, &it.CartID, &it.MedicationID, &it.PrescriptionID, &it.Quantity, &it.UnitPriceCents, &it.CreatedAt)
	return it, err
}

func scanOrder(r rowScanner) (Order, error) {
	var o Order
	err := r.Scan(
		&o.ID, &o.CustomerID, &o.BranchID, &o.CartID, &o.Status, &o.FulfillmentType,
		&o.DeliveryAddress, &o.TotalCents, &o.CreatedAt, &o.UpdatedAt,
	)
	return o, err
}

func scanOrderItem(r rowScanner) (OrderItem, error) {
	var it OrderItem
	err := r.Scan(&it.ID, &it.OrderID, &it.MedicationID, &it.BatchID, &it.Quantity, &it.UnitPriceCents, &it.CreatedAt)
	return it, err
}

func scanRefillSubscription(r rowScanner) (RefillSubscription, error) {
	var s RefillSubscription
	err := r.Scan(
		&s.ID, &s.CustomerID, &s.MedicationID, &s.BranchID, &s.PrescriptionID,
		&s.IntervalDays, &s.NextRefillAt, &s.Status, &s.CreatedAt,
	)
	return s, err
}

// ---- branches ----------------------------------------------------------------

func (q *Queries) ListBranches(ctx context.Context, limit, offset int) ([]Branch, error) {
	rows, err := q.db.Query(ctx, qListBranches, limit, offset)
	if err != nil {
		return nil, wrap("list branches", err)
	}
	return collect(rows, "list branches", scanBranch)
}

func (q *Queries) GetBranch(ctx context.Context, id string) (Branch, error) {
	b, err := scanBranch(q.db.QueryRow(ctx, qGetBranch, id))
	return b, wrap("get branch", err)
}

// ---- catalog -----------------------------------------------------------------

func (q *Queries) ListMedications(ctx context.Context, limit, offset int) ([]Medication, error) {
	rows, err := q.db.Query(ctx, qListMedications, limit, offset)
	if err != nil {
		return nil, wrap("list medications", err)
	}
	return collect(rows, "list medications", scanMedication)
}

func (q *Queries) SearchMedications(ctx context.Context, query string, limit, offset int) ([]Medication, error) {
	rows, err := q.db.Query(ctx, qSearchMedications, query, limit, offset)
	if err != nil {
		return nil, wrap("search medications", err)
	}
	return collect(rows, "search medications", scanMedication)
}

func (q *Queries) GetMedication(ctx context.Context, id string) (Medication, error) {
	m, err := scanMedication(q.db.QueryRow(ctx, qGetMedication, id))
	return m, wrap("get medication", err)
}

// ---- stock -------------------------------------------------------------------

func (q *Queries) ListBranchInventory(ctx context.Context, branchID, medicationID string, limit, offset int) ([]InventoryBatch, error) {
	rows, err := q.db.Query(ctx, qListBranchInventory, branchID, medicationID, limit, offset)
	if err != nil {
		return nil, wrap("list branch inventory", err)
	}
	return collect(rows, "list branch inventory", scanInventoryBatch)
}

func (q *Queries) GetSelectFefoBatchForUpdate(ctx context.Context, branchID, medicationID string, quantity int) (InventoryBatch, error) {
	b, err := scanInventoryBatch(q.db.QueryRow(ctx, qGetSelectFefoBatchForUpdate, branchID, medicationID, quantity))
	return b, wrap("select fefo batch", err)
}

func (q *Queries) ReserveBatchStock(ctx context.Context, batchID string, quantity int) (InventoryBatch, error) {
	b, err := scanInventoryBatch(q.db.QueryRow(ctx, qReserveBatchStock, batchID, quantity))
	return b, wrap("reserve batch stock", err)
}

func (q *Queries) CommitBatchStock(ctx context.Context, batchID string, quantity int) (InventoryBatch, error) {
	b, err := scanInventoryBatch(q.db.QueryRow(ctx, qCommitBatchStock, batchID, quantity))
	return b, wrap("commit batch stock", err)
}

func (q *Queries) ReleaseBatchStock(ctx context.Context, batchID string, quantity int) (InventoryBatch, error) {
	b, err := scanInventoryBatch(q.db.QueryRow(ctx, qReleaseBatchStock, batchID, quantity))
	return b, wrap("release batch stock", err)
}

// ---- customers ---------------------------------------------------------------

func (q *Queries) GetCustomer(ctx context.Context, id string) (Customer, error) {
	c, err := scanCustomer(q.db.QueryRow(ctx, qGetCustomer, id))
	return c, wrap("get customer", err)
}

func (q *Queries) ListCustomers(ctx context.Context, limit, offset int) ([]Customer, error) {
	rows, err := q.db.Query(ctx, qListCustomers, limit, offset)
	if err != nil {
		return nil, wrap("list customers", err)
	}
	return collect(rows, "list customers", scanCustomer)
}

func (q *Queries) UpdateCustomerProfile(ctx context.Context, p UpdateCustomerProfileParams) (Customer, error) {
	c, err := scanCustomer(q.db.QueryRow(ctx, qUpdateCustomerProfile,
		p.ID, p.FullName, p.Email, p.Address, jsonList(p.Allergies), jsonList(p.ChronicConditions),
	))
	return c, wrap("update customer profile", err)
}

// ---- prescriptions -----------------------------------------------------------

func (q *Queries) ListCustomerPrescriptions(ctx context.Context, customerID, status string, limit, offset int) ([]Prescription, error) {
	rows, err := q.db.Query(ctx, qListCustomerPrescriptions, customerID, status, limit, offset)
	if err != nil {
		return nil, wrap("list customer prescriptions", err)
	}
	return collect(rows, "list customer prescriptions", scanPrescription)
}

func (q *Queries) GetPrescription(ctx context.Context, id string) (Prescription, error) {
	p, err := scanPrescription(q.db.QueryRow(ctx, qGetPrescription, id))
	return p, wrap("get prescription", err)
}

func (q *Queries) CreatePrescription(ctx context.Context, p CreatePrescriptionParams) (Prescription, error) {
	rx, err := scanPrescription(q.db.QueryRow(ctx, qCreatePrescription,
		p.CustomerID, p.MedicationID, p.DoctorName, p.DosageInstructions,
		p.AllowedRefills, p.Status, p.Notes, p.ExpiresAt,
	))
	return rx, wrap("create prescription", err)
}

func (q *Queries) DecrementPrescriptionRefill(ctx context.Context, id string) (Prescription, error) {
	p, err := scanPrescription(q.db.QueryRow(ctx, qDecrementPrescriptionRefill, id))
	return p, wrap("decrement prescription refill", err)
}

// ---- carts -------------------------------------------------------------------

func (q *Queries) GetCart(ctx context.Context, id string) (Cart, error) {
	c, err := scanCart(q.db.QueryRow(ctx, qGetCart, id))
	return c, wrap("get cart", err)
}

func (q *Queries) GetActiveCustomerCart(ctx context.Context, customerID string) (Cart, error) {
	c, err := scanCart(q.db.QueryRow(ctx, qGetActiveCustomerCart, customerID))
	return c, wrap("get active customer cart", err)
}

func (q *Queries) CreateCart(ctx context.Context, p CreateCartParams) (Cart, error) {
	c, err := scanCart(q.db.QueryRow(ctx, qCreateCart, p.CustomerID, p.BranchID))
	return c, wrap("create cart", err)
}

func (q *Queries) UpdateCartStatus(ctx context.Context, id, status string) (Cart, error) {
	c, err := scanCart(q.db.QueryRow(ctx, qUpdateCartStatus, id, status))
	return c, wrap("update cart status", err)
}

func (q *Queries) ListCartItems(ctx context.Context, cartID string) ([]CartItem, error) {
	rows, err := q.db.Query(ctx, qListCartItems, cartID)
	if err != nil {
		return nil, wrap("list cart items", err)
	}
	return collect(rows, "list cart items", scanCartItem)
}

func (q *Queries) UpsertCartItem(ctx context.Context, p UpsertCartItemParams) (CartItem, error) {
	it, err := scanCartItem(q.db.QueryRow(ctx, qUpsertCartItem,
		p.CartID, p.MedicationID, p.PrescriptionID, p.Quantity, p.UnitPriceCents,
	))
	return it, wrap("upsert cart item", err)
}

func (q *Queries) DeleteCartItem(ctx context.Context, cartID, medicationID string) error {
	tag, err := q.db.Exec(ctx, qDeleteCartItem, cartID, medicationID)
	return affected("delete cart item", tag, err)
}

// ---- orders ------------------------------------------------------------------

func (q *Queries) CreateOrder(ctx context.Context, p CreateOrderParams) (Order, error) {
	o, err := scanOrder(q.db.QueryRow(ctx, qCreateOrder,
		p.CustomerID, p.BranchID, p.CartID, p.Status, p.FulfillmentType, p.DeliveryAddress, p.TotalCents,
	))
	return o, wrap("create order", err)
}

func (q *Queries) GetOrder(ctx context.Context, id string) (Order, error) {
	o, err := scanOrder(q.db.QueryRow(ctx, qGetOrder, id))
	return o, wrap("get order", err)
}

func (q *Queries) ListCustomerOrders(ctx context.Context, customerID string, limit, offset int) ([]Order, error) {
	rows, err := q.db.Query(ctx, qListCustomerOrders, customerID, limit, offset)
	if err != nil {
		return nil, wrap("list customer orders", err)
	}
	return collect(rows, "list customer orders", scanOrder)
}

func (q *Queries) CreateOrderItem(ctx context.Context, p CreateOrderItemParams) (OrderItem, error) {
	it, err := scanOrderItem(q.db.QueryRow(ctx, qCreateOrderItem,
		p.OrderID, p.MedicationID, p.BatchID, p.Quantity, p.UnitPriceCents,
	))
	return it, wrap("create order item", err)
}

func (q *Queries) ListOrderItems(ctx context.Context, orderID string) ([]OrderItem, error) {
	rows, err := q.db.Query(ctx, qListOrderItems, orderID)
	if err != nil {
		return nil, wrap("list order items", err)
	}
	return collect(rows, "list order items", scanOrderItem)
}

// ---- refill subscriptions ----------------------------------------------------

func (q *Queries) ListCustomerRefillSubscriptions(ctx context.Context, customerID string) ([]RefillSubscription, error) {
	rows, err := q.db.Query(ctx, qListCustomerRefillSubscriptions, customerID)
	if err != nil {
		return nil, wrap("list customer refill subscriptions", err)
	}
	return collect(rows, "list customer refill subscriptions", scanRefillSubscription)
}

func (q *Queries) CreateRefillSubscription(ctx context.Context, p CreateRefillSubscriptionParams) (RefillSubscription, error) {
	s, err := scanRefillSubscription(q.db.QueryRow(ctx, qCreateRefillSubscription,
		p.CustomerID, p.MedicationID, p.BranchID, p.PrescriptionID, p.IntervalDays, p.NextRefillAt,
	))
	return s, wrap("create refill subscription", err)
}

func (q *Queries) UpdateRefillSubscriptionStatus(ctx context.Context, id, status string) (RefillSubscription, error) {
	s, err := scanRefillSubscription(q.db.QueryRow(ctx, qUpdateRefillSubscriptionStatus, id, status))
	return s, wrap("update refill subscription status", err)
}
