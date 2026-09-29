package httpx

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogRecordsStatus(t *testing.T) {
	var buf bytes.Buffer
	h := Log(slog.New(slog.NewTextHandler(&buf, nil)))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/brew", nil))
	if out := buf.String(); !strings.Contains(out, "status=418") || !strings.Contains(out, "path=/brew") {
		t.Errorf("log = %q, want status=418 and path=/brew", out)
	}
}

func TestLogRecoversPanics(t *testing.T) {
	var buf bytes.Buffer
	h := Log(slog.New(slog.NewTextHandler(&buf, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if out := buf.String(); !strings.Contains(out, "boom") || !strings.Contains(out, "level=ERROR") {
		t.Errorf("log = %q, want the panic at error level", out)
	}
}
