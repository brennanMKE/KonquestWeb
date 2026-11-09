package rules

import (
	"math"
	"math/rand/v2"
)

// Planet stats from RULES.md.
const (
	HomeProduction = 10
	HomeKill       = 0.40
	NeutralMinProd = 5
	NeutralMaxProd = 15
	NeutralMinKill = 0.30
	NeutralMaxKill = 0.90
	NeutralGrowth  = 1
	homeCandidates = 64
)

// Sector is a box of cells, [Min, Max) on each axis. Every planet gets a
// sector of its own, so planets spread through the whole map, in 3D as well
// as flat.
type Sector struct {
	Min Pos `json:"min"`
	Max Pos `json:"max"`
}

func (s Sector) center() (float64, float64, float64) {
	return float64(s.Min.X+s.Max.X) / 2, float64(s.Min.Y+s.Max.Y) / 2, float64(s.Min.Z+s.Max.Z) / 2
}

// Sectors divides the map into near-cubic boxes, at least sectorRoom times
// as many as there are planets, so placement has room to spread out.
func Sectors(s Settings, planets int) []Sector {
	want := int(math.Ceil(float64(planets) * sectorRoom))
	dims := [3]int{s.Width, s.Height, s.Depth}
	var k [3]int
	for edge := max(s.Width, s.Height, s.Depth); edge >= 1; edge-- {
		n := 1
		for i, d := range dims {
			k[i] = (d + edge - 1) / edge
			n *= k[i]
		}
		if n >= want {
			break
		}
	}
	// Boundaries along one axis: d cells split into n near-equal parts.
	cut := func(d, n, i int) int { return i * d / n }
	var out []Sector
	for z := range k[2] {
		for y := range k[1] {
			for x := range k[0] {
				out = append(out, Sector{
					Min: Pos{cut(dims[0], k[0], x), cut(dims[1], k[1], y), cut(dims[2], k[2], z)},
					Max: Pos{cut(dims[0], k[0], x+1), cut(dims[1], k[1], y+1), cut(dims[2], k[2], z+1)},
				})
			}
		}
	}
	return out
}

// sectorRoom is how many sectors there are per planet.
const sectorRoom = 1.5

// generateMap places one home per player and the neutrals, each in a sector
// of its own, then shuffles and names them. Homes take the sectors farthest
// from the homes already placed. Everything comes from rng, so a seed gives
// one map.
func generateMap(rng *rand.Rand, players int, s Settings) []Planet {
	sectors := Sectors(s, players+s.Neutrals)
	free := make([]int, len(sectors))
	for i := range free {
		free[i] = i
	}
	take := func(j int) Sector { // removes free[j]
		sec := sectors[free[j]]
		free[j] = free[len(free)-1]
		free = free[:len(free)-1]
		return sec
	}
	// cellIn picks a random cell in a sector, keeping off its edges when the
	// sector is big enough, so planets in neighbouring sectors aren't
	// adjacent.
	cellIn := func(sec Sector) Pos {
		axis := func(lo, hi int) int {
			if hi-lo >= 3 {
				lo, hi = lo+1, hi-1
			}
			return lo + rng.IntN(hi-lo)
		}
		return Pos{axis(sec.Min.X, sec.Max.X), axis(sec.Min.Y, sec.Max.Y), axis(sec.Min.Z, sec.Max.Z)}
	}

	var planets []Planet
	var homes [][3]float64
	for seat := range players {
		best, bestDist := 0, -1.0
		for range homeCandidates {
			j := rng.IntN(len(free))
			cx, cy, cz := sectors[free[j]].center()
			d := math.Inf(1)
			for _, h := range homes {
				d = min(d, math.Hypot(math.Hypot(cx-h[0], cy-h[1]), cz-h[2]))
			}
			if d > bestDist {
				best, bestDist = j, d
			}
		}
		sec := take(best)
		cx, cy, cz := sec.center()
		homes = append(homes, [3]float64{cx, cy, cz})
		planets = append(planets, Planet{Pos: cellIn(sec), Owner: seat, Ships: HomeProduction,
			Production: HomeProduction, Kill: HomeKill, Home: true})
	}

	for range s.Neutrals {
		sec := take(rng.IntN(len(free)))
		prod := NeutralMinProd + rng.IntN(NeutralMaxProd-NeutralMinProd+1)
		kill := math.Round((NeutralMinKill+rng.Float64()*(NeutralMaxKill-NeutralMinKill))*100) / 100
		planets = append(planets, Planet{Pos: cellIn(sec), Owner: Neutral, Ships: prod, Production: prod, Kill: kill})
	}

	rng.Shuffle(len(planets), func(i, j int) { planets[i], planets[j] = planets[j], planets[i] })
	for i := range planets {
		planets[i].ID = i
		planets[i].Name = PlanetName(i)
	}
	return planets
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
