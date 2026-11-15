package services

// Wassit outbox slice: send, cancel, follow-up, no-response, notify, undo and the
// background dispatcher (see CONTRACT.md, "Outbound messages" and "Event types").

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"alphahack/internal/queries"
)

const (
	outboxBodyMin = 1
	outboxBodyMax = 1000
	dispatchBatch = 50
	undoOpReopen  = "reopen"
)

// Send queues one enquiry message per approved target and moves those targets to sent.
func (s *EnquiryService) Send(ctx context.Context, a Actor, id string) ([]queries.OutboxMessage, error) {
	var out []queries.OutboxMessage
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		e, err := q.GetEnquiryForUpdate(ctx, id)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		if isFinal(e) {
			return Conflict("this enquiry is closed")
		}
		if e.Status == queries.EnquiryAwaitingClarification || (e.ClarificationQuestion != "" && e.ClarificationAnswer == "") {
			return Conflict("cannot send while a clarification question is awaiting the requester's answer")
		}
		targets, err := q.ListEnquiryTargets(ctx, e.ID)
		if err != nil {
			return FromStore(err, "target")
		}
		var approved []queries.EnquiryTarget
		for _, t := range targets {
			if t.Status == queries.TargetApproved {
				approved = append(approved, t)
			}
		}
		if len(approved) == 0 {
			return Conflict("no approved targets to send to: propose targets and wait for the requester to approve them")
		}
		text, err := s.outboxQuestion(ctx, q, e)
		if err != nil {
			return err
		}
		sendAfter := s.now().Add(s.cfg.UndoWindow)
		msgs := make([]queries.OutboxMessage, 0, len(approved))
		msgIDs := make([]string, 0, len(approved))
		targetIDs := make([]string, 0, len(approved))
		for _, t := range approved {
			tid := t.ID
			m, err := q.CreateOutboxMessage(ctx, queries.CreateOutboxMessageParams{
				EnquiryID: e.ID, TargetID: &tid, Kind: queries.OutboxKindEnquiry, Body: text,
				SendAfter: sendAfter, CreatedByActor: a.Label(),
			})
			if err != nil {
				return FromStore(err, "message")
			}
			t.Status = queries.TargetSent
			if _, err := q.UpdateEnquiryTarget(ctx, targetToUpdate(t)); err != nil {
				return FromStore(err, "target")
			}
			msgs = append(msgs, m)
			msgIDs = append(msgIDs, m.ID)
			targetIDs = append(targetIDs, t.ID)
		}
		e.Status = queries.EnquiryWaiting
		if _, err := q.UpdateEnquiry(ctx, toUpdate(e)); err != nil {
			return FromStore(err, "enquiry")
		}
		if _, err := s.logEvent(ctx, q, e.ID, a, EvMessagesQueued, map[string]any{
			"message_ids": msgIDs, "target_ids": targetIDs, "send_after": sendAfter,
		}, &Inverse{Op: "cancel_messages", MessageIDs: msgIDs, TargetIDs: targetIDs}); err != nil {
			return err
		}
		out = msgs
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CancelMessage cancels a message that is still queued, so the target never sees it.
func (s *EnquiryService) CancelMessage(ctx context.Context, a Actor, messageID string) (queries.OutboxMessage, error) {
	var out queries.OutboxMessage
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		m0, err := q.GetOutboxMessage(ctx, messageID)
		if err != nil {
			return FromStore(err, "message")
		}
		e, err := q.GetEnquiryForUpdate(ctx, m0.EnquiryID)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		if isFinal(e) {
			return Conflict("this enquiry is closed")
		}
		// Re-read under the enquiry lock so a concurrent change is seen.
		m, err := q.GetOutboxMessage(ctx, messageID)
		if err != nil {
			return FromStore(err, "message")
		}
		if m.Status != queries.OutboxQueued || !s.now().Before(m.SendAfter) {
			return Conflict("message already sent; undo the messages_queued event to send a retraction instead")
		}
		out, err = q.UpdateOutboxMessageStatus(ctx, m.ID, queries.OutboxCancelled, nil)
		if err != nil {
			return FromStore(err, "message")
		}
		if m.TargetID != nil && m.Kind == queries.OutboxKindEnquiry {
			if _, err := s.saveTargetStatus(ctx, q, *m.TargetID, queries.TargetCancelled); err != nil {
				return err
			}
		} else if m.TargetID != nil && m.Kind == queries.OutboxKindFollowUp {
			t, err := q.GetEnquiryTarget(ctx, *m.TargetID)
			if err != nil {
				return FromStore(err, "target")
			}
			if t.FollowUps > 0 {
				t.FollowUps--
				if _, err := q.UpdateEnquiryTarget(ctx, targetToUpdate(t)); err != nil {
					return FromStore(err, "target")
				}
			}
		}
		_, err = s.logEvent(ctx, q, e.ID, a, EvMessageCancelled, map[string]any{
			"message_id": m.ID, "target_id": m.TargetID,
		}, nil)
		return err
	})
	if err != nil {
		return queries.OutboxMessage{}, err
	}
	return out, nil
}

