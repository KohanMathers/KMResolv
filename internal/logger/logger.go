package logger

import (
	"fmt"
	"log"
	"strings"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

var currentLevel LogLevel

func InitLogger(level string) {
	switch strings.ToLower(level) {
	case "debug":
		currentLevel = LevelDebug
	case "warn":
		currentLevel = LevelWarn
	case "error":
		currentLevel = LevelError
	default:
		currentLevel = LevelInfo
	}
}

func LogDebug(format string, args ...any) {
	if currentLevel <= LevelDebug {
		log.Output(2, fmt.Sprintf("[DEBUG] "+format, args...))
	}
}

func LogInfo(format string, args ...any) {
	if currentLevel <= LevelInfo {
		log.Output(2, fmt.Sprintf("[INFO]  "+format, args...))
	}
}

func LogWarn(format string, args ...any) {
	if currentLevel <= LevelWarn {
		log.Output(2, fmt.Sprintf("[WARN]  "+format, args...))
	}
}

func LogError(format string, args ...any) {
	if currentLevel <= LevelError {
		log.Output(2, fmt.Sprintf("[ERROR] "+format, args...))
	}
}
