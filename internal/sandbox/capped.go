package sandbox

import (
	"bytes"
	"sync"
)

// MaxCapturedOutput caps the output a provider keeps from a setup command.
const MaxCapturedOutput = 1 << 20

// CappedBuffer keeps the first limit bytes written to it and drops the rest.
// Write always succeeds, so the remote command is never interrupted.
type CappedBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

// NewCappedBuffer returns a buffer that holds at most limit bytes.
func NewCappedBuffer(limit int) *CappedBuffer {
	return &CappedBuffer{limit: limit}
}

// Write stores as much of p as fits and reports all of it as written.
func (c *CappedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := c.limit - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// String returns the stored bytes.
func (c *CappedBuffer) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}
