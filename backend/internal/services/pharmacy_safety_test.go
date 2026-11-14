package services

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"alphahack/internal/queries"
)

// TestCheckMedicationSafetyGates_UnreadableTagsFailClosed checks that the
// medication safety gate refuses to dispense when tag JSON cannot be read,
// instead of treating unreadable tags as "no tags" (fail open).
func TestCheckMedicationSafetyGates_UnreadableTagsFailClosed(t *testing.T) {
	const wantCode = CodeConflict
	const wantPrefix = "safety_data_unreadable:"

	// assertFailClosed requires a *Error with the conflict code and the
	// safety_data_unreadable message prefix.
	assertFailClosed := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("expected error with code %q and message prefix %q, got nil", wantCode, wantPrefix)
		}
		var se *Error
		if !errors.As(err, &se) {
			t.Fatalf("expected *services.Error, got %T (%v)", err, err)
		}
		if se.Code != wantCode {
			t.Fatalf("expected error code %q, got %q (message: %q)", wantCode, se.Code, se.Message)
		}
		if !strings.HasPrefix(se.Message, wantPrefix) {
			t.Fatalf("expected message prefix %q, got %q", wantPrefix, se.Message)
		}
	}

	// assertPasses requires the gate to return nil.
	assertPasses := func(t *testing.T, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	}

	type tc struct {
		name       string
		medTags    json.RawMessage
		allergies  json.RawMessage
		conditions json.RawMessage
		wantErr    bool
	}

	cases := []tc{
		{
			name:       "medication tags are a JSON object",
			medTags:    json.RawMessage(`{"tag":"penicillin"}`),
			allergies:  json.RawMessage(`["penicillin"]`),
			conditions: json.RawMessage(`[]`),
			wantErr:    true,
		},
		{
			name:       "medication tags are a JSON string",
			medTags:    json.RawMessage(`"penicillin"`),
			allergies:  json.RawMessage(`[]`),
			conditions: json.RawMessage(`[]`),
			wantErr:    true,
		},
		{
			name:       "customer allergies unreadable while medication has tags",
			medTags:    json.RawMessage(`["penicillin"]`),
			allergies:  json.RawMessage(`{"x":1}`),
			conditions: json.RawMessage(`[]`),
			wantErr:    true,
		},
		{
			name:       "customer conditions unreadable while medication has tags",
			medTags:    json.RawMessage(`["asthma"]`),
			allergies:  json.RawMessage(`[]`),
			conditions: json.RawMessage(`"asthma"`),
			wantErr:    true,
		},
		{
			name:       "valid empty tags still pass",
			medTags:    json.RawMessage(`[]`),
			allergies:  json.RawMessage(`[]`),
			conditions: json.RawMessage(`[]`),
			wantErr:    false,
		},
		{
			name:       "valid non-matching tags still pass",
			medTags:    json.RawMessage(`["penicillin"]`),
			allergies:  json.RawMessage(`["sulfa"]`),
			conditions: json.RawMessage(`[]`),
			wantErr:    false,
		},
		{
			name:       "customer tags unreadable but medication has no tags passes",
			medTags:    json.RawMessage(`[]`),
			allergies:  json.RawMessage(`{"x":1}`),
			conditions: json.RawMessage(`[]`),
			wantErr:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			customer := queries.Customer{
				Allergies:         c.allergies,
				ChronicConditions: c.conditions,
			}
			medication := queries.Medication{
				Controlled:           false,
				ContraindicationTags: c.medTags,
			}
			err := checkMedicationSafetyGates(customer, medication)
			if c.wantErr {
				assertFailClosed(t, err)
			} else {
				assertPasses(t, err)
			}
		})
	}
}
