package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"alphahack/internal/queries"
)

var filterCatalogue = []string{
	"Augmentin", "Amoxicillin/Clavulanate", "Brufen", "Ibuprofen", "Panadol", "Paracetamol",
	"Panadol Advance", "Zithromax", "Azithromycin",
}

var filterAllowed = []string{"Augmentin", "Amoxicillin", "Clavulanate"}

func TestCheckOutboundText(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		allowed []string
		ok      bool
		reason  string // substring of the reason when blocked
	}{
		// allowed text
		{"plain relay passes", "Hamra Pharmacy has Augmentin 1g, 2 boxes, pickup before 18:00.", filterAllowed, true, ""},
		{"ingredient of requested medicine passes", "Amoxicillin/Clavulanate 1000mg is in stock at Verdun.", filterAllowed, true, ""},
		{"alternative offered by pharmacy passes", "Byblos offers an alternative: Zithromax 500mg. Not a match; tell us if acceptable.",
			append(append([]string{}, filterAllowed...), "zithromax", "500mg"), true, ""},
		{"clarification question passes", "Which strength do you need? Options: 1000mg, 625mg.", filterAllowed, true, ""},
		{"empty text passes", "   ", filterAllowed, true, ""},
		{"word containing a name is not the name", "Brufenine is not a catalogue word", nil, true, ""},
		{"short phrase needs a boundary", "we will switch today's order", nil, true, ""},

		// English phrases
		{"EN i recommend", "I recommend Augmentin 625mg.", filterAllowed, false, "recommendation"},
		{"EN we recommend inflected", "We recommended this one.", filterAllowed, false, "we recommend"},
		{"EN you should take", "You should take two a day.", filterAllowed, false, "you should take"},
		{"EN instead take", "Instead take the 625mg.", filterAllowed, false, "instead take"},
		{"EN take instead", "Take the 625 instead, it works.", filterAllowed, true, ""},
		{"EN take instead exact", "please take instead the syrup", filterAllowed, false, "take instead"},
		{"EN try taking", "Try taking the syrup.", filterAllowed, false, "try taking"},
		{"EN switch to", "Switch to the syrup form.", filterAllowed, false, "switch to"},
		{"EN substitute inflected", "It can be substituted.", filterAllowed, false, "substitute"},
		{"EN replace it with", "Replace it with the 625mg.", filterAllowed, false, "replace it with"},
		{"EN better to take", "It is better to take 500mg.", filterAllowed, false, "better to take"},

		// French
		{"FR je recommande", "Je recommande le 625mg.", filterAllowed, false, "je recommande"},
		{"FR a la place", "Prenez le sirop à la place.", filterAllowed, false, "à la place"},
		{"FR remplacer par", "On peut remplacer par le sirop.", filterAllowed, false, "remplacer par"},
		{"FR prenez plutot", "Prenez plutôt le 500mg.", filterAllowed, false, "prenez plutôt"},

		// Arabizi
		{"Arabizi jarreb", "jarreb el 625", filterAllowed, false, "jarreb"},
		{"Arabizi khod badal", "khod badal menno", filterAllowed, false, "khod badal"},
		{"Arabizi badal", "fi badal 3anna", filterAllowed, false, "badal"},
		{"Arabizi a7san tekhod", "a7san tekhod el sirop", filterAllowed, false, "a7san tekhod"},

		// Arabic
		{"AR ansah", "أنصح بأخذ الدواء", filterAllowed, false, "أنصح"},
		{"AR badalan", "خذ هذا بدلاً منه", filterAllowed, false, "بدل"},
		{"AR badal with prefix", "وبدل هذا", filterAllowed, false, "بدل"},
		{"AR jarrib with shadda", "جرّب هذا", filterAllowed, false, "جر"},
		{"AR istabdil", "استبدل الدواء", filterAllowed, false, "استبدل"},
		{"AR diacritics trick", "أَنْصَحُ بهذا", filterAllowed, false, "أنصح"},

		// other medicine names
		{"other brand blocked", "Augmentin is out, but Brufen is available.", filterAllowed, false, `"brufen"`},
		{"other ingredient blocked", "They have ibuprofen.", filterAllowed, false, `"ibuprofen"`},
		{"other name case-insensitive", "ZITHROMAX in stock", filterAllowed, false, `"zithromax"`},
		{"multiword brand needs every word allowed", "Panadol Advance 500mg available",
			[]string{"Panadol", "Paracetamol"}, false, `"panadol advance"`},
		{"requested brand passes", "Panadol 250mg available", []string{"Panadol", "Paracetamol"}, true, ""},
		{"no allowed names at all", "Augmentin is in stock", nil, false, `"augmentin"`},

		// zero-width tricks
		{"zero-width inside phrase", "I re\u200bcom\u200dmend this", filterAllowed, false, "recommend"},
		{"zero-width inside name", "Bru\u200bfen\ufeff is there", filterAllowed, false, `"brufen"`},
		{"word joiner and soft hyphen", "sub\u2060sti\u00adtute it", filterAllowed, false, "substitute"},
		{"extra whitespace inside phrase", "you   should\n\ttake it", filterAllowed, false, "you should take"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := CheckOutboundText(tc.text, tc.allowed, filterCatalogue)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (reason %q)", ok, tc.ok, reason)
			}
			if ok && reason != "" {
				t.Fatalf("passing text has a reason: %q", reason)
			}
			if !ok {
				if !strings.Contains(reason, tc.reason) {
					t.Fatalf("reason %q does not mention %q", reason, tc.reason)
				}
				if !strings.Contains(reason, "Relay only the requested medicine") && !strings.Contains(reason, "relay only the requested medicine") {
					t.Fatalf("reason does not teach what to do instead: %q", reason)
				}
			}
		})
	}
}

