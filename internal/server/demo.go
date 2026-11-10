package server

import (
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"parallax/internal/ai"
	"parallax/internal/proto"
	"parallax/internal/rules"
)

// The start page plays a computer game behind its form. Each demo is a whole
// game between rule-based computers, played out here in milliseconds
// and sent as a replay. A demo is shared by every visitor for demoTTL, so a
// busy start page costs one game a minute.
const (
	demoTTL     = time.Minute
	demoPlayers = 6
	demoRounds  = 60
)

type demoCache struct {
	mu      sync.Mutex
	replay  *proto.Replay
	expires time.Time
}

func (s *Server) demo(w http.ResponseWriter, r *http.Request) {
	s.demos.mu.Lock()
	defer s.demos.mu.Unlock()
	if s.demos.replay == nil || time.Now().After(s.demos.expires) {
		rp, err := newDemo(rand.Uint64())
		if err != nil {
			s.log.Error("demo game", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "couldn't make a demo game"})
			return
		}
		s.demos.replay, s.demos.expires = rp, time.Now().Add(demoTTL)
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, s.demos.replay)
}

// newDemo plays a seeded game on the standard cube between Greedy
// computers (about 2 ms; Planner takes a second, too long for a page load)
// and returns it as a replay. The room code (which names the
// factions and planets) comes from the seed too.
func newDemo(seed uint64) (*proto.Replay, error) {
	ps := proto.DefaultSettings()
	ps.MaxRounds = demoRounds
	g, err := rules.New(demoPlayers, ps.Settings, seed)
	if err != nil {
		return nil, err
	}
	for !g.Over() {
		if _, err := ai.TakeTurn(g, g.Active(), ai.Greedy{}); err != nil {
			return nil, err
		}
	}
	rng := rand.New(rand.NewPCG(seed, 1))
	code := make([]byte, 4)
	for i := range code {
		code[i] = "ABCDEFGHJKMNPQRSTUVWXYZ"[rng.IntN(23)]
	}
	f := &proto.SaveFile{Version: proto.SaveVersion, Seed: seed, Room: string(code), Round: g.Round(), Settings: ps, Actions: g.Actions()}
	for range demoPlayers {
		f.Seats = append(f.Seats, proto.SavedSeat{Name: "Computer", Kind: proto.KindAI, AIKind: proto.AIGreedy})
	}
	return proto.BuildReplay(f)
}
