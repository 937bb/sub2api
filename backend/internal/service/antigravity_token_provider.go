package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	antigravityTokenRefreshSkew = 3 * time.Minute
	antigravityTokenCacheSkew   = 5 * time.Minute
	antigravityBackfillCooldown = 5 * time.Minute
	// antigravityRequestRefreshTimeout 请求路径上 token 刷新的最大等待时间。
	// 超过此时间直接放弃刷新、标记账号临时不可调度并触发 failover，
	// 让后台 TokenRefreshService 在下个周期继续重试。
	antigravityRequestRefreshTimeout = 8 * time.Second
)

// AntigravityTokenCache token cache interface.
type AntigravityTokenCache = GeminiTokenCache

// AntigravityTokenProvider manages access_token for antigravity accounts.
type AntigravityTokenProvider struct {
	accountRepo             AccountRepository
	tokenCache              AntigravityTokenCache
	antigravityOAuthService *AntigravityOAuthService
	backfillCooldown        sync.Map // key: accountID -> last attempt time
	refreshAPI              *OAuthRefreshAPI
	executor                OAuthRefreshExecutor
	refreshPolicy           ProviderRefreshPolicy
	tempUnschedCache        TempUnschedCache // 用于同步更新 Redis 临时不可调度缓存
}

func NewAntigravityTokenProvider(
	accountRepo AccountRepository,
	tokenCache AntigravityTokenCache,
	antigravityOAuthService *AntigravityOAuthService,
) *AntigravityTokenProvider {
	return &AntigravityTokenProvider{
		accountRepo:             accountRepo,
		tokenCache:              tokenCache,
		antigravityOAuthService: antigravityOAuthService,
		refreshPolicy:           AntigravityProviderRefreshPolicy(),
	}
}

// SetRefreshAPI injects unified OAuth refresh API and executor.
func (p *AntigravityTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

// SetRefreshPolicy injects caller-side refresh policy.
func (p *AntigravityTokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

// SetTempUnschedCache injects temp unschedulable cache for immediate scheduler sync.
func (p *AntigravityTokenProvider) SetTempUnschedCache(cache TempUnschedCache) {
	p.tempUnschedCache = cache
}

// GetAccessToken returns a valid access_token.
func (p *AntigravityTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	requestAccount := account
	if account.Platform != PlatformAntigravity {
		return "", errors.New("not an antigravity account")
	}

	// upstream accounts use static api_key and never refresh oauth token.
	if account.Type == AccountTypeUpstream {
		apiKey := account.GetCredential("api_key")
		if apiKey == "" {
			return "", errors.New("upstream account missing api_key in credentials")
		}
		return apiKey, nil
	}
	if account.Type != AccountTypeOAuth {
		return "", errors.New("not an antigravity oauth account")
	}

	account = p.latestAccountForTokenUse(ctx, requestAccount, account)
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= antigravityTokenRefreshSkew

	cacheKey := AntigravityTokenCacheKey(account)

	// 1) Try cache first.
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}

	// 2) Refresh if needed (pre-expiry skew).
	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		// 请求路径使用短超时，避免代理不通时阻塞过久（后台刷新服务会继续重试）
		refreshCtx, cancel := context.WithTimeout(ctx, antigravityRequestRefreshTimeout)
		defer cancel()
		result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, antigravityTokenRefreshSkew)
		if err != nil {
			// 标记账号临时不可调度，避免后续请求继续命中
			p.markTempUnschedulable(account, err)
			if p.refreshPolicy.OnRefreshError == ProviderRefreshErrorReturn {
				return "", err
			}
		} else if result.LockHeld {
			if p.refreshPolicy.OnLockHeld == ProviderLockHeldWaitForCache && p.tokenCache != nil {
				if token, cacheErr := p.tokenCache.GetAccessToken(ctx, cacheKey); cacheErr == nil && strings.TrimSpace(token) != "" {
					return token, nil
				}
			}
			// default policy: continue with existing token.
		} else {
			if result.Account != nil {
				account = result.Account
				syncAccountCredentials(requestAccount, account)
			}
			expiresAt = account.GetCredentialAsTime("expires_at")
		}
	} else if needsRefresh && p.tokenCache != nil {
		// Backward-compatible test path when refreshAPI is not injected.
		locked, err := p.tokenCache.AcquireRefreshLock(ctx, cacheKey, 30*time.Second)
		if err == nil && locked {
			defer func() { _ = p.tokenCache.ReleaseRefreshLock(ctx, cacheKey) }()
		}
	}

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	// Backfill project_id online when missing, with cooldown to avoid hammering.
	if err := p.BackfillProjectIDIfMissing(ctx, account, accessToken); err != nil {
		slog.Debug("antigravity_project_id_backfill_skipped",
			"account_id", account.ID,
			"error", err,
		)
	}
	syncAccountCredentials(requestAccount, account)

	cacheKey = AntigravityTokenCacheKey(account)

	// 3) Populate cache with TTL.
	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			slog.Debug("antigravity_token_version_stale_use_latest", "account_id", account.ID)
			account = latestAccount
			syncAccountCredentials(requestAccount, account)
			accessToken = latestAccount.GetCredential("access_token")
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
			}
		} else {
			ttl := 30 * time.Minute
			if expiresAt != nil {
				until := time.Until(*expiresAt)
				switch {
				case until > antigravityTokenCacheSkew:
					ttl = until - antigravityTokenCacheSkew
				case until > 0:
					ttl = until
				default:
					ttl = time.Minute
				}
			}
			_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
		}
	}

	return accessToken, nil
}

