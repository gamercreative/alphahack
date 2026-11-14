package agentgw

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	tokenKey   = bytes.Repeat([]byte{1}, 32)
	serviceKey = bytes.Repeat([]byte{2}, 32)
	t0         = time.Unix(1_800_000_000, 0)
)

type echoArgs struct {
	N int `json:"n"`
}

func (a *echoArgs) Validate() error {
	if a.N < 0 || a.N > 10 {
		return Invalid("n must be 0..10")
	}
	return nil
}

func newGW(t *testing.T) (*Gateway, *[]AuditEntry, *int) {
	t.Helper()
	var log []AuditEntry
	g, err := New(Config{TokenKey: tokenKey, ServiceKey: serviceKey, Now: func() time.Time { return t0 },
		Audit: func(e AuditEntry) { log = append(log, e) }})
	if err != nil {
		t.Fatal(err)
	}
	writes := 0
	Register(g, "x.read", "x:read", RiskRead, func(ctx context.Context, c *Claims, a *echoArgs) (any, error) {
		return map[string]any{"n": a.N, "user": c.Sub}, nil
	})
	Register(g, "x.write", "x:write", RiskWrite, func(ctx context.Context, c *Claims, a *echoArgs) (any, error) {
		writes++
		return map[string]any{"writes": writes}, nil
	})
	Register(g, "x.boom", "x:read", RiskRead, func(ctx context.Context, c *Claims, a *echoArgs) (any, error) {
		return nil, errors.New(`pq: relation "users_secret" does not exist`)
	})
	return g, &log, &writes
}

