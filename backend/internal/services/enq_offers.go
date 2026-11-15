package services

// Structured offers: the agent (or requester staff) reads a pharmacist's free-text
// reply into fields, each backed by an exact quote. The rules below decide; the
// model never does. An offer never changes a target's status and never closes an
// enquiry (CONTRACT rules 1, 2 and 6 are unchanged).

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"alphahack/internal/queries"
)

// Event types for offers (enquiry_events.type).
const (
	EvOfferExtracted = "offer_extracted" // actor: agent or staff (who read the reply)
	EvOfferDecided   = "offer_decided"   // actor: system "rules" (who decided)
)

// RulesActor labels decisions made by the Go offer rules.
var RulesActor = Actor{Type: queries.ActorSystem, ID: "rules"}

// MinOfferConfidence: below this the offer goes to a human.
const MinOfferConfidence = 0.7

const (
	maxOfferQuoteChars    = 200
	maxOfferStrengthChars = 40
	maxOfferPickupChars   = 80
	maxOfferCurrencyChars = 8
)

// OfferInput is the body of POST /api/v1/targets/{id}/offers. Every field that is
// set needs evidence[field] = an exact quote from the pharmacist's reply. Evidence
// keys: in_stock, strength, quantity, price, pickup_by, payment, needs_prescription.
type OfferInput struct {
	InStock           *bool             `json:"in_stock"`
	Strength          string            `json:"strength"`
	Quantity          *int              `json:"quantity"`
	PriceCents        *int64            `json:"price_cents"`
	Currency          string            `json:"currency"`
	PickupBy          string            `json:"pickup_by"`
	Payment           string            `json:"payment"`
	NeedsPrescription *bool             `json:"needs_prescription"`
	Evidence          map[string]string `json:"evidence"`
	Confidence        *float64          `json:"confidence"`
}

var offerEvidenceKeys = map[string]bool{
	"in_stock": true, "strength": true, "quantity": true, "price": true,
	"pickup_by": true, "payment": true, "needs_prescription": true,
}

// OfferRequest is what the enquiry asked for, used by EvaluateOffer.
type OfferRequest struct {
	Strength    string // requested medication strength ("" when not matched yet)
	Quantity    int
	TargetState string // enquiry_targets.status after the pharmacist replied
	ReplyText   string

	// Dose equivalence (CONTRACT rule 1). When GovLookup is set, an offer of a
	// different strength is checked by EvaluateEquivalence: accepted lifts the
	// strength block, anything else rejects. With GovLookup nil (no reference to
	// read) a different strength goes to a human as before.
	Ingredient string // requested medication's active ingredient
	DosageForm string // requested medication's dosage form
	GovLookup  func(ingredient, strength, form string) (GovProduct, bool)
}

// OfferDecision is the rules' verdict.
type OfferDecision struct {
	Decision string   `json:"decision"`
	Reasons  []string `json:"reasons"`
	// DoseCheck is set only when a different strength was checked for equivalence.
	DoseCheck *DoseCheck `json:"dose_check,omitempty"`
}

func (in *OfferInput) normalize() {
	in.Strength = strings.TrimSpace(in.Strength)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.PickupBy = strings.TrimSpace(in.PickupBy)
	in.Payment = strings.ToLower(strings.TrimSpace(in.Payment))
}

