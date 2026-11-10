package room

import (
	"slices"
	"sync"

	"parallax/internal/ai"
	"parallax/internal/proto"
)

// ModelQuota caps how many games at once may have computer players that
// ask a model (System 1), so one busy server can't swamp it. A room holds a
// slot from its game's start until the game ends or the room closes. While
// every slot is taken, lobbies don't offer those players and games with
// them can't start.
type ModelQuota struct {
	Limit int
	// changed, if set, runs (on its own goroutine) when slots free up or
	// run out, so lobbies can update what they offer.
	changed func()

	mu   sync.Mutex
	held map[string]bool
}

// NewModelQuota allows limit games at once; 0 or less means no limit.
func NewModelQuota(limit int) *ModelQuota {
	return &ModelQuota{Limit: limit, held: map[string]bool{}}
}

func (q *ModelQuota) full() bool { return q.Limit > 0 && len(q.held) >= q.Limit }

// Available reports whether room may use the model: it holds a slot, or one
// is free.
func (q *ModelQuota) Available(room string) bool {
	if q == nil {
		return true
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.held[room] || !q.full()
}

// Acquire takes a slot for room, reporting false if none is free.
func (q *ModelQuota) Acquire(room string) bool {
	if q == nil {
		return true
	}
	q.mu.Lock()
	if q.held[room] {
		q.mu.Unlock()
		return true
	}
	if q.full() {
		q.mu.Unlock()
		return false
	}
	q.held[room] = true
	nowFull := q.full()
	q.mu.Unlock()
	if nowFull {
		q.notify()
	}
	return true
}

// force takes a slot even past the limit: a game already running when the
// server restarted keeps its players.
func (q *ModelQuota) force(room string) {
	if q == nil {
		return
	}
	q.mu.Lock()
	q.held[room] = true
	q.mu.Unlock()
}

// Release gives room's slot back, if it held one.
func (q *ModelQuota) Release(room string) {
	if q == nil {
		return
	}
	q.mu.Lock()
	wasFull := q.full()
	had := q.held[room]
	delete(q.held, room)
	freed := had && wasFull && !q.full()
	q.mu.Unlock()
	if freed {
		q.notify()
	}
}

// InUse is how many games hold a slot.
func (q *ModelQuota) InUse() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.held)
}

func (q *ModelQuota) notify() {
	if q.changed != nil {
		go q.changed()
	}
}

// modelKind reports whether a kind of computer asks a model.
func (r *Room) modelKind(kind string) bool {
	_, ok := r.opts.AI[kind].(ai.Checker)
	return ok
}

// usesModel reports whether any seat is a computer that asks a model.
func (r *Room) usesModel() bool {
	return slices.ContainsFunc(r.seats, func(s *seat) bool { return s.kind == proto.KindAI && r.modelKind(s.aiKind) })
}
