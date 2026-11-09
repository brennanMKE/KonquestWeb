// Package ai holds computer players. Each one sees only its own per-player
// view, never the full game state.
package ai

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"parallax/internal/rules"
)

// Order is one fleet order.
type Order struct {
	From  int `json:"from"`
	To    int `json:"to"`
	Ships int `json:"ships"`
}

// Player decides a turn's orders from a view.
type Player interface {
	Name() string
	Orders(v rules.View) []Order
}

// TakeTurn plays seat's turn in g with p: it launches p's orders, then ends
// the turn. It returns the round's events if the turn resolved the round.
func TakeTurn(g *rules.Game, seat int, p Player) ([]rules.Event, error) {
	for _, o := range p.Orders(g.View(seat)) {
		if _, err := g.Launch(seat, o.From, o.To, o.Ships); err != nil {
			return nil, fmt.Errorf("%s order %+v: %w", p.Name(), o, err)
		}
	}
	return g.EndTurn(seat)
}

// Greedy is the rule-based player. Each turn, every planet keeps a small
// reserve and sends its spare ships at the best target it can take alone,
// valuing production per ship spent and per round of travel. It's
// deterministic, and the baseline the System 1 advisor has to beat.
type Greedy struct{}

// Name implements Player.
func (Greedy) Name() string { return "Greedy" }

// Guesses for stats a view hides.
const (
	unknownShips      = 15
	unknownProduction = 10
	unknownKill       = 0.6
)

// Orders implements Player.
func (Greedy) Orders(v rules.View) []Order {
	var own, targets []rules.PlanetView
	for _, p := range v.Planets {
		if p.Owner == v.Seat {
			own = append(own, p)
		} else {
			targets = append(targets, p)
		}
	}
	spare := map[int]int{}
	for _, p := range own {
		spare[p.ID] = max(0, *p.Ships-reserve(p))
	}
	// Strongest planets choose first; ties by ID keep it deterministic.
	slices.SortFunc(own, func(a, b rules.PlanetView) int {
		return cmp.Or(*b.Ships-*a.Ships, a.ID-b.ID)
	})

	targeted := map[int]bool{}
	var orders []Order
	for _, src := range own {
		best, bestNeed, bestScore := -1, 0, 0.0
		for _, t := range targets {
			if targeted[t.ID] {
				continue
			}
			turns := rules.TravelTurns(src.Pos, t.Pos, v.Settings.ShipSpeed)
			need := shipsNeeded(t, turns, *src.Kill)
			if need > spare[src.ID] {
				continue
			}
			score := float64(production(t)) / float64(need+3*turns)
			if score > bestScore {
				best, bestNeed, bestScore = t.ID, need, score
			}
		}
		if best >= 0 {
			orders = append(orders, Order{From: src.ID, To: best, Ships: bestNeed})
			spare[src.ID] -= bestNeed
			targeted[best] = true
		}
	}
	return orders
}

// reserve is how many ships a planet keeps home: half a round's production.
func reserve(p rules.PlanetView) int { return *p.Production / 2 }

func production(p rules.PlanetView) int {
	if p.Production != nil {
		return *p.Production
	}
	return unknownProduction
}

// shipsNeeded estimates the fleet that takes t after turns rounds of travel,
// sent from a planet with kill percentage attKill. Each exchange the attacker
// loses about defKill ships and the defender about attKill, so the attacker
// needs defenders × defKill/attKill, plus a margin for bad luck.
func shipsNeeded(t rules.PlanetView, turns int, attKill float64) int {
	ships := unknownShips
	if t.Ships != nil {
		ships = *t.Ships
	}
	growth := rules.NeutralGrowth
	if t.Owner != rules.Neutral {
		growth = production(t)
	}
	defKill := unknownKill
	if t.Kill != nil {
		defKill = *t.Kill
	}
	defenders := float64(ships + growth*turns)
	return int(math.Ceil(defenders*max(1, defKill/attKill)*1.25)) + 1
}
