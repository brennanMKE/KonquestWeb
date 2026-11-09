package rules

import (
	"errors"
	"math/rand/v2"
	"reflect"
	"testing"
)

// testGame builds a game from a hand-made map. Planets get IDs and names
// from their order.
func testGame(t *testing.T, players int, s Settings, planets []Planet) *Game {
	t.Helper()
	g := &Game{rng: rand.New(rand.NewPCG(1, 2)), seed: 1}
	g.st = State{Settings: s, Round: 1, Winner: -1, Players: make([]Player, players)}
	for i := range g.st.Players {
		g.st.Players[i] = Player{Seat: i, Alive: true}
	}
	for i := range planets {
		planets[i].ID, planets[i].Name = i, PlanetName(i)
	}
	g.st.Planets = planets
	return g
}

// must panics on error, which fails the test with a stack trace.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// endRound ends every remaining turn in the current round and returns the
// round's events.
func endRound(t *testing.T, g *Game) []Event {
	t.Helper()
	r := g.Round()
	for !g.Over() && g.Round() == r {
		if evs := must(g.EndTurn(g.Active())); evs != nil {
			return evs
		}
	}
	return nil
}

func TestTravelTurns(t *testing.T) {
	tests := []struct {
		a, b  Pos
		speed float64
		want  int
	}{
		{Pos{0, 0, 0}, Pos{1, 0, 0}, 2, 1},
		{Pos{0, 0, 0}, Pos{4, 0, 0}, 2, 2},
		{Pos{0, 0, 0}, Pos{5, 0, 0}, 2, 3},
		{Pos{0, 0, 0}, Pos{3, 4, 0}, 2, 3},  // distance 5
		{Pos{0, 0, 0}, Pos{2, 3, 6}, 2, 4},  // 3D distance 7
		{Pos{0, 0, 0}, Pos{2, 3, 6}, 1, 7},  // exact multiple
		{Pos{0, 0, 0}, Pos{1, 1, 1}, 10, 1}, // never below 1
		{Pos{5, 5, 0}, Pos{0, 0, 0}, 1, 8},  // 7.07 rounds up
	}
	for _, tt := range tests {
		if got := TravelTurns(tt.a, tt.b, tt.speed); got != tt.want {
			t.Errorf("TravelTurns(%v, %v, %v) = %d, want %d", tt.a, tt.b, tt.speed, got, tt.want)
		}
	}
}

