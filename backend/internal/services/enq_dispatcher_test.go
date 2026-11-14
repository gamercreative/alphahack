package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"alphahack/internal/queries"
)

// syncBuffer is a goroutine-safe io.Writer for capturing slog output.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

var _ io.Writer = (*syncBuffer)(nil)

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// dispatchStore fakes the store: InTx returns err (or nil) and never runs fn,
// so DispatchDue fails or succeeds without touching any real queries.
type dispatchStore struct {
	queries.Querier
	err error
}

func (s *dispatchStore) InTx(ctx context.Context, fn func(q queries.Querier) error) error {
	return s.err
}

func TestRunDispatcherLogsDispatchErrors(t *testing.T) {
	t.Run("dispatch error is logged", func(t *testing.T) {
		var buf syncBuffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		store := &dispatchStore{err: errors.New("db is down")}
		svc := NewEnquiryService(store, EnquiryConfig{Logger: logger})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			svc.RunDispatcher(ctx, 5*time.Millisecond)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for !strings.Contains(buf.String(), "outbox dispatch failed") && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
		<-done

		out := buf.String()
		if !strings.Contains(out, "outbox dispatch failed") {
			t.Fatalf("log output missing %q; got:\n%s", "outbox dispatch failed", out)
		}
		if !strings.Contains(out, "level=ERROR") {
			t.Fatalf("log output missing %q; got:\n%s", "level=ERROR", out)
		}
		if !strings.Contains(out, "db is down") {
			t.Fatalf("log output missing error attribute %q; got:\n%s", "db is down", out)
		}
	})

	t.Run("no log on success", func(t *testing.T) {
		var buf syncBuffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		store := &dispatchStore{err: nil}
		svc := NewEnquiryService(store, EnquiryConfig{Logger: logger})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			svc.RunDispatcher(ctx, 5*time.Millisecond)
		}()

		time.Sleep(50 * time.Millisecond)
		cancel()
		<-done

		if out := buf.String(); strings.Contains(out, "outbox dispatch failed") {
			t.Fatalf("successful dispatch must not log %q; got:\n%s", "outbox dispatch failed", out)
		}
	})
}
