package room

import (
	"math/rand/v2"
	"strings"
	"sync"

	"parallax/internal/proto"
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
	m := &Manager{rooms: map[string]*Room{}}
	if opts.ModelGames > 0 {
		opts.quota = NewModelQuota(opts.ModelGames)
		// Lobbies stop or start offering model players as slots run out
		// or free up.
		opts.quota.changed = m.RefreshLobbies
	}
	m.opts = opts
	return m
}

// ModelGames is how many games hold a model slot, and the limit (0 = none).
func (m *Manager) ModelGames() (inUse, limit int) {
	return m.opts.quota.InUse(), m.opts.ModelGames
}

// Create starts a new room with an unused code.
func (m *Manager) Create() *Room {
	r, _ := m.create(nil)
	return r
}

// Load starts a room from a saved game.
func (m *Manager) Load(f *proto.SaveFile) (*Room, error) { return m.create(f) }

func (m *Manager) create(f *proto.SaveFile) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	code := newCode()
	for m.rooms[code] != nil {
		code = newCode()
	}
	r := New(code, m.opts)
	if f != nil {
		if err := r.load(f); err != nil {
			return nil, err
		}
	}
	m.start(r)
	return r, nil
}

// start registers and runs a room; m.mu must be held.
func (m *Manager) start(r *Room) {
	code := r.code
	m.rooms[code] = r
	go func() {
		r.Run()
		m.mu.Lock()
		if m.rooms[code] == r {
			delete(m.rooms, code)
		}
		m.mu.Unlock()
	}()
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

// RefreshLobbies resends the lobby to every room still in it, say when a
// computer player kind becomes available or unavailable.
func (m *Manager) RefreshLobbies() {
	m.mu.Lock()
	rooms := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	m.mu.Unlock()
	for _, r := range rooms {
		r.post(func() {
			if r.phase == proto.PhaseLobby {
				r.broadcastLobby()
			}
		})
	}
}
