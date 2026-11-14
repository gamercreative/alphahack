package queries

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// Wassit enquiries: staff and sessions, enquiries, targets, inbox, candidate
// branches, outbox, events, agent runs and tool calls, stats. Row types and
// params live in enquiry_models.go. Statements live in sql/<method_snake_case>.sql.

var (
	qGetStaffByEmail        = mustSQL("get_staff_by_email")
	qGetStaff               = mustSQL("get_staff")
	qCreateStaffSession     = mustSQL("create_staff_session")
	qGetStaffBySessionToken = mustSQL("get_staff_by_session_token")

	qCreateEnquiry         = mustSQL("create_enquiry")
	qGetEnquiry            = mustSQL("get_enquiry")
	qGetEnquiryForUpdate   = mustSQL("get_enquiry_for_update")
	qListOutgoingEnquiries = mustSQL("list_outgoing_enquiries")
	qListIncomingEnquiries = mustSQL("list_incoming_enquiries")
	qUpdateEnquiry         = mustSQL("update_enquiry")
	qListAgentWakeups      = mustSQL("list_agent_wakeups")
	qClearEnquiryWake      = mustSQL("clear_enquiry_wake")

	qCreateEnquiryTarget = mustSQL("create_enquiry_target")
	qGetEnquiryTarget    = mustSQL("get_enquiry_target")
	qListEnquiryTargets  = mustSQL("list_enquiry_targets")
	qUpdateEnquiryTarget = mustSQL("update_enquiry_target")
	qDeleteEnquiryTarget = mustSQL("delete_enquiry_target")

	qListInbox             = mustSQL("list_inbox")
	qListCandidateBranches = mustSQL("list_candidate_branches")

	qCreateOutboxMessage       = mustSQL("create_outbox_message")
	qGetOutboxMessage          = mustSQL("get_outbox_message")
	qListOutboxMessages        = mustSQL("list_outbox_messages")
	qUpdateOutboxMessageStatus = mustSQL("update_outbox_message_status")
	qListDueOutboxMessages     = mustSQL("list_due_outbox_messages")

	qCreateEnquiryEvent     = mustSQL("create_enquiry_event")
	qGetEnquiryEvent        = mustSQL("get_enquiry_event")
	qListEnquiryEvents      = mustSQL("list_enquiry_events")
	qMarkEnquiryEventUndone = mustSQL("mark_enquiry_event_undone")

	qCreateEnquiryAgentRun      = mustSQL("create_enquiry_agent_run")
	qGetEnquiryAgentRun         = mustSQL("get_enquiry_agent_run")
	qFinishEnquiryAgentRun      = mustSQL("finish_enquiry_agent_run")
	qListEnquiryAgentRuns       = mustSQL("list_enquiry_agent_runs")
	qCreateEnquiryAgentToolCall = mustSQL("create_enquiry_agent_tool_call")

	qListEnquiryStats = mustSQL("list_enquiry_stats")
)

// ---- parameter helpers -------------------------------------------------------

// jsonObjectArg returns raw for a jsonb parameter, defaulting to {}.
func jsonObjectArg(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return []byte(raw)
}

