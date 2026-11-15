package services

// DOSYN client tickets. A ticket wraps one client-desk enquiry (requester =
// the DOSYN client desk branch, handled_by = agent). Pharmacies only ever see
// the desk, never the client (CONTRACT rule 8). The thread the client reads is
// built here from enquiry events, outbox messages, targets, offers and photos;
// nothing extra is stored for it. The best offer is computed in Go (rule 11).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"alphahack/internal/queries"
)

const (
	maxTicketQuantity = 10

	EvHoldRequested = "hold_requested" // client reserved: a hold notification is queued to that pharmacy
	photoDisclaimer = "A photo alone does not prove a box is genuine."
	noAnswerText    = "No reply after one follow-up. Recorded as no response, never as unavailable."
)

// ---- views -------------------------------------------------------------------

type TicketMedication struct {
	ID               string `json:"id"`
	BrandName        string `json:"brand_name"`
	ActiveIngredient string `json:"active_ingredient"`
	Strength         string `json:"strength"`
	DosageForm       string `json:"dosage_form"`
	UnitPriceCents   int64  `json:"unit_price_cents"`
}

type BranchRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TicketView is the ticket row plus the enquiry's quantity.
type TicketView struct {
	queries.ClientTicket
	Quantity int `json:"quantity"`
}

// ThreadRow is one entry of the client's ticket thread. Kinds: client_request,
// wassit_note, outreach, pharmacy_reply, declined, no_answer, offer, dose_check,
// photo_check, best_offer, reserved. decider: rules | human | pharmacy | client | wassit.
type ThreadRow struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Decider  string    `json:"decider"`
	BranchID *string   `json:"branch_id"`
	Text     string    `json:"text"`
	Data     any       `json:"data"`
}

type TicketDetail struct {
	Ticket     TicketView       `json:"ticket"`
	Medication TicketMedication `json:"medication"`
	Branches   []BranchRef      `json:"branches"`
	Working    bool             `json:"working"`
	Thread     []ThreadRow      `json:"thread"`
	BestOffer  *BestOffer       `json:"best_offer"`
}

type TicketSummary struct {
	ID               string           `json:"id"`
	EnquiryID        string           `json:"enquiry_id"`
	ScheduleID       *string          `json:"schedule_id"`
	Medication       TicketMedication `json:"medication"`
	Quantity         int              `json:"quantity"`
	Mode             string           `json:"mode"`
	PickupWindow     string           `json:"pickup_window"`
	Status           string           `json:"status"`
	ReservedBranchID *string          `json:"reserved_branch_id"`
	CreatedAt        time.Time        `json:"created_at"`
	Working          bool             `json:"working"`
	BestOffer        *BestOffer       `json:"best_offer"`
}

type CreateTicketInput struct {
	MedicationID string `json:"medication_id"`
	Quantity     int    `json:"quantity"`
	PickupWindow string `json:"pickup_window"`
}

type ReserveInput struct {
	BranchID string `json:"branch_id"`
}

func clientOnlyErr(a Actor) error {
	if a.IsClient() {
		return nil
	}
	if a.IsAgent() {
		return AgentForbidden()
	}
	return Forbidden("this route is for DOSYN clients only")
}

func ticketMed(m queries.Medication) TicketMedication {
	return TicketMedication{ID: m.ID, BrandName: m.BrandName, ActiveIngredient: m.ActiveIngredient,
		Strength: m.Strength, DosageForm: m.DosageForm, UnitPriceCents: m.UnitPriceCents}
}

func medLabel(m queries.Medication) string {
	return strings.Join(strings.Fields(m.BrandName+" "+m.Strength+" "+m.DosageForm), " ")
}

// PickupWindows are the only accepted pickup windows (the values the UI offers).
// Free text is refused: it reaches pharmacies and the agent, so a client could
// otherwise put a name, a phone number or instructions there (rule 8).
var PickupWindows = []string{
	"Today before 18:00", "Tomorrow morning", "Within 2 days",
	"Same day before 18:00", "Same day after 18:00", "Next morning",
}

// validatePickup returns the canonical spelling of an allowed pickup window.
func validatePickup(p string) (string, error) {
	p = strings.Join(strings.Fields(clean(p)), " ")
	for _, w := range PickupWindows {
		if strings.EqualFold(p, w) {
			return w, nil
		}
	}
	return "", Invalid("pickup_window must be one of: %s", strings.Join(PickupWindows, ", "))
}

