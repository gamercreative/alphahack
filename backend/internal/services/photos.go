package services

// Package photos (CONTRACT rule 10): evidence, not proof. The target pharmacist
// uploads a photo and types the product code and expiry next to it; the agent
// may add its own reading of the same photo. Go decides: the code must be the
// government reference code of the offered product and the expiry must not be
// before the pickup month. Anything unclear waits for a human reviewer.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata" // Asia/Beirut on hosts without a zoneinfo database
	"unicode/utf8"

	"alphahack/internal/queries"
)

const (
	// MaxPhotoBytes is the upload limit for one package photo.
	MaxPhotoBytes = 5 << 20
	// MinAgentReadingConfidence: below this the agent's reading sends the photo to a reviewer.
	MinAgentReadingConfidence = 0.6

	EvPhotoUploaded = "photo_uploaded" // actor staff (target pharmacist)
	EvPhotoChecked  = "photo_checked"  // actor system/rules
	EvPhotoRead     = "photo_read"     // actor agent
	EvPhotoReviewed = "photo_reviewed" // actor staff (reviewer)

	WakePhoto = "photo" // wake trigger: a package photo was uploaded or reviewed

	maxPhotoNoteChars = 300
	maxPhotoCodeChars = 32
)

var photoTypes = map[string]string{"image/jpeg": ".jpg", "image/png": ".png"}

// beirut is the clock pickup windows are read in.
var beirut = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Beirut"); err == nil {
		return loc
	}
	return time.FixedZone("EET", 2*3600)
}()

type UploadPhotoInput struct {
	DeclaredType string
	SniffedType  string
	Data         []byte
	Code         string
	ExpiryMonth  string
}

// PhotoResult is the body of POST /api/v1/targets/{id}/photos and the photo routes.
type PhotoResult struct {
	ID          string `json:"id"`
	TargetID    string `json:"target_id"`
	BranchID    string `json:"branch_id"`
	Status      string `json:"status"`
	GovMatch    bool   `json:"gov_match"`
	Code        string `json:"code"`
	ExpiryMonth string `json:"expiry_month"`
	DecidedBy   string `json:"decided_by"`
	Note        string `json:"note"`
}

func photoResult(p queries.PackagePhoto) PhotoResult {
	return PhotoResult{ID: p.ID, TargetID: p.TargetID, BranchID: p.BranchID, Status: p.Status,
		GovMatch: p.GovMatch, Code: p.TypedCode, ExpiryMonth: p.TypedExpiryMonth, DecidedBy: p.DecidedBy, Note: p.Note}
}

// PhotoSummary is the compact photo row the agent reads in the enquiry detail.
type PhotoSummary struct {
	ID               string   `json:"id"`
	TargetID         string   `json:"target_id"`
	BranchID         string   `json:"branch_id"`
	Status           string   `json:"status"`
	DecidedBy        string   `json:"decided_by"`
	TypedCode        string   `json:"typed_code"`
	TypedExpiryMonth string   `json:"typed_expiry_month"`
	AgentCode        string   `json:"agent_code"`
	AgentExpiryMonth string   `json:"agent_expiry_month"`
	AgentConfidence  *float64 `json:"agent_confidence"`
}

func photoSummaries(ps []queries.PackagePhoto) []PhotoSummary {
	out := make([]PhotoSummary, 0, len(ps))
	for _, p := range ps {
		out = append(out, PhotoSummary{ID: p.ID, TargetID: p.TargetID, BranchID: p.BranchID, Status: p.Status, DecidedBy: p.DecidedBy,
			TypedCode: p.TypedCode, TypedExpiryMonth: p.TypedExpiryMonth, AgentCode: p.AgentCode,
			AgentExpiryMonth: p.AgentExpiryMonth, AgentConfidence: p.AgentConfidence})
	}
	return out
}

// pickupMonth is the "YYYY-MM" of the pickup day: today for same-day windows,
// tomorrow or in two days for the others, in Beirut time.
func pickupMonth(now time.Time, window string) string {
	days := 0
	switch strings.ToLower(window) {
	case "tomorrow morning", "next morning":
		days = 1
	case "within 2 days":
		days = 2
	}
	return now.In(beirut).AddDate(0, 0, days).Format("2006-01")
}

func normCode(s string) string { return strings.Join(strings.Fields(s), "") }

// photoVerdict is the rules' decision for one typed (code, expiry).
type photoVerdict struct {
	Status   string
	GovMatch bool
	Note     string
}

