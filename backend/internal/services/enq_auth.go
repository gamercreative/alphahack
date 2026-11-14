package services

// Wassit auth: staff login and session lookup, agent token check, and "who am I".
// Services never import net/http; the handlers pass the raw Authorization header in.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"

	"alphahack/internal/queries"
)

// LoginResult is the body of POST /api/v1/auth/login.
type LoginResult struct {
	Token  string         `json:"token"`
	Staff  queries.Staff  `json:"staff"`
	Branch queries.Branch `json:"branch"`
}

// MeResult is the body of GET /api/v1/me.
type MeResult struct {
	Staff  queries.Staff  `json:"staff"`
	Branch queries.Branch `json:"branch"`
}

// sha256Hex returns the lowercase hex SHA-256 of s. Used for PIN hashes and
// for session tokens (only the hash is stored).
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Login checks email and PIN, creates a session and returns the raw token once.
func (s *EnquiryService) Login(ctx context.Context, email, pin string) (LoginResult, error) {
	email = strings.TrimSpace(email)
	pin = strings.TrimSpace(pin)
	if email == "" || pin == "" {
		return LoginResult{}, Invalid("email and pin are required")
	}

	staff, err := s.store.GetStaffByEmail(ctx, email)
	if errors.Is(err, queries.ErrNotFound) {
		return LoginResult{}, Unauthorized("wrong email or PIN")
	}
	if err != nil {
		return LoginResult{}, FromStore(err, "staff")
	}
	if subtle.ConstantTimeCompare([]byte(sha256Hex(pin)), []byte(staff.PinHash)) != 1 {
		return LoginResult{}, Unauthorized("wrong email or PIN")
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return LoginResult{}, Internal(err)
	}
	token := hex.EncodeToString(raw)

	if err := s.store.CreateStaffSession(ctx, queries.CreateStaffSessionParams{
		TokenHash: sha256Hex(token),
		StaffID:   staff.ID,
		ExpiresAt: s.now().Add(s.cfg.SessionTTL),
	}); err != nil {
		return LoginResult{}, FromStore(err, "session")
	}

	branch, err := s.store.GetBranch(ctx, staff.BranchID)
	if err != nil {
		return LoginResult{}, FromStore(err, "branch")
	}
	return LoginResult{Token: token, Staff: staff, Branch: branch}, nil
}

// Authenticate turns an Authorization header into an Actor.
// "Agent <token>" is checked against cfg.AgentToken (constant time; empty disables it).
// "Bearer <token>" is looked up as a staff session by its hash.
func (s *EnquiryService) Authenticate(ctx context.Context, authorization string) (Actor, error) {
	switch {
	case strings.HasPrefix(authorization, "Agent "):
		token := strings.TrimPrefix(authorization, "Agent ")
		if s.cfg.AgentToken == "" ||
			subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.AgentToken)) != 1 {
			return Actor{}, Unauthorized("invalid agent token")
		}
		return Actor{Type: queries.ActorAgent, ID: "wassit"}, nil

	case strings.HasPrefix(authorization, "Bearer "):
		token := strings.TrimPrefix(authorization, "Bearer ")
		staff, err := s.store.GetStaffBySessionToken(ctx, sha256Hex(token))
		if errors.Is(err, queries.ErrNotFound) {
			return Actor{}, Unauthorized("session expired or invalid; log in again with POST /api/v1/auth/login")
		}
		if err != nil {
			return Actor{}, FromStore(err, "session")
		}
		return Actor{Type: queries.ActorStaff, ID: staff.ID, BranchID: staff.BranchID}, nil

	default:
		return Actor{}, Unauthorized("missing credential: send 'Authorization: Bearer <token>' from POST /api/v1/auth/login")
	}
}

// Me returns the staff member and branch of the calling actor.
func (s *EnquiryService) Me(ctx context.Context, a Actor) (MeResult, error) {
	staff, err := s.store.GetStaff(ctx, a.ID)
	if err != nil {
		return MeResult{}, FromStore(err, "staff")
	}
	branch, err := s.store.GetBranch(ctx, staff.BranchID)
	if err != nil {
		return MeResult{}, FromStore(err, "branch")
	}
	return MeResult{Staff: staff, Branch: branch}, nil
}
