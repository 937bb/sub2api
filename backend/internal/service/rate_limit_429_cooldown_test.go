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

type rateLimit429AccountRepoStub struct {
	mockAccountRepoForGemini
	rateLimitCalls     int
	lastRateLimitID    int64
	lastRateLimitReset time.Time
}

func (r *rateLimit429AccountRepoStub) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitCalls++
	r.lastRateLimitID = id
	r.lastRateLimitReset = resetAt
	return nil
}

func TestGetRateLimit429CooldownSettings_DefaultsWhenNotSet(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetRateLimit429CooldownSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 5, settings.CooldownSeconds)
}

func TestGetRateLimit429CooldownSettings_ReadsFromDB(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: false, CooldownSeconds: 12})
	repo.data[SettingKeyRateLimit429CooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetRateLimit429CooldownSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 12, settings.CooldownSeconds)
}

func TestSetRateLimit429CooldownSettings_EnabledRejectsOutOfRange(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	for _, seconds := range []int{0, -1, 7201, 99999} {
		err := svc.SetRateLimit429CooldownSettings(context.Background(), &RateLimit429CooldownSettings{
			Enabled: true, CooldownSeconds: seconds,
		})
		require.Error(t, err, "should reject enabled=true + cooldown_seconds=%d", seconds)
		require.Contains(t, err.Error(), "cooldown_seconds must be between 1-7200")
	}
}

func TestHandle429_FallbackUsesDBSeconds(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: true, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
	after := time.Now()

	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.Equal(t, int64(42), accountRepo.lastRateLimitID)
	require.True(t, !accountRepo.lastRateLimitReset.Before(before.Add(12*time.Second)) && !accountRepo.lastRateLimitReset.After(after.Add(12*time.Second)))
}

func TestHandle429_FallbackDisabledSkipsLocalMark(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: false, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))

	require.Zero(t, accountRepo.rateLimitCalls)
}

// Anthropic 无 reset 头的 429（如 Extra usage required）也应走兜底冷却，
// 否则账号永不冷却，调度器会让每个请求反复撞同一批 429 账号（旋转木马）。
func TestHandle429_AnthropicNoResetTimeUsesFallbackCooldown(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: true, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 45, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"type":"rate_limit_error","message":"Extra usage required"}}`))
	after := time.Now()

	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.Equal(t, int64(45), accountRepo.lastRateLimitID)
	require.True(t, !accountRepo.lastRateLimitReset.Before(before.Add(12*time.Second)) && !accountRepo.lastRateLimitReset.After(after.Add(12*time.Second)))
}

// 管理端关闭兜底冷却时，Anthropic 无 reset 头的 429 保持旧行为：不标记账号。
func TestHandle429_AnthropicNoResetTimeFallbackDisabledSkipsMark(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(RateLimit429CooldownSettings{Enabled: false, CooldownSeconds: 12})
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 46, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"type":"rate_limit_error","message":"Extra usage required"}}`))

	require.Zero(t, accountRepo.rateLimitCalls)
}

func TestHandle429_FallbackUsesDefaultSecondsWhenSettingServiceMissing(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	cfg := &config.Config{}
	svc := NewRateLimitService(accountRepo, nil, cfg, nil, nil)

	account := &Account{ID: 44, Platform: PlatformGemini, Type: AccountTypeAPIKey}
	before := time.Now()
	svc.handle429(context.Background(), account, http.Header{}, []byte(`{"error":{"message":"slow down"}}`))
	after := time.Now()

	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.Equal(t, int64(44), accountRepo.lastRateLimitID)
	require.True(t, !accountRepo.lastRateLimitReset.Before(before.Add(5*time.Second)) && !accountRepo.lastRateLimitReset.After(after.Add(5*time.Second)))
}

func storeOpenAIOAuth429DynamicSettings(t *testing.T, repo *mockSettingRepo, settings OpenAIOAuth429DynamicSettings) {
	t.Helper()
	data, err := json.Marshal(settings)
	require.NoError(t, err)
	repo.data[SettingKeyOpenAIOAuth429DynamicSettings] = string(data)
}

func TestOpenAIOAuth429Dynamic_RateLimitsOnlyAfterThreshold(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	storeOpenAIOAuth429DynamicSettings(t, settingRepo, OpenAIOAuth429DynamicSettings{
		Enabled: true, WindowSeconds: 60, MinSamples: 3, Min429: 2,
		RatioThreshold: 0.6, BlockSeconds: 12,
	})
	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)
	account := &Account{ID: 47, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	svc.handle429(context.Background(), account, http.Header{}, nil)
	require.Zero(t, accountRepo.rateLimitCalls)
	svc.RecordOpenAIOAuthUpstreamOutcome(context.Background(), account, http.StatusOK)
	require.Zero(t, accountRepo.rateLimitCalls)

	before := time.Now()
	svc.handle429(context.Background(), account, http.Header{}, nil)
	after := time.Now()
	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.True(t, !accountRepo.lastRateLimitReset.Before(before.Add(12*time.Second)) && !accountRepo.lastRateLimitReset.After(after.Add(12*time.Second)))
	require.False(t, svc.hasOpenAIOAuth429DynamicStats(account.ID))
}

