package room

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"parallax/internal/proto"
	"parallax/internal/rules"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newRoom(t *testing.T, opts Options) *Room {
	t.Helper()
	if opts.NewSeed == nil {
		var n uint64
		opts.NewSeed = func() uint64 { n++; return n }
	}
	opts.Logger = quietLogger()
	r := New("TEST", opts)
	go r.Run()
	t.Cleanup(r.Close)
	return r
}

// sync waits until the room has processed everything posted so far.
func (r *Room) sync() {
	ch := make(chan struct{})
	if r.post(func() { close(ch) }) {
		<-ch
	}
}

type testClient struct {
	t    *testing.T
	r    *Room
	c    *Client
	seen []any
}

func connect(t *testing.T, r *Room) *testClient {
	t.Helper()
	c := NewClient()
	if !r.Connect(c) {
		t.Fatal("room closed")
	}
	return &testClient{t: t, r: r, c: c}
}

func (tc *testClient) send(msg any) {
	tc.t.Helper()
	b, err := proto.Encode(msg)
	if err != nil {
		tc.t.Fatal(err)
	}
	tc.r.Receive(tc.c, b)
}

// next returns the next message, failing after a timeout. It returns nil if
// the client was dropped.
func (tc *testClient) next() any {
	tc.t.Helper()
	select {
	case b, ok := <-tc.c.Out():
		if !ok {
			return nil
		}
		msg, err := proto.Decode(b)
		if err != nil {
			tc.t.Fatal(err)
		}
		tc.seen = append(tc.seen, msg)
		return msg
	case <-time.After(2 * time.Second):
		tc.t.Fatal("timed out waiting for a message")
		return nil
	}
}

// expect skips messages until one of type T arrives.
func expect[T any](tc *testClient) *T {
	tc.t.Helper()
	for {
		msg := tc.next()
		if msg == nil {
			var zero T
			tc.t.Fatalf("client dropped while waiting for %T", zero)
		}
		if m, ok := msg.(*T); ok {
			return m
		}
	}
}

// pending returns the messages already queued, without waiting.
func (tc *testClient) pending() []any {
	tc.r.sync()
	var out []any
	for {
		select {
		case b, ok := <-tc.c.Out():
			if !ok {
				return out
			}
			msg, _ := proto.Decode(b)
			tc.seen = append(tc.seen, msg)
			out = append(out, msg)
		default:
			return out
		}
	}
}

func (tc *testClient) closed() bool {
	tc.r.sync()
	for {
		select {
		case _, ok := <-tc.c.Out():
			if !ok {
				return true
			}
		default:
			return false
		}
	}
}

func join(t *testing.T, r *Room, name string) (*testClient, *proto.Welcome) {
	t.Helper()
	tc := connect(t, r)
	tc.send(&proto.Join{Name: name})
	w := expect[proto.Welcome](tc)
	expect[proto.Lobby](tc)
	return tc, w
}

func errCode(t *testing.T, tc *testClient) string {
	t.Helper()
	return expect[proto.Error](tc).Code
}

