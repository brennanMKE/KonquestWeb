package rules

import "testing"

func TestTimeline(t *testing.T) {
	s := DefaultSettings()
	g, err := New(2, s, 7)
	if err != nil {
		t.Fatal(err)
	}
	// Each seat sends half its home fleet at the other's home every round
	// until the game ends or 30 rounds pass.
	home := func(seat int) int {
		for _, p := range g.State().Planets {
			if p.Owner == seat && p.Home {
				return p.ID
			}
		}
		return -1
	}
	for g.Round() <= 30 && !g.Over() {
		for seat := 0; seat < 2 && !g.Over(); seat++ {
			if g.Active() != seat {
				continue
			}
			if from, to := home(seat), home(1-seat); from >= 0 && to >= 0 {
				if n := g.State().Planets[from].Ships / 2; n > 0 {
					if _, err := g.Launch(seat, from, to, n); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := g.EndTurn(seat); err != nil {
				t.Fatal(err)
			}
		}
	}

	frames, err := Timeline(2, s, 7, g.Actions())
	if err != nil {
		t.Fatal(err)
	}
	resolved := g.Round() - 1
	if g.Over() {
		resolved = g.Round()
	}
	if len(frames) != resolved+1 {
		t.Fatalf("got %d frames, want %d", len(frames), resolved+1)
	}
	if frames[0].Round != 0 || len(frames[0].Events) != 0 || frames[0].View.Round != 1 {
		t.Errorf("frame 0 = round %d, %d events, view round %d", frames[0].Round, len(frames[0].Events), frames[0].View.Round)
	}
	launched := 0
	for i, f := range frames[1:] {
		if f.Round != i+1 {
			t.Errorf("frame %d is round %d", i+1, f.Round)
		}
		launched += len(f.Launched)
		for _, p := range f.View.Planets {
			if p.Ships == nil {
				t.Fatalf("round %d: planet %d hides its ships", f.Round, p.ID)
			}
		}
	}
	if want := g.Standings()[0].Stats.FleetsLaunched + g.Standings()[1].Stats.FleetsLaunched; launched != want {
		t.Errorf("frames launch %d fleets, game launched %d", launched, want)
	}
	last := frames[len(frames)-1]
	if got, want := last.View.Over, g.Over(); got != want {
		t.Errorf("last frame over = %v, want %v", got, want)
	}
	st := g.State()
	for i, p := range last.View.Planets {
		if p.Owner != st.Planets[i].Owner || *p.Ships != st.Planets[i].Ships {
			t.Errorf("planet %d: frame has %d/%d, game has %d/%d", i, p.Owner, *p.Ships, st.Planets[i].Owner, st.Planets[i].Ships)
		}
	}

	if _, err := Timeline(2, s, 7, []Action{{Kind: ActEndTurn, Seat: 1}}); err == nil {
		t.Error("an out-of-turn action should fail")
	}
}
