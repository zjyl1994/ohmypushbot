package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type GormLogger struct {
	Log           *slog.Logger
	SlowThreshold time.Duration
	LogLevel      logger.LogLevel
}

func NewGormLogger(log *slog.Logger) *GormLogger {
	return &GormLogger{
		Log:           log,
		SlowThreshold: 200 * time.Millisecond,
		LogLevel:      logger.Warn,
	}
}

func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newLogger := *l
	newLogger.LogLevel = level
	return &newLogger
}

func (l *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Info {
		l.Log.InfoContext(ctx, fmt.Sprintf(msg, data...))
	}
}

func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Warn {
		l.Log.WarnContext(ctx, fmt.Sprintf(msg, data...))
	}
}

func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Error {
		l.Log.ErrorContext(ctx, fmt.Sprintf(msg, data...))
	}
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	attrs := []any{
		slog.Duration("elapsed", elapsed),
		slog.Int64("rows", rows),
	}

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && l.LogLevel >= logger.Error {
		attrs = append(attrs, slog.String("sql", sql), slog.Any("error", err))
		l.Log.ErrorContext(ctx, "gorm trace", attrs...)
		return
	}

	if l.SlowThreshold != 0 && elapsed > l.SlowThreshold && l.LogLevel >= logger.Warn {
		attrs = append(attrs, slog.String("sql", sql))
		l.Log.WarnContext(ctx, "gorm slow query", attrs...)
		return
	}

	if l.LogLevel >= logger.Info {
		attrs = append(attrs, slog.String("sql", sql))
		l.Log.InfoContext(ctx, "gorm trace", attrs...)
	}
}
