package agentgw

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const Prefix = "/agent/v1/ops/"

var idemRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type AuditEntry struct {
	Time     time.Time `json:"time"`
	Sub      string    `json:"sub,omitempty"`
	Sid      string    `json:"sid,omitempty"`
	Op       string    `json:"op,omitempty"`
	Outcome  string    `json:"outcome"`
	Detail   string    `json:"detail,omitempty"` // internal only, never sent to the model
	ArgsHash string    `json:"args_sha256,omitempty"`
	DurMS    int64     `json:"dur_ms"`
}

type Config struct {
	TokenKey   []byte        // Go-only: signs delegation tokens
	ServiceKey []byte        // shared with Cornilia: signs each request
	MaxSkew    time.Duration // default 30s
	MaxBody    int64         // default 32 KiB
	OpTimeout  time.Duration // default 5s
	Audit      func(AuditEntry)
	Now        func() time.Time
}

type budgetState struct {
	used int
	exp  int64
}

// idemEntry is one write's idempotency record: the body hash it was reserved
// with, and the cached response (nil while the write is still in progress).
type idemEntry struct {
	argsHash string
	resp     []byte
}

type Gateway struct {
	cfg    Config
	ops    map[string]Op
	nonces *nonceCache

	mu      sync.Mutex
	budgets map[string]*budgetState // sid -> usage
	idem    map[string]idemEntry    // sid|key -> cached response
}