func validateOffer(in OfferInput) error {
	if in.Confidence == nil || math.IsNaN(*in.Confidence) || *in.Confidence < 0 || *in.Confidence > 1 {
		return Invalid("confidence must be a number from 0 to 1 (how sure you are of the whole reading)")
	}
	if in.Quantity != nil && (*in.Quantity < 0 || *in.Quantity > maxEnquiryQuantity) {
		return Invalid("quantity must be between 0 and %d", maxEnquiryQuantity)
	}
	if in.PriceCents != nil && *in.PriceCents < 0 {
		return Invalid("price_cents must be 0 or more")
	}
	if in.PriceCents != nil && in.Currency == "" {
		return Invalid("currency is required with price_cents, e.g. LBP or USD")
	}
	switch in.Payment {
	case "", "cash", "card", "any":
	default:
		return Invalid("payment must be cash, card, any or empty")
	}
	if utf8.RuneCountInString(in.Strength) > maxOfferStrengthChars {
		return Invalid("strength must be %d characters or fewer", maxOfferStrengthChars)
	}
	if utf8.RuneCountInString(in.PickupBy) > maxOfferPickupChars {
		return Invalid("pickup_by must be %d characters or fewer", maxOfferPickupChars)
	}
	if utf8.RuneCountInString(in.Currency) > maxOfferCurrencyChars {
		return Invalid("currency must be %d characters or fewer", maxOfferCurrencyChars)
	}
	for k, v := range in.Evidence {
		if !offerEvidenceKeys[k] {
			return Invalid("unknown evidence key %q; use in_stock, strength, quantity, price, pickup_by, payment or needs_prescription", k)
		}
		if utf8.RuneCountInString(v) > maxOfferQuoteChars {
			return Invalid("evidence[%s] must be %d characters or fewer; quote only the words that support the field", k, maxOfferQuoteChars)
		}
	}
	if in.InStock == nil && in.Strength == "" && in.Quantity == nil && in.PriceCents == nil &&
		in.PickupBy == "" && in.Payment == "" && in.NeedsPrescription == nil {
		return Invalid("set at least one field read from the reply (in_stock, strength, quantity, price_cents, pickup_by, payment, needs_prescription)")
	}
	return nil
}

// EvaluateOffer applies the offer rules. It is pure: same input, same verdict.
// rejected beats escalated beats matches.
func EvaluateOffer(req OfferRequest, in OfferInput) OfferDecision {
	var reject, escalate []string
	reply := normQuote(req.ReplyText)

	// Dose equivalence for a different strength; equivReason is set only when accepted.
	dose := offerDoseCheck(req, in)
	equivReason := ""
	if dose != nil && dose.Accepted {
		rv, ru, _ := doseStrength(req.Strength)
		ov, ou, _ := doseStrength(in.Strength)
		equivReason = fmt.Sprintf("Rules-checked dose equivalent: %d × %s = %s",
			dose.UnitsPerDose, formatDoseStrength(ov, ou), formatDoseStrength(rv, ru))
	}

	// 1. Every field that is set must be backed by a quote that really is in the reply.
	set := map[string]bool{
		"in_stock": in.InStock != nil, "strength": in.Strength != "", "quantity": in.Quantity != nil,
		"price": in.PriceCents != nil, "pickup_by": in.PickupBy != "", "payment": in.Payment != "",
		"needs_prescription": in.NeedsPrescription != nil,
	}
	for _, k := range []string{"in_stock", "strength", "quantity", "price", "pickup_by", "payment", "needs_prescription"} {
		if !set[k] {
			continue
		}
		q := normQuote(in.Evidence[k])
		switch {
		case q == "":
			escalate = append(escalate, fmt.Sprintf("%s has no quoted evidence", k))
		case !strings.Contains(reply, q):
			escalate = append(escalate, fmt.Sprintf("%s: quote %q is not in the pharmacist's reply", k, in.Evidence[k]))
		default:
			// A real quote must also say the number it backs: "3ande" proves stock, not "5 boxes".
			if num, ok := offerNumberShown(k, in, in.Evidence[k]); !ok {
				escalate = append(escalate, fmt.Sprintf("%s: quote %q does not contain the number %s", k, in.Evidence[k], num))
			}
		}
	}

	// 2. Confidence.
	if in.Confidence != nil && *in.Confidence < MinOfferConfidence {
		escalate = append(escalate, fmt.Sprintf("low confidence %.2f (needs %.2f)", *in.Confidence, MinOfferConfidence))
	}

	// 3. What the pharmacist said with the reply kind.
	switch req.TargetState {
	case queries.TargetAlternativeOffered:
		// A rules-accepted dose equivalent is the one alternative that can match (rule 1).
		if equivReason == "" {
			escalate = append(escalate, "the pharmacist offered an alternative; an alternative is never a match")
		}
	case queries.TargetUnavailable:
		// Only "out of stock" agrees with an unavailable reply; any other reading
		// (in stock, or a quantity with in_stock unset) contradicts the pharmacist.
		if in.InStock == nil || *in.InStock {
			escalate = append(escalate, "the reading does not say out of stock but the pharmacist replied unavailable")
		}
	}

	// 4. Stock.
	if in.InStock != nil && !*in.InStock {
		reject = append(reject, "pharmacy says it is out of stock")
	}
	if in.InStock == nil && in.Quantity == nil {
		escalate = append(escalate, "the reply does not say whether it is in stock")
	}

	// 5. Prescription.
	if in.NeedsPrescription != nil && *in.NeedsPrescription {
		escalate = append(escalate, "pharmacy asks for a prescription")
	}

	// 6. Strength: a different strength is a substitution, which only a human or the prescriber approves.
	if in.Strength != "" {
		want, okWant := parseStrength(req.Strength)
		got, okGot := parseStrength(in.Strength)
		switch {
		case !okGot:
			escalate = append(escalate, fmt.Sprintf("strength %q has no clear unit (mg, mcg or g)", in.Strength))
		case !okWant:
			escalate = append(escalate, "the enquiry has no matched medication strength to compare with")
		case math.Abs(want-got) > 1e-6 && dose != nil:
			if !dose.Accepted {
				var failed []string
				for _, c := range dose.Checks {
					if !c.OK {
						failed = append(failed, c.Label)
					}
				}
				reject = append(reject, "Dose equivalent rejected: "+strings.Join(failed, ", "))
			}
		case math.Abs(want-got) > 1e-6:
			escalate = append(escalate, fmt.Sprintf("strength %s differs from the requested %s; a substitution needs prescriber approval", in.Strength, req.Strength))
		}
	}

	// 7. Quantity.
	if in.Quantity != nil {
		switch {
		case *in.Quantity == 0:
			reject = append(reject, "pharmacy has none")
		case *in.Quantity < req.Quantity:
			escalate = append(escalate, fmt.Sprintf("only %d of the %d requested", *in.Quantity, req.Quantity))
		}
	}

	switch {
	case len(reject) > 0:
		return OfferDecision{Decision: queries.OfferRejected, Reasons: append(reject, escalate...), DoseCheck: dose}
	case len(escalate) > 0:
		if equivReason != "" {
			escalate = append(escalate, equivReason)
		}
		return OfferDecision{Decision: queries.OfferEscalated, Reasons: escalate, DoseCheck: dose}
	case equivReason != "":
		return OfferDecision{Decision: queries.OfferMatches, DoseCheck: dose,
			Reasons: []string{equivReason, "every field is quoted from the reply"}}
	default:
		return OfferDecision{Decision: queries.OfferMatches, Reasons: []string{"every field is quoted from the reply and matches the request"}}
	}
}

