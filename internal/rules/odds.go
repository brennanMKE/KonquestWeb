package rules

// WinTable holds exact attacker win probabilities for one pair of kill
// percentages, following the exchange rule in RULES.md: the defenders fire
// first, then the attackers if any are left.
type WinTable struct {
	maxAtt, maxDef int
	p              []float64 // p[a*(maxDef+1)+d]
}

// NewWinTable computes P(attacker wins) for every attacker count up to
// maxAtt and defender count up to maxDef. It costs O(maxAtt × maxDef).
// Kill percentages must be in (0, 1].
func NewWinTable(maxAtt int, attKill float64, maxDef int, defKill float64) *WinTable {
	maxAtt, maxDef = max(maxAtt, 0), max(maxDef, 0)
	t := &WinTable{maxAtt: maxAtt, maxDef: maxDef, p: make([]float64, (maxAtt+1)*(maxDef+1))}
	at := func(a, d int) float64 { return t.p[a*(maxDef+1)+d] }
	// An exchange where nobody hits leaves the state unchanged, so divide
	// out that self-loop.
	stay := (1 - defKill) * (1 - attKill)
	for a := 1; a <= maxAtt; a++ {
		t.p[a*(maxDef+1)] = 1 // no defenders: the planet falls
		for d := 1; d <= maxDef; d++ {
			// Defenders hit: one attacker lost; any survivors fire back.
			hit := 0.0
			if a > 1 {
				hit = attKill*at(a-1, d-1) + (1-attKill)*at(a-1, d)
			}
			miss := attKill * at(a, d-1)
			t.p[a*(maxDef+1)+d] = (defKill*hit + (1-defKill)*miss) / (1 - stay)
		}
	}
	return t
}

// Win is P(attacker wins) for att attackers against def defenders, clamped
// to the table's size.
func (t *WinTable) Win(att, def int) float64 {
	att, def = min(max(att, 0), t.maxAtt), min(max(def, 0), t.maxDef)
	return t.p[att*(t.maxDef+1)+def]
}

// ShipsFor is the smallest attacker count up to the table's size that wins
// with at least probability p against def defenders, or -1 if none does.
func (t *WinTable) ShipsFor(def int, p float64) int {
	for a := 1; a <= t.maxAtt; a++ {
		if t.Win(a, def) >= p {
			return a
		}
	}
	return -1
}

// WinProbability is P(attacker wins) for one battle.
func WinProbability(att int, attKill float64, def int, defKill float64) float64 {
	return NewWinTable(att, attKill, def, defKill).Win(att, def)
}
