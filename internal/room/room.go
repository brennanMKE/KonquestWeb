// Package room runs game rooms: the lobby, seats, settings and the turn
// lifecycle. Each room is an actor: one goroutine owns all of its state, and
// everything else talks to it through its inbox.
package room

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	mrand "math/rand/v2"
	"slices"
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
	// AI lists the computer players hosts can add, by kind. Nil offers
	// Planner and Greedy. A seat whose kind isn't listed (say, from a saved
	// game) plays as Planner.
	AI map[string]ai.Player
	// DefaultAI is the kind added when the host doesn't name one, and the
	// lobby's first choice. Empty means Planner.
	DefaultAI string
}

type seat struct {
	name   string
	kind   string
	aiKind string
	token  string
	client *Client
	// watching is set on the host while they watch instead of playing; a
	// watching host isn't in Room.seats.
	watching bool
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

	// turnGen increments with every announced turn, so timers set for an
	// earlier turn do nothing.
	turnGen  int
	deadline int64
	idle     *time.Timer
	idleGen  int
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
	if opts.AI == nil {
		opts.AI = map[string]ai.Player{proto.AIPlanner: ai.Planner{}, proto.AIGreedy: ai.Greedy{}}
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
	if r.phase == proto.PhasePlaying {
		r.announceTurn() // a loaded game: start its computer players
	}
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
		if r.phase == proto.PhasePlaying && r.seatIndex(s) == r.game.Active() {
			r.scheduleDisconnectSkip(s)
		}
	}
}

// members are everyone with a seat or watching.
func (r *Room) members() []*seat {
	if r.host != nil && r.host.watching {
		return append(slices.Clone(r.seats), r.host)
	}
	return r.seats
}

func (r *Room) broadcastSeatStatus(s *seat) {
	if s.watching {
		return
	}
	msg := &proto.SeatStatus{Seat: r.seatIndex(s), Connected: s.client != nil}
	for c := range r.clients {
		r.send(c, msg)
	}
}

