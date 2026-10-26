package logging

import (
	"context"
	"log/slog"
	"os"
	"sync"
)

type LogLevel string

const (
	Debug   LogLevel = "debug"
	Info    LogLevel = "info"
	Warning LogLevel = "warning"
	Error   LogLevel = "error"
)

var logLevels = []LogLevel{Debug, Info, Warning, Error}

var (
	currentLevel LogLevel = Info
	levelMu      sync.RWMutex
	logger       = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
)

func SetLogLevel(level LogLevel) {
	levelMu.Lock()
	defer levelMu.Unlock()
	currentLevel = level
}

func GetLogLevel() LogLevel {
	levelMu.RLock()
	defer levelMu.RUnlock()
	return currentLevel
}

func ShouldLog(level LogLevel) bool {
	levelMu.RLock()
	defer levelMu.RUnlock()

	var currentIdx, levelIdx int
	for i, l := range logLevels {
		if l == currentLevel {
			currentIdx = i
		}
		if l == level {
			levelIdx = i
		}
	}
	return levelIdx >= currentIdx
}

func LogMessage(level LogLevel, message string) {
	if !ShouldLog(level) {
		return
	}

	var slogLevel slog.Level
	switch level {
	case Debug:
		slogLevel = slog.LevelDebug
	case Info:
		slogLevel = slog.LevelInfo
	case Warning:
		slogLevel = slog.LevelWarn
	case Error:
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	logger.Log(context.Background(), slogLevel, message)
}
