package services

import (
	"context"
	"errors"
	"testing"

	"alphahack/internal/queries"
)

// TestAuthenticateAgent covers the "Agent <token>" branch of Authenticate.
// The store is not used on these paths, so it is nil.
func TestAuthenticateAgent(t *testing.T) {
	ctx := context.Background()
	svc := NewEnquiryService(nil, EnquiryConfig{AgentToken: "t"})

	t.Run("correct token gives agent actor", func(t *testing.T) {
		a, err := svc.Authenticate(ctx, "Agent t")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Type != queries.ActorAgent || a.ID != "wassit" || a.BranchID != "" {
			t.Errorf("actor = %+v, want agent wassit with no branch", a)
		}
	})

	cases := []struct {
		name          string
		cfg           EnquiryConfig
		authorization string
	}{
		{"wrong token", EnquiryConfig{AgentToken: "t"}, "Agent nope"},
		{"empty configured token", EnquiryConfig{AgentToken: ""}, "Agent "},
		{"empty configured token with any token", EnquiryConfig{AgentToken: ""}, "Agent t"},
		{"missing header", EnquiryConfig{AgentToken: "t"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewEnquiryService(nil, tc.cfg)
			_, err := s.Authenticate(ctx, tc.authorization)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			var se *Error
			if !errors.As(err, &se) {
				t.Fatalf("error %T is not *services.Error", err)
			}
			if se.Code != CodeUnauthorized {
				t.Errorf("code = %q, want %q", se.Code, CodeUnauthorized)
			}
		})
	}
}