func verdictNote(reason string) string { return reason + " " + photoDisclaimer }

// decidePhoto applies rule 10 to a typed code and expiry. expected is the gov
// code of the offered product ("" when none is known); known reports whether a
// code exists in the government reference.
func decidePhoto(code, expiry, expected, pickup string, known bool) photoVerdict {
	code, expiry = normCode(code), normExpiryMonth(expiry)
	switch {
	case code == "":
		return photoVerdict{queries.PhotoFlagged, false, verdictNote("No product code was typed; a pharmacist on our team checks the photo.")}
	case !known:
		return photoVerdict{queries.PhotoFlagged, false, verdictNote("The typed code is not in the government reference; a pharmacist on our team checks it.")}
	case expected == "":
		return photoVerdict{queries.PhotoFlagged, true, verdictNote("No government reference row is known for the offered product; a pharmacist on our team checks it.")}
	case code != expected:
		return photoVerdict{queries.PhotoMismatch, false, verdictNote("The typed code belongs to a different product in the government reference.")}
	case expiry == "":
		return photoVerdict{queries.PhotoFlagged, true, verdictNote("No readable expiry (YYYY-MM) was typed; a pharmacist on our team checks it.")}
	case expiry < pickup:
		return photoVerdict{queries.PhotoMismatch, true, verdictNote(fmt.Sprintf("The box expires %s, before the pickup month %s.", expiry, pickup))}
	}
	return photoVerdict{queries.PhotoVerified, true, verdictNote("Code matches the government reference and the expiry is after pickup.")}
}

// expectedGovCode: the rules-accepted equivalent's gov code when the latest offer
// of the target is one, else the reference row of the enquiry medication.
func (s *EnquiryService) expectedGovCode(ctx context.Context, q queries.Querier, e queries.Enquiry, targetID string) (string, error) {
	offers, err := q.ListTargetOffers(ctx, e.ID)
	if err != nil {
		return "", FromStore(err, "offer")
	}
	var latest *queries.TargetOffer
	for i := range offers {
		if offers[i].TargetID == targetID {
			latest = &offers[i]
		}
	}
	if latest != nil {
		events, err := q.ListEnquiryEvents(ctx, e.ID, 0)
		if err != nil {
			return "", FromStore(err, "event")
		}
		x := offerExtrasByID(events)[latest.ID]
		if x.DoseCheck != nil && x.DoseCheck.Accepted && x.DoseCheck.GovCode != "" {
			return x.DoseCheck.GovCode, nil
		}
		if x.GovCode != "" && x.Equivalent != nil && *x.Equivalent {
			return x.GovCode, nil
		}
	}
	if e.MedicationID == nil {
		return "", nil
	}
	m, err := q.GetMedication(ctx, *e.MedicationID)
	if err != nil {
		return "", FromStore(err, "medication")
	}
	rows, err := q.ListGovProducts(ctx, queries.ListGovProductsParams{
		Ingredient: m.ActiveIngredient, DosageForm: m.DosageForm, Limit: MaxLimit,
	})
	if err != nil {
		return "", FromStore(err, "reference product")
	}
	want, wantUnit, okWant := doseStrength(m.Strength)
	best := ""
	for _, g := range rows {
		v, u, ok := doseStrength(g.Strength)
		if !okWant || !ok || u != wantUnit || math.Abs(v-want) > 1e-9 {
			continue
		}
		if strings.EqualFold(g.BrandName, m.BrandName) {
			return g.Code, nil
		}
		if best == "" {
			best = g.Code
		}
	}
	return best, nil
}

func (s *EnquiryService) checkPhoto(ctx context.Context, q queries.Querier, e queries.Enquiry, t queries.EnquiryTarget,
	code, expiry string) (photoVerdict, error) {
	expected, err := s.expectedGovCode(ctx, q, e, t.ID)
	if err != nil {
		return photoVerdict{}, err
	}
	known := false
	if c := normCode(code); c != "" {
		_, err := q.GetGovProduct(ctx, c)
		switch {
		case err == nil:
			known = true
		case !errors.Is(err, queries.ErrNotFound):
			return photoVerdict{}, FromStore(err, "reference product")
		}
	}
	window := ""
	if ticket, err := q.GetClientTicketByEnquiry(ctx, e.ID); err == nil {
		window = ticket.PickupWindow
	}
	return decidePhoto(code, expiry, expected, pickupMonth(s.now(), window), known), nil
}

