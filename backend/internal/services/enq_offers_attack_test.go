package services

// Attack cases for structured offers and speech. Each test is named after the
// attack it stops. The store is an in-memory fake of queries.Querier.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"alphahack/internal/ai"
	"alphahack/internal/queries"
)

// offerStore fakes the Querier methods RecordOffer and ListOffers use. Any other
// method panics on the nil embedded interface.
type offerStore struct {
	queries.Querier
	enquiry queries.Enquiry
	target  queries.EnquiryTarget
	med     queries.Medication
	offers  []queries.TargetOffer
	events  []queries.CreateEnquiryEventParams
}

func (s *offerStore) InTx(_ context.Context, fn func(q queries.Querier) error) error { return fn(s) }

func (s *offerStore) GetEnquiryTarget(_ context.Context, id string) (queries.EnquiryTarget, error) {
	if id != s.target.ID {
		return queries.EnquiryTarget{}, queries.ErrNotFound
	}
	return s.target, nil
}

func (s *offerStore) GetEnquiry(_ context.Context, id string) (queries.Enquiry, error) {
	if id != s.enquiry.ID {
		return queries.Enquiry{}, queries.ErrNotFound
	}
	return s.enquiry, nil
}

func (s *offerStore) GetEnquiryForUpdate(ctx context.Context, id string) (queries.Enquiry, error) {
	return s.GetEnquiry(ctx, id)
}

func (s *offerStore) GetMedication(_ context.Context, id string) (queries.Medication, error) {
	if id != s.med.ID {
		return queries.Medication{}, queries.ErrNotFound
	}
	return s.med, nil
}

func (s *offerStore) CreateTargetOffer(_ context.Context, p queries.CreateTargetOfferParams) (queries.TargetOffer, error) {
	o := queries.TargetOffer{
		ID: "offer-" + string(rune('a'+len(s.offers))), EnquiryID: p.EnquiryID, TargetID: p.TargetID,
		BranchID: p.BranchID, SourceText: p.SourceText, ExtractedBy: p.ExtractedBy, InStock: p.InStock,
		Strength: p.Strength, Quantity: p.Quantity, PriceCents: p.PriceCents, Currency: p.Currency,
		PickupBy: p.PickupBy, Payment: p.Payment, NeedsPrescription: p.NeedsPrescription,
		Evidence: p.Evidence, Confidence: p.Confidence, Decision: p.Decision, DecidedBy: p.DecidedBy,
		Reasons: p.Reasons,
	}
	s.offers = append(s.offers, o)
	return o, nil
}

func (s *offerStore) CreateEnquiryEvent(_ context.Context, p queries.CreateEnquiryEventParams) (queries.EnquiryEvent, error) {
	s.events = append(s.events, p)
	return queries.EnquiryEvent{EnquiryID: p.EnquiryID, Type: p.Type}, nil
}

func (s *offerStore) ListTargetOffers(_ context.Context, enquiryID string) ([]queries.TargetOffer, error) {
	out := []queries.TargetOffer{}
	for _, o := range s.offers {
		if o.EnquiryID == enquiryID {
			out = append(out, o)
		}
	}
	return out, nil
}

const attackReply = "eh 3ande 2 3elab 1g, 12$ total, ta3e abel l 6, cash"

// newOfferStore: an agent-handled enquiry for 2 x 1g from Hamra, one target at
// Achrafieh that replied available with attackReply.
func newOfferStore(handledBy string) *offerStore {
	med := "med-augmentin-1g"
	return &offerStore{
		enquiry: queries.Enquiry{
			ID: "enq-1", RequesterBranchID: "branch-hamra", MedicationID: &med, Quantity: 2,
			Status: queries.EnquiryWaiting, HandledBy: handledBy,
		},
		target: queries.EnquiryTarget{
			ID: "tgt-1", EnquiryID: "enq-1", BranchID: "branch-achrafieh",
			Status: queries.TargetAvailable, ReplyText: attackReply,
		},
		med: queries.Medication{ID: med, Strength: "1000mg"},
	}
}