// BackfillProjectIDIfMissing preserves the legacy online project_id discovery
// for no-fallback OAuth accounts. Configured fallback accounts deliberately
// remain project_id-free so their cache key stays account-scoped.
func (p *AntigravityTokenProvider) BackfillProjectIDIfMissing(ctx context.Context, account *Account, accessToken string) error {
	if account == nil {
		return errors.New("account is nil")
	}
	if account.Platform != PlatformAntigravity || account.Type != AccountTypeOAuth {
		return nil
	}
	if strings.TrimSpace(account.GetCredential("project_id")) != "" || hasAntigravityOAuthProjectFallback(account) {
		return nil
	}
	if p == nil {
		return errAntigravityProjectIDRequired
	}
	originalAccount := account
	account = p.latestAccountForProjectBackfill(ctx, account)
	if strings.TrimSpace(account.GetCredential("project_id")) != "" || hasAntigravityOAuthProjectFallback(account) {
		syncAccountCredentials(originalAccount, account)
		return nil
	}
	if p.antigravityOAuthService == nil {
		return errAntigravityProjectIDRequired
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return errors.New("access_token not found in credentials")
	}
	if !p.shouldAttemptBackfill(account.ID) {
		return errAntigravityProjectIDRequired
	}

	p.markBackfillAttempted(account.ID)
	projectID, err := p.antigravityOAuthService.FillProjectID(ctx, account, accessToken)
	if err != nil {
		return err
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return errAntigravityProjectIDRequired
	}
	credentials := cloneCredentials(account.Credentials)
	credentials["project_id"] = projectID
	account.Credentials = credentials
	if updateErr := persistAccountCredentials(ctx, p.accountRepo, account, account.Credentials); updateErr != nil {
		slog.Warn("antigravity_project_id_backfill_persist_failed",
			"account_id", account.ID,
			"error", updateErr,
		)
	}
	syncAccountCredentials(originalAccount, account)
	return nil
}

func (p *AntigravityTokenProvider) latestAccountForProjectBackfill(ctx context.Context, account *Account) *Account {
	if p == nil || p.accountRepo == nil || account == nil || account.ID == 0 {
		return account
	}
	latest, err := p.accountRepo.GetByID(ctx, account.ID)
	if err != nil || latest == nil {
		return account
	}
	if latest.Platform != account.Platform || latest.Type != account.Type {
		return account
	}
	syncAccountCredentials(account, latest)
	return latest
}

func (p *AntigravityTokenProvider) latestAccountForTokenUse(ctx context.Context, requestAccount, account *Account) *Account {
	if p == nil || p.accountRepo == nil || account == nil || account.ID == 0 {
		return account
	}
	latest, err := p.accountRepo.GetByID(ctx, account.ID)
	if err != nil || latest == nil {
		return account
	}
	if latest.Platform != account.Platform || latest.Type != account.Type {
		return account
	}
	syncAccountCredentials(requestAccount, latest)
	return latest
}

func syncAccountCredentials(target, source *Account) {
	if target == nil || source == nil || target == source {
		return
	}
	target.Credentials = cloneCredentials(source.Credentials)
}

// shouldAttemptBackfill checks backfill cooldown.
func (p *AntigravityTokenProvider) shouldAttemptBackfill(accountID int64) bool {
	if v, ok := p.backfillCooldown.Load(accountID); ok {
		if lastAttempt, ok := v.(time.Time); ok {
			return time.Since(lastAttempt) > antigravityBackfillCooldown
		}
	}
	return true
}

// markTempUnschedulable 在请求路径上 token 刷新失败时标记账号临时不可调度。
// 同时写 DB 和 Redis 缓存，确保调度器立即跳过该账号。
// 使用 background context 因为请求 context 可能已超时。
func (p *AntigravityTokenProvider) markTempUnschedulable(account *Account, refreshErr error) {
	if p.accountRepo == nil || account == nil {
		return
	}
	now := time.Now()
	until := now.Add(tokenRefreshTempUnschedDuration)
	reason := "token refresh failed on request path: " + refreshErr.Error()
	bgCtx := context.Background()
	if err := p.accountRepo.SetTempUnschedulable(bgCtx, account.ID, until, reason); err != nil {
		slog.Warn("antigravity_token_provider.set_temp_unschedulable_failed",
			"account_id", account.ID,
			"error", err,
		)
		return
	}
	slog.Warn("antigravity_token_provider.temp_unschedulable_set",
		"account_id", account.ID,
		"until", until.Format(time.RFC3339),
		"reason", reason,
	)
	// 同步写 Redis 缓存，调度器立即生效
	if p.tempUnschedCache != nil {
		state := &TempUnschedState{
			UntilUnix:       until.Unix(),
			TriggeredAtUnix: now.Unix(),
			ErrorMessage:    reason,
		}
		if err := p.tempUnschedCache.SetTempUnsched(bgCtx, account.ID, state); err != nil {
			slog.Warn("antigravity_token_provider.temp_unsched_cache_set_failed",
				"account_id", account.ID,
				"error", err,
			)
		}
	}
}

func (p *AntigravityTokenProvider) markBackfillAttempted(accountID int64) {
	p.backfillCooldown.Store(accountID, time.Now())
}

func AntigravityTokenCacheKey(account *Account) string {
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID != "" {
		return "ag:" + projectID
	}
	return "ag:account:" + strconv.FormatInt(account.ID, 10)
}