func validateTicketQuantity(q int) error {
	if q < 1 || q > maxTicketQuantity {
		return Invalid("quantity must be between 1 and %d", maxTicketQuantity)
	}
	return nil
}

// ---- create --------------------------------------------------------------------

// CreateTicket opens a client ticket and its client-desk enquiry in one transaction.
func (s *EnquiryService) CreateTicket(ctx context.Context, a Actor, in CreateTicketInput) (TicketView, error) {
	if err := clientOnlyErr(a); err != nil {
		return TicketView{}, err
	}
	if err := validateTicketQuantity(in.Quantity); err != nil {
		return TicketView{}, err
	}
	pickup, err := validatePickup(in.PickupWindow)
	if err != nil {
		return TicketView{}, err
	}
	medID := clean(in.MedicationID)
	if medID == "" {
		return TicketView{}, Invalid("medication_id is required; pick one from GET /api/v1/medications")
	}
	var out TicketView
	err = s.enquiryTx(ctx, func(q queries.Querier) error {
		out, err = s.openTicket(ctx, q, a.ID, medID, in.Quantity, pickup, queries.TicketOnce, nil, WakeCreated)
		return err
	})
	return out, err
}

// openTicket creates the desk enquiry (raw_request from catalogue fields only,
// never client data), the ticket row and the enquiry_created event (actor client).
func (s *EnquiryService) openTicket(ctx context.Context, q queries.Querier, clientID, medID string, qty int,
	pickup, mode string, scheduleID *string, wake string) (TicketView, error) {
	m, err := q.GetMedication(ctx, medID)
	if err != nil {
		return TicketView{}, FromStore(err, "medication")
	}
	raw := fmt.Sprintf("need %s x%d, pickup %s", medLabel(m), qty, pickup)
	e, err := q.CreateEnquiry(ctx, queries.CreateEnquiryParams{
		RequesterBranchID: queries.DeskBranchID, RequesterStaffID: queries.DeskStaffID,
		RawRequest: raw, MedicationID: &m.ID, Quantity: qty, Status: queries.EnquiryOpen,
		HandledBy: "agent", WakeTrigger: wake,
	})
	if err != nil {
		return TicketView{}, FromStore(err, "enquiry")
	}
	t, err := q.CreateClientTicket(ctx, queries.CreateClientTicketParams{
		ClientID: clientID, EnquiryID: e.ID, ScheduleID: scheduleID, Mode: mode, PickupWindow: pickup,
	})
	if err != nil {
		return TicketView{}, FromStore(err, "ticket")
	}
	if _, err := s.logEvent(ctx, q, e.ID, Actor{Type: queries.ActorClient, ID: clientID}, EvEnquiryCreated,
		map[string]any{"raw_request": raw, "quantity": qty, "handled_by": "agent", "medication_id": m.ID,
			"ticket_id": t.ID, "mode": mode, "pickup_window": pickup}, nil); err != nil {
		return TicketView{}, err
	}
	return TicketView{ClientTicket: t, Quantity: qty}, nil
}

// ---- read ----------------------------------------------------------------------

// ownTicket loads a ticket of the calling client; anyone else's ticket is 404.
func (s *EnquiryService) ownTicket(ctx context.Context, q queries.Querier, a Actor, id string) (queries.ClientTicket, error) {
	t, err := q.GetClientTicket(ctx, id)
	if err != nil {
		return t, FromStore(err, "ticket")
	}
	if t.ClientID != a.ID {
		return t, NotFound("ticket")
	}
	return t, nil
}

func (s *EnquiryService) ListTickets(ctx context.Context, a Actor, status string, p Page) ([]TicketSummary, error) {
	if err := clientOnlyErr(a); err != nil {
		return nil, err
	}
	status = clean(status)
	switch status {
	case "", queries.TicketOpen, queries.TicketReserved, queries.TicketClosed:
	default:
		return nil, Invalid("status must be open, reserved, closed or empty")
	}
	p = p.Normalize()
	rows, err := s.store.ListClientTickets(ctx, a.ID, status, p.Limit, p.Offset)
	if err != nil {
		return nil, FromStore(err, "ticket")
	}
	out := make([]TicketSummary, 0, len(rows))
	for _, t := range rows {
		d, err := s.ticketDetail(ctx, s.store, t)
		if err != nil {
			return nil, err
		}
		out = append(out, TicketSummary{
			ID: t.ID, EnquiryID: t.EnquiryID, ScheduleID: t.ScheduleID, Medication: d.Medication,
			Quantity: d.Ticket.Quantity, Mode: t.Mode, PickupWindow: t.PickupWindow, Status: t.Status,
			ReservedBranchID: t.ReservedBranchID, CreatedAt: t.CreatedAt, Working: d.Working, BestOffer: d.BestOffer,
		})
	}
	return out, nil
}

