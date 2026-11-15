package queries

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// SQL-level integration tests for the enquiry schema (0004_enquiries) and the
// enquiry queries that read it. Each test creates its own rows and deletes its
// enquiry at cleanup (child rows cascade). Seed ids come from 0003/0005.

// newEnquiry creates an open enquiry from branch-hamra (staff-hamra) that is deleted at cleanup.
func newEnquiry(t *testing.T, q *Queries, handledBy, wakeTrigger string) Enquiry {
	t.Helper()
	e, err := q.CreateEnquiry(context.Background(), CreateEnquiryParams{
		RequesterBranchID: "branch-hamra",
		RequesterStaffID:  "staff-hamra",
		RawRequest:        uniq("req"),
		Quantity:          2,
		Status:            EnquiryOpen,
		HandledBy:         handledBy,
		WakeTrigger:       wakeTrigger,
	})
	if err != nil {
		t.Fatalf("CreateEnquiry: %v", err)
	}
	t.Cleanup(func() {
		_, _ = q.db.Exec(context.Background(), "DELETE FROM enquiries WHERE id = $1", e.ID)
	})
	return e
}

// enqPgError asserts err is a PostgreSQL error with the exact SQLSTATE code and,
// when constraint is non-empty, the exact constraint name.
func enqPgError(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("err = %v, want *pgconn.PgError with code %s", err, code)
	}
	if pgErr.Code != code {
		t.Fatalf("SQLSTATE = %q, want %q (message: %s)", pgErr.Code, code, pgErr.Message)
	}
	if constraint != "" && pgErr.ConstraintName != constraint {
		t.Fatalf("constraint = %q, want %q (message: %s)", pgErr.ConstraintName, constraint, pgErr.Message)
	}
}

func enqTarget(t *testing.T, q *Queries, enquiryID, branchID, status string) EnquiryTarget {
	t.Helper()
	tg, err := q.CreateEnquiryTarget(context.Background(), CreateEnquiryTargetParams{
		EnquiryID: enquiryID,
		BranchID:  branchID,
		Status:    status,
	})
	if err != nil {
		t.Fatalf("CreateEnquiryTarget(%s): %v", branchID, err)
	}
	return tg
}

func enqOutbox(t *testing.T, q *Queries, enquiryID string, targetID *string, kind string, sendAfter time.Time) OutboxMessage {
	t.Helper()
	m, err := q.CreateOutboxMessage(context.Background(), CreateOutboxMessageParams{
		EnquiryID:      enquiryID,
		TargetID:       targetID,
		Kind:           kind,
		Body:           "body",
		SendAfter:      sendAfter,
		CreatedByActor: "system",
	})
	if err != nil {
		t.Fatalf("CreateOutboxMessage(%s): %v", kind, err)
	}
	return m
}

// enqWakeupTrigger returns the trigger ListAgentWakeups reports for enquiryID, and whether it is listed.
func enqWakeupTrigger(t *testing.T, q *Queries, enquiryID string) (string, bool) {
	t.Helper()
	list, err := q.ListAgentWakeups(context.Background(), 1000)
	if err != nil {
		t.Fatalf("ListAgentWakeups: %v", err)
	}
	for _, w := range list {
		if w.EnquiryID == enquiryID {
			return w.Trigger, true
		}
	}
	return "", false
}

func enqDueContains(t *testing.T, q *Queries, messageID string) bool {
	t.Helper()
	list, err := q.ListDueOutboxMessages(context.Background(), 1000)
	if err != nil {
		t.Fatalf("ListDueOutboxMessages: %v", err)
	}
	for _, m := range list {
		if m.ID == messageID {
			return true
		}
	}
	return false
}

func enqInboxContains(t *testing.T, q *Queries, branchID, targetID string) bool {
	t.Helper()
	items, err := q.ListInbox(context.Background(), branchID, 1000, 0)
	if err != nil {
		t.Fatalf("ListInbox(%s): %v", branchID, err)
	}
	for _, it := range items {
		if it.TargetID == targetID {
			return true
		}
	}
	return false
}

