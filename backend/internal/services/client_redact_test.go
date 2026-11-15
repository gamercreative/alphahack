package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"alphahack/internal/queries"
)

// deskDetailStore serves one client-desk enquiry with a client event, a client
// notification and a hold message to target branch b.
type deskDetailStore struct{ queries.Querier }

var deskT0 = time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

func (deskDetailStore) GetEnquiry(context.Context, string) (queries.Enquiry, error) {
	return queries.Enquiry{ID: "e1", RequesterBranchID: queries.DeskBranchID, RequesterStaffID: queries.DeskStaffID,
		RawRequest: "need Augmentin 1000mg tablet x2, pickup Today before 18:00", Quantity: 2, HandledBy: "agent",
		Status: queries.EnquiryWaiting}, nil
}
func (deskDetailStore) ListEnquiryTargets(context.Context, string) ([]queries.EnquiryTarget, error) {
	return []queries.EnquiryTarget{{ID: "t-b", EnquiryID: "e1", BranchID: "branch-b", Status: queries.TargetSent}}, nil
}
func (deskDetailStore) GetBranch(_ context.Context, id string) (queries.Branch, error) {
	return queries.Branch{ID: id, Name: id}, nil
}
func (deskDetailStore) ListOutboxMessages(context.Context, string) ([]queries.OutboxMessage, error) {
	tb := "t-b"
	return []queries.OutboxMessage{
		{ID: "m1", Kind: queries.OutboxKindEnquiry, TargetID: &tb, Body: "Availability enquiry from DOSYN client desk", CreatedByActor: "agent"},
		{ID: "m2", Kind: queries.OutboxKindNotification, Body: "Maya, two pharmacies are checking", CreatedByActor: "agent"},
		{ID: "m3", Kind: queries.OutboxKindNotification, TargetID: &tb, Body: "Please hold", CreatedByActor: "client:client-maya"},
	}, nil
}
func (deskDetailStore) ListEnquiryEvents(context.Context, string, int64) ([]queries.EnquiryEvent, error) {
	p1, _ := json.Marshal(map[string]any{"ticket_id": "tk-1", "client_id": "client-maya", "quantity": 2})
	p2, _ := json.Marshal(map[string]any{"message_id": "m2", "body": "Maya, two pharmacies are checking"})
	return []queries.EnquiryEvent{
		{ID: "ev1", Seq: 1, EnquiryID: "e1", ActorType: queries.ActorClient, ActorID: "client-maya", Type: EvEnquiryCreated, Payload: p1, CreatedAt: deskT0},
		{ID: "ev2", Seq: 2, EnquiryID: "e1", ActorType: queries.ActorAgent, ActorID: "wassit", Type: EvRequesterNotified, Payload: p2, CreatedAt: deskT0},
	}, nil
}
func (deskDetailStore) ListEnquiryAgentRuns(context.Context, string) ([]queries.EnquiryAgentRun, error) {
	return nil, nil
}
func (deskDetailStore) ListEnquiryPhotos(context.Context, string) ([]queries.PackagePhoto, error) {
	return nil, nil
}

func TestDeskEnquiryHidesTheClient(t *testing.T) {
	svc := NewEnquiryService(deskDetailStore{}, EnquiryConfig{})
	target := Actor{Type: queries.ActorStaff, ID: "staff-b", BranchID: "branch-b"}

	d, err := svc.GetEnquiryDetail(context.Background(), target, "e1")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(d)
	for _, leak := range []string{"client-maya", "tk-1", "Maya"} {
		if strings.Contains(string(out), leak) {
			t.Fatalf("target pharmacy sees %q: %s", leak, out)
		}
	}
	if len(d.Messages) != 2 {
		t.Fatalf("want the enquiry + hold messages only, got %d", len(d.Messages))
	}

	evs, err := svc.EventsSince(context.Background(), target, "e1", 0)
	if err != nil {
		t.Fatal(err)
	}
	out, _ = json.Marshal(evs)
	if strings.Contains(string(out), "client-maya") || strings.Contains(string(out), "Maya") || strings.Contains(string(out), "tk-1") {
		t.Fatalf("stream leaks the client: %s", out)
	}

	// The agent keeps its own notification text but never sees the client id.
	ad, err := svc.GetEnquiryDetail(context.Background(), Actor{Type: queries.ActorAgent, ID: "wassit"}, "e1")
	if err != nil {
		t.Fatal(err)
	}
	out, _ = json.Marshal(ad.Events)
	if strings.Contains(string(out), "client-maya") || strings.Contains(string(out), "tk-1") {
		t.Fatalf("agent sees the client id: %s", out)
	}
	if len(ad.Messages) != 3 {
		t.Fatalf("agent should keep every message, got %d", len(ad.Messages))
	}

	// Desk staff (the requester side) see everything.
	desk := Actor{Type: queries.ActorStaff, ID: queries.DeskStaffID, BranchID: queries.DeskBranchID}
	dd, err := svc.GetEnquiryDetail(context.Background(), desk, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if out, _ := json.Marshal(dd); !strings.Contains(string(out), "client-maya") {
		t.Fatal("desk staff should see the unredacted trace")
	}
}