func TestPlanetName(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 51: "AZ", 52: "BA", 701: "ZZ", 702: "AAA"} {
		if got := PlanetName(i); got != want {
			t.Errorf("PlanetName(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestSettingsValidate(t *testing.T) {
	ok := DefaultSettings()
	if err := ok.Validate(9); err != nil {
		t.Fatalf("default settings, 9 players: %v", err)
	}
	bad := map[string]func(s *Settings) int{
		"one player":     func(s *Settings) int { return 1 },
		"ten players":    func(s *Settings) int { return 10 },
		"tiny":           func(s *Settings) int { s.Width = 3; return 2 },
		"no depth":       func(s *Settings) int { s.Depth = 0; return 2 },
		"zero speed":     func(s *Settings) int { s.ShipSpeed = 0; return 2 },
		"negative max":   func(s *Settings) int { s.MaxRounds = -1; return 2 },
		"too many":       func(s *Settings) int { s.Width, s.Height, s.Neutrals = 4, 4, 7; return 2 },
		"neg neutrals":   func(s *Settings) int { s.Neutrals = -1; return 2 },
		"huge":           func(s *Settings) int { s.Height = 65; return 2 },
		"too deep":       func(s *Settings) int { s.Depth = 65; return 2 },
		"NaN ship speed": func(s *Settings) int { s.ShipSpeed = nanValue(); return 2 },
	}
	for name, mod := range bad {
		s := DefaultSettings()
		if err := s.Validate(mod(&s)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func nanValue() float64 { z := 0.0; return z / z }

func TestGenerateMap(t *testing.T) {
	for _, depth := range []int{1, 4} {
		s := DefaultSettings()
		s.Depth = depth
		for seed := range uint64(50) {
			g := must(New(4, s, seed))
			st := g.State()
			if len(st.Planets) != 4+s.Neutrals {
				t.Fatalf("depth %d seed %d: %d planets", depth, seed, len(st.Planets))
			}
			seen := map[Pos]bool{}
			names := map[string]bool{}
			homes := map[int]bool{}
			zs := map[int]bool{}
			for i, p := range st.Planets {
				if p.ID != i || p.Name != PlanetName(i) {
					t.Fatalf("planet %d has ID %d name %q", i, p.ID, p.Name)
				}
				if seen[p.Pos] || names[p.Name] {
					t.Fatalf("duplicate position or name: %+v", p)
				}
				seen[p.Pos], names[p.Name] = true, true
				zs[p.Pos.Z] = true
				if p.Pos.X < 0 || p.Pos.X >= s.Width || p.Pos.Y < 0 || p.Pos.Y >= s.Height || p.Pos.Z < 0 || p.Pos.Z >= depth {
					t.Fatalf("out of bounds: %+v", p)
				}
				if p.Home {
					if homes[p.Owner] || p.Production != HomeProduction || p.Kill != HomeKill || p.Ships != HomeProduction {
						t.Fatalf("bad home: %+v", p)
					}
					homes[p.Owner] = true
				} else if p.Owner != Neutral || p.Production < NeutralMinProd || p.Production > NeutralMaxProd ||
					p.Kill < NeutralMinKill || p.Kill > NeutralMaxKill || p.Ships != p.Production {
					t.Fatalf("bad neutral: %+v", p)
				}
			}
			if len(homes) != 4 {
				t.Fatalf("want 4 homes, got %v", homes)
			}
			if depth == 1 && !reflect.DeepEqual(zs, map[int]bool{0: true}) {
				t.Fatalf("flat map has z values %v", zs)
			}
		}
	}
}

func TestGenerateMapDeterministicAndVaried(t *testing.T) {
	s := DefaultSettings()
	a := must(New(3, s, 42)).State()
	b := must(New(3, s, 42)).State()
	c := must(New(3, s, 43)).State()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed gave different maps")
	}
	if reflect.DeepEqual(a.Planets, c.Planets) {
		t.Fatal("different seeds gave the same map")
	}
}

func TestGenerateMapFullBoard(t *testing.T) {
	// Half the cells filled: placement must fall back to scanning.
	s := DefaultSettings()
	s.Width, s.Height, s.Neutrals = 4, 4, 6
	for seed := range uint64(20) {
		g := must(New(2, s, seed))
		if len(g.State().Planets) != 8 {
			t.Fatal("wrong planet count")
		}
	}
}

func TestHomesSpreadApart(t *testing.T) {
	// Homes pick the farthest of 64 candidates, so two homes on a 16x16 map
	// should almost never be close.
	s := DefaultSettings()
	for seed := range uint64(100) {
		var homes []Pos
		for _, p := range must(New(2, s, seed)).State().Planets {
			if p.Home {
				homes = append(homes, p.Pos)
			}
		}
		if d := Distance(homes[0], homes[1]); d < 8 {
			t.Errorf("seed %d: homes only %.1f apart", seed, d)
		}
	}
}

func TestBattle(t *testing.T) {
	tests := []struct {
		name             string
		att, def         int
		attKill, defKill float64
		wantAtt, wantDef int
	}{
		{"empty planet falls", 5, 0, 0.5, 0.5, 5, 0},
		// With kill 1 every shot hits: each exchange the attacker loses one,
		// then the defender loses one if the attacker has ships left.
		{"sure shots, attacker wins", 3, 1, 1, 1, 2, 0},
		{"sure shots, tie goes to defender", 3, 3, 1, 1, 0, 1},
		{"sure shots, defender wins", 2, 5, 1, 1, 0, 4},
		{"single attacker vs sure defender", 1, 1, 1, 1, 0, 1},
	}
	for _, tt := range tests {
		rng := rand.New(rand.NewPCG(7, 7))
		a, d := battle(rng, tt.att, tt.attKill, tt.def, tt.defKill)
		if a != tt.wantAtt || d != tt.wantDef {
			t.Errorf("%s: got (%d, %d), want (%d, %d)", tt.name, a, d, tt.wantAtt, tt.wantDef)
		}
	}
}

func TestBattleFixedSeedDeterministic(t *testing.T) {
	for seed := range uint64(20) {
		a1, d1 := battle(rand.New(rand.NewPCG(seed, 0)), 20, 0.4, 15, 0.6)
		a2, d2 := battle(rand.New(rand.NewPCG(seed, 0)), 20, 0.4, 15, 0.6)
		if a1 != a2 || d1 != d2 {
			t.Fatalf("seed %d: (%d,%d) vs (%d,%d)", seed, a1, d1, a2, d2)
		}
		if (a1 == 0) == (d1 == 0) {
			t.Fatalf("seed %d: exactly one side must be 0, got (%d,%d)", seed, a1, d1)
		}
	}
}

func TestBattleOdds(t *testing.T) {
	rng := rand.New(rand.NewPCG(99, 99))
	wins := func(att int, attKill float64, def int, defKill float64) float64 {
		n := 0
		for range 4000 {
			if _, d := battle(rng, att, attKill, def, defKill); d == 0 {
				n++
			}
		}
		return float64(n) / 4000
	}
	if p := wins(10, 0.5, 10, 0.5); p > 0.5 {
		t.Errorf("equal forces: attacker wins %.2f, want under 0.5 (defender fires first)", p)
	}
	if p := wins(30, 0.5, 10, 0.5); p < 0.95 {
		t.Errorf("3:1 attack: attacker wins %.2f, want over 0.95", p)
	}
	if p := wins(10, 0.3, 10, 0.9); p > 0.05 {
		t.Errorf("weak attacker vs strong defender: attacker wins %.2f", p)
	}
	if p := wins(10, 0.9, 10, 0.3); p < 0.9 {
		t.Errorf("strong attacker vs weak defender: attacker wins %.2f", p)
	}
}

func twoPlayerMap() []Planet {
	return []Planet{
		{Pos: Pos{0, 0, 0}, Owner: 0, Ships: 10, Production: 10, Kill: 0.4, Home: true}, // A
		{Pos: Pos{9, 0, 0}, Owner: 1, Ships: 10, Production: 10, Kill: 0.4, Home: true}, // B
		{Pos: Pos{2, 0, 0}, Owner: Neutral, Ships: 0, Production: 7, Kill: 0.5},         // C, 1 round from A
		{Pos: Pos{4, 0, 0}, Owner: Neutral, Ships: 5, Production: 8, Kill: 0.5},         // D, 2 rounds from A
	}
}

func TestLaunchValidation(t *testing.T) {
	g := testGame(t, 2, DefaultSettings(), twoPlayerMap())
	tests := []struct {
		name                  string
		seat, from, to, ships int
		want                  error
	}{
		{"not active", 1, 1, 0, 1, ErrNotYourTurn},
		{"no such source", 0, 9, 0, 1, ErrNoSuchPlanet},
		{"no such destination", 0, 0, -1, 1, ErrNoSuchPlanet},
		{"same planet", 0, 0, 0, 1, ErrSamePlanet},
		{"not owner", 0, 1, 0, 1, ErrNotOwner},
		{"neutral source", 0, 2, 0, 1, ErrNotOwner},
		{"zero ships", 0, 0, 1, 0, ErrBadShipCount},
		{"too many ships", 0, 0, 1, 11, ErrBadShipCount},
	}
	for _, tt := range tests {
		if _, err := g.Launch(tt.seat, tt.from, tt.to, tt.ships); !errors.Is(err, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, err, tt.want)
		}
	}
	if _, err := g.EndTurn(1); !errors.Is(err, ErrNotYourTurn) {
		t.Errorf("end turn out of order: got %v", err)
	}
	if len(g.Actions()) != 0 {
		t.Errorf("rejected actions were logged: %v", g.Actions())
	}
	if g.State().Planets[0].Ships != 10 {
		t.Error("rejected launch changed ships")
	}
}

func TestLaunch(t *testing.T) {
	g := testGame(t, 2, DefaultSettings(), twoPlayerMap())
	f := must(g.Launch(0, 0, 3, 4))
	want := Fleet{ID: 0, Owner: 0, From: 0, To: 3, Ships: 4, Kill: 0.4, LaunchRound: 1, ArrivalRound: 2}
	if f != want {
		t.Fatalf("fleet = %+v, want %+v", f, want)
	}
	if got := g.State().Planets[0].Ships; got != 6 {
		t.Fatalf("source has %d ships, want 6", got)
	}
	f2 := must(g.Launch(0, 0, 2, 6))
	if f2.ID != 1 || f2.ArrivalRound != 1 {
		t.Fatalf("second fleet = %+v", f2)
	}
	if _, err := g.Launch(0, 0, 2, 1); !errors.Is(err, ErrBadShipCount) {
		t.Fatalf("launch from empty planet: %v", err)
	}
	if got := g.State().Players[0].Stats.FleetsLaunched; got != 2 {
		t.Fatalf("fleets launched = %d", got)
	}
}

func TestTurnRotation(t *testing.T) {
	g := must(New(3, DefaultSettings(), 1))
	for round := 1; round <= 3; round++ {
		for seat := range 3 {
			if g.Round() != round || g.Active() != seat {
				t.Fatalf("want round %d seat %d, got round %d seat %d", round, seat, g.Round(), g.Active())
			}
			evs := must(g.EndTurn(seat))
			if (seat == 2) != (evs != nil || g.Round() == round+1) {
				t.Fatalf("round %d seat %d: resolution at wrong time", round, seat)
			}
		}
	}
}

func TestReinforcement(t *testing.T) {
	planets := twoPlayerMap()
	planets[2].Owner, planets[2].Ships = 0, 3
	g := testGame(t, 2, DefaultSettings(), planets)
	must(g.Launch(0, 0, 2, 4)) // 1 round
	evs := endRound(t, g)
	if len(evs) != 1 || evs[0].Kind != EvReinforced || evs[0].Ships != 4 || evs[0].Remaining != 7 {
		t.Fatalf("events = %+v", evs)
	}
	// Then production: 3 + 4 + 7.
	if got := g.State().Planets[2].Ships; got != 14 {
		t.Fatalf("planet C has %d ships, want 14", got)
	}
	if !reflect.DeepEqual(evs[0].Seats, []int{0}) {
		t.Fatalf("reinforcement visible to %v", evs[0].Seats)
	}
}

func TestFleetArrivesOnTime(t *testing.T) {
	g := testGame(t, 2, DefaultSettings(), twoPlayerMap())
	must(g.Launch(0, 0, 3, 10)) // D is 2 rounds away
	if evs := endRound(t, g); evs == nil || len(evs) != 0 {
		t.Fatalf("round 1 events: %+v", evs)
	}
	if len(g.State().Fleets) != 1 {
		t.Fatal("fleet should still be in flight")
	}
	evs := endRound(t, g)
	if len(evs) != 1 || evs[0].Planet != 3 {
		t.Fatalf("round 2 events: %+v", evs)
	}
	if len(g.State().Fleets) != 0 {
		t.Fatal("fleet should have landed")
	}
}

func TestProduction(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		s := DefaultSettings()
		s.CaptureProducesImmediately = immediate
		g := testGame(t, 2, s, twoPlayerMap())
		must(g.Launch(0, 0, 2, 3)) // C has 0 ships: captured with all 3
		evs := endRound(t, g)
		if len(evs) != 1 || evs[0].Kind != EvCaptured || evs[0].AttackerLost != 0 {
			t.Fatalf("events = %+v", evs)
		}
		st := g.State()
		wantC := 3
		if immediate {
			wantC += 7
		}
		if st.Planets[0].Ships != 7+10 || st.Planets[1].Ships != 20 || st.Planets[2].Ships != wantC || st.Planets[3].Ships != 6 {
			t.Fatalf("immediate=%v: ships = %d %d %d %d", immediate,
				st.Planets[0].Ships, st.Planets[1].Ships, st.Planets[2].Ships, st.Planets[3].Ships)
		}
		if st.Planets[2].Owner != 0 || st.Planets[2].CapturedRound != 1 {
			t.Fatalf("C = %+v", st.Planets[2])
		}
		wantBuilt := 10
		if immediate {
			wantBuilt += 7
		}
		if st.Players[0].Stats.ShipsBuilt != wantBuilt || st.Players[0].Stats.PlanetsConquered != 1 {
			t.Fatalf("stats = %+v", st.Players[0].Stats)
		}
		// Next round C produces either way.
		endRound(t, g)
		if got := g.State().Planets[2].Ships; got != wantC+7 {
			t.Fatalf("immediate=%v: C has %d after round 2, want %d", immediate, got, wantC+7)
		}
	}
}

func TestCombatOutcomesAndStats(t *testing.T) {
	// Sure shots make the outcome exact: 3 attackers vs 1 defender leaves 2.
	planets := twoPlayerMap()
	planets[0].Kill = 1
	planets[1].Kill = 1
	planets[1].Pos = Pos{1, 0, 0}
	planets[1].Ships = 1
	g := testGame(t, 2, DefaultSettings(), planets)
	must(g.Launch(0, 0, 1, 3))
	evs := endRound(t, g)
	// The capture, then B's elimination and game over.
	if len(evs) != 3 || evs[1].Kind != EvEliminated || evs[2].Kind != EvGameOver {
		t.Fatalf("events = %+v", evs)
	}
	e := evs[0]
	if e.Kind != EvCaptured || e.AttackerLost != 1 || e.DefenderLost != 1 || e.Remaining != 2 ||
		!reflect.DeepEqual(e.Seats, []int{0, 1}) {
		t.Fatalf("event = %+v", e)
	}
	st := g.State()
	a, b := st.Players[0].Stats, st.Players[1].Stats
	if a.ShipsLost != 1 || a.ShipsDestroyed != 1 || b.ShipsLost != 1 || b.ShipsDestroyed != 1 {
		t.Fatalf("stats = %+v / %+v", a, b)
	}
	// B lost its only planet and has no fleets: eliminated, and A wins.
	if !g.Over() || g.Winner() != 0 || st.Players[1].Alive || g.Active() != -1 {
		t.Fatalf("over=%v winner=%d alive=%v", g.Over(), g.Winner(), st.Players[1].Alive)
	}
	if _, err := g.EndTurn(0); !errors.Is(err, ErrGameOver) {
		t.Fatalf("end turn after game over: %v", err)
	}
}

func TestHeld(t *testing.T) {
	planets := twoPlayerMap()
	planets[0].Kill = 1
	planets[3].Kill = 1
	planets[3].Ships = 5
	g := testGame(t, 2, DefaultSettings(), planets)
	must(g.Launch(0, 0, 3, 3))
	endRound(t, g)
	evs := endRound(t, g)
	e := evs[0]
	if e.Kind != EvHeld || e.AttackerLost != 3 || e.DefenderLost != 2 || e.Remaining != 4 || e.Defender != Neutral ||
		!reflect.DeepEqual(e.Seats, []int{0}) {
		t.Fatalf("event = %+v", e)
	}
	// D grew to 6 in round 1, lost 2 in the battle, then grew by 1.
	if got := g.State().Planets[3].Ships; got != 5 {
		t.Fatalf("D has %d ships, want 5", got)
	}
}

func TestSeveralFleetsOnePlanet(t *testing.T) {
	// A and B both hit C in round 1. A launched first, so A captures the
	// empty planet, then B's fleet fights A's new garrison.
	planets := twoPlayerMap()
	planets[1].Pos = Pos{3, 0, 0} // 1 round from C too
	planets[0].Kill, planets[1].Kill, planets[2].Kill = 1, 1, 1
	g := testGame(t, 2, DefaultSettings(), planets)
	must(g.Launch(0, 0, 2, 2))
	must(g.EndTurn(0))
	must(g.Launch(1, 1, 2, 5))
	evs := must(g.EndTurn(1))
	if len(evs) != 2 {
		t.Fatalf("events = %+v", evs)
	}
	if evs[0].Kind != EvCaptured || evs[0].Attacker != 0 || evs[0].Defender != Neutral {
		t.Fatalf("first event = %+v", evs[0])
	}
	// B: 5 attackers vs 2 defenders, all sure shots (A defends with C's kill
	// percentage): B loses 2, A loses 2.
	if evs[1].Kind != EvCaptured || evs[1].Attacker != 1 || evs[1].Defender != 0 || evs[1].Remaining != 3 {
		t.Fatalf("second event = %+v", evs[1])
	}
	st := g.State()
	if st.Planets[2].Owner != 1 || st.Planets[2].Ships != 3 {
		t.Fatalf("C = %+v", st.Planets[2])
	}
	if st.Players[0].Stats.PlanetsConquered != 1 || st.Players[1].Stats.PlanetsConquered != 1 {
		t.Fatal("both should have conquered once")
	}
}

func TestSameRoundOrderIsLaunchOrderAcrossRounds(t *testing.T) {
	// A fleet launched earlier but traveling longer lands before a later
	// fleet due the same round.
	planets := twoPlayerMap()
	planets[0].Kill, planets[1].Kill = 1, 1
	planets[1].Pos = Pos{5, 0, 0} // D is 1 round from B, 2 from A
	g := testGame(t, 2, DefaultSettings(), planets)
	must(g.Launch(0, 0, 3, 8)) // arrives end of round 2
	endRound(t, g)
	must(g.EndTurn(0))
	must(g.Launch(1, 1, 3, 8)) // arrives end of round 2
	evs := must(g.EndTurn(1))
	if len(evs) != 2 || evs[0].Attacker != 0 || evs[1].Attacker != 1 {
		t.Fatalf("events = %+v", evs)
	}
}

func TestEliminationWaitsForFleets(t *testing.T) {
	// Seat 1 sends everything off its home, then loses the home. It survives
	// while its fleet is in flight, then captures and stays in.
	planets := []Planet{
		{Pos: Pos{0, 0, 0}, Owner: 0, Ships: 10, Production: 10, Kill: 1},
		{Pos: Pos{2, 0, 0}, Owner: 1, Ships: 3, Production: 1, Kill: 1},
		{Pos: Pos{20, 0, 0}, Owner: Neutral, Ships: 0, Production: 5, Kill: 0.5},
		{Pos: Pos{0, 9, 0}, Owner: 2, Ships: 1, Production: 1, Kill: 0.5},
	}
	s := DefaultSettings()
	s.Width = 32
	g := testGame(t, 3, s, planets)
	must(g.Launch(0, 0, 1, 10)) // 1 round: takes B
	must(g.EndTurn(0))
	must(g.Launch(1, 1, 2, 3)) // 9 rounds away
	must(g.EndTurn(1))
	must(g.EndTurn(2))
	st := g.State()
	if st.Planets[1].Owner != 0 || !st.Players[1].Alive {
		t.Fatalf("B owner %d, seat 1 alive %v", st.Planets[1].Owner, st.Players[1].Alive)
	}
	// Seat 1 still takes turns while its fleet flies.
	if g.Active() != 0 {
		t.Fatal("round 2 should start at seat 0")
	}
	must(g.EndTurn(0))
	if g.Active() != 1 {
		t.Fatalf("active = %d, want 1", g.Active())
	}
}

func TestEliminatedSeatsAreSkipped(t *testing.T) {
	planets := []Planet{
		{Pos: Pos{0, 0, 0}, Owner: 0, Ships: 10, Production: 10, Kill: 1},
		{Pos: Pos{1, 0, 0}, Owner: 1, Ships: 1, Production: 1, Kill: 1},
		{Pos: Pos{9, 9, 0}, Owner: 2, Ships: 1, Production: 1, Kill: 0.5},
	}
	g := testGame(t, 3, DefaultSettings(), planets)
	must(g.Launch(0, 0, 1, 5))
	must(g.EndTurn(0))
	must(g.EndTurn(1))
	evs := must(g.EndTurn(2))
	var elim []int
	for _, e := range evs {
		if e.Kind == EvEliminated {
			elim = append(elim, e.Attacker)
			if !e.VisibleTo(0) || !e.VisibleTo(2) {
				t.Fatal("elimination should be global")
			}
		}
	}
	if !reflect.DeepEqual(elim, []int{1}) || g.Over() {
		t.Fatalf("eliminated %v, over %v", elim, g.Over())
	}
	must(g.EndTurn(0))
	if g.Active() != 2 {
		t.Fatalf("active = %d, want 2 (seat 1 skipped)", g.Active())
	}
	if _, err := g.EndTurn(1); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("eliminated seat acted: %v", err)
	}
}

func TestMaxRounds(t *testing.T) {
	tests := []struct {
		name       string
		ships      [2]int
		planets1   int
		wantWinner int
	}{
		{"more planets wins", [2]int{1, 50}, 2, 1},
		{"tie on planets, more ships wins", [2]int{30, 10}, 1, 0},
		{"full tie, no winner", [2]int{10, 10}, 1, -1},
	}
	for _, tt := range tests {
		s := DefaultSettings()
		s.MaxRounds = 2
		planets := []Planet{
			{Pos: Pos{0, 0, 0}, Owner: 0, Ships: tt.ships[0], Production: 5, Kill: 0.5},
			{Pos: Pos{9, 9, 0}, Owner: 1, Ships: tt.ships[1], Production: 5, Kill: 0.5},
			{Pos: Pos{5, 0, 0}, Owner: Neutral, Ships: 1, Production: 5, Kill: 0.5},
		}
		if tt.planets1 == 2 {
			planets[2].Owner = 1
		}
		g := testGame(t, 2, s, planets)
		endRound(t, g)
		if g.Over() {
			t.Fatalf("%s: over after round 1", tt.name)
		}
		evs := endRound(t, g)
		if !g.Over() || g.Winner() != tt.wantWinner {
			t.Fatalf("%s: over=%v winner=%d", tt.name, g.Over(), g.Winner())
		}
		if last := evs[len(evs)-1]; last.Kind != EvGameOver || last.Attacker != tt.wantWinner || len(last.Seats) != 0 {
			t.Fatalf("%s: last event %+v", tt.name, last)
		}
		st := g.Standings()
		if tt.wantWinner == -1 && (st[0].Rank != 1 || st[1].Rank != 1) {
			t.Fatalf("%s: tied ranks = %+v", tt.name, st)
		}
	}
}

func TestStandingsCountFleets(t *testing.T) {
	g := testGame(t, 2, DefaultSettings(), twoPlayerMap())
	must(g.Launch(0, 0, 3, 6))
	st := g.Standings()
	// Both have 1 planet and 10 ships (seat 0: 4 home + 6 in flight); tie
	// keeps seat order.
	if st[0].Seat != 0 || st[0].Ships != 10 || st[0].Rank != 1 || st[1].Rank != 1 {
		t.Fatalf("standings = %+v", st)
	}
}

func TestViewVisibility(t *testing.T) {
	tests := []struct {
		name                   string
		blind, neutralStats    bool
		wantEnemy, wantNeutral bool
	}{
		{"default", false, true, true, true},
		{"neutral stats off", false, false, true, false},
		{"blind", true, true, false, false},
	}
	for _, tt := range tests {
		s := DefaultSettings()
		s.BlindMap, s.NeutralStats = tt.blind, tt.neutralStats
		g := testGame(t, 2, s, twoPlayerMap())
		must(g.Launch(0, 0, 3, 2))
		must(g.EndTurn(0))
		must(g.Launch(1, 1, 3, 2))

		v := g.View(0)
		if v.Planets[0].Ships == nil || *v.Planets[0].Ships != 8 || v.Planets[0].Kill == nil {
			t.Fatalf("%s: own planet hidden: %+v", tt.name, v.Planets[0])
		}
		enemy, neutral := v.Planets[1], v.Planets[3]
		if (enemy.Ships != nil) != tt.wantEnemy || (enemy.Production != nil) != tt.wantEnemy || (enemy.Kill != nil) != tt.wantEnemy {
			t.Errorf("%s: enemy visibility wrong: %+v", tt.name, enemy)
		}
		if (neutral.Ships != nil) != tt.wantNeutral || (neutral.Production != nil) != tt.wantNeutral {
			t.Errorf("%s: neutral visibility wrong: %+v", tt.name, neutral)
		}
		if enemy.Owner != 1 || enemy.Name != "B" || neutral.Owner != Neutral {
			t.Errorf("%s: owner or name hidden", tt.name)
		}
		if len(v.Fleets) != 1 || v.Fleets[0].Owner != 0 {
			t.Errorf("%s: seat 0 sees fleets %+v", tt.name, v.Fleets)
		}
		if v1 := g.View(1); len(v1.Fleets) != 1 || v1.Fleets[0].Owner != 1 {
			t.Errorf("%s: seat 1 sees fleets %+v", tt.name, v1.Fleets)
		}
	}
}

func TestViewDoesNotAlias(t *testing.T) {
	g := testGame(t, 2, DefaultSettings(), twoPlayerMap())
	v := g.View(0)
	*v.Planets[0].Ships = 999
	if g.State().Planets[0].Ships != 10 {
		t.Fatal("view aliases game state")
	}
	st := g.State()
	st.Planets[0].Ships = 999
	if g.State().Planets[0].Ships != 10 {
		t.Fatal("State aliases game state")
	}
}

func TestFilterEvents(t *testing.T) {
	evs := []Event{
		{Kind: EvHeld, Seats: []int{0}},
		{Kind: EvCaptured, Seats: []int{1, 2}},
		{Kind: EvEliminated},
	}
	if got := FilterEvents(evs, 0); len(got) != 2 || got[0].Kind != EvHeld || got[1].Kind != EvEliminated {
		t.Fatalf("seat 0: %+v", got)
	}
	if got := FilterEvents(evs, 2); len(got) != 2 || got[0].Kind != EvCaptured {
		t.Fatalf("seat 2: %+v", got)
	}
}

// playRandom plays legal random actions, driven by its own RNG so the game's
// RNG stays untouched.
func playRandom(t *testing.T, g *Game, seed uint64, maxActions int) {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, 3))
	for range maxActions {
		if g.Over() {
			return
		}
		seat := g.Active()
		st := g.State()
		var own []Planet
		for _, p := range st.Planets {
			if p.Owner == seat && p.Ships > 0 {
				own = append(own, p)
			}
		}
		if len(own) == 0 || r.IntN(3) == 0 {
			must(g.EndTurn(seat))
			continue
		}
		src := own[r.IntN(len(own))]
		to := r.IntN(len(st.Planets))
		if to == src.ID {
			continue
		}
		must(g.Launch(seat, src.ID, to, 1+r.IntN(src.Ships)))
	}
}

