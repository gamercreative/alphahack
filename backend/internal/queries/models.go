package queries

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is returned by Get*/Update*/Delete*/Finish* when no row matches.
var ErrNotFound = errors.New("queries: not found")

// Run statuses (runs.status).
const (
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
)

// Message roles (messages.role).
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Row types. These are returned as-is in API responses, so the json tags are
// part of the HTTP contract (see openapi.yaml).

type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	SystemPrompt string    `json:"system_prompt"`
	Model        string    `json:"model"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Conversation struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	RunID          *string   `json:"run_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

type Run struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	Status         string     `json:"status"`
	StopReason     string     `json:"stop_reason"`
	Error          string     `json:"error"`
	Model          string     `json:"model"`
	InputTokens    int64      `json:"input_tokens"`
	OutputTokens   int64      `json:"output_tokens"`
	Iterations     int        `json:"iterations"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

type ToolCall struct {
	ID         string          `json:"id"`
	RunID      string          `json:"run_id"`
	Name       string          `json:"name"`
	Input      json.RawMessage `json:"input"`
	Output     string          `json:"output"`
	IsError    bool            `json:"is_error"`
	DurationMS int             `json:"duration_ms"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Parameter structs.

type CreateAgentParams struct {
	Name         string
	SystemPrompt string
	Model        string
}

// UpdateAgentParams is a full replace of the mutable columns; the service
// merges PATCH input with the current row before calling UpdateAgent.
type UpdateAgentParams struct {
	ID           string
	Name         string
	SystemPrompt string
	Model        string
}

type CreateConversationParams struct {
	AgentID string
	Title   string
}

type CreateMessageParams struct {
	ConversationID string
	RunID          *string
	Role           string
	Content        string
}

type CreateRunParams struct {
	ConversationID string
	Model          string
}

// FinishRunParams sets the terminal state of a run and stamps finished_at = now().
type FinishRunParams struct {
	ID           string
	Status       string
	StopReason   string
	Error        string
	Model        string
	InputTokens  int64
	OutputTokens int64
	Iterations   int
}

type CreateToolCallParams struct {
	RunID      string
	Name       string
	Input      json.RawMessage
	Output     string
	IsError    bool
	DurationMS int
}

// ---- B2C pharmacy ------------------------------------------------------------

// Medication dosage forms (medications.dosage_form).
const (
	DosageFormTablet    = "tablet"
	DosageFormCapsule   = "capsule"
	DosageFormSyrup     = "syrup"
	DosageFormInhaler   = "inhaler"
	DosageFormInjection = "injection"
	DosageFormCream     = "cream"
	DosageFormDrops     = "drops"
)

// Prescription statuses (prescriptions.status).
const (
	PrescriptionStatusPending  = "pending"
	PrescriptionStatusVerified = "verified"
	PrescriptionStatusRejected = "rejected"
	PrescriptionStatusExpired  = "expired"
)

// Cart statuses (carts.status).
const (
	CartStatusActive     = "active"
	CartStatusCheckedOut = "checked_out"
	CartStatusAbandoned  = "abandoned"
)

// Order statuses (orders.status).
const (
	OrderStatusPendingRx      = "pending_rx"
	OrderStatusConfirmed      = "confirmed"
	OrderStatusReadyForPickup = "ready_for_pickup"
	OrderStatusOutForDelivery = "out_for_delivery"
	OrderStatusCompleted      = "completed"
	OrderStatusCancelled      = "cancelled"
)

// Fulfillment types (orders.fulfillment_type).
const (
	FulfillmentPickup   = "pickup"
	FulfillmentDelivery = "delivery"
)

// Refill subscription statuses (refill_subscriptions.status).
const (
	RefillStatusActive    = "active"
	RefillStatusPaused    = "paused"
	RefillStatusCancelled = "cancelled"
)

type Branch struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	City             string    `json:"city"`
	Address          string    `json:"address"`
	Phone            string    `json:"phone"`
	SupportsDelivery bool      `json:"supports_delivery"`
	CreatedAt        time.Time `json:"created_at"`
}

type Medication struct {
	ID                   string          `json:"id"`
	BrandName            string          `json:"brand_name"`
	ActiveIngredient     string          `json:"active_ingredient"`
	TherapeuticClass     string          `json:"therapeutic_class"`
	DosageForm           string          `json:"dosage_form"`
	Strength             string          `json:"strength"`
	RxRequired           bool            `json:"rx_required"`
	Controlled           bool            `json:"controlled"`
	ChronicEligible      bool            `json:"chronic_eligible"`
	UnitPriceCents       int64           `json:"unit_price_cents"`
	ContraindicationTags json.RawMessage `json:"contraindication_tags"` // JSON array of strings
	Description          string          `json:"description"`
	CreatedAt            time.Time       `json:"created_at"`
}

