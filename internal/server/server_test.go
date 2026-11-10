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

func TestSaveAndLoad(t *testing.T) {
	ts := newServer(t)
	code := createRoom(t, ts)
	alice := dial(t, ts, code)
	alice.send(&proto.Join{Name: "Alice"})
	token := expect[proto.Welcome](alice).Token
	bob := dial(t, ts, code)
	bob.send(&proto.Join{Name: "Bob"})
	bobToken := expect[proto.Welcome](bob).Token

	saveURL := ts.URL + "/api/rooms/" + code + "/save?token="
	if resp, _ := http.Get(saveURL + token); resp.StatusCode != http.StatusConflict {
		t.Fatalf("save before start: %d", resp.StatusCode)
	}

	s := proto.DefaultSettings()
	s.AIDelayMs = 0
	alice.send(&proto.SetSettings{Settings: s})
	alice.send(&proto.AddAI{})
	alice.send(&proto.StartGame{})
	for range 2 {
		yt := expect[proto.YourTurn](alice)
		home := homeOf(yt.View)
		alice.send(&proto.SendFleet{From: home.ID, To: (home.ID + 1) % len(yt.View.Planets), Ships: 2})
		expect[proto.FleetAck](alice)
		alice.send(&proto.EndTurn{})
		expect[proto.YourTurn](bob)
		bob.send(&proto.EndTurn{})
	}
	before := expect[proto.YourTurn](alice)

	if resp, _ := http.Get(saveURL + bobToken); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("save by non-host: %d", resp.StatusCode)
	}
	// A running game can't be watched back: it would show every order.
	if resp, _ := http.Get(ts.URL + "/api/rooms/" + code + "/replay"); resp.StatusCode != http.StatusConflict {
		t.Fatalf("replay of a running game: %d", resp.StatusCode)
	}
	resp, err := http.Get(saveURL + token)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("save: %v %v", resp, err)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "round-3") {
		t.Errorf("Content-Disposition %q", cd)
	}
	var f proto.SaveFile
	json.NewDecoder(resp.Body).Decode(&f)
	resp.Body.Close()
	if f.Version != proto.SaveVersion || len(f.Seats) != 3 || f.Seats[2].Kind != proto.KindAI || len(f.Actions) == 0 {
		t.Fatalf("save file: %+v", f)
	}

	if f.Room != code {
		t.Errorf("save file room %q, want %q", f.Room, code)
	}

	body, _ := json.Marshal(f)
	// The file plays back without opening a room: the start and two rounds.
	resp, err = http.Post(ts.URL+"/api/replay", "application/json", strings.NewReader(string(body)))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("file replay: %v %v", resp, err)
	}
	var rp proto.Replay
	json.NewDecoder(resp.Body).Decode(&rp)
	resp.Body.Close()
	if len(rp.Frames) != 3 || rp.Room != code || len(rp.Seats) != 3 || rp.Over || len(rp.Frames[1].Launched) != 1 {
		t.Fatalf("file replay: %d frames, room %q, over %v", len(rp.Frames), rp.Room, rp.Over)
	}

	resp, err = http.Post(ts.URL+"/api/load", "application/json", strings.NewReader(string(body)))
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("load: %v %v", resp, err)
	}
	var loaded struct{ Code string }
	json.NewDecoder(resp.Body).Decode(&loaded)
	resp.Body.Close()

	// Bob claims his seat by name, Alice takes the remaining one.
	bob2 := dial(t, ts, loaded.Code)
	bob2.send(&proto.Join{Name: "bob"})
	if w := expect[proto.Welcome](bob2); w.Seat != 1 || w.Token == "" {
		t.Fatalf("bob's seat: %+v", w)
	}
	alice2 := dial(t, ts, loaded.Code)
	alice2.send(&proto.Join{Name: "Someone"})
	if w := expect[proto.Welcome](alice2); w.Seat != 0 {
		t.Fatalf("alice's seat: %+v", w)
	}
	if l := expect[proto.Lobby](alice2); l.Host != 0 || l.Phase != proto.PhasePlaying {
		t.Fatalf("lobby: %+v", l)
	}
	after := expect[proto.YourTurn](alice2)
	if after.Round != before.Round || len(after.View.Fleets) != len(before.View.Fleets) {
		t.Fatalf("loaded game differs: round %d vs %d", after.Round, before.Round)
	}
	for i := range before.View.Planets {
		a, b := before.View.Planets[i], after.View.Planets[i]
		if a.Owner != b.Owner || (a.Ships == nil) != (b.Ships == nil) || (a.Ships != nil && *a.Ships != *b.Ships) {
			t.Fatalf("planet %d differs: %+v vs %+v", i, a, b)
		}
	}
	// Nobody else can join: every seat is claimed.
	eve := dial(t, ts, loaded.Code)
	eve.send(&proto.Join{Name: "Eve"})
	if e := expect[proto.Error](eve); e.Code != proto.ErrWrongPhase {
		t.Fatalf("eve: %+v", e)
	}
}