// nullableJSONArg returns raw for a nullable jsonb parameter; nil becomes SQL NULL.
func nullableJSONArg(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

// scanOne runs the scanner on a single row, mapping pgx.ErrNoRows to ErrNotFound.
func scanOne[T any](row pgx.Row, op string, scan func(rowScanner) (T, error)) (T, error) {
	v, err := scan(row)
	if err != nil {
		var zero T
		return zero, wrap(op, err)
	}
	return v, nil
}

// ---- scanners (column order matches the SQL files) ---------------------------

func scanStaff(r rowScanner) (Staff, error) {
	var s Staff
	err := r.Scan(&s.ID, &s.BranchID, &s.FullName, &s.Email, &s.PinHash, &s.Role, &s.CreatedAt)
	return s, err
}

func scanEnquiry(r rowScanner) (Enquiry, error) {
	var e Enquiry
	err := r.Scan(
		&e.ID, &e.RequesterBranchID, &e.RequesterStaffID, &e.RawRequest, &e.MedicationID,
		&e.Quantity, &e.Status, &e.Outcome, &e.OutcomeBranchID, &e.HandledBy,
		&e.ClarificationQuestion, &e.ClarificationAnswer, &e.WakeTrigger, &e.NextWakeAt,
		&e.CreatedAt, &e.UpdatedAt, &e.ClosedAt,
	)
	return e, err
}

func scanEnquiryTarget(r rowScanner) (EnquiryTarget, error) {
	var t EnquiryTarget
	err := r.Scan(
		&t.ID, &t.EnquiryID, &t.BranchID, &t.Status, &t.ReplyQuantity, &t.ReplyText,
		&t.RepliedByStaffID, &t.RepliedAt, &t.FollowUps, &t.CreatedAt,
	)
	return t, err
}

func scanOutboxMessage(r rowScanner) (OutboxMessage, error) {
	var m OutboxMessage
	err := r.Scan(
		&m.ID, &m.EnquiryID, &m.TargetID, &m.Kind, &m.Body, &m.Status,
		&m.SendAfter, &m.SentAt, &m.CreatedByActor, &m.CreatedAt,
	)
	return m, err
}

func scanEnquiryEvent(r rowScanner) (EnquiryEvent, error) {
	var e EnquiryEvent
	var payload, inverse []byte
	err := r.Scan(
		&e.ID, &e.Seq, &e.EnquiryID, &e.ActorType, &e.ActorID, &e.Type,
		&payload, &inverse, &e.UndoneAt, &e.UndoneByEventID, &e.CreatedAt,
	)
	e.Payload = json.RawMessage(payload)
	if inverse != nil {
		e.Inverse = json.RawMessage(inverse)
	}
	return e, err
}

func scanEnquiryAgentRun(r rowScanner) (EnquiryAgentRun, error) {
	var run EnquiryAgentRun
	err := r.Scan(
		&run.ID, &run.EnquiryID, &run.Trigger, &run.Model, &run.InputTokens, &run.OutputTokens,
		&run.Steps, &run.CostMicros, &run.Status, &run.Error, &run.StartedAt, &run.FinishedAt,
	)
	return run, err
}

func scanEnquiryAgentToolCall(r rowScanner) (EnquiryAgentToolCall, error) {
	var c EnquiryAgentToolCall
	var input []byte
	err := r.Scan(
		&c.ID, &c.Seq, &c.RunID, &c.Name, &input, &c.Output, &c.IsError, &c.DurationMs, &c.CreatedAt,
	)
	c.Input = json.RawMessage(input)
	return c, err
}

func scanAgentWakeup(r rowScanner) (AgentWakeup, error) {
	var w AgentWakeup
	err := r.Scan(&w.EnquiryID, &w.Trigger)
	return w, err
}

func scanInboxItem(r rowScanner) (InboxItem, error) {
	var it InboxItem
	err := r.Scan(
		&it.TargetID, &it.TargetStatus, &it.FollowUps, &it.EnquiryID, &it.RequesterBranchID,
		&it.RequesterBranchName, &it.MedicationID, &it.MedicationName, &it.RawRequest,
		&it.Quantity, &it.SentAt,
	)
	return it, err
}

func scanCandidateBranch(r rowScanner) (CandidateBranch, error) {
	var c CandidateBranch
	err := r.Scan(&c.BranchID, &c.Name, &c.City, &c.Lat, &c.Lng, &c.AvailableQuantity, &c.StockUpdatedAt)
	return c, err
}

func scanEnquiryStat(r rowScanner) (EnquiryStat, error) {
	var s EnquiryStat
	err := r.Scan(
		&s.EnquiryID, &s.RawRequest, &s.HandledBy, &s.Status, &s.Outcome, &s.HITLCount,
		&s.AgentRuns, &s.AgentSteps, &s.InputTokens, &s.OutputTokens, &s.CostMicros, &s.CreatedAt,
	)
	return s, err
}

// ---- staff and sessions ------------------------------------------------------

func (q *Queries) GetStaffByEmail(ctx context.Context, email string) (Staff, error) {
	return scanOne(q.db.QueryRow(ctx, qGetStaffByEmail, email), "get staff by email", scanStaff)
}

func (q *Queries) GetStaff(ctx context.Context, id string) (Staff, error) {
	return scanOne(q.db.QueryRow(ctx, qGetStaff, id), "get staff", scanStaff)
}

func (q *Queries) CreateStaffSession(ctx context.Context, p CreateStaffSessionParams) error {
	_, err := q.db.Exec(ctx, qCreateStaffSession, p.TokenHash, p.StaffID, p.ExpiresAt)
	return wrap("create staff session", err)
}

func (q *Queries) GetStaffBySessionToken(ctx context.Context, tokenHash string) (Staff, error) {
	return scanOne(q.db.QueryRow(ctx, qGetStaffBySessionToken, tokenHash), "get staff by session token", scanStaff)
}

// ---- enquiries ---------------------------------------------------------------

func (q *Queries) CreateEnquiry(ctx context.Context, p CreateEnquiryParams) (Enquiry, error) {
	return scanOne(q.db.QueryRow(ctx, qCreateEnquiry,
		p.RequesterBranchID, p.RequesterStaffID, p.RawRequest, p.MedicationID,
		p.Quantity, p.Status, p.HandledBy, p.WakeTrigger,
	), "create enquiry", scanEnquiry)
}

func (q *Queries) GetEnquiry(ctx context.Context, id string) (Enquiry, error) {
	return scanOne(q.db.QueryRow(ctx, qGetEnquiry, id), "get enquiry", scanEnquiry)
}

func (q *Queries) GetEnquiryForUpdate(ctx context.Context, id string) (Enquiry, error) {
	return scanOne(q.db.QueryRow(ctx, qGetEnquiryForUpdate, id), "get enquiry for update", scanEnquiry)
}

func (q *Queries) ListOutgoingEnquiries(ctx context.Context, branchID, status string, limit, offset int) ([]Enquiry, error) {
	rows, err := q.db.Query(ctx, qListOutgoingEnquiries, branchID, status, limit, offset)
	if err != nil {
		return nil, wrap("list outgoing enquiries", err)
	}
	return collect(rows, "list outgoing enquiries", scanEnquiry)
}

func (q *Queries) ListIncomingEnquiries(ctx context.Context, branchID, status string, limit, offset int) ([]Enquiry, error) {
	rows, err := q.db.Query(ctx, qListIncomingEnquiries, branchID, status, limit, offset)
	if err != nil {
		return nil, wrap("list incoming enquiries", err)
	}
	return collect(rows, "list incoming enquiries", scanEnquiry)
}

func (q *Queries) UpdateEnquiry(ctx context.Context, p UpdateEnquiryParams) (Enquiry, error) {
	return scanOne(q.db.QueryRow(ctx, qUpdateEnquiry,
		p.ID, p.MedicationID, p.Status, p.Outcome, p.OutcomeBranchID,
		p.ClarificationQuestion, p.ClarificationAnswer, p.WakeTrigger,
		p.NextWakeAt, p.ClosedAt,
	), "update enquiry", scanEnquiry)
}

func (q *Queries) ListAgentWakeups(ctx context.Context, limit int) ([]AgentWakeup, error) {
	rows, err := q.db.Query(ctx, qListAgentWakeups, limit)
	if err != nil {
		return nil, wrap("list agent wakeups", err)
	}
	return collect(rows, "list agent wakeups", scanAgentWakeup)
}

func (q *Queries) ClearEnquiryWake(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx, qClearEnquiryWake, id)
	return affected("clear enquiry wake", tag, err)
}