func TestLobby(t *testing.T) {
	r := newRoom(t, Options{})
	alice, wa := join(t, r, "  Alice   Smith ")
	bob, wb := join(t, r, "")
	if wa.Seat != 0 || wb.Seat != 1 || wa.Token == "" || wa.Token == wb.Token || wa.Room != "TEST" {
		t.Fatalf("welcomes: %+v %+v", wa, wb)
	}
	l := expect[proto.Lobby](alice)
	if l.Host != 0 || len(l.Seats) != 2 || l.Seats[0].Name != "Alice Smith" || l.Seats[1].Name != "Player 2" ||
		l.Phase != proto.PhaseLobby || len(l.MapPreview) != 12 {
		t.Fatalf("lobby: %+v", l)
	}
	for _, p := range l.MapPreview {
		if p.Ships != nil || p.Production != nil || p.Kill != nil {
			t.Fatal("map preview leaks stats")
		}
	}

	// Host-only actions.
	for _, msg := range []any{&proto.AddAI{}, &proto.RerollMap{}, &proto.StartGame{},
		&proto.SetSettings{Settings: proto.DefaultSettings()}, &proto.RemoveSeat{Seat: 0}} {
		bob.send(msg)
		if code := errCode(t, bob); code != proto.ErrNotHost {
			t.Fatalf("%T from guest: %s", msg, code)
		}
	}

	alice.send(&proto.AddAI{Kind: "chess"})
	if code := errCode(t, alice); code != proto.ErrBadMessage {
		t.Fatalf("unknown AI kind: %s", code)
	}
	alice.send(&proto.AddAI{})
	l = expect[proto.Lobby](alice)
	if len(l.Seats) != 3 || l.Seats[2].Kind != proto.KindAI || l.Seats[2].AIKind != proto.AIGreedy || l.Seats[2].Name != "Computer 1" {
		t.Fatalf("after add_ai: %+v", l.Seats)
	}

	bad := proto.DefaultSettings()
	bad.Width = 2
	alice.send(&proto.SetSettings{Settings: bad})
	if code := errCode(t, alice); code != proto.ErrBadSetting {
		t.Fatalf("bad settings: %s", code)
	}
	good := proto.DefaultSettings()
	good.Depth, good.BlindMap, good.AIDelayMs = 3, true, 0
	alice.send(&proto.SetSettings{Settings: good})
	l = expect[proto.Lobby](alice)
	if l.Settings != good {
		t.Fatalf("settings = %+v", l.Settings)
	}

	before := l.MapPreview
	alice.send(&proto.RerollMap{})
	l = expect[proto.Lobby](alice)
	if l.MapPreview[0] == before[0] && l.MapPreview[1] == before[1] {
		t.Fatal("reroll didn't change the map")
	}

	alice.send(&proto.RemoveSeat{Seat: 0})
	if code := errCode(t, alice); code != proto.ErrBadSeat {
		t.Fatalf("host removing self: %s", code)
	}
	alice.send(&proto.RemoveSeat{Seat: 1})
	if code := errCode(t, bob); code != proto.ErrRemoved {
		t.Fatalf("removed bob: %s", code)
	}
	if !bob.closed() {
		t.Fatal("removed client should be closed")
	}
	l = expect[proto.Lobby](alice)
	if len(l.Seats) != 2 || l.Seats[1].Kind != proto.KindAI {
		t.Fatalf("after remove: %+v", l.Seats)
	}
}

func TestJoinErrors(t *testing.T) {
	r := newRoom(t, Options{})
	c := connect(t, r)
	c.send(&proto.EndTurn{})
	if code := errCode(t, c); code != proto.ErrNotJoined {
		t.Fatalf("before join: %s", code)
	}
	r.Receive(c.c, []byte("garbage"))
	if code := errCode(t, c); code != proto.ErrBadMessage {
		t.Fatalf("garbage: %s", code)
	}
	c.send(&proto.Join{Token: "nope"})
	if code := errCode(t, c); code != proto.ErrBadToken {
		t.Fatalf("bad token: %s", code)
	}
	c.send(&proto.Join{Name: "A"})
	expect[proto.Welcome](c)
	c.send(&proto.Join{Name: "A"})
	if code := errCode(t, c); code != proto.ErrBadMessage {
		t.Fatalf("double join: %s", code)
	}
	c.send(&proto.Welcome{})
	if code := errCode(t, c); code != proto.ErrBadMessage {
		t.Fatalf("server message from client: %s", code)
	}
	c.send(&proto.EndTurn{})
	if code := errCode(t, c); code != proto.ErrWrongPhase {
		t.Fatalf("end turn in lobby: %s", code)
	}
	c.send(&proto.StartGame{})
	if code := errCode(t, c); code != proto.ErrBadSetting {
		t.Fatalf("start with one seat: %s", code)
	}
}