var (
	agentActor     = Actor{Type: queries.ActorAgent, ID: "wassit"}
	requesterStaff = Actor{Type: queries.ActorStaff, ID: "staff-hamra", BranchID: "branch-hamra"}
	targetStaff    = Actor{Type: queries.ActorStaff, ID: "staff-achrafieh", BranchID: "branch-achrafieh"}
)

// honestOffer is a correct reading of attackReply.
func honestOffer() OfferInput {
	return OfferInput{
		InStock: bp(true), Strength: "1g", Quantity: ip(2), PriceCents: i64p(1200), Currency: "usd",
		PickupBy: "before 18:00", Payment: "cash",
		Evidence: map[string]string{
			"in_stock": "3ande", "strength": "1g", "quantity": "2 3elab", "price": "12$ total",
			"pickup_by": "abel l 6", "payment": "cash",
		},
		Confidence: fp(0.9),
	}
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("want error code %q, got %v", code, err)
	}
}

func TestOfferHonestReadingMatchesAndLogsTwoEvents(t *testing.T) {
	st := newOfferStore("agent")
	o, err := NewEnquiryService(st, EnquiryConfig{}).RecordOffer(context.Background(), agentActor, "tgt-1", honestOffer())
	if err != nil {
		t.Fatal(err)
	}
	if o.Decision != queries.OfferMatches || o.DecidedBy != "rules" || o.ExtractedBy != "agent" || o.Currency != "USD" {
		t.Fatalf("offer = %+v reasons %s", o, o.Reasons)
	}
	if o.SourceText != attackReply {
		t.Fatalf("source_text must be the stored reply, got %q", o.SourceText)
	}
	if len(st.events) != 2 || st.events[0].Type != EvOfferExtracted || st.events[0].ActorType != queries.ActorAgent ||
		st.events[1].Type != EvOfferDecided || st.events[1].ActorType != queries.ActorSystem || st.events[1].ActorID != "rules" {
		t.Fatalf("events = %+v", st.events)
	}
}

func TestOfferAttackEvidenceQuoteNotInReply(t *testing.T) {
	st := newOfferStore("agent")
	in := honestOffer()
	in.Evidence["quantity"] = "we have 10 boxes ready" // invented, not in the reply
	o, err := NewEnquiryService(st, EnquiryConfig{}).RecordOffer(context.Background(), agentActor, "tgt-1", in)
	if err != nil {
		t.Fatal(err)
	}
	if o.Decision != queries.OfferEscalated || !strings.Contains(string(o.Reasons), "not in the pharmacist's reply") {
		t.Fatalf("invented quote must escalate, got %s %s", o.Decision, o.Reasons)
	}
	// Quotes are matched against the stored reply, never against text the caller sends.
	var dec OfferDecision
	if err := json.Unmarshal(st.events[1].Payload, &dec); err != nil || dec.Decision != queries.OfferEscalated {
		t.Fatalf("offer_decided payload = %s (%v)", st.events[1].Payload, err)
	}
}

func TestOfferAttackRealQuoteWithoutTheNumber(t *testing.T) {
	// The quote is really in the reply, but it does not say the number it backs.
	cases := map[string]func(*OfferInput){
		"quantity inflated, quote 3ande":   func(in *OfferInput) { in.Quantity = ip(5); in.Evidence["quantity"] = "3ande" },
		"quantity from arabizi letter":     func(in *OfferInput) { in.Quantity = ip(3); in.Evidence["quantity"] = "2 3elab" },
		"price invented, quote cash":       func(in *OfferInput) { in.PriceCents = i64p(500); in.Evidence["price"] = "cash" },
		"price off by 10x":                 func(in *OfferInput) { in.PriceCents = i64p(12000); in.Evidence["price"] = "12$ total" },
		"strength amount not in the quote": func(in *OfferInput) { in.Strength = "1000mg"; in.Evidence["strength"] = "3ande" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := honestOffer()
			mutate(&in)
			in.normalize()
			req := OfferRequest{Strength: "1000mg", Quantity: 2, TargetState: queries.TargetAvailable, ReplyText: attackReply}
			d := EvaluateOffer(req, in)
			if d.Decision == queries.OfferMatches || !strings.Contains(strings.Join(d.Reasons, "|"), "does not contain the number") {
				t.Fatalf("got %s %v", d.Decision, d.Reasons)
			}
		})
	}
}

