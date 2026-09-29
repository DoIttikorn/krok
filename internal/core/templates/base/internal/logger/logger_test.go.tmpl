package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		var buf bytes.Buffer
		l := slog.New(newHandler(&buf, slog.LevelInfo, asJSON))
		l.Debug("hidden")
		l.Info("shown", "key", "value")
		l.Error("failed", "error", "boom")

		out := buf.String()
		if strings.Contains(out, "hidden") {
			t.Errorf("json=%v: debug record logged at info level:\n%s", asJSON, out)
		}
		for _, want := range []string{"shown", "value", "failed", "boom"} {
			if !strings.Contains(out, want) {
				t.Errorf("json=%v: output lacks %q:\n%s", asJSON, want, out)
			}
		}
		if asJSON {
			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				if !json.Valid([]byte(line)) {
					t.Errorf("not a JSON line: %s", line)
				}
			}
		}
	}
}

func TestNewRejectsBadSettings(t *testing.T) {
	t.Setenv("LOG_LEVEL", "loud")
	if _, err := New(); err == nil {
		t.Error("LOG_LEVEL=loud: want an error")
	}
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "xml")
	if _, err := New(); err == nil {
		t.Error("LOG_FORMAT=xml: want an error")
	}
}
