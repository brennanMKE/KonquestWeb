package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"parallax/internal/rules"
)

// wordModel answers the move question by reading the sentence's words, the
// way a perfect reader would. If the descriptions are faithful, it agrees
// with the oracle whenever the map isn't blind.
type wordModel struct{ calls atomic.Int32 }

func readMove(state string) string {
	if strings.Contains(state, "good bet") && !strings.Contains(state, "very long trip") {
		return "good"
	}
	return "bad"
}

func readDanger(state string) string {
	if strings.Contains(state, "is in danger") {
		return "danger"
	}
	return "safe"
}

func (m *wordModel) Ask(_ context.Context, p Prompt) (Answers, error) {
	m.calls.Add(1)
	out := Answers{}
	for key, sub := range p.Batch {
		for q := range sub.Questions {
			c := readMove(sub.State)
			if q == "danger" {
				c = readDanger(sub.State)
			}
			out[key+"."+q] = Answer{Type: "choice", Choice: c, Probabilities: map[string]float64{c: 0.9}}
		}
	}
	return out, nil
}

type failingModel struct{}

func (failingModel) Ask(context.Context, Prompt) (Answers, error) {
	return nil, errors.New("model down")
}

func newGame(t *testing.T, players int, s rules.Settings, seed uint64) *rules.Game {
	t.Helper()
	g, err := rules.New(players, s, seed)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestCandidates(t *testing.T) {
	g := newGame(t, 3, rules.DefaultSettings(), 4)
	// Let ships build up so there's something to send.
	for range 6 {
		if _, err := g.EndTurn(g.Active()); err != nil {
			t.Fatal(err)
		}
	}
	v := g.View(g.Active())
	cands := Candidates(v)
	if len(cands) == 0 || len(cands) > maxCandidates {
		t.Fatalf("%d candidates", len(cands))
	}
	keys := map[string]bool{}
	for _, c := range cands {
		src := v.Planets[c.From]
		if src.Owner != v.Seat || v.Planets[c.To].Owner == v.Seat {
			t.Fatalf("bad candidate %+v", c)
		}
		if c.Ships < 1 || c.Ships > *src.Ships-reserve(src) {
			t.Fatalf("candidate sends %d of %d", c.Ships, *src.Ships)
		}
		if c.Win < 0 || c.Win > 1 || c.State == "" || keys[c.Key] {
			t.Fatalf("bad candidate %+v", c)
		}
		keys[c.Key] = true
		if strings.ContainsAny(c.State, "0123456789") {
			t.Errorf("state has digits; describe in words: %q", c.State)
		}
	}
	p := Situation{Attacks: cands}.Prompt()
	if len(p.Batch) != len(cands) || p.Batch[cands[0].Key].Questions["move"].Type != "choice" {
		t.Fatalf("prompt %+v", p)
	}
}

func TestOracleAgreesWithWords(t *testing.T) {
	// On an open map the words carry everything the oracle uses, so a
	// perfect reader matches it on every question.
	for seed := range uint64(5) {
		g := newGame(t, 3, rules.DefaultSettings(), seed)
		asked, agree := 0, 0
		for !g.Over() && g.Round() < 40 {
			v := g.View(g.Active())
			sit := Assess(v)
			truth := OracleAnswers(g.State(), sit)
			for _, c := range sit.Attacks {
				asked++
				if readMove(c.State) == truth[c.Key+".move"].Choice {
					agree++
				} else {
					t.Logf("attack %+v truth %s", c, truth[c.Key+".move"].Choice)
				}
			}
			for _, d := range sit.Dangers {
				asked++
				if readDanger(d.State) == truth[d.Key+".danger"].Choice {
					agree++
				} else {
					t.Logf("danger %+v truth %s", d, truth[d.Key+".danger"].Choice)
				}
			}
			if _, err := TakeTurn(g, g.Active(), Oracle{Game: g}); err != nil {
				t.Fatal(err)
			}
		}
		if asked == 0 || agree != asked {
			t.Errorf("seed %d: words agree with the oracle on %d of %d", seed, agree, asked)
		}
	}
}

func TestOracleAndSystem1Play(t *testing.T) {
	s := rules.DefaultSettings()
	s.MaxRounds = 100
	model := &wordModel{}
	for seed := range uint64(3) {
		g := newGame(t, 3, s, seed)
		players := []Player{Oracle{Game: g}, &System1{Model: model}, Greedy{}}
		for !g.Over() {
			if _, err := TakeTurn(g, g.Active(), players[g.Active()]); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
		}
		for seat, st := range g.State().Players {
			if st.Stats.PlanetsConquered == 0 && seat < 2 {
				t.Errorf("seed %d: %s never captured a planet", seed, players[seat].Name())
			}
		}
	}
	if model.calls.Load() == 0 {
		t.Fatal("model never asked")
	}
}

func TestSystem1FallsBack(t *testing.T) {
	g := newGame(t, 2, rules.DefaultSettings(), 3)
	for range 6 {
		g.EndTurn(g.Active())
	}
	var errs int
	p := &System1{Model: failingModel{}, Fallback: Greedy{}, OnError: func(error) { errs++ }}
	v := g.View(g.Active())
	got, want := p.Orders(v), Greedy{}.Orders(v)
	if errs != 1 || len(got) != len(want) {
		t.Fatalf("errs %d, orders %v want %v", errs, got, want)
	}
	if (&System1{Model: failingModel{}}).Orders(v) != nil {
		t.Fatal("no fallback should give no orders")
	}
	// While the model is known to be down it isn't asked at all.
	errs = 0
	p.Health = fixedHealth(false)
	if got := p.Orders(v); errs != 0 || len(got) != len(want) {
		t.Fatalf("not ready: errs %d, orders %v want %v", errs, got, want)
	}
}

func TestDecide(t *testing.T) {
	ships := func(n int) *int { return &n }
	prod, kill := 10, 0.4
	v := rules.View{Seat: 0, Planets: []rules.PlanetView{
		{ID: 0, Owner: 0, Ships: ships(30), Production: &prod, Kill: &kill},
		{ID: 1, Owner: rules.Neutral},
		{ID: 2, Owner: rules.Neutral},
	}}
	cands := []Candidate{
		{Key: "a0", From: 0, To: 1, Ships: 15},
		{Key: "a1", From: 0, To: 2, Ships: 10},
		{Key: "a2", From: 0, To: 1, Ships: 5},
	}
	ans := Answers{
		"a0.move": {Type: "choice", Probabilities: map[string]float64{"good": 0.6, "bad": 0.4}},
		"a1.move": {Type: "choice", Probabilities: map[string]float64{"good": 0.9, "bad": 0.1}},
		"a2.move": {Type: "choice", Probabilities: map[string]float64{"good": 0.95}},
	}
	// Spare is 30 - 5 reserve = 25: a2 (5) first, then a1 (10); a0 targets
	// planet 1 again, so it's skipped.
	got := Decide(Situation{View: v, Attacks: cands}, ans)
	want := []Order{{From: 0, To: 1, Ships: 5}, {From: 0, To: 2, Ships: 10}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
	ans["a2.move"] = Answer{Type: "choice", Choice: "bad", Probabilities: map[string]float64{"bad": 0.8}}
	if got := Decide(Situation{View: v, Attacks: cands}, ans); len(got) != 2 || got[0] != (Order{0, 2, 10}) || got[1] != (Order{0, 1, 15}) {
		t.Fatalf("got %v", got)
	}
}

func TestClient(t *testing.T) {
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		var p Prompt
		json.NewDecoder(r.Body).Decode(&p)
		if p.Batch != nil {
			// Nested answers, as some endpoints return them.
			nested := map[string]map[string]Answer{}
			for k, sub := range p.Batch {
				nested[k] = map[string]Answer{"move": {Type: "choice", Choice: readMove(sub.State)}}
			}
			json.NewEncoder(w).Encode(map[string]any{"answers": nested})
			return
		}
		if p.State == "boom" {
			http.Error(w, "kaboom", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"answers": map[string]Answer{"move": {Type: "choice", Choice: readMove(p.State)}}})
	}))
	defer srv.Close()

	prompt := Situation{Attacks: []Candidate{
		{Key: "a0", State: "Our attack on D is a good bet: it would almost certainly win."},
		{Key: "a1", State: "Our attack on E is a bad bet: it would probably lose."},
	}}.Prompt()
	for _, batch := range []bool{true, false} {
		c := &Client{URL: srv.URL, APIKey: "k", Batch: batch}
		ans, err := c.Ask(context.Background(), prompt)
		if err != nil {
			t.Fatal(err)
		}
		if ans["a0.move"].Choice != "good" || ans["a1.move"].Choice != "bad" || len(ans) != 2 {
			t.Fatalf("batch=%v: %+v", batch, ans)
		}
		if auth.Load() != "Bearer k" {
			t.Fatalf("auth header %v", auth.Load())
		}
	}
	c := &Client{URL: srv.URL}
	if _, err := c.Ask(context.Background(), Situation{Attacks: []Candidate{{Key: "x", State: "boom"}}}.Prompt()); err == nil ||
		!strings.Contains(err.Error(), "kaboom") {
		t.Fatalf("want server error, got %v", err)
	}
}

