package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func openAIPromptCacheTestContext(path string, apiKeyID int64) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", path, nil)
	c.Set("api_key", &APIKey{ID: apiKeyID})
	return c
}

func TestEnsureOpenAIOAuthPromptCacheKeyStableAcrossGrowingConversation(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	c := openAIPromptCacheTestContext("/v1/responses", 71)
	first := []byte(`{"model":"gpt-6.1-sol","instructions":"Be concise","tools":[{"type":"function","name":"lookup"}],"input":[{"type":"message","role":"user","content":"find alpha"}],"stream":true}`)
	second := []byte(`{"model":"gpt-6.1-sol","instructions":"Be concise","tools":[{"type":"function","name":"lookup"}],"input":[{"type":"message","role":"user","content":"find alpha"},{"type":"message","role":"assistant","content":"alpha"},{"type":"message","role":"user","content":"add tests"}],"stream":true}`)

	firstOut, firstKey, firstChanged, err := ensureOpenAIOAuthPromptCacheKey(c, account, first)
	require.NoError(t, err)
	require.True(t, firstChanged)
	require.NotEmpty(t, firstKey)
	require.Equal(t, firstKey, gjson.GetBytes(firstOut, "prompt_cache_key").String())

	secondOut, secondKey, secondChanged, err := ensureOpenAIOAuthPromptCacheKey(c, account, second)
	require.NoError(t, err)
	require.True(t, secondChanged)
	require.Equal(t, firstKey, secondKey)
	require.Equal(t, secondKey, gjson.GetBytes(secondOut, "prompt_cache_key").String())
}

func TestEnsureOpenAIOAuthPromptCacheKeyPreservesExplicitAndScopesTenant(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"model":"gpt-6.1-sol","input":[{"role":"user","content":"hello"}]}`)

	firstOut, firstKey, changed, err := ensureOpenAIOAuthPromptCacheKey(openAIPromptCacheTestContext("/v1/responses", 11), account, body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, firstKey, gjson.GetBytes(firstOut, "prompt_cache_key").String())

	_, secondKey, changed, err := ensureOpenAIOAuthPromptCacheKey(openAIPromptCacheTestContext("/v1/responses", 12), account, body)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotEqual(t, firstKey, secondKey)

	explicit := []byte(`{"model":"gpt-6.1-sol","prompt_cache_key":"client-session","input":[{"role":"user","content":"hello"}]}`)
	explicitOut, explicitKey, changed, err := ensureOpenAIOAuthPromptCacheKey(openAIPromptCacheTestContext("/v1/responses", 11), account, explicit)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, "client-session", explicitKey)
	require.JSONEq(t, string(explicit), string(explicitOut))
}

func TestEnsureOpenAIOAuthPromptCacheKeySkipsUnsafeShapes(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"model":"gpt-6.1-sol","input":[{"role":"user","content":"hello"}]}`)

	for _, test := range []struct {
		name    string
		ctx     *gin.Context
		account *Account
		body    []byte
	}{
		{name: "compact", ctx: openAIPromptCacheTestContext("/v1/responses/compact", 11), account: oauth, body: body},
		{name: "api key upstream", ctx: openAIPromptCacheTestContext("/v1/responses", 11), account: apiKey, body: body},
		{name: "missing tenant", ctx: openAIPromptCacheTestContext("/v1/responses", 0), account: oauth, body: body},
		{name: "no user anchor", ctx: openAIPromptCacheTestContext("/v1/responses", 11), account: oauth, body: []byte(`{"model":"gpt-6.1-sol","input":[]}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			out, key, changed, err := ensureOpenAIOAuthPromptCacheKey(test.ctx, test.account, test.body)
			require.NoError(t, err)
			require.False(t, changed)
			require.Empty(t, key)
			require.JSONEq(t, string(test.body), string(out))
		})
	}
}
