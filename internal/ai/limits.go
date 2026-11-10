package ai

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// ErrBusy is a model saying it has too much to do right now (HTTP 503).
// It's load, not an outage: players fall back for the turn, and the model
// isn't marked down.
var ErrBusy = errors.New("model busy")

// Gate caps how many requests are in flight to a model across every game on
// the server. A request that can't get a slot within Wait plays its
// fallback instead, so the model only ever sees a small, steady load.
type Gate struct {
	slots chan struct{}
	Wait  time.Duration
}

// NewGate allows n requests at once, each waiting up to wait for a slot.
func NewGate(n int, wait time.Duration) *Gate {
	return &Gate{slots: make(chan struct{}, max(n, 1)), Wait: wait}
}

// Acquire takes a slot, waiting up to Wait (or until ctx ends). It reports
// whether it got one; call the returned function to give it back.
func (g *Gate) Acquire(ctx context.Context) (release func(), ok bool) {
	if g == nil {
		return func() {}, true
	}
	t := time.NewTimer(g.Wait)
	defer t.Stop()
	select {
	case g.slots <- struct{}{}:
		return func() { <-g.slots }, true
	case <-t.C:
	case <-ctx.Done():
	}
	return nil, false
}

// InFlight is how many slots are taken.
func (g *Gate) InFlight() int {
	if g == nil {
		return 0
	}
	return len(g.slots)
}

// Stats counts what a System 1 player did with each decision.
type Stats struct {
	Asked    atomic.Int64 // decisions that went to the model
	Answered atomic.Int64 // and got answers
	// Fallbacks, by why the model wasn't used or didn't answer.
	Down    atomic.Int64 // the model was known to be down
	Queued  atomic.Int64 // no request slot free in time (Gate)
	Busy    atomic.Int64 // the model answered 503
	Timeout atomic.Int64 // no answer in time
	Failed  atomic.Int64 // any other error
}

// StatsSnapshot is Stats at one moment.
type StatsSnapshot struct {
	Asked, Answered, Down, Queued, Busy, Timeout, Failed int64
}

// Snapshot reads every counter.
func (s *Stats) Snapshot() StatsSnapshot {
	if s == nil {
		return StatsSnapshot{}
	}
	return StatsSnapshot{s.Asked.Load(), s.Answered.Load(), s.Down.Load(), s.Queued.Load(), s.Busy.Load(), s.Timeout.Load(), s.Failed.Load()}
}

// Fallbacks is every decision the fallback player made.
func (s StatsSnapshot) Fallbacks() int64 { return s.Down + s.Queued + s.Busy + s.Timeout + s.Failed }

func (s *Stats) add(c func(*Stats) *atomic.Int64) {
	if s != nil {
		c(s).Add(1)
	}
}
