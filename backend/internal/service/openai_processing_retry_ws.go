package service

import (
	"context"
	"sync"

	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/tidwall/gjson"
)

type processingWSFrame struct {
	typ  coderws.MessageType
	data []byte
}

// This adapter owns a single reader and serialized writes, just like FrameConn.
// A replacement connection replays only the current, stateless, uncommitted
// turn. Overlapping turns and connection-bound continuations disable replay.
type openAIProcessingRetryFrameConn struct {
	mu             sync.Mutex
	inner          openaiwsv2.FrameConn
	reconnect      func(context.Context, []byte) (openaiwsv2.FrameConn, error)
	allowTemporary bool
	closed         bool
	active         bool
	committed      bool
	retried        bool
	replay         processingWSFrame
}

func (c *openAIProcessingRetryFrameConn) WriteFrame(ctx context.Context, typ coderws.MessageType, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errOpenAIWSConnClosed
	}
	if gjson.GetBytes(payload, "type").String() == "response.create" {
		c.committed = c.active
		c.active = true
		c.retried = false
		c.replay = processingWSFrame{}
		if !c.committed && gjson.GetBytes(payload, "previous_response_id").String() == "" {
			c.replay = processingWSFrame{typ: typ, data: append([]byte(nil), payload...)}
		}
	}
	return c.inner.WriteFrame(ctx, typ, payload)
}

func (c *openAIProcessingRetryFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return coderws.MessageText, nil, errOpenAIWSConnClosed
		}
		inner := c.inner
		c.mu.Unlock()
		typ, payload, err := inner.ReadFrame(ctx)
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return typ, nil, errOpenAIWSConnClosed
		}
		event := gjson.GetBytes(payload, "type").String()
		canRetry := c.active && !c.committed && !c.retried && len(c.replay.data) > 0 && c.reconnect != nil
		retryFailure := (err == nil && (event == "error" || event == "response.failed") &&
			(isOpenAIProcessingFailure(payload) || (c.allowTemporary && isOpenAIPreOutputRetryFailure(payload)))) ||
			(c.allowTemporary && openAIRetryReadError(err))
		if canRetry && retryFailure && ctx.Err() == nil {
			c.retried = true
			_ = c.inner.Close()
			fresh, dialErr := c.reconnect(ctx, c.replay.data)
			if dialErr != nil {
				c.mu.Unlock()
				return typ, nil, dialErr
			}
			c.inner = fresh
			writeErr := fresh.WriteFrame(ctx, c.replay.typ, c.replay.data)
			c.mu.Unlock()
			if writeErr != nil {
				return typ, nil, writeErr
			}
			continue
		}
		if err != nil {
			c.mu.Unlock()
			return typ, payload, err
		}
		c.committed = true
		if isOpenAIWSTerminalEvent(event) || event == "error" {
			c.active = false
			c.replay = processingWSFrame{}
		}
		c.mu.Unlock()
		return typ, payload, nil
	}
}

func (c *openAIProcessingRetryFrameConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	c.replay = processingWSFrame{}
	return c.inner.Close()
}
