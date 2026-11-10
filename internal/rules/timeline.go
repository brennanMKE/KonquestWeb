package rules

import "fmt"

// Frame is the whole game after one round, for watching a game back: the
// full map with every fleet, what happened as the round resolved, and the
// standings. Frame 0 is the map before anyone moves.
type Frame struct {
	Round int `json:"round"`
	// Launched are the fleets sent during the round, including any that
	// arrived as it resolved.
	Launched  []Fleet    `json:"launched"`
	Events    []Event    `json:"events"`
	View      View       `json:"view"`
	Standings []Standing `json:"standings"`
}

// Timeline replays a game from its seed, settings and actions log and
// returns a frame for the start and for each resolved round. A game saved
// mid-round has a last frame for its last resolved round; the rest of the
// round's orders aren't shown.
func Timeline(players int, s Settings, seed uint64, actions []Action) ([]Frame, error) {
	g, err := New(players, s, seed)
	if err != nil {
		return nil, err
	}
	frames := []Frame{g.frame(0, nil, []Event{})}
	var launched []Fleet
	for i, a := range actions {
		switch a.Kind {
		case ActLaunch:
			f, err := g.Launch(a.Seat, a.From, a.To, a.Ships)
			if err != nil {
				return nil, fmt.Errorf("action %d (%+v): %w", i, a, err)
			}
			launched = append(launched, f)
		case ActEndTurn:
			r := g.Round()
			events, err := g.EndTurn(a.Seat)
			if err != nil {
				return nil, fmt.Errorf("action %d (%+v): %w", i, a, err)
			}
			if events != nil {
				frames = append(frames, g.frame(r, launched, events))
				launched = nil
			}
		default:
			return nil, fmt.Errorf("action %d: %w", i, ErrUnknownAction)
		}
	}
	return frames, nil
}

func (g *Game) frame(round int, launched []Fleet, events []Event) Frame {
	if launched == nil {
		launched = []Fleet{}
	}
	return Frame{Round: round, Launched: launched, Events: events, View: g.Spectate(true), Standings: g.Standings()}
}
