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
	gateway.setOpenAICodexTurnStateScanEnqueuer(func(accountID int64, model string) bool {
		queuedAccountID = accountID
		queuedModel = model
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

func TestRequiredOpenAICodexTurnStateDoesNotGateAPIKeyOrLegacyAccount(t *testing.T) {
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
