package rules

import "slices"

// PlanetView is a planet as one player may see it. Nil fields are hidden.
type PlanetView struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	Pos        Pos      `json:"pos"`
	Owner      int      `json:"owner"`
	Ships      *int     `json:"ships,omitempty"`
	Production *int     `json:"production,omitempty"`
	Kill       *float64 `json:"kill,omitempty"`
}

// PlayerView is what everyone may see about a seat.
type PlayerView struct {
	Seat  int  `json:"seat"`
	Alive bool `json:"alive"`
}

// View is the game as one seat may see it. It never contains enemy fleets
// or stats the settings hide.
type View struct {
	Seat     int          `json:"seat"`
	Round    int          `json:"round"`
	Active   int          `json:"active"`
	Settings Settings     `json:"settings"`
	Planets  []PlanetView `json:"planets"`
	Fleets   []Fleet      `json:"fleets"`
	Players  []PlayerView `json:"players"`
	Stats    Stats        `json:"stats"`
	Over     bool         `json:"over"`
	Winner   int          `json:"winner"`
}

// Spectator is the seat number of someone watching rather than playing.
const Spectator = -2

// Spectate is the view of someone watching. With full, they see everything:
// every planet's stats and every fleet (for games with no human players,
// where nothing can be spoiled). Otherwise they see what any player could.
func (g *Game) Spectate(full bool) View {
	v := g.View(Spectator)
	if !full {
		return v
	}
	for i, p := range g.st.Planets {
		ships, prod, kill := p.Ships, p.Production, p.Kill
		v.Planets[i].Ships, v.Planets[i].Production, v.Planets[i].Kill = &ships, &prod, &kill
	}
	v.Fleets = slices.Clone(g.st.Fleets)
	if v.Fleets == nil {
		v.Fleets = []Fleet{}
	}
	return v
}

// View builds seat's view of the game.
func (g *Game) View(seat int) View {
	st := &g.st
	v := View{Seat: seat, Round: st.Round, Active: st.Active, Settings: st.Settings,
		Over: st.Over, Winner: st.Winner, Fleets: []Fleet{}}
	if seat >= 0 && seat < len(st.Players) {
		v.Stats = st.Players[seat].Stats
	}
	for _, p := range st.Planets {
		pv := PlanetView{ID: p.ID, Name: p.Name, Pos: p.Pos, Owner: p.Owner}
		if visibleStats(st.Settings, seat, p.Owner) {
			ships, prod, kill := p.Ships, p.Production, p.Kill
			pv.Ships, pv.Production, pv.Kill = &ships, &prod, &kill
		}
		v.Planets = append(v.Planets, pv)
	}
	for _, f := range st.Fleets {
		if f.Owner == seat {
			v.Fleets = append(v.Fleets, f)
		}
	}
	for _, p := range st.Players {
		v.Players = append(v.Players, PlayerView{Seat: p.Seat, Alive: p.Alive})
	}
	return v
}

func visibleStats(s Settings, seat, owner int) bool {
	switch {
	case owner == seat:
		return true
	case s.BlindMap:
		return false
	case owner == Neutral:
		return s.NeutralStats
	}
	return true
}

// FilterEvents returns the events seat may see.
func FilterEvents(events []Event, seat int) []Event {
	var out []Event
	for _, e := range events {
		if e.VisibleTo(seat) {
			out = append(out, e)
		}
	}
	return out
}

// Standing is one seat's place in the standings.
type Standing struct {
	Seat    int   `json:"seat"`
	Rank    int   `json:"rank"`
	Planets int   `json:"planets"`
	Ships   int   `json:"ships"`
	Alive   bool  `json:"alive"`
	Stats   Stats `json:"stats"`
}

// Standings ranks every seat by planets, then total ships. Ties share a rank.
func (g *Game) Standings() []Standing { return standings(&g.st) }

func standings(st *State) []Standing {
	out := make([]Standing, len(st.Players))
	for i, p := range st.Players {
		out[i] = Standing{Seat: i, Alive: p.Alive, Stats: p.Stats}
	}
	for _, p := range st.Planets {
		if p.Owner != Neutral {
			out[p.Owner].Planets++
			out[p.Owner].Ships += p.Ships
		}
	}
	for _, f := range st.Fleets {
		out[f.Owner].Ships += f.Ships
	}
	slices.SortStableFunc(out, func(a, b Standing) int {
		if a.Planets != b.Planets {
			return b.Planets - a.Planets
		}
		return b.Ships - a.Ships
	})
	for i := range out {
		if i > 0 && out[i].Planets == out[i-1].Planets && out[i].Ships == out[i-1].Ships {
			out[i].Rank = out[i-1].Rank
		} else {
			out[i].Rank = i + 1
		}
	}
	return out
}

// winnerOf is the top seat, or -1 if the top rank is shared.
func winnerOf(s []Standing) int {
	if len(s) == 0 || (len(s) > 1 && s[1].Rank == 1) {
		return -1
	}
	return s[0].Seat
}