func enqStatFor(t *testing.T, q *Queries, enquiryID string) EnquiryStat {
	t.Helper()
	stats, err := q.ListEnquiryStats(context.Background(), 1000, 0)
	if err != nil {
		t.Fatalf("ListEnquiryStats: %v", err)
	}
	for _, s := range stats {
		if s.EnquiryID == enquiryID {
			return s
		}
	}
	t.Fatalf("ListEnquiryStats: enquiry %s not in the first 1000 rows", enquiryID)
	return EnquiryStat{}
}

// ---- 1. CHECK constraints ----------------------------------------------------

func TestIntegrationEnquiryCheckConstraints(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()
	parent := newEnquiry(t, q, "manual", "")

	const enquiryInsert = `INSERT INTO enquiries (requester_branch_id, requester_staff_id, raw_request, quantity, status, outcome, handled_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)`

	type check struct {
		name       string
		query      string
		args       []any
		constraint string
	}
	cases := []check{
		{"enquiry_quantity_zero", enquiryInsert,
			[]any{"branch-hamra", "staff-hamra", uniq("req"), 0, "open", "", "manual"}, "enquiries_quantity_check"},
		{"enquiry_status_bogus", enquiryInsert,
			[]any{"branch-hamra", "staff-hamra", uniq("req"), 2, "bogus", "", "manual"}, "enquiries_status_check"},
		{"enquiry_handled_by_robot", enquiryInsert,
			[]any{"branch-hamra", "staff-hamra", uniq("req"), 2, "open", "", "robot"}, "enquiries_handled_by_check"},
		{"enquiry_outcome_maybe", enquiryInsert,
			[]any{"branch-hamra", "staff-hamra", uniq("req"), 2, "open", "maybe", "manual"}, "enquiries_outcome_check"},
		{"target_status_bogus", `INSERT INTO enquiry_targets (enquiry_id, branch_id, status) VALUES ($1, $2, $3)`,
			[]any{parent.ID, "branch-achrafieh", "bogus"}, "enquiry_targets_status_check"},
		{"outbox_kind_sms", `INSERT INTO outbox_messages (enquiry_id, kind, status, send_after) VALUES ($1, $2, $3, $4)`,
			[]any{parent.ID, "sms", "queued", time.Now()}, "outbox_messages_kind_check"},
		{"outbox_status_failed", `INSERT INTO outbox_messages (enquiry_id, kind, status, send_after) VALUES ($1, $2, $3, $4)`,
			[]any{parent.ID, "enquiry", "failed", time.Now()}, "outbox_messages_status_check"},
		{"event_actor_robot", `INSERT INTO enquiry_events (enquiry_id, actor_type, actor_id, type) VALUES ($1, $2, $3, $4)`,
			[]any{parent.ID, "robot", "", "clarified"}, "enquiry_events_actor_type_check"},
		{"agent_run_status_bogus", `INSERT INTO enquiry_agent_runs (enquiry_id, trigger, model, status) VALUES ($1, $2, $3, $4)`,
			[]any{parent.ID, "created", "m-1", "bogus"}, "enquiry_agent_runs_status_check"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// If the constraint were missing the enquiry insert would succeed; remove any such row.
			if c.query == enquiryInsert {
				rawRequest := c.args[2].(string)
				t.Cleanup(func() {
					_, _ = q.db.Exec(context.Background(), "DELETE FROM enquiries WHERE raw_request = $1", rawRequest)
				})
			}
			_, err := q.db.Exec(ctx, c.query, c.args...)
			enqPgError(t, err, "23514", c.constraint)
		})
	}
}

// ---- 2. one target per enquiry and branch ------------------------------------

