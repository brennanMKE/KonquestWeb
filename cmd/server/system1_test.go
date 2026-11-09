package main

import (
	"context"
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

	if err := h.Check(context.Background()); err != nil || !h.Up() {
		t.Fatalf("healthy model: up=%v err=%v", h.Up(), err)
	}
	h.failed(errors.New("timeout"))
	if h.Up() {
		t.Fatal("still up after a failed request")
	}
	ok.Store(false)
	if h.Check(context.Background()) == nil || h.Up() {
		t.Fatal("a failing health check brought it back")
	}
	ok.Store(true)
	if err := h.Check(context.Background()); err != nil || !h.Up() {
		t.Fatalf("recovered model: up=%v err=%v", h.Up(), err)
	}
	model.Close()
	if h.Check(context.Background()) == nil || h.Up() {
		t.Fatal("unreachable model counted as up")
	}
}
