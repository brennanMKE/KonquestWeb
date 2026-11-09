// Package room runs game rooms: the lobby, seats, settings and the turn
// lifecycle. Each room is an actor: one goroutine owns all of its state, and
// everything else talks to it through its inbox.
package room

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	mrand "math/rand/v2"
	"strings"
	"time"

	"parallax/internal/ai"
	"parallax/internal/proto"
	"parallax/internal/rules"
)

// Client is one connection's outbound queue. The room is the only writer
// and the only one that closes Out; the connection drains it.
type Client struct {
	out chan []byte
}

// clientBuffer is how many messages may queue for a slow client before the
// room drops it.
const clientBuffer = 128

// NewClient returns a client with an outbound queue.
func NewClient() *Client { return &Client{out: make(chan []byte, clientBuffer)} }

// Out is the client's outbound messages. It's closed when the room drops the
// client.
func (c *Client) Out() <-chan []byte { return c.out }

// Options configure a room.
type Options struct {
	// IdleTimeout closes the room after no human has been connected for
	// this long. 0 means 10 minutes.
	IdleTimeout time.Duration
	// NewSeed picks map seeds. Nil uses a random seed.
	NewSeed func() uint64
	// Logger receives room logs. Nil uses slog.Default().
	Logger *slog.Logger
}

type seat struct {
	name   string
	kind   string
	aiKind string
	token  string
	client *Client
}

// Room is one game room.
type Room struct {
	code   string
	opts   Options
	log    *slog.Logger
	inbox  chan func()
	done   chan struct{}
	closed bool

	clients  map[*Client]*seat
	seats    []*seat
	host     *seat
	settings proto.Settings
	seed     uint64
	phase    string
	game     *rules.Game

	aiGen   int
	idle    *time.Timer
	idleGen int
}

