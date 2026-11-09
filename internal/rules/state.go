package rules

import (
	"math"
	"slices"
)

// Neutral is the owner of planets no player holds.
const Neutral = -1

// Pos is a cell on the map. Z is 0 on a flat map.
type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
}

// Distance is the Euclidean distance between two cells.
func Distance(a, b Pos) float64 {
	dx, dy, dz := float64(a.X-b.X), float64(a.Y-b.Y), float64(a.Z-b.Z)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// TravelTurns is how many rounds a fleet takes between two cells at speed
// cells per round: the distance divided by speed, rounded up, at least 1.
func TravelTurns(a, b Pos, speed float64) int {
	// The small epsilon keeps exact multiples (distance 4 at speed 2) from
	// rounding up because of floating-point error.
	t := int(math.Ceil(Distance(a, b)/speed - 1e-9))
	return max(t, 1)
}

// Planet is one planet on the map.
type Planet struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Pos        Pos     `json:"pos"`
	Owner      int     `json:"owner"`
	Ships      int     `json:"ships"`
	Production int     `json:"production"`
	Kill       float64 `json:"kill"`
	Home       bool    `json:"home"`
	// CapturedRound is the round the planet last changed owner, 0 if never.
	CapturedRound int `json:"capturedRound"`
}

// Fleet is a group of ships in flight.
type Fleet struct {
	ID    int `json:"id"`
	Owner int `json:"owner"`
	From  int `json:"from"`
	To    int `json:"to"`
	Ships int `json:"ships"`
	// Kill is the source planet's kill percentage, fixed at launch.
	Kill         float64 `json:"kill"`
	LaunchRound  int     `json:"launchRound"`
	ArrivalRound int     `json:"arrivalRound"`
}

// Stats are a player's running totals for the standings screen.
type Stats struct {
	ShipsBuilt       int `json:"shipsBuilt"`
	ShipsDestroyed   int `json:"shipsDestroyed"`
	ShipsLost        int `json:"shipsLost"`
	PlanetsConquered int `json:"planetsConquered"`
	FleetsLaunched   int `json:"fleetsLaunched"`
}

// Player is one seat's standing in the game.
type Player struct {
	Seat  int   `json:"seat"`
	Alive bool  `json:"alive"`
	Stats Stats `json:"stats"`
}

// State is the full, unfiltered game state. Only the server sees it; players
// get a View.
type State struct {
	Settings Settings `json:"settings"`
	Round    int      `json:"round"`
	// Active is the seat whose turn it is, or -1 when the game is over.
	Active      int      `json:"active"`
	Planets     []Planet `json:"planets"`
	Fleets      []Fleet  `json:"fleets"`
	Players     []Player `json:"players"`
	NextFleetID int      `json:"nextFleetId"`
	Over        bool     `json:"over"`
	// Winner is the winning seat, or -1 for none (game running or a tie).
	Winner int `json:"winner"`
}

func (s State) clone() State {
	s.Planets = slices.Clone(s.Planets)
	s.Fleets = slices.Clone(s.Fleets)
	s.Players = slices.Clone(s.Players)
	return s
}
