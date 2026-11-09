package rules

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestWinProbabilityExact(t *testing.T) {
	tests := []struct {
		att      int
		ak       float64
		def      int
		dk, want float64
	}{
		{5, 0.5, 0, 0.5, 1},
		{0, 0.5, 3, 0.5, 0},
		{3, 1, 1, 1, 1},           // sure shots, attacker wins
		{3, 1, 3, 1, 0},           // sure shots, tie goes to the defender
		{1, 0.5, 1, 0.5, 1.0 / 3}, // per exchange: att hit 1/2; else att scores 1/2 → (1/4)/(3/4)
	}
	for _, tt := range tests {
		if got := WinProbability(tt.att, tt.ak, tt.def, tt.dk); math.Abs(got-tt.want) > 1e-12 {
			t.Errorf("WinProbability(%d,%v,%d,%v) = %v, want %v", tt.att, tt.ak, tt.def, tt.dk, got, tt.want)
		}
	}
}

func TestWinProbabilityMatchesBattle(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 5))
	cases := []struct {
		att int
		ak  float64
		def int
		dk  float64
	}{
		{10, 0.4, 10, 0.4},
		{20, 0.4, 12, 0.8},
		{15, 0.9, 20, 0.3},
		{7, 0.55, 4, 0.72},
	}
	const n = 20000
	for _, c := range cases {
		wins := 0
		for range n {
			if _, d := battle(rng, c.att, c.ak, c.def, c.dk); d == 0 {
				wins++
			}
		}
		sim := float64(wins) / n
		exact := WinProbability(c.att, c.ak, c.def, c.dk)
		if math.Abs(sim-exact) > 0.015 {
			t.Errorf("%+v: simulated %.3f, exact %.3f", c, sim, exact)
		}
	}
}

func TestWinTable(t *testing.T) {
	tab := NewWinTable(60, 0.4, 20, 0.6)
	prev := 0.0
	for a := 0; a <= 60; a++ {
		p := tab.Win(a, 15)
		if p < prev-1e-12 || p < 0 || p > 1 {
			t.Fatalf("Win(%d, 15) = %v after %v: must rise within [0,1]", a, p, prev)
		}
		prev = p
	}
	n := tab.ShipsFor(15, 0.8)
	if n < 1 || tab.Win(n, 15) < 0.8 || tab.Win(n-1, 15) >= 0.8 {
		t.Fatalf("ShipsFor(15, 0.8) = %d", n)
	}
	if tab.ShipsFor(20, 0.9999999) != -1 {
		t.Fatal("want -1 when the table is too small")
	}
	if tab.Win(1000, 1000) != tab.Win(60, 20) {
		t.Fatal("Win should clamp to the table")
	}
}
