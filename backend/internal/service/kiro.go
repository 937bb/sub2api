package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// KiroCachePolicy selects group-scoped response reuse or local prefix billing.
type KiroCachePolicy = domain.KiroResponseCachePolicy

// Cache policy is loaded by effective request group, never from account metadata.
func (s *GatewayService) kiroGroupCachePolicy(ctx context.Context, groupID int64) (KiroCachePolicy, error) {
	p := domain.DefaultKiroResponseCachePolicy()
	if groupID <= 0 {
		return p, nil
	}
	if s.groupRepo == nil {
		return p, errors.New("Kiro group repository unavailable")
	}
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return p, err
	}
	if group == nil {
		return p, errors.New("Kiro request group unavailable")
	}
	p = group.KiroResponseCache
	// Zero values occur in old cached snapshots and in-memory group fixtures.
	if p == (KiroCachePolicy{}) {
		p = domain.DefaultKiroResponseCachePolicy()
	}
	return p, p.Validate()
}

type KiroResponseCache interface {
	GetKiroResponse(context.Context, int64, string, KiroCachePolicy) ([]byte, error)
	PutKiroResponse(context.Context, int64, string, []byte, KiroCachePolicy) error
}

type KiroPrefixCache interface {
	LookupKiroPrefix(context.Context, int64, string, *kiro.PrefixPlan, KiroCachePolicy) (*kiro.PrefixPoint, error)
	CommitKiroPrefix(context.Context, int64, string, *kiro.PrefixPlan, *kiro.PrefixPoint, KiroCachePolicy) error
}

func normalizeKiroAccount(account *Account) error {
	if account == nil || account.Platform != PlatformKiro {
		return nil
	}
	if account.Type != AccountTypeOAuth {
		return errors.New("Kiro accounts require oauth credential import")
	}
	if account.ProxyID == nil || *account.ProxyID <= 0 {
		return infraerrors.BadRequest("KIRO_PROXY_REQUIRED", "Kiro requires a dedicated static proxy")
	}
	if len(AccountProxyFallbackIDs(account.Extra)) > 0 {
		return infraerrors.BadRequest("KIRO_STATIC_PROXY_REQUIRED", "Kiro does not support proxy rotation or fallback")
	}
	if account.ID == 0 {
		disabled, _ := account.Credentials["disabled"].(bool)
		if disabled || account.Credentials["status"] == "disabled" {
			account.Status = "inactive"
			account.Schedulable = false
		}
	}
	creds, err := kiro.Normalize(account.Credentials)
	if err != nil {
		return infraerrors.BadRequest("KIRO_INVALID_CREDENTIALS", err.Error())
	}
	account.Credentials = creds
	return nil
}

type KiroOAuthRefreshRepository interface {
	UpdateKiroOAuthCredentialsIfUnchanged(context.Context, int64, map[string]any, *int64, map[string]any) (bool, error)
}

type kiroRefreshExecutor struct {
	upstream HTTPUpstream
	// Manual refresh only forces the credential snapshot selected by the admin.
	// A concurrently rotated token must not be consumed a second time.
	forceCredentials map[string]any
}

