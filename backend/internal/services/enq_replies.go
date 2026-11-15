package services

// Wassit replies slice: inbox, pharmacy replies, clarification and approval.
// See CONTRACT.md (product rules 2, 3 and 4; endpoints; event types).

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"alphahack/internal/queries"
)

// ReplyInput is the body of POST /api/v1/targets/{id}/reply.
type ReplyInput struct {
	Kind     string `json:"kind"`
	Quantity *int   `json:"quantity"`
	Text     string `json:"text"`
}

const (
	maxReplyTextChars     = 500
	maxClarificationChars = 500
	maxAnswerChars        = 1000
	maxApproveTargets     = 10
)

func errEnquiryClosed() error { return Conflict("this enquiry is closed") }

// Inbox lists the targets at the caller's branch whose enquiry message was sent.
func (s *EnquiryService) Inbox(ctx context.Context, a Actor, page Page) ([]queries.InboxItem, error) {
	p := page.Normalize()
	items, err := s.store.ListInbox(ctx, a.BranchID, p.Limit, p.Offset)
	if err != nil {
		return nil, FromStore(err, "inbox")
	}
	return items, nil
}

// Reply records a human pharmacist's answer for one target (never the agent).
func (s *EnquiryService) Reply(ctx context.Context, a Actor, targetID string, in ReplyInput) (queries.EnquiryTarget, error) {
	var out queries.EnquiryTarget
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		t0, err := q.GetEnquiryTarget(ctx, targetID)
		if err != nil {
			return FromStore(err, "target")
		}
		if t0.BranchID != a.BranchID {
			return Forbidden("only pharmacists at the contacted pharmacy can reply")
		}
		e, err := q.GetEnquiryForUpdate(ctx, t0.EnquiryID)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if isFinal(e) {
			return errEnquiryClosed()
		}
		// Re-read the target under the enquiry lock so concurrent replies serialize.
		t, err := q.GetEnquiryTarget(ctx, targetID)
		if err != nil {
			return FromStore(err, "target")
		}
		msgs, err := q.ListOutboxMessages(ctx, e.ID)
		if err != nil {
			return FromStore(err, "outbox")
		}
		if !deliveredToTarget(msgs, t.ID) {
			return Conflict("this enquiry has not been delivered to your pharmacy yet")
		}
		// A late reply is accepted while the target is still waiting for an answer.
		if t.Status != queries.TargetSent && t.Status != queries.TargetNoResponse {
			return Conflict("this enquiry was already answered or withdrawn")
		}

		var status string
		switch in.Kind {
		case "available":
			status = queries.TargetAvailable
		case "unavailable":
			status = queries.TargetUnavailable
		case "alternative":
			status = queries.TargetAlternativeOffered
		default:
			return Invalid("kind must be available, unavailable or alternative")
		}
		if in.Quantity != nil && (*in.Quantity < 0 || *in.Quantity > maxEnquiryQuantity) {
			return Invalid("quantity must be between 0 and %d", maxEnquiryQuantity)
		}
		if in.Kind == "available" && (in.Quantity == nil || *in.Quantity <= 0) {
			return Invalid("quantity must be at least 1 when replying available")
		}
		text := strings.TrimSpace(in.Text)
		if utf8.RuneCountInString(text) > maxReplyTextChars {
			return Invalid("text must be %d characters or fewer", maxReplyTextChars)
		}
		if in.Kind == "alternative" && text == "" {
			return Invalid("describe the alternative, e.g. '625mg only'")
		}

		t.Status = status
		t.ReplyQuantity = in.Quantity
		t.ReplyText = text
		t.RepliedByStaffID = strPtr(a.ID)
		t.RepliedAt = timePtr(s.now())
		updated, err := q.UpdateEnquiryTarget(ctx, targetToUpdate(t))
		if err != nil {
			return FromStore(err, "target")
		}

		u := toUpdate(e)
		wakeAgent(e, &u, WakeReply)
		if _, err := q.UpdateEnquiry(ctx, u); err != nil {
			return FromStore(err, "enquiry")
		}

		if _, err := s.logEvent(ctx, q, e.ID, a, EvReplyReceived, map[string]any{
			"target_id": t.ID,
			"branch_id": t.BranchID,
			"kind":      in.Kind,
			"quantity":  in.Quantity,
			"text":      text,
		}, nil); err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return queries.EnquiryTarget{}, err
	}
	return out, nil
}

