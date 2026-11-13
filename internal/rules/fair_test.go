package rules

import "testing"

// The map from a game where one home's nearest neutral was 5 rounds away
// and the other's 2. A fair map from the same seed evens that out.
func TestFairMapEvensTheStart(t *testing.T) {
	s := Settings{Width: 32, Height: 32, Depth: 32, Neutrals: 16, ShipSpeed: 4, NeutralStats: true}
	const seed = 6351578944967778473
	unfair, err := New(2, s, seed)
	if err != nil {
		t.Fatal(err)
	}
	before := Unfairness(unfair.State().Planets, s.ShipSpeed)
	s.FairMap = true
	fair, err := New(2, s, seed)
	if err != nil {
		t.Fatal(err)
	}
	after := Unfairness(fair.State().Planets, s.ShipSpeed)
	if after >= before || after > 0.3 {
		t.Fatalf("unfairness %.2f before, %.2f after", before, after)
	}
	rs := Reaches(fair.State().Planets, s.ShipSpeed)
	if d := rs[0].First - rs[1].First; d < -1 || d > 1 {
		t.Fatalf("first captures %d and %d rounds away", rs[0].First, rs[1].First)
	}
	again, _ := New(2, s, seed)
	if Unfairness(again.State().Planets, s.ShipSpeed) != after {
		t.Fatal("the same seed gave a different fair map")
	}
}

// Across many seeds and player counts, fair maps are far more even than
// the maps they replace.
func TestFairMapsAcrossSeeds(t *testing.T) {
	for _, players := range []int{2, 3, 4, 6} {
		s := DefaultSettings()
		s.Width, s.Height, s.Depth, s.Neutrals, s.ShipSpeed = 32, 32, 32, 16, 4
		var plain, fair float64
		const n = 40
		for seed := range uint64(n) {
			s.FairMap = false
			g, _ := New(players, s, seed)
			plain += Unfairness(g.State().Planets, s.ShipSpeed)
			s.FairMap = true
			g, _ = New(players, s, seed)
			fair += Unfairness(g.State().Planets, s.ShipSpeed)
		}
		t.Logf("%d players: mean unfairness %.2f plain, %.2f fair", players, plain/n, fair/n)
		if fair > plain/2 {
			t.Errorf("%d players: fair maps %.2f, plain %.2f", players, fair/n, plain/n)
		}
	}
}
