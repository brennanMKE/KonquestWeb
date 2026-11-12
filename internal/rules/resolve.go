package rules

import (
	"math/rand/v2"
	"slices"
)

// EventKind names something that happened during round resolution.
type EventKind string

const (
	EvReinforced EventKind = "reinforced"
	EvCaptured   EventKind = "captured"
	EvHeld       EventKind = "held"
	EvEliminated EventKind = "eliminated"
	// EvFallen: a player lost their last planet but still has fleets in
	// flight. EvRisen: a player with no planets took one.
	EvFallen   EventKind = "fallen"
	EvRisen    EventKind = "risen"
	EvGameOver EventKind = "game_over"
)

// Event is one entry in the turn log.
type Event struct {
	Kind  EventKind `json:"kind"`
	Round int       `json:"round"`
	// Planet is the planet involved, or -1.
	Planet int `json:"planet"`
	// Attacker is the fleet's owner (reinforced, captured, held), the
	// eliminated, fallen or risen seat, or the winner (game_over, -1 for
	// none).
	Attacker int `json:"attacker"`
	// Defender is the planet's owner before the battle (may be Neutral).
	Defender int `json:"defender"`
	// Ships is the number of ships that arrived.
	Ships        int `json:"ships"`
	AttackerLost int `json:"attackerLost"`
	DefenderLost int `json:"defenderLost"`
	// Remaining is the planet's garrison after the event.
	Remaining int `json:"remaining"`
	// Seats are the players who may see this event; empty means everyone.
	Seats []int `json:"seats,omitempty"`
}

// VisibleTo reports whether seat may see the event.
func (e Event) VisibleTo(seat int) bool {
	return len(e.Seats) == 0 || slices.Contains(e.Seats, seat)
}

// resolve runs end-of-round resolution in the order RULES.md gives:
// arrivals, production, elimination, game over.
func (g *Game) resolve() []Event {
	r := g.st.Round
	events := []Event{} // non-nil: EndTurn callers use nil to mean "round not resolved"

	var arriving, staying []Fleet
	for _, f := range g.st.Fleets {
		if f.ArrivalRound <= r {
			arriving = append(arriving, f)
		} else {
			staying = append(staying, f)
		}
	}
	g.st.Fleets = staying
	slices.SortFunc(arriving, func(a, b Fleet) int { return a.ID - b.ID })
	before := g.planetCounts()
	for _, f := range arriving {
		events = append(events, g.arrive(f))
	}
	events = append(events, g.fallenAndRisen(r, before, events)...)

	for i := range g.st.Planets {
		p := &g.st.Planets[i]
		switch {
		case p.Owner == Neutral:
			p.Ships += NeutralGrowth
		case p.CapturedRound == r && !g.st.Settings.CaptureProducesImmediately:
		default:
			p.Ships += p.Production
			g.st.Players[p.Owner].Stats.ShipsBuilt += p.Production
		}
	}

	alive := 0
	for seat := range g.st.Players {
		pl := &g.st.Players[seat]
		if !pl.Alive {
			continue
		}
		if !g.hasAssets(seat) {
			pl.Alive = false
			events = append(events, Event{Kind: EvEliminated, Round: r, Planet: -1, Attacker: seat, Defender: Neutral})
			continue
		}
		alive++
	}

	switch {
	case alive <= 1:
		g.st.Over = true
		g.st.Winner = g.nextAlive(0)
	case g.st.Settings.MaxRounds > 0 && r >= g.st.Settings.MaxRounds:
		g.st.Over = true
		g.st.Winner = winnerOf(standings(&g.st))
	}
	if g.st.Over {
		events = append(events, Event{Kind: EvGameOver, Round: r, Planet: -1, Attacker: g.st.Winner, Defender: Neutral})
	}
	return events
}

// planetCounts is how many planets each seat owns.
func (g *Game) planetCounts() []int {
	n := make([]int, len(g.st.Players))
	for _, p := range g.st.Planets {
		if p.Owner >= 0 && p.Owner < len(n) {
			n[p.Owner]++
		}
	}
	return n
}

// fallenAndRisen reports the seats that lost their last planet this round
// while fleets of theirs are still in flight (with none left, elimination
// says it), and the seats that had no planet and took one. Everyone sees
// both. A risen event names the first planet taken.
func (g *Game) fallenAndRisen(r int, before []int, arrivals []Event) []Event {
	after := g.planetCounts()
	var out []Event
	for seat := range g.st.Players {
		if !g.st.Players[seat].Alive {
			continue
		}
		switch {
		case before[seat] > 0 && after[seat] == 0 && g.hasAssets(seat):
			out = append(out, Event{Kind: EvFallen, Round: r, Planet: -1, Attacker: seat, Defender: Neutral})
		case before[seat] == 0 && after[seat] > 0:
			planet := -1
			for _, e := range arrivals {
				if e.Kind == EvCaptured && e.Attacker == seat {
					planet = e.Planet
					break
				}
			}
			out = append(out, Event{Kind: EvRisen, Round: r, Planet: planet, Attacker: seat, Defender: Neutral})
		}
	}
	return out
}

func (g *Game) hasAssets(seat int) bool {
	for _, p := range g.st.Planets {
		if p.Owner == seat {
			return true
		}
	}
	for _, f := range g.st.Fleets {
		if f.Owner == seat {
			return true
		}
	}
	return false
}

// arrive lands one fleet: reinforcement if its owner holds the planet,
// otherwise combat.
func (g *Game) arrive(f Fleet) Event {
	r := g.st.Round
	p := &g.st.Planets[f.To]
	if p.Owner == f.Owner {
		p.Ships += f.Ships
		return Event{Kind: EvReinforced, Round: r, Planet: p.ID, Attacker: f.Owner, Defender: f.Owner,
			Ships: f.Ships, Remaining: p.Ships, Seats: []int{f.Owner}}
	}

	def := p.Owner
	attLeft, defLeft := battle(g.rng, f.Ships, f.Kill, p.Ships, p.Kill)
	ev := Event{Round: r, Planet: p.ID, Attacker: f.Owner, Defender: def, Ships: f.Ships,
		AttackerLost: f.Ships - attLeft, DefenderLost: p.Ships - defLeft, Seats: []int{f.Owner}}
	if def != Neutral {
		ev.Seats = append(ev.Seats, def)
	}

	att := &g.st.Players[f.Owner].Stats
	att.ShipsLost += ev.AttackerLost
	att.ShipsDestroyed += ev.DefenderLost
	if def != Neutral {
		d := &g.st.Players[def].Stats
		d.ShipsLost += ev.DefenderLost
		d.ShipsDestroyed += ev.AttackerLost
	}

	if defLeft == 0 {
		p.Owner, p.Ships, p.CapturedRound = f.Owner, attLeft, r
		att.PlanetsConquered++
		ev.Kind = EvCaptured
	} else {
		p.Ships = defLeft
		ev.Kind = EvHeld
	}
	ev.Remaining = p.Ships
	return ev
}

// battle fights exchanges until one side has no ships. In each exchange the
// defenders fire first, then the attackers if any are left. It returns the
// survivors on each side; exactly one of them is 0. Kill values must be > 0.
func battle(rng *rand.Rand, att int, attKill float64, def int, defKill float64) (int, int) {
	for att > 0 && def > 0 {
		if rng.Float64() < defKill {
			att--
			if att == 0 {
				break
			}
		}
		if rng.Float64() < attKill {
			def--
		}
	}
	return att, def
}
