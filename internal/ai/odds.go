package ai

import (
	"math"

	"parallax/internal/rules"
)

// oddsCap bounds the exact win table. Bigger battles are scaled down to it
// with both sides in proportion: with hundreds of ships a side, the odds
// depend on the ratio of the forces, so this stays accurate while the cost
// stays at most oddsCap² per table.
const oddsCap = 400

// scale is the factor that brings the largest count down to oddsCap.
func scale(counts ...int) float64 {
	m := 0
	for _, c := range counts {
		m = max(m, c)
	}
	if m <= oddsCap {
		return 1
	}
	return oddsCap / float64(m)
}

func scaled(n int, f float64) int { return int(math.Round(float64(n) * f)) }

// winChance is P(attacker wins), exact up to oddsCap ships a side.
func winChance(att int, attKill float64, def int, defKill float64) float64 {
	f := scale(att, def)
	return rules.WinProbability(scaled(att, f), attKill, scaled(def, f), defKill)
}

// shipsToWin is the smallest fleet up to available that wins against def
// with probability at least p, or available if none does.
func shipsToWin(available int, attKill float64, def int, defKill, p float64) int {
	f := scale(available, def)
	n := rules.NewWinTable(scaled(available, f), attKill, scaled(def, f), defKill).ShipsFor(scaled(def, f), p)
	if n < 0 {
		return available
	}
	return min(available, int(math.Ceil(float64(n)/f)))
}
