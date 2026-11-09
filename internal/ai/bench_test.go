package ai

import (
	"testing"

	"parallax/internal/rules"
)

// A Planner turn from a mid-game position, to keep decisions fast enough
// for a room's AI delay.
func BenchmarkPlannerTurn(b *testing.B) {
	g, _ := rules.New(4, rules.DefaultSettings(), 7)
	for g.Round() < 60 && !g.Over() {
		TakeTurn(g, g.Active(), Greedy{})
	}
	v := g.View(g.Active())
	b.ResetTimer()
	for range b.N {
		Planner{}.Orders(v)
	}
}