func (s *EnquiryService) GetTicket(ctx context.Context, a Actor, id string) (TicketDetail, error) {
	if err := clientOnlyErr(a); err != nil {
		return TicketDetail{}, err
	}
	t, err := s.ownTicket(ctx, s.store, a, id)
	if err != nil {
		return TicketDetail{}, err
	}
	return s.ticketDetail(ctx, s.store, t)
}

// ticketState is everything the thread and the best offer are built from.
type ticketState struct {
	ticket   queries.ClientTicket
	enquiry  queries.Enquiry
	med      queries.Medication
	targets  []queries.EnquiryTarget
	msgs     []queries.OutboxMessage
	events   []queries.EnquiryEvent
	offers   []queries.TargetOffer
	photos   []queries.PackagePhoto
	branches map[string]string // id -> name
}

func (s *EnquiryService) loadTicketState(ctx context.Context, q queries.Querier, t queries.ClientTicket) (ticketState, error) {
	st := ticketState{ticket: t, branches: map[string]string{}}
	var err error
	if st.enquiry, err = q.GetEnquiry(ctx, t.EnquiryID); err != nil {
		return st, FromStore(err, "enquiry")
	}
	if st.enquiry.MedicationID != nil {
		if st.med, err = q.GetMedication(ctx, *st.enquiry.MedicationID); err != nil {
			return st, FromStore(err, "medication")
		}
	}
	if st.targets, err = q.ListEnquiryTargets(ctx, t.EnquiryID); err != nil {
		return st, FromStore(err, "target")
	}
	if st.msgs, err = q.ListOutboxMessages(ctx, t.EnquiryID); err != nil {
		return st, FromStore(err, "message")
	}
	if st.events, err = q.ListEnquiryEvents(ctx, t.EnquiryID, 0); err != nil {
		return st, FromStore(err, "event")
	}
	if st.offers, err = q.ListTargetOffers(ctx, t.EnquiryID); err != nil {
		return st, FromStore(err, "offer")
	}
	if st.photos, err = q.ListEnquiryPhotos(ctx, t.EnquiryID); err != nil {
		return st, FromStore(err, "photo")
	}
	for _, tg := range st.targets {
		if _, ok := st.branches[tg.BranchID]; ok {
			continue
		}
		b, err := q.GetBranch(ctx, tg.BranchID)
		if err != nil {
			return st, FromStore(err, "branch")
		}
		st.branches[b.ID] = b.Name
	}
	return st, nil
}

func (s *EnquiryService) ticketDetail(ctx context.Context, q queries.Querier, t queries.ClientTicket) (TicketDetail, error) {
	st, err := s.loadTicketState(ctx, q, t)
	if err != nil {
		return TicketDetail{}, err
	}
	return buildTicketDetail(st), nil
}

// ---- offer extras (read from the offer_decided event payload) ------------------

// offerExtras are the equivalence fields the offer rules put in the
// offer_decided payload (target_offers has no columns for them).
type offerExtras struct {
	DoseCheck    *DoseCheck `json:"dose_check"`
	ExpiryMonth  string     `json:"expiry_month"`
	UnitStrength string     `json:"unit_strength"`
	TotalCents   *int64     `json:"total_cents"`
	Equivalent   *bool      `json:"equivalent"`
	GovCode      string     `json:"gov_code"`
}

func offerExtrasByID(events []queries.EnquiryEvent) map[string]offerExtras {
	out := map[string]offerExtras{}
	for _, ev := range events {
		if ev.Type != EvOfferDecided {
			continue
		}
		var p struct {
			OfferID string `json:"offer_id"`
			offerExtras
		}
		if json.Unmarshal(ev.Payload, &p) != nil || p.OfferID == "" {
			continue
		}
		p.ExpiryMonth = normExpiryMonth(p.ExpiryMonth)
		out[p.OfferID] = p.offerExtras
	}
	return out
}

// offerTotal is the offer's total in cents: the rules' total, else price x quantity.
func offerTotal(o queries.TargetOffer, x offerExtras) (int64, bool) {
	if x.TotalCents != nil {
		return *x.TotalCents, true
	}
	if o.PriceCents != nil && o.Quantity != nil {
		return *o.PriceCents * int64(*o.Quantity), true
	}
	return 0, false
}

