package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
)

func ginSlogMiddleware(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return gin.LoggerWithFormatter(func(params gin.LogFormatterParams) string {
		level := slog.LevelInfo
		switch {
		case params.StatusCode >= 500:
			level = slog.LevelError
		case params.StatusCode >= 400:
			level = slog.LevelWarn
		}

		args := []any{
			"status", params.StatusCode,
			"method", params.Method,
			"path", sanitizePath(params.Path),
			"ip", params.ClientIP,
			"latency", params.Latency,
			"size", params.BodySize,
		}
		if msg := strings.TrimSpace(params.ErrorMessage); msg != "" {
			args = append(args, "error", msg)
		}
		logger.Log(context.Background(), level, "gin request", args...)
		return ""
	})
}

func sanitizePath(path string) string {
	if path == "" {
		return path
	}
	parts := strings.Split(path, "/")
	for i := 0; i < len(parts)-1; i++ {
		switch parts[i] {
		case "push", "webhook":
			if parts[i+1] != "" {
				parts[i+1] = "***"
			}
		}
	}
	return strings.Join(parts, "/")
}

type slogWriter struct {
	logger    *slog.Logger
	level     slog.Level
	component string
}

func newSlogWriter(logger *slog.Logger, level slog.Level, component string) *slogWriter {
	return &slogWriter{
		logger:    logger,
		level:     level,
		component: component,
	}
}

func (w *slogWriter) Write(p []byte) (int, error) {
	if w.logger == nil {
		return len(p), nil
	}
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}
	lines := strings.Split(msg, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		w.logger.Log(context.Background(), w.level, line, "component", w.component)
	}
	return len(p), nil
}
