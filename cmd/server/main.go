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
	s1URL := flag.String("system1", "", "System 1 decision endpoint for computer players, e.g. http://127.0.0.1:8000/predict")
	s1Key := flag.String("system1-key", "", "API key for the System 1 endpoint (sent as a Bearer token)")
	s1Batch := flag.Bool("system1-batch", true, "send several states per request; off sends one request per state")
	flag.Parse()

	level := slog.LevelInfo
	if *dev {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	players := map[string]ai.Player{proto.AIGreedy: ai.Greedy{}}
	if *s1URL != "" {
		players[proto.AISystem1] = &ai.System1{
			Model:    &ai.Client{URL: *s1URL, APIKey: *s1Key, Batch: *s1Batch},
			Fallback: ai.Greedy{},
			OnError:  func(err error) { log.Warn("System 1 model failed; playing Greedy this turn", "err", err) },
		}
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(server.Options{Dev: *dev, Static: static(), Logger: log, Room: room.Options{AI: players}}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	log.Info("listening", "addr", *addr, "dev", *dev)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}
