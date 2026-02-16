package app

import (
	"strings"
	"sync"
)

// LogBuffer stores formatted log lines for the TUI.
type LogBuffer struct {
	mu    sync.RWMutex
	lines []string
}

func NewLogBuffer() *LogBuffer {
	return &LogBuffer{
		lines: make([]string, 0, 256),
	}
}

// Write implements io.Writer so slog can stream formatted text into the buffer.
func (b *LogBuffer) Write(p []byte) (int, error) {
	b.appendBytes(p)
	return len(p), nil
}

// appendBytes normalizes multi-line writes into discrete log lines.
func (b *LogBuffer) appendBytes(p []byte) {
	text := strings.TrimRight(string(p), "\n")
	if text == "" {
		return
	}

	parts := strings.Split(text, "\n")
	b.mu.Lock()
	for _, line := range parts {
		if line == "" {
			continue
		}
		b.lines = append(b.lines, line)
	}
	b.mu.Unlock()
}

// Lines returns a copy of the buffered log lines.
func (b *LogBuffer) Lines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	lines := make([]string, len(b.lines))
	copy(lines, b.lines)
	return lines
}
