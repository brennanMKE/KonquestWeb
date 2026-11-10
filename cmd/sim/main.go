// Command sim plays a seeded game between computer players and prints the
// turn log and final standings.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"parallax/internal/ai"
	"parallax/internal/proto"
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
	kinds := flag.String("ai", "greedy", "computer player per seat, comma-separated and repeated: greedy, planner, oracle or system1")
	url := flag.String("url", "", "System 1 decision endpoint, e.g. http://127.0.0.1:8000/predict")
	key := flag.String("key", "", "API key for the endpoint (sent as a Bearer token)")
	batch := flag.Bool("batch", true, "send several states in one request; off sends one request per state")
	savePath := flag.String("save", "", "write the finished game to this file, for the replay viewer (Watch saved games)")
	room := flag.String("room", "STAR", "room code in the saved file; faction and planet names come from it")
	verbose := flag.Bool("v", false, "with system1 seats, print every answer that disagrees with the oracle")
	flag.Parse()

	cfg := config{kinds: strings.Split(*kinds, ","), quiet: *quiet, verbose: *verbose, save: *savePath, room: strings.ToUpper(*room)}
	if *url != "" {
		cfg.client = &ai.Client{URL: *url, APIKey: *key, Batch: *batch}
	}
	if err := run(*players, s, *seed, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "sim:", err)
		os.Exit(1)
	}
}

type config struct {
	kinds   []string
	client  *ai.Client
	quiet   bool
	verbose bool
	save    string
	room    string
}

// tally counts a System 1 model's answers against the oracle's.
type tally struct {
	asked, agreed int
	byTruth       map[string][2]int // truth -> {asked, agreed}
	elapsed       time.Duration
	decisions     int
}

func run(players int, s rules.Settings, seed uint64, cfg config) error {
	quiet := cfg.quiet
	g, err := rules.New(players, s, seed)
	if err != nil {
		return err
	}
	kinds := make([]string, players)
	for i := range kinds {
		kinds[i] = strings.TrimSpace(cfg.kinds[i%len(cfg.kinds)])
		switch kinds[i] {
		case "greedy", "planner", "oracle":
		case "system1":
			if cfg.client == nil {
				return fmt.Errorf("system1 needs -url")
			}
		default:
			return fmt.Errorf("unknown AI %q", kinds[i])
		}
	}
	t := tally{byTruth: map[string][2]int{}}
	st := g.State()
	if !quiet {
		fmt.Printf("Seed %d, %d players, %dx%dx%d map\n", seed, players, s.Width, s.Height, s.Depth)
		for _, p := range st.Planets {
			fmt.Printf("  %-2s at (%2d,%2d,%2d)  %-9s prod %2d  kill %.2f\n",
				p.Name, p.Pos.X, p.Pos.Y, p.Pos.Z, owner(p.Owner), p.Production, p.Kill)
		}
	}
	for !g.Over() {
		round := g.Round()
		events, err := turn(g, kinds[g.Active()], cfg, &t)
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

	if cfg.save != "" {
		if err := saveGame(cfg.save, cfg.room, g, s, kinds); err != nil {
			return err
		}
	}
	if t.asked > 0 {
		fmt.Printf("System 1 agreed with the oracle on %d of %d questions (%.1f%%), %.0f ms per decision.\n",
			t.agreed, t.asked, 100*float64(t.agreed)/float64(t.asked), float64(t.elapsed.Milliseconds())/float64(t.decisions))
		for _, truth := range []string{"good", "bad", "danger", "safe"} {
			n := t.byTruth[truth]
			if n[0] > 0 {
				fmt.Printf("  when the oracle said %-7s %d of %d\n", truth+":", n[1], n[0])
			}
		}
	}
	fmt.Printf("Game over after %d rounds. ", g.Round())
	if w := g.Winner(); w >= 0 {
		fmt.Printf("%s wins.\n", owner(w))
	} else {
		fmt.Println("No winner.")
	}
	fmt.Println("Rank  Player    AI        Planets  Ships  Built  Destroyed  Lost  Conquered  Fleets")
	for _, r := range g.Standings() {
		fmt.Printf("%4d  %-8s  %-8s  %7d  %5d  %5d  %9d  %4d  %9d  %6d\n", r.Rank, owner(r.Seat), kinds[r.Seat], r.Planets, r.Ships,
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

// turn plays the active seat. System 1 seats ask the model, and every answer
// is checked against the oracle.
func turn(g *rules.Game, kind string, cfg config, t *tally) ([]rules.Event, error) {
	seat := g.Active()
	switch kind {
	case "greedy":
		return ai.TakeTurn(g, seat, ai.Greedy{})
	case "planner":
		return ai.TakeTurn(g, seat, ai.Planner{})
	case "oracle":
		return ai.TakeTurn(g, seat, ai.Oracle{Game: g})
	}
	v := g.View(seat)
	sit := ai.Assess(v)
	var orders []ai.Order
	if sit.Empty() {
		orders = ai.Decide(sit, ai.Answers{})
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		start := time.Now()
		ans, err := cfg.client.Ask(ctx, sit.Prompt())
		cancel()
		if err != nil {
			return nil, fmt.Errorf("model: %w", err)
		}
		t.elapsed += time.Since(start)
		t.decisions++
		truth := ai.OracleAnswers(g.State(), sit)
		check := func(key, state string) {
			want, got := truth[key].Choice, ans[key].Choice
			n := t.byTruth[want]
			n[0]++
			t.asked++
			if got == want {
				n[1]++
				t.agreed++
			} else if cfg.verbose {
				fmt.Printf("  model said %q, oracle %q: %s\n", got, want, state)
			}
			t.byTruth[want] = n
		}
		for _, d := range sit.Dangers {
			check(d.Key+".danger", d.State)
		}
		for _, c := range sit.Attacks {
			check(c.Key+".move", c.State)
		}
		orders = ai.Decide(sit, ans)
	}
	for _, o := range orders {
		if _, err := g.Launch(seat, o.From, o.To, o.Ships); err != nil {
			return nil, err
		}
	}
	return g.EndTurn(seat)
}

// saveGame writes the game as a save file, every seat a computer.
func saveGame(path, room string, g *rules.Game, s rules.Settings, kinds []string) error {
	ps := proto.DefaultSettings()
	ps.Settings = s
	f := proto.SaveFile{Version: proto.SaveVersion, Seed: g.Seed(), Room: room, Round: g.Round(), Settings: ps, Actions: g.Actions()}
	for i, k := range kinds {
		if k == "oracle" {
			k = proto.AIPlanner
		}
		f.Seats = append(f.Seats, proto.SavedSeat{Name: fmt.Sprintf("Player %d", i+1), Kind: proto.KindAI, AIKind: k})
	}
	b, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
