package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestCodexWSJSONMatchesObservedSerialization(t *testing.T) {
	value := map[string]any{"type": "response.create", "input": "<tool>&中文\n\"quoted\""}
	payload, err := marshalCodexWSJSON(value)
	require.NoError(t, err)
	require.False(t, strings.HasSuffix(string(payload), "\n"))
	require.Contains(t, string(payload), "<tool>&中文")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, value, decoded)
	_, err = marshalCodexWSJSON(make(chan int))
	require.Error(t, err)
}

func TestCodexWSJSONWireOnlyChangesMeasuredClient(t *testing.T) {
	for _, ua := range []string{codexCLIUserAgent, "other-client/1.0"} {
		t.Run(ua, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			received := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, err := coderws.Accept(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer func() { _ = c.CloseNow() }()
				_, p, err := c.Read(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				received <- p
			}))
			defer server.Close()
			dialer := newDefaultOpenAIWSClientDialer()
			c, _, _, err := dialer.Dial(ctx, strings.Replace(server.URL, "http://", "ws://", 1), http.Header{"User-Agent": {ua}}, "")
			require.NoError(t, err)
			defer func() { _ = c.Close() }()
			require.NoError(t, c.WriteJSON(ctx, map[string]any{"input": "<tool>&中文"}))
			select {
			case p := <-received:
				require.Equal(t, ua != codexCLIUserAgent, strings.HasSuffix(string(p), "\n"))
				if ua == codexCLIUserAgent {
					require.Contains(t, string(p), "<tool>&中文")
				} else {
					require.Contains(t, string(p), `\u003c`)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}