// FollowUp sends one reminder to a target that has been sent the enquiry and has not replied.
func (s *EnquiryService) FollowUp(ctx context.Context, a Actor, targetID string) (queries.OutboxMessage, error) {
	var out queries.OutboxMessage
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		t0, err := q.GetEnquiryTarget(ctx, targetID)
		if err != nil {
			return FromStore(err, "target")
		}
		e, err := q.GetEnquiryForUpdate(ctx, t0.EnquiryID)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		if isFinal(e) {
			return Conflict("this enquiry is closed")
		}
		// Re-read under the enquiry lock so two follow-ups cannot both pass.
		t, err := q.GetEnquiryTarget(ctx, targetID)
		if err != nil {
			return FromStore(err, "target")
		}
		if t.Status != queries.TargetSent {
			return Conflict("only a pharmacy that was sent the enquiry and has not replied can be followed up")
		}
		if t.FollowUps >= 1 {
			return Conflict("a follow-up was already sent; mark it no_response if it stays silent")
		}
		msgs, err := q.ListOutboxMessages(ctx, e.ID)
		if err != nil {
			return FromStore(err, "message")
		}
		var first *queries.OutboxMessage
		for i := range msgs {
			m := msgs[i]
			if m.Kind == queries.OutboxKindEnquiry && m.TargetID != nil && *m.TargetID == t.ID {
				first = &msgs[i] // the latest enquiry message for this target
			}
		}
		if first == nil || first.Status != queries.OutboxSent {
			return Conflict("the first message has not been delivered yet; wait")
		}
		text, err := s.outboxQuestion(ctx, q, e)
		if err != nil {
			return err
		}
		tid := t.ID
		out, err = q.CreateOutboxMessage(ctx, queries.CreateOutboxMessageParams{
			EnquiryID: e.ID, TargetID: &tid, Kind: queries.OutboxKindFollowUp, Body: "Reminder: " + text,
			SendAfter: s.now().Add(s.cfg.UndoWindow), CreatedByActor: a.Label(),
		})
		if err != nil {
			return FromStore(err, "message")
		}
		t.FollowUps++
		if _, err := q.UpdateEnquiryTarget(ctx, targetToUpdate(t)); err != nil {
			return FromStore(err, "target")
		}
		_, err = s.logEvent(ctx, q, e.ID, a, EvFollowUpQueued, map[string]any{
			"target_id": t.ID, "message_id": out.ID,
		}, nil)
		return err
	})
	if err != nil {
		return queries.OutboxMessage{}, err
	}
	return out, nil
}

