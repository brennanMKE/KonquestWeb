package ai

import (
	"testing"

	"parallax/internal/rules"
)

// play runs a game where every seat is p, returning the game.
func play(t *testing.T, players int, s rules.Settings, seed uint64, p Player) *rules.Game {
	t.Helper()
	g, err := rules.New(players, s, seed)
	if err != nil {
		t.Fatal(err)
	}
	for !g.Over() {
		if _, err := TakeTurn(g, g.Active(), p); err != nil {
			t.Fatalf("seed %d round %d: %v", seed, g.Round(), err)
		}
	}
	return g
}

func TestGreedyGamesFinish(t *testing.T) {
	for _, depth := range []int{1, 4} {
		for _, blind := range []bool{false, true} {
			s := rules.DefaultSettings()
			s.Depth, s.BlindMap, s.MaxRounds = depth, blind, 300
			for seed := range uint64(10) {
				g := play(t, 4, s, seed, Greedy{})
				conquered := 0
				for _, st := range g.Standings() {
					conquered += st.Stats.PlanetsConquered
				}
				if conquered < s.Neutrals/2 {
					t.Errorf("depth %d blind %v seed %d: only %d captures", depth, blind, seed, conquered)
				}
			}
		}
	}
}

func TestGreedyUsesOnlyView(t *testing.T) {
	// The same view must give the same orders, whatever else is in the game.
	g, err := rules.New(3, rules.DefaultSettings(), 5)
	if err != nil {
		t.Fatal(err)
	}
	v := g.View(0)
	a, b := Greedy{}.Orders(v), Greedy{}.Orders(v)
	if len(a) != len(b) {
		t.Fatal("orders are not deterministic")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("orders are not deterministic")
		}
	}
}

func TestGreedyTakesEasyNeutral(t *testing.T) {
	g, err := rules.New(2, rules.DefaultSettings(), 1)
	if err != nil {
		t.Fatal(err)
	}
	// On a fresh map with 10 home ships, Greedy should find something it can
	// take within a few rounds.
	for range 5 {
		if _, err := TakeTurn(g, g.Active(), Greedy{}); err != nil {
			t.Fatal(err)
		}
		if _, err := TakeTurn(g, g.Active(), Greedy{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(g.State().Fleets) == 0 && g.Standings()[0].Stats.FleetsLaunched == 0 {
		t.Fatal("Greedy never launched a fleet")
	}
}