// offerDoseCheck runs the dose equivalence rules when the offer's strength is set,
// both strengths parse, they differ ("1g" equals "1000mg") and a government
// reference lookup is available. Otherwise it returns nil. The pharmacy is offering
// the same product line, so ingredient and form are the requested ones.
func offerDoseCheck(req OfferRequest, in OfferInput) *DoseCheck {
	if in.Strength == "" || req.GovLookup == nil {
		return nil
	}
	want, okWant := parseStrength(req.Strength)
	got, okGot := parseStrength(in.Strength)
	if !okWant || !okGot || math.Abs(want-got) <= 1e-6 {
		return nil
	}
	qty := 0
	if in.Quantity != nil {
		qty = *in.Quantity
	}
	dc := EvaluateEquivalence(
		DoseRequest{Ingredient: req.Ingredient, Strength: req.Strength, DosageForm: req.DosageForm, Quantity: req.Quantity},
		DoseOffer{Ingredient: req.Ingredient, UnitStrength: in.Strength, DosageForm: req.DosageForm, Quantity: qty},
		req.GovLookup)
	return &dc
}

// govLookup reads the government reference through the existing list query
// (ingredient, strength and form are case-insensitive exact filters). The strength
// is normalised first so "250 mg" and "0.25g" find "250mg". A store error is kept
// in *lookupErr and reported as "not listed" so the caller can fail the request.
func govLookup(ctx context.Context, q queries.Querier, lookupErr *error) func(ingredient, strength, form string) (GovProduct, bool) {
	return func(ingredient, strength, form string) (GovProduct, bool) {
		if v, unit, ok := doseStrength(strength); ok {
			strength = formatDoseStrength(v, unit)
		}
		ingredient, strength, form = strings.TrimSpace(ingredient), strings.TrimSpace(strength), strings.TrimSpace(form)
		if ingredient == "" || strength == "" || form == "" {
			return GovProduct{}, false
		}
		list, err := q.ListGovProducts(ctx, queries.ListGovProductsParams{
			Ingredient: ingredient, Strength: strength, DosageForm: form, Limit: 1,
		})
		if err != nil {
			*lookupErr = err
			return GovProduct{}, false
		}
		if len(list) == 0 {
			return GovProduct{}, false
		}
		g := list[0]
		return GovProduct{Code: g.Code, BrandName: g.BrandName, Ingredient: g.Ingredient, Strength: g.Strength, DosageForm: g.DosageForm}, true
	}
}

