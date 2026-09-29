package server

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestProbes(t *testing.T) {
	s := newTestServer()
	h := s.Handler()
	for _, path := range []string{"/livez", "/readyz", "/health"} {
		if rec := do(t, h, http.MethodGet, path, "", nil); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}

func TestReadyzFailsWhenDependencyIsDown(t *testing.T) {
	s := newTestServer()
	s.checks = map[string]func(context.Context) error{
		"database": func(context.Context) error { return errors.New("connection refused") },
	}
	h := s.Handler()

	var body map[string]string
	if rec := do(t, h, http.MethodGet, "/readyz", "", &body); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if body["database"] != "down" {
		t.Errorf("body = %v, want database down", body)
	}
	// Liveness ignores dependencies, so the pod is not restarted.
	if rec := do(t, h, http.MethodGet, "/livez", "", nil); rec.Code != http.StatusOK {
		t.Errorf("GET /livez = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyzFailsWhileDraining(t *testing.T) {
	s := newTestServer()
	s.draining.Store(true)
	h := s.Handler()

	if rec := do(t, h, http.MethodGet, "/readyz", "", nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if rec := do(t, h, http.MethodGet, "/livez", "", nil); rec.Code != http.StatusOK {
		t.Errorf("GET /livez = %d, want %d", rec.Code, http.StatusOK)
	}
}