// armIdle starts the idle timer if no human is connected.
func (r *Room) armIdle() {
	for _, s := range r.members() {
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
	for _, s := range r.members() {
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
	case *proto.NewGame:
		if r.isHost(c, s) {
			r.newGame(c)
		}
	case *proto.Watch:
		if r.hostLobby(c, s) {
			r.setWatching(c, m.Watch)
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
		for _, x := range r.members() {
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
	} else if s = r.openSeat(j.Name); s != nil {
		// A loaded game: take an unclaimed seat.
		s.token = newToken()
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
	r.welcome(s)
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
	if err := checkTimers(ps); err != nil {
		r.sendError(c, proto.ErrBadSetting, "%v", err)
		return
	}
	r.settings = ps
	r.broadcastLobby()
}

func (r *Room) aiKind(kind string) (string, bool) {
	if kind == "" {
		kind = r.defaultAI()
	}
	_, ok := r.opts.AI[kind]
	return kind, ok
}

// available reports whether a kind can be added now, as last known: a
// player that depends on a service (System 1) only while it's up.
func (r *Room) available(kind string) bool {
	p, ok := r.opts.AI[kind]
	if !ok {
		return false
	}
	ch, ok := p.(ai.Checker)
	return !ok || ch.Up()
}

func (r *Room) defaultAI() string {
	for _, k := range []string{r.opts.DefaultAI, proto.AIPlanner} {
		if r.available(k) {
			return k
		}
	}
	for _, k := range slices.Sorted(maps.Keys(r.opts.AI)) {
		if r.available(k) {
			return k
		}
	}
	return proto.AIPlanner
}

// aiKinds lists the kinds on offer, the default first.
func (r *Room) aiKinds() []string {
	def := r.defaultAI()
	kinds := []string{}
	if r.available(def) {
		kinds = append(kinds, def)
	}
	for _, k := range slices.Sorted(maps.Keys(r.opts.AI)) {
		if k != def && r.available(k) {
			kinds = append(kinds, k)
		}
	}
	return kinds
}

func (r *Room) addAI(c *Client, kind string) {
	kind, ok := r.aiKind(kind)
	if !ok {
		r.sendError(c, proto.ErrBadMessage, "unknown AI kind %q", kind)
		return
	}
	if len(r.seats) >= rules.MaxPlayers {
		r.sendError(c, proto.ErrRoomFull, "the room is full")
		return
	}
	ch, checked := r.opts.AI[kind].(ai.Checker)
	if !checked {
		r.seats = append(r.seats, &seat{kind: proto.KindAI, aiKind: kind, name: r.aiName()})
		r.broadcastLobby()
		return
	}
	// Ask the service now, off the room goroutine, and add the player only
	// if it answers.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()
		err := ch.Check(ctx)
		r.post(func() {
			if r.phase != proto.PhaseLobby {
				return
			}
			if err != nil {
				r.unavailable(c, ch)
				return
			}
			if len(r.seats) >= rules.MaxPlayers {
				r.sendError(c, proto.ErrRoomFull, "the room is full")
				return
			}
			r.seats = append(r.seats, &seat{kind: proto.KindAI, aiKind: kind, name: r.aiName()})
			r.broadcastLobby()
		})
	}()
}

// checkTimeout bounds asking a computer player's service whether it's up.
const checkTimeout = 3 * time.Second

// unavailable tells the host a kind of computer can't be used now, and
// refreshes the lobby so it's no longer offered.
func (r *Room) unavailable(c *Client, p ai.Player) {
	r.sendError(c, proto.ErrAIUnavailable, "The %s model can't be reached right now, so %s players can't be added. Add a Planner instead, or try again later.", p.Name(), p.Name())
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
	r.renumber(i)
	r.broadcastLobby()
}

// welcome tells s's client its seat and whether it hosts.
func (r *Room) welcome(s *seat) {
	n := r.seatIndex(s)
	if s.watching {
		n = rules.Spectator
	}
	r.send(s.client, &proto.Welcome{Seat: n, Token: s.token, Room: r.code, Host: s == r.host})
}

// renumber re-welcomes the players from seat i on, after seats moved down.
func (r *Room) renumber(i int) {
	for j := i; j < len(r.seats); j++ {
		r.welcome(r.seats[j])
	}
}

// setWatching moves the host out of the seats to watch, or back in to play.
func (r *Room) setWatching(c *Client, watch bool) {
	h := r.host
	switch {
	case watch == h.watching:
		return
	case watch:
		i := r.seatIndex(h)
		r.seats = slices.Delete(r.seats, i, i+1)
		h.watching = true
		r.renumber(i)
	default:
		if len(r.seats) >= rules.MaxPlayers {
			r.sendError(c, proto.ErrRoomFull, "the room is full; remove a seat first")
			return
		}
		h.watching = false
		r.seats = append(r.seats, h)
	}
	r.welcome(h)
	r.broadcastLobby()
}

// allComputers reports whether no human plays, so watchers may see
// everything.
func (r *Room) allComputers() bool {
	for _, s := range r.seats {
		if s.kind == proto.KindHuman {
			return false
		}
	}
	return true
}

func (r *Room) start(c *Client) {
	// Computer players that depend on a service start only if it answers.
	var checks []ai.Checker
	for _, s := range r.seats {
		if ch, ok := r.opts.AI[s.aiKind].(ai.Checker); ok && s.kind == proto.KindAI && !slices.Contains(checks, ch) {
			checks = append(checks, ch)
		}
	}
	if len(checks) == 0 {
		r.begin(c)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()
		var failed ai.Checker
		for _, ch := range checks {
			if ch.Check(ctx) != nil {
				failed = ch
				break
			}
		}
		r.post(func() {
			if r.phase != proto.PhaseLobby {
				return
			}
			if failed != nil {
				r.sendError(c, proto.ErrAIUnavailable, "The %s model can't be reached right now. Remove its players or try again later.", failed.Name())
				r.broadcastLobby()
				return
			}
			r.begin(c)
		})
	}()
}

// newGame returns a finished game's room to the lobby. Everyone keeps their
// seat, computers included, along with the settings; the map is new.
func (r *Room) newGame(c *Client) {
	if r.phase != proto.PhaseOver {
		r.sendError(c, proto.ErrWrongPhase, "the game isn't over")
		return
	}
	r.game = nil
	r.phase = proto.PhaseLobby
	r.seed = r.opts.NewSeed()
	r.turnGen++ // stops any timer from the last game
	r.deadline = 0
	r.log.Info("back to the lobby for a new game")
	r.broadcastLobby()
}

func (r *Room) begin(c *Client) {
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
	kind, ok := r.aiKind(m.Kind)
	if !ok {
		r.sendError(c, proto.ErrBadMessage, "unknown AI kind %q", m.Kind)
		return
	}
	if !r.available(kind) {
		r.unavailable(c, r.opts.AI[kind])
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
		if h := r.host; h != nil && h.watching {
			full := r.allComputers()
			seen := events
			if !full {
				seen = rules.FilterEvents(events, rules.Spectator)
			}
			r.send(h.client, &proto.RoundResult{Round: round, Events: nonNil(seen), View: r.game.Spectate(full)})
		}
	}
	if r.game.Over() {
		r.phase = proto.PhaseOver
		r.log.Info("game over", "winner", r.game.Winner(), "round", round)
		msg := r.gameOverMsg()
		for _, s := range r.members() {
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
// A human's turn may also have a deadline (turn timer), and a disconnected
// human's turn is skipped after the grace period.
func (r *Room) announceTurn() {
	r.turnGen++
	r.deadline = 0
	s := r.seats[r.game.Active()]
	if s.kind == proto.KindHuman && r.settings.TurnTimerMs > 0 {
		d := time.Duration(r.settings.TurnTimerMs) * time.Millisecond
		r.deadline = time.Now().Add(d).UnixMilli()
		r.afterTurn(d, func() {
			r.log.Info("turn timer expired", "seat", r.game.Active())
			r.finishTurn(r.game.Active())
		})
	}
	for _, x := range r.members() {
		r.sendTurn(x)
	}
	switch {
	case s.kind == proto.KindAI:
		r.scheduleAI()
	case s.client == nil:
		r.scheduleDisconnectSkip(s)
	}
}

func (r *Room) sendTurn(s *seat) {
	i, active := r.seatIndex(s), r.game.Active()
	if s.watching {
		r.send(s.client, &proto.PlayerTurn{Round: r.game.Round(), Seat: active, Name: r.seats[active].name,
			View: r.game.Spectate(r.allComputers()), Deadline: r.deadline})
		return
	}
	if i == active {
		r.send(s.client, &proto.YourTurn{Round: r.game.Round(), View: r.game.View(i), Deadline: r.deadline})
		return
	}
	r.send(s.client, &proto.PlayerTurn{Round: r.game.Round(), Seat: active, Name: r.seats[active].name,
		View: r.game.View(i), Deadline: r.deadline})
}

// afterTurn runs f after d, but only if the same turn is still in play.
func (r *Room) afterTurn(d time.Duration, f func()) {
	gen := r.turnGen
	time.AfterFunc(d, func() {
		r.post(func() {
			if gen == r.turnGen && r.phase == proto.PhasePlaying {
				f()
			}
		})
	})
}

// scheduleAI runs the active computer player after the AI delay.
func (r *Room) scheduleAI() {
	r.afterTurn(time.Duration(r.settings.AIDelayMs)*time.Millisecond, r.runAI)
}

// scheduleDisconnectSkip ends a disconnected player's turn after the grace
// period, unless they come back first. A grace of 0 waits forever (the host
// can still hand the seat to a computer).
func (r *Room) scheduleDisconnectSkip(s *seat) {
	if r.settings.DisconnectGraceMs <= 0 {
		return
	}
	r.afterTurn(time.Duration(r.settings.DisconnectGraceMs)*time.Millisecond, func() {
		if s.client == nil && r.seatIndex(s) == r.game.Active() {
			r.log.Info("skipping disconnected player", "seat", r.game.Active())
			r.finishTurn(r.game.Active())
		}
	})
}

func (r *Room) runAI() {
	seatN := r.game.Active()
	s := r.seats[seatN]
	if s.kind != proto.KindAI {
		return
	}
	p, ok := r.opts.AI[s.aiKind]
	if !ok {
		p = ai.Planner{}
	}
	// A model-backed player can take seconds, so it decides on its own
	// goroutine from a copy of its view; the room keeps serving meanwhile.
	v, gen := r.game.View(seatN), r.turnGen
	go func() {
		orders := p.Orders(v)
		r.post(func() {
			if gen != r.turnGen || r.phase != proto.PhasePlaying {
				return
			}
			for _, o := range orders {
				if _, err := r.game.Launch(seatN, o.From, o.To, o.Ships); err != nil {
					r.log.Error("AI order rejected", "ai", p.Name(), "order", o, "err", err)
					break
				}
			}
			r.finishTurn(seatN)
		})
	}()
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
	hostName := ""
	if r.host != nil {
		hostName = r.host.name
	}
	msg := &proto.Lobby{Room: r.code, Phase: r.phase, Host: r.seatIndex(r.host), HostName: hostName,
		Seats: r.seatInfos(), Settings: r.settings, MapPreview: r.mapPreview(), AIKinds: r.aiKinds()}
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

// openSeat finds an unclaimed human seat, which only a loaded game has: the
// seat with this name if there is one, else the first.
func (r *Room) openSeat(name string) *seat {
	var first *seat
	for _, s := range r.seats {
		if s.kind != proto.KindHuman || s.token != "" {
			continue
		}
		if strings.EqualFold(s.name, cleanName(name, "")) {
			return s
		}
		if first == nil {
			first = s
		}
	}
	return first
}

// Save returns the game file for a started game: seed, settings, seats and
// the actions log. It asks the room goroutine and waits. Only the host's
// token may save while the game runs, because the log reveals every
// player's orders; once it is over anyone may.
func (r *Room) Save(token string) (*proto.SaveFile, error) {
	return r.saveFile(func() error {
		if r.phase == proto.PhaseOver || (r.host != nil && token != "" && r.host.token == token) {
			return nil
		}
		return ErrNotHost
	})
}

// Finished returns the game file of a finished game, for anyone to watch.
func (r *Room) Finished() (*proto.SaveFile, error) {
	return r.saveFile(func() error {
		if r.phase != proto.PhaseOver {
			return ErrNotOver
		}
		return nil
	})
}

func (r *Room) saveFile(allowed func() error) (*proto.SaveFile, error) {
	type result struct {
		f   *proto.SaveFile
		err error
	}
	ch := make(chan result, 1)
	if !r.post(func() {
		if r.game == nil {
			ch <- result{err: ErrNoGame}
			return
		}
		if err := allowed(); err != nil {
			ch <- result{err: err}
			return
		}
		f := &proto.SaveFile{Version: proto.SaveVersion, Seed: r.game.Seed(), Room: r.code, Round: r.game.Round(),
			Settings: r.settings, Actions: r.game.Actions()}
		for _, s := range r.seats {
			f.Seats = append(f.Seats, proto.SavedSeat{Name: s.name, Kind: s.kind, AIKind: s.aiKind})
		}
		ch <- result{f: f}
	}) {
		return nil, ErrNoRoom
	}
	res := <-ch
	return res.f, res.err
}

// Errors from Save and Load.
var (
	ErrNotHost = errors.New("only the host can save the game")
	ErrNoGame  = errors.New("the game hasn't started")
	ErrNotOver = errors.New("the game isn't over yet")
)

// load sets up a new, not yet running room from a game file: the game is
// replayed, computer seats are ready, and human seats wait to be claimed.
func (r *Room) load(f *proto.SaveFile) error {
	if f.Version != proto.SaveVersion {
		return fmt.Errorf("unsupported save version %d", f.Version)
	}
	if err := checkTimers(f.Settings); err != nil {
		return err
	}
	g, err := rules.Replay(len(f.Seats), f.Settings.Settings, f.Seed, f.Actions)
	if err != nil {
		return fmt.Errorf("replaying the game: %w", err)
	}
	for i, ss := range f.Seats {
		s := &seat{name: cleanName(ss.Name, fmt.Sprintf("Player %d", i+1)), kind: ss.Kind, aiKind: ss.AIKind}
		switch ss.Kind {
		case proto.KindHuman:
			if r.host == nil {
				r.host = s
			}
		case proto.KindAI:
		default:
			return fmt.Errorf("seat %d has unknown kind %q", i, ss.Kind)
		}
		r.seats = append(r.seats, s)
	}
	if r.host == nil {
		return errors.New("a saved game needs at least one human seat")
	}
	r.settings, r.seed, r.game = f.Settings, f.Seed, g
	r.phase = proto.PhasePlaying
	if g.Over() {
		r.phase = proto.PhaseOver
	}
	return nil
}

func checkTimers(ps proto.Settings) error {
	switch {
	case ps.AIDelayMs < 0 || ps.AIDelayMs > 10000:
		return errors.New("AI delay must be 0 to 10000 ms")
	case ps.TurnTimerMs < 0 || ps.TurnTimerMs > 3600000:
		return errors.New("turn timer must be 0 to 1 hour")
	case ps.DisconnectGraceMs < 0 || ps.DisconnectGraceMs > 600000:
		return errors.New("disconnect grace must be 0 to 10 minutes")
	}
	return nil
}