// ---- the hook in Notify / RequestClarification ------------------------------

// filterStore is an in-memory Querier for one enquiry. InTx records whether the
// transaction committed (fn returned nil).
type filterStore struct {
	queries.Querier
	enquiry   queries.Enquiry
	med       queries.Medication
	meds      []queries.Medication
	targets   []queries.EnquiryTarget
	events    []queries.CreateEnquiryEventParams
	outbox    []queries.CreateOutboxMessageParams
	updates   []queries.UpdateEnquiryParams
	committed int
}

func (f *filterStore) InTx(ctx context.Context, fn func(q queries.Querier) error) error {
	events, outbox, updates := len(f.events), len(f.outbox), len(f.updates)
	if err := fn(f); err != nil {
		f.events, f.outbox, f.updates = f.events[:events], f.outbox[:outbox], f.updates[:updates] // rollback
		return err
	}
	f.committed++
	return nil
}

func (f *filterStore) GetEnquiryForUpdate(ctx context.Context, id string) (queries.Enquiry, error) {
	if id != f.enquiry.ID {
		return queries.Enquiry{}, queries.ErrNotFound
	}
	return f.enquiry, nil
}

func (f *filterStore) GetMedication(ctx context.Context, id string) (queries.Medication, error) {
	if id != f.med.ID {
		return queries.Medication{}, queries.ErrNotFound
	}
	return f.med, nil
}

func (f *filterStore) ListMedications(ctx context.Context, limit, offset int) ([]queries.Medication, error) {
	return f.meds, nil
}

func (f *filterStore) ListEnquiryTargets(ctx context.Context, enquiryID string) ([]queries.EnquiryTarget, error) {
	return f.targets, nil
}

func (f *filterStore) CreateEnquiryEvent(ctx context.Context, p queries.CreateEnquiryEventParams) (queries.EnquiryEvent, error) {
	f.events = append(f.events, p)
	return queries.EnquiryEvent{ID: "ev", EnquiryID: p.EnquiryID, ActorType: p.ActorType, ActorID: p.ActorID, Type: p.Type}, nil
}

func (f *filterStore) CreateOutboxMessage(ctx context.Context, p queries.CreateOutboxMessageParams) (queries.OutboxMessage, error) {
	f.outbox = append(f.outbox, p)
	return queries.OutboxMessage{ID: "msg", EnquiryID: p.EnquiryID, Kind: p.Kind, Body: p.Body}, nil
}

func (f *filterStore) UpdateEnquiry(ctx context.Context, p queries.UpdateEnquiryParams) (queries.Enquiry, error) {
	f.updates = append(f.updates, p)
	e := f.enquiry
	e.Status, e.ClarificationQuestion = p.Status, p.ClarificationQuestion
	return e, nil
}

func newFilterStore(medicationSet bool) *filterStore {
	f := &filterStore{
		enquiry: queries.Enquiry{
			ID: "e1", RequesterBranchID: "b-a", RawRequest: "need augmentin 1g tabs x2",
			Status: queries.EnquiryOpen, HandledBy: "agent",
		},
		med: queries.Medication{ID: "m1", BrandName: "Augmentin", ActiveIngredient: "Amoxicillin/Clavulanate"},
		meds: []queries.Medication{
			{ID: "m1", BrandName: "Augmentin", ActiveIngredient: "Amoxicillin/Clavulanate"},
			{ID: "m2", BrandName: "Brufen", ActiveIngredient: "Ibuprofen"},
			{ID: "m3", BrandName: "Zithromax", ActiveIngredient: "Azithromycin"},
		},
		targets: []queries.EnquiryTarget{{ID: "t1", BranchID: "b-c", ReplyText: "3ande Zithromax bas"}},
	}
	if medicationSet {
		f.enquiry.MedicationID = strPtr("m1")
	}
	return f
}

