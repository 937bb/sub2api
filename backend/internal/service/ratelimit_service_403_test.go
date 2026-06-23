//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type runtimeBlockRecorder struct {
	accounts   []*Account
	until      []time.Time
	reasons    []string
	clearedIDs []int64
}

func (r *runtimeBlockRecorder) BlockAccountScheduling(account *Account, until time.Time, reason string) {
	r.accounts = append(r.accounts, account)
	r.until = append(r.until, until)
	r.reasons = append(r.reasons, reason)
}

func (r *runtimeBlockRecorder) ClearAccountSchedulingBlock(accountID int64) {
	r.clearedIDs = append(r.clearedIDs, accountID)
}

func TestRateLimitService_HandleUpstreamError_OpenAI403FirstHitTempUnschedulable(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{1}}
	blocker := &runtimeBlockRecorder{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	service.SetAccountRuntimeBlocker(blocker)
	account := &Account{
		ID:       301,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusForbidden,
		http.Header{},
		[]byte(`{"error":{"message":"temporary edge rejection"}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, "temporary edge rejection")
	require.Contains(t, repo.lastTempReason, "(1/3)")
	require.Len(t, blocker.accounts, 1)
	require.Equal(t, account.ID, blocker.accounts[0].ID)
	require.Equal(t, "openai_403_temp", blocker.reasons[0])
	require.True(t, blocker.until[0].After(time.Now()))
}

func TestRateLimitService_HandleUpstreamError_OpenAI403ThresholdDisables(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{3}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := &Account{
		ID:       302,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusForbidden,
		http.Header{},
		[]byte(`{"error":{"message":"workspace forbidden by policy"}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Contains(t, repo.lastErrorMsg, "workspace forbidden by policy")
	require.Contains(t, repo.lastErrorMsg, "consecutive_403=3/3")
}

func TestRateLimitService_HandleUpstreamError_OpenAIPATWorkspace403DisablesImmediately(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{1}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := &Account{
		ID:       303,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"personal_access_token": "pat-test",
		},
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusForbidden,
		http.Header{},
		[]byte(`{"error":{"message":"Personal access token owner is not an active member of the selected workspace."}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, []int64{1}, counter.counts)
	require.Contains(t, repo.lastErrorMsg, "Personal access token owner is not an active member of the selected workspace")
	require.NotContains(t, repo.lastErrorMsg, "consecutive_403")
}

func TestRateLimitService_HandleUpstreamError_OpenAIPATOwnerInactive403DisablesImmediately(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{1}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := &Account{
		ID:       305,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"personal_access_token": "pat-test",
		},
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusForbidden,
		http.Header{},
		[]byte(`{"error":{"message":"Personal access token owner is inactive."}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls)
	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, []int64{1}, counter.counts)
	require.Contains(t, repo.lastErrorMsg, "Personal access token owner is inactive")
	require.NotContains(t, repo.lastErrorMsg, "consecutive_403")
}

func TestRateLimitService_HandleUpstreamError_OpenAIOAuthWorkspace403UsesTempCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{1}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := &Account{
		ID:       304,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "oauth-test",
		},
	}

	shouldDisable := service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusForbidden,
		http.Header{},
		[]byte(`{"error":{"message":"Personal access token owner is not an active member of the selected workspace."}}`),
	)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, "Personal access token owner is not an active member of the selected workspace")
}

func TestRateLimitService_HandleUpstreamError_OpenAI403RawFallbackRedactsSensitiveMetadata(t *testing.T) {
	installationID := "550e8400-e29b-41d4-a716-446655440000"
	threadID := "018fed75-1b7e-7000-8000-000000000123"
	accessToken := "setup-secret-access-token"
	rawUA := "codex-tui/0.136.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.136.0)"
	body := []byte(`{"diagnostic":{"x-codex-installation-id":"` + installationID + `","thread_id":"` + threadID + `","authorization":"Bearer ` + accessToken + `","raw_user_agent":"` + rawUA + `"},"diagnostic_text":"x-codex-installation-id=` + installationID + ` thread-id=` + threadID + ` Authorization=Bearer ` + accessToken + ` ua ` + rawUA + `"}`)

	t.Run("temporary cooldown", func(t *testing.T) {
		repo := &rateLimitAccountRepoStub{}
		counter := &openAI403CounterCacheStub{counts: []int64{1}}
		service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		service.SetOpenAI403CounterCache(counter)
		account := &Account{ID: 303, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

		shouldDisable := service.HandleUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{}, body)

		require.True(t, shouldDisable)
		require.Equal(t, 0, repo.setErrorCalls)
		require.Equal(t, 1, repo.tempCalls)
		assertOpenAIDiagnosticRedacted(t, repo.lastTempReason, installationID, threadID, accessToken, rawUA)
		require.Contains(t, repo.lastTempReason, "[redacted]")
		require.Contains(t, repo.lastTempReason, "[codex-user-agent-redacted]")
	})

	t.Run("threshold disable", func(t *testing.T) {
		repo := &rateLimitAccountRepoStub{}
		counter := &openAI403CounterCacheStub{counts: []int64{3}}
		service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		service.SetOpenAI403CounterCache(counter)
		account := &Account{ID: 304, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

		shouldDisable := service.HandleUpstreamError(context.Background(), account, http.StatusForbidden, http.Header{}, body)

		require.True(t, shouldDisable)
		require.Equal(t, 1, repo.setErrorCalls)
		require.Equal(t, 0, repo.tempCalls)
		assertOpenAIDiagnosticRedacted(t, repo.lastErrorMsg, installationID, threadID, accessToken, rawUA)
		require.Contains(t, repo.lastErrorMsg, "consecutive_403=3/3")
	})
}

func TestRateLimitService_OpenAITempUnschedulableReasonRedactsSensitiveMetadata(t *testing.T) {
	installationID := "550e8400-e29b-41d4-a716-446655440000"
	threadID := "018fed75-1b7e-7000-8000-000000000123"
	accessToken := "setup-secret-access-token"
	rawUA := "codex-tui/0.136.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.136.0)"
	body := []byte(`{"error":{"message":"overloaded","x-codex-installation-id":"` + installationID + `","thread_id":"` + threadID + `","authorization":"Bearer ` + accessToken + `","raw_user_agent":"` + rawUA + `"}}`)
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       305,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusServiceUnavailable),
					"keywords":         []any{"overloaded"},
					"duration_minutes": float64(5),
				},
			},
		},
	}

	shouldDisable := service.HandleUpstreamError(context.Background(), account, http.StatusServiceUnavailable, http.Header{}, body)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	assertOpenAIDiagnosticRedacted(t, repo.lastTempReason, installationID, threadID, accessToken, rawUA)
	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(repo.lastTempReason), &state))
	assertOpenAIDiagnosticRedacted(t, state.ErrorMessage, installationID, threadID, accessToken, rawUA)
	require.Contains(t, state.ErrorMessage, "[redacted]")
}

func assertOpenAIDiagnosticRedacted(t *testing.T, text string, leaked ...string) {
	t.Helper()
	for _, fragment := range leaked {
		require.NotContains(t, text, fragment)
	}
}