func TestRoomFull(t *testing.T) {
	r := newRoom(t, Options{})
	host, _ := join(t, r, "Host")
	for range rules.MaxPlayers - 1 {
		host.send(&proto.AddAI{})
	}
	host.send(&proto.AddAI{})
	if code := errCode(t, host); code != proto.ErrRoomFull {
		t.Fatalf("add AI to full room: %s", code)
	}
	c := connect(t, r)
	c.send(&proto.Join{Name: "Late"})
	if code := errCode(t, c); code != proto.ErrRoomFull {
		t.Fatalf("join full room: %s", code)
	}
}

// startTwo starts a game between two humans with the given settings.
func startTwo(t *testing.T, s proto.Settings) (*Room, *testClient, *testClient) {
	t.Helper()
	r := newRoom(t, Options{})
	alice, _ := join(t, r, "Alice")
	bob, _ := join(t, r, "Bob")
	alice.send(&proto.SetSettings{Settings: s})
	alice.send(&proto.StartGame{})
	return r, alice, bob
}

func homeOf(v rules.View) rules.PlanetView {
	for _, p := range v.Planets {
		if p.Owner == v.Seat {
			return p
		}
	}
	panic("no home")
}

func TestTurns(t *testing.T) {
	s := proto.DefaultSettings()
	s.BlindMap = true
	_, alice, bob := startTwo(t, s)

	yt := expect[proto.YourTurn](alice)
	pt := expect[proto.PlayerTurn](bob)
	if yt.Round != 1 || yt.View.Seat != 0 || pt.Seat != 0 || pt.Name != "Alice" || pt.View.Seat != 1 {
		t.Fatalf("turn messages: %+v / %+v", yt, pt)
	}
	home := homeOf(yt.View)
	bobHome := homeOf(pt.View)

	bob.send(&proto.SendFleet{From: bobHome.ID, To: home.ID, Ships: 1})
	if code := errCode(t, bob); code != proto.ErrNotYourTurn {
		t.Fatalf("order from waiting player: %s", code)
	}
	bob.send(&proto.EndTurn{})
	if code := errCode(t, bob); code != proto.ErrNotYourTurn {
		t.Fatalf("end turn from waiting player: %s", code)
	}
	alice.send(&proto.SendFleet{From: bobHome.ID, To: home.ID, Ships: 1})
	if code := errCode(t, alice); code != proto.ErrBadOrder {
		t.Fatalf("order from enemy planet: %s", code)
	}

	alice.send(&proto.SendFleet{From: home.ID, To: bobHome.ID, Ships: 4})
	ack := expect[proto.FleetAck](alice)
	if ack.FleetID != 0 || len(ack.View.Fleets) != 1 || *homeOf(ack.View).Ships != 6 {
		t.Fatalf("ack: %+v", ack)
	}
	if got := bob.pending(); len(got) != 0 {
		t.Fatalf("waiting player got messages during the active turn: %#v", got)
	}

	alice.send(&proto.EndTurn{})
	expect[proto.PlayerTurn](alice)
	yt = expect[proto.YourTurn](bob)
	if yt.Round != 1 || len(yt.View.Fleets) != 0 {
		t.Fatalf("bob's turn: %+v", yt)
	}
	bob.send(&proto.EndTurn{})
	for _, c := range []*testClient{alice, bob} {
		rr := expect[proto.RoundResult](c)
		if rr.Round != 1 || rr.Events == nil {
			t.Fatalf("round result: %+v", rr)
		}
	}
	yt = expect[proto.YourTurn](alice)
	if yt.Round != 2 {
		t.Fatalf("round 2 turn: %+v", yt)
	}
	expect[proto.PlayerTurn](bob)

	// Nothing bob received may reveal alice's fleets or, on a blind map,
	// alice's ship counts.
	for _, msg := range bob.seen {
		var v *rules.View
		switch m := msg.(type) {
		case *proto.YourTurn:
			v = &m.View
		case *proto.PlayerTurn:
			v = &m.View
		case *proto.RoundResult:
			v = &m.View
		case *proto.FleetAck:
			t.Fatal("bob got a fleet ack")
		}
		if v == nil {
			continue
		}
		for _, f := range v.Fleets {
			if f.Owner != 1 {
				t.Fatalf("bob sees fleet %+v", f)
			}
		}
		for _, p := range v.Planets {
			if p.Owner != 1 && p.Ships != nil {
				t.Fatalf("bob sees ships on %+v", p)
			}
		}
	}
}