func (e *kiroRefreshExecutor) CanRefresh(a *Account) bool {
	return a != nil && a.Platform == PlatformKiro && a.Type == AccountTypeOAuth && a.GetCredential("auth_method") != "api_key"
}
func (e *kiroRefreshExecutor) NeedsRefresh(a *Account, window time.Duration) bool {
	if e.forceCredentials != nil && reflect.DeepEqual(e.forceCredentials, a.Credentials) {
		return true
	}
	exp := a.GetCredentialAsTime("expires_at")
	return a.GetCredential("access_token") == "" || exp == nil || time.Until(*exp) <= window
}
func (e *kiroRefreshExecutor) CacheKey(a *Account) string {
	return "kiro:oauth:" + strconv.FormatInt(a.ID, 10)
}
func (e *kiroRefreshExecutor) Refresh(ctx context.Context, a *Account) (map[string]any, error) {
	endpoint, payload := kiro.RefreshPayload(a.Credentials)
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = prepareKiroHTTPRequest(req, a.Credentials, "", kiro.OperationRefresh)
	proxy, err := kiroProxy(a)
	if err != nil {
		return nil, err
	}
	resp, err := e.upstream.Do(req, proxy, a.ID, a.Concurrency)
	if err != nil {
		return nil, kiro.TransportError(kiro.OperationRefresh, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, kiro.ResponseError(kiro.OperationRefresh, resp)
	}
	var result struct {
		AccessToken  string `json:"accessToken"`
		IDToken      string `json:"idToken"`
		RefreshToken string `json:"refreshToken"`
		ProfileARN   string `json:"profileArn"`
		ExpiresIn    int    `json:"expiresIn"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil || result.AccessToken == "" {
		return nil, &kiro.UpstreamError{Operation: kiro.OperationRefresh, Reason: "invalid_response"}
	}
	if result.ExpiresIn <= 0 {
		result.ExpiresIn = 3600
	}
	out := shallowCopyMap(a.Credentials)
	out["access_token"] = result.AccessToken
	if result.IDToken != "" {
		out["id_token"] = result.IDToken
	}
	if result.RefreshToken != "" {
		out["refresh_token"] = result.RefreshToken
	}
	if result.ProfileARN != "" {
		out["profile_arn"] = result.ProfileARN
	}
	out["expires_at"] = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	return out, nil
}

// Kiro must never silently switch egress or fall back to a direct connection.
func kiroProxy(a *Account) (string, error) {
	if a == nil || a.ProxyID == nil || *a.ProxyID <= 0 {
		return "", errors.New("Kiro requires a dedicated static proxy")
	}
	p := a.Proxy
	if p == nil || p.ID != *a.ProxyID || !p.IsActive() || p.IsExpired(time.Now()) {
		return "", errors.New("Kiro account proxy is unavailable")
	}
	if (p.FallbackMode != "" && p.FallbackMode != FallbackModeNone) || p.BackupProxyID != nil || len(AccountProxyFallbackIDs(a.Extra)) > 0 || a.ProxyFallbackOriginID != nil {
		return "", errors.New("Kiro requires a fixed proxy without fallback")
	}
	if p.Host == "" || p.Port <= 0 || p.Port > 65535 {
		return "", errors.New("invalid Kiro proxy endpoint")
	}
	switch p.Protocol {
	case "http", "https", "socks5", "socks5h":
	default:
		return "", errors.New("unsupported Kiro proxy protocol")
	}
	return p.URL(), nil
}

func (s *adminServiceImpl) validateKiroProxy(ctx context.Context, a *Account) error {
	if a == nil || a.Platform != PlatformKiro {
		return nil
	}
	if err := normalizeKiroAccount(a); err != nil {
		return err
	}
	if s.proxyRepo == nil {
		return errors.New("proxy repository unavailable")
	}
	p, err := s.proxyRepo.GetByID(ctx, *a.ProxyID)
	if err != nil {
		return err
	}
	a.Proxy = p
	if _, err = kiroProxy(a); err != nil {
		return infraerrors.BadRequest("KIRO_STATIC_PROXY_REQUIRED", err.Error())
	}
	bound, err := s.proxyRepo.ListAccountSummariesByProxyID(ctx, p.ID)
	if err != nil {
		return err
	}
	for _, other := range bound {
		if other.ID != a.ID {
			return infraerrors.Conflict("KIRO_PROXY_IN_USE", "Kiro proxy is already assigned to another account")
		}
	}
	return nil
}
func (s *GatewayService) kiroAccessToken(ctx context.Context, a *Account) (*Account, string, error) {
	if a.GetCredential("auth_method") == "api_key" {
		return a, a.GetCredential("api_key"), nil
	}
	executor := &kiroRefreshExecutor{upstream: s.httpUpstream}
	if executor.NeedsRefresh(a, 3*time.Minute) {
		var err error
		a, err = s.refreshKiroAccount(ctx, a, executor)
		if err != nil {
			return a, "", err
		}
	}
	token := a.GetCredential("access_token")
	// Enterprise IdC may issue a distinct data-plane JWT.
	if a.GetCredential("auth_method") == "idc" && a.GetCredential("id_token") != "" {
		token = a.GetCredential("id_token")
	}
	if token == "" {
		return a, "", errors.New("Kiro access token is missing")
	}
	return a, token, nil
}
func kiroResult(out *kiro.Response, requested, mapped string, start time.Time, stream bool) *ForwardResult {
	result := &ForwardResult{KiroRefused: out.StopReason == "refusal", RequestID: out.ID, Model: requested, UpstreamModel: mapped, UpstreamResponseModel: mapped, Stream: stream, Duration: time.Since(start), Usage: ClaudeUsage{InputTokens: out.Usage.InputTokens, OutputTokens: out.Usage.OutputTokens, CacheReadInputTokens: out.Usage.CacheReadInputTokens, CacheCreationInputTokens: out.Usage.CacheCreationInputTokens}}
	if out.Usage.CacheCreation != nil {
		result.Usage.CacheCreation5mTokens = out.Usage.CacheCreation.Ephemeral5mInputTokens
		result.Usage.CacheCreation1hTokens = out.Usage.CacheCreation.Ephemeral1hInputTokens
	}
	return result
}
func (s *GatewayService) forwardKiro(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest) (*ForwardResult, error) {
	start := time.Now()
	if _, err := kiroProxy(account); err != nil {
		return nil, err
	}
	mapped := account.GetMappedModel(parsed.Model)
	keyValue, _ := c.Get("api_key")
	apiKey, _ := keyValue.(*APIKey)
	if apiKey == nil {
		return nil, errors.New("Kiro requires an authenticated API key scope")
	}
	groupID := derefGroupID(apiKey.GroupID)
	if parsed.GroupID != nil {
		groupID = *parsed.GroupID
	}
	scope := fmt.Sprintf("%d:%d:%d:%d", apiKey.UserID, apiKey.ID, groupID, account.ID)
	session := s.GenerateSessionHash(parsed)
	if session == "" {
		session = uuid.NewString()
	}
	policy, err := s.kiroGroupCachePolicy(ctx, groupID)
	if err != nil {
		return nil, err
	}
	cache, cacheOK := s.cache.(KiroResponseCache)
	identity := sha256.Sum256([]byte(account.GetCredential("refresh_token") + account.GetCredential("api_key")))
	cacheScope := fmt.Sprintf("%s:%s:%s:%s:%x", scope, mapped, kiro.EnvironmentKey(account.Credentials), account.GetCredential("profile_arn"), identity)
	cacheKey, eligible := kiro.CacheKey(parsed.Body.Bytes(), cacheScope)
	useCache := cacheOK && eligible && policy.Enabled && policy.Mode != "prefix" && policy.TargetPercent > 0
	c.Header("X-Sub2api-Cache", "bypass")
	c.Header("X-Sub2api-Usage-Source", "estimated")
	finalize := kiroCacheFinalizer(ctx, cache, groupID, cacheKey, policy, useCache)
	if policy.PrefixEnabled() {
		prefixScope := fmt.Sprintf("%s:%s:%s:%s:%x", scope, kiro.ModelID(mapped), kiro.EnvironmentKey(account.Credentials), account.GetCredential("profile_arn"), identity)
		finalize = s.kiroPrefixFinalizer(ctx, groupID, prefixScope, parsed.Body.Bytes(), mapped, policy)
		c.Header("X-Sub2api-Cache", "local-prefix-metering")
		c.Header("X-Sub2api-Cache-Mode", "local_prefix")
	}
	emit := func(name string, event any) error {
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Accel-Buffering", "no")
		if _, err = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", name, encoded); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	if useCache {
		c.Header("X-Sub2api-Cache", "miss")
		cached, cacheErr := cache.GetKiroResponse(ctx, groupID, cacheKey, policy)
		if cacheErr == nil && len(cached) > 0 {
			var out kiro.Response
			if json.Unmarshal(cached, &out) == nil && len(out.Content) > 0 {
				out.ID = "msg_" + uuid.NewString()
				kiroCacheRead(&out)
				c.Header("X-Sub2api-Cache", "hit")
				c.Header("X-Sub2api-Usage-Source", "local_cache")
				if parsed.OnUpstreamAccepted != nil {
					parsed.OnUpstreamAccepted()
				}
				if parsed.Stream {
					err = kiro.Replay(&out, emit)
				} else {
					c.JSON(http.StatusOK, &out)
				}
				return kiroResult(&out, parsed.Model, mapped, start, parsed.Stream), err
			}
		}
	}
	resp, inputTokens, toolNames, err := s.requestKiro(ctx, account, parsed.Body.Bytes(), mapped, kiro.StableID(scope, session))
	if err != nil {
		if infraerrors.IsBadRequest(err) {
			c.JSON(400, gin.H{"type": "error", "error": gin.H{"type": "invalid_request_error", "message": err.Error()}})
			return nil, err
		}
		return nil, s.handleUpstreamTransportError(ctx, c, account, err, OpsUpstreamErrorEvent{UpstreamURL: kiro.Endpoint(account.Credentials)})
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if s.rateLimitService != nil {
			s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, raw, mapped)
		}
		if s.shouldFailoverUpstreamError(resp.StatusCode) || resp.StatusCode == 402 {
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: []byte(`{"error":{"message":"Kiro upstream rejected the request"}}`), ResponseHeaders: resp.Header.Clone()}
		}
		upstreamErr := kiro.GenerationResponseError(resp.StatusCode, raw)
		c.JSON(resp.StatusCode, gin.H{"type": "error", "error": gin.H{"type": "api_error", "message": upstreamErr.Error()}})
		return nil, upstreamErr
	}
	if parsed.OnUpstreamAccepted != nil {
		parsed.OnUpstreamAccepted()
	}
	var emitter kiro.Emit
	if parsed.Stream {
		emitter = emit
	}
	out, err := kiro.DecodeWithOptions(resp.Body, parsed.Model, inputTokens, emitter, finalize, kiro.DecodeOptionsForRequest(parsed.Body.Bytes()), toolNames)
	if err != nil {
		if writeKiroEmptyResponse(c, err, "messages") {
			return nil, err
		}
		if parsed.Stream && c.Writer.Written() {
			_ = emit("error", gin.H{"type": "error", "error": gin.H{"type": "api_error", "message": "Kiro stream interrupted"}})
		}
		if out != nil && len(out.Content) > 0 {
			return kiroResult(out, parsed.Model, mapped, start, parsed.Stream), err
		}
		return nil, err
	}
	if !parsed.Stream {
		c.Header("X-Sub2api-Usage-Source", out.UsageSource)
		if out.Credits != nil {
			c.Header("X-Kiro-Credits", strconv.FormatFloat(*out.Credits, 'f', -1, 64))
		}
		c.JSON(http.StatusOK, out)
	}
	result := kiroResult(out, parsed.Model, mapped, start, parsed.Stream)
	result.UpstreamHeaders = resp.Header.Clone()
	return result, nil
}

// refreshKiroAccount shares locking, durable token rotation and fixed egress
// between inference and the admin refresh action.
func (s *GatewayService) refreshKiroAccount(ctx context.Context, a *Account, executor *kiroRefreshExecutor) (*Account, error) {
	if s.claudeTokenProvider == nil || s.claudeTokenProvider.refreshAPI == nil {
		return a, errors.New("Kiro refresh service is unavailable")
	}
	refreshCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		result, err := s.claudeTokenProvider.refreshAPI.RefreshIfNeeded(withOAuthRefreshRequestPath(refreshCtx), a, executor, 3*time.Minute)
		if err != nil {
			return a, err
		}
		if result == nil {
			return a, errors.New("Kiro refresh returned no account")
		}
		if !result.LockHeld {
			if result.Account != nil {
				a = result.Account
			}
			break
		}
		if err := sleepWithContext(refreshCtx, 100*time.Millisecond); err != nil {
			return a, err
		}
	}
	return a, nil
}

func (s *AccountTestService) RefreshKiroAccount(ctx context.Context, a *Account) (*Account, error) {
	if s == nil || a == nil || a.Platform != PlatformKiro || a.Type != AccountTypeOAuth {
		return nil, infraerrors.BadRequest("KIRO_REFRESH_UNAVAILABLE", "Kiro refresh service is unavailable")
	}
	if a.GetCredential("auth_method") == "api_key" {
		return nil, infraerrors.BadRequest("KIRO_REFRESH_UNAVAILABLE", "Kiro API keys do not support OAuth refresh")
	}
	gateway := &GatewayService{httpUpstream: s.httpUpstream, claudeTokenProvider: s.claudeTokenProvider}
	executor := &kiroRefreshExecutor{upstream: s.httpUpstream, forceCredentials: shallowCopyMap(a.Credentials)}
	return gateway.refreshKiroAccount(ctx, a, executor)
}

// KiroAdminErrorMessage exposes only structured, credential-free diagnostics.
func KiroAdminErrorMessage(err error) string {
	var upstream *kiro.UpstreamError
	if errors.As(err, &upstream) {
		return upstream.Error()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Kiro credential refresh timed out"
	}
	if errors.Is(err, context.Canceled) {
		return "Kiro operation canceled"
	}
	return "Kiro operation failed; check account credentials and its assigned static proxy"
}
