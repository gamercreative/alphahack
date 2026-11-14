package agentgw

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	HdrTimestamp = "X-Cornilia-Timestamp"
	HdrNonce     = "X-Cornilia-Nonce"
	HdrSignature = "X-Cornilia-Signature"
	sigDomain    = "cornilia-req-v1"
)

var nonceRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// CanonicalString is exactly what both sides sign. Any change to the method,
// path, time, nonce or body breaks the signature.
func CanonicalString(method, path, ts, nonce string, body []byte) string {
	sum := sha256.Sum256(body)
	return strings.Join([]string{method, path, ts, nonce, hex.EncodeToString(sum[:])}, "\n")
}

// SignRequest returns the hex HMAC Cornilia puts in X-Cornilia-Signature.
func SignRequest(serviceKey []byte, method, path, ts, nonce string, body []byte) string {
	return hex.EncodeToString(mac(serviceKey, sigDomain, []byte(CanonicalString(method, path, ts, nonce, body))))
}

type nonceCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
}

func newNonceCache(ttl time.Duration) *nonceCache {
	return &nonceCache{seen: map[string]time.Time{}, ttl: ttl}
}

// checkAndAdd returns false if the nonce was already used (replay).
func (n *nonceCache) checkAndAdd(nonce string, now time.Time) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.seen) > 50_000 {
		for k, exp := range n.seen {
			if now.After(exp) {
				delete(n.seen, k)
			}
		}
	}
	if exp, ok := n.seen[nonce]; ok && now.Before(exp) {
		return false
	}
	n.seen[nonce] = now.Add(n.ttl)
	return true
}

var (
	errSigMissing = errors.New("missing signature headers")
	errSigSkew    = errors.New("timestamp outside allowed window")
	errSigBad     = errors.New("bad signature")
	errSigReplay  = errors.New("nonce reused")
)

func (g *Gateway) verifySignature(r *http.Request, body []byte, now time.Time) error {
	ts, nonce, sig := r.Header.Get(HdrTimestamp), r.Header.Get(HdrNonce), r.Header.Get(HdrSignature)
	if ts == "" || sig == "" || !nonceRe.MatchString(nonce) {
		return errSigMissing
	}
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errSigMissing
	}
	skew := now.Sub(time.Unix(t, 0))
	if skew < 0 {
		skew = -skew
	}
	if skew > g.cfg.MaxSkew {
		return errSigSkew
	}
	got, err := hex.DecodeString(sig)
	want := mac(g.cfg.ServiceKey, sigDomain, []byte(CanonicalString(r.Method, r.URL.Path, ts, nonce, body)))
	if err != nil || !hmac.Equal(got, want) {
		return errSigBad
	}
	// Only record the nonce after the signature is valid, so junk traffic
	// can't fill the cache.
	if !g.nonces.checkAndAdd(nonce, now) {
		return errSigReplay
	}
	return nil
}
