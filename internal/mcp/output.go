package mcp

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	// maxOutputBytes caps each captured stream of an exec call.
	maxOutputBytes = 256 << 10
	// defaultExecTimeout stops an exec call that never returns, so one prompt
	// cannot hold the server and a sandbox forever.
	defaultExecTimeout = 30 * time.Minute
	// maxExecTimeout is the longest an agent may ask for.
	maxExecTimeout = 2 * time.Hour
)

// execTimeout turns the timeout_seconds argument into a duration.
func execTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultExecTimeout
	}
	return min(time.Duration(seconds)*time.Second, maxExecTimeout)
}

// limitedBuffer keeps the first limit bytes written and drops the rest.
// Write always reports success so the remote command is not interrupted.
type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	room := l.limit - l.buf.Len()
	switch {
	case room <= 0:
		l.truncated = len(p) > 0 || l.truncated
	case len(p) > room:
		l.buf.Write(p[:room])
		l.truncated = true
	default:
		l.buf.Write(p)
	}
	return len(p), nil
}

// String returns the captured text with invalid UTF-8 replaced and a marker
// appended when output was dropped.
func (l *limitedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	text := strings.ToValidUTF8(l.buf.String(), "\uFFFD")
	if l.truncated {
		text += fmt.Sprintf("\n[output truncated at %d bytes]", l.limit)
	}
	return text
}

func (l *limitedBuffer) wasTruncated() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.truncated
}
