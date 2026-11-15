package queries

import (
	"context"
	"encoding/json"
)

// Structured offers (target_offers, migration 0006).

var (
	qCreateTargetOffer = mustSQL("create_target_offer")
	qListTargetOffers  = mustSQL("list_target_offers")
)

func scanTargetOffer(r rowScanner) (TargetOffer, error) {
	var o TargetOffer
	var evidence, reasons []byte
	err := r.Scan(
		&o.ID, &o.EnquiryID, &o.TargetID, &o.BranchID, &o.SourceText, &o.ExtractedBy,
		&o.InStock, &o.Strength, &o.Quantity, &o.PriceCents, &o.Currency, &o.PickupBy,
		&o.Payment, &o.NeedsPrescription, &evidence, &o.Confidence, &o.Decision,
		&o.DecidedBy, &reasons, &o.CreatedAt,
	)
	o.Evidence = json.RawMessage(evidence)
	o.Reasons = json.RawMessage(reasons)
	return o, err
}

func jsonOr(b json.RawMessage, empty string) []byte {
	if len(b) == 0 {
		return []byte(empty)
	}
	return b
}

func (q *Queries) CreateTargetOffer(ctx context.Context, p CreateTargetOfferParams) (TargetOffer, error) {
	return scanOne(q.db.QueryRow(ctx, qCreateTargetOffer,
		p.EnquiryID, p.TargetID, p.BranchID, p.SourceText, p.ExtractedBy, p.InStock,
		p.Strength, p.Quantity, p.PriceCents, p.Currency, p.PickupBy, p.Payment,
		p.NeedsPrescription, jsonOr(p.Evidence, "{}"), p.Confidence, p.Decision,
		p.DecidedBy, jsonOr(p.Reasons, "[]"),
	), "create target offer", scanTargetOffer)
}

func (q *Queries) ListTargetOffers(ctx context.Context, enquiryID string) ([]TargetOffer, error) {
	rows, err := q.db.Query(ctx, qListTargetOffers, enquiryID)
	if err != nil {
		return nil, wrap("list target offers", err)
	}
	return collect(rows, "list target offers", scanTargetOffer)
}
