package ai

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"parallax/internal/rules"
)

// Danger is the strongest strike one enemy could aim at one of our planets
// soon, from the enemy planets in reach. Enemy fleets are invisible, so this
// is what the enemy could do, not what it is doing.
type Danger struct {
	Key    string `json:"key"`
	Planet int    `json:"planet"`
	Enemy  int    `json:"enemy"`
	// Strike is the enemy ships in reach, with their ship-weighted kill
	// percentage.
	Strike int     `json:"strike"`
	Kill   float64 `json:"kill"`
	// Soonest and Latest are the shortest and longest trips among the enemy
	// planets in reach.
	Soonest int `json:"soonest"`
	Latest  int `json:"latest"`
	// Defenders is our garrison when the last of the strike could arrive,
	// and DefKill its kill percentage.
	Defenders int     `json:"defenders"`
	DefKill   float64 `json:"defKill"`
	// Win is the chance the strike captures the planet.
	Win float64 `json:"win"`
	// Seen is false when some of the enemy's ship counts are hidden.
	Seen  bool   `json:"seen"`
	State string `json:"state"`
}

// Tuning for defense.
const (
	dangerHorizon = 4   // rounds of travel an enemy strike may take
	dangerWin     = 0.5 // a strike this likely to win puts a planet in danger
	safeWin       = 0.3 // reinforce until the strike is at most this likely
	maxDangers    = 8   // danger questions per decision
)

// dangerQuestion's options repeat the words the danger states lead with.
var dangerQuestion = Question{
	Type:         "choice",
	Instructions: "Is this planet in danger or safe?",
	Criteria:     map[string]string{"danger": "in danger", "safe": "safe"},
}

// Dangers assesses each of our planets that an enemy could reach within
// dangerHorizon rounds. Planets no enemy can reach are safe and not asked
// about.
func Dangers(v rules.View) []Danger {
	var out []Danger
	for _, p := range v.Planets {
		if p.Owner != v.Seat {
			continue
		}
		if d, ok := dangerTo(v, p); ok {
			out = append(out, d)
		}
	}
	slices.SortStableFunc(out, func(a, b Danger) int { return cmp.Compare(b.Win, a.Win) })
	out = out[:min(maxDangers, len(out))]
	for i := range out {
		out[i].Key = fmt.Sprintf("d%d", i)
	}
	return out
}

// dangerTo finds the most dangerous enemy strike on p, if any enemy is in
// reach.
func dangerTo(v rules.View, p rules.PlanetView) (Danger, bool) {
	type strike struct {
		ships           int
		killSum         float64
		soonest, latest int
		seen            bool
	}
	byEnemy := map[int]*strike{}
	var enemies []int
	for _, e := range v.Planets {
		if e.Owner == v.Seat || e.Owner == rules.Neutral {
			continue
		}
		t := rules.TravelTurns(e.Pos, p.Pos, v.Settings.ShipSpeed)
		if t > dangerHorizon {
			continue
		}
		ships, kill := unknownShips, unknownKill
		if e.Ships != nil {
			ships = *e.Ships
		}
		if e.Kill != nil {
			kill = *e.Kill
		}
		if ships == 0 {
			continue
		}
		s := byEnemy[e.Owner]
		if s == nil {
			s = &strike{soonest: t, seen: true}
			byEnemy[e.Owner] = s
			enemies = append(enemies, e.Owner)
		}
		s.ships += ships
		s.killSum += kill * float64(ships)
		s.soonest, s.latest = min(s.soonest, t), max(s.latest, t)
		s.seen = s.seen && e.Ships != nil
	}
	best, found := Danger{}, false
	slices.Sort(enemies)
	for _, o := range enemies {
		s := byEnemy[o]
		d := Danger{
			Planet: p.ID, Enemy: o, Strike: s.ships, Kill: s.killSum / float64(s.ships),
			Soonest: s.soonest, Latest: s.latest, DefKill: *p.Kill, Seen: s.seen,
			Defenders: *p.Ships + *p.Production*(s.latest-1),
		}
		d.Win = winChance(d.Strike, d.Kill, d.Defenders, d.DefKill)
		if !found || d.Win > best.Win {
			best, found = d, true
		}
	}
	if found {
		best.State = describeDanger(p, best)
	}
	return best, found
}

// describeDanger leads with the verdict, in the same words the question's
// options use.
func describeDanger(p rules.PlanetView, d Danger) string {
	var s string
	switch {
	case d.Win >= 0.9:
		s = "Planet %s is in danger: enemy ships nearby would almost certainly capture it."
	case d.Win >= dangerWin:
		s = "Planet %s is in danger: enemy ships nearby would probably capture it."
	case d.Win >= 0.15:
		s = "Planet %s is safe: enemy ships nearby would probably fail to capture it."
	default:
		s = "Planet %s is safe: enemy ships nearby are too weak to capture it."
	}
	s = fmt.Sprintf(s, p.Name)
	if !d.Seen {
		s += " We can't see all of their ships."
	}
	return s
}

func dangerGuess(d Danger) string {
	if d.Win >= dangerWin {
		return "danger"
	}
	return "safe"
}

// reinforcements is how many more defenders p needs for the strike to win
// at most safeWin of the time, or -1 if no reasonable number does.
func reinforcements(d Danger) int {
	f := scale(d.Strike)
	att := scaled(d.Strike, f)
	from := scaled(d.Defenders, f)
	maxDef := from + 3*att + 20
	tab := rules.NewWinTable(att, d.Kill, maxDef, d.DefKill)
	for def := from; def <= maxDef; def++ {
		if tab.Win(att, def) <= safeWin {
			return max(0, int(math.Ceil(float64(def)/f))-d.Defenders)
		}
	}
	return -1
}

// fullView is seat's view with every planet's stats shown: what the oracle
// knows.
func fullView(st rules.State, seat int) rules.View {
	v := rules.View{Seat: seat, Round: st.Round, Active: st.Active, Settings: st.Settings}
	for _, p := range st.Planets {
		ships, prod, kill := p.Ships, p.Production, p.Kill
		v.Planets = append(v.Planets, rules.PlanetView{ID: p.ID, Name: p.Name, Pos: p.Pos, Owner: p.Owner,
			Ships: &ships, Production: &prod, Kill: &kill})
	}
	return v
}