func TestReplay(t *testing.T) {
	s := DefaultSettings()
	s.Depth = 3
	s.MaxRounds = 60
	for seed := range uint64(10) {
		g := must(New(4, s, seed))
		playRandom(t, g, seed, 3000)
		r := must(Replay(4, s, seed, g.Actions()))
		if !reflect.DeepEqual(g.State(), r.State()) {
			t.Fatalf("seed %d: replay differs", seed)
		}
		again := must(New(4, s, seed))
		playRandom(t, again, seed, 3000)
		if !reflect.DeepEqual(g.State(), again.State()) {
			t.Fatalf("seed %d: same seed and actions differ", seed)
		}
	}
}

func TestReplayRejectsBadLog(t *testing.T) {
	_, err := Replay(2, DefaultSettings(), 1, []Action{{Kind: ActEndTurn, Seat: 1}})
	if !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("got %v", err)
	}
	_, err = Replay(2, DefaultSettings(), 1, []Action{{Kind: "warp", Seat: 0}})
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("got %v", err)
	}
}

func TestInvariants(t *testing.T) {
	// Ships are never negative, and a fleet's arrival round is never before
	// its launch round.
	s := DefaultSettings()
	s.MaxRounds = 80
	for seed := range uint64(20) {
		g := must(New(3, s, seed))
		playRandom(t, g, seed+100, 4000)
		st := g.State()
		for _, p := range st.Planets {
			if p.Ships < 0 {
				t.Fatalf("seed %d: negative ships %+v", seed, p)
			}
		}
		for _, f := range st.Fleets {
			if f.Ships < 1 || f.ArrivalRound < f.LaunchRound || f.ArrivalRound < st.Round {
				t.Fatalf("seed %d: bad fleet %+v at round %d", seed, f, st.Round)
			}
		}
	}
}