func TestIntegrationTargetUniquePerBranch(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()
	e := newEnquiry(t, q, "manual", "")

	if _, err := q.CreateEnquiryTarget(ctx, CreateEnquiryTargetParams{
		EnquiryID: e.ID, BranchID: "branch-achrafieh", Status: TargetProposed,
	}); err != nil {
		t.Fatalf("first CreateEnquiryTarget: %v", err)
	}

	_, err := q.CreateEnquiryTarget(ctx, CreateEnquiryTargetParams{
		EnquiryID: e.ID, BranchID: "branch-achrafieh", Status: TargetProposed,
	})
	enqPgError(t, err, "23505", "enquiry_targets_enquiry_id_branch_id_key")
}

// ---- 3. agent wakeups --------------------------------------------------------

func TestIntegrationAgentWakeups(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()

	t.Run("agent_created_wake_is_listed_with_its_trigger", func(t *testing.T) {
		e := newEnquiry(t, q, "agent", "created")
		trig, ok := enqWakeupTrigger(t, q, e.ID)
		if !ok {
			t.Fatalf("agent enquiry with wake_trigger 'created' is not listed by ListAgentWakeups")
		}
		if trig != "created" {
			t.Fatalf("trigger = %q, want %q", trig, "created")
		}
	})

	t.Run("manual_enquiry_with_wake_is_not_listed", func(t *testing.T) {
		e := newEnquiry(t, q, "manual", "created")
		if trig, ok := enqWakeupTrigger(t, q, e.ID); ok {
			t.Fatalf("manual enquiry listed with trigger %q, want absent", trig)
		}
	})

	t.Run("agent_enquiry_without_wake_is_not_listed", func(t *testing.T) {
		e := newEnquiry(t, q, "agent", "")
		if trig, ok := enqWakeupTrigger(t, q, e.ID); ok {
			t.Fatalf("agent enquiry with no wake listed with trigger %q, want absent", trig)
		}
	})

	t.Run("agent_enquiry_past_next_wake_is_listed_as_timer", func(t *testing.T) {
		e := newEnquiry(t, q, "agent", "")
		if _, err := q.db.Exec(ctx, "UPDATE enquiries SET next_wake_at = now() - interval '1 second' WHERE id = $1", e.ID); err != nil {
			t.Fatalf("set next_wake_at in the past: %v", err)
		}
		trig, ok := enqWakeupTrigger(t, q, e.ID)
		if !ok {
			t.Fatalf("agent enquiry with past next_wake_at is not listed by ListAgentWakeups")
		}
		if trig != "timer" {
			t.Fatalf("trigger = %q, want %q", trig, "timer")
		}
	})

	t.Run("agent_enquiry_future_next_wake_is_not_listed", func(t *testing.T) {
		e := newEnquiry(t, q, "agent", "")
		if _, err := q.db.Exec(ctx, "UPDATE enquiries SET next_wake_at = now() + interval '1 hour' WHERE id = $1", e.ID); err != nil {
			t.Fatalf("set next_wake_at in the future: %v", err)
		}
		if trig, ok := enqWakeupTrigger(t, q, e.ID); ok {
			t.Fatalf("agent enquiry with future next_wake_at listed with trigger %q, want absent", trig)
		}
	})

	t.Run("closed_agent_enquiry_with_reply_wake_is_not_listed", func(t *testing.T) {
		e := newEnquiry(t, q, "agent", "reply")
		if trig, ok := enqWakeupTrigger(t, q, e.ID); !ok || trig != "reply" {
			t.Fatalf("precondition: open agent enquiry with wake 'reply' listed = %v trigger %q, want listed as 'reply'", ok, trig)
		}
		if _, err := q.db.Exec(ctx, "UPDATE enquiries SET status = $2 WHERE id = $1", e.ID, EnquiryClosed); err != nil {
			t.Fatalf("close enquiry: %v", err)
		}
		if trig, ok := enqWakeupTrigger(t, q, e.ID); ok {
			t.Fatalf("closed agent enquiry listed with trigger %q, want absent", trig)
		}
	})
}

// ---- 4. due outbox messages --------------------------------------------------

