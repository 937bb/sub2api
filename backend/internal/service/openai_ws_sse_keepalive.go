package service

import (
	"context"
	"time"
)

// readOpenAIWSWithKeepalive keeps all downstream writes on the caller goroutine.
// A heartbeat never cancels or restarts the upstream read: doing so can destroy
// a WebSocket connection and lose a partially received message.
func readOpenAIWSWithKeepalive(ctx context.Context, read func(context.Context) ([]byte, error), ticks <-chan time.Time, heartbeat func()) ([]byte, error) {
	if ticks == nil {
		return read(ctx)
	}
	type result struct {
		message []byte
		err     error
	}
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan result, 1)
	go func() {
		message, err := read(readCtx)
		done <- result{message: message, err: err}
	}()
	for {
		select {
		case r := <-done:
			return r.message, r.err
		case <-ticks:
			heartbeat()
		}
	}
}
