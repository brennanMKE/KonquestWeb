package ai

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	"parallax/internal/rules"
)

// The System 1 player follows the approach from System 1 Arcade: the game
// does the arithmetic and describes each candidate attack in one short
// plain-English sentence, a System 1 model (such as Laya) answers a typed
// question about each sentence with probabilities, and the code below turns
// the answers into orders. An oracle answers the same questions from the
// true game state, which measures both the descriptions and the model.

// Prompt is a request in the Laya/Jev shape: State and Questions, or Batch
// with one small prompt per key. Answers to a batch are keyed "key.question".
type Prompt struct {
	State     string              `json:"state,omitempty"`
	Questions map[string]Question `json:"questions,omitempty"`
	Batch     map[string]Prompt   `json:"batch,omitempty"`
}

// Question is a typed question: "choice" with criteria, or "noul" (yes/no).
type Question struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria,omitempty"`
}

// Answer is one answered question in Laya's result format.
type Answer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Noul          float64            `json:"noul"`
}

// Answers maps question names to answers.
type Answers map[string]Answer

// moveQuestion is asked about every candidate attack. Its options repeat
// the words the state uses.
var moveQuestion = Question{
	Type:         "choice",
	Instructions: "Is this attack a good bet or a bad bet?",
	Criteria:     map[string]string{"good": "a good bet", "bad": "a bad bet"},
}

// P(good) for a move answer.
func (a Answer) good() float64 {
	if p, ok := a.Probabilities["good"]; ok {
		return p
	}
	if a.Choice == "good" {
		return 1
	}
	return 0
}

// Candidate is one attack the game considers, with the numbers behind its
// description.
type Candidate struct {
	Key   string `json:"key"`
	From  int    `json:"from"`
	To    int    `json:"to"`
	Ships int    `json:"ships"`
	Turns int    `json:"turns"`
	// Win is the estimated chance the attack takes the planet, from what
	// the player can see.
	Win   float64 `json:"win"`
	State string  `json:"state"`
}

// Tuning for candidate generation.
const (
	targetWin     = 0.85 // send enough ships for this win chance, if we can
	perSource     = 3    // candidates per source planet
	maxCandidates = 10   // candidates per decision (states per batch)
	goodWin       = 0.75 // the oracle's bar for a good attack
	maxGoodTurns  = 8    // and for how far it may travel
)

// Candidates lists the attacks worth asking about, from seat's view.
func Candidates(v rules.View) []Candidate {
	type scored struct {
		c     Candidate
		score float64
	}
	var all []scored
	for _, src := range v.Planets {
		if src.Owner != v.Seat || src.Ships == nil {
			continue
		}
		spare := *src.Ships - reserve(src)
		if spare < 1 {
			continue
		}
		var mine []scored
		for _, t := range v.Planets {
			if t.Owner == v.Seat {
				continue
			}
			turns := rules.TravelTurns(src.Pos, t.Pos, v.Settings.ShipSpeed)
			def, defKill := defendersAtArrival(t, turns)
			ships := shipsToWin(spare, *src.Kill, def, defKill, targetWin)
			win := winChance(ships, *src.Kill, def, defKill)
			c := Candidate{From: src.ID, To: t.ID, Ships: ships, Turns: turns, Win: win}
			c.State = describe(v, src, t, c)
			mine = append(mine, scored{c, win * float64(production(t)) / float64(turns+2)})
		}
		slices.SortStableFunc(mine, func(a, b scored) int { return cmp.Compare(b.score, a.score) })
		all = append(all, mine[:min(perSource, len(mine))]...)
	}
	slices.SortStableFunc(all, func(a, b scored) int { return cmp.Compare(b.score, a.score) })
	all = all[:min(maxCandidates, len(all))]
	out := make([]Candidate, len(all))
	for i, s := range all {
		s.c.Key = fmt.Sprintf("a%d", i)
		out[i] = s.c
	}
	return out
}

// defendersAtArrival estimates the garrison a fleet meets after turns
// rounds, and its kill percentage. A fleet arrives before that round's
// production, so the planet produces turns-1 more times.
func defendersAtArrival(t rules.PlanetView, turns int) (int, float64) {
	ships := unknownShips
	if t.Ships != nil {
		ships = *t.Ships
	}
	growth := rules.NeutralGrowth
	if t.Owner != rules.Neutral {
		growth = production(t)
	}
	kill := unknownKill
	if t.Kill != nil {
		kill = *t.Kill
	}
	return ships + growth*(turns-1), kill
}

// describe writes the candidate as one sentence with conclusions in words,
// because a System 1 model reads words well and numbers badly.
func describe(v rules.View, src, t rules.PlanetView, c Candidate) string {
	kind := "a neutral planet"
	if t.Owner != rules.Neutral {
		kind = "an enemy planet"
	}
	builds := "an unknown number of ships"
	if t.Production != nil {
		switch p := *t.Production; {
		case p >= 12:
			builds = "many ships"
		case p >= 8:
			builds = "a fair number of ships"
		default:
			builds = "few ships"
		}
	}
	// The outcome leads: a System 1 model reads the start of a short state
	// most reliably.
	odds := winWords(c.Win)
	if t.Ships == nil {
		odds += " against an unknown number of defenders"
	}
	return fmt.Sprintf("Our attack on %s %s. It arrives %s, from %s, at %s that builds %s.",
		t.Name, odds, arrivalWords(c.Turns), src.Name, kind, builds)
}

