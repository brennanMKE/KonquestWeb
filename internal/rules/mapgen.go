package rules

import (
	"math"
	"math/rand/v2"
)

// Planet stats from RULES.md.
const (
	HomeProduction    = 10
	HomeKill          = 0.40
	NeutralMinProd    = 5
	NeutralMaxProd    = 15
	NeutralMinKill    = 0.30
	NeutralMaxKill    = 0.90
	NeutralGrowth     = 1
	homeCandidates    = 64
	neutralPlacements = 100
)

// generateMap places one home per player and the neutrals, then shuffles
// and names them. Everything comes from rng, so a seed gives one map.
func generateMap(rng *rand.Rand, players int, s Settings) []Planet {
	used := map[Pos]bool{}
	var planets []Planet
	randPos := func() Pos {
		return Pos{rng.IntN(s.Width), rng.IntN(s.Height), rng.IntN(s.Depth)}
	}
	// anyFree scans from a random cell so placement always succeeds when the
	// random tries miss.
	anyFree := func() Pos {
		cells := s.Width * s.Height * s.Depth
		start := rng.IntN(cells)
		for i := range cells {
			c := (start + i) % cells
			p := Pos{c % s.Width, c / s.Width % s.Height, c / (s.Width * s.Height)}
			if !used[p] {
				return p
			}
		}
		panic("rules: no free cell; Settings.Validate should prevent this")
	}
	place := func(p Planet) {
		used[p.Pos] = true
		planets = append(planets, p)
	}

	var homes []Pos
	for seat := range players {
		best, bestDist := Pos{}, -1.0
		for range homeCandidates {
			p := randPos()
			if used[p] {
				continue
			}
			d := math.Inf(1)
			for _, h := range homes {
				d = min(d, Distance(p, h))
			}
			if d > bestDist {
				best, bestDist = p, d
			}
		}
		if bestDist < 0 {
			best = anyFree()
		}
		homes = append(homes, best)
		place(Planet{Pos: best, Owner: seat, Ships: HomeProduction,
			Production: HomeProduction, Kill: HomeKill, Home: true})
	}

	for range s.Neutrals {
		p, ok := Pos{}, false
		for range neutralPlacements {
			if p = randPos(); !used[p] && !adjacent(p, used) {
				ok = true
				break
			}
		}
		if !ok {
			p = anyFree()
		}
		prod := NeutralMinProd + rng.IntN(NeutralMaxProd-NeutralMinProd+1)
		kill := math.Round((NeutralMinKill+rng.Float64()*(NeutralMaxKill-NeutralMinKill))*100) / 100
		place(Planet{Pos: p, Owner: Neutral, Ships: prod, Production: prod, Kill: kill})
	}

	rng.Shuffle(len(planets), func(i, j int) { planets[i], planets[j] = planets[j], planets[i] })
	for i := range planets {
		planets[i].ID = i
		planets[i].Name = PlanetName(i)
	}
	return planets
}

// adjacent reports whether any used cell touches p, including diagonally.
func adjacent(p Pos, used map[Pos]bool) bool {
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for dz := -1; dz <= 1; dz++ {
				if used[Pos{p.X + dx, p.Y + dy, p.Z + dz}] {
					return true
				}
			}
		}
	}
	return false
}

// PlanetName returns the name for planet index i: A..Z, then AA, AB, ...
func PlanetName(i int) string {
	name := ""
	for i++; i > 0; i /= 26 {
		i--
		name = string(rune('A'+i%26)) + name
	}
	return name
}