func TestAISeatsAndGameOver(t *testing.T) {
	r := newRoom(t, Options{})
	alice, _ := join(t, r, "Alice")
	alice.send(&proto.AddAI{})
	alice.send(&proto.AddAI{})
	s := proto.DefaultSettings()
	s.AIDelayMs, s.MaxRounds = 0, 3
	alice.send(&proto.SetSettings{Settings: s})
	alice.send(&proto.StartGame{})
	for round := 1; round <= 3; round++ {
		yt := expect[proto.YourTurn](alice)
		if yt.Round != round {
			t.Fatalf("round %d: got your_turn for %d", round, yt.Round)
		}
		alice.send(&proto.EndTurn{})
		for _, seat := range []int{1, 2} {
			if pt := expect[proto.PlayerTurn](alice); pt.Seat != seat || pt.Round != round {
				t.Fatalf("round %d: player_turn %+v", round, pt)
			}
		}
		if rr := expect[proto.RoundResult](alice); rr.Round != round {
			t.Fatalf("round result %+v", rr)
		}
	}
	over := expect[proto.GameOver](alice)
	if len(over.Standings) != 3 {
		t.Fatalf("game over: %+v", over)
	}
	if l := expect[proto.Lobby](alice); l.Phase != proto.PhaseOver {
		t.Fatalf("phase %s", l.Phase)
	}
	alice.send(&proto.EndTurn{})
	if code := errCode(t, alice); code != proto.ErrWrongPhase {
		t.Fatalf("end turn after game over: %s", code)
	}
	// A late reconnect gets the result.
	c := connect(t, r)
	c.send(&proto.Join{Token: expectToken(t, alice)})
	expect[proto.GameOver](c)
}

func expectToken(t *testing.T, tc *testClient) string {
	t.Helper()
	for _, m := range tc.seen {
		if w, ok := m.(*proto.Welcome); ok {
			return w.Token
		}
	}
	t.Fatal("no welcome seen")
	return ""
}

func TestReconnect(t *testing.T) {
	r, alice, bob := startTwo(t, proto.DefaultSettings())
	expect[proto.YourTurn](alice)
	expect[proto.PlayerTurn](bob)
	token := expectToken(t, bob)

	r.Leave(bob.c)
	if st := expect[proto.SeatStatus](alice); st.Seat != 1 || st.Connected {
		t.Fatalf("seat status: %+v", st)
	}
	late := connect(t, r)
	late.send(&proto.Join{Name: "Eve"})
	if code := errCode(t, late); code != proto.ErrWrongPhase {
		t.Fatalf("new player mid-game: %s", code)
	}

	bob2 := connect(t, r)
	bob2.send(&proto.Join{Token: token})
	if w := expect[proto.Welcome](bob2); w.Seat != 1 || w.Token != token {
		t.Fatalf("rejoin welcome: %+v", w)
	}
	if l := expect[proto.Lobby](bob2); l.Phase != proto.PhasePlaying || l.MapPreview != nil {
		t.Fatalf("rejoin lobby: %+v", l)
	}
	if pt := expect[proto.PlayerTurn](bob2); pt.Seat != 0 {
		t.Fatalf("rejoin turn: %+v", pt)
	}
	if st := expect[proto.SeatStatus](alice); st.Seat != 1 || !st.Connected {
		t.Fatalf("seat status: %+v", st)
	}

	// Joining again from another window replaces the old connection.
	bob3 := connect(t, r)
	bob3.send(&proto.Join{Token: token})
	if code := errCode(t, bob2); code != proto.ErrReplaced {
		t.Fatalf("old window: %s", code)
	}
	if !bob2.closed() {
		t.Fatal("old window should be closed")
	}
	expect[proto.Welcome](bob3)

	alice.send(&proto.EndTurn{})
	expect[proto.YourTurn](bob3)
	// Reconnecting during your own turn resends your_turn.
	r.Leave(bob3.c)
	bob4 := connect(t, r)
	bob4.send(&proto.Join{Token: token})
	expect[proto.YourTurn](bob4)
}

