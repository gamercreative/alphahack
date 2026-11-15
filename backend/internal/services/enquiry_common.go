package services

// Wassit enquiry service: shared types and helpers (owned by the lead, see CONTRACT.md).
// Slice files add methods on *EnquiryService:
//   enq_auth.go       Login, Authenticate, Me
//   enq_enquiries.go  CreateEnquiry, GetEnquiryDetail, ListEnquiries, SetMedication, Candidates, AddTargets, CloseEnquiry
//   enq_replies.go    Inbox, Reply, RequestClarification, Clarify, ApproveTargets
//   enq_outbox.go     Send, CancelMessage, FollowUp, MarkNoResponse, Notify, Undo, DispatchDue, RunDispatcher
//   enq_agentapi.go   Wakeups, StartRun, FinishRun, RecordToolCall, Wait
//   enq_stats.go      Stats, EventsSince

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"alphahack/internal/ai"
	"alphahack/internal/queries"
)

// Actor is who is calling. Set by the handlers' auth wrapper.
type Actor struct {
	Type     string // queries.ActorStaff | queries.ActorAgent
	ID       string // staff id, or "wassit" for the agent
	BranchID string // staff branch; "" for the agent
}

func (a Actor) IsAgent() bool { return a.Type == queries.ActorAgent }

// IsClient reports a DOSYN client (client token).
func (a Actor) IsClient() bool { return a.Type == queries.ActorClient }

// Label is stored in outbox_messages.created_by_actor: "staff:<id>", "client:<id>" or "agent".
func (a Actor) Label() string {
	if a.IsAgent() {
		return "agent"
	}
	if a.IsClient() {
		return "client:" + a.ID
	}
	return "staff:" + a.ID
}

// SystemActor is used by the outbox dispatcher.
var SystemActor = Actor{Type: queries.ActorSystem, ID: "dispatcher"}

type EnquiryConfig struct {
	UndoWindow           time.Duration // outbox send delay
	AgentToken           string        // "" disables agent auth
	AgentPriceInPerMTok  float64
	AgentPriceOutPerMTok float64
	SessionTTL           time.Duration // default 12h
	WaitUnit             time.Duration // one wait() "minute"; default 1m (demo can shrink it)
	Now                  func() time.Time
	Logger               *slog.Logger // background errors (outbox dispatcher); nil = slog.Default()
	Speech               ai.Speech    // text to speech for simulated calls; nil = off
	UploadDir            string       // package photos go to UploadDir/photos; "" = ./uploads
}

type EnquiryService struct {
	store queries.Querier
	cfg   EnquiryConfig
}

