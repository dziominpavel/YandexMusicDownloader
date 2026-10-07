package downloader

import (
	"context"
	"errors"
)

// ErrStopped reports a download aborted on user request (the «Остановить»
// button). It is a cancellation, never a failure: the pool counts it
// separately and the UI renders "[stopped]" instead of "[error]".
var ErrStopped = errors.New("download stopped by user")

// StartJob arms the client for a new job: a fresh context for every
// request and a cleared stop flag. The previous job's context is
// canceled so no stale transfer can outlive its job.
func (c *Client) StartJob() {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	if c.stopCancel != nil {
		c.stopCancel()
	}
	c.stopCtx, c.stopCancel = context.WithCancel(context.Background())
	c.stop.Store(false)
}

// Stop aborts the current job: the request context is canceled, which
// cuts in-flight transfers at once (instead of waiting for the next
// read), and Stopped() turns true so the pool stops launching tracks.
// Safe to call from any goroutine; idempotent.
func (c *Client) Stop() {
	c.stop.Store(true)
	c.stopMu.Lock()
	// Stop может прийти до первого запроса: контекст создаём сами.
	if c.stopCtx == nil {
		c.stopCtx, c.stopCancel = context.WithCancel(context.Background())
	}
	cancel := c.stopCancel
	c.stopMu.Unlock()
	cancel()
}

// Stopped reports whether Stop was requested for the current job.
func (c *Client) Stopped() bool {
	return c.stop.Load()
}

// reqCtx is the context every request carries (see newRequest). Canceling
// it is what makes «Остановить» break sockets mid-transfer.
func (c *Client) reqCtx() context.Context {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	if c.stopCtx == nil {
		// Job-less client (tests, previews): a context nobody cancels.
		c.stopCtx, c.stopCancel = context.WithCancel(context.Background())
	}
	return c.stopCtx
}
