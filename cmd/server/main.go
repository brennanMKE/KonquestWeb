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
	s1Key := flag.String("system1-key", "", "API key for the System 1 endpoint (sent as a Bearer token)")
	s1Batch := flag.Bool("system1-batch", true, "send several states per request; off sends one request per state")
	origins := flag.String("origin", "", "comma-separated hosts whose pages may connect, for a proxy that rewrites Host (e.g. yoyo.example.ts.net)")
	flag.Parse()

	level := slog.LevelInfo
	if *dev {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	players := map[string]ai.Player{proto.AIPlanner: ai.Planner{}, proto.AIGreedy: ai.Greedy{}}
	defaultAI := proto.AIPlanner
	if *s1URL != "" && *s1URL != "off" {
		model := &ai.Client{URL: *s1URL, APIKey: *s1Key, Batch: *s1Batch}
		health := newModelHealth(log, *s1URL)
		players[proto.AISystem1] = &ai.System1{
			Model:    model,
			Fallback: ai.Planner{},
			OnError:  health.failed,
			OnAnswer: health.answered,
		}
		defaultAI = proto.AISystem1
		go health.probe(*s1URL)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(server.Options{Dev: *dev, Origins: splitList(*origins), Static: static(), Logger: log, Room: room.Options{AI: players, DefaultAI: defaultAI}}),
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