var spaceRun = regexp.MustCompile(`\s+`)

// normQuote lowercases and collapses whitespace so quotes match despite spacing.
func normQuote(s string) string {
	return strings.TrimSpace(spaceRun.ReplaceAllString(strings.ToLower(s), " "))
}

// arabicDigits maps Arabic-Indic and Persian digits to ASCII so "٢ علب" shows 2.
var arabicDigits = strings.NewReplacer(
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4", "٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
	"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4", "۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9",
)

var numberRe = regexp.MustCompile(`[0-9]+(?:[.,'][0-9]+)*`)

// quoteNumbers lists the numbers written in a quote. "1.200" yields 1.2 and 1200
// (decimal or thousands). standalone skips digits glued to a letter, which in
// Arabizi are letters ("3ande", "ma3ak"); "x2" and "×2" still count.
func quoteNumbers(quote string, standalone bool) []float64 {
	s := arabicDigits.Replace(quote)
	var out []float64
	for _, loc := range numberRe.FindAllStringIndex(s, -1) {
		if standalone {
			before, _ := utf8.DecodeLastRuneInString(s[:loc[0]])
			after, _ := utf8.DecodeRuneInString(s[loc[1]:])
			if loc[0] > 0 && unicode.IsLetter(before) && before != 'x' && before != 'X' && before != '×' {
				continue
			}
			if loc[1] < len(s) && unicode.IsLetter(after) {
				continue
			}
		}
		tok := s[loc[0]:loc[1]]
		if v, err := strconv.ParseFloat(strings.NewReplacer(".", "", ",", "", "'", "").Replace(tok), 64); err == nil {
			out = append(out, v)
		}
		if strings.Count(tok, ".")+strings.Count(tok, ",") == 1 && !strings.Contains(tok, "'") {
			if v, err := strconv.ParseFloat(strings.ReplaceAll(tok, ",", "."), 64); err == nil {
				out = append(out, v)
			}
		}
	}
	return out
}

func hasNumber(vals []float64, want ...float64) bool {
	for _, v := range vals {
		for _, w := range want {
			if math.Abs(v-w) < 1e-9 {
				return true
			}
		}
	}
	return false
}

// offerNumberShown checks that the quote for a numeric field writes that number:
// quantity as a standalone number, strength's amount, price in cents or whole units.
// Other fields always pass. It returns the number it looked for.
func offerNumberShown(key string, in OfferInput, quote string) (string, bool) {
	switch key {
	case "quantity":
		if in.Quantity == nil {
			return "", true
		}
		return strconv.Itoa(*in.Quantity), hasNumber(quoteNumbers(quote, true), float64(*in.Quantity))
	case "strength":
		m := strings.TrimSpace(strengthAmountRe.FindString(in.Strength))
		v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", "."), 64)
		if m == "" || err != nil {
			return "", true // no amount to look for; the strength rule escalates it
		}
		return m, hasNumber(quoteNumbers(quote, false), v)
	case "price":
		if in.PriceCents == nil {
			return "", true
		}
		c := float64(*in.PriceCents)
		return fmt.Sprintf("%d or %g", *in.PriceCents, c/100), hasNumber(quoteNumbers(quote, false), c, c/100)
	}
	return "", true
}

var strengthAmountRe = regexp.MustCompile(`^\s*[0-9]+(?:[.,][0-9]+)?`)

var strengthRe = regexp.MustCompile(`(?i)^\s*([0-9]+(?:[.,][0-9]+)?)\s*(mcg|µg|ug|mg|g)\s*$`)

// parseStrength turns "500mg", "0.5 g" or "500 mcg" into micrograms.
func parseStrength(s string) (float64, bool) {
	m := strengthRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(m[2]) {
	case "g":
		v *= 1e6
	case "mg":
		v *= 1e3
	}
	return v, true
}