func tok(t *testing.T, scopes []string, budget int, exp time.Time) string {
	s, err := MintToken(tokenKey, Claims{Sub: "u_1", Sid: "s_1", Scopes: scopes, Exp: exp.Unix(), Budget: budget})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func nonce() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type reqOpt struct {
	nonce, idem string
	ts          time.Time
	key         []byte
	tamperBody  string
}

func call(g *Gateway, token, op, body string, o reqOpt) *httptest.ResponseRecorder {
	if o.nonce == "" {
		o.nonce = nonce()
	}
	if o.ts.IsZero() {
		o.ts = t0
	}
	if o.key == nil {
		o.key = serviceKey
	}
	path := Prefix + op
	ts := strconv.FormatInt(o.ts.Unix(), 10)
	sig := SignRequest(o.key, "POST", path, ts, o.nonce, []byte(body))
	sent := body
	if o.tamperBody != "" {
		sent = o.tamperBody
	}
	r := httptest.NewRequest("POST", path, strings.NewReader(sent))
	r.Header.Set("Authorization", "Agent "+token)
	r.Header.Set(HdrTimestamp, ts)
	r.Header.Set(HdrNonce, o.nonce)
	r.Header.Set(HdrSignature, sig)
	if o.idem != "" {
		r.Header.Set("Idempotency-Key", o.idem)
	}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	return w
}

func code(w *httptest.ResponseRecorder) string {
	var e envelope
	json.Unmarshal(w.Body.Bytes(), &e)
	if e.Error == nil {
		return ""
	}
	return e.Error.Code
}

func TestHappyPathUsesUserFromToken(t *testing.T) {
	g, _, _ := newGW(t)
	w := call(g, tok(t, []string{"x:read"}, 5, t0.Add(time.Minute)), "x.read", `{"n":3}`, reqOpt{})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"user":"u_1"`) {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestSignatureAttacks(t *testing.T) {
	g, _, _ := newGW(t)
	tk := tok(t, []string{"x:read"}, 50, t0.Add(time.Minute))
	cases := map[string]reqOpt{
		"wrong key":     {key: bytes.Repeat([]byte{9}, 32)},
		"tampered body": {tamperBody: `{"n":9}`},
		"stale":         {ts: t0.Add(-time.Minute)},
		"future":        {ts: t0.Add(time.Minute)},
		"short nonce":   {nonce: "abc"},
	}
	for name, o := range cases {
		if w := call(g, tk, "x.read", `{"n":1}`, o); w.Code != 401 {
			t.Errorf("%s: want 401, got %d", name, w.Code)
		}
	}
	n := nonce()
	if w := call(g, tk, "x.read", `{"n":1}`, reqOpt{nonce: n}); w.Code != 200 {
		t.Fatalf("first: %d", w.Code)
	}
	if w := call(g, tk, "x.read", `{"n":1}`, reqOpt{nonce: n}); w.Code != 401 {
		t.Fatalf("replay: want 401, got %d", w.Code)
	}
}

func TestTokenAttacks(t *testing.T) {
	g, _, _ := newGW(t)
	good := tok(t, []string{"x:read"}, 5, t0.Add(time.Minute))
	// Forge a token granting x:write by editing the payload.
	p, s, _ := strings.Cut(good, ".")
	raw, _ := b64.DecodeString(p)
	forged := b64.EncodeToString(bytes.Replace(raw, []byte(`"x:read"`), []byte(`"x:write"`), 1)) + "." + s
	for name, tk := range map[string]string{
		"forged scope": forged,
		"expired":      tok(t, []string{"x:read"}, 5, t0.Add(-time.Second)),
		"garbage":      "nope",
	} {
		if w := call(g, tk, "x.read", `{"n":1}`, reqOpt{}); w.Code != 401 {
			t.Errorf("%s: want 401, got %d", name, w.Code)
		}
	}
}

func TestScopeAndUnknownOp(t *testing.T) {
	g, _, _ := newGW(t)
	tk := tok(t, []string{"x:read"}, 5, t0.Add(time.Minute))
	if w := call(g, tk, "x.write", `{"n":1}`, reqOpt{idem: "abcdefgh1"}); w.Code != 403 {
		t.Fatalf("scope: %d", w.Code)
	}
	if w := call(g, tk, "sql.exec", `{"q":"DROP TABLE users"}`, reqOpt{}); w.Code != 404 {
		t.Fatalf("unknown op: %d", w.Code)
	}
}

func TestStrictArgs(t *testing.T) {
	g, _, _ := newGW(t)
	tk := tok(t, []string{"x:read"}, 50, t0.Add(time.Minute))
	for name, body := range map[string]string{
		"smuggled user_id": `{"n":1,"user_id":"u_admin"}`,
		"trailing value":   `{"n":1}{"n":2}`,
		"wrong type":       `{"n":"1; DROP TABLE x"}`,
		"out of range":     `{"n":999}`,
	} {
		if w := call(g, tk, "x.read", body, reqOpt{}); w.Code != 400 {
			t.Errorf("%s: want 400, got %d %s", name, w.Code, w.Body)
		}
	}
}

func TestBudgetStopsRunawayLoop(t *testing.T) {
	g, _, _ := newGW(t)
	tk := tok(t, []string{"x:read"}, 3, t0.Add(time.Minute))
	for i := 0; i < 3; i++ {
		if w := call(g, tk, "x.read", `{"n":1}`, reqOpt{}); w.Code != 200 {
			t.Fatalf("call %d: %d", i, w.Code)
		}
	}
	if w := call(g, tk, "x.read", `{"n":1}`, reqOpt{}); w.Code != 429 {
		t.Fatalf("want 429, got %d", w.Code)
	}
}

func TestWritesAreIdempotent(t *testing.T) {
	g, _, writes := newGW(t)
	tk := tok(t, []string{"x:write"}, 10, t0.Add(time.Minute))
	if w := call(g, tk, "x.write", `{"n":1}`, reqOpt{}); code(w) != "idempotency_required" {
		t.Fatalf("missing key: %d %s", w.Code, w.Body)
	}
	a := call(g, tk, "x.write", `{"n":1}`, reqOpt{idem: "book-ev2-abc"})
	b := call(g, tk, "x.write", `{"n":1}`, reqOpt{idem: "book-ev2-abc"})
	if a.Code != 200 || b.Code != 200 || *writes != 1 || b.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("writes=%d a=%d b=%d", *writes, a.Code, b.Code)
	}
}

func TestIdempotencyKeyReusedWithDifferentArgs(t *testing.T) {
	g, _, writes := newGW(t)
	tk := tok(t, []string{"x:write"}, 10, t0.Add(time.Minute))

	t.Run("same key different args is rejected", func(t *testing.T) {
		a := call(g, tk, "x.write", `{"n":1}`, reqOpt{idem: "book-ev2-abc"})
		if a.Code != 200 {
			t.Fatalf("first write: want 200, got %d %s", a.Code, a.Body)
		}
		b := call(g, tk, "x.write", `{"n":2}`, reqOpt{idem: "book-ev2-abc"})
		if b.Code != 422 || code(b) != "idempotency_key_reused" {
			t.Errorf("same key, different args: want 422 idempotency_key_reused, got %d %q %s",
				b.Code, code(b), b.Body)
		}
		if *writes != 1 {
			t.Errorf("writes: want 1 (new args must not apply), got %d", *writes)
		}
		if h := b.Header().Get("Idempotent-Replay"); h != "" {
			t.Errorf("Idempotent-Replay: want empty on rejection, got %q", h)
		}
	})

	t.Run("same key same args still replays", func(t *testing.T) {
		c := call(g, tk, "x.write", `{"n":1}`, reqOpt{idem: "book-ev2-abc"})
		if c.Code != 200 {
			t.Fatalf("replay: want 200, got %d %s", c.Code, c.Body)
		}
		if h := c.Header().Get("Idempotent-Replay"); h != "true" {
			t.Errorf("Idempotent-Replay: want true, got %q", h)
		}
		if *writes != 1 {
			t.Errorf("writes: want 1 after replay, got %d", *writes)
		}
	})

	t.Run("different key different args writes", func(t *testing.T) {
		d := call(g, tk, "x.write", `{"n":2}`, reqOpt{idem: "book-ev2-xyz"})
		if d.Code != 200 {
			t.Fatalf("new key: want 200, got %d %s", d.Code, d.Body)
		}
		if *writes != 2 {
			t.Errorf("writes: want 2, got %d", *writes)
		}
	})
}

func TestInternalErrorsDoNotLeak(t *testing.T) {
	g, log, _ := newGW(t)
	w := call(g, tok(t, []string{"x:read"}, 5, t0.Add(time.Minute)), "x.boom", `{"n":1}`, reqOpt{})
	if w.Code != 500 || strings.Contains(w.Body.String(), "users_secret") {
		t.Fatalf("leaked: %s", w.Body)
	}
	last := (*log)[len(*log)-1]
	if !strings.Contains(last.Detail, "users_secret") {
		t.Fatal("audit should keep the real error internally")
	}
}

func TestAgentCredentialsRejectedOnNormalAPI(t *testing.T) {
	h := RejectAgentCredentials(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	r := httptest.NewRequest("POST", "/api/agent/run", nil)
	r.Header.Set("Authorization", "Agent whatever")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("want 403, got %d", w.Code)
	}
}

func TestKeysMustDiffer(t *testing.T) {
	if _, err := New(Config{TokenKey: tokenKey, ServiceKey: tokenKey}); err == nil {
		t.Fatal("same key accepted")
	}
}