func TestOfferQuoteNumbersAcceptRealForms(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		in    OfferInput
	}{
		{"arabic-indic digits", "نعم عندي ٢ علب", OfferInput{InStock: bp(true), Quantity: ip(2),
			Evidence: map[string]string{"in_stock": "عندي", "quantity": "٢ علب"}, Confidence: fp(0.9)}},
		{"x2", "fi x2", OfferInput{InStock: bp(true), Quantity: ip(2),
			Evidence: map[string]string{"in_stock": "fi", "quantity": "x2"}, Confidence: fp(0.9)}},
		{"lbp thousands", "fi 2, 1,200,000 lbp", OfferInput{InStock: bp(true), Quantity: ip(2), PriceCents: i64p(1200000), Currency: "LBP",
			Evidence: map[string]string{"in_stock": "fi", "quantity": "fi 2", "price": "1,200,000 lbp"}, Confidence: fp(0.9)}},
		{"strength in grams", "fi 2 of 1 g", OfferInput{InStock: bp(true), Quantity: ip(2), Strength: "1 g",
			Evidence: map[string]string{"in_stock": "fi", "quantity": "fi 2", "strength": "1 g"}, Confidence: fp(0.9)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.in.normalize()
			d := EvaluateOffer(OfferRequest{Strength: "1000mg", Quantity: 2, TargetState: queries.TargetAvailable, ReplyText: c.reply}, c.in)
			if d.Decision != queries.OfferMatches {
				t.Fatalf("got %s %v", d.Decision, d.Reasons)
			}
		})
	}
}

func TestOfferAttackUnavailableReplyReadAsQuantity(t *testing.T) {
	// The pharmacist said unavailable; a reading that leaves in_stock unset but
	// claims a quantity must not become a match.
	req := OfferRequest{Strength: "1000mg", Quantity: 2, TargetState: queries.TargetUnavailable, ReplyText: "ma fi, kenet fi 2 bas khalsit"}
	d := EvaluateOffer(req, OfferInput{Quantity: ip(2), Evidence: map[string]string{"quantity": "fi 2"}, Confidence: fp(0.95)})
	if d.Decision == queries.OfferMatches {
		t.Fatalf("unavailable reply read as a match: %v", d.Reasons)
	}
}

func TestOfferAttackAgentTriesToSetDecision(t *testing.T) {
	// OfferInput has no decision/decided_by field (the HTTP decoder rejects unknown
	// fields, see handlers); even a reading tuned to look perfect is decided by rules
	// and a stored reply that is an alternative never matches.
	st := newOfferStore("agent")
	st.target.Status = queries.TargetAlternativeOffered
	o, err := NewEnquiryService(st, EnquiryConfig{}).RecordOffer(context.Background(), agentActor, "tgt-1", honestOffer())
	if err != nil {
		t.Fatal(err)
	}
	if o.Decision != queries.OfferEscalated || o.DecidedBy != "rules" {
		t.Fatalf("alternative must escalate, decided by rules; got %s by %s", o.Decision, o.DecidedBy)
	}
	if st.target.Status != queries.TargetAlternativeOffered || st.enquiry.Status != queries.EnquiryWaiting {
		t.Fatal("an offer must never change the target or the enquiry")
	}
}

