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
	SentAt              *time.Time `json:"sent_at"` // sent_at of the enquiry outbox message
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
