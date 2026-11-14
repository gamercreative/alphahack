package queries

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Staff login/session integration tests run against the seed in
// db/migrations/0005_enquiry_seed.up.sql (staff-hamra, a@wassit.demo, branch-hamra).
// Session rows are keyed by unique token hashes and deleted at cleanup.

func newStaffSession(t *testing.T, q *Queries, tokenHash, staffID string, expiresAt time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := q.CreateStaffSession(ctx, CreateStaffSessionParams{
		TokenHash: tokenHash,
		StaffID:   staffID,
		ExpiresAt: expiresAt,
	}); err != nil {
		t.Fatalf("CreateStaffSession: %v", err)
	}
	t.Cleanup(func() {
		_, _ = q.db.Exec(context.Background(), "DELETE FROM staff_sessions WHERE token_hash = $1", tokenHash)
	})
}

func TestIntegrationStaffSessions(t *testing.T) {
	q := testQueries(t)
	ctx := context.Background()

	t.Run("staff_by_email_found", func(t *testing.T) {
		s, err := q.GetStaffByEmail(ctx, "a@wassit.demo")
		if err != nil {
			t.Fatalf("GetStaffByEmail(a@wassit.demo) err = %v, want nil", err)
		}
		if s.ID != "staff-hamra" {
			t.Fatalf("ID = %q, want %q", s.ID, "staff-hamra")
		}
		if s.BranchID != "branch-hamra" {
			t.Fatalf("BranchID = %q, want %q", s.BranchID, "branch-hamra")
		}
	})

	t.Run("staff_by_unknown_email", func(t *testing.T) {
		_, err := q.GetStaffByEmail(ctx, uniq("nobody")+"@x.test")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetStaffByEmail(unknown) err = %v, want ErrNotFound", err)
		}
	})

	t.Run("valid_session_resolves_staff", func(t *testing.T) {
		tokenHash := uniq("tok")
		newStaffSession(t, q, tokenHash, "staff-hamra", time.Now().Add(time.Hour))

		s, err := q.GetStaffBySessionToken(ctx, tokenHash)
		if err != nil {
			t.Fatalf("GetStaffBySessionToken(valid) err = %v, want nil", err)
		}
		if s.ID != "staff-hamra" {
			t.Fatalf("ID = %q, want %q", s.ID, "staff-hamra")
		}
	})

	t.Run("expired_session_is_rejected", func(t *testing.T) {
		tokenHash := uniq("tok")
		newStaffSession(t, q, tokenHash, "staff-hamra", time.Now().Add(-time.Minute))

		_, err := q.GetStaffBySessionToken(ctx, tokenHash)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetStaffBySessionToken(expired) err = %v, want ErrNotFound", err)
		}
	})

	t.Run("unknown_token_hash_is_rejected", func(t *testing.T) {
		_, err := q.GetStaffBySessionToken(ctx, uniq("missing"))
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetStaffBySessionToken(unknown) err = %v, want ErrNotFound", err)
		}
	})
}
