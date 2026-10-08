// Package logging keeps human-readable messages out of stdout.
package logging

import (
	"fmt"
	"io"
	"sync"
	"time"
)

type Logger struct {
	mu    sync.Mutex
	Out   io.Writer
	Level string
}

func (l *Logger) Log(level, format string, args ...interface{}) {
	levels := map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}
	if levels[level] < levels[l.Level] {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.Out, "%s [%s] %s\n", time.Now().Format(time.RFC3339), level, fmt.Sprintf(format, args...))
}