// MarkNoResponse marks a target that stayed silent after one follow-up. Silence is never "unavailable".
func (s *EnquiryService) MarkNoResponse(ctx context.Context, a Actor, targetID string) (queries.EnquiryTarget, error) {
	var out queries.EnquiryTarget
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		t0, err := q.GetEnquiryTarget(ctx, targetID)
		if err != nil {
			return FromStore(err, "target")
		}
		e, err := q.GetEnquiryForUpdate(ctx, t0.EnquiryID)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		if isFinal(e) {
			return Conflict("this enquiry is closed")
		}
		t, err := q.GetEnquiryTarget(ctx, targetID)
		if err != nil {
			return FromStore(err, "target")
		}
		if t.Status != queries.TargetSent || t.FollowUps < 1 {
			return Conflict("send one follow_up first and wait; no reply is never 'unavailable'")
		}
		// The follow-up must have been delivered and left unanswered for one full wait (5 wait units).
		msgs, err := q.ListOutboxMessages(ctx, e.ID)
		if err != nil {
			return FromStore(err, "message")
		}
		var fuSentAt *time.Time
		for _, m := range msgs {
			if m.Kind == queries.OutboxKindFollowUp && m.TargetID != nil && *m.TargetID == t.ID && m.Status == queries.OutboxSent {
				fuSentAt = m.SentAt
			}
		}
		if fuSentAt == nil || s.now().Sub(*fuSentAt) < 5*s.cfg.WaitUnit {
			return Conflict("the follow-up was sent too recently: wait(5) for a reply before marking no_response")
		}
		prev := t.Status
		t.Status = queries.TargetNoResponse
		out, err = q.UpdateEnquiryTarget(ctx, targetToUpdate(t))
		if err != nil {
			return FromStore(err, "target")
		}
		_, err = s.logEvent(ctx, q, e.ID, a, EvMarkedNoResponse, map[string]any{
			"target_id": t.ID,
		}, &Inverse{Op: "set_target_status", TargetID: t.ID, Status: prev})
		return err
	})
	if err != nil {
		return queries.EnquiryTarget{}, err
	}
	return out, nil
}

// Notify queues a note to the requester. It is not sent to any pharmacy, so it has no send window.
func (s *EnquiryService) Notify(ctx context.Context, a Actor, id, body string) (queries.OutboxMessage, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < outboxBodyMin || n > outboxBodyMax {
		return queries.OutboxMessage{}, Invalid("body must be between %d and %d characters", outboxBodyMin, outboxBodyMax)
	}
	var out queries.OutboxMessage
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		e, err := q.GetEnquiryForUpdate(ctx, id)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		if isFinal(e) {
			return Conflict("this enquiry is closed")
		}
		out, err = q.CreateOutboxMessage(ctx, queries.CreateOutboxMessageParams{
			EnquiryID: e.ID, TargetID: nil, Kind: queries.OutboxKindNotification, Body: body,
			SendAfter: s.now(), CreatedByActor: a.Label(),
		})
		if err != nil {
			return FromStore(err, "message")
		}
		_, err = s.logEvent(ctx, q, e.ID, a, EvRequesterNotified, map[string]any{
			"message_id": out.ID, "body": body,
		}, nil)
		return err
	})
	if err != nil {
		return queries.OutboxMessage{}, err
	}
	return out, nil
}