func TestFinishedReplay(t *testing.T) {
	ts := newServer(t)
	code := createRoom(t, ts)
	alice := dial(t, ts, code)
	alice.send(&proto.Join{Name: "Alice"})
	expect[proto.Welcome](alice)
	s := proto.DefaultSettings()
	s.AIDelayMs = 0
	s.MaxRounds = 2
	alice.send(&proto.SetSettings{Settings: s})
	alice.send(&proto.AddAI{})
	alice.send(&proto.StartGame{})
	for range 2 {
		expect[proto.YourTurn](alice)
		alice.send(&proto.EndTurn{})
	}
	over := expect[proto.GameOver](alice)

	// Anyone with the code may watch a finished game, with no token.
	resp, err := http.Get(ts.URL + "/api/rooms/" + code + "/replay")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("replay: %v %v", resp, err)
	}
	var rp proto.Replay
	json.NewDecoder(resp.Body).Decode(&rp)
	resp.Body.Close()
	if len(rp.Frames) != 3 || !rp.Over || rp.Winner != over.Winner || rp.Seats[0].Name != "Alice" {
		t.Fatalf("replay: %d frames, over %v, winner %d", len(rp.Frames), rp.Over, rp.Winner)
	}
	if resp, _ := http.Get(ts.URL + "/api/rooms/ZZZZ/replay"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("replay of no room: %d", resp.StatusCode)
	}
	// Once it's over, anyone may save the file too.
	if resp, _ := http.Get(ts.URL + "/api/rooms/" + code + "/save"); resp.StatusCode != http.StatusOK {
		t.Errorf("save of a finished game: %d", resp.StatusCode)
	}
	resp, _ = http.Post(ts.URL+"/api/replay", "application/json", strings.NewReader(`{"version": 1, "seats": []}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("replay of an empty file: %d", resp.StatusCode)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	ts := newServer(t)
	for _, body := range []string{
		`not json`,
		`{"version": 99}`,
		`{"version": 1, "seed": 1, "settings": {"width": 16, "height": 16, "depth": 1, "neutrals": 10, "shipSpeed": 2},
		  "seats": [{"name": "A", "kind": "ai"}, {"name": "B", "kind": "ai"}], "actions": []}`,
		`{"version": 1, "seed": 1, "settings": {"width": 16, "height": 16, "depth": 1, "neutrals": 10, "shipSpeed": 2},
		  "seats": [{"name": "A", "kind": "human"}, {"name": "B", "kind": "ai"}], "actions": [{"kind": "end_turn", "seat": 1}]}`,
	} {
		resp, err := http.Post(ts.URL+"/api/load", "application/json", strings.NewReader(body))
		if err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%.40s: %v %v", body, resp.StatusCode, err)
		}
		resp.Body.Close()
	}
}

func TestHealthz(t *testing.T) {
	ts := httptest.NewServer(New(Options{}))
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
}

func TestOrigins(t *testing.T) {
	dial := func(ts *httptest.Server, origin string) error {
		code := createRoom(t, ts)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?room=" + code
		conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}}})
		if err == nil {
			conn.CloseNow()
		}
		return err
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	plain := httptest.NewServer(New(Options{Logger: quiet}))
	defer plain.Close()
	if dial(plain, "https://yoyo.example.ts.net") == nil {
		t.Fatal("a page on another host connected")
	}
	proxied := httptest.NewServer(New(Options{Logger: quiet, Origins: []string{"yoyo.example.ts.net"}}))
	defer proxied.Close()
	if err := dial(proxied, "https://yoyo.example.ts.net"); err != nil {
		t.Fatalf("an allowed origin: %v", err)
	}
	if dial(proxied, "https://evil.example") == nil {
		t.Fatal("an unlisted origin connected")
	}
}