// RecordOffer stores a structured reading of one target's reply and the rules' decision.
// Callers: the agent (agent-handled enquiries) or staff at the requesting pharmacy.
func (s *EnquiryService) RecordOffer(ctx context.Context, a Actor, targetID string, in OfferInput) (queries.TargetOffer, error) {
	in.normalize()
	if err := validateOffer(in); err != nil {
		return queries.TargetOffer{}, err
	}
	var out queries.TargetOffer
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
			return Conflict("this enquiry is closed; offers are recorded only while it is open")
		}
		t, err := q.GetEnquiryTarget(ctx, targetID)
		if err != nil {
			return FromStore(err, "target")
		}
		switch t.Status {
		case queries.TargetAvailable, queries.TargetUnavailable, queries.TargetAlternativeOffered:
		default:
			return Conflict("this pharmacy has not replied yet; offers are read only from a pharmacist's reply")
		}
		if strings.TrimSpace(t.ReplyText) == "" {
			return Conflict("this reply has no text to read an offer from")
		}

		strength, ingredient, form := "", "", ""
		if e.MedicationID != nil {
			med, err := q.GetMedication(ctx, *e.MedicationID)
			if err != nil {
				return FromStore(err, "medication")
			}
			strength, ingredient, form = med.Strength, med.ActiveIngredient, med.DosageForm
		}
		var lookupErr error
		d := EvaluateOffer(OfferRequest{
			Strength: strength, Quantity: e.Quantity, TargetState: t.Status, ReplyText: t.ReplyText,
			Ingredient: ingredient, DosageForm: form, GovLookup: govLookup(ctx, q, &lookupErr),
		}, in)
		if lookupErr != nil {
			return FromStore(lookupErr, "reference product")
		}

		evidence, err := json.Marshal(nonNilMap(in.Evidence))
		if err != nil {
			return Internal(err)
		}
		reasons, err := json.Marshal(d.Reasons)
		if err != nil {
			return Internal(err)
		}
		extractedBy := "staff"
		if a.IsAgent() {
			extractedBy = "agent"
		}
		o, err := q.CreateTargetOffer(ctx, queries.CreateTargetOfferParams{
			EnquiryID: e.ID, TargetID: t.ID, BranchID: t.BranchID, SourceText: t.ReplyText,
			ExtractedBy: extractedBy, InStock: in.InStock, Strength: in.Strength, Quantity: in.Quantity,
			PriceCents: in.PriceCents, Currency: in.Currency, PickupBy: in.PickupBy, Payment: in.Payment,
			NeedsPrescription: in.NeedsPrescription, Evidence: evidence, Confidence: *in.Confidence,
			Decision: d.Decision, DecidedBy: "rules", Reasons: reasons,
		})
		if err != nil {
			return FromStore(err, "offer")
		}

		if _, err := s.logEvent(ctx, q, e.ID, a, EvOfferExtracted, map[string]any{
			"offer_id": o.ID, "target_id": t.ID, "branch_id": t.BranchID, "confidence": o.Confidence,
		}, nil); err != nil {
			return err
		}
		decided := map[string]any{
			"offer_id": o.ID, "target_id": t.ID, "branch_id": t.BranchID,
			"decision": d.Decision, "reasons": d.Reasons,
		}
		if d.DoseCheck != nil {
			decided["dose_check"] = d.DoseCheck
		}
		if _, err := s.logEvent(ctx, q, e.ID, RulesActor, EvOfferDecided, decided, nil); err != nil {
			return err
		}
		out = o
		return nil
	})
	if err != nil {
		return queries.TargetOffer{}, err
	}
	return out, nil
}

// ListOffers returns an enquiry's offers to the requesting pharmacy or the agent.
// Target pharmacies never see each other's offers.
func (s *EnquiryService) ListOffers(ctx context.Context, a Actor, enquiryID string) ([]queries.TargetOffer, error) {
	e, err := s.store.GetEnquiry(ctx, enquiryID)
	if err != nil {
		return nil, FromStore(err, "enquiry")
	}
	if err := canAct(a, e); err != nil {
		return nil, err
	}
	offers, err := s.store.ListTargetOffers(ctx, e.ID)
	if err != nil {
		return nil, FromStore(err, "offer")
	}
	return offers, nil
}

func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
