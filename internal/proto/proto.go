// Package proto defines the WebSocket messages between browser and server.
// Every message is a JSON object with a "type" field.
package proto

import (
	"encoding/json"
	"errors"
	"fmt"

	"parallax/internal/rules"
)

// Settings are the room options: the game's rules settings plus room-level
// options. The rules fields are flattened into the same JSON object.
type Settings struct {
	rules.Settings
	// AIDelayMs is how long a computer player waits before moving, so humans
	// can follow along.
	AIDelayMs int `json:"aiDelayMs"`
	// TurnTimerMs ends a human's turn after this long, keeping the orders
	// they gave. 0 is off.
	TurnTimerMs int `json:"turnTimerMs"`
	// DisconnectGraceMs skips a disconnected player's turn after this long.
	// 0 waits for them (the host can hand the seat to a computer).
	DisconnectGraceMs int `json:"disconnectGraceMs"`
}

// DefaultSettings returns the standard room options: a 32 × 32 × 32 cube
// with planets spread through it, one per sector. Ships fly 4 cells a round
// there, so a hop to a neighbouring sector takes about 3 rounds.
func DefaultSettings() Settings {
	s := rules.DefaultSettings()
	s.Width, s.Height, s.Depth = 32, 32, 32
	s.Neutrals = 16
	s.ShipSpeed = 4
	return Settings{Settings: s, AIDelayMs: 1000}
}

// Seat kinds.
const (
	KindHuman = "human"
	KindAI    = "ai"
)

// AI kinds.
const (
	AIGreedy  = "greedy"
	AIPlanner = "planner"
	AISystem1 = "system1"
)

// Phases of a room.
const (
	PhaseLobby   = "lobby"
	PhasePlaying = "playing"
	PhaseOver    = "over"
)

// Error codes.
const (
	ErrBadMessage  = "bad_message"
	ErrNotJoined   = "not_joined"
	ErrBadToken    = "bad_token"
	ErrRoomFull    = "room_full"
	ErrNotHost     = "not_host"
	ErrWrongPhase  = "wrong_phase"
	ErrBadSeat     = "bad_seat"
	ErrBadSetting  = "bad_settings"
	ErrNotYourTurn = "not_your_turn"
	ErrBadOrder    = "bad_order"
	ErrRemoved     = "removed"
	ErrReplaced    = "replaced"
	// ErrAIUnavailable: that kind of computer player needs a service that
	// can't be reached right now.
	ErrAIUnavailable = "ai_unavailable"
)

// Client to server.

type Join struct {
	Name  string `json:"name"`
	Token string `json:"token,omitempty"`
}
type SetSettings struct {
	Settings Settings `json:"settings"`
}
type AddAI struct {
	Kind string `json:"kind,omitempty"`
}
type RemoveSeat struct {
	Seat int `json:"seat"`
}

// ReplaceWithAI hands a disconnected human's seat to a computer player.
type ReplaceWithAI struct {
	Seat int    `json:"seat"`
	Kind string `json:"kind,omitempty"`
}

// Watch makes the host a watcher instead of a player, or a player again.
// Lobby only.
type Watch struct {
	Watch bool `json:"watch"`
}
type RerollMap struct{}
type StartGame struct{}

// NewGame takes a finished game's room back to the lobby for another game
// with the same seats. Host only.
type NewGame struct{}
type SendFleet struct {
	From  int `json:"from"`
	To    int `json:"to"`
	Ships int `json:"ships"`
}
type EndTurn struct{}

// Server to client.

// Welcome tells a client its seat (rules.Spectator when watching) and
// whether it hosts the room.
type Welcome struct {
	Seat  int    `json:"seat"`
	Token string `json:"token"`
	Room  string `json:"room"`
	Host  bool   `json:"host"`
}

