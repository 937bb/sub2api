package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type sharedTurnStateCache struct {
	GatewayCache
	mu          sync.Mutex
	owners      map[string]string
	unavailable bool
}

func (c *sharedTurnStateCache) BindOpenAICodexTurnStateOwner(_ context.Context, key, owner string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.unavailable {
		return errors.New("cache unavailable")
	}
	if c.owners == nil {
		c.owners = map[string]string{}
	}
	c.owners[key] = owner
	return nil
}
func (c *sharedTurnStateCache) GetOpenAICodexTurnStateOwner(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.unavailable {
		return "", errors.New("cache unavailable")
	}
	return c.owners[key], nil
}

func TestTurnStateOwnershipAcrossProcessesAndOldBlobs(t *testing.T) {
	shared := &sharedTurnStateCache{}
	producer := &OpenAIGatewayService{cache: shared}
	consumer := &OpenAIGatewayService{cache: shared}
	a, b := accountDeviceFixture(11, "a"), accountDeviceFixture(12, "b")
	c, _ := newTurnStateTestContext(t, 7, "session")
	producer.noteOpenAICodexTurnStateProvenance(c, a, "state-a")
	producer.noteOpenAICodexTurnStateProvenance(c, b, "state-b")
	for _, svc := range []*OpenAIGatewayService{producer, consumer} {
		require.False(t, svc.isForeignOpenAICodexTurnState(c, a, "state-a"))
		require.True(t, svc.isForeignOpenAICodexTurnState(c, b, "state-a"))
		require.False(t, svc.isForeignOpenAICodexTurnState(c, b, "state-b"))
		require.True(t, svc.isForeignOpenAICodexTurnState(c, a, "state-b"))
		require.False(t, svc.isForeignOpenAICodexTurnState(c, b, "unknown"))
	}
	// Refreshing a bearer and using another row for the same credential is safe.
	refreshed := accountDeviceFixture(99, "a")
	refreshed.Credentials["access_token"] = "refreshed-fixture"
	require.False(t, consumer.isForeignOpenAICodexTurnState(c, refreshed, "state-a"))
	c.Request.Header.Set("session_id", "another-thread")
	require.True(t, consumer.isForeignOpenAICodexTurnState(c, b, "state-a"))
	otherTenant, _ := newTurnStateTestContext(t, 8, "session")
	require.False(t, consumer.isForeignOpenAICodexTurnState(otherTenant, b, "state-a"))
	for key, owner := range shared.owners {
		require.Len(t, key, 64)
		require.Len(t, owner, 64)
		require.NotContains(t, key, "state")
	}
}

func TestTurnStateCacheFailureDoesNotRejectUnknownHistory(t *testing.T) {
	shared := &sharedTurnStateCache{unavailable: true}
	svc := &OpenAIGatewayService{cache: shared}
	c, _ := newTurnStateTestContext(t, 7, "s")
	a, b := accountDeviceFixture(11, "a"), accountDeviceFixture(12, "b")
	svc.noteOpenAICodexTurnStateProvenance(c, a, "known")
	require.True(t, svc.isForeignOpenAICodexTurnState(c, b, "known"), "local provenance still works")
	require.False(t, svc.isForeignOpenAICodexTurnState(c, b, "unknown"))
}

func TestTurnStateOwnedFallbackRejectsOtherAccountAndLegacyEntries(t *testing.T) {
	store := NewOpenAIWSStateStore(nil)
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 7, "s")
	a, b := accountDeviceFixture(11, "a"), accountDeviceFixture(12, "b")
	store.BindSessionTurnState(1, "scope", "unowned", time.Hour)
	_, ok := getOwnedOpenAIWSTurnState(c, a, store, 1, "scope")
	require.False(t, ok)
	svc.bindOpenAIWSTurnState(c, a, store, 1, "scope", "owned")
	state, ok := getOwnedOpenAIWSTurnState(c, a, store, 1, "scope")
	require.True(t, ok)
	require.Equal(t, "owned", state)
	_, ok = getOwnedOpenAIWSTurnState(c, b, store, 1, "scope")
	require.False(t, ok)
	_, ok = getOwnedOpenAIWSTurnState(c, a, store, 2, "scope")
	require.False(t, ok)
}