var cfAgent = Actor{Type: queries.ActorAgent, ID: "wassit"}

func newFilterService(f *filterStore) *EnquiryService {
	return NewEnquiryService(f, EnquiryConfig{Now: func() time.Time { return time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC) }})
}

func wantContentFiltered(t *testing.T, err error, f *filterStore) {
	t.Helper()
	se, ok := err.(*Error)
	if !ok || se.Code != CodeContentFiltered {
		t.Fatalf("err = %v, want content_filtered", err)
	}
	if len(f.outbox) != 0 || len(f.updates) != 0 {
		t.Fatalf("blocked text changed state: outbox %d, updates %d", len(f.outbox), len(f.updates))
	}
	if len(f.events) != 1 || f.events[0].Type != EvContentFiltered ||
		f.events[0].ActorType != queries.ActorSystem || f.events[0].ActorID != "rules" {
		t.Fatalf("want one content_filtered event by system/rules, got %+v", f.events)
	}
	if !strings.Contains(string(f.events[0].Payload), `"reason"`) || !strings.Contains(string(f.events[0].Payload), `"text"`) {
		t.Fatalf("event payload lacks text/reason: %s", f.events[0].Payload)
	}
	if f.committed != 1 {
		t.Fatalf("the content_filtered event must be committed, committed = %d", f.committed)
	}
}

func TestNotifyContentFilter(t *testing.T) {
	t.Run("agent recommendation is blocked and logged", func(t *testing.T) {
		f := newFilterStore(true)
		_, err := newFilterService(f).Notify(context.Background(), cfAgent, "e1", "I recommend Augmentin 625mg instead.")
		wantContentFiltered(t, err, f)
	})
	t.Run("agent naming another medicine is blocked", func(t *testing.T) {
		f := newFilterStore(true)
		_, err := newFilterService(f).Notify(context.Background(), cfAgent, "e1", "Augmentin is out everywhere; Brufen is in stock.")
		wantContentFiltered(t, err, f)
	})
	t.Run("agent relaying what a pharmacy offered passes", func(t *testing.T) {
		f := newFilterStore(true)
		_, err := newFilterService(f).Notify(context.Background(), cfAgent, "e1",
			"Byblos offers an alternative: 3ande Zithromax bas. Not a match; tell us if acceptable.")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.outbox) != 1 || len(f.events) != 1 || f.events[0].Type != EvRequesterNotified {
			t.Fatalf("want one queued notification, got outbox %d events %+v", len(f.outbox), f.events)
		}
	})
	t.Run("staff text is not filtered", func(t *testing.T) {
		f := newFilterStore(true)
		f.enquiry.HandledBy = "manual"
		staff := Actor{Type: queries.ActorStaff, ID: "s-a", BranchID: "b-a"}
		if _, err := newFilterService(f).Notify(context.Background(), staff, "e1", "I recommend Brufen instead."); err != nil {
			t.Fatalf("staff text was filtered: %v", err)
		}
		if len(f.outbox) != 1 {
			t.Fatalf("staff notification not queued")
		}
	})
}

func TestRequestClarificationContentFilter(t *testing.T) {
	t.Run("question naming the requested brand passes before the medicine is set", func(t *testing.T) {
		f := newFilterStore(false)
		_, err := newFilterService(f).RequestClarification(context.Background(), cfAgent, "e1",
			"Which strength of Augmentin do you need? Options: 1000mg, 625mg.")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.updates) != 1 || f.updates[0].Status != queries.EnquiryAwaitingClarification {
			t.Fatalf("want awaiting_clarification, got %+v", f.updates)
		}
	})
	t.Run("question suggesting a substitute is blocked", func(t *testing.T) {
		f := newFilterStore(true)
		_, err := newFilterService(f).RequestClarification(context.Background(), cfAgent, "e1",
			"Would you accept Bru\u200bfen if Augmentin is out?")
		wantContentFiltered(t, err, f)
	})
	t.Run("Arabic recommendation is blocked", func(t *testing.T) {
		f := newFilterStore(true)
		_, err := newFilterService(f).RequestClarification(context.Background(), cfAgent, "e1", "أنصح بالشراب بدلاً من الحبوب؟")
		wantContentFiltered(t, err, f)
	})
}