func (s *EnquiryService) logPhotoChecked(ctx context.Context, q queries.Querier, e queries.Enquiry, p queries.PackagePhoto) error {
	_, err := s.logEvent(ctx, q, e.ID, RulesActor, EvPhotoChecked, map[string]any{
		"photo_id": p.ID, "target_id": p.TargetID, "branch_id": p.BranchID,
		"status": p.Status, "gov_match": p.GovMatch, "note": p.Note,
	}, nil)
	return err
}

// wakeForPhoto sets the photo wake trigger on agent-handled open enquiries.
func (s *EnquiryService) wakeForPhoto(ctx context.Context, q queries.Querier, e queries.Enquiry) error {
	if e.HandledBy != "agent" || isFinal(e) {
		return nil
	}
	u := toUpdate(e)
	u.WakeTrigger = WakePhoto
	_, err := q.UpdateEnquiry(ctx, u)
	return FromStore(err, "enquiry")
}

// UploadPhoto stores a package photo for a target (staff at that target branch
// only) and records the rules' verdict on the typed code and expiry.
func (s *EnquiryService) UploadPhoto(ctx context.Context, a Actor, targetID string, in UploadPhotoInput) (PhotoResult, error) {
	if a.IsAgent() {
		return PhotoResult{}, AgentForbidden()
	}
	if a.IsClient() {
		return PhotoResult{}, Forbidden("only the pharmacist at the target pharmacy can upload a package photo")
	}
	if len(in.Data) == 0 {
		return PhotoResult{}, Invalid(`the photo is empty; attach a JPEG or PNG in the multipart field "photo"`)
	}
	if len(in.Data) > MaxPhotoBytes {
		return PhotoResult{}, Invalid("the photo is larger than 5 MB; upload a smaller one")
	}
	sniffed := mediaType(in.SniffedType)
	ext, ok := photoTypes[sniffed]
	if !ok {
		return PhotoResult{}, Invalid("only JPEG or PNG photos are accepted; this file's content looks like %s", sniffed)
	}
	if declared := mediaType(in.DeclaredType); declared != "" && declared != "application/octet-stream" && declared != sniffed {
		return PhotoResult{}, Invalid("the photo is sent as %s but its content is %s; send the real file", declared, sniffed)
	}
	code := normCode(in.Code)
	if utf8.RuneCountInString(code) > maxPhotoCodeChars {
		return PhotoResult{}, Invalid("code must be at most %d characters (the 13-digit code on the box)", maxPhotoCodeChars)
	}
	expiry := normExpiryMonth(in.ExpiryMonth)
	if strings.TrimSpace(in.ExpiryMonth) != "" && expiry == "" {
		return PhotoResult{}, Invalid("expiry_month must look like 2027-08 or 08/2027")
	}

	id, err := newUUID()
	if err != nil {
		return PhotoResult{}, Internal(err)
	}
	dir := filepath.Join(s.cfg.UploadDir, "photos")
	path := filepath.Join(dir, id+ext)
	written := false
	defer func() {
		if !written {
			_ = os.Remove(path)
		}
	}()

	var out queries.PackagePhoto
	err = s.enquiryTx(ctx, func(q queries.Querier) error {
		t, err := q.GetEnquiryTarget(ctx, targetID)
		if err != nil {
			return FromStore(err, "target")
		}
		if a.BranchID != t.BranchID {
			return Forbidden("only the pharmacist at the target pharmacy can upload a package photo (rule 2)")
		}
		e, err := q.GetEnquiryForUpdate(ctx, t.EnquiryID)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if isFinal(e) {
			return Conflict("this enquiry is closed")
		}
		switch t.Status {
		case queries.TargetAvailable, queries.TargetAlternativeOffered:
		default:
			return Conflict("reply available or alternative first; a photo backs an offer")
		}
		v, err := s.checkPhoto(ctx, q, e, t, code, expiry)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return Internal(err)
		}
		if err := os.WriteFile(path, in.Data, 0o640); err != nil {
			return Internal(err)
		}
		out, err = q.CreatePackagePhoto(ctx, queries.CreatePackagePhotoParams{
			ID: id, TargetID: t.ID, BranchID: t.BranchID, StoragePath: path, ContentType: sniffed,
			SizeBytes: int64(len(in.Data)), TypedCode: code, TypedExpiryMonth: expiry,
			Status: v.Status, GovMatch: v.GovMatch, Note: v.Note,
		})
		if err != nil {
			return FromStore(err, "photo")
		}
		if _, err := s.logEvent(ctx, q, e.ID, a, EvPhotoUploaded, map[string]any{
			"photo_id": out.ID, "target_id": t.ID, "branch_id": t.BranchID, "code": code, "expiry_month": expiry,
		}, nil); err != nil {
			return err
		}
		if err := s.logPhotoChecked(ctx, q, e, out); err != nil {
			return err
		}
		return s.wakeForPhoto(ctx, q, e)
	})
	if err != nil {
		return PhotoResult{}, err
	}
	written = true
	return photoResult(out), nil
}

