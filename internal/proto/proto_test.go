package proto

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"parallax/internal/rules"
)

func TestRoundTrip(t *testing.T) {
	msgs := []any{
		&Join{Name: "Alice", Token: "abc"},
		&SetSettings{Settings: DefaultSettings()},
		&AddAI{Kind: AIGreedy},
		&RemoveSeat{Seat: 2},
		&ReplaceWithAI{Seat: 1},
		&RerollMap{},
		&StartGame{},
		&SendFleet{From: 1, To: 2, Ships: 3},
		&EndTurn{},
		&Welcome{Seat: 1, Token: "t", Room: "ABCD"},
		&Error{Code: ErrBadOrder, Message: "nope"},
		&GameOver{Winner: -1, Standings: []rules.Standing{{Seat: 0, Rank: 1}}},
	}
	for _, m := range msgs {
		b, err := Encode(m)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Decode(b)
		if err != nil {
			t.Fatalf("%s: %v", b, err)
		}
		if !reflect.DeepEqual(got, m) {
			t.Errorf("round trip %s: got %+v", b, got)
		}
	}
}

func TestEncodeShape(t *testing.T) {
	b, _ := Encode(&EndTurn{})
	if string(b) != `{"type":"end_turn"}` {
		t.Errorf("got %s", b)
	}
	b, _ = Encode(&SetSettings{Settings: DefaultSettings()})
	var m struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	// Rules settings are flattened next to room settings.
	if m.Settings["width"] != 16.0 || m.Settings["aiDelayMs"] != 1000.0 {
		t.Errorf("settings = %v", m.Settings)
	}
	if _, err := Encode(struct{}{}); err == nil {
		t.Error("want error for unknown type")
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := Decode([]byte(`{"type":"warp"}`)); !errors.Is(err, ErrUnknownType) {
		t.Errorf("got %v", err)
	}
	if _, err := Decode([]byte(`{}`)); !errors.Is(err, ErrUnknownType) {
		t.Errorf("got %v", err)
	}
	if _, err := Decode([]byte(`not json`)); err == nil {
		t.Error("want error")
	}
	if _, err := Decode([]byte(`{"type":"send_fleet","ships":"many"}`)); err == nil {
		t.Error("want error")
	}
}
