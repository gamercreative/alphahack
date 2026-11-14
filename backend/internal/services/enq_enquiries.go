package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
	"unicode/utf8"

	"alphahack/internal/queries"
)

// Enquiries slice: create, read, list, medication, candidates, targets, close.
// See CONTRACT.md (Product rules 1, 6; Endpoints; Event types).

const (
	maxRawRequestLen   = 1000
	maxEnquiryQuantity = 1000
	maxTargetsPerCall  = 10
	defaultCandLimit   = 5
	maxCandLimit       = 20
	defaultRadiusKm    = 15.0
)

// enquiryTx runs fn in a transaction. fn returns *Error values (or passes store
// errors through FromStore), so they are returned as-is; only begin/commit
// failures are wrapped here.
func (s *EnquiryService) enquiryTx(ctx context.Context, fn func(q queries.Querier) error) error {
	err := s.store.InTx(ctx, fn)
	var se *Error
	if err == nil || errors.As(err, &se) {
		return err
	}
	return FromStore(err, "enquiry")
}

// ---- create ----------------------------------------------------------------

type CreateEnquiryInput struct {
	RawRequest   string  `json:"raw_request"`
	MedicationID *string `json:"medication_id"`
	Quantity     int     `json:"quantity"`
	HandledBy    string  `json:"handled_by"`
}

func (s *EnquiryService) CreateEnquiry(ctx context.Context, a Actor, in CreateEnquiryInput) (queries.Enquiry, error) {
	raw := clean(in.RawRequest)
	if n := utf8.RuneCountInString(raw); n < 1 || n > maxRawRequestLen {
		return queries.Enquiry{}, Invalid("raw_request must be 1 to %d characters", maxRawRequestLen)
	}
	if in.Quantity < 1 || in.Quantity > maxEnquiryQuantity {
		return queries.Enquiry{}, Invalid("quantity must be between 1 and %d", maxEnquiryQuantity)
	}
	handledBy := clean(in.HandledBy)
	if handledBy == "" {
		handledBy = "manual"
	}
	if handledBy != "manual" && handledBy != "agent" {
		return queries.Enquiry{}, Invalid("handled_by must be manual or agent")
	}
	var medID *string
	if in.MedicationID != nil {
		if m := clean(*in.MedicationID); m != "" {
			medID = &m
		}
	}
	wake := ""
	if handledBy == "agent" {
		wake = WakeCreated
	}

	var out queries.Enquiry
	err := s.enquiryTx(ctx, func(q queries.Querier) error {
		if medID != nil {
			if _, err := q.GetMedication(ctx, *medID); err != nil {
				return FromStore(err, "medication")
			}
		}
		e, err := q.CreateEnquiry(ctx, queries.CreateEnquiryParams{
			RequesterBranchID: a.BranchID,
			RequesterStaffID:  a.ID,
			RawRequest:        raw,
			MedicationID:      medID,
			Quantity:          in.Quantity,
			Status:            queries.EnquiryOpen,
			HandledBy:         handledBy,
			WakeTrigger:       wake,
		})
		if err != nil {
			return FromStore(err, "enquiry")
		}
		payload := map[string]any{
			"raw_request":   raw,
			"quantity":      in.Quantity,
			"handled_by":    handledBy,
			"medication_id": medID,
		}
		if _, err := s.logEvent(ctx, q, e.ID, a, EvEnquiryCreated, payload, nil); err != nil {
			return err
		}
		out = e
		return nil
	})
	if err != nil {
		return queries.Enquiry{}, err
	}
	return out, nil
}

// ---- detail ----------------------------------------------------------------

// MedSummary is the catalogue view the agent and staff need. Contraindication
// fields are out of scope and never included.
type MedSummary struct {
	ID               string `json:"id"`
	BrandName        string `json:"brand_name"`
	ActiveIngredient string `json:"active_ingredient"`
	DosageForm       string `json:"dosage_form"`
	Strength         string `json:"strength"`
}

type TargetView struct {
	queries.EnquiryTarget
	BranchName string `json:"branch_name"`
}

type EnquiryDetail struct {
	Enquiry    queries.Enquiry           `json:"enquiry"`
	Medication *MedSummary               `json:"medication"`
	Targets    []TargetView              `json:"targets"`
	Messages   []queries.OutboxMessage   `json:"messages"`
	Events     []queries.EnquiryEvent    `json:"events"`
	Runs       []queries.EnquiryAgentRun `json:"runs"`
	HITLCount  int                       `json:"hitl_count"`
}

func isHITLEvent(typ string) bool {
	for _, t := range HITLEventTypes {
		if t == typ {
			return true
		}
	}
	return false
}

