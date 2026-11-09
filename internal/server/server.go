// Package server serves the HTTP API and the WebSocket endpoint, and
// connects each WebSocket to its room.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"parallax/internal/room"
)

// Options configure the server.
type Options struct {
	// Dev accepts WebSocket connections from any origin, for a frontend dev
	// server on another port.
	Dev bool
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
	s.mux.HandleFunc("GET /ws", s.websocket)
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

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	rm, ok := s.rooms.Get(r.URL.Query().Get("room"))
	if !ok {
		http.Error(w, "no such room", http.StatusNotFound)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: s.opts.Dev})
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
