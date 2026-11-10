// Package server serves the HTTP API and the WebSocket endpoint, and
// connects each WebSocket to its room.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"parallax/internal/proto"
	"parallax/internal/room"
)

// Options configure the server.
type Options struct {
	// Dev accepts WebSocket connections from any origin, for a frontend dev
	// server on another port.
	Dev bool
	// Origins are extra host patterns (path.Match syntax, e.g.
	// "yoyo.example.ts.net") whose pages may open WebSockets, for proxies
	// that don't pass the browser's Host header through. The page's own host
	// is always allowed.
	Origins []string
	// Static serves the built frontend at /. Nil serves a placeholder.
	Static fs.FS
	// Room options for every room.
	Room   room.Options
	Logger *slog.Logger
}

// Limits for a connection.
const (
	readLimit    = 16 << 10
	writeTimeout = 10 * time.Second
	pingInterval = 30 * time.Second
)

// Server is the HTTP handler.
type Server struct {
	opts  Options
	log   *slog.Logger
	rooms *room.Manager
	mux   *http.ServeMux
	demos demoCache
}

// New returns a server.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Room.Logger == nil {
		opts.Room.Logger = opts.Logger
	}
	s := &Server{opts: opts, log: opts.Logger, rooms: room.NewManager(opts.Room), mux: http.NewServeMux()}
	s.mux.HandleFunc("POST /api/rooms", s.createRoom)
	s.mux.HandleFunc("GET /api/rooms/{code}", s.getRoom)
	s.mux.HandleFunc("GET /api/rooms/{code}/save", s.saveRoom)
	s.mux.HandleFunc("GET /api/rooms/{code}/replay", s.roomReplay)
	s.mux.HandleFunc("POST /api/load", s.loadRoom)
	s.mux.HandleFunc("POST /api/replay", s.fileReplay)
	s.mux.HandleFunc("GET /api/demo", s.demo)
	s.mux.HandleFunc("GET /ws", s.websocket)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ok\n"))
	})
	if opts.Static != nil {
		s.mux.Handle("GET /", http.FileServerFS(opts.Static))
	} else {
		s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("Parallax server is running. The web client isn't built yet.\n"))
		})
	}
	return s
}

// Rooms is the server's room manager.
func (s *Server) Rooms() *room.Manager { return s.rooms }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	rm := s.rooms.Create()
	s.log.Info("room created", "room", rm.Code())
	writeJSON(w, http.StatusCreated, map[string]string{"code": rm.Code()})
}

func (s *Server) getRoom(w http.ResponseWriter, r *http.Request) {
	rm, ok := s.rooms.Get(r.PathValue("code"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such room"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": rm.Code()})
}

// saveRoom downloads the game file. The host's token is required, because
// the actions log reveals every player's orders.
func (s *Server) saveRoom(w http.ResponseWriter, r *http.Request) {
	rm, ok := s.rooms.Get(r.PathValue("code"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such room"})
		return
	}
	f, err := rm.Save(r.URL.Query().Get("token"))
	switch {
	case errors.Is(err, room.ErrNotHost):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	case err != nil:
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="parallax-%s-round-%d.json"`,
		rm.Code(), f.Round))
	writeJSON(w, http.StatusOK, f)
}

// roomReplay sends a finished game's every round, for watching it back.
func (s *Server) roomReplay(w http.ResponseWriter, r *http.Request) {
	rm, ok := s.rooms.Get(r.PathValue("code"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such room"})
		return
	}
	f, err := rm.Finished()
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.sendReplay(w, f)
}

// fileReplay sends the rounds of a game file, without opening a room.
func (s *Server) fileReplay(w http.ResponseWriter, r *http.Request) {
	var f proto.SaveFile
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&f); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not a game file: " + err.Error()})
		return
	}
	s.sendReplay(w, &f)
}

func (s *Server) sendReplay(w http.ResponseWriter, f *proto.SaveFile) {
	rp, err := proto.BuildReplay(f)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rp)
}

// loadRoom starts a new room from a game file and returns its code.
func (s *Server) loadRoom(w http.ResponseWriter, r *http.Request) {
	var f proto.SaveFile
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&f); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not a game file: " + err.Error()})
		return
	}
	rm, err := s.rooms.Load(&f)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.log.Info("room loaded", "room", rm.Code(), "actions", len(f.Actions))
	writeJSON(w, http.StatusCreated, map[string]string{"code": rm.Code()})
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	rm, ok := s.rooms.Get(r.URL.Query().Get("room"))
	if !ok {
		http.Error(w, "no such room", http.StatusNotFound)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: s.opts.Dev, OriginPatterns: s.opts.Origins})
	if err != nil {
		return // Accept has written the response.
	}
	conn.SetReadLimit(readLimit)
	c := room.NewClient()
	if !rm.Connect(c) {
		conn.Close(websocket.StatusGoingAway, "room closed")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go s.writeLoop(ctx, cancel, conn, c, rm)

	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			if status := websocket.CloseStatus(err); status == -1 && !errors.Is(err, context.Canceled) {
				s.log.Debug("read", "room", rm.Code(), "err", err)
			}
			break
		}
		if typ != websocket.MessageText || !rm.Receive(c, data) {
			break
		}
	}
	rm.Leave(c)
	conn.CloseNow()
}

// writeLoop sends the room's messages to the socket until the room drops the
// client or closes, and pings to keep idle connections alive.
func (s *Server) writeLoop(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, c *room.Client, rm *room.Room) {
	defer cancel()
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case msg, ok := <-c.Out():
			if !ok {
				conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			wctx, wcancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Write(wctx, websocket.MessageText, msg)
			wcancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		case <-rm.Done():
			conn.Close(websocket.StatusGoingAway, "room closed")
			return
		case <-ctx.Done():
			return
		}
	}
}
