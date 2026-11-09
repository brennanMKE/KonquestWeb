// Command sim plays a seeded game between computer players and prints the
// turn log and final standings.
package main

import (
	"flag"
	"fmt"
	"os"

	"parallax/internal/ai"
	"parallax/internal/rules"
)

func main() {
	s := rules.DefaultSettings()
	players := flag.Int("players", 4, "number of players (2-9)")
	seed := flag.Uint64("seed", 1, "game seed")
	flag.IntVar(&s.Width, "width", s.Width, "map width")
	flag.IntVar(&s.Height, "height", s.Height, "map height")
	flag.IntVar(&s.Depth, "depth", s.Depth, "map depth (1 = flat)")
	flag.IntVar(&s.Neutrals, "neutrals", s.Neutrals, "neutral planets")
	flag.Float64Var(&s.ShipSpeed, "speed", s.ShipSpeed, "ship speed in cells per round")
	flag.BoolVar(&s.BlindMap, "blind", s.BlindMap, "blind map")
	flag.IntVar(&s.MaxRounds, "rounds", 200, "max rounds (0 = unlimited)")
	quiet := flag.Bool("q", false, "print only the standings")
	flag.Parse()

	if err := run(*players, s, *seed, *quiet); err != nil {
		fmt.Fprintln(os.Stderr, "sim:", err)
		os.Exit(1)
	}
}

func run(players int, s rules.Settings, seed uint64, quiet bool) error {
	g, err := rules.New(players, s, seed)
	if err != nil {
		return err
	}
	st := g.State()
	if !quiet {
		fmt.Printf("Seed %d, %d players, %dx%dx%d map\n", seed, players, s.Width, s.Height, s.Depth)
		for _, p := range st.Planets {
			fmt.Printf("  %-2s at (%2d,%2d,%2d)  %-9s prod %2d  kill %.2f\n",
				p.Name, p.Pos.X, p.Pos.Y, p.Pos.Z, owner(p.Owner), p.Production, p.Kill)
		}
	}
	player := ai.Greedy{}
	for !g.Over() {
		round := g.Round()
		events, err := ai.TakeTurn(g, g.Active(), player)
		if err != nil {
			return err
		}
		if quiet || len(events) == 0 {
			continue
		}
		fmt.Printf("Round %d\n", round)
		for _, e := range events {
			fmt.Println("  " + describe(e, st.Planets))
		}
	}

	fmt.Printf("Game over after %d rounds. ", g.Round())
	if w := g.Winner(); w >= 0 {
		fmt.Printf("%s wins.\n", owner(w))
	} else {
		fmt.Println("No winner.")
	}
	fmt.Println("Rank  Player    Planets  Ships  Built  Destroyed  Lost  Conquered  Fleets")
	for _, r := range g.Standings() {
		fmt.Printf("%4d  %-8s  %7d  %5d  %5d  %9d  %4d  %9d  %6d\n", r.Rank, owner(r.Seat), r.Planets, r.Ships,
			r.Stats.ShipsBuilt, r.Stats.ShipsDestroyed, r.Stats.ShipsLost, r.Stats.PlanetsConquered, r.Stats.FleetsLaunched)
	}
	return nil
}

func owner(seat int) string {
	if seat == rules.Neutral {
		return "neutral"
	}
	return fmt.Sprintf("Player %d", seat+1)
}

func describe(e rules.Event, planets []rules.Planet) string {
	name := func() string { return "Planet " + planets[e.Planet].Name }
	switch e.Kind {
	case rules.EvReinforced:
		return fmt.Sprintf("Reinforcements have arrived at %s: %d ships, now %d.", name(), e.Ships, e.Remaining)
	case rules.EvCaptured:
		return fmt.Sprintf("%s has fallen to %s (from %s): %d attackers, %d lost, %d defenders destroyed.",
			name(), owner(e.Attacker), owner(e.Defender), e.Ships, e.AttackerLost, e.DefenderLost)
	case rules.EvHeld:
		return fmt.Sprintf("%s (%s) has held against an attack from %s: %d attackers destroyed, %d defenders left.",
			name(), owner(e.Defender), owner(e.Attacker), e.Ships, e.Remaining)
	case rules.EvEliminated:
		return fmt.Sprintf("%s has been eliminated.", owner(e.Attacker))
	case rules.EvGameOver:
		return "The game is over."
	}
	return string(e.Kind)
}
