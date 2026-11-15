package services

// Client medicine schedules: each cycle opens one client ticket (mode
// scheduled, wake trigger 'scheduled'). no_bulk: one ticket per cycle with that
// cycle's quantity; a cycle never opens while the previous ticket is still open.

import (
	"context"
	"time"

	"alphahack/internal/queries"
)

const (
	WakeScheduled      = "scheduled"
	EvScheduleRun      = "schedule_run" // logged on the new ticket's enquiry (actor client or system/scheduler)
	scheduleRunHour    = 9              // 09:00 Asia/Beirut
	dueSchedulesBatch  = 20
	scheduleEveryWeek  = "week"
	scheduleEvery2Week = "2weeks"
	scheduleEveryMonth = "month"
)

// SchedulerActor is the background scheduler.
var SchedulerActor = Actor{Type: queries.ActorSystem, ID: "scheduler"}

type CreateScheduleInput struct {
	MedicationID string `json:"medication_id"`
	Quantity     int    `json:"quantity"`
	Every        string `json:"every"`   // week | 2weeks | month
	Weekday      *int   `json:"weekday"` // 0 = Sunday .. 6 = Saturday
	PickupWindow string `json:"pickup_window"`
}

type UpdateScheduleInput struct {
	Status string `json:"status"` // active | paused
}

type ScheduleView struct {
	ID           string           `json:"id"`
	Medication   TicketMedication `json:"medication"`
	Quantity     int              `json:"quantity"`
	Every        string           `json:"every"`
	Weekday      int              `json:"weekday"`
	PickupWindow string           `json:"pickup_window"`
	NoBulk       bool             `json:"no_bulk"`
	Status       string           `json:"status"`
	NextRunAt    time.Time        `json:"next_run_at"`
	CreatedAt    time.Time        `json:"created_at"`
	TicketIDs    []string         `json:"ticket_ids"`
}

// nextWeekdayAt returns the first `weekday` 09:00 Beirut strictly after t.
func nextWeekdayAt(t time.Time, weekday int) time.Time {
	lt := t.In(beirut)
	c := time.Date(lt.Year(), lt.Month(), lt.Day(), scheduleRunHour, 0, 0, 0, beirut)
	c = c.AddDate(0, 0, (weekday-int(c.Weekday())+7)%7)
	if !c.After(t) {
		c = c.AddDate(0, 0, 7)
	}
	return c.UTC()
}

// nextRunAfter is the next cycle after a run at t: the next weekday 09:00, plus
// one more week for 2weeks and three more for month (14 / 28 days per cycle).
func nextRunAfter(t time.Time, every string, weekday int) time.Time {
	n := nextWeekdayAt(t, weekday)
	switch every {
	case scheduleEvery2Week:
		n = n.AddDate(0, 0, 7)
	case scheduleEveryMonth:
		n = n.AddDate(0, 0, 21)
	}
	return n
}

func (s *EnquiryService) scheduleView(ctx context.Context, q queries.Querier, m queries.MedicineSchedule) (ScheduleView, error) {
	med, err := q.GetMedication(ctx, m.MedicationID)
	if err != nil {
		return ScheduleView{}, FromStore(err, "medication")
	}
	tickets, err := q.ListScheduleTickets(ctx, m.ID)
	if err != nil {
		return ScheduleView{}, FromStore(err, "ticket")
	}
	ids := make([]string, 0, len(tickets))
	for _, t := range tickets {
		ids = append(ids, t.ID)
	}
	return ScheduleView{ID: m.ID, Medication: ticketMed(med), Quantity: m.Quantity, Every: m.Every,
		Weekday: m.Weekday, PickupWindow: m.PickupWindow, NoBulk: m.NoBulk, Status: m.Status,
		NextRunAt: m.NextRunAt, CreatedAt: m.CreatedAt, TicketIDs: ids}, nil
}

