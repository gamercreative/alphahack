package queries

import (
	"encoding/json"
	"time"
)

// Wassit enquiry domain (migration 0004_enquiries). Owned by the lead; see CONTRACT.md.
// Nullable columns are pointers. JSON tags are part of the HTTP contract.

// Enquiry statuses (enquiries.status).
const (
	EnquiryOpen                  = "open"
	EnquiryAwaitingClarification = "awaiting_clarification"
	EnquiryAwaitingApproval      = "awaiting_approval"
	EnquiryWaiting               = "waiting"
	EnquiryClosed                = "closed"
	EnquiryCancelled             = "cancelled"
)

// Enquiry outcomes (enquiries.outcome).
const (
	OutcomeNone            = ""
	OutcomeAvailable       = "available"
	OutcomeUnavailable     = "unavailable"
	OutcomeUnableToConfirm = "unable_to_confirm"
	OutcomeCancelled       = "cancelled"
)

// Target statuses (enquiry_targets.status).
const (
	TargetProposed           = "proposed"
	TargetApproved           = "approved"
	TargetSent               = "sent"
	TargetAvailable          = "available"
	TargetUnavailable        = "unavailable"
	TargetAlternativeOffered = "alternative_offered"
	TargetNoResponse         = "no_response"
	TargetCancelled          = "cancelled"
)

// Outbox kinds and statuses.
const (
	OutboxKindEnquiry      = "enquiry"
	OutboxKindFollowUp     = "follow_up"
	OutboxKindRetraction   = "retraction"
	OutboxKindNotification = "notification"

	OutboxQueued    = "queued"
	OutboxSent      = "sent"
	OutboxCancelled = "cancelled"
)

// Actor types (enquiry_events.actor_type).
const (
	ActorStaff  = "staff"
	ActorAgent  = "agent"
	ActorSystem = "system"
	ActorClient = "client" // a DOSYN client (migration 0008)
)

type Staff struct {
	ID        string    `json:"id"`
	BranchID  string    `json:"branch_id"`
	FullName  string    `json:"full_name"`
	Email     string    `json:"email"`
	PinHash   string    `json:"-"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Enquiry struct {
	ID                    string     `json:"id"`
	RequesterBranchID     string     `json:"requester_branch_id"`
	RequesterStaffID      string     `json:"requester_staff_id"`
	RawRequest            string     `json:"raw_request"`
	MedicationID          *string    `json:"medication_id"`
	Quantity              int        `json:"quantity"`
	Status                string     `json:"status"`
	Outcome               string     `json:"outcome"`
	OutcomeBranchID       *string    `json:"outcome_branch_id"`
	HandledBy             string     `json:"handled_by"`
	ClarificationQuestion string     `json:"clarification_question"`
	ClarificationAnswer   string     `json:"clarification_answer"`
	WakeTrigger           string     `json:"wake_trigger"`
	NextWakeAt            *time.Time `json:"next_wake_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	ClosedAt              *time.Time `json:"closed_at"`
}

type EnquiryTarget struct {
	ID               string     `json:"id"`
	EnquiryID        string     `json:"enquiry_id"`
	BranchID         string     `json:"branch_id"`
	Status           string     `json:"status"`
	ReplyQuantity    *int       `json:"reply_quantity"`
	ReplyText        string     `json:"reply_text"`
	RepliedByStaffID *string    `json:"replied_by_staff_id"`
	RepliedAt        *time.Time `json:"replied_at"`
	FollowUps        int        `json:"follow_ups"`
	CreatedAt        time.Time  `json:"created_at"`
}

type OutboxMessage struct {
	ID             string     `json:"id"`
	EnquiryID      string     `json:"enquiry_id"`
	TargetID       *string    `json:"target_id"`
	Kind           string     `json:"kind"`
	Body           string     `json:"body"`
	Status         string     `json:"status"`
	SendAfter      time.Time  `json:"send_after"`
	SentAt         *time.Time `json:"sent_at"`
	CreatedByActor string     `json:"created_by_actor"`
	CreatedAt      time.Time  `json:"created_at"`
}

type EnquiryEvent struct {
	ID              string          `json:"id"`
	Seq             int64           `json:"seq"`
	EnquiryID       string          `json:"enquiry_id"`
	ActorType       string          `json:"actor_type"`
	ActorID         string          `json:"actor_id"`
	Type            string          `json:"type"`
	Payload         json.RawMessage `json:"payload"`
	Inverse         json.RawMessage `json:"inverse"` // null when not reversible
	UndoneAt        *time.Time      `json:"undone_at"`
	UndoneByEventID *string         `json:"undone_by_event_id"`
	CreatedAt       time.Time       `json:"created_at"`
}

