package services

// Dose equivalence wired into offer recording (CONTRACT rule 1): a different
// strength matches only when the Go dose rules accept it.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"alphahack/internal/queries"
)

// equivStore is offerStore plus a government reference list.
type equivStore struct {
	*offerStore
	gov     []queries.GovProduct
	lookups int
}

func (s *equivStore) InTx(_ context.Context, fn func(q queries.Querier) error) error { return fn(s) }

func (s *equivStore) ListGovProducts(_ context.Context, p queries.ListGovProductsParams) ([]queries.GovProduct, error) {
	s.lookups++
	out := []queries.GovProduct{}
	for _, g := range s.gov {
		if strings.EqualFold(g.Ingredient, p.Ingredient) && strings.EqualFold(g.Strength, p.Strength) &&
			strings.EqualFold(g.DosageForm, p.DosageForm) {
			out = append(out, g)
		}
	}
	return out, nil
}

var equivGov = []queries.GovProduct{
	{Code: "6280000000010", BrandName: "Zithromax", Ingredient: "Azithromycin", Strength: "500mg", DosageForm: "tablet"},
	{Code: "6280000000011", BrandName: "Zithromax", Ingredient: "Azithromycin", Strength: "250mg", DosageForm: "tablet"},
	{Code: "6280000000001", BrandName: "Augmentin", Ingredient: "Amoxicillin/Clavulanate", Strength: "1000mg", DosageForm: "tablet"},
	{Code: "6280000000002", BrandName: "Augmentin", Ingredient: "Amoxicillin/Clavulanate", Strength: "625mg", DosageForm: "tablet"},
}

// newEquivStore: an enquiry for qty x med from Hamra; Achrafieh replied with reply.
func newEquivStore(med queries.Medication, qty int, status, reply string) *equivStore {
	st := newOfferStore("agent")
	st.med = med
	st.enquiry.MedicationID = &st.med.ID
	st.enquiry.Quantity = qty
	st.target.Status = status
	st.target.ReplyText = reply
	return &equivStore{offerStore: st, gov: equivGov}
}

var (
	zithromax500 = queries.Medication{ID: "med-zithromax-500", ActiveIngredient: "Azithromycin", DosageForm: "tablet", Strength: "500mg"}
	augmentin1g  = queries.Medication{ID: "med-augmentin-1g", ActiveIngredient: "Amoxicillin/Clavulanate", DosageForm: "tablet", Strength: "1000mg"}
)