// SeatInfo describes one seat in the lobby or game.
type SeatInfo struct {
	Seat      int    `json:"seat"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	AIKind    string `json:"aiKind,omitempty"`
	Connected bool   `json:"connected"`
}
type Lobby struct {
	Room  string `json:"room"`
	Phase string `json:"phase"`
	// Host is the host's seat, or -1 when the host is watching.
	Host       int                `json:"host"`
	HostName   string             `json:"hostName"`
	Seats      []SeatInfo         `json:"seats"`
	Settings   Settings           `json:"settings"`
	MapPreview []rules.PlanetView `json:"mapPreview"`
	// AIKinds are the computer players this server offers.
	AIKinds []string `json:"aiKinds"`
}
type YourTurn struct {
	Round int        `json:"round"`
	View  rules.View `json:"view"`
	// Deadline is when the turn timer ends the turn, in Unix milliseconds;
	// 0 means no timer.
	Deadline int64 `json:"deadline,omitempty"`
}

// PlayerTurn tells a waiting player whose turn it is, with the waiting
// player's own view.
type PlayerTurn struct {
	Round    int        `json:"round"`
	Seat     int        `json:"seat"`
	Name     string     `json:"name"`
	View     rules.View `json:"view"`
	Deadline int64      `json:"deadline,omitempty"`
}
type FleetAck struct {
	FleetID      int        `json:"fleetId"`
	ArrivalRound int        `json:"arrivalRound"`
	View         rules.View `json:"view"`
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type SeatStatus struct {
	Seat      int  `json:"seat"`
	Connected bool `json:"connected"`
}
type RoundResult struct {
	Round  int           `json:"round"`
	Events []rules.Event `json:"events"`
	View   rules.View    `json:"view"`
}
type GameOver struct {
	Winner    int              `json:"winner"`
	Standings []rules.Standing `json:"standings"`
}

var types = map[string]func() any{
	"join":            func() any { return &Join{} },
	"set_settings":    func() any { return &SetSettings{} },
	"add_ai":          func() any { return &AddAI{} },
	"remove_seat":     func() any { return &RemoveSeat{} },
	"replace_with_ai": func() any { return &ReplaceWithAI{} },
	"reroll_map":      func() any { return &RerollMap{} },
	"watch":           func() any { return &Watch{} },
	"start_game":      func() any { return &StartGame{} },
	"new_game":        func() any { return &NewGame{} },
	"send_fleet":      func() any { return &SendFleet{} },
	"end_turn":        func() any { return &EndTurn{} },
	"welcome":         func() any { return &Welcome{} },
	"lobby":           func() any { return &Lobby{} },
	"your_turn":       func() any { return &YourTurn{} },
	"player_turn":     func() any { return &PlayerTurn{} },
	"fleet_ack":       func() any { return &FleetAck{} },
	"error":           func() any { return &Error{} },
	"seat_status":     func() any { return &SeatStatus{} },
	"round_result":    func() any { return &RoundResult{} },
	"game_over":       func() any { return &GameOver{} },
}

var names = func() map[string]string {
	m := map[string]string{}
	for name, f := range types {
		m[fmt.Sprintf("%T", f())] = name
	}
	return m
}()

// TypeOf returns the wire type name of a message, or "".
func TypeOf(msg any) string { return names[fmt.Sprintf("%T", msg)] }

// Encode marshals a message (a pointer to one of this package's message
// types) with its "type" field.
func Encode(msg any) ([]byte, error) {
	name := TypeOf(msg)
	if name == "" {
		return nil, fmt.Errorf("proto: unknown message type %T", msg)
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	head := `{"type":"` + name + `"`
	if string(body) == "{}" {
		return []byte(head + "}"), nil
	}
	return append([]byte(head+","), body[1:]...), nil
}

// ErrUnknownType is returned by Decode for a missing or unknown type.
var ErrUnknownType = errors.New("proto: unknown message type")

// Decode parses a message and returns a pointer to its typed struct.
func Decode(data []byte) (any, error) {
	var env struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	f, ok := types[env.Type]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownType, env.Type)
	}
	msg := f()
	if err := json.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// SaveVersion is the current game file format.
const SaveVersion = 1

// SaveFile is a saved game: enough to replay it exactly.
type SaveFile struct {
	Version int    `json:"version"`
	Seed    uint64 `json:"seed"`
	// Room is the room code the game was played in. Faction and planet
	// names come from it, so a replay names things as the players saw them.
	Room string `json:"room,omitempty"`
	// Round is the round the game was saved in, for information only.
	Round    int            `json:"round"`
	Settings Settings       `json:"settings"`
	Seats    []SavedSeat    `json:"seats"`
	Actions  []rules.Action `json:"actions"`
}

// SavedSeat is one seat in a saved game.
type SavedSeat struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	AIKind string `json:"aiKind,omitempty"`
}

// Replay is a whole game for watching back: who played, and the full map
// after every round.
type Replay struct {
	Room     string        `json:"room"`
	Settings Settings      `json:"settings"`
	Seats    []SavedSeat   `json:"seats"`
	Frames   []rules.Frame `json:"frames"`
	Over     bool          `json:"over"`
	Winner   int           `json:"winner"`
}

// BuildReplay replays a game file into its frames.
func BuildReplay(f *SaveFile) (*Replay, error) {
	if f.Version != SaveVersion {
		return nil, fmt.Errorf("unsupported save version %d", f.Version)
	}
	if len(f.Seats) == 0 {
		return nil, errors.New("the game file has no seats")
	}
	frames, err := rules.Timeline(len(f.Seats), f.Settings.Settings, f.Seed, f.Actions)
	if err != nil {
		return nil, fmt.Errorf("replaying the game: %w", err)
	}
	last := frames[len(frames)-1].View
	return &Replay{Room: f.Room, Settings: f.Settings, Seats: f.Seats, Frames: frames, Over: last.Over, Winner: last.Winner}, nil
}