// RequestClarification lets the agent ask the requester a question (agent only).
func (s *EnquiryService) RequestClarification(ctx context.Context, a Actor, id, question string) (queries.Enquiry, error) {
	var out queries.Enquiry
	var blocked string // content filter reason; the tx commits only its event
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		e, err := q.GetEnquiryForUpdate(ctx, id)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		if isFinal(e) {
			return errEnquiryClosed()
		}
		if e.Status == queries.EnquiryAwaitingClarification {
			return Conflict("a question is already waiting for the requester; stop now and end this run")
		}
		question = strings.TrimSpace(question)
		if n := utf8.RuneCountInString(question); n < 1 || n > maxClarificationChars {
			return Invalid("question must be 1 to %d characters", maxClarificationChars)
		}
		if a.IsAgent() {
			if blocked, err = s.filterAgentText(ctx, q, e, question); err != nil || blocked != "" {
				return err
			}
		}

		u := toUpdate(e)
		u.Status = queries.EnquiryAwaitingClarification
		u.ClarificationQuestion = question
		u.ClarificationAnswer = ""
		u.WakeTrigger = ""
		updated, err := q.UpdateEnquiry(ctx, u)
		if err != nil {
			return FromStore(err, "enquiry")
		}

		if _, err := s.logEvent(ctx, q, e.ID, a, EvClarificationRequested,
			map[string]any{"question": question}, nil); err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return queries.Enquiry{}, err
	}
	if blocked != "" {
		return queries.Enquiry{}, ContentFiltered(blocked)
	}
	return out, nil
}

// Clarify is the requester's answer to a pending clarification question (staff only).
func (s *EnquiryService) Clarify(ctx context.Context, a Actor, id, answer string) (queries.Enquiry, error) {
	var out queries.Enquiry
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		e, err := q.GetEnquiryForUpdate(ctx, id)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		if isFinal(e) {
			return errEnquiryClosed()
		}
		if e.Status != queries.EnquiryAwaitingClarification && !(e.ClarificationQuestion != "" && e.ClarificationAnswer == "") {
			return Conflict("no clarification is pending")
		}
		answer = strings.TrimSpace(answer)
		if n := utf8.RuneCountInString(answer); n < 1 || n > maxAnswerChars {
			return Invalid("answer must be 1 to %d characters", maxAnswerChars)
		}

		u := toUpdate(e)
		if e.Status == queries.EnquiryAwaitingClarification {
			u.Status = queries.EnquiryOpen
		}
		u.ClarificationAnswer = answer
		wakeAgent(e, &u, WakeClarified)
		updated, err := q.UpdateEnquiry(ctx, u)
		if err != nil {
			return FromStore(err, "enquiry")
		}

		if _, err := s.logEvent(ctx, q, e.ID, a, EvClarified,
			map[string]any{"question": e.ClarificationQuestion, "answer": answer}, nil); err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return queries.Enquiry{}, err
	}
	return out, nil
}

// ApproveTargets approves proposed targets of one enquiry (staff only).
func (s *EnquiryService) ApproveTargets(ctx context.Context, a Actor, id string, targetIDs []string) ([]queries.EnquiryTarget, error) {
	ids := uniqueStrings(targetIDs)
	if len(ids) < 1 || len(ids) > maxApproveTargets {
		return nil, Invalid("target_ids must contain 1 to %d ids", maxApproveTargets)
	}
	out := make([]queries.EnquiryTarget, 0, len(ids))
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		e, err := q.GetEnquiryForUpdate(ctx, id)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		if isFinal(e) {
			return errEnquiryClosed()
		}

		// Validate every target before writing anything.
		targets := make([]queries.EnquiryTarget, 0, len(ids))
		for _, tid := range ids {
			t, err := q.GetEnquiryTarget(ctx, tid)
			if err != nil {
				return FromStore(err, "target")
			}
			if t.EnquiryID != e.ID {
				return NotFound("target")
			}
			if t.Status != queries.TargetProposed {
				return Conflict(fmt.Sprintf("target %s is not awaiting approval", t.ID))
			}
			targets = append(targets, t)
		}

		for _, t := range targets {
			t.Status = queries.TargetApproved
			updated, err := q.UpdateEnquiryTarget(ctx, targetToUpdate(t))
			if err != nil {
				return FromStore(err, "target")
			}
			out = append(out, updated)
		}

		all, err := q.ListEnquiryTargets(ctx, e.ID)
		if err != nil {
			return FromStore(err, "target")
		}
		stillProposed := false
		for _, t := range all {
			if t.Status == queries.TargetProposed {
				stillProposed = true
				break
			}
		}

		u := toUpdate(e)
		if !stillProposed && e.Status == queries.EnquiryAwaitingApproval {
			u.Status = queries.EnquiryOpen
		}
		wakeAgent(e, &u, WakeApproved)
		if _, err := q.UpdateEnquiry(ctx, u); err != nil {
			return FromStore(err, "enquiry")
		}

		_, err = s.logEvent(ctx, q, e.ID, a, EvTargetsApproved,
			map[string]any{"target_ids": ids}, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// deliveredToTarget reports whether an enquiry message was sent to the target.
func deliveredToTarget(msgs []queries.OutboxMessage, targetID string) bool {
	for _, m := range msgs {
		if m.Kind == queries.OutboxKindEnquiry && m.Status == queries.OutboxSent &&
			m.TargetID != nil && *m.TargetID == targetID {
			return true
		}
	}
	return false
}

// uniqueStrings drops repeated values, keeping the first occurrence.
func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