func TestPlannerDefends(t *testing.T) {
	n := func(v int) *int { return &v }
	k := func(v float64) *float64 { return &v }
	// Our planet A (5 ships) sits 1 round from a big enemy planet C. Our
	// planet B, 1 round from A, has ships to spare and can get there in time.
	v := rules.View{Seat: 0, Settings: rules.DefaultSettings(), Planets: []rules.PlanetView{
		{ID: 0, Name: "A", Pos: rules.Pos{X: 5, Y: 5}, Owner: 0, Ships: n(5), Production: n(4), Kill: k(0.5)},
		{ID: 1, Name: "B", Pos: rules.Pos{X: 3, Y: 5}, Owner: 0, Ships: n(80), Production: n(10), Kill: k(0.5)},
		{ID: 2, Name: "C", Pos: rules.Pos{X: 7, Y: 5}, Owner: 1, Ships: n(40), Production: n(10), Kill: k(0.5)},
	}}
	s := Assess(v)
	var a Danger
	for _, d := range s.Dangers {
		if d.Planet == 0 {
			a = d
		}
	}
	if a.State == "" || !strings.HasPrefix(a.State, "Planet A is in danger") || dangerGuess(a) != "danger" {
		t.Fatalf("danger to A: %+v", a)
	}
	orders := Planner{}.Orders(v)
	sent := 0
	for _, o := range orders {
		if o.From == 0 {
			t.Fatalf("a planet in danger sent ships away: %+v", o)
		}
		if o.From == 1 && o.To == 0 {
			sent += o.Ships
		}
	}
	need := reinforcements(a)
	if need <= 0 || sent < need {
		t.Fatalf("sent %d to A, need %d; orders %+v", sent, need, orders)
	}
	if p := winChance(a.Strike, a.Kill, a.Defenders+sent, a.DefKill); p > safeWin {
		t.Fatalf("after reinforcing, the strike still wins %.2f", p)
	}
}

