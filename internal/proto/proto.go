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
}

// DefaultSettings returns the standard room options.
func DefaultSettings() Settings {
	return Settings{Settings: rules.DefaultSettings(), AIDelayMs: 1000}
}

// Seat kinds.
const (
	KindHuman = "human"
	KindAI    = "ai"
)

// AI kinds.
const AIGreedy = "greedy"

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
type RerollMap struct{}
type StartGame struct{}
type SendFleet struct {
	From  int `json:"from"`
	To    int `json:"to"`
	Ships int `json:"ships"`
}
type EndTurn struct{}

// Server to client.

type Welcome struct {
	Seat  int    `json:"seat"`
	Token string `json:"token"`
	Room  string `json:"room"`
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
	Room       string             `json:"room"`
	Phase      string             `json:"phase"`
	Host       int                `json:"host"`
	Seats      []SeatInfo         `json:"seats"`
	Settings   Settings           `json:"settings"`
	MapPreview []rules.PlanetView `json:"mapPreview"`
}
type YourTurn struct {
	Round    int        `json:"round"`
	View     rules.View `json:"view"`
	Deadline int64      `json:"deadline,omitempty"`
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
	"start_game":      func() any { return &StartGame{} },
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
