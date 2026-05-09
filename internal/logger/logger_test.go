package logger

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

func captureLog(fn func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	}()
	fn()
	return buf.String()
}

func TestInitLoggerLevels(t *testing.T) {
	cases := []struct {
		input string
		want  LogLevel
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"warn", LevelWarn},
		{"WARN", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"", LevelInfo},
		{"unknown", LevelInfo},
	}
	for _, c := range cases {
		InitLogger(c.input)
		if currentLevel != c.want {
			t.Errorf("InitLogger(%q): level = %d, want %d", c.input, currentLevel, c.want)
		}
	}
}

func TestLogDebugVisible(t *testing.T) {
	InitLogger("debug")
	out := captureLog(func() { LogDebug("hello %s", "world") })
	if !strings.Contains(out, "[DEBUG]") {
		t.Errorf("expected [DEBUG] prefix, got %q", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected formatted message, got %q", out)
	}
}

func TestLogDebugSuppressedAtInfo(t *testing.T) {
	InitLogger("info")
	out := captureLog(func() { LogDebug("should not appear") })
	if out != "" {
		t.Errorf("debug should be suppressed at info level, got %q", out)
	}
}

func TestLogDebugSuppressedAtWarn(t *testing.T) {
	InitLogger("warn")
	out := captureLog(func() { LogDebug("should not appear") })
	if out != "" {
		t.Errorf("debug should be suppressed at warn level, got %q", out)
	}
}

func TestLogInfoVisible(t *testing.T) {
	InitLogger("info")
	out := captureLog(func() { LogInfo("info message") })
	if !strings.Contains(out, "[INFO]") {
		t.Errorf("expected [INFO] prefix, got %q", out)
	}
}

func TestLogInfoSuppressedAtWarn(t *testing.T) {
	InitLogger("warn")
	out := captureLog(func() { LogInfo("should not appear") })
	if out != "" {
		t.Errorf("info should be suppressed at warn level, got %q", out)
	}
}

func TestLogWarnVisible(t *testing.T) {
	InitLogger("warn")
	out := captureLog(func() { LogWarn("warn message") })
	if !strings.Contains(out, "[WARN]") {
		t.Errorf("expected [WARN] prefix, got %q", out)
	}
}

func TestLogWarnSuppressedAtError(t *testing.T) {
	InitLogger("error")
	out := captureLog(func() { LogWarn("should not appear") })
	if out != "" {
		t.Errorf("warn should be suppressed at error level, got %q", out)
	}
}

func TestLogErrorAlwaysVisible(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		InitLogger(level)
		out := captureLog(func() { LogError("error message") })
		if !strings.Contains(out, "[ERROR]") {
			t.Errorf("at level %q, [ERROR] should be visible, got %q", level, out)
		}
	}
}

func TestLogDebugAlsoShowsAtDebug(t *testing.T) {
	InitLogger("debug")
	out := captureLog(func() {
		LogDebug("d")
		LogInfo("i")
		LogWarn("w")
		LogError("e")
	})
	for _, prefix := range []string{"[DEBUG]", "[INFO]", "[WARN]", "[ERROR]"} {
		if !strings.Contains(out, prefix) {
			t.Errorf("at debug level, %s should appear in output", prefix)
		}
	}
}

func TestLogFormatting(t *testing.T) {
	InitLogger("debug")
	out := captureLog(func() { LogInfo("x=%d name=%s", 42, "test") })
	if !strings.Contains(out, "x=42 name=test") {
		t.Errorf("expected formatted output, got %q", out)
	}
}
