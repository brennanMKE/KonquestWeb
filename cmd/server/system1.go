package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// modelHealth tracks whether the System 1 model answers. While it's down,
// computer players skip it and play Planner at once rather than waiting out
// a timeout every turn; a probe of its /health brings it back. Changes are
// logged once each way.
type modelHealth struct {
	log    *slog.Logger
	url    string
	key    string
	every  time.Duration
	client *http.Client

	// changed, if set, runs when the model goes down or comes back.
	changed func()

	mu   sync.Mutex
	down bool
}

func newModelHealth(log *slog.Logger, url, key string) *modelHealth {
	return &modelHealth{log: log, url: url, key: key, every: 15 * time.Second,
		client: &http.Client{Timeout: 5 * time.Second}}
}

// Up reports whether players should ask the model.
func (h *modelHealth) Up() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.down
}

func (h *modelHealth) failed(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.down {
		h.down = true
		h.log.Warn("System 1 model unreachable; computer players use Planner until it's back", "url", h.url, "err", err)
		h.notify()
	}
}

func (h *modelHealth) notify() {
	if h.changed != nil {
		go h.changed()
	}
}

func (h *modelHealth) answered() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.down {
		h.down = false
		h.log.Info("System 1 model is back", "url", h.url)
		h.notify()
	}
}

// run checks the model at startup and then every few seconds, so the lobby
// offers System 1 players only while the model answers.
func (h *modelHealth) run() {
	if h.Check(context.Background()) == nil {
		h.log.Info("System 1 model is up; computer players use it by default", "url", h.url)
	}
	for range time.Tick(h.every) {
		h.Check(context.Background())
	}
}

// Check asks the model's /health now.
func (h *modelHealth) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(h.url, "/predict")+"/health", nil)
	if err != nil {
		return err
	}
	if h.key != "" {
		req.Header.Set("Authorization", "Bearer "+h.key)
	}
	resp, err := h.client.Do(req)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = &statusError{resp.Status}
		}
	}
	if err != nil {
		h.failed(err)
		return err
	}
	h.answered()
	return nil
}

type statusError struct{ status string }

func (e *statusError) Error() string { return "health check: " + e.status }