func TestIntegrationDueOutbox(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()
	e := newEnquiry(t, q, "manual", "")

	t.Run("queued_past_send_after_is_due", func(t *testing.T) {
		m := enqOutbox(t, q, e.ID, nil, OutboxKindNotification, time.Now().Add(-time.Minute))
		if !enqDueContains(t, q, m.ID) {
			t.Fatalf("queued message with send_after 1 minute ago is not in ListDueOutboxMessages")
		}
	})

	t.Run("queued_future_send_after_is_not_due", func(t *testing.T) {
		m := enqOutbox(t, q, e.ID, nil, OutboxKindNotification, time.Now().Add(time.Hour))
		if enqDueContains(t, q, m.ID) {
			t.Fatalf("queued message with send_after 1 hour ahead is in ListDueOutboxMessages, want absent")
		}
	})

	t.Run("sent_message_is_no_longer_due", func(t *testing.T) {
		m := enqOutbox(t, q, e.ID, nil, OutboxKindNotification, time.Now().Add(-time.Minute))
		if !enqDueContains(t, q, m.ID) {
			t.Fatalf("precondition: queued message with send_after 1 minute ago is not due")
		}
		now := time.Now()
		if _, err := q.UpdateOutboxMessageStatus(ctx, m.ID, OutboxSent, &now); err != nil {
			t.Fatalf("UpdateOutboxMessageStatus(sent): %v", err)
		}
		if enqDueContains(t, q, m.ID) {
			t.Fatalf("message marked sent is still in ListDueOutboxMessages, want absent")
		}
	})

	t.Run("cancelled_message_is_not_due", func(t *testing.T) {
		m := enqOutbox(t, q, e.ID, nil, OutboxKindNotification, time.Now().Add(-time.Minute))
		if !enqDueContains(t, q, m.ID) {
			t.Fatalf("precondition: queued message with send_after 1 minute ago is not due")
		}
		if _, err := q.UpdateOutboxMessageStatus(ctx, m.ID, OutboxCancelled, nil); err != nil {
			t.Fatalf("UpdateOutboxMessageStatus(cancelled): %v", err)
		}
		if enqDueContains(t, q, m.ID) {
			t.Fatalf("message marked cancelled is still in ListDueOutboxMessages, want absent")
		}
	})
}

// ---- 5. stats ----------------------------------------------------------------