// normExpiryMonth accepts YYYY-MM, YYYY/MM, MM/YYYY or MM-YYYY and returns
// YYYY-MM, or "" when it cannot be read.
func normExpiryMonth(s string) string {
	s = strings.TrimSpace(s)
	var y, m int
	var rest string
	for _, layout := range []string{"%4d-%2d%s", "%4d/%2d%s", "%2d/%4d%s", "%2d-%4d%s"} {
		y, m, rest = 0, 0, ""
		var a, b int
		n, _ := fmt.Sscanf(s+"|", layout, &a, &b, &rest)
		if n < 2 || rest != "|" {
			continue
		}
		if strings.HasPrefix(layout, "%4d") {
			y, m = a, b
		} else {
			m, y = a, b
		}
		if y >= 2000 && y <= 2100 && m >= 1 && m <= 12 {
			return fmt.Sprintf("%04d-%02d", y, m)
		}
	}
	return ""
}

// ---- best offer ------------------------------------------------------------------

// bestCandidates: one candidate per target that replied (or went silent), in
// target order, with its latest offer and latest photo.
func bestCandidates(st ticketState) []BestCandidate {
	extras := offerExtrasByID(st.events)
	latestOffer := map[string]queries.TargetOffer{}
	for _, o := range st.offers {
		latestOffer[o.TargetID] = o // ordered by created_at: the last one wins
	}
	latestPhoto := map[string]queries.PackagePhoto{}
	for _, p := range st.photos {
		latestPhoto[p.TargetID] = p
	}
	var out []BestCandidate
	for _, t := range st.targets {
		switch t.Status {
		case queries.TargetAvailable, queries.TargetUnavailable, queries.TargetAlternativeOffered, queries.TargetNoResponse:
		default:
			continue
		}
		c := BestCandidate{BranchID: t.BranchID, Declined: t.Status == queries.TargetUnavailable,
			NoResponse: t.Status == queries.TargetNoResponse}
		if o, ok := latestOffer[t.ID]; ok {
			x := extras[o.ID]
			c.OfferID, c.Decision = o.ID, o.Decision
			c.TotalCents, c.HasTotal = offerTotal(o, x)
			c.ExpiryMonth = x.ExpiryMonth
			c.DoseRejected = x.DoseCheck != nil && !x.DoseCheck.Accepted
		}
		if p, ok := latestPhoto[t.ID]; ok {
			c.PhotoStatus = p.Status
			if c.ExpiryMonth == "" && p.Status == queries.PhotoVerified {
				c.ExpiryMonth = p.TypedExpiryMonth // typed next to a verified photo
			}
		}
		out = append(out, c)
	}
	return out
}

// verifiedAt reports whether branchID has a verified offer (rule 11), the
// condition for reserving there and for closing a desk enquiry available there.
func verifiedAt(st ticketState, branchID string) bool {
	for _, c := range bestCandidates(st) {
		if c.BranchID == branchID && bestVerified(c) {
			return true
		}
	}
	return false
}

// ---- thread ----------------------------------------------------------------------

func fmtCents(c int64) string {
	if c%100 == 0 {
		return fmt.Sprintf("%d$", c/100)
	}
	return fmt.Sprintf("%d.%02d$", c/100, c%100)
}

func branchPtr(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

func payloadMap(raw json.RawMessage) map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal(raw, &m)
	return m
}

