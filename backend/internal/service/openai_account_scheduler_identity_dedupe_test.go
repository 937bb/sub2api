package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeduplicateOpenAICodexAccountCandidates(t *testing.T) {
	accounts := []*Account{
		{
			ID:       12,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Priority: 20,
			Credentials: map[string]any{
				"chatgpt_account_id": "team-1",
				"chatgpt_user_id":    "member-1",
			},
		},
		{
			ID:       11,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Priority: 10,
			Credentials: map[string]any{
				"chatgpt_account_id": "team-1",
				"chatgpt_user_id":    "member-1",
			},
		},
		{
			ID:       13,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Priority: 10,
			Credentials: map[string]any{
				"chatgpt_account_id": "team-1",
				"chatgpt_user_id":    "member-2",
			},
		},
	}

	got, duplicates := deduplicateOpenAICodexAccountCandidates(accounts)

	require.Equal(t, 1, duplicates)
	require.Equal(t, []int64{11, 13}, openAIAccountCandidateIDs(got))
}

func TestDeduplicateOpenAICodexAccountCandidatesKeepsAPIKeysAndUnknownIdentities(t *testing.T) {
	accounts := []*Account{
		{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{ID: 22, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{ID: 23, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{ID: 24, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
	}

	got, duplicates := deduplicateOpenAICodexAccountCandidates(accounts)

	require.Zero(t, duplicates)
	require.Equal(t, []int64{21, 22, 23, 24}, openAIAccountCandidateIDs(got))
}

func TestDeduplicateOpenAICodexAccountCandidatesUsesLowestIDAsStableTieBreak(t *testing.T) {
	accounts := []*Account{
		{ID: 32, Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Priority: 5, Credentials: map[string]any{"access_token": "same-token"}},
		{ID: 31, Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Priority: 5, Credentials: map[string]any{"access_token": "same-token"}},
	}

	got, duplicates := deduplicateOpenAICodexAccountCandidates(accounts)

	require.Equal(t, 1, duplicates)
	require.Equal(t, []int64{31}, openAIAccountCandidateIDs(got))
}

func openAIAccountCandidateIDs(accounts []*Account) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}