type EnquiryAgentRun struct {
	ID           string     `json:"id"`
	EnquiryID    string     `json:"enquiry_id"`
	Trigger      string     `json:"trigger"`
	Model        string     `json:"model"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	Steps        int        `json:"steps"`
	CostMicros   int64      `json:"cost_micros"`
	Status       string     `json:"status"`
	Error        string     `json:"error"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

type EnquiryAgentToolCall struct {
	ID         string          `json:"id"`
	Seq        int64           `json:"seq"`
	RunID      string          `json:"run_id"`
	Name       string          `json:"name"`
	Input      json.RawMessage `json:"input"`
	Output     string          `json:"output"`
	IsError    bool            `json:"is_error"`
	DurationMs int             `json:"duration_ms"`
	CreatedAt  time.Time       `json:"created_at"`
}

// CandidateBranch is a participating branch plus its reported stock of one medication
// (sum of quantity_on_hand - quantity_reserved over unrecalled, unexpired batches; 0 if none).
type CandidateBranch struct {
	BranchID          string     `json:"branch_id"`
	Name              string     `json:"name"`
	City              string     `json:"city"`
	Lat               *float64   `json:"lat"`
	Lng               *float64   `json:"lng"`
	AvailableQuantity int        `json:"available_quantity"`
	StockUpdatedAt    *time.Time `json:"stock_updated_at"`
}

// InboxItem is one target at the caller's branch whose enquiry message was sent.
type InboxItem struct {
	TargetID            string     `json:"target_id"`
	TargetStatus        string     `json:"target_status"`
	FollowUps           int        `json:"follow_ups"`
	EnquiryID           string     `json:"enquiry_id"`
	RequesterBranchID   string     `json:"requester_branch_id"`
	RequesterBranchName string     `json:"requester_branch_name"`
	MedicationID        *string    `json:"medication_id"`
	MedicationName      string     `json:"medication_name"` // "Brand strength form", "" if unset
	RawRequest          string     `json:"raw_request"`
	Quantity            int        `json:"quantity"`
	SentAt              *time.Time `json:"sent_at"`        // sent_at of the enquiry outbox message
	RequesterKind       string     `json:"requester_kind"` // "client" (DOSYN client desk) | "pharmacy"
	PickupWindow        string     `json:"pickup_window"`  // client tickets only, else ""
}

type AgentWakeup struct {
	EnquiryID string `json:"enquiry_id"`
	Trigger   string `json:"trigger"`
}

type EnquiryStat struct {
	EnquiryID    string    `json:"enquiry_id"`
	RawRequest   string    `json:"raw_request"`
	HandledBy    string    `json:"handled_by"`
	Status       string    `json:"status"`
	Outcome      string    `json:"outcome"`
	HITLCount    int       `json:"hitl_count"` // clarified + targets_approved events, not undone
	AgentRuns    int       `json:"agent_runs"`
	AgentSteps   int       `json:"agent_steps"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CostMicros   int64     `json:"cost_micros"`
	CreatedAt    time.Time `json:"created_at"`
}

// ---- params ----

type CreateStaffSessionParams struct {
	TokenHash string
	StaffID   string
	ExpiresAt time.Time
}

type CreateEnquiryParams struct {
	RequesterBranchID string
	RequesterStaffID  string
	RawRequest        string
	MedicationID      *string
	Quantity          int
	Status            string
	HandledBy         string
	WakeTrigger       string
}

// UpdateEnquiryParams overwrites every mutable column and sets updated_at = now().
type UpdateEnquiryParams struct {
	ID                    string
	MedicationID          *string
	Status                string
	Outcome               string
	OutcomeBranchID       *string
	ClarificationQuestion string
	ClarificationAnswer   string
	WakeTrigger           string
	NextWakeAt            *time.Time
	ClosedAt              *time.Time
}

type CreateEnquiryTargetParams struct {
	EnquiryID string
	BranchID  string
	Status    string
}

// UpdateEnquiryTargetParams overwrites every mutable column.
type UpdateEnquiryTargetParams struct {
	ID               string
	Status           string
	ReplyQuantity    *int
	ReplyText        string
	RepliedByStaffID *string
	RepliedAt        *time.Time
	FollowUps        int
}

type CreateOutboxMessageParams struct {
	EnquiryID      string
	TargetID       *string
	Kind           string
	Body           string
	SendAfter      time.Time
	CreatedByActor string
}

type CreateEnquiryEventParams struct {
	EnquiryID string
	ActorType string
	ActorID   string
	Type      string
	Payload   json.RawMessage // nil -> {}
	Inverse   json.RawMessage // nil -> NULL
}

type CreateEnquiryAgentRunParams struct {
	EnquiryID string
	Trigger   string
	Model     string
}

type FinishEnquiryAgentRunParams struct {
	ID           string
	Status       string
	InputTokens  int
	OutputTokens int
	Steps        int
	CostMicros   int64
	Error        string
	FinishedAt   *time.Time
}

type CreateEnquiryAgentToolCallParams struct {
	RunID      string
	Name       string
	Input      json.RawMessage // nil -> {}
	Output     string
	IsError    bool
	DurationMs int
}

// Offer decisions (target_offers.decision), set by the Go rules in services.
const (
	OfferMatches   = "matches"
	OfferEscalated = "escalated"
	OfferRejected  = "rejected"
)

// TargetOffer is a structured offer read from one pharmacist reply (migration 0006).
type TargetOffer struct {
	ID                string          `json:"id"`
	EnquiryID         string          `json:"enquiry_id"`
	TargetID          string          `json:"target_id"`
	BranchID          string          `json:"branch_id"`
	SourceText        string          `json:"source_text"`
	ExtractedBy       string          `json:"extracted_by"`
	InStock           *bool           `json:"in_stock"`
	Strength          string          `json:"strength"`
	Quantity          *int            `json:"quantity"`
	PriceCents        *int64          `json:"price_cents"`
	Currency          string          `json:"currency"`
	PickupBy          string          `json:"pickup_by"`
	Payment           string          `json:"payment"`
	NeedsPrescription *bool           `json:"needs_prescription"`
	Evidence          json.RawMessage `json:"evidence"`
	Confidence        float64         `json:"confidence"`
	Decision          string          `json:"decision"`
	DecidedBy         string          `json:"decided_by"`
	Reasons           json.RawMessage `json:"reasons"`
	CreatedAt         time.Time       `json:"created_at"`
}

type CreateTargetOfferParams struct {
	EnquiryID         string
	TargetID          string
	BranchID          string
	SourceText        string
	ExtractedBy       string
	InStock           *bool
	Strength          string
	Quantity          *int
	PriceCents        *int64
	Currency          string
	PickupBy          string
	Payment           string
	NeedsPrescription *bool
	Evidence          json.RawMessage
	Confidence        float64
	Decision          string
	DecidedBy         string
	Reasons           json.RawMessage
}

// ---- Government reference (migration 0007_gov_reference) ----

// Source label of the seeded rows (never real MoPH data).
const GovSourceDemoSeed = "Demo seed, not real MoPH data"

type GovProduct struct {
	Code               string    `json:"code"`
	RegNo              string    `json:"reg_no"`
	BrandName          string    `json:"brand_name"`
	Ingredient         string    `json:"ingredient"`
	Strength           string    `json:"strength"`
	DosageForm         string    `json:"dosage_form"`
	OfficialPriceCents int64     `json:"official_price_cents"`
	Source             string    `json:"source"`
	DocumentID         *string   `json:"document_id"`
	CreatedAt          time.Time `json:"created_at"`
}

type CreateGovProductParams struct {
	Code               string
	RegNo              string
	BrandName          string
	Ingredient         string
	Strength           string
	DosageForm         string
	OfficialPriceCents int64
	Source             string
	DocumentID         *string
}

// ListGovProductsParams: empty strings mean "no filter". Q is an ILIKE pattern
// fragment whose % _ \ are already escaped by the caller.
type ListGovProductsParams struct {
	Q          string
	Ingredient string
	Strength   string
	DosageForm string
	Limit      int
	Offset     int
}

// ReferenceDocument: StoragePath and UploadedByStaffID stay server side (json "-").
type ReferenceDocument struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	ContentType       string    `json:"content_type"`
	SizeBytes         int64     `json:"size_bytes"`
	SHA256            string    `json:"sha256"`
	UploadedByStaffID string    `json:"-"`
	StoragePath       string    `json:"-"`
	CreatedAt         time.Time `json:"created_at"`
}

type CreateReferenceDocumentParams struct {
	ID                string
	Name              string
	ContentType       string
	SizeBytes         int64
	SHA256            string
	UploadedByStaffID string
	StoragePath       string
}

// CreateInventoryBatchParams inserts a fresh, unreserved, unrecalled batch.
type CreateInventoryBatchParams struct {
	BranchID       string
	MedicationID   string
	BatchNumber    string
	ExpiryDate     time.Time
	QuantityOnHand int
}

// ---- DOSYN clients, tickets, schedules, photos (migrations 0008-0010) ----

// DeskBranchID is the requester branch of every client ticket (participating = false).
const DeskBranchID = "branch-client-desk"

// DeskStaffID is the desk reviewer who is the requester staff of client tickets.
const DeskStaffID = "staff-client-desk"

type Client struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Phone       string    `json:"-"`
	PinHash     string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateClientSessionParams struct {
	TokenHash string
	ClientID  string
	ExpiresAt time.Time
}

// Client ticket statuses (client_tickets.status) and modes.
const (
	TicketOpen      = "open"
	TicketReserved  = "reserved"
	TicketClosed    = "closed"
	TicketOnce      = "once"
	TicketScheduled = "scheduled"
)

type ClientTicket struct {
	ID               string    `json:"id"`
	ClientID         string    `json:"client_id"`
	EnquiryID        string    `json:"enquiry_id"`
	ScheduleID       *string   `json:"schedule_id"`
	Mode             string    `json:"mode"`
	PickupWindow     string    `json:"pickup_window"`
	Status           string    `json:"status"`
	ReservedBranchID *string   `json:"reserved_branch_id"`
	CreatedAt        time.Time `json:"created_at"`
}

type CreateClientTicketParams struct {
	ClientID     string
	EnquiryID    string
	ScheduleID   *string
	Mode         string
	PickupWindow string
}

type MedicineSchedule struct {
	ID           string    `json:"id"`
	ClientID     string    `json:"client_id"`
	MedicationID string    `json:"medication_id"`
	Quantity     int       `json:"quantity"`
	Every        string    `json:"every"`   // week | 2weeks | month
	Weekday      int       `json:"weekday"` // 0 = Sunday
	PickupWindow string    `json:"pickup_window"`
	NoBulk       bool      `json:"no_bulk"`
	Status       string    `json:"status"` // active | paused
	NextRunAt    time.Time `json:"next_run_at"`
	CreatedAt    time.Time `json:"created_at"`
}

type CreateMedicineScheduleParams struct {
	ClientID     string
	MedicationID string
	Quantity     int
	Every        string
	Weekday      int
	PickupWindow string
	NextRunAt    time.Time
}

// Package photo statuses (package_photos.status).
const (
	PhotoVerified = "verified"
	PhotoFlagged  = "flagged"
	PhotoMismatch = "mismatch"
	PhotoRejected = "rejected"
)

type PackagePhoto struct {
	ID                string     `json:"id"`
	TargetID          string     `json:"target_id"`
	BranchID          string     `json:"branch_id"`
	StoragePath       string     `json:"-"`
	ContentType       string     `json:"content_type"`
	SizeBytes         int64      `json:"size_bytes"`
	TypedCode         string     `json:"typed_code"`
	TypedExpiryMonth  string     `json:"expiry_month"`
	Status            string     `json:"status"`
	GovMatch          bool       `json:"gov_match"`
	DecidedBy         string     `json:"decided_by"`
	Note              string     `json:"note"`
	ReviewedByStaffID *string    `json:"reviewed_by_staff_id"`
	AgentCode         string     `json:"agent_code"`
	AgentExpiryMonth  string     `json:"agent_expiry_month"`
	AgentConfidence   *float64   `json:"agent_confidence"`
	ReadAt            *time.Time `json:"read_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

// RecordPhotoAgentReadingParams stores the agent's reading and the rules' new
// verdict. Only matches while decided_by = 'rules' (a human review is final).
type RecordPhotoAgentReadingParams struct {
	ID               string
	AgentCode        string
	AgentExpiryMonth string
	AgentConfidence  float64
	Status           string
	GovMatch         bool
	Note             string
}

type CreatePackagePhotoParams struct {
	ID               string
	TargetID         string
	BranchID         string
	StoragePath      string
	ContentType      string
	SizeBytes        int64
	TypedCode        string
	TypedExpiryMonth string
	Status           string
	GovMatch         bool
	Note             string
}

type ReviewPackagePhotoParams struct {
	ID                string
	Status            string
	Note              string
	ReviewedByStaffID string
}