func NewEnquiryService(store queries.Querier, cfg EnquiryConfig) *EnquiryService {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	if cfg.WaitUnit == 0 {
		cfg.WaitUnit = time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.UploadDir == "" {
		cfg.UploadDir = "./uploads"
	}
	return &EnquiryService{store: store, cfg: cfg}
}

func (s *EnquiryService) now() time.Time { return s.cfg.Now().UTC() }

// Event types (enquiry_events.type).
const (
	EvEnquiryCreated         = "enquiry_created"
	EvMedicationSet          = "medication_set"
	EvClarificationRequested = "clarification_requested"
	EvClarified              = "clarified"
	EvTargetsProposed        = "targets_proposed" // agent proposed, awaiting approval
	EvTargetsAdded           = "targets_added"    // staff added, approved directly
	EvTargetsApproved        = "targets_approved"
	EvMessagesQueued         = "messages_queued"
	EvMessageCancelled       = "message_cancelled"
	EvMessageSent            = "message_sent"
	EvFollowUpQueued         = "follow_up_queued"
	EvMarkedNoResponse       = "marked_no_response"
	EvReplyReceived          = "reply_received"
	EvRequesterNotified      = "requester_notified"
	EvWaitSet                = "wait_set"
	EvEnquiryClosed          = "enquiry_closed"
	EvEventUndone            = "event_undone"
)

// HITL events: each counts as one human-in-the-loop interaction.
var HITLEventTypes = []string{EvClarified, EvTargetsApproved}

// Wake triggers (enquiries.wake_trigger), read by the agent runner.
const (
	WakeCreated   = "created"
	WakeClarified = "clarified"
	WakeApproved  = "approved"
	WakeReply     = "reply"
)

// Inverse is the stored undo instruction (enquiry_events.inverse). Undo (enq_outbox.go)
// switches on Op:
//
//	"set_medication"    MedicationID (nil = clear)          from medication_set
//	"delete_targets"    TargetIDs (only while proposed/approved) from targets_proposed / targets_added
//	"cancel_messages"   MessageIDs + TargetIDs: queued msg -> cancelled, target -> approved;
//	                    already sent -> queue a retraction to that target, target -> cancelled
//	                                                        from messages_queued
//	"set_target_status" TargetID + Status                  from marked_no_response (back to "sent")
//	"reopen"            Status (enquiry status before closing) from enquiry_closed
type Inverse struct {
	Op           string   `json:"op"`
	MedicationID *string  `json:"medication_id,omitempty"`
	TargetIDs    []string `json:"target_ids,omitempty"`
	MessageIDs   []string `json:"message_ids,omitempty"`
	TargetID     string   `json:"target_id,omitempty"`
	Status       string   `json:"status,omitempty"`
}

// logEvent writes one enquiry_events row on q (pass the tx Querier inside InTx).
// payload may be any JSON-marshalable value (nil -> {}); inverse nil -> not reversible.
func (s *EnquiryService) logEvent(ctx context.Context, q queries.Querier, enquiryID string, a Actor,
	typ string, payload any, inverse *Inverse) (queries.EnquiryEvent, error) {
	var p, inv json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return queries.EnquiryEvent{}, Internal(err)
		}
		p = b
	}
	if inverse != nil {
		b, err := json.Marshal(inverse)
		if err != nil {
			return queries.EnquiryEvent{}, Internal(err)
		}
		inv = b
	}
	ev, err := q.CreateEnquiryEvent(ctx, queries.CreateEnquiryEventParams{
		EnquiryID: enquiryID, ActorType: a.Type, ActorID: a.ID, Type: typ, Payload: p, Inverse: inv,
	})
	if err != nil {
		return ev, FromStore(err, "enquiry")
	}
	return ev, nil
}

// canAct reports whether a may change enquiry e as the requester side:
// staff at the requester branch, or the agent when handled_by = 'agent'.
func canAct(a Actor, e queries.Enquiry) error {
	if a.IsAgent() {
		if e.HandledBy != "agent" {
			return Forbidden("this enquiry is handled manually; the agent may not act on it")
		}
		return nil
	}
	if a.BranchID != e.RequesterBranchID {
		return Forbidden("only the requesting pharmacy can change this enquiry")
	}
	return nil
}

// isFinal reports whether the enquiry can no longer change (closed or cancelled).
func isFinal(e queries.Enquiry) bool {
	return e.Status == queries.EnquiryClosed || e.Status == queries.EnquiryCancelled
}

// toUpdate copies e into update params so callers change only the fields they need.
func toUpdate(e queries.Enquiry) queries.UpdateEnquiryParams {
	return queries.UpdateEnquiryParams{
		ID: e.ID, MedicationID: e.MedicationID, Status: e.Status, Outcome: e.Outcome,
		OutcomeBranchID: e.OutcomeBranchID, ClarificationQuestion: e.ClarificationQuestion,
		ClarificationAnswer: e.ClarificationAnswer, WakeTrigger: e.WakeTrigger,
		NextWakeAt: e.NextWakeAt, ClosedAt: e.ClosedAt,
	}
}

// targetToUpdate copies t into update params.
func targetToUpdate(t queries.EnquiryTarget) queries.UpdateEnquiryTargetParams {
	return queries.UpdateEnquiryTargetParams{
		ID: t.ID, Status: t.Status, ReplyQuantity: t.ReplyQuantity, ReplyText: t.ReplyText,
		RepliedByStaffID: t.RepliedByStaffID, RepliedAt: t.RepliedAt, FollowUps: t.FollowUps,
	}
}

// wakeAgent sets the wake trigger on u when the enquiry is agent-handled.
func wakeAgent(e queries.Enquiry, u *queries.UpdateEnquiryParams, trigger string) {
	if e.HandledBy == "agent" {
		u.WakeTrigger = trigger
	}
}

func strPtr(s string) *string        { return &s }
func timePtr(t time.Time) *time.Time { return &t }