func TestOpenAIOAuth429Dynamic_IncludesSetupTokenAndResetClearsWindow(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	storeOpenAIOAuth429DynamicSettings(t, settingRepo, OpenAIOAuth429DynamicSettings{
		Enabled: true, WindowSeconds: 60, MinSamples: 2, Min429: 2,
		RatioThreshold: 1, BlockSeconds: 12,
	})
	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)
	account := &Account{ID: 48, Platform: PlatformOpenAI, Type: AccountTypeSetupToken}

	svc.handle429(context.Background(), account, http.Header{}, nil)
	require.True(t, svc.hasOpenAIOAuth429DynamicStats(account.ID))
	svc.ResetOpenAIOAuth429DynamicStats(account.ID)
	svc.handle429(context.Background(), account, http.Header{}, nil)

	require.Zero(t, accountRepo.rateLimitCalls)
	require.True(t, svc.hasOpenAIOAuth429DynamicStats(account.ID))
}

func TestSetOpenAIOAuth429DynamicSettings_ValidatesEnabledValues(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})
	settings := *DefaultOpenAIOAuth429DynamicSettings()
	settings.Enabled = true
	settings.RatioThreshold = 0

	err := svc.SetOpenAIOAuth429DynamicSettings(context.Background(), &settings)
	require.ErrorContains(t, err, "ratio_threshold")
}

func TestOpenAIOAuth429Dynamic_UsesExactPlanTypePolicy(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	settings := *DefaultOpenAIOAuth429DynamicSettings()
	settings.PlanTypeSettings = []OpenAIOAuth429DynamicPlanTypeSettings{{
		PlanType: "plus",
		OpenAIOAuth429DynamicPolicy: OpenAIOAuth429DynamicPolicy{
			Enabled: true, WindowSeconds: 60, MinSamples: 2, Min429: 2,
			RatioThreshold: 1, BlockSeconds: 15,
			UsageWindow5hThresholdPercent: 100,
			UsageWindow7dThresholdPercent: 100,
		},
	}}
	storeOpenAIOAuth429DynamicSettings(t, settingRepo, settings)
	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	plus := &Account{
		ID: 49, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": " Plus "},
	}
	team := &Account{
		ID: 50, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": "team"},
	}
	svc.handle429(context.Background(), plus, http.Header{}, nil)
	svc.handle429(context.Background(), plus, http.Header{}, nil)
	svc.RecordOpenAIOAuthUpstreamOutcome(context.Background(), team, http.StatusTooManyRequests)

	require.Equal(t, 1, accountRepo.rateLimitCalls)
	require.Equal(t, plus.ID, accountRepo.lastRateLimitID)
	require.False(t, svc.hasOpenAIOAuth429DynamicStats(team.ID))
}

func TestOpenAIOAuth429Dynamic_RequiresConfiguredUsageWindow(t *testing.T) {
	accountRepo := &rateLimit429AccountRepoStub{}
	settingRepo := newMockSettingRepo()
	settings := *DefaultOpenAIOAuth429DynamicSettings()
	settings.Enabled = true
	settings.WindowSeconds = 60
	settings.MinSamples = 2
	settings.Min429 = 2
	settings.RatioThreshold = 1
	settings.BlockSeconds = 15
	settings.UsageWindowCheckEnabled = true
	settings.UsageWindow5hThresholdPercent = 90
	settings.UsageWindow7dThresholdPercent = 95
	storeOpenAIOAuth429DynamicSettings(t, settingRepo, settings)
	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)
	account := &Account{ID: 51, Platform: PlatformOpenAI, Type: AccountTypeOAuth, CreatedAt: time.Now()}

	used5h, window5h := 80.0, 300
	belowThreshold := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent: &used5h, PrimaryWindowMinutes: &window5h,
	}
	svc.RecordOpenAIOAuthUpstreamOutcome(context.Background(), account, http.StatusTooManyRequests, belowThreshold)
	svc.RecordOpenAIOAuthUpstreamOutcome(context.Background(), account, http.StatusTooManyRequests, belowThreshold)
	require.Zero(t, accountRepo.rateLimitCalls)

	used5h = 90
	atThreshold := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent: &used5h, PrimaryWindowMinutes: &window5h,
	}
	svc.RecordOpenAIOAuthUpstreamOutcome(context.Background(), account, http.StatusTooManyRequests, atThreshold)
	require.Equal(t, 1, accountRepo.rateLimitCalls)
}

func TestOpenAIOAuth429UsageWindowReached_MissingDataFallback(t *testing.T) {
	now := time.Now()
	policy := &OpenAIOAuth429DynamicPolicy{UsageWindowMissingDataFallbackSeconds: 300}

	require.False(t, openAIOAuth429UsageWindowReached(nil, nil, now.Add(-299*time.Second), now, policy))
	require.True(t, openAIOAuth429UsageWindowReached(nil, nil, now.Add(-301*time.Second), now, policy))
	require.False(t, openAIOAuth429UsageWindowReached(nil, nil, time.Time{}, now, policy))
}

func TestSetOpenAIOAuth429DynamicSettings_RejectsDuplicatePlanTypes(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})
	policy := *DefaultOpenAIOAuth429DynamicSettings().defaultPolicy()
	settings := *DefaultOpenAIOAuth429DynamicSettings()
	settings.PlanTypeSettings = []OpenAIOAuth429DynamicPlanTypeSettings{
		{PlanType: "Plus", OpenAIOAuth429DynamicPolicy: policy},
		{PlanType: " plus ", OpenAIOAuth429DynamicPolicy: policy},
	}

	err := svc.SetOpenAIOAuth429DynamicSettings(context.Background(), &settings)
	require.ErrorContains(t, err, "duplicate plan_type")
}
