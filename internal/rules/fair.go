package rules

import (
	"math"
	"math/rand/v2"
)

// Fair maps. Homes go as far apart as the map allows, but the neutrals land
// at random, so one home can start beside three rich planets while another
// waits five rounds for its first. With FairMap on, the game draws up to
// fairCandidates maps from the seed, in order, and keeps the first one fair
// enough, or else the fairest. A seed still gives one map.
const (
	fairCandidates = 64
	fairEnough     = 0.12
)

// Reach is what a home has around it: how soon it can first capture and
// how much nearby production it can grow into.
type Reach struct {
	Home int `json:"home"` // planet ID
	// First is the travel time to the nearest neutral, in rounds.
	First int `json:"first"`
	// Value sums each neutral's production, discounted by how hard it is to
	// take and by the square of its travel time.
	Value float64 `json:"value"`
}

// Reaches measures every home on a map.
func Reaches(planets []Planet, speed float64) []Reach {
	var out []Reach
	for _, h := range planets {
		if !h.Home {
			continue
		}
		r := Reach{Home: h.ID, First: math.MaxInt}
		for _, p := range planets {
			if p.Owner != Neutral {
				continue
			}
			t := TravelTurns(h.Pos, p.Pos, speed)
			r.First = min(r.First, t)
			r.Value += float64(p.Production) * (1.3 - p.Kill) / float64(t*t)
		}
		if r.First == math.MaxInt {
			r.First = 0
		}
		out = append(out, r)
	}
	return out
}

// Unfairness is 0 when every home has the same reach. It adds the spread in
// value, as a share of the best, to a quarter for each round between the
// earliest and latest first captures.
func Unfairness(planets []Planet, speed float64) float64 {
	rs := Reaches(planets, speed)
	if len(rs) < 2 {
		return 0
	}
	loV, hiV := math.Inf(1), 0.0
	loF, hiF := math.MaxInt, 0
	for _, r := range rs {
		loV, hiV = math.Min(loV, r.Value), math.Max(hiV, r.Value)
		loF, hiF = min(loF, r.First), max(hiF, r.First)
	}
	spread := 0.0
	if hiV > 0 {
		spread = (hiV - loV) / hiV
	}
	return spread + 0.25*float64(hiF-loF)
}

func fairMap(rng *rand.Rand, players int, s Settings) []Planet {
	var best []Planet
	bestScore := math.Inf(1)
	for range fairCandidates {
		m := generateMap(rng, players, s)
		score := Unfairness(m, s.ShipSpeed)
		if score < bestScore {
			best, bestScore = m, score
		}
		if score <= fairEnough {
			break
		}
	}
	return best
}
