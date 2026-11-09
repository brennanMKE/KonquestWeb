package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"parallax/internal/proto"
	"parallax/internal/rules"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(New(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
	t.Cleanup(ts.Close)
	return ts
}

func createRoom(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	resp, err := http.Post(ts.URL+"/api/rooms", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct{ Code string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create room: %d %v", resp.StatusCode, err)
	}
	return body.Code
}

type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
	seen []any
}

func dial(t *testing.T, ts *httptest.Server, code string) *wsClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?room=" + code
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return &wsClient{t: t, conn: conn}
}

func (c *wsClient) send(msg any) {
	c.t.Helper()
	b, err := proto.Encode(msg)
	if err != nil {
		c.t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, b); err != nil {
		c.t.Fatal(err)
	}
}

func (c *wsClient) next() any {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, b, err := c.conn.Read(ctx)
	if err != nil {
		c.t.Fatal(err)
	}
	msg, err := proto.Decode(b)
	if err != nil {
		c.t.Fatal(err)
	}
	c.seen = append(c.seen, msg)
	return msg
}

func expect[T any](c *wsClient) *T {
	c.t.Helper()
	for {
		if m, ok := c.next().(*T); ok {
			return m
		}
	}
}

func TestRoomAPI(t *testing.T) {
	ts := newServer(t)
	code := createRoom(t, ts)
	resp, err := http.Get(ts.URL + "/api/rooms/" + strings.ToLower(code))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get room: %v %v", resp, err)
	}
	resp.Body.Close()
	resp, _ = http.Get(ts.URL + "/api/rooms/NONE")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing room: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp, _ = http.Get(ts.URL + "/ws?room=NONE")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("ws to missing room: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp, _ = http.Get(ts.URL + "/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestGameOverWebSocket plays two humans and a computer for three rounds
// over real WebSockets, then reconnects one player mid-game.
func TestGameOverWebSocket(t *testing.T) {
	ts := newServer(t)
	code := createRoom(t, ts)
	alice := dial(t, ts, code)
	alice.send(&proto.Join{Name: "Alice"})
	expect[proto.Welcome](alice)
	bob := dial(t, ts, code)
	bob.send(&proto.Join{Name: "Bob"})
	bobToken := expect[proto.Welcome](bob).Token

	s := proto.DefaultSettings()
	s.AIDelayMs = 0
	alice.send(&proto.SetSettings{Settings: s})
	alice.send(&proto.AddAI{})
	alice.send(&proto.StartGame{})

	for round := 1; round <= 3; round++ {
		yt := expect[proto.YourTurn](alice)
		if yt.Round != round {
			t.Fatalf("alice turn round %d, want %d", yt.Round, round)
		}
		home := homeOf(yt.View)
		if ships := *home.Ships; ships > 1 {
			alice.send(&proto.SendFleet{From: home.ID, To: (home.ID + 1) % len(yt.View.Planets), Ships: ships / 2})
			expect[proto.FleetAck](alice)
		}
		alice.send(&proto.EndTurn{})
		if yt := expect[proto.YourTurn](bob); yt.Round != round {
			t.Fatalf("bob turn round %d", yt.Round)
		}
		bob.send(&proto.EndTurn{})
		expect[proto.RoundResult](alice)
		expect[proto.RoundResult](bob)
	}

	// Bob drops and comes back during Alice's turn.
	bob.conn.Close(websocket.StatusNormalClosure, "")
	if st := expect[proto.SeatStatus](alice); st.Seat != 1 || st.Connected {
		t.Fatalf("seat status %+v", st)
	}
	bob2 := dial(t, ts, code)
	bob2.send(&proto.Join{Token: bobToken})
	if pt := expect[proto.PlayerTurn](bob2); pt.Seat != 0 || pt.Round != 4 {
		t.Fatalf("rejoin: %+v", pt)
	}

	// Bob never saw Alice's fleets.
	for _, msg := range append(bob.seen, bob2.seen...) {
		var v *rules.View
		switch m := msg.(type) {
		case *proto.YourTurn:
			v = &m.View
		case *proto.PlayerTurn:
			v = &m.View
		case *proto.RoundResult:
			v = &m.View
		}
		if v != nil {
			for _, f := range v.Fleets {
				if f.Owner != 1 {
					t.Fatalf("bob saw fleet %+v", f)
				}
			}
		}
	}
}

func homeOf(v rules.View) rules.PlanetView {
	for _, p := range v.Planets {
		if p.Owner == v.Seat {
			return p
		}
	}
	panic("no planet")
}