func recordEquiv(t *testing.T, st *equivStore, in OfferInput) (queries.TargetOffer, map[string]any) {
	t.Helper()
	o, err := NewEnquiryService(st, EnquiryConfig{}).RecordOffer(context.Background(), agentActor, "tgt-1", in)
	if err != nil {
		t.Fatal(err)
	}
	last := st.events[len(st.events)-1]
	if last.Type != EvOfferDecided || last.ActorID != "rules" {
		t.Fatalf("last event = %+v, want offer_decided by rules", last)
	}
	var payload map[string]any
	if err := json.Unmarshal(last.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return o, payload
}

func strengthOffer(strength, strengthQuote string, qty int, qtyQuote string) OfferInput {
	return OfferInput{
		InStock: bp(true), Strength: strength, Quantity: ip(qty),
		Evidence:   map[string]string{"in_stock": "3ande", "strength": strengthQuote, "quantity": qtyQuote},
		Confidence: fp(0.9),
	}
}

func TestOfferEquivalentTwoHalvesMatches(t *testing.T) {
	for _, status := range []string{queries.TargetAvailable, queries.TargetAlternativeOffered} {
		st := newEquivStore(zithromax500, 2, status, "3ande 250mg bas, 4 3elab")
		o, payload := recordEquiv(t, st, strengthOffer("250 mg", "250mg", 4, "4 3elab"))
		if o.Decision != queries.OfferMatches || o.DecidedBy != "rules" {
			t.Fatalf("%s: decision = %s by %s, reasons %s", status, o.Decision, o.DecidedBy, o.Reasons)
		}
		if !strings.Contains(string(o.Reasons), "Rules-checked dose equivalent: 2 × 250mg = 500mg") {
			t.Fatalf("%s: reasons = %s", status, o.Reasons)
		}
		dc, ok := payload["dose_check"].(map[string]any)
		if !ok || dc["accepted"] != true || dc["gov_code"] != "6280000000011" || dc["units_per_dose"] != float64(2) {
			t.Fatalf("%s: dose_check = %v", status, payload["dose_check"])
		}
	}
}

func TestOfferEquivalentTooFewUnitsRejected(t *testing.T) {
	st := newEquivStore(zithromax500, 2, queries.TargetAvailable, "3ande 250mg bas, 2 3elab")
	o, payload := recordEquiv(t, st, strengthOffer("250mg", "250mg", 2, "2 3elab"))
	if o.Decision != queries.OfferRejected ||
		!strings.Contains(string(o.Reasons), "Dose equivalent rejected: "+DoseLabelTotal) {
		t.Fatalf("decision = %s, reasons %s", o.Decision, o.Reasons)
	}
	if dc, ok := payload["dose_check"].(map[string]any); !ok || dc["accepted"] != false {
		t.Fatalf("dose_check = %v", payload["dose_check"])
	}
}

func TestOfferAugmentin625NeverMakes1000Rejected(t *testing.T) {
	st := newEquivStore(augmentin1g, 2, queries.TargetAlternativeOffered, "3ande bas 625mg, 4 3elab")
	o, payload := recordEquiv(t, st, strengthOffer("625mg", "625mg", 4, "4 3elab"))
	if o.Decision != queries.OfferRejected ||
		!strings.Contains(string(o.Reasons), "Dose equivalent rejected: "+DoseLabelTotal) {
		t.Fatalf("decision = %s, reasons %s", o.Decision, o.Reasons)
	}
	if strings.Contains(string(o.Reasons), "Rules-checked") || payload["dose_check"] == nil {
		t.Fatalf("reasons %s, dose_check %v", o.Reasons, payload["dose_check"])
	}
}

func TestOfferUnlistedHalfStrengthRejected(t *testing.T) {
	// 2 x 500mg adds up to 1000mg, but the reference does not list Augmentin 500mg.
	st := newEquivStore(augmentin1g, 2, queries.TargetAvailable, "3ande 500mg, 4 3elab")
	o, payload := recordEquiv(t, st, strengthOffer("500mg", "500mg", 4, "4 3elab"))
	if o.Decision != queries.OfferRejected ||
		!strings.Contains(string(o.Reasons), "Dose equivalent rejected: "+DoseLabelGov) ||
		strings.Contains(string(o.Reasons), DoseLabelTotal) {
		t.Fatalf("decision = %s, reasons %s", o.Decision, o.Reasons)
	}
	if dc, ok := payload["dose_check"].(map[string]any); !ok || dc["accepted"] != false || dc["gov_code"] != "" {
		t.Fatalf("dose_check = %v", payload["dose_check"])
	}
}

func TestOfferEquivalentStillNeedsQuotesAndOtherRules(t *testing.T) {
	// Accepted equivalent, but the quantity quote is invented: escalated, never a match.
	st := newEquivStore(zithromax500, 2, queries.TargetAvailable, "3ande 250mg bas")
	o, _ := recordEquiv(t, st, strengthOffer("250mg", "250mg", 4, "4 3elab"))
	if o.Decision != queries.OfferEscalated || !strings.Contains(string(o.Reasons), "not in the pharmacist's reply") {
		t.Fatalf("decision = %s, reasons %s", o.Decision, o.Reasons)
	}
}

func TestOfferSameStrengthPathUnchanged(t *testing.T) {
	st := newEquivStore(augmentin1g, 2, queries.TargetAvailable, attackReply)
	o, payload := recordEquiv(t, st, honestOffer())
	if o.Decision != queries.OfferMatches ||
		string(o.Reasons) != `["every field is quoted from the reply and matches the request"]` {
		t.Fatalf("decision = %s, reasons %s", o.Decision, o.Reasons)
	}
	if _, has := payload["dose_check"]; has || st.lookups != 0 {
		t.Fatalf("same strength must not run the dose check: payload %v, lookups %d", payload, st.lookups)
	}
}