// winWords and the move question share the words "good bet" and "bad bet",
// because a System 1 model picks options that repeat the state's words.
func winWords(p float64) string {
	switch {
	case p >= 0.9:
		return "is a good bet: it would almost certainly win"
	case p >= goodWin:
		return "is a good bet: it would probably win"
	case p >= 0.5:
		return "is a bad bet: it might win or lose"
	default:
		return "is a bad bet: it would probably lose"
	}
}

func arrivalWords(turns int) string {
	switch {
	case turns <= 2:
		return "soon"
	case turns <= 5:
		return "in a few rounds"
	case turns <= maxGoodTurns:
		return "after many rounds"
	default:
		return "only after a very long trip, which makes it a bad bet"
	}
}

// attackTruth answers a candidate's move question from the true state: an
// attack is good if, against the real garrison, it wins at least goodWin of
// the time and arrives within maxGoodTurns rounds.
func attackTruth(st rules.State, c Candidate) string {
	t, src := st.Planets[c.To], st.Planets[c.From]
	growth := rules.NeutralGrowth
	if t.Owner != rules.Neutral {
		growth = t.Production
	}
	def := t.Ships + growth*(c.Turns-1)
	if winChance(c.Ships, src.Kill, def, t.Kill) >= goodWin && c.Turns <= maxGoodTurns {
		return "good"
	}
	return "bad"
}

// attackGuess answers from the player's own estimates.
func attackGuess(c Candidate) string {
	if c.Win >= goodWin && c.Turns <= maxGoodTurns {
		return "good"
	}
	return "bad"
}

// Asker answers a prompt; *Client is the HTTP implementation.
type Asker interface {
	Ask(ctx context.Context, p Prompt) (Answers, error)
}

// System1 is a computer player that asks a System 1 model the planner's
// questions (is each planet in danger, is each attack a good bet) and plays
// the planner's strategy on the model's answers. If the model can't be
// reached it plays as Fallback.
type System1 struct {
	Model    Asker
	Fallback Player
	// Timeout bounds one decision. 0 means 15 seconds.
	Timeout time.Duration
	// OnError, if set, hears about failed requests.
	OnError func(error)
	// OnAnswer, if set, hears about each request the model answered.
	OnAnswer func()
	// Health, if set, tracks whether the model is reachable. While it's
	// down the model is skipped and Fallback plays at once.
	Health Health
	// Gate, if set, caps requests in flight to the model; a decision that
	// can't get a slot in time plays Fallback.
	Gate *Gate
	// Stats, if set, counts decisions and fallbacks.
	Stats *Stats
}

// Health is a service a computer player depends on.
type Health interface {
	// Up is the last known state; it must be cheap.
	Up() bool
	// Check asks the service now and updates Up.
	Check(ctx context.Context) error
}

// Checker is a computer player that depends on a service that may be down.
// Rooms don't add one while Check fails.
type Checker interface {
	Player
	Health
}

// Up implements Checker.
func (s *System1) Up() bool { return s.Health == nil || s.Health.Up() }

// Check implements Checker.
func (s *System1) Check(ctx context.Context) error {
	if s.Health == nil {
		return nil
	}
	return s.Health.Check(ctx)
}

// Name implements Player.
func (s *System1) Name() string { return "System 1" }

// Orders implements Player.
func (s *System1) Orders(v rules.View) []Order {
	sit := Assess(v)
	if sit.Empty() {
		return Decide(sit, Answers{})
	}
	fallback := func(count func(*Stats) *atomic.Int64) []Order {
		s.Stats.add(count)
		if s.Fallback != nil {
			return s.Fallback.Orders(v)
		}
		return nil
	}
	if !s.Up() {
		return fallback(func(s *Stats) *atomic.Int64 { return &s.Down })
	}
	timeout := s.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	release, ok := s.Gate.Acquire(ctx)
	if !ok {
		return fallback(func(s *Stats) *atomic.Int64 { return &s.Queued })
	}
	s.Stats.add(func(s *Stats) *atomic.Int64 { return &s.Asked })
	ans, err := s.Model.Ask(ctx, sit.Prompt())
	release()
	switch {
	case errors.Is(err, ErrBusy):
		// Load, not an outage: the model stays up.
		return fallback(func(s *Stats) *atomic.Int64 { return &s.Busy })
	case errors.Is(err, context.DeadlineExceeded):
		// Slow under load, most likely; the health probe finds real outages.
		return fallback(func(s *Stats) *atomic.Int64 { return &s.Timeout })
	case err != nil:
		if s.OnError != nil {
			s.OnError(err)
		}
		return fallback(func(s *Stats) *atomic.Int64 { return &s.Failed })
	}
	s.Stats.add(func(s *Stats) *atomic.Int64 { return &s.Answered })
	if s.OnAnswer != nil {
		s.OnAnswer()
	}
	return Decide(sit, ans)
}

// Oracle plays the planner's strategy with perfect answers. It reads the
// true state, so it's for measuring (cmd/sim), never for rooms.
type Oracle struct {
	Game *rules.Game
}

// Name implements Player.
func (o Oracle) Name() string { return "Oracle" }

// Orders implements Player.
func (o Oracle) Orders(v rules.View) []Order {
	sit := Assess(v)
	return Decide(sit, OracleAnswers(o.Game.State(), sit))
}
