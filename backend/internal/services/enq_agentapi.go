package services

import (
	"context"
	"encoding/json"
	"math"
	"time"
	"unicode/utf8"

	"alphahack/internal/queries"
)

// Wassit agent API: wake-ups, runs, tool-call trace and wait() (see CONTRACT.md).

const (
	maxToolCallOutputBytes = 4000
	wakeupBatchLimit       = 10
)

// Wakeups returns the agent-handled enquiries that need a run, oldest first.
func (s *EnquiryService) Wakeups(ctx context.Context) ([]queries.AgentWakeup, error) {
	items, err := s.store.ListAgentWakeups(ctx, wakeupBatchLimit)
	if err != nil {
		return nil, FromStore(err, "wakeups")
	}
	if items == nil {
		items = []queries.AgentWakeup{}
	}
	return items, nil
}

// StartRun clears the wake of an agent enquiry and opens a run record for it.
func (s *EnquiryService) StartRun(ctx context.Context, enquiryID, trigger, model string) (queries.EnquiryAgentRun, error) {
	trigger = clean(trigger)
	model = clean(model)
	switch trigger {
	case WakeCreated, WakeClarified, WakeApproved, WakeReply, "timer":
	default:
		return queries.EnquiryAgentRun{}, Invalid("trigger must be created, clarified, approved, reply or timer")
	}
	if model == "" {
		return queries.EnquiryAgentRun{}, Invalid("model is required")
	}

	var run queries.EnquiryAgentRun
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		e, err := q.GetEnquiryForUpdate(ctx, enquiryID)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if e.HandledBy != "agent" {
			return Forbidden("this enquiry is handled manually")
		}
		if isFinal(e) {
			return Conflict("this enquiry is closed")
		}
		if err := q.ClearEnquiryWake(ctx, enquiryID); err != nil {
			return FromStore(err, "enquiry")
		}
		r, err := q.CreateEnquiryAgentRun(ctx, queries.CreateEnquiryAgentRunParams{
			EnquiryID: enquiryID, Trigger: trigger, Model: model,
		})
		if err != nil {
			return FromStore(err, "run")
		}
		run = r
		return nil
	})
	if err != nil {
		return queries.EnquiryAgentRun{}, err
	}
	return run, nil
}

