package logging

import (
	"log/slog"
	"os"
	"time"
)

type Logger struct {
	*slog.Logger
}

// New creates a new logger with the specified log level and custom format.
func New(level slog.Level) *Logger {
	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				// Format time as "2006-01-02 15:04:05.000"
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format("2006-01-02 15:04:05.000"))
				}
			}
			return a
		},
	}
	handler := slog.NewTextHandler(os.Stdout, opts)
	return &Logger{slog.New(handler)}
}

// LogHTTPRequest logs an incoming HTTP request.
func (l *Logger) LogHTTPRequest(method, path string, headers map[string][]string, body string) {
	l.Info("HTTP_REQUEST",
		"method", method,
		"path", path,
		"headers", headers,
		"body", body,
	)
}

// LogHTTPResponse logs an outgoing HTTP response.
func (l *Logger) LogHTTPResponse(status int, headers map[string][]string, body string) {
	l.Info("HTTP_RESPONSE",
		"status", status,
		"headers", headers,
		"body", body,
	)
}

// LogGigaChatRequest logs a request sent to GigaChat API.
func (l *Logger) LogGigaChatRequest(url string, headers map[string][]string, body string) {
	l.Info("GIGACHAT_REQUEST",
		"url", url,
		"headers", headers,
		"body", body,
	)
}

// LogGigaChatResponse logs a response received from GigaChat API.
func (l *Logger) LogGigaChatResponse(status int, headers map[string][]string, body string) {
	l.Info("GIGACHAT_RESPONSE",
		"status", status,
		"headers", headers,
		"body", body,
	)
}

// LogOllamaRequest logs a request sent to Ollama API.
func (l *Logger) LogOllamaRequest(url string, headers map[string][]string, body string) {
	l.Info("OLLAMA_REQUEST",
		"url", url,
		"headers", headers,
		"body", body,
	)
}

// LogOllamaResponse logs a response received from Ollama API.
func (l *Logger) LogOllamaResponse(status int, headers map[string][]string, body string) {
	l.Info("OLLAMA_RESPONSE",
		"status", status,
		"headers", headers,
		"body", body,
	)
}

// LogError logs an error with optional context.
func (l *Logger) LogError(err error, msg string, args ...any) {
	l.Error(msg, append([]any{"error", err}, args...)...)
}

// Default logger instance with Info level.
var defaultLogger = New(slog.LevelInfo)

// Default returns the default logger.
func Default() *Logger {
	return defaultLogger
}