func (s *EnquiryService) CreateSchedule(ctx context.Context, a Actor, in CreateScheduleInput) (ScheduleView, error) {
	if err := clientOnlyErr(a); err != nil {
		return ScheduleView{}, err
	}
	if err := validateTicketQuantity(in.Quantity); err != nil {
		return ScheduleView{}, err
	}
	pickup, err := validatePickup(in.PickupWindow)
	if err != nil {
		return ScheduleView{}, err
	}
	every := clean(in.Every)
	switch every {
	case scheduleEveryWeek, scheduleEvery2Week, scheduleEveryMonth:
	default:
		return ScheduleView{}, Invalid("every must be week, 2weeks or month")
	}
	if in.Weekday == nil || *in.Weekday < 0 || *in.Weekday > 6 {
		return ScheduleView{}, Invalid("weekday is required: 0 (Sunday) to 6 (Saturday)")
	}
	medID := clean(in.MedicationID)
	if medID == "" {
		return ScheduleView{}, Invalid("medication_id is required; pick one from GET /api/v1/medications")
	}
	if _, err := s.store.GetMedication(ctx, medID); err != nil {
		return ScheduleView{}, FromStore(err, "medication")
	}
	m, err := s.store.CreateMedicineSchedule(ctx, queries.CreateMedicineScheduleParams{
		ClientID: a.ID, MedicationID: medID, Quantity: in.Quantity, Every: every, Weekday: *in.Weekday,
		PickupWindow: pickup, NextRunAt: nextWeekdayAt(s.now(), *in.Weekday),
	})
	if err != nil {
		return ScheduleView{}, FromStore(err, "schedule")
	}
	return s.scheduleView(ctx, s.store, m)
}

