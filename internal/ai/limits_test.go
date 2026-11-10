package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"parallax/internal/rules"
)

type funcModel func(ctx context.Context, p Prompt) (Answers, error)

func (f funcModel) Ask(ctx context.Context, p Prompt) (Answers, error) { return f(ctx, p) }

func busyView(t *testing.T) rules.View {
	g := newGame(t, 2, rules.DefaultSettings(), 3)
	for range 6 {
		g.EndTurn(g.Active())
	}
	return g.View(g.Active())
}

func TestGateCapsRequestsInFlight(t *testing.T) {
	v := busyView(t)
	gate := NewGate(1, 50*time.Millisecond)
	stats := &Stats{}
	started := make(chan struct{})
	unblock := make(chan struct{})
	slow := funcModel(func(ctx context.Context, p Prompt) (Answers, error) {
		close(started)
		<-unblock
		return Answers{}, nil
	})
	p := &System1{Model: slow, Fallback: Greedy{}, Gate: gate, Stats: stats}
	done := make(chan struct{})
	go func() { p.Orders(v); close(done) }()
	<-started
	if gate.InFlight() != 1 {
		t.Fatalf("in flight %d", gate.InFlight())
	}
	// A second decision can't get the only slot: it plays Greedy at once.
	var errs int
	q := &System1{Model: failingModel{}, Fallback: Greedy{}, Gate: gate, Stats: stats, OnError: func(error) { errs++ }}
	if got, want := q.Orders(v), (Greedy{}).Orders(v); len(got) != len(want) || errs != 0 {
		t.Fatalf("queued: %v vs %v, errs %d", got, want, errs)
	}
	close(unblock)
	<-done
	s := stats.Snapshot()
	if s.Asked != 1 || s.Answered != 1 || s.Queued != 1 || gate.InFlight() != 0 {
		t.Fatalf("stats %+v, in flight %d", s, gate.InFlight())
	}
}

func TestBusyAndTimeoutDontMarkTheModelDown(t *testing.T) {
	v := busyView(t)
	var errs int
	stats := &Stats{}
	var deadline atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline.Store(r.Header.Get("X-Deadline-Ms"))
		http.Error(w, `{"error": "busy"}`, http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	p := &System1{Model: &Client{URL: srv.URL}, Fallback: Greedy{}, Stats: stats, OnError: func(error) { errs++ }}
	p.Orders(v)
	got, _ := deadline.Load().(string)
	if ms, err := strconv.ParseInt(got, 10, 64); err != nil || ms < time.Now().UnixMilli() {
		t.Errorf("X-Deadline-Ms %q", got)
	}
	slow := funcModel(func(ctx context.Context, p Prompt) (Answers, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	(&System1{Model: slow, Fallback: Greedy{}, Stats: stats, Timeout: 20 * time.Millisecond, OnError: func(error) { errs++ }}).Orders(v)
	if s := stats.Snapshot(); errs != 0 || s.Busy != 1 || s.Timeout != 1 || s.Fallbacks() != 2 {
		t.Fatalf("errs %d, stats %+v", errs, s)
	}
	// A real failure still reports. (The client sends a batch's states
	// concurrently, so the busy server answered several requests above.)
	(&System1{Model: failingModel{}, Fallback: Greedy{}, Stats: stats, OnError: func(error) { errs++ }}).Orders(v)
	if s := stats.Snapshot(); errs != 1 || s.Failed != 1 {
		t.Fatalf("errs %d, stats %+v", errs, s)
	}
}
