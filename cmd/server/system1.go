package main

import (
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

	mu   sync.Mutex
	down bool
}

func newModelHealth(log *slog.Logger, url, key string) *modelHealth {
	return &modelHealth{log: log, url: url, key: key, every: 15 * time.Second,
		client: &http.Client{Timeout: 5 * time.Second}}
}

// up reports whether players should ask the model.
func (h *modelHealth) up() bool {
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
	}
}

func (h *modelHealth) answered() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.down {
		h.down = false
		h.log.Info("System 1 model is back", "url", h.url)
	}
}

// run checks the model at startup, then keeps checking while it's down.
func (h *modelHealth) run() {
	if h.check() == nil {
		h.log.Info("System 1 model is up; computer players use it by default", "url", h.url)
	}
	for range time.Tick(h.every) {
		if h.up() {
			continue // Failed requests mark it down.
		}
		h.check()
	}
}

func (h *modelHealth) check() error {
	req, err := http.NewRequest(http.MethodGet, strings.TrimSuffix(h.url, "/predict")+"/health", nil)
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
