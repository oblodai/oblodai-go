package oblodai

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Minimal structured logging contract. Anything with these four methods fits (a slog adapter, a
// zap sugared logger, a test recorder). Every logger the client uses — the built-in text logger
// and any logger passed to WithLogger alike — is wrapped so that a field whose key looks like a
// secret is redacted BEFORE it reaches the logger: a debug log cannot leak a key, a signature or
// a cheque passcode, not even into a caller's own logging pipeline.

// LogFields is a set of structured fields attached to a log line.
type LogFields map[string]any

// Logger receives the client's diagnostics.
type Logger interface {
	Debug(message string, fields LogFields)
	Info(message string, fields LogFields)
	Warn(message string, fields LogFields)
	Error(message string, fields LogFields)
}

// LogLevel selects how much a text logger emits.
type LogLevel string

const (
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
)

var logOrder = map[LogLevel]int{LogDebug: 0, LogInfo: 1, LogWarn: 2, LogError: 3}

// redactingLogger redacts sensitive-looking fields and then delegates. WithLogger installs one
// around the caller's logger, so the raw value never reaches it.
type redactingLogger struct{ inner Logger }

func (l redactingLogger) Debug(message string, fields LogFields) {
	l.inner.Debug(message, redactFields(fields))
}

func (l redactingLogger) Info(message string, fields LogFields) {
	l.inner.Info(message, redactFields(fields))
}

func (l redactingLogger) Warn(message string, fields LogFields) {
	l.inner.Warn(message, redactFields(fields))
}

func (l redactingLogger) Error(message string, fields LogFields) {
	l.inner.Error(message, redactFields(fields))
}

// redactFields copies a field set, replacing the values of sensitive-looking keys. The copy
// matters: the caller's map must not be rewritten under it.
func redactFields(fields LogFields) LogFields {
	if len(fields) == 0 {
		return fields
	}
	out := make(LogFields, len(fields))
	for k, v := range fields {
		out[k] = redact(k, v)
	}
	return out
}

type nopLogger struct{}

func (nopLogger) Debug(string, LogFields) {}
func (nopLogger) Info(string, LogFields)  {}
func (nopLogger) Warn(string, LogFields)  {}
func (nopLogger) Error(string, LogFields) {}

// textLogger writes one line per event to a writer, gated by level.
type textLogger struct {
	min int
	out io.Writer
}

// NewTextLogger returns a logger that writes "[oblodai] LEVEL message key=value" lines to w
// (os.Stderr when w is nil). Setting OBLODAI_LOG=debug|info|warn|error installs one automatically.
func NewTextLogger(level LogLevel, w io.Writer) Logger {
	if w == nil {
		w = os.Stderr
	}
	min, ok := logOrder[level]
	if !ok {
		min = logOrder[LogWarn]
	}
	return &textLogger{min: min, out: w}
}

func (l *textLogger) Debug(message string, fields LogFields) { l.emit(LogDebug, message, fields) }
func (l *textLogger) Info(message string, fields LogFields)  { l.emit(LogInfo, message, fields) }
func (l *textLogger) Warn(message string, fields LogFields)  { l.emit(LogWarn, message, fields) }
func (l *textLogger) Error(message string, fields LogFields) { l.emit(LogError, message, fields) }

func (l *textLogger) emit(level LogLevel, message string, fields LogFields) {
	if logOrder[level] < l.min {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[oblodai] %s %s", strings.ToUpper(string(level)), message)
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%v", k, fields[k])
	}
	b.WriteString("\n")
	_, _ = io.WriteString(l.out, b.String())
}

// redactedPlaceholder is what a sensitive value reads as once redacted.
const redactedPlaceholder = "[redacted]"

// sensitive keys never reach a log line with their value intact.
var sensitiveWords = []string{"secret", "signature", "passcode", "token", "authorization", "password"}

// redact replaces the value of a sensitive-looking key.
func redact(key string, value any) any {
	lower := strings.ToLower(key)
	for _, word := range sensitiveWords {
		if strings.Contains(lower, word) {
			return redactedPlaceholder
		}
	}
	return value
}