func TestPlannerConsolidates(t *testing.T) {
	n := func(v int) *int { return &v }
	k := func(v float64) *float64 { return &v }
	// Rear planet R has spare ships and nothing worth attacking in reach;
	// front planet F is nearest the enemy. R's ships move to F.
	v := rules.View{Seat: 0, Settings: rules.DefaultSettings(), Planets: []rules.PlanetView{
		{ID: 0, Name: "R", Pos: rules.Pos{X: 0, Y: 0}, Owner: 0, Ships: n(60), Production: n(5), Kill: k(0.4)},
		{ID: 1, Name: "F", Pos: rules.Pos{X: 8, Y: 0}, Owner: 0, Ships: n(5), Production: n(5), Kill: k(0.4)},
		{ID: 2, Name: "E", Pos: rules.Pos{X: 15, Y: 0}, Owner: 1, Ships: n(500), Production: n(10), Kill: k(0.9)},
	}}
	orders := Planner{}.Orders(v)
	if len(orders) != 1 || orders[0].From != 0 || orders[0].To != 1 || orders[0].Ships != 60-reserve(v.Planets[0]) {
		t.Fatalf("orders %+v", orders)
	}
}

func TestPlannerBeatsGreedy(t *testing.T) {
	if testing.Short() {
		t.Skip("plays many games")
	}
	s := rules.DefaultSettings()
	s.MaxRounds = 300
	wins := 0
	const games = 8
	for seed := range uint64(games) {
		g := newGame(t, 2, s, seed+100)
		players := []Player{Planner{}, Greedy{}}
		if seed%2 == 1 {
			players = []Player{Greedy{}, Planner{}}
		}
		for !g.Over() {
			if _, err := TakeTurn(g, g.Active(), players[g.Active()]); err != nil {
				t.Fatal(err)
			}
		}
		if w := g.Winner(); w >= 0 && players[w].Name() == "Planner" {
			wins++
		}
	}
	if wins*2 <= games {
		t.Fatalf("Planner won %d of %d against Greedy", wins, games)
	}
}

type fixedHealth bool

func (h fixedHealth) Up() bool { return bool(h) }
func (h fixedHealth) Check(context.Context) error {
	if !h {
		return errors.New("down")
	}
	return nil
}