func TestReplaceWithAI(t *testing.T) {
	r, alice, bob := startTwo(t, func() proto.Settings {
		s := proto.DefaultSettings()
		s.AIDelayMs = 0
		return s
	}())
	expect[proto.YourTurn](alice)
	alice.send(&proto.ReplaceWithAI{Seat: 1})
	if code := errCode(t, alice); code != proto.ErrBadSeat {
		t.Fatalf("replacing a connected player: %s", code)
	}
	alice.send(&proto.ReplaceWithAI{Seat: 0})
	if code := errCode(t, alice); code != proto.ErrBadSeat {
		t.Fatalf("replacing the host: %s", code)
	}
	alice.send(&proto.EndTurn{})
	expect[proto.YourTurn](bob)
	r.Leave(bob.c)
	expect[proto.SeatStatus](alice)
	alice.send(&proto.ReplaceWithAI{Seat: 1})
	if l := expect[proto.Lobby](alice); l.Seats[1].Kind != proto.KindAI {
		t.Fatalf("seat 1: %+v", l.Seats[1])
	}
	// The computer finishes bob's turn and the round moves on.
	expect[proto.RoundResult](alice)
	if yt := expect[proto.YourTurn](alice); yt.Round != 2 {
		t.Fatalf("round %d", yt.Round)
	}
	// Bob's old token no longer works.
	c := connect(t, r)
	c.send(&proto.Join{Token: expectToken(t, bob)})
	if code := errCode(t, c); code != proto.ErrBadToken {
		t.Fatalf("old token: %s", code)
	}
}

func TestSlowClientDropped(t *testing.T) {
	r := newRoom(t, Options{})
	c := connect(t, r)
	for range clientBuffer + 10 {
		r.Receive(c.c, []byte("garbage"))
	}
	r.sync()
	n := 0
	for range c.c.Out() {
		n++
	}
	if n != clientBuffer {
		t.Fatalf("got %d messages before close, want %d", n, clientBuffer)
	}
}

func TestIdleRoomCloses(t *testing.T) {
	r := newRoom(t, Options{IdleTimeout: 20 * time.Millisecond})
	a, _ := join(t, r, "A")
	time.Sleep(50 * time.Millisecond)
	select {
	case <-r.Done():
		t.Fatal("room closed while a player was connected")
	default:
	}
	r.Leave(a.c)
	select {
	case <-r.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("idle room didn't close")
	}
	if r.Connect(NewClient()) {
		t.Fatal("connect to closed room succeeded")
	}
}

