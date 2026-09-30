package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func resetGatewayForwardingCacheForTest(t *testing.T) {
	t.Helper()
	previous := gatewayForwardingCache.Load()
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	gatewayForwardingSF.Forget("gateway_forwarding")
	t.Cleanup(func() {
		gatewayForwardingSF.Forget("gateway_forwarding")
		if cached, ok := previous.(*cachedGatewayForwardingSettings); ok && cached != nil {
			gatewayForwardingCache.Store(cached)
			return
		}
		gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
	})
}

func cacheHideMappedUpstreamModelForTest(t *testing.T, hidden bool) {
	t.Helper()
	resetGatewayForwardingCacheForTest(t)
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
		hideMappedUpstreamModel: hidden,
		expiresAt:               time.Now().Add(time.Minute).UnixNano(),
	})
}

func TestHideMappedUpstreamModelDefaultsEnabledAndSupportsHotReload(t *testing.T) {
	resetGatewayForwardingCacheForTest(t)
	repo := &gatewayTTLSettingRepo{data: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	require.True(t, svc.IsMappedUpstreamModelHidden(context.Background()))

	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.HideMappedUpstreamModel)

	settings.HideMappedUpstreamModel = false
	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	require.Equal(t, "false", repo.data[SettingKeyHideMappedUpstreamModel])
	require.False(t, svc.IsMappedUpstreamModelHidden(context.Background()))

	settings.HideMappedUpstreamModel = true
	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	require.Equal(t, "true", repo.data[SettingKeyHideMappedUpstreamModel])
	require.True(t, svc.IsMappedUpstreamModelHidden(context.Background()))
}

func TestShouldHideMappedUpstreamModelFailsClosedWithoutSettings(t *testing.T) {
	require.True(t, shouldHideMappedUpstreamModel(context.Background(), nil))
	var svc *SettingService
	require.True(t, svc.IsMappedUpstreamModelHidden(context.Background()))
}