// New creates a room. Call Run to start it.
func New(code string, opts Options) *Room {
	if opts.IdleTimeout == 0 {
		opts.IdleTimeout = 10 * time.Minute
	}
	if opts.NewSeed == nil {
		opts.NewSeed = mrand.Uint64
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	r := &Room{
		code:     code,
		opts:     opts,
		log:      opts.Logger.With("room", code),
		inbox:    make(chan func(), 64),
		done:     make(chan struct{}),
		clients:  map[*Client]*seat{},
		settings: proto.DefaultSettings(),
		phase:    proto.PhaseLobby,
	}
	r.seed = opts.NewSeed()
	return r
}

// Code is the room's code.
func (r *Room) Code() string { return r.code }

// Done is closed when the room has shut down.
func (r *Room) Done() <-chan struct{} { return r.done }

// Run processes the inbox until the room closes. It starts the idle timer,
// so a room nobody joins goes away.
func (r *Room) Run() {
	r.armIdle()
	for !r.closed {
		f := <-r.inbox
		f()
	}
	for c := range r.clients {
		close(c.out)
	}
	close(r.done)
}

// post runs f on the room goroutine. It reports false if the room is closed.
func (r *Room) post(f func()) bool {
	select {
	case <-r.done:
		return false
	default:
	}
	select {
	case r.inbox <- f:
		return true
	case <-r.done:
		return false
	}
}

// Connect registers a new connection. It must send join before anything
// else.
func (r *Room) Connect(c *Client) bool {
	return r.post(func() { r.clients[c] = nil })
}

// Receive handles one raw message from a connection.
func (r *Room) Receive(c *Client, data []byte) bool {
	return r.post(func() { r.handle(c, data) })
}

// Leave unregisters a connection that has closed.
func (r *Room) Leave(c *Client) {
	r.post(func() { r.drop(c) })
}

// Close shuts the room down.
func (r *Room) Close() {
	r.post(func() { r.closed = true })
}

func (r *Room) send(c *Client, msg any) {
	if c == nil {
		return
	}
	if _, ok := r.clients[c]; !ok {
		return
	}
	b, err := proto.Encode(msg)
	if err != nil {
		r.log.Error("encode", "err", err)
		return
	}
	select {
	case c.out <- b:
	default:
		r.log.Warn("client too slow, dropping")
		r.drop(c)
	}
}

func (r *Room) sendError(c *Client, code, format string, args ...any) {
	r.send(c, &proto.Error{Code: code, Message: fmt.Sprintf(format, args...)})
}

// drop forgets a connection and closes its queue. If it held a seat, the
// seat shows as disconnected.
func (r *Room) drop(c *Client) {
	s, ok := r.clients[c]
	if !ok {
		return
	}
	delete(r.clients, c)
	close(c.out)
	if s != nil && s.client == c {
		s.client = nil
		r.broadcastSeatStatus(s)
		r.armIdle()
	}
}

func (r *Room) broadcastSeatStatus(s *seat) {
	msg := &proto.SeatStatus{Seat: r.seatIndex(s), Connected: s.client != nil}
	for c := range r.clients {
		r.send(c, msg)
	}
}

// armIdle starts the idle timer if no human is connected.
func (r *Room) armIdle() {
	for _, s := range r.seats {
		if s.client != nil {
			return
		}
	}
	r.idleGen++
	gen := r.idleGen
	if r.idle != nil {
		r.idle.Stop()
	}
	r.idle = time.AfterFunc(r.opts.IdleTimeout, func() {
		r.post(func() {
			if gen == r.idleGen && !r.anyConnected() {
				r.log.Info("idle, closing")
				r.closed = true
			}
		})
	})
}

func (r *Room) anyConnected() bool {
	for _, s := range r.seats {
		if s.client != nil {
			return true
		}
	}
	return false
}

func (r *Room) seatIndex(s *seat) int {
	for i, x := range r.seats {
		if x == s {
			return i
		}
	}
	return -1
}

func newToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// handle decodes and dispatches one message.
func (r *Room) handle(c *Client, data []byte) {
	s, ok := r.clients[c]
	if !ok {
		return
	}
	msg, err := proto.Decode(data)
	if err != nil {
		r.sendError(c, proto.ErrBadMessage, "%v", err)
		return
	}
	if j, ok := msg.(*proto.Join); ok {
		r.join(c, j)
		return
	}
	if s == nil {
		r.sendError(c, proto.ErrNotJoined, "send join first")
		return
	}
	switch m := msg.(type) {
	case *proto.SetSettings:
		if r.hostLobby(c, s) {
			r.setSettings(c, m.Settings)
		}
	case *proto.AddAI:
		if r.hostLobby(c, s) {
			r.addAI(c, m.Kind)
		}
	case *proto.RemoveSeat:
		if r.hostLobby(c, s) {
			r.removeSeat(c, m.Seat)
		}
	case *proto.RerollMap:
		if r.hostLobby(c, s) {
			r.seed = r.opts.NewSeed()
			r.broadcastLobby()
		}
	case *proto.StartGame:
		if r.hostLobby(c, s) {
			r.start(c)
		}
	case *proto.ReplaceWithAI:
		if r.isHost(c, s) {
			r.replaceWithAI(c, m)
		}
	case *proto.SendFleet:
		if r.playing(c) {
			r.sendFleet(c, s, m)
		}
	case *proto.EndTurn:
		if r.playing(c) {
			r.endTurn(c, s)
		}
	default:
		r.sendError(c, proto.ErrBadMessage, "%s is not a client message", proto.TypeOf(msg))
	}
}

func (r *Room) isHost(c *Client, s *seat) bool {
	if s != r.host {
		r.sendError(c, proto.ErrNotHost, "only the host can do that")
		return false
	}
	return true
}

func (r *Room) hostLobby(c *Client, s *seat) bool {
	if !r.isHost(c, s) {
		return false
	}
	if r.phase != proto.PhaseLobby {
		r.sendError(c, proto.ErrWrongPhase, "the game has started")
		return false
	}
	return true
}

func (r *Room) playing(c *Client) bool {
	if r.phase != proto.PhasePlaying {
		r.sendError(c, proto.ErrWrongPhase, "no game in progress")
		return false
	}
	return true
}

const maxNameLen = 24

func cleanName(name string, fallback string) string {
	name = strings.Join(strings.Fields(name), " ")
	if r := []rune(name); len(r) > maxNameLen {
		name = string(r[:maxNameLen])
	}
	if name == "" {
		return fallback
	}
	return name
}

func (r *Room) join(c *Client, j *proto.Join) {
	if r.clients[c] != nil {
		r.sendError(c, proto.ErrBadMessage, "already joined")
		return
	}
	var s *seat
	if j.Token != "" {
		for _, x := range r.seats {
			if x.kind == proto.KindHuman && x.token == j.Token {
				s = x
			}
		}
		if s == nil {
			r.sendError(c, proto.ErrBadToken, "that seat no longer exists")
			return
		}
		if old := s.client; old != nil {
			r.sendError(old, proto.ErrReplaced, "you joined from another window")
			r.clients[old] = nil
			r.drop(old)
		}
	} else {
		if r.phase != proto.PhaseLobby {
			r.sendError(c, proto.ErrWrongPhase, "the game has started")
			return
		}
		if len(r.seats) >= rules.MaxPlayers {
			r.sendError(c, proto.ErrRoomFull, "the room is full")
			return
		}
		s = &seat{kind: proto.KindHuman, token: newToken(),
			name: cleanName(j.Name, fmt.Sprintf("Player %d", len(r.seats)+1))}
		r.seats = append(r.seats, s)
		if r.host == nil {
			r.host = s
		}
	}
	s.client = c
	r.clients[c] = s
	r.send(c, &proto.Welcome{Seat: r.seatIndex(s), Token: s.token, Room: r.code})
	r.broadcastLobby()
	r.broadcastSeatStatus(s)
	switch r.phase {
	case proto.PhasePlaying:
		r.sendTurn(s)
	case proto.PhaseOver:
		r.send(c, r.gameOverMsg())
	}
}

func (r *Room) setSettings(c *Client, ps proto.Settings) {
	if err := ps.Settings.Validate(max(len(r.seats), rules.MinPlayers)); err != nil {
		r.sendError(c, proto.ErrBadSetting, "%v", err)
		return
	}
	if ps.AIDelayMs < 0 || ps.AIDelayMs > 10000 {
		r.sendError(c, proto.ErrBadSetting, "AI delay must be 0 to 10000 ms")
		return
	}
	r.settings = ps
	r.broadcastLobby()
}

var aiPlayers = map[string]ai.Player{proto.AIGreedy: ai.Greedy{}}

func aiKind(kind string) (string, bool) {
	if kind == "" {
		kind = proto.AIGreedy
	}
	_, ok := aiPlayers[kind]
	return kind, ok
}

func (r *Room) addAI(c *Client, kind string) {
	kind, ok := aiKind(kind)
	if !ok {
		r.sendError(c, proto.ErrBadMessage, "unknown AI kind %q", kind)
		return
	}
	if len(r.seats) >= rules.MaxPlayers {
		r.sendError(c, proto.ErrRoomFull, "the room is full")
		return
	}
	r.seats = append(r.seats, &seat{kind: proto.KindAI, aiKind: kind, name: r.aiName()})
	r.broadcastLobby()
}

func (r *Room) aiName() string {
	for n := 1; ; n++ {
		name := fmt.Sprintf("Computer %d", n)
		taken := false
		for _, s := range r.seats {
			taken = taken || s.name == name
		}
		if !taken {
			return name
		}
	}
}

func (r *Room) removeSeat(c *Client, i int) {
	if i < 0 || i >= len(r.seats) || r.seats[i] == r.host {
		r.sendError(c, proto.ErrBadSeat, "can't remove seat %d", i)
		return
	}
	s := r.seats[i]
	r.seats = append(r.seats[:i], r.seats[i+1:]...)
	if s.client != nil {
		r.sendError(s.client, proto.ErrRemoved, "the host removed you")
		r.clients[s.client] = nil
		r.drop(s.client)
	}
	// Seat numbers after i moved down; tell those players.
	for j := i; j < len(r.seats); j++ {
		if x := r.seats[j]; x.client != nil {
			r.send(x.client, &proto.Welcome{Seat: j, Token: x.token, Room: r.code})
		}
	}
	r.broadcastLobby()
}

func (r *Room) start(c *Client) {
	g, err := rules.New(len(r.seats), r.settings.Settings, r.seed)
	if err != nil {
		r.sendError(c, proto.ErrBadSetting, "%v", err)
		return
	}
	r.game = g
	r.phase = proto.PhasePlaying
	r.log.Info("game started", "seats", len(r.seats), "seed", r.seed)
	r.broadcastLobby()
	r.announceTurn()
}

func (r *Room) replaceWithAI(c *Client, m *proto.ReplaceWithAI) {
	kind, ok := aiKind(m.Kind)
	if !ok {
		r.sendError(c, proto.ErrBadMessage, "unknown AI kind %q", m.Kind)
		return
	}
	if m.Seat < 0 || m.Seat >= len(r.seats) {
		r.sendError(c, proto.ErrBadSeat, "no seat %d", m.Seat)
		return
	}
	s := r.seats[m.Seat]
	if s.kind != proto.KindHuman || s.client != nil || s == r.host {
		r.sendError(c, proto.ErrBadSeat, "only a disconnected player's seat can go to a computer")
		return
	}
	s.kind, s.aiKind, s.token = proto.KindAI, kind, ""
	r.broadcastLobby()
	if r.phase == proto.PhasePlaying && r.game.Active() == m.Seat {
		r.scheduleAI()
	}
}

func (r *Room) checkActive(c *Client, s *seat) bool {
	if r.seatIndex(s) != r.game.Active() {
		r.sendError(c, proto.ErrNotYourTurn, "it's not your turn")
		return false
	}
	return true
}

func (r *Room) sendFleet(c *Client, s *seat, m *proto.SendFleet) {
	if !r.checkActive(c, s) {
		return
	}
	seatN := r.seatIndex(s)
	f, err := r.game.Launch(seatN, m.From, m.To, m.Ships)
	if err != nil {
		r.sendError(c, proto.ErrBadOrder, "%v", err)
		return
	}
	r.send(c, &proto.FleetAck{FleetID: f.ID, ArrivalRound: f.ArrivalRound, View: r.game.View(seatN)})
}

func (r *Room) endTurn(c *Client, s *seat) {
	if !r.checkActive(c, s) {
		return
	}
	r.finishTurn(r.seatIndex(s))
}

// finishTurn ends seat's turn and tells everyone what happens next.
func (r *Room) finishTurn(seatN int) {
	round := r.game.Round()
	events, err := r.game.EndTurn(seatN)
	if err != nil {
		r.log.Error("end turn", "seat", seatN, "err", err)
		return
	}
	if events != nil {
		for i, s := range r.seats {
			r.send(s.client, &proto.RoundResult{Round: round, Events: nonNil(rules.FilterEvents(events, i)), View: r.game.View(i)})
		}
	}
	if r.game.Over() {
		r.phase = proto.PhaseOver
		r.log.Info("game over", "winner", r.game.Winner(), "round", round)
		msg := r.gameOverMsg()
		for _, s := range r.seats {
			r.send(s.client, msg)
		}
		r.broadcastLobby()
		return
	}
	r.announceTurn()
}

func nonNil(e []rules.Event) []rules.Event {
	if e == nil {
		return []rules.Event{}
	}
	return e
}

func (r *Room) gameOverMsg() *proto.GameOver {
	return &proto.GameOver{Winner: r.game.Winner(), Standings: r.game.Standings()}
}

// announceTurn tells the active player it's their turn and everyone else
// who is moving, then runs the AI if the active seat is a computer.
func (r *Room) announceTurn() {
	for _, s := range r.seats {
		r.sendTurn(s)
	}
	if r.seats[r.game.Active()].kind == proto.KindAI {
		r.scheduleAI()
	}
}

func (r *Room) sendTurn(s *seat) {
	i, active := r.seatIndex(s), r.game.Active()
	if i == active {
		r.send(s.client, &proto.YourTurn{Round: r.game.Round(), View: r.game.View(i)})
		return
	}
	r.send(s.client, &proto.PlayerTurn{Round: r.game.Round(), Seat: active, Name: r.seats[active].name, View: r.game.View(i)})
}

// scheduleAI runs the active computer player after the AI delay. A
// generation number drops stale timers.
func (r *Room) scheduleAI() {
	r.aiGen++
	gen := r.aiGen
	time.AfterFunc(time.Duration(r.settings.AIDelayMs)*time.Millisecond, func() {
		r.post(func() {
			if gen == r.aiGen && r.phase == proto.PhasePlaying {
				r.runAI()
			}
		})
	})
}

func (r *Room) runAI() {
	seatN := r.game.Active()
	s := r.seats[seatN]
	if s.kind != proto.KindAI {
		return
	}
	p := aiPlayers[s.aiKind]
	for _, o := range p.Orders(r.game.View(seatN)) {
		if _, err := r.game.Launch(seatN, o.From, o.To, o.Ships); err != nil {
			r.log.Error("AI order rejected", "ai", p.Name(), "order", o, "err", err)
			break
		}
	}
	r.finishTurn(seatN)
}

func (r *Room) seatInfos() []proto.SeatInfo {
	out := make([]proto.SeatInfo, len(r.seats))
	for i, s := range r.seats {
		out[i] = proto.SeatInfo{Seat: i, Name: s.name, Kind: s.kind, AIKind: s.aiKind,
			Connected: s.kind == proto.KindAI || s.client != nil}
	}
	return out
}

func (r *Room) broadcastLobby() {
	msg := &proto.Lobby{Room: r.code, Phase: r.phase, Host: r.seatIndex(r.host),
		Seats: r.seatInfos(), Settings: r.settings, MapPreview: r.mapPreview()}
	for c, s := range r.clients {
		if s != nil {
			r.send(c, msg)
		}
	}
}

// mapPreview shows the lobby the map the current seed and seats would make:
// positions and owners only. Nil until there are enough seats.
func (r *Room) mapPreview() []rules.PlanetView {
	if r.phase != proto.PhaseLobby {
		return nil
	}
	g, err := rules.New(max(len(r.seats), rules.MinPlayers), r.settings.Settings, r.seed)
	if err != nil {
		return nil
	}
	v := g.View(rules.Neutral - 1)
	for i := range v.Planets {
		v.Planets[i].Ships, v.Planets[i].Production, v.Planets[i].Kill = nil, nil, nil
	}
	return v.Planets
}

// ErrNoRoom is returned for an unknown room code.
var ErrNoRoom = errors.New("no such room")
