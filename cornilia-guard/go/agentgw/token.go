// Package agentgw is the Go-side gateway that every Cornilia tool call must pass
// through before it can touch the database. Go is the policy enforcement point:
// the Python side may pre-check, but nothing it says is trusted here.
package agentgw

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrBadToken = errors.New("bad agent token")
	ErrExpired  = errors.New("agent token expired")
)

// Claims is the delegation token Go mints when it hands a job to Cornilia.
// It says WHO the agent acts for (Sub), WHICH run it belongs to (Sid),
// WHAT it may do (Scopes) and HOW MUCH (Budget). Cornilia only carries it;
// it cannot read-modify-sign it because only Go holds the token key.
type Claims struct {
	Sub    string   `json:"sub"` // user the agent acts on behalf of
	Sid    string   `json:"sid"` // agent session / run id
	Scopes []string `json:"scp"`
	Exp    int64    `json:"exp"` // unix seconds
	Budget int      `json:"bud"` // max tool calls for this session
}

func (c *Claims) HasScope(s string) bool {
	for _, x := range c.Scopes {
		if x == s {
			return true
		}
	}
	return false
}

var b64 = base64.RawURLEncoding

func mac(key []byte, domain string, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(domain))
	h.Write([]byte{0})
	h.Write(msg)
	return h.Sum(nil)
}

const tokenDomain = "cornilia-token-v1"

// MintToken signs claims with the token key (Go-only secret).
func MintToken(key []byte, c Claims) (string, error) {
	if len(key) < 32 {
		return "", errors.New("token key must be at least 32 bytes")
	}
	if c.Sub == "" || c.Sid == "" || c.Exp == 0 || c.Budget <= 0 {
		return "", errors.New("claims need sub, sid, exp and a positive budget")
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	p := b64.EncodeToString(payload)
	return p + "." + b64.EncodeToString(mac(key, tokenDomain, []byte(p))), nil
}

// VerifyToken checks signature first, then parses, then checks expiry.
func VerifyToken(key []byte, tok string, now time.Time) (*Claims, error) {
	p, s, ok := strings.Cut(tok, ".")
	if !ok || p == "" || s == "" {
		return nil, ErrBadToken
	}
	sig, err := b64.DecodeString(s)
	if err != nil || !hmac.Equal(sig, mac(key, tokenDomain, []byte(p))) {
		return nil, ErrBadToken
	}
	payload, err := b64.DecodeString(p)
	if err != nil {
		return nil, ErrBadToken
	}
	var c Claims
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil || c.Sub == "" || c.Sid == "" {
		return nil, ErrBadToken
	}
	if now.Unix() >= c.Exp {
		return nil, ErrExpired
	}
	return &c, nil
}

// NewSessionID returns a random id for one agent run.
func NewSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// MintSession is what your normal Go route calls right before invoking
// Cornilia. Scopes come from the FEATURE the user is using, never from the model.
func MintSession(tokenKey []byte, userID string, scopes []string, ttl time.Duration, budget int, now time.Time) (token, sid string, err error) {
	sid = NewSessionID()
	token, err = MintToken(tokenKey, Claims{
		Sub: userID, Sid: sid, Scopes: scopes, Exp: now.Add(ttl).Unix(), Budget: budget,
	})
	return token, sid, err
}
