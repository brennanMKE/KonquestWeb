package ai

import (
	"cmp"
	"math"
	"slices"

	"parallax/internal/rules"
)

// The planner is the strategy every smart computer player shares. Each turn
// it assesses the situation as questions with typed answers:
//
//   - for each of our planets an enemy could reach: is it in danger or safe?
//   - for each promising attack: is it a good bet or a bad bet?
//
// Then Decide turns the answers into orders, in priority order: defend
// planets in danger (hold their ships and send reinforcements that arrive in
// time), make the good-bet attacks, and move leftover ships from safe rear
// planets to the front. Planner answers from its own estimates, System1 asks
// a model, and Oracle answers from the true state.

// Situation is one turn's questions, with the numbers behind them.
type Situation struct {
	View    rules.View
	Attacks []Candidate
	Dangers []Danger
}

// Assess builds the turn's questions from seat's view.
func Assess(v rules.View) Situation {
	return Situation{View: v, Attacks: Candidates(v), Dangers: Dangers(v)}
}

// Empty reports whether there's nothing to ask.
func (s Situation) Empty() bool { return len(s.Attacks) == 0 && len(s.Dangers) == 0 }

// Prompt batches one short state per question.
func (s Situation) Prompt() Prompt {
	p := Prompt{Batch: map[string]Prompt{}}
	for _, d := range s.Dangers {
		p.Batch[d.Key] = Prompt{State: d.State, Questions: map[string]Question{"danger": dangerQuestion}}
	}
	for _, c := range s.Attacks {
		p.Batch[c.Key] = Prompt{State: c.State, Questions: map[string]Question{"move": moveQuestion}}
	}
	return p
}

func pick(choice string) Answer {
	return Answer{Type: "choice", Choice: choice, Probabilities: map[string]float64{choice: 1}}
}

// ViewAnswers answers every question from the player's own estimates.
func ViewAnswers(s Situation) Answers {
	out := Answers{}
	for _, d := range s.Dangers {
		out[d.Key+".danger"] = pick(dangerGuess(d))
	}
	for _, c := range s.Attacks {
		out[c.Key+".move"] = pick(attackGuess(c))
	}
	return out
}

// OracleAnswers answers every question from the true state.
func OracleAnswers(st rules.State, s Situation) Answers {
	out := Answers{}
	full := fullView(st, s.View.Seat)
	for _, d := range s.Dangers {
		truth := "safe"
		if td, ok := dangerTo(full, full.Planets[d.Planet]); ok {
			truth = dangerGuess(td)
		}
		out[d.Key+".danger"] = pick(truth)
	}
	for _, c := range s.Attacks {
		out[c.Key+".move"] = pick(attackTruth(st, c))
	}
	return out
}

// P(choice) for an answer.
func (a Answer) prob(choice string) float64 {
	if p, ok := a.Probabilities[choice]; ok {
		return p
	}
	if a.Choice == choice {
		return 1
	}
	return 0
}

// Tuning for consolidation.
const (
	frontMargin  = 2  // a rear planet is at least this many rounds farther from the enemy
	minTransfer  = 10 // don't move fewer ships than this
	maxTransport = 8  // rounds a transfer may take
)

// Decide turns answers into orders.
func Decide(s Situation, ans Answers) []Order {
	v := s.View
	speed := v.Settings.ShipSpeed
	own := map[int]rules.PlanetView{}
	spare := map[int]int{}
	var ids []int
	for _, p := range v.Planets {
		if p.Owner == v.Seat && p.Ships != nil {
			own[p.ID] = p
			spare[p.ID] = max(0, *p.Ships-reserve(p))
			ids = append(ids, p.ID)
		}
	}
	var orders []Order
	send := func(from, to, n int) {
		orders = append(orders, Order{From: from, To: to, Ships: n})
		spare[from] -= n
	}

	// 1. Defend. A planet in danger keeps all its ships; safe planets that
	// can get there before the enemy send what it needs, if together they
	// can.
	threatened := map[int]bool{}
	var dangers []Danger
	for _, d := range s.Dangers {
		if ans[d.Key+".danger"].prob("danger") > 0.5 {
			dangers = append(dangers, d)
			threatened[d.Planet] = true
			spare[d.Planet] = 0
		}
	}
	slices.SortStableFunc(dangers, func(a, b Danger) int {
		return cmp.Compare(ans[b.Key+".danger"].prob("danger"), ans[a.Key+".danger"].prob("danger"))
	})
	for _, d := range dangers {
		need := reinforcements(d)
		if need <= 0 {
			continue
		}
		target := own[d.Planet]
		var helpers []int
		total := 0
		for _, id := range ids {
			src := own[id]
			if threatened[id] || spare[id] == 0 || rules.TravelTurns(src.Pos, target.Pos, speed) > d.Soonest {
				continue
			}
			helpers = append(helpers, id)
			total += spare[id]
		}
		if total < need {
			continue // can't save it this way; it holds what it has
		}
		slices.SortStableFunc(helpers, func(a, b int) int {
			return cmp.Compare(rules.Distance(own[a].Pos, target.Pos), rules.Distance(own[b].Pos, target.Pos))
		})
		for _, id := range helpers {
			if need == 0 {
				break
			}
			n := min(need, spare[id])
			send(id, d.Planet, n)
			need -= n
		}
	}

	// 2. Attack: the attacks most believed to be good bets, one per target,
	// from planets that aren't holding.
	ranked := slices.Clone(s.Attacks)
	slices.SortStableFunc(ranked, func(a, b Candidate) int {
		return cmp.Compare(ans[b.Key+".move"].prob("good"), ans[a.Key+".move"].prob("good"))
	})
	taken := map[int]bool{}
	for _, c := range ranked {
		if ans[c.Key+".move"].prob("good") <= 0.5 || taken[c.To] || spare[c.From] < c.Ships {
			continue
		}
		send(c.From, c.To, c.Ships)
		taken[c.To] = true
	}

	// 3. Consolidate: safe planets far from the enemy send their leftover
	// ships to the planet nearest the enemy.
	front, frontDist := -1, math.Inf(1)
	dist := map[int]float64{}
	for _, id := range ids {
		dist[id] = enemyDistance(v, own[id])
		if dist[id] < frontDist {
			front, frontDist = id, dist[id]
		}
	}
	if front >= 0 && !math.IsInf(frontDist, 1) {
		for _, id := range ids {
			src := own[id]
			if id == front || threatened[id] || spare[id] < max(minTransfer, 2**src.Production) ||
				dist[id]-frontDist < frontMargin*speed ||
				rules.TravelTurns(src.Pos, own[front].Pos, speed) > maxTransport {
				continue
			}
			send(id, front, spare[id])
		}
	}
	return orders
}

// enemyDistance is how far p is from the nearest enemy planet, or from the
// nearest planet we don't own if no enemy is left in view.
func enemyDistance(v rules.View, p rules.PlanetView) float64 {
	best, any := math.Inf(1), math.Inf(1)
	for _, q := range v.Planets {
		if q.Owner == v.Seat {
			continue
		}
		d := rules.Distance(p.Pos, q.Pos)
		any = min(any, d)
		if q.Owner != rules.Neutral {
			best = min(best, d)
		}
	}
	if math.IsInf(best, 1) {
		return any
	}
	return best
}

// Planner is the rule-based player for the planner strategy: it answers its
// own questions from what it can see.
type Planner struct{}

// Name implements Player.
func (Planner) Name() string { return "Planner" }

// Orders implements Player.
func (Planner) Orders(v rules.View) []Order {
	s := Assess(v)
	return Decide(s, ViewAnswers(s))
}
