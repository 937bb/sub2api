package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRequiredOpenAICodexTurnStateBlocksPerModelAndQueuesScan(t *testing.T) {
	accountID := int64(41)
	const model = "gpt-6-astra"
	account := &Account{
		ID:       accountID,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{openAICodexStateRoutingRequiredExtraKey: true},
	}
	gateway := &OpenAIGatewayService{}
	queuedAccountID := int64(0)
	queuedModel := ""
	gateway.setOpenAICodexTurnStateScanEnqueuer(func(accountID int64, model string, force bool) bool {
		queuedAccountID = accountID
		queuedModel = model
		require.False(t, force)
		return true
	})

	require.False(t, gateway.hasRequiredOpenAICodexTurnState(account, model))
	require.Equal(t, accountID, queuedAccountID)
	require.Equal(t, model, queuedModel)

	state := testOpenAICodexTurnState(openAICodexTurnStateLength332, time.Now().UTC().Add(-time.Minute), 's')
	gateway.getOpenAICodexTurnStatePool().observe(state, &accountID, "session", model, "scanner")
	require.True(t, gateway.hasRequiredOpenAICodexTurnState(account, model))
	require.False(t, gateway.hasRequiredOpenAICodexTurnState(account, "gpt-5.5"), "state must remain scoped to one model")
}

func TestRequiredOpenAICodexTurnStateDoesNotGateAPIKeyOrLegacyAccountByDefault(t *testing.T) {
	gateway := &OpenAIGatewayService{}
	apiKey := &Account{
		ID:       41,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{openAICodexStateRoutingRequiredExtraKey: true},
	}
	legacyOAuth := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	require.True(t, gateway.hasRequiredOpenAICodexTurnState(apiKey, "gpt-6-astra"))
	require.True(t, gateway.hasRequiredOpenAICodexTurnState(legacyOAuth, "gpt-6-astra"))
}

func TestRequiredOpenAICodexTurnStateStrictRouteBindingGatesLegacyOAuthAccounts(t *testing.T) {
	gateway := &OpenAIGatewayService{}
	settings := defaultOpenAICodexTurnStateScanSettings()
	requireRouteBinding := true
	settings.RequireRouteBinding = &requireRouteBinding
	gateway.getOpenAICodexTurnStatePool().setScanSettings(settings)
	legacyOAuth := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	require.False(t, gateway.hasRequiredOpenAICodexTurnState(legacyOAuth, "gpt-6-astra"))
}

func TestRequiredOpenAICodexTurnStateDisabledPlanBypassesStrictGate(t *testing.T) {
	gateway := &OpenAIGatewayService{}
	settings := defaultOpenAICodexTurnStateScanSettings()
	requireRouteBinding := true
	settings.RequireRouteBinding = &requireRouteBinding
	settings.PlanScanEnabled["pro"] = false
	gateway.getOpenAICodexTurnStatePool().setScanSettings(settings)
	account := &Account{
		ID:          42,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": "pro"},
		Extra:       map[string]any{openAICodexStateRoutingRequiredExtraKey: true},
	}
	queued := false
	gateway.setOpenAICodexTurnStateScanEnqueuer(func(int64, string, bool) bool {
		queued = true
		return true
	})

	require.True(t, gateway.hasRequiredOpenAICodexTurnState(account, "gpt-6-astra"))
	require.False(t, queued)
}

func TestRequiredOpenAICodexTurnStateCanBeDisabledAtRuntime(t *testing.T) {
	account := &Account{
		ID:       41,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{openAICodexStateRoutingRequiredExtraKey: true},
	}
	gateway := &OpenAIGatewayService{}
	queued := false
	gateway.setOpenAICodexTurnStateScanEnqueuer(func(int64, string, bool) bool {
		queued = true
		return true
	})
	settings := defaultOpenAICodexTurnStateScanSettings()
	requireStateBeforeRouting := false
	settings.RequireStateBeforeRouting = &requireStateBeforeRouting
	gateway.getOpenAICodexTurnStatePool().setScanSettings(settings)

	require.True(t, gateway.hasRequiredOpenAICodexTurnState(account, "gpt-6-astra"))
	require.False(t, queued)
}

func TestRequiredOpenAICodexTurnStateUsesAccountPlanOnFirstRoutingCheck(t *testing.T) {
	const model = "gpt-5.6-terra"
	accountID := int64(41)
	account := &Account{
		ID:          accountID,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": "team"},
		Extra:       map[string]any{openAICodexStateRoutingRequiredExtraKey: true},
	}
	gateway := &OpenAIGatewayService{}
	settings := defaultOpenAICodexTurnStateScanSettings()
	settings.Rules = []OpenAICodexTurnStateLengthRule{
		{PlanType: "team", Model: model, TargetLengths: []int{286}},
	}
	gateway.getOpenAICodexTurnStatePool().setScanSettings(settings)
	state := testScopedOpenAICodexTurnState(286, time.Now().UTC().Add(-time.Minute), 't')
	gateway.getOpenAICodexTurnStatePool().observe(state, &accountID, "session", model, "scanner")

	require.True(t, gateway.hasRequiredOpenAICodexTurnState(account, model))
}