func (s *EnquiryService) GetEnquiryDetail(ctx context.Context, a Actor, id string) (EnquiryDetail, error) {
	e, err := s.store.GetEnquiry(ctx, id)
	if err != nil {
		return EnquiryDetail{}, FromStore(err, "enquiry")
	}
	targets, err := s.store.ListEnquiryTargets(ctx, e.ID)
	if err != nil {
		return EnquiryDetail{}, FromStore(err, "enquiry")
	}

	var allowed bool
	if a.IsAgent() {
		allowed = e.HandledBy == "agent"
	} else {
		allowed = a.BranchID == e.RequesterBranchID
		for _, t := range targets {
			if allowed {
				break
			}
			allowed = t.BranchID == a.BranchID
		}
	}
	if !allowed {
		return EnquiryDetail{}, Forbidden("you cannot view this enquiry")
	}

	var med *MedSummary
	if e.MedicationID != nil {
		m, err := s.store.GetMedication(ctx, *e.MedicationID)
		if err != nil {
			return EnquiryDetail{}, FromStore(err, "medication")
		}
		med = &MedSummary{
			ID:               m.ID,
			BrandName:        m.BrandName,
			ActiveIngredient: m.ActiveIngredient,
			DosageForm:       m.DosageForm,
			Strength:         m.Strength,
		}
	}

	views := make([]TargetView, 0, len(targets))
	for _, t := range targets {
		b, err := s.store.GetBranch(ctx, t.BranchID)
		if err != nil {
			return EnquiryDetail{}, FromStore(err, "branch")
		}
		views = append(views, TargetView{EnquiryTarget: t, BranchName: b.Name})
	}

	messages, err := s.store.ListOutboxMessages(ctx, e.ID)
	if err != nil {
		return EnquiryDetail{}, FromStore(err, "outbox")
	}
	events, err := s.store.ListEnquiryEvents(ctx, e.ID, 0)
	if err != nil {
		return EnquiryDetail{}, FromStore(err, "enquiry_event")
	}
	runs, err := s.store.ListEnquiryAgentRuns(ctx, e.ID)
	if err != nil {
		return EnquiryDetail{}, FromStore(err, "agent_run")
	}

	hitl := 0
	for _, ev := range events {
		if ev.UndoneAt == nil && isHITLEvent(ev.Type) {
			hitl++
		}
	}
	if messages == nil {
		messages = []queries.OutboxMessage{}
	}
	if events == nil {
		events = []queries.EnquiryEvent{}
	}
	if runs == nil {
		runs = []queries.EnquiryAgentRun{}
	}

	return EnquiryDetail{
		Enquiry:    e,
		Medication: med,
		Targets:    views,
		Messages:   messages,
		Events:     events,
		Runs:       runs,
		HITLCount:  hitl,
	}, nil
}

// ---- list ------------------------------------------------------------------

func (s *EnquiryService) ListEnquiries(ctx context.Context, a Actor, scope, status string, page Page) ([]queries.Enquiry, error) {
	page = page.Normalize()
	var (
		list []queries.Enquiry
		err  error
	)
	switch scope {
	case "", "outgoing":
		list, err = s.store.ListOutgoingEnquiries(ctx, a.BranchID, status, page.Limit, page.Offset)
	case "incoming":
		list, err = s.store.ListIncomingEnquiries(ctx, a.BranchID, status, page.Limit, page.Offset)
	default:
		return nil, Invalid("scope must be outgoing or incoming")
	}
	if err != nil {
		return nil, FromStore(err, "enquiry")
	}
	if list == nil {
		list = []queries.Enquiry{}
	}
	return list, nil
}

// ---- medication ------------------------------------------------------------