// ---- targets -----------------------------------------------------------------

func (q *Queries) CreateEnquiryTarget(ctx context.Context, p CreateEnquiryTargetParams) (EnquiryTarget, error) {
	return scanOne(q.db.QueryRow(ctx, qCreateEnquiryTarget, p.EnquiryID, p.BranchID, p.Status),
		"create enquiry target", scanEnquiryTarget)
}

func (q *Queries) GetEnquiryTarget(ctx context.Context, id string) (EnquiryTarget, error) {
	return scanOne(q.db.QueryRow(ctx, qGetEnquiryTarget, id), "get enquiry target", scanEnquiryTarget)
}

func (q *Queries) ListEnquiryTargets(ctx context.Context, enquiryID string) ([]EnquiryTarget, error) {
	rows, err := q.db.Query(ctx, qListEnquiryTargets, enquiryID)
	if err != nil {
		return nil, wrap("list enquiry targets", err)
	}
	return collect(rows, "list enquiry targets", scanEnquiryTarget)
}

func (q *Queries) UpdateEnquiryTarget(ctx context.Context, p UpdateEnquiryTargetParams) (EnquiryTarget, error) {
	return scanOne(q.db.QueryRow(ctx, qUpdateEnquiryTarget,
		p.ID, p.Status, p.ReplyQuantity, p.ReplyText, p.RepliedByStaffID, p.RepliedAt, p.FollowUps,
	), "update enquiry target", scanEnquiryTarget)
}