type AgentReadingInput struct {
	Code        string   `json:"code"`
	ExpiryMonth string   `json:"expiry_month"`
	Confidence  *float64 `json:"confidence"`
}

// RecordAgentReading stores the agent's reading of a photo. Go then decides:
// agreement with the typed values + the rules -> their verdict; disagreement or
// low confidence -> flagged for a reviewer.
func (s *EnquiryService) RecordAgentReading(ctx context.Context, a Actor, photoID string, in AgentReadingInput) (PhotoResult, error) {
	if !a.IsAgent() {
		return PhotoResult{}, Forbidden("this route is for the Wassit agent only")
	}
	if in.Confidence == nil || math.IsNaN(*in.Confidence) || *in.Confidence < 0 || *in.Confidence > 1 {
		return PhotoResult{}, Invalid("confidence must be a number from 0 to 1 (how sure you are of the reading)")
	}
	code := normCode(in.Code)
	if utf8.RuneCountInString(code) > maxPhotoCodeChars {
		return PhotoResult{}, Invalid("code must be at most %d characters", maxPhotoCodeChars)
	}
	expiry := normExpiryMonth(in.ExpiryMonth)
	if strings.TrimSpace(in.ExpiryMonth) != "" && expiry == "" {
		return PhotoResult{}, Invalid("expiry_month must look like 2027-08 or 08/2027; leave it empty if you cannot read it")
	}
	var out queries.PackagePhoto
	err := s.enquiryTx(ctx, func(q queries.Querier) error {
		p, err := q.GetPackagePhoto(ctx, photoID)
		if err != nil {
			return FromStore(err, "photo")
		}
		t, err := q.GetEnquiryTarget(ctx, p.TargetID)
		if err != nil {
			return FromStore(err, "target")
		}
		e, err := q.GetEnquiryForUpdate(ctx, t.EnquiryID)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		if p.DecidedBy == "human" {
			return Conflict("a reviewer already decided this photo; nothing to read")
		}
		v, err := s.checkPhoto(ctx, q, e, t, p.TypedCode, p.TypedExpiryMonth)
		if err != nil {
			return err
		}
		switch {
		case *in.Confidence < MinAgentReadingConfidence:
			v = photoVerdict{queries.PhotoFlagged, v.GovMatch, verdictNote("The agent could not read the photo with enough confidence; a pharmacist on our team checks it.")}
		case code != p.TypedCode || expiry != p.TypedExpiryMonth:
			v = photoVerdict{queries.PhotoFlagged, v.GovMatch, verdictNote("The agent's reading does not agree with the typed code or expiry; a pharmacist on our team checks it.")}
		}
		if _, err := s.logEvent(ctx, q, e.ID, a, EvPhotoRead, map[string]any{
			"photo_id": p.ID, "code": code, "expiry_month": expiry, "confidence": *in.Confidence,
		}, nil); err != nil {
			return err
		}
		out, err = q.RecordPhotoAgentReading(ctx, queries.RecordPhotoAgentReadingParams{
			ID: p.ID, AgentCode: code, AgentExpiryMonth: expiry, AgentConfidence: *in.Confidence,
			Status: v.Status, GovMatch: v.GovMatch, Note: v.Note,
		})
		if errors.Is(err, queries.ErrNotFound) {
			return Conflict("a reviewer already decided this photo; nothing to read")
		}
		if err != nil {
			return FromStore(err, "photo")
		}
		return s.logPhotoChecked(ctx, q, e, out)
	})
	if err != nil {
		return PhotoResult{}, err
	}
	return photoResult(out), nil
}

type ReviewPhotoInput struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

