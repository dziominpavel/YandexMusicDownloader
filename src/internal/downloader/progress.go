package downloader

import (
	"io"
	"time"
)

// progressInterval throttles live updates: at most one emit per interval
// unless the percent ticked up by >=1.
const progressInterval = 200 * time.Millisecond

// progressReader counts bytes flowing through r and reports them via cb.
// The final read (EOF or error) always reports, so the UI sees completion.
type progressReader struct {
	r        io.Reader
	total    int64
	done     int64
	lastEmit time.Time
	lastPct  int
	interval time.Duration
	now      func() time.Time
	cb       func(done, total int64)
}

func newProgressReader(r io.Reader, total int64, cb func(done, total int64)) *progressReader {
	return &progressReader{
		r:        r,
		total:    total,
		lastPct:  percentOf(0, total),
		interval: progressInterval,
		now:      time.Now,
		cb:       cb,
	}
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.done += int64(n)
		p.maybeEmit(false)
	}
	if err != nil {
		// EOF or failure: flush the final counter state.
		p.maybeEmit(true)
	}
	return n, err
}

func (p *progressReader) maybeEmit(force bool) {
	if p.cb == nil {
		return
	}
	pct := percentOf(p.done, p.total)
	now := p.now()
	if !force {
		if now.Sub(p.lastEmit) < p.interval && pct <= p.lastPct {
			return
		}
	}
	p.lastEmit = now
	p.lastPct = pct
	p.cb(p.done, p.total)
}
