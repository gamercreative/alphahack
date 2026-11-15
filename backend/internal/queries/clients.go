package queries

import (
	"context"
	"time"
)

// DOSYN clients, client tickets, medicine schedules and package photos
// (migrations 0008-0010).

var (
	qGetClientByPhone             = mustSQL("get_client_by_phone")
	qGetClient                    = mustSQL("get_client")
	qCreateClientSession          = mustSQL("create_client_session")
	qGetClientBySessionToken      = mustSQL("get_client_by_session_token")
	qCreateClientTicket           = mustSQL("create_client_ticket")
	qGetClientTicket              = mustSQL("get_client_ticket")
	qGetClientTicketByEnquiry     = mustSQL("get_client_ticket_by_enquiry")
	qListClientTickets            = mustSQL("list_client_tickets")
	qListScheduleTickets          = mustSQL("list_schedule_tickets")
	qUpdateClientTicketStatus     = mustSQL("update_client_ticket_status")
	qCreateMedicineSchedule       = mustSQL("create_medicine_schedule")
	qGetMedicineSchedule          = mustSQL("get_medicine_schedule")
	qGetMedicineScheduleForUpdate = mustSQL("get_medicine_schedule_for_update")
	qListClientSchedules          = mustSQL("list_client_schedules")
	qUpdateMedicineSchedule       = mustSQL("update_medicine_schedule")
	qListDueSchedules             = mustSQL("list_due_schedules")
	qCreatePackagePhoto           = mustSQL("create_package_photo")
	qGetPackagePhoto              = mustSQL("get_package_photo")
	qListEnquiryPhotos            = mustSQL("list_enquiry_photos")
	qReviewPackagePhoto           = mustSQL("review_package_photo")
	qGetGovProduct                = mustSQL("get_gov_product")
	qRecordPhotoAgentReading      = mustSQL("record_photo_agent_reading")
)

func scanClient(r rowScanner) (Client, error) {
	var c Client
	err := r.Scan(&c.ID, &c.DisplayName, &c.Phone, &c.PinHash, &c.CreatedAt)
	return c, err
}

func scanClientTicket(r rowScanner) (ClientTicket, error) {
	var t ClientTicket
	err := r.Scan(&t.ID, &t.ClientID, &t.EnquiryID, &t.ScheduleID, &t.Mode, &t.PickupWindow,
		&t.Status, &t.ReservedBranchID, &t.CreatedAt)
	return t, err
}

func scanMedicineSchedule(r rowScanner) (MedicineSchedule, error) {
	var m MedicineSchedule
	err := r.Scan(&m.ID, &m.ClientID, &m.MedicationID, &m.Quantity, &m.Every, &m.Weekday,
		&m.PickupWindow, &m.NoBulk, &m.Status, &m.NextRunAt, &m.CreatedAt)
	return m, err
}

func scanPackagePhoto(r rowScanner) (PackagePhoto, error) {
	var p PackagePhoto
	err := r.Scan(&p.ID, &p.TargetID, &p.BranchID, &p.StoragePath, &p.ContentType, &p.SizeBytes,
		&p.TypedCode, &p.TypedExpiryMonth, &p.Status, &p.GovMatch, &p.DecidedBy, &p.Note,
		&p.ReviewedByStaffID, &p.AgentCode, &p.AgentExpiryMonth, &p.AgentConfidence, &p.ReadAt, &p.CreatedAt)
	return p, err
}

// ---- clients + sessions -----------------------------------------------------

func (q *Queries) GetClientByPhone(ctx context.Context, phone string) (Client, error) {
	c, err := scanClient(q.db.QueryRow(ctx, qGetClientByPhone, phone))
	return c, wrap("get client by phone", err)
}

func (q *Queries) GetClient(ctx context.Context, id string) (Client, error) {
	c, err := scanClient(q.db.QueryRow(ctx, qGetClient, id))
	return c, wrap("get client", err)
}

func (q *Queries) CreateClientSession(ctx context.Context, p CreateClientSessionParams) error {
	_, err := q.db.Exec(ctx, qCreateClientSession, p.TokenHash, p.ClientID, p.ExpiresAt)
	return wrap("create client session", err)
}

func (q *Queries) GetClientBySessionToken(ctx context.Context, tokenHash string) (Client, error) {
	c, err := scanClient(q.db.QueryRow(ctx, qGetClientBySessionToken, tokenHash))
	return c, wrap("get client by session", err)
}

// ---- tickets -------------------------------------------------------------------

func (q *Queries) CreateClientTicket(ctx context.Context, p CreateClientTicketParams) (ClientTicket, error) {
	t, err := scanClientTicket(q.db.QueryRow(ctx, qCreateClientTicket,
		p.ClientID, p.EnquiryID, p.ScheduleID, p.Mode, p.PickupWindow))
	if err != nil {
		return t, constraint("create client ticket", err)
	}
	return t, nil
}

func (q *Queries) GetClientTicket(ctx context.Context, id string) (ClientTicket, error) {
	t, err := scanClientTicket(q.db.QueryRow(ctx, qGetClientTicket, id))
	return t, wrap("get client ticket", err)
}

func (q *Queries) GetClientTicketByEnquiry(ctx context.Context, enquiryID string) (ClientTicket, error) {
	t, err := scanClientTicket(q.db.QueryRow(ctx, qGetClientTicketByEnquiry, enquiryID))
	return t, wrap("get client ticket by enquiry", err)
}

