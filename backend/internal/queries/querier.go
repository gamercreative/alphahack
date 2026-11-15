// Package queries is the only package that talks to PostgreSQL.
//
// Contract (owned by the orchestrator, do not change signatures):
//   - Every SQL statement lives in its own file internal/queries/sql/<method_snake_case>.sql
//     (e.g. CreateAgent -> sql/create_agent.sql), embedded with //go:embed. One statement
//     per file, positional params ($1, $2 ...), no string concatenation.
//   - scripts/check_queries.sh PREPAREs every one of those files against the live schema.
//   - Get*/Update*/Delete*/Finish* return ErrNotFound when no row matches.
//   - List* return a non-nil empty slice when there are no rows (so JSON is [] not null).
//   - No business rules here: no validation, no defaults beyond what the schema provides.
package queries

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is satisfied by *pgxpool.Pool and pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Querier is the full data-access surface. *Queries (constructed with
// New(db DBTX) *Queries) must satisfy it:
//
//	var _ Querier = (*Queries)(nil)
type Querier interface {
	// Ping runs a trivial query to prove the database is reachable.
	Ping(ctx context.Context) error

	// InTx runs fn inside a transaction; the Querier passed to fn is bound to
	// that transaction. Commit when fn returns nil, roll back otherwise
	// (including on panic, which is re-raised).
	InTx(ctx context.Context, fn func(q Querier) error) error

	// Agents. ListAgents orders by created_at DESC, id.
	CreateAgent(ctx context.Context, p CreateAgentParams) (Agent, error)
	GetAgent(ctx context.Context, id string) (Agent, error)
	ListAgents(ctx context.Context, limit, offset int) ([]Agent, error)
	UpdateAgent(ctx context.Context, p UpdateAgentParams) (Agent, error) // also sets updated_at = now()
	DeleteAgent(ctx context.Context, id string) error

	// Conversations. ListConversations orders by updated_at DESC, id; an empty
	// agentID means "all agents".
	CreateConversation(ctx context.Context, p CreateConversationParams) (Conversation, error)
	GetConversation(ctx context.Context, id string) (Conversation, error)
	ListConversations(ctx context.Context, agentID string, limit, offset int) ([]Conversation, error)
	TouchConversation(ctx context.Context, id string) error // updated_at = now()
	DeleteConversation(ctx context.Context, id string) error

	// Messages. ListMessages orders by seq ASC (oldest first).
	// ListRecentMessages returns the newest `limit` messages, still ordered seq ASC.
	CreateMessage(ctx context.Context, p CreateMessageParams) (Message, error)
	ListMessages(ctx context.Context, conversationID string, limit, offset int) ([]Message, error)
	ListRecentMessages(ctx context.Context, conversationID string, limit int) ([]Message, error)

	// Runs. ListRuns orders by started_at DESC, id.
	CreateRun(ctx context.Context, p CreateRunParams) (Run, error)
	FinishRun(ctx context.Context, p FinishRunParams) (Run, error)
	GetRun(ctx context.Context, id string) (Run, error)
	ListRuns(ctx context.Context, conversationID string, limit, offset int) ([]Run, error)

	// Tool calls. ListToolCalls orders by seq ASC.
	CreateToolCall(ctx context.Context, p CreateToolCallParams) (ToolCall, error)
	ListToolCalls(ctx context.Context, runID string) ([]ToolCall, error)

	// ---- B2C pharmacy (pharmacy.go) ----

	// Branches. ListBranches orders by name, id.
	ListBranches(ctx context.Context, limit, offset int) ([]Branch, error)
	GetBranch(ctx context.Context, id string) (Branch, error)

	// Catalog. ListMedications and SearchMedications order by brand_name, id.
	// SearchMedications matches query (case-insensitive substring) against
	// brand_name, active_ingredient and therapeutic_class.
	ListMedications(ctx context.Context, limit, offset int) ([]Medication, error)
	SearchMedications(ctx context.Context, query string, limit, offset int) ([]Medication, error)
	GetMedication(ctx context.Context, id string) (Medication, error)

	// Stock. Only sellable batches are returned: not recalled, not expired and
	// with quantity available (on_hand - reserved) > 0, ordered by expiry_date ASC
	// (FEFO). An empty medicationID means every medication at the branch.
	ListBranchInventory(ctx context.Context, branchID, medicationID string, limit, offset int) ([]InventoryBatch, error)
	// GetSelectFefoBatchForUpdate locks (FOR UPDATE SKIP LOCKED) the sellable
	// batch with the earliest expiry that can cover quantity on its own. Use it
	// inside InTx. ErrNotFound when no single batch can cover quantity.
	GetSelectFefoBatchForUpdate(ctx context.Context, branchID, medicationID string, quantity int) (InventoryBatch, error)
	// ReserveBatchStock adds quantity to quantity_reserved. ErrNotFound when the
	// batch does not exist, is recalled/expired, or has less than quantity available.
	ReserveBatchStock(ctx context.Context, batchID string, quantity int) (InventoryBatch, error)
	// CommitBatchStock removes a reserved quantity from on_hand and reserved
	// (the sale). ErrNotFound when the batch does not exist or reserved < quantity.
	CommitBatchStock(ctx context.Context, batchID string, quantity int) (InventoryBatch, error)
	// ReleaseBatchStock gives a reservation back. ErrNotFound when the batch
	// does not exist or reserved < quantity.
	ReleaseBatchStock(ctx context.Context, batchID string, quantity int) (InventoryBatch, error)

	// Customers. ListCustomers orders by full_name, id.
	GetCustomer(ctx context.Context, id string) (Customer, error)
	ListCustomers(ctx context.Context, limit, offset int) ([]Customer, error)
	UpdateCustomerProfile(ctx context.Context, p UpdateCustomerProfileParams) (Customer, error) // also sets updated_at = now()

	// Prescriptions. ListCustomerPrescriptions orders by created_at DESC, id;
	// an empty status means every status.
	ListCustomerPrescriptions(ctx context.Context, customerID, status string, limit, offset int) ([]Prescription, error)
	GetPrescription(ctx context.Context, id string) (Prescription, error)
	CreatePrescription(ctx context.Context, p CreatePrescriptionParams) (Prescription, error)
	// DecrementPrescriptionRefill uses one refill. ErrNotFound when the
	// prescription does not exist, is not verified, is past expires_at, or has
	// no refills left.
	DecrementPrescriptionRefill(ctx context.Context, id string) (Prescription, error)

	// Carts. GetActiveCustomerCart returns the most recently updated active
	// cart, or ErrNotFound. ListCartItems orders by created_at, id.
	GetCart(ctx context.Context, id string) (Cart, error)
	GetActiveCustomerCart(ctx context.Context, customerID string) (Cart, error)
	CreateCart(ctx context.Context, p CreateCartParams) (Cart, error)
	UpdateCartStatus(ctx context.Context, id, status string) (Cart, error) // also sets updated_at = now()
	ListCartItems(ctx context.Context, cartID string) ([]CartItem, error)
	UpsertCartItem(ctx context.Context, p UpsertCartItemParams) (CartItem, error)
	DeleteCartItem(ctx context.Context, cartID, medicationID string) error

	// Orders. ListCustomerOrders orders by created_at DESC, id. ListOrderItems
	// orders by created_at, id.
	CreateOrder(ctx context.Context, p CreateOrderParams) (Order, error)
	GetOrder(ctx context.Context, id string) (Order, error)
	ListCustomerOrders(ctx context.Context, customerID string, limit, offset int) ([]Order, error)
	CreateOrderItem(ctx context.Context, p CreateOrderItemParams) (OrderItem, error)
	ListOrderItems(ctx context.Context, orderID string) ([]OrderItem, error)

	// Refill subscriptions. ListCustomerRefillSubscriptions orders by next_refill_at, id.
	ListCustomerRefillSubscriptions(ctx context.Context, customerID string) ([]RefillSubscription, error)
	CreateRefillSubscription(ctx context.Context, p CreateRefillSubscriptionParams) (RefillSubscription, error)
	UpdateRefillSubscriptionStatus(ctx context.Context, id, status string) (RefillSubscription, error)

	// ---- Wassit enquiries (enquiry.go; models in enquiry_models.go) ----

	// Staff + sessions. GetStaffBySessionToken joins staff_sessions on token_hash
	// and only matches when expires_at > now().
	GetStaffByEmail(ctx context.Context, email string) (Staff, error) // case-insensitive
	GetStaff(ctx context.Context, id string) (Staff, error)
	CreateStaffSession(ctx context.Context, p CreateStaffSessionParams) error
	GetStaffBySessionToken(ctx context.Context, tokenHash string) (Staff, error)

	// Enquiries. Lists order by created_at DESC, id; an empty status means all.
	// ListIncomingEnquiries: enquiries with a target at branchID whose status is
	// not proposed/approved. GetEnquiryForUpdate adds FOR UPDATE (use inside InTx).
	CreateEnquiry(ctx context.Context, p CreateEnquiryParams) (Enquiry, error)
	GetEnquiry(ctx context.Context, id string) (Enquiry, error)
	GetEnquiryForUpdate(ctx context.Context, id string) (Enquiry, error)
	ListOutgoingEnquiries(ctx context.Context, branchID, status string, limit, offset int) ([]Enquiry, error)
	ListIncomingEnquiries(ctx context.Context, branchID, status string, limit, offset int) ([]Enquiry, error)
	UpdateEnquiry(ctx context.Context, p UpdateEnquiryParams) (Enquiry, error) // sets updated_at = now()

	// Agent wake-ups: handled_by = 'agent', status not closed/cancelled, and
	// (wake_trigger <> '' OR next_wake_at <= now()). Trigger is wake_trigger, or
	// 'timer' when wake_trigger is empty. Ordered by updated_at, id.
	ListAgentWakeups(ctx context.Context, limit int) ([]AgentWakeup, error)
	ClearEnquiryWake(ctx context.Context, id string) error // wake_trigger = '', next_wake_at = NULL

	// Targets. ListEnquiryTargets orders by created_at, id.
	CreateEnquiryTarget(ctx context.Context, p CreateEnquiryTargetParams) (EnquiryTarget, error)
	GetEnquiryTarget(ctx context.Context, id string) (EnquiryTarget, error)
	ListEnquiryTargets(ctx context.Context, enquiryID string) ([]EnquiryTarget, error)
	UpdateEnquiryTarget(ctx context.Context, p UpdateEnquiryTargetParams) (EnquiryTarget, error)
	DeleteEnquiryTarget(ctx context.Context, id string) error

	// Inbox: targets at branchID that have an outbox message of kind 'enquiry'
	// with status 'sent'. Ordered by that message's sent_at DESC, target id.
	ListInbox(ctx context.Context, branchID string, limit, offset int) ([]InboxItem, error)

	// Candidates: participating branches other than excludeBranchID, each with its
	// stock of medicationID (see CandidateBranch). No ranking here; ordered by id.
	ListCandidateBranches(ctx context.Context, medicationID, excludeBranchID string) ([]CandidateBranch, error)

	// Outbox. ListOutboxMessages orders by created_at, id. ListDueOutboxMessages
	// returns queued rows with send_after <= now(), oldest first, FOR UPDATE SKIP LOCKED.
	CreateOutboxMessage(ctx context.Context, p CreateOutboxMessageParams) (OutboxMessage, error)
	GetOutboxMessage(ctx context.Context, id string) (OutboxMessage, error)
	ListOutboxMessages(ctx context.Context, enquiryID string) ([]OutboxMessage, error)
	UpdateOutboxMessageStatus(ctx context.Context, id, status string, sentAt *time.Time) (OutboxMessage, error)
	ListDueOutboxMessages(ctx context.Context, limit int) ([]OutboxMessage, error)

	// Events. ListEnquiryEvents returns seq > afterSeq, ordered by seq ASC.
	CreateEnquiryEvent(ctx context.Context, p CreateEnquiryEventParams) (EnquiryEvent, error)
	GetEnquiryEvent(ctx context.Context, id string) (EnquiryEvent, error)
	ListEnquiryEvents(ctx context.Context, enquiryID string, afterSeq int64) ([]EnquiryEvent, error)
	MarkEnquiryEventUndone(ctx context.Context, id, undoneByEventID string) (EnquiryEvent, error) // undone_at = now()

	// Agent runs + tool calls. ListEnquiryAgentRuns orders by started_at, id.
	CreateEnquiryAgentRun(ctx context.Context, p CreateEnquiryAgentRunParams) (EnquiryAgentRun, error)
	GetEnquiryAgentRun(ctx context.Context, id string) (EnquiryAgentRun, error)
	FinishEnquiryAgentRun(ctx context.Context, p FinishEnquiryAgentRunParams) (EnquiryAgentRun, error)
	ListEnquiryAgentRuns(ctx context.Context, enquiryID string) ([]EnquiryAgentRun, error)
	CreateEnquiryAgentToolCall(ctx context.Context, p CreateEnquiryAgentToolCallParams) (EnquiryAgentToolCall, error)

	// Offers (migration 0006). ListTargetOffers orders by created_at, id.
	CreateTargetOffer(ctx context.Context, p CreateTargetOfferParams) (TargetOffer, error)
	ListTargetOffers(ctx context.Context, enquiryID string) ([]TargetOffer, error)

	// Stats: one row per enquiry, newest first.
	ListEnquiryStats(ctx context.Context, limit, offset int) ([]EnquiryStat, error)

	// ---- Government reference + staff stock (reference.go, migration 0007) ----

	// ListGovProducts: ingredient/strength/form are case-insensitive exact
	// filters, Q an ILIKE fragment on brand_name or ingredient; "" = no filter.
	// Ordered by ingredient, brand_name, dosage_form, strength, code.
	ListGovProducts(ctx context.Context, p ListGovProductsParams) ([]GovProduct, error)
	// CreateGovProduct: ErrDuplicate on a taken code or (ingredient, strength,
	// dosage_form, brand_name); ErrBadReference on an unknown document_id.
	CreateGovProduct(ctx context.Context, p CreateGovProductParams) (GovProduct, error)
	// Reference documents. ListReferenceDocuments orders by created_at DESC, id.
	CreateReferenceDocument(ctx context.Context, p CreateReferenceDocumentParams) (ReferenceDocument, error)
	ListReferenceDocuments(ctx context.Context, limit, offset int) ([]ReferenceDocument, error)

	// GetStockBatchForUpdate locks (FOR UPDATE) the soonest-expiring unexpired,
	// unrecalled batch of medicationID at branchID, whatever its quantity.
	// ErrNotFound when there is none. Use inside InTx.
	GetStockBatchForUpdate(ctx context.Context, branchID, medicationID string) (InventoryBatch, error)
	// SetBatchQuantityOnHand sets quantity_on_hand (and updated_at). ErrNotFound when no batch.
	SetBatchQuantityOnHand(ctx context.Context, batchID string, quantity int) (InventoryBatch, error)
	CreateInventoryBatch(ctx context.Context, p CreateInventoryBatchParams) (InventoryBatch, error)

	// ---- DOSYN clients, tickets, schedules, photos (clients.go, migrations 0008-0010) ----

	// Clients + sessions. GetClientBySessionToken only matches when expires_at > now().
	GetClientByPhone(ctx context.Context, phone string) (Client, error)
	GetClient(ctx context.Context, id string) (Client, error)
	CreateClientSession(ctx context.Context, p CreateClientSessionParams) error
	GetClientBySessionToken(ctx context.Context, tokenHash string) (Client, error)

	// Client tickets. ListClientTickets orders by created_at DESC, id ("" status = all);
	// ListScheduleTickets the same for one schedule.
	CreateClientTicket(ctx context.Context, p CreateClientTicketParams) (ClientTicket, error)
	GetClientTicket(ctx context.Context, id string) (ClientTicket, error)
	GetClientTicketByEnquiry(ctx context.Context, enquiryID string) (ClientTicket, error)
	ListClientTickets(ctx context.Context, clientID, status string, limit, offset int) ([]ClientTicket, error)
	ListScheduleTickets(ctx context.Context, scheduleID string) ([]ClientTicket, error)
	UpdateClientTicketStatus(ctx context.Context, id, status string, reservedBranchID *string) (ClientTicket, error)

	// Medicine schedules. ListClientSchedules orders by created_at DESC, id.
	// ListDueSchedules: active with next_run_at <= now(), FOR UPDATE SKIP LOCKED (use inside InTx).
	CreateMedicineSchedule(ctx context.Context, p CreateMedicineScheduleParams) (MedicineSchedule, error)
	GetMedicineSchedule(ctx context.Context, id string) (MedicineSchedule, error)
	GetMedicineScheduleForUpdate(ctx context.Context, id string) (MedicineSchedule, error)
	ListClientSchedules(ctx context.Context, clientID string, limit, offset int) ([]MedicineSchedule, error)
	UpdateMedicineSchedule(ctx context.Context, id, status string, nextRunAt time.Time) (MedicineSchedule, error)
	ListDueSchedules(ctx context.Context, limit int) ([]MedicineSchedule, error)

	// Package photos. ListEnquiryPhotos orders by created_at, id. ReviewPackagePhoto
	// sets decided_by = 'human'.
	CreatePackagePhoto(ctx context.Context, p CreatePackagePhotoParams) (PackagePhoto, error)
	GetPackagePhoto(ctx context.Context, id string) (PackagePhoto, error)
	ListEnquiryPhotos(ctx context.Context, enquiryID string) ([]PackagePhoto, error)
	ReviewPackagePhoto(ctx context.Context, p ReviewPackagePhotoParams) (PackagePhoto, error)
	// RecordPhotoAgentReading: ErrNotFound when the photo is missing or already human-reviewed.
	RecordPhotoAgentReading(ctx context.Context, p RecordPhotoAgentReadingParams) (PackagePhoto, error)

	// GetGovProduct returns one government reference row by code (ErrNotFound if none).
	GetGovProduct(ctx context.Context, code string) (GovProduct, error)
}