func TestOfferAttackPriceNegative(t *testing.T) {
	st := newOfferStore("agent")
	in := honestOffer()
	in.PriceCents = i64p(-100)
	_, err := NewEnquiryService(st, EnquiryConfig{}).RecordOffer(context.Background(), agentActor, "tgt-1", in)
	wantCode(t, err, CodeInvalid)
	if len(st.offers) != 0 || len(st.events) != 0 {
		t.Fatal("a rejected body must write nothing")
	}
}

func TestOfferAttackBadNumbers(t *testing.T) {
	cases := map[string]func(*OfferInput){
		"quantity negative":      func(in *OfferInput) { in.Quantity = ip(-2) },
		"quantity huge":          func(in *OfferInput) { in.Quantity = ip(1 << 30) },
		"confidence above 1":     func(in *OfferInput) { in.Confidence = fp(1.01) },
		"confidence negative":    func(in *OfferInput) { in.Confidence = fp(-0.1) },
		"price without currency": func(in *OfferInput) { in.Currency = "" },
		"oversized quote":        func(in *OfferInput) { in.Evidence["in_stock"] = strings.Repeat("3ande ", 50) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := honestOffer()
			mutate(&in)
			_, err := NewEnquiryService(newOfferStore("agent"), EnquiryConfig{}).RecordOffer(context.Background(), agentActor, "tgt-1", in)
			wantCode(t, err, CodeInvalid)
		})
	}
}

func TestOfferAttackAgentOnManualEnquiry(t *testing.T) {
	// The agent credential passes route auth (agent-ok) but a manual enquiry is staff-only.
	st := newOfferStore("manual")
	svc := NewEnquiryService(st, EnquiryConfig{})
	_, err := svc.RecordOffer(context.Background(), agentActor, "tgt-1", honestOffer())
	wantCode(t, err, CodeForbidden)
	_, err = svc.ListOffers(context.Background(), agentActor, "enq-1")
	wantCode(t, err, CodeForbidden)
	if len(st.offers) != 0 || len(st.events) != 0 {
		t.Fatal("forbidden call must write nothing")
	}
}

func TestOfferAttackTargetPharmacyRecordsOrReadsOffers(t *testing.T) {
	st := newOfferStore("manual")
	svc := NewEnquiryService(st, EnquiryConfig{})
	_, err := svc.RecordOffer(context.Background(), targetStaff, "tgt-1", honestOffer())
	wantCode(t, err, CodeForbidden)
	_, err = svc.ListOffers(context.Background(), targetStaff, "enq-1")
	wantCode(t, err, CodeForbidden)
	if _, err := svc.RecordOffer(context.Background(), requesterStaff, "tgt-1", honestOffer()); err != nil {
		t.Fatalf("requester staff may record on a manual enquiry: %v", err)
	}
	if got, err := svc.ListOffers(context.Background(), requesterStaff, "enq-1"); err != nil || len(got) != 1 || got[0].ExtractedBy != "staff" {
		t.Fatalf("requester list = %+v, %v", got, err)
	}
}

func TestOfferAttackOnClosedEnquiry(t *testing.T) {
	for _, status := range []string{queries.EnquiryClosed, queries.EnquiryCancelled} {
		st := newOfferStore("agent")
		st.enquiry.Status = status
		_, err := NewEnquiryService(st, EnquiryConfig{}).RecordOffer(context.Background(), agentActor, "tgt-1", honestOffer())
		wantCode(t, err, CodeConflict)
		if len(st.offers) != 0 || len(st.events) != 0 {
			t.Fatalf("%s: closed enquiry must not get offers or events", status)
		}
	}
}

func TestOfferAttackBeforeReplyOrEmptyReply(t *testing.T) {
	for name, mutate := range map[string]func(*offerStore){
		"sent, no reply yet": func(s *offerStore) { s.target.Status = queries.TargetSent; s.target.ReplyText = "" },
		"no_response":        func(s *offerStore) { s.target.Status = queries.TargetNoResponse },
		"reply without text": func(s *offerStore) { s.target.ReplyText = "   " },
	} {
		t.Run(name, func(t *testing.T) {
			st := newOfferStore("agent")
			mutate(st)
			_, err := NewEnquiryService(st, EnquiryConfig{}).RecordOffer(context.Background(), agentActor, "tgt-1", honestOffer())
			wantCode(t, err, CodeConflict)
		})
	}
}

