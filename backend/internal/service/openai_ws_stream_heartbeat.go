package service

import (
	"context"
	"time"
)

// readOpenAIWSMessageWithHeartbeat keeps the HTTP response alive while the WS
// reader waits for a complete upstream message. Only the caller owns the HTTP
// writer: the reader goroutine never accesses gin or response headers.
//
// One read is started at a time, and is joined before returning (including
// cancellation). No next-turn frame can be consumed before the lease returns
// to its pool. The ticker never changes the upstream per-read deadline.
func readOpenAIWSMessageWithHeartbeat(
	ctx context.Context,
	lease *openAIWSConnLease,
	timeout time.Duration,
	ticks <-chan time.Time,
	heartbeat func() bool,
) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ticks == nil {
		return lease.ReadMessageWithContextTimeout(ctx, timeout)
	}
	var message []byte
	var readErr error
	done := make(chan struct{})
	readCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(done)
		message, readErr = lease.ReadMessageWithContextTimeout(readCtx, timeout)
	}()
	// Also cancel and join if the HTTP writer or callback panics. Releasing a
	// lease while an old reader still owns it would corrupt the next pool user.
	defer func() {
		cancel()
		<-done
	}()
	for {
		select {
		case <-done:
			return message, readErr
		case <-ticks:
			// Prefer a completed read over an unnecessary heartbeat. Reading
			// message/readErr only after done establishes the happens-before.
			select {
			case <-done:
				return message, readErr
			default:
			}
			if ctx.Err() == nil && !heartbeat() {
				// A failed downstream write does not necessarily cancel the HTTP
				// request context. Interrupt this read, not just the next one.
				cancel()
				<-done
				return message, readErr
			}
		}
	}
}