func TestManager(t *testing.T) {
	m := NewManager(Options{IdleTimeout: 20 * time.Millisecond, Logger: quietLogger()})
	r := m.Create()
	if len(r.Code()) != CodeLen {
		t.Fatalf("code %q", r.Code())
	}
	if got, ok := m.Get(string([]rune(r.Code()))); !ok || got != r {
		t.Fatal("Get failed")
	}
	if _, ok := m.Get("zzzz"); ok {
		t.Fatal("found missing room")
	}
	<-r.Done()
	deadline := time.Now().Add(2 * time.Second)
	for m.Len() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("closed room not removed")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTurnTimer(t *testing.T) {
	s := proto.DefaultSettings()
	s.TurnTimerMs = 60
	start := time.Now()
	_, alice, bob := startTwo(t, s)
	yt := expect[proto.YourTurn](alice)
	if pt := expect[proto.PlayerTurn](bob); pt.Deadline != yt.Deadline {
		t.Fatalf("deadlines differ: %d vs %d", pt.Deadline, yt.Deadline)
	}
	if yt.Deadline < start.UnixMilli()+50 || yt.Deadline > time.Now().UnixMilli()+70 {
		t.Fatalf("deadline %d is off", yt.Deadline)
	}
	home := homeOf(yt.View)
	alice.send(&proto.SendFleet{From: home.ID, To: (home.ID + 1) % len(yt.View.Planets), Ships: 3})
	expect[proto.FleetAck](alice)

	// Alice never ends her turn; the timer does, keeping her fleet.
	expect[proto.YourTurn](bob)
	rr := expect[proto.RoundResult](alice) // bob's timer runs out too
	if rr.Round != 1 || len(rr.View.Fleets)+len(rr.Events) == 0 {
		t.Fatalf("alice's fleet was lost: %+v", rr)
	}
	if yt := expect[proto.YourTurn](alice); yt.Round != 2 || yt.Deadline == 0 {
		t.Fatalf("round 2: %+v", yt)
	}
}

func TestDisconnectSkip(t *testing.T) {
	s := proto.DefaultSettings()
	s.DisconnectGraceMs = 40
	r, alice, bob := startTwo(t, s)
	expect[proto.YourTurn](alice)

	// Bob leaves while waiting; his turn is skipped when it comes up.
	r.Leave(bob.c)
	expect[proto.SeatStatus](alice)
	alice.send(&proto.EndTurn{})
	if pt := expect[proto.PlayerTurn](alice); pt.Seat != 1 {
		t.Fatalf("player turn %+v", pt)
	}
	expect[proto.RoundResult](alice)
	expect[proto.YourTurn](alice)

	// Bob comes back, then leaves during his own turn.
	bob2 := connect(t, r)
	bob2.send(&proto.Join{Token: expectToken(t, bob)})
	expect[proto.PlayerTurn](bob2)
	alice.send(&proto.EndTurn{})
	expect[proto.YourTurn](bob2)
	r.Leave(bob2.c)
	expect[proto.RoundResult](alice)
	if yt := expect[proto.YourTurn](alice); yt.Round != 3 {
		t.Fatalf("round %d", yt.Round)
	}
}

func TestDisconnectWaitsWithoutGrace(t *testing.T) {
	r, alice, bob := startTwo(t, proto.DefaultSettings())
	expect[proto.YourTurn](alice)
	alice.send(&proto.EndTurn{})
	expect[proto.YourTurn](bob)
	alice.pending()
	r.Leave(bob.c)
	time.Sleep(100 * time.Millisecond)
	for _, m := range alice.pending() {
		if _, ok := m.(*proto.RoundResult); ok {
			t.Fatal("turn skipped with no grace period")
		}
	}
}

func TestTimerSettingsValidated(t *testing.T) {
	r := newRoom(t, Options{})
	alice, _ := join(t, r, "A")
	for _, mod := range []func(*proto.Settings){
		func(s *proto.Settings) { s.TurnTimerMs = -1 },
		func(s *proto.Settings) { s.TurnTimerMs = 3600001 },
		func(s *proto.Settings) { s.DisconnectGraceMs = -1 },
		func(s *proto.Settings) { s.DisconnectGraceMs = 600001 },
		func(s *proto.Settings) { s.AIDelayMs = 10001 },
	} {
		s := proto.DefaultSettings()
		mod(&s)
		alice.send(&proto.SetSettings{Settings: s})
		if code := errCode(t, alice); code != proto.ErrBadSetting {
			t.Fatalf("settings %+v: %s", s, code)
		}
	}
}