// Undo runs the stored inverse of one event and records an event_undone row.
func (s *EnquiryService) Undo(ctx context.Context, a Actor, eventID string) (queries.EnquiryEvent, error) {
	var newEv queries.EnquiryEvent
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		ev0, err := q.GetEnquiryEvent(ctx, eventID)
		if err != nil {
			return FromStore(err, "event")
		}
		e, err := q.GetEnquiryForUpdate(ctx, ev0.EnquiryID)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		// Re-read under the enquiry lock so a concurrent undo cannot run twice.
		ev, err := q.GetEnquiryEvent(ctx, eventID)
		if err != nil {
			return FromStore(err, "event")
		}
		if len(ev.Inverse) == 0 || string(ev.Inverse) == "null" {
			return Conflict("this event cannot be undone")
		}
		if ev.UndoneAt != nil {
			return Conflict("this event was already undone")
		}
		var inv Inverse
		if err := json.Unmarshal(ev.Inverse, &inv); err != nil {
			return Internal(err)
		}
		if isFinal(e) && inv.Op != undoOpReopen {
			return Conflict("this enquiry is closed")
		}
		switch inv.Op {
		case "set_medication":
			e.MedicationID = inv.MedicationID
		case "delete_targets":
			if err := s.undoDeleteTargets(ctx, q, &e, inv.TargetIDs); err != nil {
				return err
			}
		case "cancel_messages":
			if err := s.undoCancelMessages(ctx, q, a, e, inv.MessageIDs); err != nil {
				return err
			}
			e.Status = queries.EnquiryOpen
		case "set_target_status":
			t, err := q.GetEnquiryTarget(ctx, inv.TargetID)
			if err != nil {
				return FromStore(err, "target")
			}
			if t.Status != queries.TargetNoResponse {
				return Conflict("cannot undo: target status has changed since marked_no_response")
			}
			if _, err := s.saveTargetStatus(ctx, q, inv.TargetID, inv.Status); err != nil {
				return err
			}
		case undoOpReopen:
			e.Status = inv.Status
			e.Outcome = queries.OutcomeNone
			e.OutcomeBranchID = nil
			e.ClosedAt = nil
			if e.HandledBy == "agent" && e.WakeTrigger == "" && e.NextWakeAt == nil &&
				e.Status != queries.EnquiryAwaitingClarification && e.Status != queries.EnquiryAwaitingApproval {
				e.WakeTrigger = WakeCreated
			}
		default:
			return Conflict("unknown undo operation")
		}
		if _, err := q.UpdateEnquiry(ctx, toUpdate(e)); err != nil {
			return FromStore(err, "enquiry")
		}
		newEv, err = s.logEvent(ctx, q, e.ID, a, EvEventUndone, map[string]any{
			"undone_event_id": ev.ID, "op": inv.Op,
		}, nil)
		if err != nil {
			return err
		}
		if _, err := q.MarkEnquiryEventUndone(ctx, ev.ID, newEv.ID); err != nil {
			return FromStore(err, "event")
		}
		return nil
	})
	if err != nil {
		return queries.EnquiryEvent{}, err
	}
	return newEv, nil
}

// DispatchDue marks due queued messages as sent and logs a message_sent system event for each.
func (s *EnquiryService) DispatchDue(ctx context.Context) (int, error) {
	count := 0
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		due, err := q.ListDueOutboxMessages(ctx, dispatchBatch)
		if err != nil {
			return FromStore(err, "message")
		}
		now := s.now()
		for _, m := range due {
			e, err := q.GetEnquiryForUpdate(ctx, m.EnquiryID)
			if err != nil {
				return FromStore(err, "enquiry")
			}
			cur, err := q.GetOutboxMessage(ctx, m.ID)
			if err != nil {
				return FromStore(err, "message")
			}
			if cur.Status != queries.OutboxQueued {
				continue
			}
			if isFinal(e) {
				if _, err := q.UpdateOutboxMessageStatus(ctx, cur.ID, queries.OutboxCancelled, nil); err != nil {
					return FromStore(err, "message")
				}
				continue
			}
			if _, err := q.UpdateOutboxMessageStatus(ctx, cur.ID, queries.OutboxSent, &now); err != nil {
				return FromStore(err, "message")
			}
			if _, err := s.logEvent(ctx, q, cur.EnquiryID, SystemActor, EvMessageSent, map[string]any{
				"message_id": cur.ID, "kind": cur.Kind, "target_id": cur.TargetID,
			}, nil); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// RunDispatcher calls DispatchDue every `every` until ctx is done. Errors are logged
// and the next tick retries.
func (s *EnquiryService) RunDispatcher(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.DispatchDue(ctx); err != nil && ctx.Err() == nil {
				s.cfg.Logger.Error("outbox dispatch failed", "error", err)
			}
		}
	}
}

// ---- helpers ---------------------------------------------------------------

// outboxQuestion builds the enquiry text sent to targets: "Availability enquiry from <requester>: do you have <what> x<qty>? ...".
func (s *EnquiryService) outboxQuestion(ctx context.Context, q queries.Querier, e queries.Enquiry) (string, error) {
	br, err := q.GetBranch(ctx, e.RequesterBranchID)
	if err != nil {
		return "", FromStore(err, "branch")
	}
	what := e.RawRequest
	if e.MedicationID != nil {
		m, err := q.GetMedication(ctx, *e.MedicationID)
		if err != nil {
			return "", FromStore(err, "medication")
		}
		what = strings.Join(strings.Fields(m.BrandName+" "+m.Strength+" "+m.DosageForm), " ")
	}
	return fmt.Sprintf("Availability enquiry from %s: do you have %s x%d? Reply available, unavailable or alternative.",
		br.Name, what, e.Quantity), nil
}