func TestOfferAttackUnknownTarget(t *testing.T) {
	_, err := NewEnquiryService(newOfferStore("agent"), EnquiryConfig{}).RecordOffer(context.Background(), agentActor, "nope", honestOffer())
	wantCode(t, err, CodeNotFound)
}

// ---- speech ------------------------------------------------------------------

type countingSpeech struct {
	calls int
	err   error
}

func (c *countingSpeech) Name() string { return "counting" }

func (c *countingSpeech) Synthesize(_ context.Context, text, lang, speaker string) (ai.Audio, error) {
	c.calls++
	if c.err != nil {
		return ai.Audio{}, c.err
	}
	return ai.Audio{Data: []byte("RIFF" + text), ContentType: "audio/wav", Voice: lang + "-" + speaker}, nil
}

func TestSpeechAttackOversizedText(t *testing.T) {
	sp := &countingSpeech{}
	s := NewEnquiryService(nil, EnquiryConfig{Speech: sp})
	for name, text := range map[string]string{
		"601 latin":          strings.Repeat("a", maxSpeechChars+1),
		"601 arabic":         strings.Repeat("ب", maxSpeechChars+1),
		"100k bytes":         strings.Repeat("x", 100_000),
		"601 after trimming": "  " + strings.Repeat("a", maxSpeechChars+1) + "  ",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := s.Speak(context.Background(), agentActor, SpeechInput{Text: text})
			wantCode(t, err, CodeInvalid)
		})
	}
	if sp.calls != 0 {
		t.Fatalf("provider called %d times for oversized text", sp.calls)
	}
	// The limit counts characters, not bytes: 600 Arabic letters (1200 bytes) pass.
	if _, err := s.Speak(context.Background(), agentActor, SpeechInput{Text: strings.Repeat("ب", maxSpeechChars)}); err != nil {
		t.Fatalf("600 Arabic characters must pass: %v", err)
	}
}

func TestSpeechAttackEmptyText(t *testing.T) {
	sp := &countingSpeech{}
	s := NewEnquiryService(nil, EnquiryConfig{Speech: sp})
	for _, text := range []string{"", " ", "\n\t  \r\n"} {
		_, err := s.Speak(context.Background(), agentActor, SpeechInput{Text: text})
		wantCode(t, err, CodeInvalid)
	}
	if sp.calls != 0 {
		t.Fatalf("provider called %d times for empty text", sp.calls)
	}
}

func TestSpeechTrimsAndDetects(t *testing.T) {
	sp := &countingSpeech{}
	s := NewEnquiryService(nil, EnquiryConfig{Speech: sp})
	a, err := s.Speak(context.Background(), agentActor, SpeechInput{Text: "  Bonjour  ", Speaker: " PHARMACY "})
	if err != nil {
		t.Fatal(err)
	}
	if string(a.Data) != "RIFFBonjour" || a.Voice != "fr-pharmacy" {
		t.Fatalf("got %q voice %q", a.Data, a.Voice)
	}
}

func TestSpeechAttackProviderErrorDoesNotLeak(t *testing.T) {
	sp := &countingSpeech{err: errors.New("azure speech: key=SECRET status 401")}
	_, err := NewEnquiryService(nil, EnquiryConfig{Speech: sp}).Speak(context.Background(), agentActor, SpeechInput{Text: "hi"})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeUpstream || strings.Contains(e.Message, "SECRET") {
		t.Fatalf("want upstream error without provider details, got %v", err)
	}
}

func TestSpeechOffIsConflict(t *testing.T) {
	_, err := NewEnquiryService(nil, EnquiryConfig{}).Speak(context.Background(), agentActor, SpeechInput{Text: "hi"})
	wantCode(t, err, CodeConflict)
}
