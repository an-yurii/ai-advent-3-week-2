package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	logger := New(slog.LevelDebug)
	if logger == nil {
		t.Error("Logger should not be nil")
	}
}

func TestLogHTTPRequest(t *testing.T) {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format("2006-01-02 15:04:05.000"))
				}
			}
			return a
		},
	}
	handler := slog.NewTextHandler(&buf, opts)
	logger := &Logger{slog.New(handler)}

	logger.LogHTTPRequest("GET", "/test", map[string][]string{"X-Test": {"value"}}, "body")
	output := buf.String()
	if !strings.Contains(output, "HTTP_REQUEST") {
		t.Errorf("Expected log to contain HTTP_REQUEST, got %s", output)
	}
	if !strings.Contains(output, "method=GET") {
		t.Errorf("Expected log to contain method=GET, got %s", output)
	}
}

func TestLogError(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	logger := &Logger{slog.New(handler)}

	logger.LogError(&testError{msg: "something went wrong"}, "test error", "key", "value")
	output := buf.String()
	if !strings.Contains(output, "test error") {
		t.Errorf("Expected log to contain 'test error', got %s", output)
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}