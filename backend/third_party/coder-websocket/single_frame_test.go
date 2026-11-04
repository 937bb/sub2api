//go:build !js

package websocket

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSingleFrameMessages(t *testing.T) {
	for _, mode := range []CompressionMode{CompressionDisabled, CompressionContextTakeover, CompressionNoContextTakeover} {
		for _, bits := range []int{9, 15} {
			t.Run(fmt.Sprintf("mode=%d/window=%d", mode, bits), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				large := make([]byte, 100000)
				seed := uint32(7)
				for i := range large {
					seed = seed*1664525 + 1013904223
					large[i] = byte(33 + seed%90)
				}
				messages := [][]byte{nil, []byte("x"), bytes.Repeat([]byte("<tool>"), 1000), large, large, append([]byte("x"), large...)}
				done := make(chan error, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					rw := w
					if mode != CompressionDisabled {
						rw = clientWindowResponseWriter{w, bits}
					}
					conn, err := Accept(rw, r, &AcceptOptions{CompressionMode: mode})
					if err != nil {
						done <- err
						return
					}
					defer func() { _ = conn.CloseNow() }()
					conn.SetReadLimit(1 << 20)
					for i, want := range messages {
						h, err := readFrameHeader(conn.br, make([]byte, 8))
						if err != nil {
							done <- err
							return
						}
						compressed := mode != CompressionDisabled && len(want) >= 128 && i != len(messages)-1
						if !h.fin || !h.masked || h.opcode != opText || h.rsv1 != compressed || h.rsv2 || h.rsv3 {
							done <- fmt.Errorf("message %d header %+v compressed=%v", i, h, compressed)
							return
						}
						if h.payloadLength > 1<<20 {
							done <- fmt.Errorf("oversized frame")
							return
						}
						wirePayload := make([]byte, h.payloadLength)
						if _, err = io.ReadFull(conn.br, wirePayload); err != nil {
							done <- err
							return
						}
						// Replay the observed frame through the normal decoder, including its
						// negotiated window and takeover state, to verify payload integrity.
						var raw bytes.Buffer
						bw := bufio.NewWriter(&raw)
						if err = writeFrameHeader(h, bw, make([]byte, 8)); err != nil {
							done <- err
							return
						}
						if _, err = bw.Write(wirePayload); err != nil {
							done <- err
							return
						}
						if err = bw.Flush(); err != nil {
							done <- err
							return
						}
						conn.br = bufio.NewReader(io.MultiReader(bytes.NewReader(raw.Bytes()), conn.br))
						_, got, err := conn.Read(ctx)
						if err != nil {
							done <- err
							return
						}
						if !bytes.Equal(got, want) {
							done <- fmt.Errorf("message %d payload changed", i)
							return
						}
					}
					done <- nil
				}))
				defer server.Close()
				conn, _, err := Dial(ctx, server.URL, &DialOptions{CompressionMode: mode, CompressionThreshold: 128, OfferClientMaxWindowBits: true, SingleFrameMessages: true})
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = conn.CloseNow() }()
				for i, payload := range messages {
					if i == len(messages)-1 {
						writer, err := conn.Writer(ctx, MessageText)
						if err != nil {
							t.Fatal(err)
						}
						if _, err = writer.Write(payload[:1]); err != nil {
							t.Fatal(err)
						}
						if _, err = writer.Write(payload[1:]); err != nil {
							t.Fatal(err)
						}
						if err = writer.Close(); err != nil {
							t.Fatal(err)
						}
					} else if err := conn.Write(ctx, MessageText, payload); err != nil {
						t.Fatal(err)
					}
				}
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			})
		}
	}
}
