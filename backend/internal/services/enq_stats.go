package services

import (
	"context"

	"alphahack/internal/queries"
)

// Wassit stats and event feed (see CONTRACT.md).

// StatTotals is summed over the enquiries of one stats page.
type StatTotals struct {
	Enquiries      int   `json:"enquiries"`
	AgentEnquiries int   `json:"agent_enquiries"`
	HITLCount      int   `json:"hitl_count"`
	AgentRuns      int   `json:"agent_runs"`
	AgentSteps     int   `json:"agent_steps"`
	InputTokens    int   `json:"input_tokens"`
	OutputTokens   int   `json:"output_tokens"`
	CostMicros     int64 `json:"cost_micros"`
}

// StatsResult is the body of GET /api/v1/stats.
type StatsResult struct {
	Data   []queries.EnquiryStat `json:"data"`
	Totals StatTotals            `json:"totals"`
}

// Stats returns one row per enquiry (newest first) plus totals for that page.
func (s *EnquiryService) Stats(ctx context.Context, page Page) (StatsResult, error) {
	p := page.Normalize()
	rows, err := s.store.ListEnquiryStats(ctx, p.Limit, p.Offset)
	if err != nil {
		return StatsResult{}, FromStore(err, "stats")
	}
	if rows == nil {
		rows = []queries.EnquiryStat{}
	}

	var t StatTotals
	for _, r := range rows {
		t.Enquiries++
		if r.HandledBy == "agent" {
			t.AgentEnquiries++
		}
		t.HITLCount += r.HITLCount
		t.AgentRuns += r.AgentRuns
		t.AgentSteps += r.AgentSteps
		t.InputTokens += r.InputTokens
		t.OutputTokens += r.OutputTokens
		t.CostMicros += r.CostMicros
	}
	return StatsResult{Data: rows, Totals: t}, nil
}

// EventsSince returns the events of an enquiry with seq > afterSeq. Allowed for the
// requesting branch and for branches that are targets of the enquiry.
func (s *EnquiryService) EventsSince(ctx context.Context, a Actor, enquiryID string, afterSeq int64) ([]queries.EnquiryEvent, error) {
	e, err := s.store.GetEnquiry(ctx, enquiryID)
	if err != nil {
		return nil, FromStore(err, "enquiry")
	}
	if a.BranchID != e.RequesterBranchID {
		targets, err := s.store.ListEnquiryTargets(ctx, enquiryID)
		if err != nil {
			return nil, FromStore(err, "enquiry")
		}
		isTarget := false
		for _, t := range targets {
			if t.BranchID == a.BranchID {
				isTarget = true
				break
			}
		}
		if !isTarget {
			return nil, Forbidden("you cannot view this enquiry")
		}
	}

	events, err := s.store.ListEnquiryEvents(ctx, enquiryID, afterSeq)
	if err != nil {
		return nil, FromStore(err, "enquiry")
	}
	return events, nil
}