func (s *EnquiryService) SetMedication(ctx context.Context, a Actor, id, medicationID string) (queries.Enquiry, error) {
	medicationID = clean(medicationID)
	var out queries.Enquiry
	err := s.enquiryTx(ctx, func(q queries.Querier) error {
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
		m, err := q.GetMedication(ctx, medicationID)
		if err != nil {
			return FromStore(err, "medication")
		}
		prev := e.MedicationID
		u := toUpdate(e)
		u.MedicationID = strPtr(m.ID)
		updated, err := q.UpdateEnquiry(ctx, u)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		payload := map[string]any{
			"medication_id": m.ID,
			"brand_name":    m.BrandName,
			"strength":      m.Strength,
		}
		if _, err := s.logEvent(ctx, q, e.ID, a, EvMedicationSet, payload,
			&Inverse{Op: "set_medication", MedicationID: prev}); err != nil {
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

// ---- candidates ------------------------------------------------------------

type Candidate struct {
	BranchID         string     `json:"branch_id"`
	Name             string     `json:"name"`
	City             string     `json:"city"`
	DistanceKm       float64    `json:"distance_km"`
	ReportedQuantity int        `json:"reported_quantity"`
	HasReportedStock bool       `json:"has_reported_stock"`
	StockUpdatedAt   *time.Time `json:"stock_updated_at"`
}

// haversineKm returns the great-circle distance in km between two lat/lng points.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0
	const rad = math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusKm * math.Asin(math.Min(1, math.Sqrt(h)))
}

func roundKm(d float64) float64 { return math.Round(d*10) / 10 }

// newerStockFirst orders stock timestamps newest first; nil (unknown) sorts last.
func newerStockFirst(x, y *time.Time) bool {
	if x == nil {
		return false
	}
	if y == nil {
		return true
	}
	return x.After(*y)
}

func (s *EnquiryService) Candidates(ctx context.Context, a Actor, id string, limit int, radiusKm float64) ([]Candidate, error) {
	if limit <= 0 {
		limit = defaultCandLimit
	}
	if limit > maxCandLimit {
		limit = maxCandLimit
	}
	if radiusKm <= 0 {
		radiusKm = defaultRadiusKm
	}

	e, err := s.store.GetEnquiry(ctx, id)
	if err != nil {
		return nil, FromStore(err, "enquiry")
	}
	if err := canAct(a, e); err != nil {
		return nil, err
	}
	if e.MedicationID == nil {
		return nil, Conflict("set the medication first: search_medications, then set_medication")
	}
	med := *e.MedicationID

	// The requester's own coordinates: its row in the candidate list (nothing is
	// excluded here, so the requester row is present when it has coordinates).
	all, err := s.store.ListCandidateBranches(ctx, med, "__none__")
	if err != nil {
		return nil, FromStore(err, "branch")
	}
	var reqLat, reqLng float64
	hasReq := false
	for _, c := range all {
		if c.BranchID == e.RequesterBranchID && c.Lat != nil && c.Lng != nil {
			reqLat, reqLng, hasReq = *c.Lat, *c.Lng, true
		}
	}

	targets, err := s.store.ListEnquiryTargets(ctx, e.ID)
	if err != nil {
		return nil, FromStore(err, "target")
	}
	taken := map[string]bool{}
	for _, t := range targets {
		if t.Status != queries.TargetCancelled {
			taken[t.BranchID] = true
		}
	}

	rows, err := s.store.ListCandidateBranches(ctx, med, e.RequesterBranchID)
	if err != nil {
		return nil, FromStore(err, "branch")
	}
	out := []Candidate{}
	for _, c := range rows {
		if taken[c.BranchID] || c.Lat == nil || c.Lng == nil {
			continue
		}
		dist := 0.0
		if hasReq {
			dist = haversineKm(reqLat, reqLng, *c.Lat, *c.Lng)
			if dist > radiusKm {
				continue
			}
		}
		dist = roundKm(dist)
		out = append(out, Candidate{
			BranchID:         c.BranchID,
			Name:             c.Name,
			City:             c.City,
			DistanceKm:       dist,
			ReportedQuantity: c.AvailableQuantity,
			HasReportedStock: c.AvailableQuantity >= e.Quantity,
			StockUpdatedAt:   c.StockUpdatedAt,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		x, y := out[i], out[j]
		if x.HasReportedStock != y.HasReportedStock {
			return x.HasReportedStock
		}
		if x.DistanceKm != y.DistanceKm {
			return x.DistanceKm < y.DistanceKm
		}
		return newerStockFirst(x.StockUpdatedAt, y.StockUpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---- targets ---------------------------------------------------------------

// activeTargetBranches returns the branch ids that have a non-cancelled target.
func activeTargetBranches(targets []queries.EnquiryTarget) map[string]bool {
	m := map[string]bool{}
	for _, t := range targets {
		if t.Status != queries.TargetCancelled {
			m[t.BranchID] = true
		}
	}
	return m
}

func (s *EnquiryService) AddTargets(ctx context.Context, a Actor, id string, branchIDs []string) ([]queries.EnquiryTarget, error) {
	ids := make([]string, 0, len(branchIDs))
	seen := map[string]bool{}
	for _, raw := range branchIDs {
		b := clean(raw)
		if b == "" {
			return nil, Invalid("branch_ids must not contain empty ids")
		}
		if seen[b] {
			continue
		}
		seen[b] = true
		ids = append(ids, b)
	}
	if len(ids) < 1 || len(ids) > maxTargetsPerCall {
		return nil, Invalid("branch_ids must list 1 to %d pharmacies", maxTargetsPerCall)
	}

	var out []queries.EnquiryTarget
	err := s.enquiryTx(ctx, func(q queries.Querier) error {
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
		existing, err := q.ListEnquiryTargets(ctx, e.ID)
		if err != nil {
			return FromStore(err, "target")
		}
		taken := activeTargetBranches(existing)

		status := queries.TargetApproved
		if a.IsAgent() {
			status = queries.TargetProposed
		}

		created := make([]queries.EnquiryTarget, 0, len(ids))
		for _, bid := range ids {
			b, err := q.GetBranch(ctx, bid)
			if err != nil {
				return FromStore(err, "branch")
			}
			if bid == e.RequesterBranchID {
				return Invalid("cannot target your own pharmacy")
			}
			if taken[bid] {
				return Conflict(fmt.Sprintf("pharmacy %s is already a target", b.Name))
			}
			t, err := q.CreateEnquiryTarget(ctx, queries.CreateEnquiryTargetParams{
				EnquiryID: e.ID,
				BranchID:  bid,
				Status:    status,
			})
			if err != nil {
				return FromStore(err, "target")
			}
			created = append(created, t)
		}

		if a.IsAgent() {
			u := toUpdate(e)
			u.Status = queries.EnquiryAwaitingApproval
			if _, err := q.UpdateEnquiry(ctx, u); err != nil {
				return FromStore(err, "enquiry")
			}
		}

		targetIDs := make([]string, 0, len(created))
		for _, t := range created {
			targetIDs = append(targetIDs, t.ID)
		}
		typ := EvTargetsAdded
		if a.IsAgent() {
			typ = EvTargetsProposed
		}
		payload := map[string]any{"branch_ids": ids, "target_ids": targetIDs}
		if _, err := s.logEvent(ctx, q, e.ID, a, typ, payload,
			&Inverse{Op: "delete_targets", TargetIDs: targetIDs}); err != nil {
			return err
		}
		out = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---- close -----------------------------------------------------------------

// CloseEnquiry applies product rule 6. Closing rules are enforced here, not in the model.
func (s *EnquiryService) CloseEnquiry(ctx context.Context, a Actor, id, outcome string, branchID *string) (queries.Enquiry, error) {
	outcome = clean(outcome)
	var out queries.Enquiry
	err := s.enquiryTx(ctx, func(q queries.Querier) error {
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
		targets, err := q.ListEnquiryTargets(ctx, e.ID)
		if err != nil {
			return FromStore(err, "target")
		}
		considered := make([]queries.EnquiryTarget, 0, len(targets))
		for _, t := range targets {
			if t.Status != queries.TargetCancelled {
				considered = append(considered, t)
			}
		}

		prev := e.Status
		u := toUpdate(e)
		u.Outcome = outcome
		u.OutcomeBranchID = nil

		switch outcome {
		case queries.OutcomeAvailable:
			bid := ""
			if branchID != nil {
				bid = clean(*branchID)
			}
			if bid == "" {
				return Invalid("branch_id is required to close as available")
			}
			found := false
			for _, t := range considered {
				if t.BranchID == bid && t.Status == queries.TargetAvailable {
					found = true
					break
				}
			}
			if !found {
				return Conflict("cannot close as available: no contacted pharmacy has replied available. Wait for replies or close as unable_to_confirm")
			}
			u.OutcomeBranchID = strPtr(bid)
			u.Status = queries.EnquiryClosed
		case queries.OutcomeUnavailable:
			all := len(considered) > 0
			for _, t := range considered {
				if t.Status != queries.TargetUnavailable {
					all = false
					break
				}
			}
			if !all {
				return Conflict("cannot close as unavailable: every contacted pharmacy must reply unavailable; no_response or alternative_offered is not unavailable. Close as unable_to_confirm")
			}
			u.Status = queries.EnquiryClosed
		case queries.OutcomeUnableToConfirm:
			// The agent may not give up on a pharmacy that is still pending: it must follow up and
			// mark no_response first (a human pharmacist may close early).
			if a.IsAgent() {
				for _, t := range considered {
					if t.Status == queries.TargetSent || t.Status == queries.TargetApproved || t.Status == queries.TargetProposed {
						return Conflict("cannot close yet: a contacted pharmacy has not answered. Wait, follow_up once, then mark_no_response before closing as unable_to_confirm")
					}
				}
			}
			u.Status = queries.EnquiryClosed
		case queries.OutcomeCancelled:
			if a.IsAgent() {
				return Forbidden("only the requester can cancel")
			}
			u.Status = queries.EnquiryCancelled
		default:
			return Invalid("outcome must be available, unavailable, unable_to_confirm or cancelled")
		}

		now := s.now()
		u.ClosedAt = timePtr(now)
		u.WakeTrigger = ""
		u.NextWakeAt = nil

		updated, err := q.UpdateEnquiry(ctx, u)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		payload := map[string]any{"outcome": outcome, "branch_id": u.OutcomeBranchID}
		if _, err := s.logEvent(ctx, q, e.ID, a, EvEnquiryClosed, payload,
			&Inverse{Op: "reopen", Status: prev}); err != nil {
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
