package main

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestModelHealth(t *testing.T) {
	var ok atomic.Bool
	ok.Store(true)
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" || r.Header.Get("Authorization") != "Bearer k" || !ok.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
		}
	}))
	defer model.Close()
	h := newModelHealth(slog.New(slog.NewTextHandler(io.Discard, nil)), model.URL+"/predict", "k")

	if err := h.check(); err != nil || !h.up() {
		t.Fatalf("healthy model: up=%v err=%v", h.up(), err)
	}
	h.failed(errors.New("timeout"))
	if h.up() {
		t.Fatal("still up after a failed request")
	}
	ok.Store(false)
	if h.check() == nil || h.up() {
		t.Fatal("a failing health check brought it back")
	}
	ok.Store(true)
	if err := h.check(); err != nil || !h.up() {
		t.Fatalf("recovered model: up=%v err=%v", h.up(), err)
	}
	model.Close()
	if h.check() == nil || h.up() {
		t.Fatal("unreachable model counted as up")
	}
}