func New(cfg Config) (*Gateway, error) {
	if len(cfg.TokenKey) < 32 || len(cfg.ServiceKey) < 32 {
		return nil, errors.New("agentgw: keys must be at least 32 bytes")
	}
	if bytes.Equal(cfg.TokenKey, cfg.ServiceKey) {
		return nil, errors.New("agentgw: token key and service key must differ")
	}
	if cfg.MaxSkew == 0 {
		cfg.MaxSkew = 30 * time.Second
	}
	if cfg.MaxBody == 0 {
		cfg.MaxBody = 32 << 10
	}
	if cfg.OpTimeout == 0 {
		cfg.OpTimeout = 5 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Gateway{
		cfg:     cfg,
		ops:     map[string]Op{},
		nonces:  newNonceCache(2 * cfg.MaxSkew),
		budgets: map[string]*budgetState{},
		idem:    map[string]idemEntry{},
	}, nil
}

// Ops lists registered operations (useful for generating Cornilia tool specs).
func (g *Gateway) Ops() []Op {
	out := make([]Op, 0, len(g.ops))
	for _, op := range g.ops {
		out = append(out, op)
	}
	return out
}

type apiErr struct {
	Code string `json:"code"`
	Msg  string `json:"message"`
}

type envelope struct {
	OK    bool    `json:"ok"`
	Data  any     `json:"data,omitempty"`
	Error *apiErr `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, envelope{OK: false, Error: &apiErr{code, msg}})
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	now := g.cfg.Now()
	e := AuditEntry{Time: now, Outcome: "error"}
	defer func() {
		e.DurMS = time.Since(now).Milliseconds()
		if g.cfg.Audit != nil {
			g.cfg.Audit(e)
		}
	}()

	if r.Method != http.MethodPost {
		e.Outcome = "bad_method"
		fail(w, 405, "method_not_allowed", "POST only")
		return
	}
	opName, ok := strings.CutPrefix(r.URL.Path, Prefix)
	if !ok || opName == "" {
		e.Outcome = "bad_path"
		fail(w, 404, "unknown_op", "unknown operation")
		return
	}
	e.Op = opName

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, g.cfg.MaxBody))
	if err != nil {
		e.Outcome = "body_too_large"
		fail(w, 413, "too_large", "request body too large")
		return
	}
	sum := sha256.Sum256(body)
	e.ArgsHash = hex.EncodeToString(sum[:])

	// 1) Is this really Cornilia? (shared service key, anti-replay)
	if err := g.verifySignature(r, body, now); err != nil {
		e.Outcome, e.Detail = "bad_signature", err.Error()
		fail(w, 401, "unauthorized", "request signature rejected")
		return
	}

	// 2) Who is it acting for and what may it do? (Go-minted token)
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Agent ")
	if !ok {
		e.Outcome = "no_token"
		fail(w, 401, "unauthorized", "missing agent token")
		return
	}
	claims, err := VerifyToken(g.cfg.TokenKey, tok, now)
	if err != nil {
		e.Outcome, e.Detail = "bad_token", err.Error()
		fail(w, 401, "unauthorized", "agent token rejected")
		return
	}
	e.Sub, e.Sid = claims.Sub, claims.Sid

	// 3) Is this an allowlisted op, and does the token cover it?
	op, ok := g.ops[opName]
	if !ok {
		e.Outcome = "unknown_op"
		fail(w, 404, "unknown_op", "unknown operation")
		return
	}
	if !claims.HasScope(op.Scope) {
		e.Outcome = "forbidden_scope"
		fail(w, 403, "forbidden", "this session is not allowed to use "+opName)
		return
	}

	// 4) Writes must be idempotent: a retried or looping call can't double-apply.
	var idemKey string
	markCommitted := func() {}
	if op.Risk == RiskWrite {
		idemKey = r.Header.Get("Idempotency-Key")
		if !idemRe.MatchString(idemKey) {
			e.Outcome = "no_idempotency_key"
			fail(w, 400, "idempotency_required", "write operations need an Idempotency-Key")
			return
		}
		idemKey = claims.Sid + "|" + opName + "|" + idemKey
		g.mu.Lock()
		cached, hit := g.idem[idemKey]
		if !hit {
			// reserve: a concurrent duplicate sees "in progress"
			g.idem[idemKey] = idemEntry{argsHash: e.ArgsHash}
		}
		g.mu.Unlock()
		if hit && cached.argsHash != e.ArgsHash {
			// Same key, different body: refuse and leave the existing entry alone.
			// This returns before the release defer below is installed.
			e.Outcome = "idempotency_key_reused"
			fail(w, 422, "idempotency_key_reused", "this Idempotency-Key was already used with different arguments")
			return
		}
		if hit && cached.resp == nil {
			e.Outcome = "in_progress"
			fail(w, 409, "in_progress", "an identical write is already running")
			return
		}
		committed := false
		defer func() {
			if !committed { // failed before success: release the reservation
				g.mu.Lock()
				delete(g.idem, idemKey)
				g.mu.Unlock()
			}
		}()
		markCommitted = func() { committed = true }
		if hit {
			committed = true
			e.Outcome = "replayed"
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotent-Replay", "true")
			w.WriteHeader(200)
			_, _ = w.Write(cached.resp)
			return
		}
	}

	// 5) Per-session budget, enforced server side. A runaway agent loop dies here
	// even if the Python guard is buggy or bypassed.
	if !g.spend(claims) {
		e.Outcome = "budget_exhausted"
		fail(w, 429, "budget_exhausted", "tool call budget for this session is used up")
		return
	}

	// 6) Strict args: exact shape, no unknown fields, one JSON value, then Validate.
	args := op.newArgs()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(args); err != nil {
		e.Outcome, e.Detail = "bad_args", err.Error()
		fail(w, 400, "invalid_args", "arguments do not match the operation schema")
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		e.Outcome = "bad_args"
		fail(w, 400, "invalid_args", "trailing data after arguments")
		return
	}
	if err := args.Validate(); err != nil {
		e.Outcome, e.Detail = "invalid_args", err.Error()
		var ue *UserError
		if errors.As(err, &ue) {
			fail(w, ue.Status, ue.Code, ue.Msg)
		} else {
			fail(w, 400, "invalid_args", "arguments failed validation")
		}
		return
	}

	// 7) Run it with a deadline.
	ctx, cancel := context.WithTimeout(r.Context(), g.cfg.OpTimeout)
	defer cancel()
	res, err := op.handle(ctx, claims, args)
	if err != nil {
		var ue *UserError
		if errors.As(err, &ue) {
			e.Outcome, e.Detail = "user_error", ue.Error()
			fail(w, ue.Status, ue.Code, ue.Msg)
			return
		}
		e.Outcome, e.Detail = "handler_error", err.Error()
		fail(w, 500, "internal", "internal error") // never leak DB errors
		return
	}

	out, err := json.Marshal(envelope{OK: true, Data: res})
	if err != nil {
		e.Outcome, e.Detail = "encode_error", err.Error()
		fail(w, 500, "internal", "internal error")
		return
	}
	if idemKey != "" {
		g.mu.Lock()
		g.idem[idemKey] = idemEntry{argsHash: e.ArgsHash, resp: out}
		g.mu.Unlock()
		markCommitted()
	}
	e.Outcome = "ok"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(out)
}

func (g *Gateway) spend(c *Claims) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.budgets) > 50_000 {
		now := g.cfg.Now().Unix()
		for k, b := range g.budgets {
			if now >= b.exp {
				delete(g.budgets, k)
			}
		}
	}
	b := g.budgets[c.Sid]
	if b == nil {
		b = &budgetState{exp: c.Exp}
		g.budgets[c.Sid] = b
	}
	if b.used >= c.Budget {
		return false
	}
	b.used++
	return true
}

// RejectAgentCredentials wraps your NORMAL API. An agent token or a Cornilia
// request signature is only valid on the gateway, so a tool can never call an
// endpoint that triggers another agent run. That is what breaks the
// Go -> Cornilia -> Go -> Cornilia loop.
func RejectAgentCredentials(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Authorization"), "Agent ") || r.Header.Get(HdrSignature) != "" {
			fail(w, 403, "forbidden", "agent credentials are not accepted on this API")
			return
		}
		next.ServeHTTP(w, r)
	})
}