func TestTurnStateGuardAllTransportsAndDoesNotMutateInput(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 7, "s")
	a, b := accountDeviceFixture(11, "a"), accountDeviceFixture(12, "b")
	svc.noteOpenAICodexTurnStateProvenance(c, a, "foreign")
	c.Request.Header.Set(openAICodexTurnStateHeader, "foreign")
	body := []byte(`{"model":"gpt-5","input":[],"client_metadata":{"x-codex-turn-state":"foreign","counter":9007199254740993}}`)
	for _, passthrough := range []bool{false, true} {
		var req *http.Request
		var err error
		if passthrough {
			req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, b, body, "fixture")
		} else {
			req, err = svc.buildUpstreamRequest(context.Background(), c, b, body, "fixture", true, "", true)
		}
		require.NoError(t, err)
		require.Empty(t, req.Header.Get(openAICodexTurnStateHeader))
		actual, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(actual, "client_metadata."+openAICodexTurnStateHeader).Exists())
		require.Equal(t, "9007199254740993", gjson.GetBytes(actual, "client_metadata.counter").Raw)
	}
	h, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, b, "fixture", OpenAIWSProtocolDecision{}, true, "foreign", "", "", "", "")
	require.NoError(t, err)
	require.Empty(t, h.Get(openAICodexTurnStateHeader))
	metadata := map[string]any{openAICodexTurnStateHeader: "foreign", "other": "keep"}
	payload := map[string]any{"client_metadata": metadata}
	svc.guardOpenAICodexTurnStatePayload(c, b, payload)
	require.NotContains(t, payload["client_metadata"], openAICodexTurnStateHeader)
	require.Equal(t, "foreign", metadata[openAICodexTurnStateHeader])
	require.Equal(t, "foreign", c.GetHeader(openAICodexTurnStateHeader))
	require.Equal(t, "foreign", gjson.GetBytes(body, "client_metadata."+openAICodexTurnStateHeader).String())
}

func TestCodexWSTurnStateMovesFromHandshakeIntoResponseCreateMetadata(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 7, "s")
	account := accountDeviceFixture(11, "upstream-a")

	headers, _, err := svc.buildOpenAIWSHeaders(
		context.Background(), c, account, "fixture", OpenAIWSProtocolDecision{},
		true, "owned-state", "", "", "gpt-5.6-codex", "",
	)
	require.NoError(t, err)
	require.Empty(t, headers.Get(openAICodexTurnStateHeader), "native Codex WS upgrade must not carry turn state")

	body, err := svc.applyOpenAICodexTurnStateBody(c, account, []byte(`{"type":"response.create","client_metadata":{"keep":"yes"}}`), "owned-state")
	require.NoError(t, err)
	require.Equal(t, "owned-state", gjson.GetBytes(body, "client_metadata."+openAICodexTurnStateHeader).String())
	require.Equal(t, "yes", gjson.GetBytes(body, "client_metadata.keep").String())

	explicit, err := svc.applyOpenAICodexTurnStateBody(c, account, []byte(`{"type":"response.create","client_metadata":{"x-codex-turn-state":"frame-state"}}`), "owned-state")
	require.NoError(t, err)
	require.Equal(t, "frame-state", gjson.GetBytes(explicit, "client_metadata."+openAICodexTurnStateHeader).String())

	payload := map[string]any{"type": "response.create", "client_metadata": map[string]any{"keep": "map"}}
	svc.applyOpenAICodexTurnStatePayload(c, account, payload, "owned-state")
	metadata := payload["client_metadata"].(map[string]any)
	require.Equal(t, "owned-state", metadata[openAICodexTurnStateHeader])
	require.Equal(t, "map", metadata["keep"])
}

func TestTurnStateProvenanceBoundedUnderConcurrentWrites(t *testing.T) {
	svc := &OpenAIGatewayService{}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 1024; i++ {
				key := fmt.Sprintf("%d/%d", worker, i)
				svc.openaiCodexTurnStateOrigins.Store(key, openAICodexTurnStateOrigin{owner: "fixture", expiresAt: time.Now().Add(time.Minute)})
				svc.openaiCodexTurnStateOrigins.Load(key)
				svc.sweepOpenAICodexTurnStateOrigins()
			}
		}(worker)
	}
	wg.Wait()
	require.LessOrEqual(t, len(svc.openaiCodexTurnStateOrigins.entries), openAICodexTurnStateMaxOrigins)
}
