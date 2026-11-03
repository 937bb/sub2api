//go:build !js

package websocket

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientMaxWindowNegotiation(t *testing.T) {
	for bits := 8; bits <= 15; bits++ {
		opts := &compressionOptions{offerClientMaxWindowBits: true}
		h := http.Header{"Sec-Websocket-Extensions": {fmt.Sprintf("permessage-deflate; client_max_window_bits=%d", bits)}}
		got, err := verifyServerExtensions(opts, h)
		if err != nil || got.clientMaxWindowBits != bits {
			t.Fatalf("bits=%d: opts=%+v error=%v", bits, got, err)
		}
		if opts.clientMaxWindowBits != 0 {
			t.Fatal("mutated offer")
		}
	}
	for _, params := range []string{"client_max_window_bits", "client_max_window_bits=7", "client_max_window_bits=16", "client_max_window_bits=abc", "client_max_window_bits=08", "client_max_window_bits=+8", "client_max_window_bits=8; client_max_window_bits=8", "server_max_window_bits=16", "client_no_context_takeover; client_no_context_takeover"} {
		h := http.Header{"Sec-Websocket-Extensions": {"permessage-deflate; " + params}}
		if _, err := verifyServerExtensions(&compressionOptions{offerClientMaxWindowBits: true}, h); err == nil {
			t.Errorf("accepted malformed %q", params)
		}
	}
	if _, err := verifyServerExtensions(&compressionOptions{}, http.Header{"Sec-Websocket-Extensions": {"permessage-deflate; client_max_window_bits=15"}}); err == nil {
		t.Fatal("accepted unoffered window")
	}
	if got := (&compressionOptions{offerClientMaxWindowBits: true}).String(); got != "permessage-deflate; client_max_window_bits" {
		t.Fatal(got)
	}
	if got := (&compressionOptions{}).String(); got != "permessage-deflate" {
		t.Fatal("changed default offer", got)
	}
}

type clientWindowResponseWriter struct {
	http.ResponseWriter
	bits int
}

func (w clientWindowResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w clientWindowResponseWriter) WriteHeader(status int) {
	if status == 101 {
		w.Header().Set("Sec-WebSocket-Extensions", w.Header().Get("Sec-WebSocket-Extensions")+fmt.Sprintf("; client_max_window_bits=%d", w.bits))
	}
	w.ResponseWriter.WriteHeader(status)
}

func TestClientMaxWindowCompressedRoundTrip(t *testing.T) {
	for bits := 8; bits <= 15; bits++ {
		for _, mode := range []CompressionMode{CompressionContextTakeover, CompressionNoContextTakeover} {
			t.Run(fmt.Sprintf("window=%d/mode=%d", bits, mode), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				done := make(chan error, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !strings.Contains(r.Header.Get("Sec-WebSocket-Extensions"), "client_max_window_bits") {
						done <- fmt.Errorf("missing offer")
						return
					}
					c, err := Accept(clientWindowResponseWriter{w, bits}, r, &AcceptOptions{CompressionMode: mode, CompressionThreshold: 1})
					if err != nil {
						done <- err
						return
					}
					defer c.CloseNow()
					c.SetReadLimit(1 << 20)
					for range 3 {
						typ, data, err := c.Read(ctx)
						if err != nil {
							done <- err
							return
						}
						if err = c.Write(ctx, typ, data); err != nil {
							done <- err
							return
						}
					}
					done <- nil
				}))
				defer server.Close()
				c, _, err := Dial(ctx, server.URL, &DialOptions{CompressionMode: mode, CompressionThreshold: 1, OfferClientMaxWindowBits: true})
				if err != nil {
					t.Fatal(err)
				}
				defer c.CloseNow()
				c.SetReadLimit(1 << 20)
				if c.copts.clientMaxWindowBits != bits {
					t.Fatalf("window=%d", c.copts.clientMaxWindowBits)
				}
				for i := range 3 {
					data := bytes.Repeat([]byte(fmt.Sprintf("round-%d:abcdefghijklmnopqrstuvwxyz0123456789", i)), 2000)
					if err := c.Write(ctx, MessageText, data); err != nil {
						t.Fatal(err)
					}
					_, got, err := c.Read(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(got, data) {
						t.Fatal("compressed payload changed")
					}
				}
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
