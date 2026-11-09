package main

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// modelHealth logs the System 1 model going down and coming back, once each
// way rather than on every turn.
type modelHealth struct {
	log  *slog.Logger
	url  string
	mu   sync.Mutex
	down bool
}

func newModelHealth(log *slog.Logger, url string) *modelHealth {
	return &modelHealth{log: log, url: url}
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

// probe reports at startup whether the model's /health answers.
func (h *modelHealth) probe(url string) {
	health := strings.TrimSuffix(url, "/predict") + "/health"
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(health)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			h.log.Info("System 1 model is up; computer players use it by default", "url", url)
			return
		}
	}
	h.failed(err)
}