// FinishRunInput is the body of PATCH /api/v1/agent/runs/{id}.
type FinishRunInput struct {
	Status       string `json:"status"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Steps        int    `json:"steps"`
	Error        string `json:"error"`
}

// costMicros converts token counts to micro-USD. Prices are USD per million tokens,
// so tokens * price is already micro-USD.
func costMicros(in, out int, priceIn, priceOut float64) int64 {
	return int64(math.Round(float64(in)*priceIn + float64(out)*priceOut))
}

// FinishRun closes a run with its totals and the cost computed from the configured prices.
func (s *EnquiryService) FinishRun(ctx context.Context, runID string, in FinishRunInput) (queries.EnquiryAgentRun, error) {
	if in.Status != "completed" && in.Status != "failed" {
		return queries.EnquiryAgentRun{}, Invalid("status must be completed or failed")
	}
	if in.InputTokens < 0 || in.OutputTokens < 0 || in.Steps < 0 ||
		in.InputTokens > math.MaxInt32 || in.OutputTokens > math.MaxInt32 || in.Steps > math.MaxInt32 {
		return queries.EnquiryAgentRun{}, Invalid("input_tokens, output_tokens and steps must be between 0 and %d", math.MaxInt32)
	}
	finishedAt := s.now()
	var run queries.EnquiryAgentRun
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		existing, err := q.GetEnquiryAgentRun(ctx, runID)
		if err != nil {
			return FromStore(err, "run")
		}
		if _, err := q.GetEnquiryForUpdate(ctx, existing.EnquiryID); err != nil {
			return FromStore(err, "enquiry")
		}
		existing, err = q.GetEnquiryAgentRun(ctx, runID)
		if err != nil {
			return FromStore(err, "run")
		}
		if existing.Status != queries.RunStatusRunning {
			return Conflict("this agent run is already finished")
		}
		r, err := q.FinishEnquiryAgentRun(ctx, queries.FinishEnquiryAgentRunParams{
			ID:           runID,
			Status:       in.Status,
			InputTokens:  in.InputTokens,
			OutputTokens: in.OutputTokens,
			Steps:        in.Steps,
			CostMicros: costMicros(in.InputTokens, in.OutputTokens,
				s.cfg.AgentPriceInPerMTok, s.cfg.AgentPriceOutPerMTok),
			Error:      in.Error,
			FinishedAt: &finishedAt,
		})
		if err != nil {
			return FromStore(err, "run")
		}
		run = r
		return nil
	})
	if err != nil {
		return queries.EnquiryAgentRun{}, err
	}
	return run, nil
}

// ToolCallInput is the body of POST /api/v1/agent/runs/{id}/tool-calls.
type ToolCallInput struct {
	Name       string          `json:"name"`
	Input      json.RawMessage `json:"input"`
	Output     string          `json:"output"`
	IsError    bool            `json:"is_error"`
	DurationMs int             `json:"duration_ms"`
}

// RecordToolCall stores one tool call of a run. Output is cut to 4000 bytes on a
// UTF-8 boundary so the stored text stays valid.
func (s *EnquiryService) RecordToolCall(ctx context.Context, runID string, in ToolCallInput) (queries.EnquiryAgentToolCall, error) {
	name := clean(in.Name)
	if name == "" {
		return queries.EnquiryAgentToolCall{}, Invalid("name is required")
	}
	if in.DurationMs < 0 || in.DurationMs > math.MaxInt32 {
		return queries.EnquiryAgentToolCall{}, Invalid("duration_ms must be between 0 and %d", math.MaxInt32)
	}
	output := in.Output
	if len(output) > maxToolCallOutputBytes {
		cut := maxToolCallOutputBytes
		for cut > 0 && !utf8.RuneStart(output[cut]) {
			cut--
		}
		output = output[:cut]
	}
	run, err := s.store.GetEnquiryAgentRun(ctx, runID)
	if err != nil {
		return queries.EnquiryAgentToolCall{}, FromStore(err, "run")
	}
	if run.Status != queries.RunStatusRunning {
		return queries.EnquiryAgentToolCall{}, Conflict("this agent run is already finished")
	}
	call, err := s.store.CreateEnquiryAgentToolCall(ctx, queries.CreateEnquiryAgentToolCallParams{
		RunID:      runID,
		Name:       name,
		Input:      in.Input,
		Output:     output,
		IsError:    in.IsError,
		DurationMs: in.DurationMs,
	})
	if err != nil {
		return queries.EnquiryAgentToolCall{}, FromStore(err, "tool call")
	}
	return call, nil
}

// Wait parks an agent enquiry for minutes (5, 15 or 30 wait units) and records wait_set.
func (s *EnquiryService) Wait(ctx context.Context, a Actor, id string, minutes int) (queries.Enquiry, error) {
	if minutes != 5 && minutes != 15 && minutes != 30 {
		return queries.Enquiry{}, Invalid("minutes must be 5, 15 or 30")
	}

	var out queries.Enquiry
	err := s.store.InTx(ctx, func(q queries.Querier) error {
		e, err := q.GetEnquiryForUpdate(ctx, id)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if err := canAct(a, e); err != nil {
			return err
		}
		if isFinal(e) {
			return Conflict("this enquiry is closed")
		}

		next := s.now().Add(time.Duration(minutes) * s.cfg.WaitUnit)
		u := toUpdate(e)
		u.NextWakeAt = &next
		if e.Status == queries.EnquiryOpen {
			u.Status = queries.EnquiryWaiting
		}
		updated, err := q.UpdateEnquiry(ctx, u)
		if err != nil {
			return FromStore(err, "enquiry")
		}
		if _, err := s.logEvent(ctx, q, id, a, EvWaitSet,
			map[string]any{"minutes": minutes, "next_wake_at": next}, nil); err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return queries.Enquiry{}, err
	}
	return out, nil
}
