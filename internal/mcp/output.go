package mcp

import (
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

// limitedBuffer keeps the last limit bytes written and drops the rest.
// Write always reports success so the remote command is not interrupted.
type limitedBuffer struct {
	mu        sync.Mutex
	buf       []byte
	start     int
	size      int
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	if l.limit <= 0 {
		l.truncated = true
		return len(p), nil
	}
	if l.buf == nil {
		l.buf = make([]byte, l.limit)
	}
	// A write at least as long as the cap is the whole tail of the stream.
	// An exact fill of an empty buffer keeps every byte, so it is not truncated.
	if len(p) >= l.limit {
		if len(p) > l.limit || l.size > 0 {
			l.truncated = true
		}
		copy(l.buf, p[len(p)-l.limit:])
		l.start = 0
		l.size = l.limit
		return len(p), nil
	}
	off := (l.start + l.size) % l.limit
	first := copy(l.buf[off:], p)
	if first < len(p) {
		copy(l.buf, p[first:])
	}
	if l.size+len(p) <= l.limit {
		l.size += len(p)
		return len(p), nil
	}
	l.start = (l.start + l.size + len(p) - l.limit) % l.limit
	l.size = l.limit
	l.truncated = true
	return len(p), nil
}

// bytes returns the kept bytes in order.
func (l *limitedBuffer) bytes() []byte {
	if l.size == 0 {
		return nil
	}
	out := make([]byte, l.size)
	if l.start+l.size <= l.limit {
		copy(out, l.buf[l.start:l.start+l.size])
		return out
	}
	n := copy(out, l.buf[l.start:])
	copy(out[n:], l.buf[:l.size-n])
	return out
}

// String returns the captured text with invalid UTF-8 replaced and a marker
// appended when output was dropped.
func (l *limitedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	text := strings.ToValidUTF8(string(l.bytes()), "\uFFFD")
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
