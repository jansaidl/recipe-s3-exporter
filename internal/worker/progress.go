package worker

import (
	"context"
	"io"
	"time"
)

// countingReader wraps an io.Reader and reports cumulative bytes read to the
// database, throttled so we don't write on every chunk.
type countingReader struct {
	r       io.Reader
	total   int64
	flush   func(ctx context.Context, n int64)
	ctx     context.Context
	lastAt  time.Time
	lastVal int64
}

func newCountingReader(ctx context.Context, r io.Reader, flush func(context.Context, int64)) *countingReader {
	return &countingReader{r: r, flush: flush, ctx: ctx}
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.total += int64(n)
		now := time.Now()
		// Persist at most ~3x/sec or every 8 MiB, whichever comes first.
		if now.Sub(c.lastAt) > 350*time.Millisecond || c.total-c.lastVal > 8<<20 {
			c.lastAt = now
			c.lastVal = c.total
			c.flush(c.ctx, c.total)
		}
	}
	return n, err
}
