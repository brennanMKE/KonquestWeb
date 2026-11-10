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
	stateDir := flag.String("state", "", "directory to keep open rooms in across restarts (off when empty); rooms are saved there every 30 s and at shutdown, and reopened at start")
	origins := flag.String("origin", "", "comma-separated hosts whose pages may connect, for a proxy that rewrites Host (e.g. yoyo.example.ts.net)")
	flag.Parse()

	level := slog.LevelInfo
	if *dev {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	players := map[string]ai.Player{proto.AIPlanner: ai.Planner{}, proto.AIGreedy: ai.Greedy{}}
	defaultAI := proto.AIPlanner
	var health *modelHealth
	if *s1URL != "" && *s1URL != "off" {
		model := &ai.Client{URL: *s1URL, APIKey: *s1Key, Batch: *s1Batch}
		health = newModelHealth(log, *s1URL, *s1Key)
		players[proto.AISystem1] = &ai.System1{
			Model:    model,
			Fallback: ai.Planner{},
			Timeout:  *s1Timeout,
			Health:   health,
			OnError:  health.failed,
			OnAnswer: health.answered,
		}
		defaultAI = proto.AISystem1
	}

	handler := server.New(server.Options{Dev: *dev, Origins: splitList(*origins), Static: static(), Logger: log, Room: room.Options{AI: players, DefaultAI: defaultAI}})
	if health != nil {
		// Lobbies show System 1 only while the model answers.
		health.changed = handler.Rooms().RefreshLobbies
		go health.run()
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
