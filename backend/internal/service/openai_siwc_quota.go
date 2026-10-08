package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/imroc/req/v3"
)

const siwcQuotaAccessTokenKey = "siwc_quota_access_token"

type siwcQuotaCacheEntry struct {
	fingerprint string
	queriedAt   time.Time
	usage       *UsageInfo
}

type siwcWebQuotaWindow struct {
	UsedPercent        *float64 `json:"used_percent"`
	LimitWindowSeconds int64    `json:"limit_window_seconds"`
	ResetAfterSeconds  int64    `json:"reset_after_seconds"`
	ResetAt            int64    `json:"reset_at"`
}

// Web and SIWC grants are independent. Never use the SIWC token as a fallback,
// refresh the web token with the SIWC refresh token, or redeem reset credits.
func (s *OpenAIQuotaService) querySIWCWebUsage(ctx context.Context, account *Account) (*UsageInfo, error) {
	fail := func(code, message string) (*UsageInfo, error) {
		return nil, infraerrors.New(http.StatusBadGateway, code, message)
	}
	if !account.IsOpenAISiwc() || account.GetCredential(siwcQuotaAccessTokenKey) == "" {
		return fail("siwc_quota_web_token_required", "Configure this account's ChatGPT web access token for quota queries")
	}
	email := strings.TrimSpace(account.GetCredential("email"))
	if email == "" {
		return fail("siwc_quota_identity_missing", "Reauthorize SIWC to verify the account email before querying quota")
	}
	if s == nil || s.privacyClientFactory == nil {
		return fail("siwc_quota_unavailable", "Quota query service is unavailable")
	}
	proxyURL := ""
	if account.ProxyID != nil {
		proxy := account.Proxy
		if proxy == nil && s.proxyRepo != nil {
			proxy, _ = s.proxyRepo.GetByID(ctx, *account.ProxyID)
		}
		if proxy == nil || strings.TrimSpace(proxy.URL()) == "" {
			return fail("siwc_quota_proxy_unavailable", "The configured account proxy is unavailable")
		}
		proxyURL = proxy.URL()
	}
	client, err := s.privacyClientFactory(proxyURL)
	if err != nil || client == nil {
		return fail("siwc_quota_unavailable", "Could not create the quota query client")
	}
	client = client.Clone().SetCookieJar(nil).SetRedirectPolicy(req.NoRedirectPolicy())
	resp, err := client.R().SetContext(ctx).SetRetryCount(0).SetHeaders(map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(account.GetCredential(siwcQuotaAccessTokenKey)),
		"Accept":        "application/json",
	}).Get(chatGPTUsageURL)
	if err != nil {
		// Do not include transport errors or upstream bodies that might echo credentials.
		return fail("siwc_quota_unavailable", "The web quota request failed")
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fail("siwc_quota_web_token_invalid", "The quota web token expired or was rejected; replace it with a current web access token")
	case http.StatusForbidden:
		return fail("siwc_quota_web_forbidden", "The web quota request was denied")
	case http.StatusTooManyRequests:
		return fail("siwc_quota_query_limited", "Quota queries are temporarily rate limited; retry later")
	case http.StatusOK:
	default:
		return fail("siwc_quota_unavailable", "The web quota endpoint returned an unexpected status")
	}
	var payload struct {
		Email     string `json:"email"`
		RateLimit *struct {
			PrimaryWindow   *siwcWebQuotaWindow `json:"primary_window"`
			SecondaryWindow *siwcWebQuotaWindow `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if json.Unmarshal(resp.Bytes(), &payload) != nil || payload.RateLimit == nil {
		return fail("siwc_quota_invalid_response", "The upstream did not return a quota snapshot")
	}
	// Only the authenticated upstream response can establish the queried identity.
	if payload.Email == "" || !strings.EqualFold(strings.TrimSpace(payload.Email), email) {
		return fail("siwc_quota_account_mismatch", "The web token must belong to the same account as the SIWC authorization")
	}
	now := time.Now()
	usage := &UsageInfo{Source: "active", UpdatedAt: &now}
	for _, window := range []*siwcWebQuotaWindow{payload.RateLimit.PrimaryWindow, payload.RateLimit.SecondaryWindow} {
		if window == nil || window.UsedPercent == nil || math.IsNaN(*window.UsedPercent) || math.IsInf(*window.UsedPercent, 0) || *window.UsedPercent < 0 {
			continue
		}
		progress := &UsageProgress{Utilization: *window.UsedPercent}
		var reset time.Time
		if window.ResetAt > 0 {
			reset = time.Unix(window.ResetAt, 0)
		} else if window.ResetAfterSeconds > 0 && window.ResetAfterSeconds <= 31*24*3600 {
			reset = now.Add(time.Duration(window.ResetAfterSeconds) * time.Second)
		}
		if !reset.IsZero() {
			if !reset.After(now) {
				continue
			}
			progress.ResetsAt = &reset
			progress.RemainingSeconds = int(reset.Sub(now).Seconds())
		}
		switch window.LimitWindowSeconds {
		case 5 * 3600:
			usage.FiveHour = progress
		case 7 * 24 * 3600:
			usage.SevenDay = progress
		}
	}
	if usage.FiveHour == nil && usage.SevenDay == nil {
		return fail("siwc_quota_invalid_response", "No current five-hour or weekly quota window was returned")
	}
	return usage, nil
}

func (s *AccountUsageService) getSIWCUsage(ctx context.Context, account *Account, force bool) *UsageInfo {
	if strings.TrimSpace(account.GetCredential(siwcQuotaAccessTokenKey)) == "" {
		return &UsageInfo{Source: "active", ErrorCode: "siwc_quota_web_token_required",
			Error: "Configure this account's ChatGPT web access token in Edit account to query quota"}
	}
	// Token rotation, account identity or proxy changes invalidate both positive and
	// negative caches. No raw tokens are used as map keys or exposed in the response.
	proxyIdentity := "direct"
	if account.ProxyID != nil {
		proxyIdentity = strconv.FormatInt(*account.ProxyID, 10)
	}
	digest := sha256.Sum256([]byte(account.GetCredential(siwcQuotaAccessTokenKey) + "\x00" +
		account.GetCredential("subject") + "\x00" + account.GetCredential("email") + "\x00" + proxyIdentity))
	fingerprint := hex.EncodeToString(digest[:])
	load := func() *UsageInfo {
		if s.cache == nil {
			return nil
		}
		if raw, ok := s.cache.siwcQuotaCache.Load(account.ID); ok {
			cached, ok := raw.(*siwcQuotaCacheEntry)
			if !ok || cached.fingerprint != fingerprint {
				return nil
			}
			ttl := apiCacheTTL
			if cached.usage.ErrorCode != "" {
				ttl = apiErrorCacheTTL
			}
			if time.Since(cached.queriedAt) < ttl && (!force || cached.usage.ErrorCode != "") {
				return cloneSIWCQuotaUsage(cached.usage)
			}
		}
		return nil
	}
	query := func() (any, error) {
		if cached := load(); cached != nil {
			return cached, nil
		}
		usage, err := s.openAIQuotaService.querySIWCWebUsage(ctx, account)
		if err != nil {
			usage = &UsageInfo{Source: "active", ErrorCode: infraerrors.Reason(err), Error: infraerrors.Message(err)}
		}
		if s.cache != nil {
			s.cache.siwcQuotaCache.Store(account.ID, &siwcQuotaCacheEntry{fingerprint: fingerprint, queriedAt: time.Now(), usage: usage})
		}
		return usage, nil
	}
	if cached := load(); cached != nil {
		return cached
	}
	var result any
	if s.cache != nil {
		result, _, _ = s.cache.siwcQuotaFlight.Do(strconv.FormatInt(account.ID, 10)+":"+fingerprint, query)
	} else {
		result, _ = query()
	}
	return cloneSIWCQuotaUsage(result.(*UsageInfo))
}

func cloneSIWCQuotaUsage(usage *UsageInfo) *UsageInfo {
	result := *usage
	cloneWindow := func(window *UsageProgress) *UsageProgress {
		if window == nil {
			return nil
		}
		cloned := *window
		if window.ResetsAt != nil {
			cloned.RemainingSeconds = int(time.Until(*window.ResetsAt).Seconds())
			if cloned.RemainingSeconds <= 0 {
				return nil // An elapsed window is unknown until queried again, not 0% used.
			}
		}
		return &cloned
	}
	result.FiveHour, result.SevenDay = cloneWindow(usage.FiveHour), cloneWindow(usage.SevenDay)
	return &result
}

// Refresh local billing statistics independently of the web-token quota cache.
// Missing quota must not suppress accounting or fabricate a 0% utilization bar.
func (s *AccountUsageService) attachSIWCLocalUsage(ctx context.Context, accountID int64, usage *UsageInfo, now time.Time) {
	if usage == nil {
		return
	}
	if s.usageLogRepo == nil {
		usage.LocalUsageError = "local_usage_unavailable"
		return
	}
	readWindow := func(quota *UsageProgress, duration time.Duration) *LocalUsageWindow {
		start := now.Add(-duration)
		rolling := true
		if quota != nil && quota.ResetsAt != nil && quota.ResetsAt.After(now) {
			candidate := quota.ResetsAt.Add(-duration)
			if !candidate.After(now) {
				start, rolling = candidate, false
			}
		}
		stats, err := s.usageLogRepo.GetAccountWindowStats(ctx, accountID, start)
		if err != nil || stats == nil {
			usage.LocalUsageError = "local_usage_unavailable"
			return nil
		}
		return &LocalUsageWindow{StartAt: start, Rolling: rolling, Stats: windowStatsFromAccountStats(stats)}
	}
	usage.LocalFiveHour = readWindow(usage.FiveHour, 5*time.Hour)
	usage.LocalSevenDay = readWindow(usage.SevenDay, 7*24*time.Hour)
}
