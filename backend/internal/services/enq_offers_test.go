package services

import (
	"context"
	"strings"
	"testing"

	"alphahack/internal/ai"
	"alphahack/internal/queries"
)

func bp(b bool) *bool       { return &b }
func ip(i int) *int         { return &i }
func fp(f float64) *float64 { return &f }
func i64p(i int64) *int64   { return &i }

const messyReply = "3ande bas 625, ta3e abel l 6, cash"

func baseReq() OfferRequest {
	return OfferRequest{Strength: "1000mg", Quantity: 2, TargetState: queries.TargetAvailable, ReplyText: "3ande 1g, 2 3elab, ta3e abel l 6, cash"}
}

func TestEvaluateOfferMatches(t *testing.T) {
	d := EvaluateOffer(baseReq(), OfferInput{
		InStock: bp(true), Strength: "1g", Quantity: ip(2), PickupBy: "before 18:00", Payment: "cash",
		Evidence: map[string]string{
			"in_stock": "3ande", "strength": "1g", "quantity": "2 3elab",
			"pickup_by": "abel l 6", "payment": "CASH",
		},
		Confidence: fp(0.9),
	})
	if d.Decision != queries.OfferMatches {
		t.Fatalf("decision = %s, reasons %v", d.Decision, d.Reasons)
	}
}

func TestEvaluateOfferStrengthSubstitutionEscalates(t *testing.T) {
	req := baseReq()
	req.ReplyText = messyReply
	d := EvaluateOffer(req, OfferInput{
		InStock: bp(true), Strength: "625mg", Payment: "cash",
		Evidence:   map[string]string{"in_stock": "3ande", "strength": "625", "payment": "cash"},
		Confidence: fp(0.85),
	})
	if d.Decision != queries.OfferEscalated || !strings.Contains(strings.Join(d.Reasons, "|"), "prescriber approval") {
		t.Fatalf("want escalated for substitution, got %s %v", d.Decision, d.Reasons)
	}
}

func TestEvaluateOfferQuoteNotInReplyEscalates(t *testing.T) {
	d := EvaluateOffer(baseReq(), OfferInput{
		InStock: bp(true), Quantity: ip(2), PriceCents: i64p(500), Currency: "USD",
		Evidence:   map[string]string{"in_stock": "3ande", "quantity": "2 3elab", "price": "5 dollars"},
		Confidence: fp(0.95),
	})
	if d.Decision != queries.OfferEscalated || !strings.Contains(d.Reasons[0], "not in the pharmacist's reply") {
		t.Fatalf("invented quote must escalate, got %s %v", d.Decision, d.Reasons)
	}
}

func TestEvaluateOfferMissingEvidenceEscalates(t *testing.T) {
	d := EvaluateOffer(baseReq(), OfferInput{InStock: bp(true), Quantity: ip(2), Confidence: fp(0.9),
		Evidence: map[string]string{"in_stock": "3ande"}})
	if d.Decision != queries.OfferEscalated {
		t.Fatalf("got %s %v", d.Decision, d.Reasons)
	}
}

func TestEvaluateOfferOutOfStockRejects(t *testing.T) {
	req := baseReq()
	req.TargetState = queries.TargetUnavailable
	req.ReplyText = "ma fi, khalsan"
	d := EvaluateOffer(req, OfferInput{InStock: bp(false), Evidence: map[string]string{"in_stock": "ma fi"}, Confidence: fp(0.9)})
	if d.Decision != queries.OfferRejected {
		t.Fatalf("got %s %v", d.Decision, d.Reasons)
	}
}

