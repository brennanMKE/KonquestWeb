package room

import (
	"math/rand/v2"
	"strings"
	"sync"
)

// codeAlphabet leaves out letters that are easy to confuse (I, L, O).
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ"

// CodeLen is the length of a room code.
const CodeLen = 4

// Manager creates rooms and finds them by code. It only guards the code
// table; each room owns its own state.
type Manager struct {
	opts  Options
	mu    sync.Mutex
	rooms map[string]*Room
}

// NewManager returns a manager whose rooms use opts.
func NewManager(opts Options) *Manager {
	return &Manager{opts: opts, rooms: map[string]*Room{}}
}

// Create starts a new room with an unused code.
func (m *Manager) Create() *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	code := newCode()
	for m.rooms[code] != nil {
		code = newCode()
	}
	r := New(code, m.opts)
	m.rooms[code] = r
	go func() {
		r.Run()
		m.mu.Lock()
		delete(m.rooms, code)
		m.mu.Unlock()
	}()
	return r
}

// Get finds a room by code, ignoring case.
func (m *Manager) Get(code string) (*Room, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rooms[strings.ToUpper(code)]
	return r, ok
}

// Len is the number of open rooms.
func (m *Manager) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rooms)
}

func newCode() string {
	b := make([]byte, CodeLen)
	for i := range b {
		b[i] = codeAlphabet[rand.IntN(len(codeAlphabet))]
	}
	return string(b)
}
