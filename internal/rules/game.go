package rules

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

// Errors returned for rejected actions.
var (
	ErrGameOver      = errors.New("game is over")
	ErrNotYourTurn   = errors.New("not your turn")
	ErrNoSuchPlanet  = errors.New("no such planet")
	ErrSamePlanet    = errors.New("source and destination are the same planet")
	ErrNotOwner      = errors.New("you don't own the source planet")
	ErrBadShipCount  = errors.New("ship count must be from 1 to the ships on the planet")
	ErrUnknownAction = errors.New("unknown action")
)

// ActionKind names an entry in the actions log.
type ActionKind string

const (
	ActLaunch  ActionKind = "launch"
	ActEndTurn ActionKind = "end_turn"
)

// Action is one accepted player action. The seed, settings and actions log
// replay a game exactly.
type Action struct {
	Kind  ActionKind `json:"kind"`
	Seat  int        `json:"seat"`
	From  int        `json:"from,omitempty"`
	To    int        `json:"to,omitempty"`
	Ships int        `json:"ships,omitempty"`
}

// Game is a running game: its state, its seeded RNG and its actions log.
// It isn't safe for concurrent use; the room goroutine owns it.
type Game struct {
	st      State
	rng     *rand.Rand
	seed    uint64
	actions []Action
}

// New creates a game for players seats. The seed decides the map and every
// battle.
func New(players int, s Settings, seed uint64) (*Game, error) {
	if err := s.Validate(players); err != nil {
		return nil, err
	}
	g := &Game{rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), seed: seed}
	g.st = State{Settings: s, Round: 1, Active: 0, Winner: -1, Players: make([]Player, players)}
	for i := range g.st.Players {
		g.st.Players[i] = Player{Seat: i, Alive: true}
	}
	g.st.Planets = generateMap(g.rng, players, s)
	return g, nil
}

// Replay rebuilds a game from its seed, settings and actions log.
func Replay(players int, s Settings, seed uint64, actions []Action) (*Game, error) {
	g, err := New(players, s, seed)
	if err != nil {
		return nil, err
	}
	for i, a := range actions {
		if err := g.Apply(a); err != nil {
			return nil, fmt.Errorf("action %d (%+v): %w", i, a, err)
		}
	}
	return g, nil
}

// Apply performs a logged action. Events from a resolved round are dropped;
// use Launch and EndTurn directly to receive them.
func (g *Game) Apply(a Action) error {
	switch a.Kind {
	case ActLaunch:
		_, err := g.Launch(a.Seat, a.From, a.To, a.Ships)
		return err
	case ActEndTurn:
		_, err := g.EndTurn(a.Seat)
		return err
	}
	return ErrUnknownAction
}

// State returns a copy of the full state.
func (g *Game) State() State { return g.st.clone() }

// Seed returns the game's seed.
func (g *Game) Seed() uint64 { return g.seed }

// Actions returns a copy of the actions log.
func (g *Game) Actions() []Action { return append([]Action(nil), g.actions...) }

// Round is the current round, starting at 1.
func (g *Game) Round() int { return g.st.Round }

// Active is the seat whose turn it is, or -1 when the game is over.
func (g *Game) Active() int { return g.st.Active }

// Over reports whether the game has ended.
func (g *Game) Over() bool { return g.st.Over }

// Winner is the winning seat, or -1.
func (g *Game) Winner() int { return g.st.Winner }

// Players is the number of seats.
func (g *Game) Players() int { return len(g.st.Players) }

func (g *Game) checkTurn(seat int) error {
	if g.st.Over {
		return ErrGameOver
	}
	if seat != g.st.Active {
		return ErrNotYourTurn
	}
	return nil
}

// Launch sends ships from one of seat's planets to another planet. The ships
// leave at once and the order is final.
func (g *Game) Launch(seat, from, to, ships int) (Fleet, error) {
	if err := g.checkTurn(seat); err != nil {
		return Fleet{}, err
	}
	n := len(g.st.Planets)
	if from < 0 || from >= n || to < 0 || to >= n {
		return Fleet{}, ErrNoSuchPlanet
	}
	if from == to {
		return Fleet{}, ErrSamePlanet
	}
	src := &g.st.Planets[from]
	if src.Owner != seat {
		return Fleet{}, ErrNotOwner
	}
	if ships < 1 || ships > src.Ships {
		return Fleet{}, ErrBadShipCount
	}
	src.Ships -= ships
	r := g.st.Round
	f := Fleet{
		ID: g.st.NextFleetID, Owner: seat, From: from, To: to, Ships: ships, Kill: src.Kill,
		LaunchRound:  r,
		ArrivalRound: r + TravelTurns(src.Pos, g.st.Planets[to].Pos, g.st.Settings.ShipSpeed) - 1,
	}
	g.st.NextFleetID++
	g.st.Fleets = append(g.st.Fleets, f)
	g.st.Players[seat].Stats.FleetsLaunched++
	g.actions = append(g.actions, Action{Kind: ActLaunch, Seat: seat, From: from, To: to, Ships: ships})
	return f, nil
}

// EndTurn ends seat's turn. If it was the last living seat in the round, the
// round resolves and its events are returned, non-nil even when empty.
// Otherwise events are nil.
func (g *Game) EndTurn(seat int) ([]Event, error) {
	if err := g.checkTurn(seat); err != nil {
		return nil, err
	}
	g.actions = append(g.actions, Action{Kind: ActEndTurn, Seat: seat})
	if next := g.nextAlive(seat + 1); next >= 0 {
		g.st.Active = next
		return nil, nil
	}
	events := g.resolve()
	if g.st.Over {
		g.st.Active = -1
	} else {
		g.st.Round++
		g.st.Active = g.nextAlive(0)
	}
	return events, nil
}

// nextAlive returns the first living seat at or after from, or -1.
func (g *Game) nextAlive(from int) int {
	for s := from; s < len(g.st.Players); s++ {
		if g.st.Players[s].Alive {
			return s
		}
	}
	return -1
}
