package services

import (
	"reflect"
	"testing"
)

func verifiedCand(branch, offer string, total int64, expiry string) BestCandidate {
	return BestCandidate{
		BranchID:    branch,
		OfferID:     offer,
		Decision:    "matches",
		PhotoStatus: "verified",
		TotalCents:  total,
		HasTotal:    true,
		ExpiryMonth: expiry,
	}
}

func TestPickBestOffer(t *testing.T) {
	flagged := verifiedCand("b-flagged", "o-flagged", 100, "2027-01")
	flagged.PhotoStatus = "flagged"

	tests := []struct {
		name     string
		cands    []BestCandidate
		winner   string // "" means nil result
		excluded []BestExcluded
	}{
		{
			name:   "cheapest verified wins",
			cands:  []BestCandidate{verifiedCand("b1", "o1", 5000, "2027-01"), verifiedCand("b2", "o2", 4000, "2026-12")},
			winner: "b2",
			excluded: []BestExcluded{
				{BranchID: "b1", Reason: "Higher total price"},
			},
		},
		{
			name:   "tie on total goes to later expiry",
			cands:  []BestCandidate{verifiedCand("b1", "o1", 4000, "2026-11"), verifiedCand("b2", "o2", 4000, "2027-03")},
			winner: "b2",
			excluded: []BestExcluded{
				{BranchID: "b1", Reason: "Earlier expiry"},
			},
		},
		{
			name:   "empty expiry is earliest",
			cands:  []BestCandidate{verifiedCand("b1", "o1", 4000, ""), verifiedCand("b2", "o2", 4000, "2026-11")},
			winner: "b2",
			excluded: []BestExcluded{
				{BranchID: "b1", Reason: "Earlier expiry"},
			},
		},
		{
			name:   "flagged photo excluded with reason",
			cands:  []BestCandidate{flagged, verifiedCand("b2", "o2", 9000, "2026-11")},
			winner: "b2",
			excluded: []BestExcluded{
				{BranchID: "b-flagged", Reason: "Photo awaiting human check"},
			},
		},
		{
			name:   "nil when none verified",
			cands:  []BestCandidate{flagged},
			winner: "",
		},
		{
			name:   "nil when no candidates",
			cands:  nil,
			winner: "",
		},
		{
			name: "excluded order and reasons",
			cands: []BestCandidate{
				{BranchID: "d1", Declined: true, Decision: "matches", PhotoStatus: "verified", HasTotal: true, TotalCents: 1},
				{BranchID: "d2", NoResponse: true, Decision: "matches", PhotoStatus: "verified", HasTotal: true, TotalCents: 1},
				{BranchID: "d3", DoseRejected: true, Decision: "matches", PhotoStatus: "verified", HasTotal: true, TotalCents: 1},
				{BranchID: "d4", Decision: "matches", PhotoStatus: "mismatch", HasTotal: true, TotalCents: 1},
				{BranchID: "d5", Decision: "matches", PhotoStatus: "", HasTotal: true, TotalCents: 1},
				{BranchID: "d6", Decision: "escalated", TotalCents: 1},
				{BranchID: "d7", Decision: "rejected", TotalCents: 1},
				verifiedCand("win", "ow", 3000, "2026-11"),
				verifiedCand("lose-total", "ol", 3500, "2027-06"),
			},
			winner: "win",
			excluded: []BestExcluded{
				{BranchID: "d1", Reason: "Declined"},
				{BranchID: "d2", Reason: "No response"},
				{BranchID: "d3", Reason: "Dose equivalent rejected by rules"},
				{BranchID: "d4", Reason: "Photo did not match the government reference"},
				{BranchID: "d5", Reason: "No verified photo yet"},
				{BranchID: "d6", Reason: "Offer waiting for a human"},
				{BranchID: "d7", Reason: "Offer rejected by rules"},
				{BranchID: "lose-total", Reason: "Higher total price"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PickBestOffer(tc.cands)
			if tc.winner == "" {
				if got != nil {
					t.Fatalf("PickBestOffer = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("PickBestOffer = nil, want winner %q", tc.winner)
			}
			if got.BranchID != tc.winner {
				t.Fatalf("winner branch = %q, want %q", got.BranchID, tc.winner)
			}
			if !reflect.DeepEqual(got.Excluded, tc.excluded) {
				t.Fatalf("Excluded = %+v, want %+v", got.Excluded, tc.excluded)
			}
		})
	}
}

func TestPickBestOfferFields(t *testing.T) {
	got := PickBestOffer([]BestCandidate{
		verifiedCand("b1", "o1", 4200, "2027-02"),
	})
	if got == nil {
		t.Fatal("PickBestOffer = nil")
	}
	if got.OfferID != "o1" || got.TotalCents != 4200 || got.ExpiryMonth != "2027-02" {
		t.Fatalf("unexpected winner fields: %+v", got)
	}
	if got.Excluded == nil || len(got.Excluded) != 0 {
		t.Fatalf("Excluded = %#v, want empty non-nil slice", got.Excluded)
	}
}
