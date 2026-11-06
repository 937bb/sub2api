package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/smartws"
	coderws "github.com/coder/websocket"
	"github.com/gorilla/websocket"
)

var groupSmartWSDialer = newDefaultOpenAIWSClientDialer()

// Preserve the existing account TLS profile, header ordering and WS framing.
func dialGroupSmartWS(ctx context.Context, target string, headers http.Header, route string) (smartws.Conn, *http.Response, error) {
	conn, status, responseHeaders, err := groupSmartWSDialer.Dial(ctx, target, headers, route)
	resp := &http.Response{StatusCode: http.StatusSwitchingProtocols, Header: responseHeaders}
	if err != nil {
		if status == 0 {
			return nil, nil, err
		}
		resp.StatusCode = status
		resp.ContentLength = -1
		if size, parseErr := strconv.ParseInt(responseHeaders.Get("Content-Length"), 10, 64); parseErr == nil {
			resp.ContentLength = size
		}
		var handshake *openAIWSHandshakeError
		if errors.As(err, &handshake) {
			resp.Body = io.NopCloser(bytes.NewReader(handshake.Body))
		} else {
			resp.Body = http.NoBody
		}
		return nil, resp, err
	}
	return &groupSmartWSConn{conn: conn}, resp, nil
}

type groupSmartWSConn struct{ conn openAIWSClientConn }

func (c *groupSmartWSConn) ReadMessage() (int, []byte, error) {
	if frameConn, ok := c.conn.(interface {
		ReadFrame(context.Context) (coderws.MessageType, []byte, error)
	}); ok {
		kind, data, err := frameConn.ReadFrame(context.Background())
		if status := coderws.CloseStatus(err); status != -1 {
			err = &websocket.CloseError{Code: int(status)}
		}
		return int(kind), data, err
	}
	data, err := c.conn.ReadMessage(context.Background())
	return websocket.TextMessage, data, err
}

func (c *groupSmartWSConn) WriteMessage(kind int, data []byte) error {
	frameConn, ok := c.conn.(interface {
		WriteFrame(context.Context, coderws.MessageType, []byte) error
	})
	if !ok {
		return errors.New("smart WS transport does not support raw frames")
	}
	return frameConn.WriteFrame(context.Background(), coderws.MessageType(kind), data)
}

func (c *groupSmartWSConn) SetReadLimit(limit int64) {
	if conn, ok := c.conn.(*coderOpenAIWSClientConn); ok {
		conn.conn.SetReadLimit(limit)
	}
}

func (c *groupSmartWSConn) Close() error {
	if closer, ok := c.conn.(openAIWSForceCloser); ok {
		return closer.CloseNow()
	}
	return c.conn.Close()
}
