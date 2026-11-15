package services

// Best offer for a client ticket (CONTRACT rule 11). Computed in Go from verified
// offers only: lowest total price, then latest expiry, then lower branch id.
// The model never ranks offers. Pure function: no database, no HTTP.

// BestCandidate is one target's offer as the ranking sees it.
type BestCandidate struct {
	BranchID     string
	OfferID      string
	Decision     string // rules verdict: matches | escalated | rejected | ""
	PhotoStatus  string // verified | flagged | mismatch | ""
	TotalCents   int64
	HasTotal     bool
	ExpiryMonth  string // "YYYY-MM"; empty is the earliest
	Declined     bool
	NoResponse   bool
	DoseRejected bool
}

// BestExcluded says why a candidate did not win.
type BestExcluded struct {
	BranchID string `json:"branch_id"`
	Reason   string `json:"reason"`
}

// BestOffer is the winning verified offer plus every other candidate's reason.
type BestOffer struct {
	BranchID    string         `json:"branch_id"`
	OfferID     string         `json:"offer_id"`
	TotalCents  int64          `json:"total_cents"`
	ExpiryMonth string         `json:"expiry_month"`
	Excluded    []BestExcluded `json:"excluded"`
}

// bestVerified: a candidate can only win when the rules matched it, the photo is
// verified, it has a total, and no decline, silence or dose rejection applies.
func bestVerified(c BestCandidate) bool {
	return !c.Declined && !c.NoResponse && !c.DoseRejected &&
		c.Decision == "matches" && c.PhotoStatus == "verified" && c.HasTotal
}

// bestBetter reports whether a beats b: lower total, then later expiry, then lower branch id.
func bestBetter(a, b BestCandidate) bool {
	if a.TotalCents != b.TotalCents {
		return a.TotalCents < b.TotalCents
	}
	if a.ExpiryMonth != b.ExpiryMonth {
		return a.ExpiryMonth > b.ExpiryMonth
	}
	return a.BranchID < b.BranchID
}

// bestExcludeReason returns the first matching reason for a candidate that did not win.
func bestExcludeReason(c BestCandidate, w BestCandidate) string {
	switch {
	case c.Declined:
		return "Declined"
	case c.NoResponse:
		return "No response"
	case c.DoseRejected:
		return "Dose equivalent rejected by rules"
	case c.Decision == "matches" && c.PhotoStatus == "flagged":
		return "Photo awaiting human check"
	case c.Decision == "matches" && c.PhotoStatus == "mismatch":
		return "Photo did not match the government reference"
	case c.Decision == "matches" && c.PhotoStatus == "":
		return "No verified photo yet"
	case c.Decision == "escalated":
		return "Offer waiting for a human"
	case c.Decision == "rejected":
		return "Offer rejected by rules"
	}
	if c.Decision == "matches" && c.PhotoStatus == "verified" && !c.HasTotal {
		return "No total price yet"
	}
	if !bestVerified(c) {
		return "Not verified"
	}
	// Verified but lost to the winner.
	if c.TotalCents == w.TotalCents {
		return "Earlier expiry"
	}
	return "Higher total price"
}

// PickBestOffer returns the winning verified candidate, or nil when none is verified.
// Excluded lists every other candidate once, in input order.
func PickBestOffer(cands []BestCandidate) *BestOffer {
	win := -1
	for i, c := range cands {
		if !bestVerified(c) {
			continue
		}
		if win < 0 || bestBetter(c, cands[win]) {
			win = i
		}
	}
	if win < 0 {
		return nil
	}
	w := cands[win]
	out := &BestOffer{
		BranchID:    w.BranchID,
		OfferID:     w.OfferID,
		TotalCents:  w.TotalCents,
		ExpiryMonth: w.ExpiryMonth,
		Excluded:    make([]BestExcluded, 0, len(cands)-1),
	}
	for i, c := range cands {
		if i == win {
			continue
		}
		out.Excluded = append(out.Excluded, BestExcluded{
			BranchID: c.BranchID,
			Reason:   bestExcludeReason(c, w),
		})
	}
	return out
}
