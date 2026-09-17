package service

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type protectionTestStore struct {
	account *Account
}

func (s *protectionTestStore) GetAccount(context.Context, int64) (*Account, error) {
	copy := *s.account
	copy.Extra = maps.Clone(s.account.Extra)
	copy.CodexProxyIDs = append([]int64(nil), s.account.CodexProxyIDs...)
	return &copy, nil
}

func (s *protectionTestStore) UpdateAccount(_ context.Context, _ int64, input *UpdateAccountInput) (*Account, error) {
	copy := *s.account
	if input.Extra != nil {
		copy.Extra = maps.Clone(input.Extra)
	}
	if input.Concurrency != nil {
		copy.Concurrency = *input.Concurrency
	}
	s.account = &copy
	return s.GetAccount(context.Background(), copy.ID)
}

func TestMode1ProtectionPreservesConcurrencyAndCodexProxyPool(t *testing.T) {
	store := &protectionTestStore{account: &Account{
		ID:            42,
		Platform:      PlatformOpenAI,
		Type:          AccountTypeOAuth,
		Concurrency:   100,
		CodexProxyIDs: []int64{11, 12, 13, 14, 15},
		Extra:         map[string]any{},
		UpdatedAt:     time.Now().UTC(),
	}}

	updated, err := NewAntiDegradeService(store).Apply(context.Background(), store.account.ID)
	require.NoError(t, err)
	require.Equal(t, 100, updated.Concurrency)
	require.Equal(t, []int64{11, 12, 13, 14, 15}, updated.CodexProxyIDs)
	require.Equal(t, string(AntiDegradeMode1), updated.ProtectionMode())
	require.Equal(t, "observe", updated.RequestIntegrityMode())
	require.Equal(t, codexFingerprintDevice, updated.GetCodexFingerprintMode())
	require.False(t, updated.IsTLSFingerprintEnabled())
	require.Equal(t, "pool", ResolveProtectionRuntime(updated, nil, nil).ProxyMode)
}

func TestMode1ProtectionKeepsExplicitIntegrityEnforcement(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			requestIntegrityModeKey: "enforce",
			AntiDegradeMarkerExtraKey: map[string]any{
				"enabled": true, "mode": string(AntiDegradeMode1), "policy_version": mode1PolicyVersion,
			},
		},
	}
	require.Equal(t, "enforce", account.RequestIntegrityMode())
}

func TestMode1IntegrityRejectsLostToolSemantics(t *testing.T) {
	account := &Account{
		ID:       43,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			AntiDegradationExtraKey: true,
			AntiDegradeMarkerExtraKey: map[string]any{
				"enabled": true, "mode": string(AntiDegradeMode1), "policy_version": mode1PolicyVersion,
			},
		},
	}
	original := []byte(`{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":"hello"}],"tools":[{"type":"function","name":"lookup"}]}`)
	forwarded := []byte(`{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":"hello"}]}`)
	require.ErrorContains(t, validateMode1RequestIntegrityForAccount(account, original, forwarded), "tools")
}