func (s *EnquiryService) ListSchedules(ctx context.Context, a Actor, p Page) ([]ScheduleView, error) {
	if err := clientOnlyErr(a); err != nil {
		return nil, err
	}
	p = p.Normalize()
	rows, err := s.store.ListClientSchedules(ctx, a.ID, p.Limit, p.Offset)
	if err != nil {
		return nil, FromStore(err, "schedule")
	}
	out := make([]ScheduleView, 0, len(rows))
	for _, m := range rows {
		v, err := s.scheduleView(ctx, s.store, m)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *EnquiryService) ownSchedule(ctx context.Context, q queries.Querier, a Actor, id string) (queries.MedicineSchedule, error) {
	m, err := q.GetMedicineScheduleForUpdate(ctx, id)
	if err != nil {
		return m, FromStore(err, "schedule")
	}
	if m.ClientID != a.ID {
		return m, NotFound("schedule")
	}
	return m, nil
}

// UpdateSchedule pauses or resumes a schedule. Resuming a schedule whose next
// run has passed moves it to the next weekday 09:00 (missed cycles never pile up).
func (s *EnquiryService) UpdateSchedule(ctx context.Context, a Actor, id string, in UpdateScheduleInput) (ScheduleView, error) {
	if err := clientOnlyErr(a); err != nil {
		return ScheduleView{}, err
	}
	status := clean(in.Status)
	if status != "active" && status != "paused" {
		return ScheduleView{}, Invalid("status must be active or paused")
	}
	var out ScheduleView
	err := s.enquiryTx(ctx, func(q queries.Querier) error {
		m, err := s.ownSchedule(ctx, q, a, id)
		if err != nil {
			return err
		}
		next := m.NextRunAt
		if status == "active" && !next.After(s.now()) {
			next = nextWeekdayAt(s.now(), m.Weekday)
		}
		m, err = q.UpdateMedicineSchedule(ctx, m.ID, status, next)
		if err != nil {
			return FromStore(err, "schedule")
		}
		out, err = s.scheduleView(ctx, q, m)
		return err
	})
	return out, err
}

// openScheduleTicket reports whether a ticket of the schedule is still open.
func openScheduleTicket(ctx context.Context, q queries.Querier, scheduleID string) (bool, error) {
	tickets, err := q.ListScheduleTickets(ctx, scheduleID)
	if err != nil {
		return false, FromStore(err, "ticket")
	}
	for _, t := range tickets {
		if t.Status != queries.TicketOpen {
			continue
		}
		e, err := q.GetEnquiry(ctx, t.EnquiryID)
		if err != nil {
			return false, FromStore(err, "enquiry")
		}
		if !isFinal(e) {
			return true, nil
		}
	}
	return false, nil
}

// runSchedule opens this cycle's ticket and moves next_run_at to the next cycle.
func (s *EnquiryService) runSchedule(ctx context.Context, q queries.Querier, m queries.MedicineSchedule, by Actor) (TicketView, error) {
	sid := m.ID
	t, err := s.openTicket(ctx, q, m.ClientID, m.MedicationID, m.Quantity, m.PickupWindow, queries.TicketScheduled, &sid, WakeScheduled)
	if err != nil {
		return TicketView{}, err
	}
	if _, err := s.logEvent(ctx, q, t.EnquiryID, by, EvScheduleRun,
		map[string]any{"schedule_id": m.ID, "ticket_id": t.ID, "quantity": m.Quantity}, nil); err != nil {
		return TicketView{}, err
	}
	if _, err := q.UpdateMedicineSchedule(ctx, m.ID, m.Status, nextRunAfter(s.now(), m.Every, m.Weekday)); err != nil {
		return TicketView{}, FromStore(err, "schedule")
	}
	return t, nil
}

// RunScheduleNow opens this cycle's ticket now (client, owner). 409 while a
// ticket of the schedule is still open: cycles are never merged (no bulk).
func (s *EnquiryService) RunScheduleNow(ctx context.Context, a Actor, id string) (TicketView, error) {
	if err := clientOnlyErr(a); err != nil {
		return TicketView{}, err
	}
	var out TicketView
	err := s.enquiryTx(ctx, func(q queries.Querier) error {
		m, err := s.ownSchedule(ctx, q, a, id)
		if err != nil {
			return err
		}
		if open, err := openScheduleTicket(ctx, q, m.ID); err != nil {
			return err
		} else if open {
			return Conflict("this schedule's current ticket is still open; one ticket per cycle, cycles are never merged (no bulk)")
		}
		out, err = s.runSchedule(ctx, q, m, Actor{Type: queries.ActorClient, ID: a.ID})
		return err
	})
	return out, err
}

// RunDueSchedules opens a ticket for every active schedule whose next_run_at
// has passed. A cycle whose previous ticket is still open is skipped (no bulk).
// Returns the number of tickets opened.
func (s *EnquiryService) RunDueSchedules(ctx context.Context) (int, error) {
	opened := 0
	err := s.enquiryTx(ctx, func(q queries.Querier) error {
		due, err := q.ListDueSchedules(ctx, dueSchedulesBatch)
		if err != nil {
			return FromStore(err, "schedule")
		}
		for _, m := range due {
			open, err := openScheduleTicket(ctx, q, m.ID)
			if err != nil {
				return err
			}
			if open {
				if _, err := q.UpdateMedicineSchedule(ctx, m.ID, m.Status, nextRunAfter(s.now(), m.Every, m.Weekday)); err != nil {
					return FromStore(err, "schedule")
				}
				continue
			}
			if _, err := s.runSchedule(ctx, q, m, SchedulerActor); err != nil {
				return err
			}
			opened++
		}
		return nil
	})
	return opened, err
}

// RunScheduler calls RunDueSchedules every interval until ctx is done.
func (s *EnquiryService) RunScheduler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := s.RunDueSchedules(ctx); err != nil && ctx.Err() == nil {
				s.cfg.Logger.Error("schedules", "err", err)
			} else if n > 0 {
				s.cfg.Logger.Info("schedules opened tickets", "count", n)
			}
		}
	}
}