func TestEvaluateOfferEscalationCases(t *testing.T) {
	cases := map[string]struct {
		req OfferRequest
		in  OfferInput
	}{
		"low confidence": {baseReq(), OfferInput{InStock: bp(true), Quantity: ip(2),
			Evidence: map[string]string{"in_stock": "3ande", "quantity": "2 3elab"}, Confidence: fp(0.4)}},
		"prescription": {OfferRequest{Strength: "1000mg", Quantity: 2, TargetState: queries.TargetAvailable, ReplyText: "fi bas badna wasfe"},
			OfferInput{InStock: bp(true), NeedsPrescription: bp(true), Evidence: map[string]string{"in_stock": "fi", "needs_prescription": "badna wasfe"}, Confidence: fp(0.9)}},
		"partial quantity": {baseReq(), OfferInput{InStock: bp(true), Quantity: ip(1),
			Evidence: map[string]string{"in_stock": "3ande", "quantity": "2 3elab"}, Confidence: fp(0.9)}},
		"alternative reply": {OfferRequest{Strength: "1000mg", Quantity: 2, TargetState: queries.TargetAlternativeOffered, ReplyText: "625mg only"},
			OfferInput{InStock: bp(true), Evidence: map[string]string{"in_stock": "625mg only"}, Confidence: fp(0.9)}},
		"unitless strength": {OfferRequest{Strength: "1000mg", Quantity: 2, TargetState: queries.TargetAvailable, ReplyText: messyReply},
			OfferInput{InStock: bp(true), Strength: "625", Evidence: map[string]string{"in_stock": "3ande", "strength": "625"}, Confidence: fp(0.9)}},
		"stock not stated": {baseReq(), OfferInput{Payment: "cash", Evidence: map[string]string{"payment": "cash"}, Confidence: fp(0.9)}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if d := EvaluateOffer(c.req, c.in); d.Decision != queries.OfferEscalated {
				t.Fatalf("got %s %v", d.Decision, d.Reasons)
			}
		})
	}
}

func TestParseStrength(t *testing.T) {
	cases := map[string]float64{"1g": 1e6, "1000mg": 1e6, "1000 MG": 1e6, "500mcg": 500, "0,5 mg": 500, "100µg": 100}
	for in, want := range cases {
		got, ok := parseStrength(in)
		if !ok || got != want {
			t.Errorf("parseStrength(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "625", "two tablets", "5ml"} {
		if _, ok := parseStrength(bad); ok {
			t.Errorf("parseStrength(%q) should fail", bad)
		}
	}
}

func TestValidateOffer(t *testing.T) {
	bad := []OfferInput{
		{InStock: bp(true)}, // no confidence
		{InStock: bp(true), Confidence: fp(1.5)},
		{Confidence: fp(0.9)}, // no fields
		{InStock: bp(true), Payment: "bitcoin", Confidence: fp(0.9)},
		{PriceCents: i64p(100), Confidence: fp(0.9)}, // no currency
		{InStock: bp(true), Evidence: map[string]string{"dose": "x"}, Confidence: fp(0.9)},
		{Quantity: ip(-1), Confidence: fp(0.9)},
	}
	for i, in := range bad {
		in.normalize()
		if err := validateOffer(in); err == nil {
			t.Errorf("case %d: want validation error", i)
		}
	}
}

func TestDetectLang(t *testing.T) {
	cases := map[string]string{
		"مرحبا، عندكن دوا؟":              "ar",
		"Bonjour, avez-vous du Concor ?": "fr",
		"Hello, do you have Augmentin?":  "en",
		"3ande bas 625":                  "en",
	}
	for in, want := range cases {
		if got := DetectLang(in); got != want {
			t.Errorf("DetectLang(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestSpeakWithFakeProvider(t *testing.T) {
	sp, err := ai.NewSpeech(ai.SpeechOptions{Provider: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	s := NewEnquiryService(nil, EnquiryConfig{Speech: sp})
	a, err := s.Speak(context.Background(), Actor{}, SpeechInput{Text: "مرحبا", Speaker: "pharmacy"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ContentType != "audio/wav" || string(a.Data[:4]) != "RIFF" || a.Voice != "fake-ar-pharmacy" {
		t.Fatalf("unexpected audio %s %q %q", a.ContentType, a.Data[:4], a.Voice)
	}
	for _, in := range []SpeechInput{{Text: ""}, {Text: "hi", Lang: "de"}, {Text: "hi", Speaker: "doctor"}, {Text: strings.Repeat("a", 601)}} {
		if _, err := s.Speak(context.Background(), Actor{}, in); err == nil {
			t.Errorf("want error for %+v", in)
		}
	}
	off := NewEnquiryService(nil, EnquiryConfig{})
	if _, err := off.Speak(context.Background(), Actor{}, SpeechInput{Text: "hi"}); err == nil {
		t.Error("speech off must fail")
	}
}
