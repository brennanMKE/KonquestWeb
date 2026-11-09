package rules

import "fmt"

// MinPlayers and MaxPlayers bound the number of seats in a game.
const (
	MinPlayers = 2
	MaxPlayers = 9
)

// Settings are the room options that shape a game. See RULES.md.
type Settings struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	// Depth is the number of layers; 1 is a flat map.
	Depth    int `json:"depth"`
	Neutrals int `json:"neutrals"`
	// ShipSpeed is how many cells a fleet travels per round.
	ShipSpeed float64 `json:"shipSpeed"`
	// BlindMap hides ships, production and kill percentage of planets the
	// player doesn't own.
	BlindMap bool `json:"blindMap"`
	// NeutralStats shows neutral planets' stats when the map isn't blind.
	NeutralStats bool `json:"neutralStats"`
	// CaptureProducesImmediately lets a planet produce in the round it's
	// captured. Off means it waits a round.
	CaptureProducesImmediately bool `json:"captureProducesImmediately"`
	// MaxRounds ends the game after that many rounds; 0 means no limit.
	MaxRounds int `json:"maxRounds"`
}

// DefaultSettings returns the standard room options.
func DefaultSettings() Settings {
	return Settings{
		Width:        16,
		Height:       16,
		Depth:        1,
		Neutrals:     10,
		ShipSpeed:    2,
		NeutralStats: true,
	}
}

// Validate reports whether the settings can produce a game for players.
func (s Settings) Validate(players int) error {
	switch {
	case players < MinPlayers || players > MaxPlayers:
		return fmt.Errorf("players must be %d to %d, got %d", MinPlayers, MaxPlayers, players)
	case s.Width < 4 || s.Width > 64 || s.Height < 4 || s.Height > 64:
		return fmt.Errorf("width and height must be 4 to 64, got %dx%d", s.Width, s.Height)
	case s.Depth < 1 || s.Depth > 64:
		return fmt.Errorf("depth must be 1 to 64, got %d", s.Depth)
	case s.Neutrals < 0:
		return fmt.Errorf("neutrals must not be negative, got %d", s.Neutrals)
	case !(s.ShipSpeed > 0):
		return fmt.Errorf("ship speed must be positive, got %v", s.ShipSpeed)
	case s.MaxRounds < 0:
		return fmt.Errorf("max rounds must not be negative, got %d", s.MaxRounds)
	}
	if cells, planets := s.Width*s.Height*s.Depth, players+s.Neutrals; planets*2 > cells {
		return fmt.Errorf("%d planets need at least %d cells, map has %d", planets, planets*2, cells)
	}
	return nil
}