func TestIntegrationStatsRow(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()

	t.Run("row_aggregates_runs_steps_tokens_and_cost", func(t *testing.T) {
		e := newEnquiry(t, q, "agent", "")
		now := time.Now()
		runs := []struct {
			input, output, steps int
			cost                 int64
		}{
			{100, 10, 2, 100},
			{200, 20, 3, 250},
		}
		for _, r := range runs {
			run, err := q.CreateEnquiryAgentRun(ctx, CreateEnquiryAgentRunParams{
				EnquiryID: e.ID, Trigger: "created", Model: "m-1",
			})
			if err != nil {
				t.Fatalf("CreateEnquiryAgentRun: %v", err)
			}
			if _, err := q.FinishEnquiryAgentRun(ctx, FinishEnquiryAgentRunParams{
				ID: run.ID, Status: "completed", InputTokens: r.input, OutputTokens: r.output,
				Steps: r.steps, CostMicros: r.cost, FinishedAt: &now,
			}); err != nil {
				t.Fatalf("FinishEnquiryAgentRun: %v", err)
			}
		}

		s := enqStatFor(t, q, e.ID)
		if s.AgentRuns != 2 {
			t.Errorf("AgentRuns = %d, want 2", s.AgentRuns)
		}
		if s.AgentSteps != 5 {
			t.Errorf("AgentSteps = %d, want 5", s.AgentSteps)
		}
		if s.InputTokens != 300 {
			t.Errorf("InputTokens = %d, want 300", s.InputTokens)
		}
		if s.OutputTokens != 30 {
			t.Errorf("OutputTokens = %d, want 30", s.OutputTokens)
		}
		if s.CostMicros != 350 {
			t.Errorf("CostMicros = %d, want 350", s.CostMicros)
		}
	})

	// hitlEvents creates an enquiry with one clarified, one targets_approved and one reply_received event.
	hitlEvents := func(t *testing.T) (clarified, reply EnquiryEvent, enquiryID string) {
		t.Helper()
		e := newEnquiry(t, q, "agent", "")
		mk := func(typ string) EnquiryEvent {
			ev, err := q.CreateEnquiryEvent(ctx, CreateEnquiryEventParams{
				EnquiryID: e.ID, ActorType: ActorStaff, ActorID: "staff-hamra", Type: typ,
			})
			if err != nil {
				t.Fatalf("CreateEnquiryEvent(%s): %v", typ, err)
			}
			return ev
		}
		clarified = mk("clarified")
		mk("targets_approved")
		reply = mk("reply_received")
		return clarified, reply, e.ID
	}

	t.Run("hitl_count_is_clarified_plus_approved_only", func(t *testing.T) {
		_, _, enquiryID := hitlEvents(t)
		if got := enqStatFor(t, q, enquiryID).HITLCount; got != 2 {
			t.Fatalf("HITLCount = %d, want 2 (reply_received must not count)", got)
		}
	})

	t.Run("undone_clarified_event_drops_hitl_count_to_one", func(t *testing.T) {
		clarified, reply, enquiryID := hitlEvents(t)
		// undone_by_event_id is plain TEXT (no foreign key), so the reply event id is a valid marker.
		if _, err := q.MarkEnquiryEventUndone(ctx, clarified.ID, reply.ID); err != nil {
			t.Fatalf("MarkEnquiryEventUndone: %v", err)
		}
		if got := enqStatFor(t, q, enquiryID).HITLCount; got != 1 {
			t.Fatalf("HITLCount after undo = %d, want 1", got)
		}
	})
}

// ---- 6. inbox ----------------------------------------------------------------

func TestIntegrationInbox(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()

	// sentTarget creates an enquiry targeting branch-achrafieh with one enquiry message.
	// The message is left queued unless markSent is true.
	sentTarget := func(t *testing.T, markSent bool) (EnquiryTarget, OutboxMessage) {
		t.Helper()
		e := newEnquiry(t, q, "manual", "")
		tg := enqTarget(t, q, e.ID, "branch-achrafieh", TargetSent)
		m := enqOutbox(t, q, e.ID, &tg.ID, OutboxKindEnquiry, time.Now())
		if markSent {
			now := time.Now()
			if _, err := q.UpdateOutboxMessageStatus(ctx, m.ID, OutboxSent, &now); err != nil {
				t.Fatalf("UpdateOutboxMessageStatus(sent): %v", err)
			}
		}
		return tg, m
	}

	t.Run("sent_enquiry_message_is_listed_in_achrafieh_inbox", func(t *testing.T) {
		tg, _ := sentTarget(t, true)
		if !enqInboxContains(t, q, "branch-achrafieh", tg.ID) {
			t.Fatalf("target with sent enquiry message is not in ListInbox(branch-achrafieh)")
		}
	})

	t.Run("queued_enquiry_message_is_not_in_inbox", func(t *testing.T) {
		tg, _ := sentTarget(t, false)
		if enqInboxContains(t, q, "branch-achrafieh", tg.ID) {
			t.Fatalf("target whose enquiry message is still queued is in ListInbox(branch-achrafieh), want absent")
		}
	})

	t.Run("target_is_never_in_baabda_inbox", func(t *testing.T) {
		tg, _ := sentTarget(t, true)
		if !enqInboxContains(t, q, "branch-achrafieh", tg.ID) {
			t.Fatalf("precondition: target with sent message is not in ListInbox(branch-achrafieh)")
		}
		if enqInboxContains(t, q, "branch-baabda", tg.ID) {
			t.Fatalf("target addressed to branch-achrafieh is in ListInbox(branch-baabda), want absent")
		}
	})
}
