// Command server runs the Parallax game server.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"parallax/internal/ai"
	"parallax/internal/proto"
	"parallax/internal/room"
	"parallax/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dev := flag.Bool("dev", false, "development mode: accept WebSockets from any origin, debug logs")
	s1URL := flag.String("system1", "", `System 1 decision endpoint for computer players, e.g. http://127.0.0.1:8799/predict (off when empty). When set, System 1 is the default computer; while it's unreachable those players play as Planner`)
	s1Key := flag.String("system1-key", os.Getenv("PARALLAX_SYSTEM1_KEY"), "API key for the System 1 endpoint, sent as a Bearer token (default $PARALLAX_SYSTEM1_KEY, which keeps it out of the process list)")
	s1Batch := flag.Bool("system1-batch", true, "send several states per request; off sends one request per state")
	s1Timeout := flag.Duration("system1-timeout", 15*time.Second, "longest wait for one System 1 decision before playing Planner that turn")
	s1Concurrency := flag.Int("system1-concurrency", 2, "most System 1 requests in flight at once, across every game")
	s1Wait := flag.Duration("system1-wait", 2*time.Second, "longest wait for a free request slot before playing Planner that turn")
	s1Games := flag.Int("system1-games", 10, "most games at once with System 1 players (0 = no limit); past it, new games can't use System 1")
	publicURL := flag.String("public-url", os.Getenv("PARALLAX_PUBLIC_URL"), `the site's address as players reach it, e.g. https://game.example.com, for link previews (default $PARALLAX_PUBLIC_URL, else worked out from each request)`)
	stateDir := flag.String("state", "", "directory to keep open rooms in across restarts (off when empty); rooms are saved there every 30 s and at shutdown, and reopened at start")
	origins := flag.String("origin", "", "comma-separated hosts whose pages may connect, for a proxy that rewrites Host (e.g. game.example.ts.net)")
	flag.Parse()

	level := slog.LevelInfo
	if *dev {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	players := map[string]ai.Player{proto.AIPlanner: ai.Planner{}, proto.AIGreedy: ai.Greedy{}}
	defaultAI := proto.AIPlanner
	var health *modelHealth
	var s1Stats *ai.Stats
	var s1Gate *ai.Gate
	if *s1URL != "" && *s1URL != "off" {
		s1Stats, s1Gate = &ai.Stats{}, ai.NewGate(*s1Concurrency, *s1Wait)
		model := &ai.Client{URL: *s1URL, APIKey: *s1Key, Batch: *s1Batch}
		health = newModelHealth(log, *s1URL, *s1Key)
		players[proto.AISystem1] = &ai.System1{
			Model:    model,
			Fallback: ai.Planner{},
			Timeout:  *s1Timeout,
			Health:   health,
			Gate:     s1Gate,
			Stats:    s1Stats,
			OnError:  health.failed,
			OnAnswer: health.answered,
		}
		defaultAI = proto.AISystem1
	}

	handler := server.New(server.Options{Dev: *dev, Origins: splitList(*origins), Static: static(), PublicURL: *publicURL, Logger: log, Room: room.Options{AI: players, DefaultAI: defaultAI, ModelGames: *s1Games}})
	if health != nil {
		// Lobbies show System 1 only while the model answers.
		health.changed = handler.Rooms().RefreshLobbies
		go health.run()
		go logModelUse(log, s1Stats, s1Gate, handler.Rooms())
	}
	if *stateDir != "" {
		if n := handler.Rooms().RestoreAll(*stateDir); n > 0 {
			log.Info("reopened saved rooms", "rooms", n, "dir", *stateDir)
		}
	}
	saveRooms := func() {
		if *stateDir == "" {
			return
		}
		n, err := handler.Rooms().SaveAll(*stateDir)
		if err != nil {
			log.Error("saving rooms", "err", err)
		}
		log.Debug("saved rooms", "rooms", n)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				saveRooms()
			case <-ctx.Done():
				shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				srv.Shutdown(shutdown)
				// Rooms keep running until the process exits: save them last,
				// so a restart brings every game back where it was.
				saveRooms()
				if *stateDir != "" {
					log.Info("saved rooms for the next start", "dir", *stateDir)
				}
				return
			}
		}
	}()

	log.Info("listening", "addr", *addr, "dev", *dev)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server", "err", err)
		os.Exit(1)
	}
	<-stopped
}

// logModelUse logs System 1 use every five minutes when it has changed:
// decisions asked and answered, fallbacks by reason, and games using it.
func logModelUse(log *slog.Logger, stats *ai.Stats, gate *ai.Gate, rooms *room.Manager) {
	var last ai.StatsSnapshot
	for range time.Tick(5 * time.Minute) {
		s := stats.Snapshot()
		if s == last {
			continue
		}
		last = s
		games, limit := rooms.ModelGames()
		log.Info("System 1 use", "asked", s.Asked, "answered", s.Answered, "fallbacks", s.Fallbacks(),
			"down", s.Down, "no_slot", s.Queued, "busy", s.Busy, "timeout", s.Timeout, "failed", s.Failed,
			"in_flight", gate.InFlight(), "games", games, "game_limit", limit)
	}
}

// splitList splits a comma-separated flag, dropping empty entries.
func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