// ReviewPhoto is the human decision on a photo (staff with role reviewer only).
func (s *EnquiryService) ReviewPhoto(ctx context.Context, a Actor, photoID string, in ReviewPhotoInput) (PhotoResult, error) {
	if a.IsAgent() {
		return PhotoResult{}, AgentForbidden()
	}
	if a.IsClient() {
		return PhotoResult{}, Forbidden("only a reviewer can review a photo")
	}
	staff, err := s.store.GetStaff(ctx, a.ID)
	if err != nil {
		return PhotoResult{}, FromStore(err, "staff")
	}
	if staff.Role != "reviewer" {
		return PhotoResult{}, Forbidden("only staff with the reviewer role can review a photo")
	}
	status := clean(in.Status)
	if status != queries.PhotoVerified && status != queries.PhotoRejected {
		return PhotoResult{}, Invalid("status must be verified or rejected")
	}
	note := clean(in.Note)
	if utf8.RuneCountInString(note) > maxPhotoNoteChars {
		return PhotoResult{}, Invalid("note must be at most %d characters", maxPhotoNoteChars)
	}
	if !strings.Contains(note, photoDisclaimer) {
		note = strings.TrimSpace(note + " " + photoDisclaimer)
	}
	var out queries.PackagePhoto
	err = s.enquiryTx(ctx, func(q queries.Querier) error {
		p, err := q.GetPackagePhoto(ctx, photoID)
		if err != nil {
			return FromStore(err, "photo")
		}
		t, err := q.GetEnquiryTarget(ctx, p.TargetID)
		if err != nil {
			return FromStore(err, "target")
		}
		e, err := q.GetEnquiryForUpdate(ctx, t.EnquiryID)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if isFinal(e) {
			return Conflict("this enquiry is closed")
		}
		out, err = q.ReviewPackagePhoto(ctx, queries.ReviewPackagePhotoParams{
			ID: p.ID, Status: status, Note: note, ReviewedByStaffID: a.ID,
		})
		if err != nil {
			return FromStore(err, "photo")
		}
		if _, err := s.logEvent(ctx, q, e.ID, a, EvPhotoReviewed, map[string]any{
			"photo_id": p.ID, "target_id": p.TargetID, "branch_id": p.BranchID, "status": status, "note": note,
		}, nil); err != nil {
			return err
		}
		return s.wakeForPhoto(ctx, q, e)
	})
	if err != nil {
		return PhotoResult{}, err
	}
	return photoResult(out), nil
}

// PhotoFile is the stored image of one photo.
type PhotoFile struct {
	ContentType string
	Data        []byte
}

// GetPhotoFile returns the image to the target pharmacy, the requester pharmacy,
// a reviewer, the agent (agent-handled enquiries) or the ticket's client.
func (s *EnquiryService) GetPhotoFile(ctx context.Context, a Actor, photoID string) (PhotoFile, error) {
	p, err := s.store.GetPackagePhoto(ctx, photoID)
	if err != nil {
		return PhotoFile{}, FromStore(err, "photo")
	}
	t, err := s.store.GetEnquiryTarget(ctx, p.TargetID)
	if err != nil {
		return PhotoFile{}, FromStore(err, "target")
	}
	e, err := s.store.GetEnquiry(ctx, t.EnquiryID)
	if err != nil {
		return PhotoFile{}, FromStore(err, "enquiry")
	}
	allowed := false
	switch {
	case a.IsAgent():
		allowed = e.HandledBy == "agent"
	case a.IsClient():
		tk, err := s.store.GetClientTicketByEnquiry(ctx, e.ID)
		allowed = err == nil && tk.ClientID == a.ID
	default:
		allowed = a.BranchID == p.BranchID || a.BranchID == e.RequesterBranchID
		if !allowed {
			if st, err := s.store.GetStaff(ctx, a.ID); err == nil && st.Role == "reviewer" {
				allowed = true
			}
		}
	}
	if !allowed {
		return PhotoFile{}, NotFound("photo")
	}
	data, err := os.ReadFile(p.StoragePath)
	if err != nil {
		return PhotoFile{}, NotFound("photo file")
	}
	return PhotoFile{ContentType: p.ContentType, Data: data}, nil
}

// wakePhotoID is the photo that set wake_trigger 'photo' (the latest photo
// upload or review event), or nil when the enquiry is not woken for a photo.
func wakePhotoID(e queries.Enquiry, events []queries.EnquiryEvent) *string {
	if e.WakeTrigger != WakePhoto {
		return nil
	}
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type == EvPhotoUploaded || ev.Type == EvPhotoReviewed {
			if id := payloadString(payloadMap(ev.Payload), "photo_id"); id != "" {
				return &id
			}
		}
	}
	return nil
}
