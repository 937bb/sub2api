package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForceOpenAIPriorityTierInBody(t *testing.T) {
	openAIAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	enabledKey := &APIKey{OpenAIForcePriorityTier: true}

	t.Run("disabled key preserves body", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.5","service_tier":"flex"}`)
		updated, err := forceOpenAIPriorityTierInBody(&APIKey{}, openAIAccount, body)
		require.NoError(t, err)
		require.Equal(t, body, updated)
	})

	t.Run("injects missing tier", func(t *testing.T) {
		updated, err := forceOpenAIPriorityTierInBody(enabledKey, openAIAccount, []byte(`{"model":"gpt-5.5"}`))
		require.NoError(t, err)
		require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(updated, "service_tier").String())
		require.Equal(t, "gpt-5.5", gjson.GetBytes(updated, "model").String())
	})

	t.Run("overrides client tier", func(t *testing.T) {
		updated, err := forceOpenAIPriorityTierInBody(enabledKey, openAIAccount, []byte(`{"service_tier":"flex"}`))
		require.NoError(t, err)
		require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(updated, "service_tier").String())
	})

	t.Run("does not affect compatible non OpenAI accounts", func(t *testing.T) {
		body := []byte(`{"model":"grok-4.5"}`)
		updated, err := forceOpenAIPriorityTierInBody(enabledKey, &Account{Platform: PlatformGrok}, body)
		require.NoError(t, err)
		require.Equal(t, body, updated)
	})
}

func TestForceOpenAIPriorityTier_AdminPolicyStillFilters(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, openAIFastFilterPriorityPolicy())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	forced, err := forceOpenAIPriorityTierInBody(
		&APIKey{OpenAIForcePriorityTier: true},
		account,
		[]byte(`{"model":"gpt-5.5"}`),
	)
	require.NoError(t, err)
	require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(forced, "service_tier").String())

	filtered, err := svc.applyOpenAIFastPolicyToBody(context.Background(), account, "gpt-5.5", forced)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(filtered, "service_tier").Exists())
}

func TestForceOpenAIPriorityTier_WebSocketResponseCreate(t *testing.T) {
	svc := newOpenAIGatewayServiceWithSettings(t, DefaultOpenAIFastPolicySettings())
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	frame := []byte(`{"type":"response.create","model":"gpt-5.5"}`)

	forced, err := forceOpenAIPriorityTierInBody(&APIKey{OpenAIForcePriorityTier: true}, account, frame)
	require.NoError(t, err)
	updated, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), account, "gpt-5.5", forced)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(updated, "service_tier").String())
}

func TestAPIKeyAuthSnapshotRoundTripPreservesOpenAIForcePriorityTier(t *testing.T) {
	svc := &APIKeyService{}
	apiKey := &APIKey{
		ID:                      10,
		UserID:                  20,
		Key:                     "sk-test",
		Status:                  StatusActive,
		OpenAIForcePriorityTier: true,
		User: &User{
			ID:     20,
			Status: StatusActive,
		},
	}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.OpenAIForcePriorityTier)

	roundTrip := svc.snapshotToAPIKey(apiKey.Key, snapshot)
	require.NotNil(t, roundTrip)
	require.True(t, roundTrip.OpenAIForcePriorityTier)
}