func (q *Queries) DeleteEnquiryTarget(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx, qDeleteEnquiryTarget, id)
	return affected("delete enquiry target", tag, err)
}

// ---- inbox and candidates ----------------------------------------------------

func (q *Queries) ListInbox(ctx context.Context, branchID string, limit, offset int) ([]InboxItem, error) {
	rows, err := q.db.Query(ctx, qListInbox, branchID, limit, offset)
	if err != nil {
		return nil, wrap("list inbox", err)
	}
	return collect(rows, "list inbox", scanInboxItem)
}

func (q *Queries) ListCandidateBranches(ctx context.Context, medicationID, excludeBranchID string) ([]CandidateBranch, error) {
	rows, err := q.db.Query(ctx, qListCandidateBranches, medicationID, excludeBranchID)
	if err != nil {
		return nil, wrap("list candidate branches", err)
	}
	return collect(rows, "list candidate branches", scanCandidateBranch)
}

// ---- outbox ------------------------------------------------------------------

func (q *Queries) CreateOutboxMessage(ctx context.Context, p CreateOutboxMessageParams) (OutboxMessage, error) {
	return scanOne(q.db.QueryRow(ctx, qCreateOutboxMessage,
		p.EnquiryID, p.TargetID, p.Kind, p.Body, p.SendAfter, p.CreatedByActor,
	), "create outbox message", scanOutboxMessage)
}

func (q *Queries) GetOutboxMessage(ctx context.Context, id string) (OutboxMessage, error) {
	return scanOne(q.db.QueryRow(ctx, qGetOutboxMessage, id), "get outbox message", scanOutboxMessage)
}

func (q *Queries) ListOutboxMessages(ctx context.Context, enquiryID string) ([]OutboxMessage, error) {
	rows, err := q.db.Query(ctx, qListOutboxMessages, enquiryID)
	if err != nil {
		return nil, wrap("list outbox messages", err)
	}
	return collect(rows, "list outbox messages", scanOutboxMessage)
}

func (q *Queries) UpdateOutboxMessageStatus(ctx context.Context, id, status string, sentAt *time.Time) (OutboxMessage, error) {
	return scanOne(q.db.QueryRow(ctx, qUpdateOutboxMessageStatus, id, status, sentAt),
		"update outbox message status", scanOutboxMessage)
}

func (q *Queries) ListDueOutboxMessages(ctx context.Context, limit int) ([]OutboxMessage, error) {
	rows, err := q.db.Query(ctx, qListDueOutboxMessages, limit)
	if err != nil {
		return nil, wrap("list due outbox messages", err)
	}
	return collect(rows, "list due outbox messages", scanOutboxMessage)
}