func (q *Queries) ListClientTickets(ctx context.Context, clientID, status string, limit, offset int) ([]ClientTicket, error) {
	rows, err := q.db.Query(ctx, qListClientTickets, clientID, status, limit, offset)
	if err != nil {
		return nil, wrap("list client tickets", err)
	}
	return collect(rows, "list client tickets", scanClientTicket)
}

func (q *Queries) ListScheduleTickets(ctx context.Context, scheduleID string) ([]ClientTicket, error) {
	rows, err := q.db.Query(ctx, qListScheduleTickets, scheduleID)
	if err != nil {
		return nil, wrap("list schedule tickets", err)
	}
	return collect(rows, "list schedule tickets", scanClientTicket)
}

func (q *Queries) UpdateClientTicketStatus(ctx context.Context, id, status string, reservedBranchID *string) (ClientTicket, error) {
	t, err := scanClientTicket(q.db.QueryRow(ctx, qUpdateClientTicketStatus, id, status, reservedBranchID))
	return t, wrap("update client ticket status", err)
}

// ---- schedules -----------------------------------------------------------------

func (q *Queries) CreateMedicineSchedule(ctx context.Context, p CreateMedicineScheduleParams) (MedicineSchedule, error) {
	m, err := scanMedicineSchedule(q.db.QueryRow(ctx, qCreateMedicineSchedule,
		p.ClientID, p.MedicationID, p.Quantity, p.Every, p.Weekday, p.PickupWindow, p.NextRunAt))
	if err != nil {
		return m, constraint("create medicine schedule", err)
	}
	return m, nil
}

func (q *Queries) GetMedicineSchedule(ctx context.Context, id string) (MedicineSchedule, error) {
	m, err := scanMedicineSchedule(q.db.QueryRow(ctx, qGetMedicineSchedule, id))
	return m, wrap("get medicine schedule", err)
}

func (q *Queries) GetMedicineScheduleForUpdate(ctx context.Context, id string) (MedicineSchedule, error) {
	m, err := scanMedicineSchedule(q.db.QueryRow(ctx, qGetMedicineScheduleForUpdate, id))
	return m, wrap("get medicine schedule for update", err)
}

func (q *Queries) ListClientSchedules(ctx context.Context, clientID string, limit, offset int) ([]MedicineSchedule, error) {
	rows, err := q.db.Query(ctx, qListClientSchedules, clientID, limit, offset)
	if err != nil {
		return nil, wrap("list client schedules", err)
	}
	return collect(rows, "list client schedules", scanMedicineSchedule)
}

func (q *Queries) UpdateMedicineSchedule(ctx context.Context, id, status string, nextRunAt time.Time) (MedicineSchedule, error) {
	m, err := scanMedicineSchedule(q.db.QueryRow(ctx, qUpdateMedicineSchedule, id, status, nextRunAt))
	return m, wrap("update medicine schedule", err)
}

func (q *Queries) ListDueSchedules(ctx context.Context, limit int) ([]MedicineSchedule, error) {
	rows, err := q.db.Query(ctx, qListDueSchedules, limit)
	if err != nil {
		return nil, wrap("list due schedules", err)
	}
	return collect(rows, "list due schedules", scanMedicineSchedule)
}

// ---- package photos ------------------------------------------------------------

func (q *Queries) CreatePackagePhoto(ctx context.Context, p CreatePackagePhotoParams) (PackagePhoto, error) {
	ph, err := scanPackagePhoto(q.db.QueryRow(ctx, qCreatePackagePhoto,
		p.ID, p.TargetID, p.BranchID, p.StoragePath, p.ContentType, p.SizeBytes,
		p.TypedCode, p.TypedExpiryMonth, p.Status, p.GovMatch, p.Note))
	if err != nil {
		return ph, constraint("create package photo", err)
	}
	return ph, nil
}

func (q *Queries) GetPackagePhoto(ctx context.Context, id string) (PackagePhoto, error) {
	p, err := scanPackagePhoto(q.db.QueryRow(ctx, qGetPackagePhoto, id))
	return p, wrap("get package photo", err)
}

func (q *Queries) ListEnquiryPhotos(ctx context.Context, enquiryID string) ([]PackagePhoto, error) {
	rows, err := q.db.Query(ctx, qListEnquiryPhotos, enquiryID)
	if err != nil {
		return nil, wrap("list enquiry photos", err)
	}
	return collect(rows, "list enquiry photos", scanPackagePhoto)
}

func (q *Queries) ReviewPackagePhoto(ctx context.Context, p ReviewPackagePhotoParams) (PackagePhoto, error) {
	ph, err := scanPackagePhoto(q.db.QueryRow(ctx, qReviewPackagePhoto, p.ID, p.Status, p.Note, p.ReviewedByStaffID))
	return ph, wrap("review package photo", err)
}

func (q *Queries) GetGovProduct(ctx context.Context, code string) (GovProduct, error) {
	g, err := scanGovProduct(q.db.QueryRow(ctx, qGetGovProduct, code))
	return g, wrap("get gov product", err)
}

func (q *Queries) RecordPhotoAgentReading(ctx context.Context, p RecordPhotoAgentReadingParams) (PackagePhoto, error) {
	ph, err := scanPackagePhoto(q.db.QueryRow(ctx, qRecordPhotoAgentReading,
		p.ID, p.AgentCode, p.AgentExpiryMonth, p.AgentConfidence, p.Status, p.GovMatch, p.Note))
	return ph, wrap("record photo agent reading", err)
}
