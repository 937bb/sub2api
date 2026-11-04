package tlsfingerprint

import (
	"io"
	"net/http"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// Bound per-profile identity partitions, including shared auth/quota clients.
const codexIdentityPoolCapacity = 64

type codexIdentityEntry struct {
	transport http.RoundTripper
	lastUsed  uint64
	retired   bool
}

type codexIdentityTransport struct {
	mu       sync.Mutex
	entries  map[string]*codexIdentityEntry
	sequence uint64
	factory  func() http.RoundTripper
}

func newCodexIdentityTransport(factory func() http.RoundTripper) *codexIdentityTransport {
	return &codexIdentityTransport{entries: make(map[string]*codexIdentityEntry), factory: factory}
}

func closeIdentityIdle(rt http.RoundTripper) {
	if closer, ok := rt.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (t *codexIdentityTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	key := openai.TransportIdentityKey(r.Context(), r.Header)
	t.mu.Lock()
	t.sequence++
	entry := t.entries[key]
	var retired *codexIdentityEntry
	if entry == nil {
		if len(t.entries) >= codexIdentityPoolCapacity {
			var oldestKey string
			for k, e := range t.entries {
				if retired == nil || e.lastUsed < retired.lastUsed {
					oldestKey, retired = k, e
				}
			}
			delete(t.entries, oldestKey)
			retired.retired = true
		}
		entry = &codexIdentityEntry{transport: t.factory()}
		t.entries[key] = entry
	}
	entry.lastUsed = t.sequence
	t.mu.Unlock()
	if retired != nil {
		closeIdentityIdle(retired.transport)
	}
	resp, err := entry.transport.RoundTrip(r)
	if err != nil {
		t.release(entry)
		return resp, err
	}
	if resp.Body == nil {
		t.release(entry)
		return resp, nil
	}
	body := &codexIdentityBody{ReadCloser: resp.Body, release: func() { t.release(entry) }}
	// Preserve the optional bidirectional body contract for HTTP upgrades.
	if writer, ok := resp.Body.(io.Writer); ok {
		resp.Body = &codexIdentityReadWriteBody{codexIdentityBody: body, Writer: writer}
	} else {
		resp.Body = body
	}
	return resp, nil
}

func (t *codexIdentityTransport) release(entry *codexIdentityEntry) {
	t.mu.Lock()
	retired := entry.retired
	t.mu.Unlock()
	if retired {
		closeIdentityIdle(entry.transport)
	}
}

func (t *codexIdentityTransport) CloseIdleConnections() {
	t.mu.Lock()
	entries := make([]*codexIdentityEntry, 0, len(t.entries))
	for key, entry := range t.entries {
		entry.retired = true
		entries = append(entries, entry)
		delete(t.entries, key)
	}
	t.mu.Unlock()
	for _, entry := range entries {
		closeIdentityIdle(entry.transport)
	}
}

type codexIdentityBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *codexIdentityBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}

type codexIdentityReadWriteBody struct {
	*codexIdentityBody
	io.Writer
}