func TestSectors(t *testing.T) {
	cube := DefaultSettings()
	cube.Width, cube.Height, cube.Depth, cube.Neutrals = 32, 32, 32, 16
	for _, s := range []Settings{DefaultSettings(), cube} {
		planets := 4 + s.Neutrals
		secs := Sectors(s, planets)
		if float64(len(secs)) < float64(planets)*sectorRoom {
			t.Fatalf("%dx%dx%d: only %d sectors for %d planets", s.Width, s.Height, s.Depth, len(secs), planets)
		}
		// The sectors tile the map exactly.
		cells := 0
		for _, sec := range secs {
			cells += (sec.Max.X - sec.Min.X) * (sec.Max.Y - sec.Min.Y) * (sec.Max.Z - sec.Min.Z)
		}
		if cells != s.Width*s.Height*s.Depth {
			t.Fatalf("sectors cover %d cells, map has %d", cells, s.Width*s.Height*s.Depth)
		}
		in := func(p Pos) int {
			for i, sec := range secs {
				if p.X >= sec.Min.X && p.X < sec.Max.X && p.Y >= sec.Min.Y && p.Y < sec.Max.Y && p.Z >= sec.Min.Z && p.Z < sec.Max.Z {
					return i
				}
			}
			return -1
		}
		for seed := range uint64(20) {
			g := must(New(4, s, seed))
			seen := map[int]bool{}
			for _, p := range g.State().Planets {
				i := in(p.Pos)
				if i < 0 || seen[i] {
					t.Fatalf("seed %d: planet %s at %v shares a sector", seed, p.Name, p.Pos)
				}
				seen[i] = true
			}
		}
	}
}

func TestCubeFillsTheVolume(t *testing.T) {
	// On a 32-cube, planets reach every part of the volume: each half of
	// each axis holds some.
	s := DefaultSettings()
	s.Width, s.Height, s.Depth, s.Neutrals = 32, 32, 32, 16
	for seed := range uint64(10) {
		var low, high [3]int
		for _, p := range must(New(4, s, seed)).State().Planets {
			for i, c := range [3]int{p.Pos.X, p.Pos.Y, p.Pos.Z} {
				if c < 16 {
					low[i]++
				} else {
					high[i]++
				}
			}
		}
		for i := range 3 {
			if low[i] < 3 || high[i] < 3 {
				t.Fatalf("seed %d axis %d: %d low, %d high", seed, i, low[i], high[i])
			}
		}
	}
}