// ---- events ------------------------------------------------------------------

func (q *Queries) CreateEnquiryEvent(ctx context.Context, p CreateEnquiryEventParams) (EnquiryEvent, error) {
	return scanOne(q.db.QueryRow(ctx, qCreateEnquiryEvent,
		p.EnquiryID, p.ActorType, p.ActorID, p.Type,
		jsonObjectArg(p.Payload), nullableJSONArg(p.Inverse),
	), "create enquiry event", scanEnquiryEvent)
}

func (q *Queries) GetEnquiryEvent(ctx context.Context, id string) (EnquiryEvent, error) {
	return scanOne(q.db.QueryRow(ctx, qGetEnquiryEvent, id), "get enquiry event", scanEnquiryEvent)
}

func (q *Queries) ListEnquiryEvents(ctx context.Context, enquiryID string, afterSeq int64) ([]EnquiryEvent, error) {
	rows, err := q.db.Query(ctx, qListEnquiryEvents, enquiryID, afterSeq)
	if err != nil {
		return nil, wrap("list enquiry events", err)
	}
	return collect(rows, "list enquiry events", scanEnquiryEvent)
}

func (q *Queries) MarkEnquiryEventUndone(ctx context.Context, id, undoneByEventID string) (EnquiryEvent, error) {
	return scanOne(q.db.QueryRow(ctx, qMarkEnquiryEventUndone, id, undoneByEventID),
		"mark enquiry event undone", scanEnquiryEvent)
}

// ---- agent runs and tool calls -----------------------------------------------

func (q *Queries) CreateEnquiryAgentRun(ctx context.Context, p CreateEnquiryAgentRunParams) (EnquiryAgentRun, error) {
	return scanOne(q.db.QueryRow(ctx, qCreateEnquiryAgentRun, p.EnquiryID, p.Trigger, p.Model),
		"create enquiry agent run", scanEnquiryAgentRun)
}

func (q *Queries) GetEnquiryAgentRun(ctx context.Context, id string) (EnquiryAgentRun, error) {
	return scanOne(q.db.QueryRow(ctx, qGetEnquiryAgentRun, id), "get enquiry agent run", scanEnquiryAgentRun)
}

func (q *Queries) FinishEnquiryAgentRun(ctx context.Context, p FinishEnquiryAgentRunParams) (EnquiryAgentRun, error) {
	return scanOne(q.db.QueryRow(ctx, qFinishEnquiryAgentRun,
		p.ID, p.Status, p.InputTokens, p.OutputTokens, p.Steps, p.CostMicros, p.Error, p.FinishedAt,
	), "finish enquiry agent run", scanEnquiryAgentRun)
}

func (q *Queries) ListEnquiryAgentRuns(ctx context.Context, enquiryID string) ([]EnquiryAgentRun, error) {
	rows, err := q.db.Query(ctx, qListEnquiryAgentRuns, enquiryID)
	if err != nil {
		return nil, wrap("list enquiry agent runs", err)
	}
	return collect(rows, "list enquiry agent runs", scanEnquiryAgentRun)
}

func (q *Queries) CreateEnquiryAgentToolCall(ctx context.Context, p CreateEnquiryAgentToolCallParams) (EnquiryAgentToolCall, error) {
	return scanOne(q.db.QueryRow(ctx, qCreateEnquiryAgentToolCall,
		p.RunID, p.Name, jsonObjectArg(p.Input), p.Output, p.IsError, p.DurationMs,
	), "create enquiry agent tool call", scanEnquiryAgentToolCall)
}

// ---- stats -------------------------------------------------------------------

func (q *Queries) ListEnquiryStats(ctx context.Context, limit, offset int) ([]EnquiryStat, error) {
	rows, err := q.db.Query(ctx, qListEnquiryStats, limit, offset)
	if err != nil {
		return nil, wrap("list enquiry stats", err)
	}
	return collect(rows, "list enquiry stats", scanEnquiryStat)
}