// InventoryBatch is one batch of a medication at a branch. QuantityAvailable
// is not a column: it is on_hand - reserved, computed when the row is scanned.
type InventoryBatch struct {
	ID                string    `json:"id"`
	BranchID          string    `json:"branch_id"`
	MedicationID      string    `json:"medication_id"`
	BatchNumber       string    `json:"batch_number"`
	ExpiryDate        time.Time `json:"expiry_date"`
	QuantityOnHand    int       `json:"quantity_on_hand"`
	QuantityReserved  int       `json:"quantity_reserved"`
	QuantityAvailable int       `json:"quantity_available"`
	IsRecalled        bool      `json:"is_recalled"`
	RecallReason      string    `json:"recall_reason"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Customer struct {
	ID                string          `json:"id"`
	FullName          string          `json:"full_name"`
	Phone             string          `json:"phone"`
	Email             string          `json:"email"`
	Address           string          `json:"address"`
	Allergies         json.RawMessage `json:"allergies"`          // JSON array of strings
	ChronicConditions json.RawMessage `json:"chronic_conditions"` // JSON array of strings
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type Prescription struct {
	ID                 string    `json:"id"`
	CustomerID         string    `json:"customer_id"`
	MedicationID       string    `json:"medication_id"`
	DoctorName         string    `json:"doctor_name"`
	DosageInstructions string    `json:"dosage_instructions"`
	AllowedRefills     int       `json:"allowed_refills"`
	RemainingRefills   int       `json:"remaining_refills"`
	Status             string    `json:"status"`
	Notes              string    `json:"notes"`
	ExpiresAt          time.Time `json:"expires_at"`
	CreatedAt          time.Time `json:"created_at"`
}

type Cart struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	BranchID   string    `json:"branch_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CartItem struct {
	ID             string    `json:"id"`
	CartID         string    `json:"cart_id"`
	MedicationID   string    `json:"medication_id"`
	PrescriptionID *string   `json:"prescription_id"`
	Quantity       int       `json:"quantity"`
	UnitPriceCents int64     `json:"unit_price_cents"`
	CreatedAt      time.Time `json:"created_at"`
}

type Order struct {
	ID              string    `json:"id"`
	CustomerID      string    `json:"customer_id"`
	BranchID        string    `json:"branch_id"`
	CartID          *string   `json:"cart_id"`
	Status          string    `json:"status"`
	FulfillmentType string    `json:"fulfillment_type"`
	DeliveryAddress string    `json:"delivery_address"`
	TotalCents      int64     `json:"total_cents"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type OrderItem struct {
	ID             string    `json:"id"`
	OrderID        string    `json:"order_id"`
	MedicationID   string    `json:"medication_id"`
	BatchID        string    `json:"batch_id"`
	Quantity       int       `json:"quantity"`
	UnitPriceCents int64     `json:"unit_price_cents"`
	CreatedAt      time.Time `json:"created_at"`
}

type RefillSubscription struct {
	ID             string    `json:"id"`
	CustomerID     string    `json:"customer_id"`
	MedicationID   string    `json:"medication_id"`
	BranchID       string    `json:"branch_id"`
	PrescriptionID *string   `json:"prescription_id"`
	IntervalDays   int       `json:"interval_days"`
	NextRefillAt   time.Time `json:"next_refill_at"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// Pharmacy parameter structs.

// UpdateCustomerProfileParams is a full replace of the mutable profile
// columns (phone is the identity and is not changed here). Empty JSON lists
// are stored as [].
type UpdateCustomerProfileParams struct {
	ID                string
	FullName          string
	Email             string
	Address           string
	Allergies         json.RawMessage
	ChronicConditions json.RawMessage
}

// CreatePrescriptionParams: remaining_refills starts equal to AllowedRefills.
type CreatePrescriptionParams struct {
	CustomerID         string
	MedicationID       string
	DoctorName         string
	DosageInstructions string
	AllowedRefills     int
	Status             string
	Notes              string
	ExpiresAt          time.Time
}

type CreateCartParams struct {
	CustomerID string
	BranchID   string
}

// UpsertCartItemParams inserts a line or, when the cart already has this
// medication, replaces its quantity, price and prescription.
type UpsertCartItemParams struct {
	CartID         string
	MedicationID   string
	PrescriptionID *string
	Quantity       int
	UnitPriceCents int64
}

type CreateOrderParams struct {
	CustomerID      string
	BranchID        string
	CartID          *string
	Status          string
	FulfillmentType string
	DeliveryAddress string
	TotalCents      int64
}

type CreateOrderItemParams struct {
	OrderID        string
	MedicationID   string
	BatchID        string
	Quantity       int
	UnitPriceCents int64
}

type CreateRefillSubscriptionParams struct {
	CustomerID     string
	MedicationID   string
	BranchID       string
	PrescriptionID *string
	IntervalDays   int
	NextRefillAt   time.Time
}
