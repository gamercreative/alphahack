package services

// DOSYN client auth: phone + PIN login, client sessions, "who am I". Client
// tokens carry ClientTokenPrefix so Authenticate knows which session table to
// read without trying both.

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"

	"alphahack/internal/queries"
)

// ClientTokenPrefix starts every client session token ("c_" + 64 hex chars).
const ClientTokenPrefix = "c_"

// ClientView is the client as the API shows it (no phone, no PIN).
type ClientView struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// ClientLoginResult is the body of POST /api/v1/client/auth/login.
type ClientLoginResult struct {
	Token  string     `json:"token"`
	Client ClientView `json:"client"`
}

// ClientMeResult is the body of GET /api/v1/client/me.
type ClientMeResult struct {
	Client ClientView `json:"client"`
}

func clientView(c queries.Client) ClientView {
	return ClientView{ID: c.ID, DisplayName: c.DisplayName}
}

// ClientLogin checks phone and PIN, creates a 12h session and returns the raw token once.
func (s *EnquiryService) ClientLogin(ctx context.Context, phone, pin string) (ClientLoginResult, error) {
	phone = strings.Join(strings.Fields(phone), "")
	pin = strings.TrimSpace(pin)
	if phone == "" || pin == "" {
		return ClientLoginResult{}, Invalid("phone and pin are required")
	}
	c, err := s.store.GetClientByPhone(ctx, phone)
	if errors.Is(err, queries.ErrNotFound) {
		return ClientLoginResult{}, Unauthorized("wrong phone or PIN")
	}
	if err != nil {
		return ClientLoginResult{}, FromStore(err, "client")
	}
	if subtle.ConstantTimeCompare([]byte(sha256Hex(pin)), []byte(c.PinHash)) != 1 {
		return ClientLoginResult{}, Unauthorized("wrong phone or PIN")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return ClientLoginResult{}, Internal(err)
	}
	token := ClientTokenPrefix + hex.EncodeToString(raw)
	if err := s.store.CreateClientSession(ctx, queries.CreateClientSessionParams{
		TokenHash: sha256Hex(token), ClientID: c.ID, ExpiresAt: s.now().Add(s.cfg.SessionTTL),
	}); err != nil {
		return ClientLoginResult{}, FromStore(err, "session")
	}
	return ClientLoginResult{Token: token, Client: clientView(c)}, nil
}

func (s *EnquiryService) authenticateClient(ctx context.Context, token string) (Actor, error) {
	c, err := s.store.GetClientBySessionToken(ctx, sha256Hex(token))
	if errors.Is(err, queries.ErrNotFound) {
		return Actor{}, Unauthorized("session expired or invalid; log in again with POST /api/v1/client/auth/login")
	}
	if err != nil {
		return Actor{}, FromStore(err, "session")
	}
	return Actor{Type: queries.ActorClient, ID: c.ID}, nil
}

// ClientMe returns the calling client.
func (s *EnquiryService) ClientMe(ctx context.Context, a Actor) (ClientMeResult, error) {
	if !a.IsClient() {
		return ClientMeResult{}, Forbidden("this route is for DOSYN clients only")
	}
	c, err := s.store.GetClient(ctx, a.ID)
	if err != nil {
		return ClientMeResult{}, FromStore(err, "client")
	}
	return ClientMeResult{Client: clientView(c)}, nil
}
