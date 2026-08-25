package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type runtimeDefaultsSettingRepoStub struct {
	SettingRepository
	values map[string]string
}

func (s *runtimeDefaultsSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (s *runtimeDefaultsSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func resetOpenAIRuntimeDefaultsForTest() {
	openAIRuntimeDefaultsSF.Forget("openai_runtime_defaults")
	openAIRuntimeDefaultsCache.Store(nil)
}

func TestOpenAIRuntimeDefaultsMissingAndStoredValues(t *testing.T) {
	defer publishOpenAIRuntimeDefaults(true, true)

	resetOpenAIRuntimeDefaultsForTest()
	repo := &runtimeDefaultsSettingRepoStub{values: map[string]string{}}
	settings := NewSettingService(repo, &config.Config{})
	wsEnabled, fingerprintFull := settings.GetOpenAIRuntimeDefaults(context.Background())
	require.True(t, wsEnabled)
	require.True(t, fingerprintFull)

	resetOpenAIRuntimeDefaultsForTest()
	repo.values[SettingKeyOpenAIOAuthWSDefaultEnabled] = "false"
	repo.values[SettingKeyOpenAICodexFingerprintDefaultFullEnabled] = "false"
	wsEnabled, fingerprintFull = settings.GetOpenAIRuntimeDefaults(context.Background())
	require.False(t, wsEnabled)
	require.False(t, fingerprintFull)
}

func TestOpenAIWSRuntimeDefaultAndAccountOverride(t *testing.T) {
	defer publishOpenAIRuntimeDefaults(true, true)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	svc := &OpenAIGatewayService{cfg: cfg, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), settingService: &SettingService{}}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 10}

	publishOpenAIRuntimeDefaults(false, true)
	decision := svc.resolveOpenAIWSProtocolDecision(context.Background(), account)
	require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
	require.Equal(t, "system_default_disabled", decision.Reason)

	account.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool}
	decision = svc.resolveOpenAIWSProtocolDecision(context.Background(), account)
	require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)

	publishOpenAIRuntimeDefaults(true, true)
	account.Extra["openai_oauth_responses_websockets_v2_mode"] = OpenAIWSIngressModeOff
	decision = svc.resolveOpenAIWSProtocolDecision(context.Background(), account)
	require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
	require.Equal(t, "account_mode_off", decision.Reason)

	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	account.Extra = nil
	publishOpenAIRuntimeDefaults(false, true)
	require.False(t, svc.isOpenAIAccountTransportCompatible(context.Background(), account, OpenAIUpstreamTransportResponsesWebsocketV2Ingress))

	account.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool}
	require.True(t, svc.isOpenAIAccountTransportCompatible(context.Background(), account, OpenAIUpstreamTransportResponsesWebsocketV2Ingress))
}

func TestCodexFingerprintRuntimeDefaultCoversOAuthLike(t *testing.T) {
	defer publishOpenAIRuntimeDefaults(true, true)
	svc := &OpenAIGatewayService{settingService: &SettingService{}}
	setupToken := &Account{ID: 9101, Platform: PlatformOpenAI, Type: AccountTypeSetupToken}

	publishOpenAIRuntimeDefaults(true, true)
	ids := svc.resolveCodexFingerprintIDsForRequest(context.Background(), setupToken, http.Header{})
	require.NotNil(t, ids)
	require.Equal(t, codexFingerprintFull, ids.mode)

	publishOpenAIRuntimeDefaults(true, false)
	require.Nil(t, svc.resolveCodexFingerprintIDsForRequest(context.Background(), setupToken, http.Header{}))

	setupToken.Extra = map[string]any{
		codexFingerprintModeExtraKey: string(codexFingerprintFull),
		codexFingerprintSeedExtraKey: testCodexFingerprintSeed,
	}
	ids = svc.resolveCodexFingerprintIDsForRequest(context.Background(), setupToken, http.Header{})
	require.NotNil(t, ids)
	require.Equal(t, codexFingerprintFull, ids.mode)
}