// saveTargetStatus loads a target, sets its status and saves it. Returns the saved row.
func (s *EnquiryService) saveTargetStatus(ctx context.Context, q queries.Querier, targetID, status string) (queries.EnquiryTarget, error) {
	t, err := q.GetEnquiryTarget(ctx, targetID)
	if err != nil {
		return queries.EnquiryTarget{}, FromStore(err, "target")
	}
	t.Status = status
	out, err := q.UpdateEnquiryTarget(ctx, targetToUpdate(t))
	if err != nil {
		return queries.EnquiryTarget{}, FromStore(err, "target")
	}
	return out, nil
}

// undoDeleteTargets removes proposed or approved targets. Once none are proposed, an
// awaiting_approval enquiry goes back to open.
func (s *EnquiryService) undoDeleteTargets(ctx context.Context, q queries.Querier, e *queries.Enquiry, targetIDs []string) error {
	for _, id := range targetIDs {
		t, err := q.GetEnquiryTarget(ctx, id)
		if err != nil {
			return FromStore(err, "target")
		}
		if t.Status != queries.TargetProposed && t.Status != queries.TargetApproved {
			return Conflict("these pharmacies were already contacted; undo the messages_queued event instead")
		}
	}
	for _, id := range targetIDs {
		if err := q.DeleteEnquiryTarget(ctx, id); err != nil {
			return FromStore(err, "target")
		}
	}
	if e.Status == queries.EnquiryAwaitingApproval {
		targets, err := q.ListEnquiryTargets(ctx, e.ID)
		if err != nil {
			return FromStore(err, "target")
		}
		proposed := false
		for _, t := range targets {
			if t.Status == queries.TargetProposed {
				proposed = true
				break
			}
		}
		if !proposed {
			e.Status = queries.EnquiryOpen
		}
	}
	return nil
}

// undoCancelMessages reverses a messages_queued event. Queued messages are cancelled and
// their targets go back to approved. Messages already sent get a retraction, and their
// targets are cancelled.
func (s *EnquiryService) undoCancelMessages(ctx context.Context, q queries.Querier, a Actor, e queries.Enquiry, messageIDs []string) error {
	now := s.now()
	for _, id := range messageIDs {
		m, err := q.GetOutboxMessage(ctx, id)
		if err != nil {
			return FromStore(err, "message")
		}
		if m.TargetID != nil {
			t, err := q.GetEnquiryTarget(ctx, *m.TargetID)
			if err != nil {
				return FromStore(err, "target")
			}
			if t.Status == queries.TargetAvailable || t.Status == queries.TargetUnavailable || t.Status == queries.TargetAlternativeOffered {
				return Conflict("cannot undo send: a contacted pharmacy has already replied")
			}
		}
		switch m.Status {
		case queries.OutboxCancelled:
			continue
		case queries.OutboxQueued:
			if now.Before(m.SendAfter) {
				if _, err := q.UpdateOutboxMessageStatus(ctx, m.ID, queries.OutboxCancelled, nil); err != nil {
					return FromStore(err, "message")
				}
				if m.TargetID != nil {
					if _, err := s.saveTargetStatus(ctx, q, *m.TargetID, queries.TargetApproved); err != nil {
						return err
					}
				}
				continue
			}
			if _, err := q.UpdateOutboxMessageStatus(ctx, m.ID, queries.OutboxSent, &now); err != nil {
				return FromStore(err, "message")
			}
			fallthrough
		case queries.OutboxSent:
			if _, err := q.CreateOutboxMessage(ctx, queries.CreateOutboxMessageParams{
				EnquiryID: e.ID, TargetID: m.TargetID, Kind: queries.OutboxKindRetraction,
				Body:      "Please disregard our earlier availability enquiry: it has been withdrawn.",
				SendAfter: now, CreatedByActor: a.Label(),
			}); err != nil {
				return FromStore(err, "message")
			}
			if m.TargetID != nil {
				if _, err := s.saveTargetStatus(ctx, q, *m.TargetID, queries.TargetCancelled); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
