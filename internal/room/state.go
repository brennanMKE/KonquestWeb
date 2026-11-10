package room

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"parallax/internal/proto"
	"parallax/internal/rules"
)

// StateVersion is the format of a saved room.
const StateVersion = 1

// State is everything needed to bring a room back after a server restart:
// its code, seats with their tokens, settings and the game so far. Players
// rejoin with the tokens their browsers kept.
type State struct {
	Version  int            `json:"version"`
	Code     string         `json:"code"`
	Phase    string         `json:"phase"`
	Settings proto.Settings `json:"settings"`
	Seed     uint64         `json:"seed"`
	Seats    []SeatState    `json:"seats"`
	// Host is the host's index in Seats, or -1 when the host watches.
	Host int `json:"host"`
	// Watcher is the host while watching instead of playing.
	Watcher *SeatState `json:"watcher,omitempty"`
	// Actions is the game's log; nil in the lobby.
	Actions []rules.Action `json:"actions"`
}

// SeatState is one saved seat. The token is the seat's secret.
type SeatState struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	AIKind string `json:"aiKind,omitempty"`
	Token  string `json:"token,omitempty"`
}

func seatState(s *seat) SeatState {
	return SeatState{Name: s.name, Kind: s.kind, AIKind: s.aiKind, Token: s.token}
}

func (ss SeatState) seat() *seat {
	return &seat{name: ss.Name, kind: ss.Kind, aiKind: ss.AIKind, token: ss.Token}
}

// snapshot returns the room's state. Room goroutine only.
func (r *Room) snapshot() *State {
	st := &State{Version: StateVersion, Code: r.code, Phase: r.phase, Settings: r.settings, Seed: r.seed, Host: -1}
	for i, s := range r.seats {
		st.Seats = append(st.Seats, seatState(s))
		if s == r.host {
			st.Host = i
		}
	}
	if h := r.host; h != nil && h.watching {
		w := seatState(h)
		st.Watcher = &w
	}
	if r.game != nil {
		st.Actions = r.game.Actions()
	}
	return st
}

// Snapshot asks the room for its state; it reports false if the room has
// closed.
func (r *Room) Snapshot() (*State, bool) {
	ch := make(chan *State, 1)
	if !r.post(func() { ch <- r.snapshot() }) {
		return nil, false
	}
	select {
	case st := <-ch:
		return st, true
	case <-r.done:
		return nil, false
	}
}

// restore sets up a new, not yet running room from a saved state.
func (r *Room) restore(st *State) error {
	if st.Version != StateVersion {
		return fmt.Errorf("unsupported room state version %d", st.Version)
	}
	switch st.Phase {
	case proto.PhaseLobby, proto.PhasePlaying, proto.PhaseOver:
	default:
		return fmt.Errorf("unknown phase %q", st.Phase)
	}
	if err := checkTimers(st.Settings); err != nil {
		return err
	}
	for _, ss := range st.Seats {
		if ss.Kind != proto.KindHuman && ss.Kind != proto.KindAI {
			return fmt.Errorf("unknown seat kind %q", ss.Kind)
		}
		r.seats = append(r.seats, ss.seat())
	}
	switch {
	case st.Watcher != nil:
		r.host = st.Watcher.seat()
		r.host.watching = true
	case st.Host >= 0 && st.Host < len(r.seats):
		r.host = r.seats[st.Host]
	}
	r.settings, r.seed, r.phase = st.Settings, st.Seed, st.Phase
	if st.Phase != proto.PhaseLobby {
		g, err := rules.Replay(len(r.seats), st.Settings.Settings, st.Seed, st.Actions)
		if err != nil {
			return fmt.Errorf("replaying the game: %w", err)
		}
		r.game = g
		if g.Over() {
			r.phase = proto.PhaseOver
		} else if r.usesModel() {
			// A game that was running keeps its model players.
			r.opts.quota.force(r.code)
		}
	}
	return nil
}

// SaveAll writes every open room to dir, one file per room, and removes
// files of rooms that have closed. Files hold seat tokens, so only the
// server's user may read them.
func (m *Manager) SaveAll(dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	m.mu.Lock()
	rooms := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	m.mu.Unlock()

	keep := map[string]bool{}
	var errs []error
	for _, r := range rooms {
		st, ok := r.Snapshot()
		if !ok {
			continue
		}
		name := st.Code + ".json"
		if err := writeFile(filepath.Join(dir, name), st); err != nil {
			errs = append(errs, err)
			continue
		}
		keep[name] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		errs = append(errs, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") && !keep[e.Name()] {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return len(keep), errors.Join(errs...)
}

func writeFile(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RestoreAll reopens the rooms saved in dir, with their codes. A file that
// can't be restored is logged and skipped. It returns how many reopened.
func (m *Manager) RestoreAll(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			m.log().Error("reading saved rooms", "dir", dir, "err", err)
		}
		return 0
	}
	n := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := m.restoreFile(path); err != nil {
			m.log().Error("restoring a room", "file", path, "err", err)
			continue
		}
		n++
	}
	return n
}

func (m *Manager) restoreFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	if len(st.Code) != CodeLen || strings.Trim(st.Code, codeAlphabet) != "" {
		return fmt.Errorf("bad room code %q", st.Code)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rooms[st.Code] != nil {
		return fmt.Errorf("room %s is already open", st.Code)
	}
	r := New(st.Code, m.opts)
	if err := r.restore(&st); err != nil {
		return err
	}
	m.start(r)
	return nil
}

func (m *Manager) log() *slog.Logger {
	if m.opts.Logger != nil {
		return m.opts.Logger
	}
	return slog.Default()
}