func payloadString(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

func buildTicketDetail(st ticketState) TicketDetail {
	t, e := st.ticket, st.enquiry
	d := TicketDetail{
		Ticket:     TicketView{ClientTicket: t, Quantity: e.Quantity},
		Medication: ticketMed(st.med),
		Branches:   []BranchRef{},
		Thread:     []ThreadRow{},
	}
	targetBranch := map[string]string{}
	pending, active := false, 0
	for _, tg := range st.targets {
		targetBranch[tg.ID] = tg.BranchID
		if tg.Status == queries.TargetCancelled {
			continue
		}
		active++
		d.Branches = append(d.Branches, BranchRef{ID: tg.BranchID, Name: st.branches[tg.BranchID]})
		switch tg.Status {
		case queries.TargetProposed, queries.TargetApproved, queries.TargetSent:
			pending = true
		}
	}
	// Working: Wassit still has something to do (no pharmacy asked yet, or one still pending).
	d.Working = !isFinal(e) && (pending || active == 0)

	rows := &d.Thread
	add := func(r ThreadRow) { *rows = append(*rows, r) }
	extras := offerExtrasByID(st.events)
	var closedByClient *queries.EnquiryEvent

	for i := range st.events {
		ev := st.events[i]
		p := payloadMap(ev.Payload)
		switch ev.Type {
		case EvEnquiryCreated:
			add(ThreadRow{ID: ev.ID, At: ev.CreatedAt, Kind: "client_request", Decider: "client",
				Text: fmt.Sprintf("%s × %d · %s · pickup %s", medLabel(st.med), e.Quantity, t.Mode, t.PickupWindow),
				Data: map[string]any{}})
		case EvClarificationRequested:
			add(ThreadRow{ID: ev.ID, At: ev.CreatedAt, Kind: "wassit_note", Decider: "wassit",
				Text: payloadString(p, "question"), Data: map[string]any{}})
		case EvReplyReceived:
			kind := "pharmacy_reply"
			if payloadString(p, "kind") == "unavailable" {
				kind = "declined"
			}
			add(ThreadRow{ID: ev.ID, At: ev.CreatedAt, Kind: kind, Decider: "pharmacy",
				BranchID: branchPtr(payloadString(p, "branch_id")), Text: payloadString(p, "text"), Data: map[string]any{}})
		case EvMarkedNoResponse:
			add(ThreadRow{ID: ev.ID, At: ev.CreatedAt, Kind: "no_answer", Decider: "rules",
				BranchID: branchPtr(targetBranch[payloadString(p, "target_id")]), Text: noAnswerText, Data: map[string]any{}})
		case EvEnquiryClosed:
			if ev.ActorType == queries.ActorClient && ev.UndoneAt == nil {
				closedByClient = &st.events[i]
			}
		}
	}

	// Outreach: one row per distinct enquiry message (what the pharmacies saw).
	type group struct {
		at       time.Time
		id       string
		branches []string
	}
	groups := map[string]*group{}
	var order []string
	for _, m := range st.msgs {
		switch {
		case m.Kind == queries.OutboxKindEnquiry && m.Status != queries.OutboxCancelled && m.TargetID != nil:
			g, ok := groups[m.Body]
			if !ok {
				g = &group{at: m.CreatedAt, id: m.ID}
				groups[m.Body] = g
				order = append(order, m.Body)
			}
			g.branches = append(g.branches, targetBranch[*m.TargetID])
		case m.Kind == queries.OutboxKindNotification && m.TargetID == nil && m.Status != queries.OutboxCancelled:
			add(ThreadRow{ID: m.ID, At: m.CreatedAt, Kind: "wassit_note", Decider: "wassit", Text: m.Body, Data: map[string]any{}})
		}
	}
	for _, body := range order {
		g := groups[body]
		add(ThreadRow{ID: g.id, At: g.at, Kind: "outreach", Decider: "wassit",
			Text: fmt.Sprintf("Asked %d pharmacies.", len(g.branches)),
			Data: map[string]any{"branch_ids": g.branches, "message": body}})
	}

	// Offers, with the rules' dose check just before the offer it belongs to.
	for _, o := range st.offers {
		x := extras[o.ID]
		if dc := x.DoseCheck; dc != nil {
			add(doseCheckRow(o, *dc))
		}
		add(ThreadRow{ID: o.ID, At: o.CreatedAt, Kind: "offer", Decider: "rules", BranchID: branchPtr(o.BranchID),
			Text: offerText(o, x), Data: map[string]any{"offer": offerData(o, x)}})
	}
	for _, p := range st.photos {
		add(photoRow(p))
	}

	sort.SliceStable(*rows, func(i, j int) bool { return (*rows)[i].At.Before((*rows)[j].At) })

	// Best offer (computed, never stored), then the reservation.
	cands := bestCandidates(st)
	d.BestOffer = PickBestOffer(cands)
	if len(cands) > 0 {
		last := e.CreatedAt
		if n := len(*rows); n > 0 {
			last = (*rows)[n-1].At
		}
		add(bestOfferRow(st, cands, d.BestOffer, last.Add(time.Millisecond)))
	}
	if closedByClient != nil && t.ReservedBranchID != nil {
		bid := *t.ReservedBranchID
		name := st.branches[bid]
		at := closedByClient.CreatedAt
		if n := len(*rows); n > 0 && !at.After((*rows)[n-1].At) {
			at = (*rows)[n-1].At.Add(time.Millisecond)
		}
		add(ThreadRow{ID: closedByClient.ID, At: at, Kind: "reserved", Decider: "client", BranchID: branchPtr(bid),
			Text: "Reserved at " + name, Data: map[string]any{}})
		add(ThreadRow{ID: closedByClient.ID + ":note", At: at.Add(time.Millisecond), Kind: "wassit_note", Decider: "wassit",
			Text: fmt.Sprintf("I asked %s to hold it until %s. Pay at the counter.", name, t.PickupWindow), Data: map[string]any{}})
	}
	return d
}

func offerData(o queries.TargetOffer, x offerExtras) map[string]any {
	b, _ := json.Marshal(o)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	total, ok := offerTotal(o, x)
	if ok {
		m["total_cents"] = total
	} else {
		m["total_cents"] = nil
	}
	m["expiry_month"] = x.ExpiryMonth
	m["unit_strength"] = x.UnitStrength
	m["equivalent"] = x.Equivalent != nil && *x.Equivalent
	m["gov_code"] = x.GovCode
	return m
}

func offerText(o queries.TargetOffer, x offerExtras) string {
	switch o.Decision {
	case queries.OfferMatches:
		if total, ok := offerTotal(o, x); ok {
			return "Offer matches the request: " + fmtCents(total) + " total."
		}
		return "Offer matches the request."
	case queries.OfferRejected:
		return "Offer rejected by the rules."
	default:
		return "Offer needs a human check."
	}
}

func doseCheckRow(o queries.TargetOffer, dc DoseCheck) ThreadRow {
	text := fmt.Sprintf("Dose equivalent rejected by rules: %s does not add up to %s. Not counted as a match.", dc.Offered, dc.Requested)
	if dc.Accepted {
		text = fmt.Sprintf("Dose equivalent accepted by rules: %s = %s, same ingredient and form, listed in the government reference.", dc.Offered, dc.Requested)
	}
	var gov any
	if dc.GovCode != "" {
		gov = map[string]any{"code": dc.GovCode}
	}
	checks := dc.Checks
	if checks == nil {
		checks = []DoseCheckItem{}
	}
	return ThreadRow{ID: o.ID + ":dose", At: o.CreatedAt.Add(-time.Microsecond), Kind: "dose_check", Decider: "rules",
		BranchID: branchPtr(o.BranchID), Text: text,
		Data: map[string]any{"accepted": dc.Accepted, "requested": dc.Requested, "offered": dc.Offered,
			"checks": checks, "gov_ref": gov}}
}

func photoRow(p queries.PackagePhoto) ThreadRow {
	decider := "rules"
	if p.DecidedBy == "human" || p.Status == queries.PhotoFlagged {
		decider = "human"
	}
	var text string
	switch p.Status {
	case queries.PhotoVerified:
		text = "Package photo checked: the typed code matches the government reference and the expiry is after pickup."
	case queries.PhotoMismatch:
		text = "Package photo did not match the government reference."
	case queries.PhotoRejected:
		text = "Package photo rejected by a pharmacist on our team."
	default:
		text = "Package photo needs a human check; a pharmacist on our team reviews it."
	}
	var expiry any
	if p.TypedExpiryMonth != "" {
		expiry = p.TypedExpiryMonth
	}
	return ThreadRow{ID: p.ID, At: p.CreatedAt, Kind: "photo_check", Decider: decider, BranchID: branchPtr(p.BranchID),
		Text: text, Data: map[string]any{"photo_id": p.ID, "status": p.Status, "barcode": p.TypedCode,
			"gov_match": p.GovMatch, "expiry": expiry, "note": p.Note}}
}

func bestOfferRow(st ticketState, cands []BestCandidate, best *BestOffer, at time.Time) ThreadRow {
	data := map[string]any{"branch_id": nil, "total_cents": nil, "expiry_month": nil, "offer_id": nil}
	var excluded []BestExcluded
	text := "No verified offer yet. An offer counts only after the rules match it and its package photo is verified."
	var bid *string
	if best != nil {
		bid = branchPtr(best.BranchID)
		data["branch_id"], data["total_cents"], data["offer_id"] = best.BranchID, best.TotalCents, best.OfferID
		if best.ExpiryMonth != "" {
			data["expiry_month"] = best.ExpiryMonth
		}
		excluded = best.Excluded
		text = fmt.Sprintf("Best verified offer: %s, %s total.", st.branches[best.BranchID], fmtCents(best.TotalCents))
		if best.ExpiryMonth != "" {
			text = strings.TrimSuffix(text, ".") + ", expiry " + best.ExpiryMonth + "."
		}
	} else {
		for _, c := range cands {
			excluded = append(excluded, BestExcluded{BranchID: c.BranchID, Reason: bestExcludeReason(c, BestCandidate{})})
		}
	}
	if excluded == nil {
		excluded = []BestExcluded{}
	}
	data["excluded"] = excluded
	b, _ := json.Marshal(data)
	sum := sha256.Sum256(b)
	return ThreadRow{ID: "best_offer:" + hex.EncodeToString(sum[:6]), At: at, Kind: "best_offer", Decider: "rules",
		BranchID: bid, Text: text, Data: data}
}

// ---- reserve ---------------------------------------------------------------------

// Reserve closes the desk enquiry available at a branch with a verified offer
// (rules 6 and 11), queues a hold notification to that pharmacy and marks the
// ticket reserved. Client (owner) only.
func (s *EnquiryService) Reserve(ctx context.Context, a Actor, ticketID string, in ReserveInput) (TicketView, error) {
	if err := clientOnlyErr(a); err != nil {
		return TicketView{}, err
	}
	bid := clean(in.BranchID)
	if bid == "" {
		return TicketView{}, Invalid("branch_id is required: the pharmacy of the best verified offer")
	}
	var out TicketView
	err := s.enquiryTx(ctx, func(q queries.Querier) error {
		t, err := s.ownTicket(ctx, q, a, ticketID)
		if err != nil {
			return err
		}
		if t.Status != queries.TicketOpen {
			return Conflict("this ticket is already " + t.Status)
		}
		e, err := q.GetEnquiryForUpdate(ctx, t.EnquiryID)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if isFinal(e) {
			return Conflict("this ticket's search is closed; open a new ticket")
		}
		st, err := s.loadTicketState(ctx, q, t)
		if err != nil {
			return err
		}
		if !verifiedAt(st, bid) {
			return Conflict("only a pharmacy with a verified offer can be reserved: the rules must match its offer and its package photo must be verified")
		}
		var target queries.EnquiryTarget
		for _, tg := range st.targets {
			if tg.BranchID == bid && tg.Status != queries.TargetCancelled {
				target = tg
			}
		}
		actor := Actor{Type: queries.ActorClient, ID: a.ID}

		// Close available at that branch; queued messages are cancelled like any close.
		for _, m := range st.msgs {
			if m.Status == queries.OutboxQueued {
				if _, err := q.UpdateOutboxMessageStatus(ctx, m.ID, queries.OutboxCancelled, nil); err != nil {
					return FromStore(err, "message")
				}
			}
		}
		u := toUpdate(e)
		u.Status, u.Outcome, u.OutcomeBranchID = queries.EnquiryClosed, queries.OutcomeAvailable, strPtr(bid)
		u.ClosedAt, u.WakeTrigger, u.NextWakeAt = timePtr(s.now()), "", nil
		if _, err := q.UpdateEnquiry(ctx, u); err != nil {
			return FromStore(err, "enquiry")
		}
		if _, err := s.logEvent(ctx, q, e.ID, actor, EvEnquiryClosed,
			map[string]any{"outcome": queries.OutcomeAvailable, "branch_id": bid, "ticket_id": t.ID, "reserved": true}, nil); err != nil {
			return err
		}

		tid := target.ID
		body := fmt.Sprintf("Please hold %s x%d for a DOSYN client until %s.", medLabel(st.med), e.Quantity, t.PickupWindow)
		msg, err := q.CreateOutboxMessage(ctx, queries.CreateOutboxMessageParams{
			EnquiryID: e.ID, TargetID: &tid, Kind: queries.OutboxKindNotification, Body: body,
			SendAfter: s.now().Add(s.cfg.UndoWindow), CreatedByActor: actor.Label(),
		})
		if err != nil {
			return FromStore(err, "message")
		}
		if _, err := s.logEvent(ctx, q, e.ID, actor, EvHoldRequested,
			map[string]any{"message_id": msg.ID, "branch_id": bid, "target_id": tid}, nil); err != nil {
			return err
		}
		updated, err := q.UpdateClientTicketStatus(ctx, t.ID, queries.TicketReserved, &bid)
		if err != nil {
			return FromStore(err, "ticket")
		}
		out = TicketView{ClientTicket: updated, Quantity: e.Quantity}
		return nil
	})
	return out, err
}

// ---- hooks used by the enquiry slice ------------------------------------------------

// isDesk reports a client-desk enquiry.
func isDesk(e queries.Enquiry) bool { return e.RequesterBranchID == queries.DeskBranchID }

// deskCloseCheck enforces amended rule 6 on a client-desk enquiry: closing
// available needs a verified offer at that branch. It also closes the ticket
// for any other outcome. Called inside CloseEnquiry's transaction.
func (s *EnquiryService) deskCloseCheck(ctx context.Context, q queries.Querier, a Actor, e queries.Enquiry, outcome, branchID string) error {
	if !isDesk(e) {
		return nil
	}
	t, err := q.GetClientTicketByEnquiry(ctx, e.ID)
	if errors.Is(err, queries.ErrNotFound) {
		return nil // a desk enquiry opened by staff by hand, not a client ticket
	}
	if err != nil {
		return FromStore(err, "ticket")
	}
	if outcome == queries.OutcomeAvailable {
		if a.IsAgent() {
			return Conflict("a client ticket closes available only when the client reserves the best verified offer; end this run with wait()")
		}
		st, err := s.loadTicketState(ctx, q, t)
		if err != nil {
			return err
		}
		if !verifiedAt(st, branchID) {
			return Conflict("cannot close a client ticket as available: that pharmacy has no verified offer (rules match + verified package photo). Wait for the photo check; the client reserves the best verified offer")
		}
		if t.Status == queries.TicketOpen { // desk staff reserved on the client's behalf
			if _, err := q.UpdateClientTicketStatus(ctx, t.ID, queries.TicketReserved, &branchID); err != nil {
				return FromStore(err, "ticket")
			}
		}
		return nil
	}
	if t.Status == queries.TicketOpen {
		if _, err := q.UpdateClientTicketStatus(ctx, t.ID, queries.TicketClosed, nil); err != nil {
			return FromStore(err, "ticket")
		}
	}
	return nil
}

// deskAutoApprove reports whether agent-proposed branches on a client-desk
// enquiry are all in the ranked candidate list, so Go approves them itself.
func (s *EnquiryService) deskAutoApprove(ctx context.Context, a Actor, e queries.Enquiry, branchIDs []string) (bool, error) {
	if !a.IsAgent() || !isDesk(e) {
		return false, nil
	}
	cands, err := s.Candidates(ctx, a, e.ID, maxCandLimit, 0)
	if err != nil {
		var se *Error
		if errors.As(err, &se) && se.Code == CodeConflict {
			return false, nil
		}
		return false, err
	}
	ranked := map[string]bool{}
	for _, c := range cands {
		ranked[c.BranchID] = true
	}
	for _, b := range branchIDs {
		if !ranked[b] {
			return false, nil
		}
	}
	return len(branchIDs) > 0, nil
}

// ---- rule 8: pharmacies and the agent never see who the client is ---------------

// redactDesk reports whether a must get a redacted view of enquiry e: a
// client-desk enquiry seen by anyone but the desk staff (target pharmacies, the agent).
func redactDesk(a Actor, e queries.Enquiry) bool {
	return isDesk(e) && !(a.Type == queries.ActorStaff && a.BranchID == e.RequesterBranchID)
}

// clientPayloadKeys are dropped from event payloads in a redacted view.
var clientPayloadKeys = []string{"ticket_id", "client_id", "schedule_id"}

// redactDeskEvents hides the client: actor_id of client events becomes "client"
// and ticket/client/schedule ids leave the payloads. For pharmacies
// (dropBodies) the text of notifications to the client is dropped too.
func redactDeskEvents(events []queries.EnquiryEvent, dropBodies bool) []queries.EnquiryEvent {
	out := make([]queries.EnquiryEvent, 0, len(events))
	for _, ev := range events {
		if ev.ActorType == queries.ActorClient {
			ev.ActorID = "client"
		}
		if len(ev.Payload) > 0 {
			var m map[string]any
			if json.Unmarshal(ev.Payload, &m) == nil && m != nil {
				for _, k := range clientPayloadKeys {
					delete(m, k)
				}
				if dropBodies && ev.Type == EvRequesterNotified {
					delete(m, "body")
				}
				if b, err := json.Marshal(m); err == nil {
					ev.Payload = b
				}
			}
		}
		out = append(out, ev)
	}
	return out
}

// redactDeskMessages drops the notifications written to the client (target_id
// NULL) from a pharmacy's view; messages addressed to pharmacies stay.
func redactDeskMessages(msgs []queries.OutboxMessage) []queries.OutboxMessage {
	out := make([]queries.OutboxMessage, 0, len(msgs))
	for _, m := range msgs {
		if m.Kind == queries.OutboxKindNotification && m.TargetID == nil {
			continue
		}
		if strings.HasPrefix(m.CreatedByActor, "client:") {
			m.CreatedByActor = "client"
		}
		out = append(out, m)
	}
	return out
}
