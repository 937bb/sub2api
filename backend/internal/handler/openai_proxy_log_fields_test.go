package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestOpenAIProxyLogFieldsUsesLoadedTransportProxyOnly(t *testing.T) {
	proxyID := int64(17)
	staleProxyID := int64(99)

	tests := []struct {
		name       string
		account    *service.Account
		wantFields map[string]any
	}{
		{
			name:       "nil account",
			account:    nil,
			wantFields: map[string]any{},
		},
		{
			name:       "direct account",
			account:    &service.Account{ProxyID: nil},
			wantFields: map[string]any{},
		},
		{
			name:       "stale unloaded proxy binding",
			account:    &service.Account{ProxyID: &proxyID},
			wantFields: map[string]any{},
		},
		{
			name:       "loaded relation without binding",
			account:    &service.Account{Proxy: &service.Proxy{ID: proxyID}},
			wantFields: map[string]any{},
		},
		{
			name: "loaded proxy",
			account: &service.Account{
				ProxyID: &staleProxyID,
				Proxy:   &service.Proxy{ID: proxyID, Name: "private-name", Host: "private.internal", Port: 8443},
			},
			wantFields: map[string]any{"proxy_id": proxyID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)
			logger := zap.New(core)
			logger.Info("proxy-attempt", openAIProxyLogFields(tt.account)...)

			require.Len(t, logs.All(), 1)
			require.Equal(t, tt.wantFields, logs.All()[0].ContextMap())
		})
	}
}
