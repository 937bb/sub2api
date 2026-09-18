package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const (
	openAICodexTurnStateScanWorkers       = 4
	openAICodexTurnStateScanQueueSize     = 256
	openAICodexTurnStateScanSweepInterval = 30 * time.Second
	openAICodexTurnStateScanRefreshBefore = 5 * time.Minute
	openAICodexTurnStateActiveUsageWindow = time.Hour
	openAICodexTurnStateScanMaxErrorBytes = 240
)

type openAICodexTurnStateScanJob struct {
	accountID int64
	model     string
	force     bool
}

type openAICodexTurnStateHarvestResult struct {
	stateValue    string
	stateLength   int
	upstreamModel string
	officialModel string
	sessionID     string
	upstreamOK    bool
	statusCode    int
	errorMessage  string
}

type openAICodexTurnStateScanner struct {
	repo        OpenAICodexTurnStateScannerRepository
	accountRepo AccountRepository
	gateway     *OpenAIGatewayService

	queue chan openAICodexTurnStateScanJob

	mu       sync.Mutex
	inFlight map[openAICodexTurnStateBucketKey]struct{}
	cancel   context.CancelFunc
	done     chan struct{}
	proxySeq atomic.Uint64
}

func newOpenAICodexTurnStateScanner(repo OpenAICodexTurnStateScannerRepository, accountRepo AccountRepository, gateway *OpenAIGatewayService) *openAICodexTurnStateScanner {
	if repo == nil || accountRepo == nil || gateway == nil {
		return nil
	}
	return &openAICodexTurnStateScanner{
		repo:        repo,
		accountRepo: accountRepo,
		gateway:     gateway,
		queue:       make(chan openAICodexTurnStateScanJob, openAICodexTurnStateScanQueueSize),
		inFlight:    make(map[openAICodexTurnStateBucketKey]struct{}),
	}
}

func (s *openAICodexTurnStateScanner) Start(parent context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.mu.Unlock()

	go func() {
		defer close(s.done)
		var workers sync.WaitGroup
		for range openAICodexTurnStateScanWorkers {
			workers.Add(1)
			go func() {
				defer workers.Done()
				s.runWorker(ctx)
			}()
		}
		s.enqueueSweep(ctx)
		ticker := time.NewTicker(openAICodexTurnStateScanSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				workers.Wait()
				return
			case <-ticker.C:
				s.enqueueSweep(ctx)
			}
		}
	}()
}

func (s *openAICodexTurnStateScanner) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (s *openAICodexTurnStateScanner) Enqueue(accountID int64, model string, force bool) bool {
	key, ok := newOpenAICodexTurnStateBucketKey(accountID, model)
	if s == nil || !ok {
		return false
	}
	s.mu.Lock()
	if _, exists := s.inFlight[key]; exists {
		s.mu.Unlock()
		return false
	}
	s.inFlight[key] = struct{}{}
	s.mu.Unlock()

	select {
	case s.queue <- openAICodexTurnStateScanJob{accountID: key.accountID, model: key.model, force: force}:
		return true
	default:
		s.finish(key)
		return false
	}
}

func (s *openAICodexTurnStateScanner) finish(key openAICodexTurnStateBucketKey) {
	s.mu.Lock()
	delete(s.inFlight, key)
	s.mu.Unlock()
}

func (s *openAICodexTurnStateScanner) targetModels() []string {
	models := []string{openAICodexTicketDefaultModel, openAICodexTicketDefaultSolModel}
	if s != nil && s.gateway != nil {
		models = s.gateway.openAICodexTicketConfig().Models
	}
	result := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = normalizeOpenAICodexTurnStateModel(model)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	if len(result) == 0 {
		return []string{openAICodexTicketDefaultModel, openAICodexTicketDefaultSolModel}
	}
	return result
}

func (s *openAICodexTurnStateScanner) enqueueAccount(accountID int64, force bool) int {
	queued := 0
	for _, model := range s.targetModels() {
		if s.Enqueue(accountID, model, force) {
			queued++
		}
	}
	return queued
}

func (s *openAICodexTurnStateScanner) enqueueSweep(ctx context.Context) {
	now := time.Now()
	accounts, err := s.listRecentlyUsedAccounts(ctx, now)
	if err != nil {
		log.WithError(err).Warn("failed to list Codex turn-state scan accounts")
		return
	}
	for _, account := range accounts {
		for _, model := range s.targetModels() {
			model = openAICodexTurnStateUpstreamModel(account, model)
			if model == "" {
				continue
			}
			if expiresAt, ok := s.gateway.getOpenAICodexTurnStatePool().preferredExpiryForBucket(account.ID, model); ok && expiresAt.After(time.Now().Add(openAICodexTurnStateScanRefreshBefore)) {
				continue
			}
			scan, scanErr := s.repo.GetOpenAICodexTurnStateScan(ctx, account.ID, model)
			if scanErr != nil || (scan != nil && scan.NextAttemptAt != nil && scan.NextAttemptAt.After(time.Now())) {
				continue
			}
			s.Enqueue(account.ID, model, false)
		}
	}
}

func (s *openAICodexTurnStateScanner) listRecentlyUsedAccounts(ctx context.Context, now time.Time) ([]*Account, error) {
	ids, err := s.repo.ListRecentlyUsedOpenAICodexAccountIDs(ctx, now.Add(-openAICodexTurnStateActiveUsageWindow))
	if err != nil {
		return nil, err
	}
	accounts := make([]*Account, 0, len(ids))
	for _, id := range ids {
		account, accountErr := s.accountRepo.GetByID(ctx, id)
		if accountErr != nil {
			return nil, fmt.Errorf("load recently used Codex account %d: %w", id, accountErr)
		}
		if isRecentlyUsedOpenAICodexAccount(account, now) {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func isRecentlyUsedOpenAICodexAccount(account *Account, now time.Time) bool {
	if !isEligibleOpenAICodexTurnStateAccount(account, now) || account.LastUsedAt == nil ||
		account.LastUsedAt.Before(now.Add(-openAICodexTurnStateActiveUsageWindow)) {
		return false
	}
	return true
}

func isEligibleOpenAICodexTurnStateAccount(account *Account, now time.Time) bool {
	if account == nil || account.ParentAccountID != nil || !account.IsOpenAIOAuthLike() ||
		account.Status != StatusActive || !account.Schedulable {
		return false
	}
	if account.ExpiresAt != nil && !account.ExpiresAt.After(now) {
		return false
	}
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now) {
		return false
	}
	if account.OverloadUntil != nil && account.OverloadUntil.After(now) {
		return false
	}
	return account.RateLimitResetAt == nil || !account.RateLimitResetAt.After(now)
}

func (s *openAICodexTurnStateScanner) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.queue:
			key, _ := newOpenAICodexTurnStateBucketKey(job.accountID, job.model)
			s.runJob(ctx, job)
			s.finish(key)
		}
	}
}

func (s *openAICodexTurnStateScanner) runJob(ctx context.Context, job openAICodexTurnStateScanJob) {
	account, err := s.accountRepo.GetByID(ctx, job.accountID)
	now := time.Now()
	if err != nil || !isEligibleOpenAICodexTurnStateAccount(account, now) ||
		(!job.force && !isRecentlyUsedOpenAICodexAccount(account, now)) {
		return
	}
	upstreamModel := openAICodexTurnStateUpstreamModel(account, job.model)
	if upstreamModel == "" {
		return
	}
	if !job.force {
		if expiresAt, ok := s.gateway.getOpenAICodexTurnStatePool().preferredExpiryForBucket(account.ID, upstreamModel); ok && expiresAt.After(time.Now().Add(openAICodexTurnStateScanRefreshBefore)) {
			return
		}
	}

	previous, _ := s.repo.GetOpenAICodexTurnStateScan(ctx, account.ID, upstreamModel)
	scan := &OpenAICodexTurnStateScan{AccountID: account.ID, Model: upstreamModel, Status: "running"}
	if previous != nil {
		*scan = *previous
		scan.Status = "running"
	}
	attemptAt := time.Now()
	scan.AttemptCount++
	scan.LastAttemptAt = &attemptAt
	scan.LastError = ""
	scan.NextAttemptAt = nil
	_ = s.repo.UpsertOpenAICodexTurnStateScan(ctx, scan)

	proxy := s.selectProxy(ctx)
	proxyURL := ""
	if proxy != nil {
		proxyURL = proxy.ProxyURL
		scan.LastProxyURL = maskOpenAICodexTurnStateProxyURL(proxy.ProxyURL)
		if proxy.Source == "state" && proxy.ID > 0 {
			scan.LastProxyID = &proxy.ID
		} else {
			scan.LastProxyID = nil
		}
	} else {
		scan.LastProxyID = nil
		scan.LastProxyURL = ""
	}
	result := s.gateway.harvestOpenAICodexTurnState(ctx, account, upstreamModel, proxyURL)
	scan.LastStateLength = result.stateLength
	if proxy != nil && proxy.Source == "state" {
		s.updateProxyHealth(ctx, proxy, result)
	}
	failure := openAICodexTurnStateScanFailure(upstreamModel, result)
	if failure == "" {
		accountID := account.ID
		s.gateway.getOpenAICodexTurnStatePool().observe(result.stateValue, &accountID, hashOpenAICodexTurnState(result.sessionID), upstreamModel, "scanner")
		doneAt := time.Now()
		scan.Status = "ready"
		scan.LastSuccessAt = &doneAt
		scan.LastError = ""
		next := doneAt.Add(openAICodexTurnStateTTL - openAICodexTurnStateScanRefreshBefore)
		scan.NextAttemptAt = &next
	} else {
		scan.Status = "retry_wait"
		scan.LastError = failure
		next := time.Now().Add(openAICodexTurnStateRetryDelay(scan.AttemptCount))
		scan.NextAttemptAt = &next
	}
	if err := s.repo.UpsertOpenAICodexTurnStateScan(ctx, scan); err != nil {
		log.WithError(err).WithFields(log.Fields{"account_id": account.ID, "model": upstreamModel}).Warn("failed to persist Codex turn-state scan")
	}
}

func openAICodexTurnStateScanFailure(requested string, result openAICodexTurnStateHarvestResult) string {
	if message := strings.TrimSpace(result.errorMessage); message != "" {
		return message
	}
	if !result.upstreamOK {
		return "upstream request failed"
	}
	if result.statusCode < http.StatusOK || result.statusCode >= http.StatusMultipleChoices {
		return fmt.Sprintf("upstream returned HTTP %d", result.statusCode)
	}
	if result.stateValue == "" {
		return "upstream response did not include x-codex-turn-state"
	}
	if !isReusableOpenAICodexTurnStateLength(result.stateLength) {
		return fmt.Sprintf("received turn-state length %d, expected 292 or 332", result.stateLength)
	}
	if strings.TrimSpace(result.officialModel) == "" {
		return "upstream response did not declare an official model"
	}
	if !openAICodexTurnStateModelsMatch(requested, result.officialModel) {
		return fmt.Sprintf("upstream model mismatch: requested %s, received %s", requested, result.officialModel)
	}
	return ""
}

func openAICodexTurnStateRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 5 * time.Second
	for i := 1; i < attempt && delay < 5*time.Minute; i++ {
		delay *= 2
	}
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func (s *openAICodexTurnStateScanner) selectProxy(ctx context.Context) *OpenAICodexTurnStateProxy {
	dedicated, dedicatedErr := s.repo.ListOpenAICodexTurnStateProxies(ctx, true)
	shared, sharedErr := s.repo.ListReusableOpenAICodexTurnStateProxies(ctx)
	if dedicatedErr != nil && sharedErr != nil {
		return nil
	}
	proxies := mergeOpenAICodexTurnStateScanProxies(dedicated, shared)
	if len(proxies) == 0 {
		return nil
	}
	index := int(s.proxySeq.Add(1)-1) % len(proxies)
	return proxies[index]
}

func mergeOpenAICodexTurnStateScanProxies(groups ...[]*OpenAICodexTurnStateProxy) []*OpenAICodexTurnStateProxy {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	proxies := make([]*OpenAICodexTurnStateProxy, 0, total)
	seen := make(map[string]struct{}, total)
	for _, group := range groups {
		for _, candidate := range group {
			if candidate == nil {
				continue
			}
			normalized, err := normalizeOpenAICodexTurnStateProxyURL(candidate.ProxyURL)
			if err != nil {
				continue
			}
			if _, exists := seen[normalized]; exists {
				continue
			}
			seen[normalized] = struct{}{}
			copyCandidate := *candidate
			copyCandidate.ProxyURL = normalized
			proxies = append(proxies, &copyCandidate)
		}
	}
	return proxies
}

func (s *openAICodexTurnStateScanner) updateProxyHealth(ctx context.Context, proxy *OpenAICodexTurnStateProxy, result openAICodexTurnStateHarvestResult) {
	now := time.Now()
	proxy.LastCheckedAt = &now
	if result.upstreamOK {
		proxy.HealthStatus = "healthy"
		proxy.ConsecutiveFailures = 0
		proxy.LastSuccessAt = &now
		proxy.LastError = ""
	} else {
		proxy.HealthStatus = "unhealthy"
		proxy.ConsecutiveFailures++
		proxy.LastError = result.errorMessage
	}
	_ = s.repo.UpdateOpenAICodexTurnStateProxyHealth(ctx, proxy)
}

func (s *OpenAIGatewayService) harvestOpenAICodexTurnState(ctx context.Context, account *Account, model, proxyURL string) openAICodexTurnStateHarvestResult {
	result := openAICodexTurnStateHarvestResult{}
	if s == nil || account == nil || !account.IsOpenAIOAuthLike() {
		result.errorMessage = "account does not use the Codex OAuth protocol"
		return result
	}
	account = s.withOpenAICodexInstallationID(ctx, account)
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		result.errorMessage = truncateString(err.Error(), openAICodexTurnStateScanMaxErrorBytes)
		return result
	}
	model = openAICodexTurnStateUpstreamModel(account, model)
	result.upstreamModel = model
	sessionID := scopeCodexAccountIdentityValue(account, 0, "session", uuid.NewString())
	result.sessionID = sessionID
	threadID := scopeCodexAccountIdentityValue(account, 0, "thread", uuid.NewString())
	body := []byte(fmt.Sprintf(`{"model":%s,"stream":true,"store":false,"instructions":"Reply with exactly: pong","input":[{"role":"user","content":[{"type":"input_text","text":"ping"}]}],"client_metadata":{"session_id":%s,"thread_id":%s}}`,
		strconv.Quote(model), strconv.Quote(sessionID), strconv.Quote(threadID)))
	body, _, err = applyCodexClientEnvironmentRaw(body, account)
	if err != nil {
		result.errorMessage = truncateString(err.Error(), openAICodexTurnStateScanMaxErrorBytes)
		return result
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptCodexURL, bytes.NewReader(body))
	if err != nil {
		result.errorMessage = truncateString(err.Error(), openAICodexTurnStateScanMaxErrorBytes)
		return result
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAIHarvest))
	req.Close = true
	req.Host = "chatgpt.com"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("session-id", sessionID)
	req.Header.Set("thread-id", threadID)
	req.Header.Set("x-client-request-id", threadID)
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, req.Header, account); err != nil {
		result.errorMessage = truncateString(err.Error(), openAICodexTurnStateScanMaxErrorBytes)
		return result
	}
	ensureCodexIdentityHeaders(req.Header)
	enforceCodexIdentityHeadersWithUA(req.Header, s.codexIdentityOverrideUA(account))
	if installationID := account.GetOpenAIDeviceID(); installationID != "" {
		req.Header.Set("x-codex-installation-id", installationID)
	}
	applyCodexClientEnvironmentHeaders(req.Header, account)
	setOpenAICodexRoutingHintFromBody(req.Header, account, body)

	// Scanner probes must use the selected proxy and a no-reuse transport. Routing
	// through the business plugin can ignore the probe proxy and collapse every
	// attempt back onto the account's normal connection pool.
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Mode1EffectiveConcurrency())
	if err != nil {
		result.errorMessage = truncateString(err.Error(), openAICodexTurnStateScanMaxErrorBytes)
		return result
	}
	if resp == nil {
		result.errorMessage = "empty upstream response"
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	result.upstreamOK = true
	result.statusCode = resp.StatusCode
	result.officialModel = firstNonEmptyCodexHeader(resp.Header, "OpenAI-Model", "openai-model")
	state := extractOpenAICodexTurnState(resp.Header)
	result.stateValue = state
	result.stateLength = len(state)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		message := strings.TrimSpace(extractUpstreamErrorMessage(raw))
		if message == "" {
			message = strings.TrimSpace(string(raw))
		}
		result.errorMessage = truncateString(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, message), openAICodexTurnStateScanMaxErrorBytes)
		return result
	}
	if result.officialModel == "" {
		result.officialModel = readOpenAICodexTurnStateOfficialModel(resp.Body)
	}
	if state == "" {
		result.errorMessage = "upstream response did not include x-codex-turn-state"
	} else if result.officialModel == "" {
		result.errorMessage = "upstream response did not declare an official model"
	}
	return result
}

func readOpenAICodexTurnStateOfficialModel(body io.Reader) string {
	if body == nil {
		return ""
	}
	scanner := bufio.NewScanner(io.LimitReader(body, 64<<10))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if bytes.HasPrefix(line, []byte("data:")) {
			line = bytes.TrimSpace(line[len("data:"):])
		}
		if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
			continue
		}
		if model := firstValidTrimmedGJSONString(line, "response.model", "model"); model != "" {
			return normalizeOpenAICodexTurnStateModel(model)
		}
	}
	return ""
}

func openAICodexTurnStateUpstreamModel(account *Account, model string) string {
	if account == nil {
		return ""
	}
	return normalizeOpenAICodexTurnStateModel(normalizeOpenAIModelForUpstream(account, account.GetMappedModel(model)))
}

func openAICodexTurnStateModelsMatch(requested, official string) bool {
	requested = normalizeOpenAICodexTurnStateModel(requested)
	official = normalizeOpenAICodexTurnStateModel(official)
	return requested != "" && official != "" && official == requested
}

func normalizeOpenAICodexTurnStateProxyURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("proxy URL is empty")
	}
	if !strings.Contains(raw, "://") {
		parts := strings.Split(raw, ":")
		if len(parts) >= 4 {
			host, port, username := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
			password := strings.Join(parts[3:], ":")
			if host != "" && port != "" && username != "" {
				raw = (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), User: url.UserPassword(username, password)}).String()
			}
		} else {
			raw = "http://" + raw
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.Port() == "" {
		return "", errors.New("invalid proxy URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return "", errors.New("proxy scheme must be http, https, socks5, or socks5h")
	}
	if _, err := strconv.Atoi(parsed.Port()); err != nil {
		return "", errors.New("invalid proxy port")
	}
	return parsed.String(), nil
}

func maskOpenAICodexTurnStateProxyURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return "***"
	}
	if parsed.User != nil {
		username := parsed.User.Username()
		if username == "" {
			username = "***"
		}
		parsed.User = url.UserPassword(username, "***")
	}
	return parsed.String()
}

func (s *OpsService) codexTurnStateScannerRepository() (OpenAICodexTurnStateScannerRepository, error) {
	repo, ok := s.opsRepo.(OpenAICodexTurnStateScannerRepository)
	if !ok {
		return nil, errors.New("Codex turn-state scanner repository is unavailable")
	}
	return repo, nil
}

func (s *OpsService) ListOpenAICodexTurnStateProxies(ctx context.Context) ([]*OpenAICodexTurnStateProxy, error) {
	repo, err := s.codexTurnStateScannerRepository()
	if err != nil {
		return nil, err
	}
	items, err := repo.ListOpenAICodexTurnStateProxies(ctx, false)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		item.MaskedURL = maskOpenAICodexTurnStateProxyURL(item.ProxyURL)
		item.ProxyURL = ""
	}
	return items, nil
}

func (s *OpsService) AddOpenAICodexTurnStateProxies(ctx context.Context, values []string) (int, error) {
	repo, err := s.codexTurnStateScannerRepository()
	if err != nil {
		return 0, err
	}
	unique := make(map[string]struct{}, len(values))
	items := make([]*OpenAICodexTurnStateProxy, 0, len(values))
	for index, value := range values {
		normalized, normalizeErr := normalizeOpenAICodexTurnStateProxyURL(value)
		if normalizeErr != nil {
			return 0, fmt.Errorf("proxy line %d: %w", index+1, normalizeErr)
		}
		if _, exists := unique[normalized]; exists {
			continue
		}
		unique[normalized] = struct{}{}
		items = append(items, &OpenAICodexTurnStateProxy{Name: fmt.Sprintf("State proxy %d", index+1), ProxyURL: normalized, Enabled: true})
	}
	if len(items) == 0 {
		return 0, errors.New("no proxy URLs provided")
	}
	return repo.CreateOpenAICodexTurnStateProxies(ctx, items)
}

func (s *OpsService) SetOpenAICodexTurnStateProxyEnabled(ctx context.Context, id int64, enabled bool) error {
	repo, err := s.codexTurnStateScannerRepository()
	if err != nil {
		return err
	}
	return repo.SetOpenAICodexTurnStateProxyEnabled(ctx, id, enabled)
}

func (s *OpsService) DeleteOpenAICodexTurnStateProxy(ctx context.Context, id int64) error {
	repo, err := s.codexTurnStateScannerRepository()
	if err != nil {
		return err
	}
	return repo.DeleteOpenAICodexTurnStateProxy(ctx, id)
}

func (s *OpsService) ListOpenAICodexTurnStateAccountStatuses(ctx context.Context, accountIDs []int64, page, pageSize int) (*OpenAICodexTurnStateAccountStatusList, error) {
	repo, err := s.codexTurnStateScannerRepository()
	if err != nil {
		return nil, err
	}
	targetModels := s.codexTurnStateScanner.targetModels()
	if len(accountIDs) > 0 {
		page = 1
		pageSize = len(accountIDs) * len(targetModels)
	}
	result, err := repo.ListOpenAICodexTurnStateAccountStatuses(ctx, accountIDs, targetModels, page, pageSize)
	if err != nil {
		return nil, err
	}
	for _, item := range result.Items {
		item.LastProxyMasked = maskOpenAICodexTurnStateProxyURL(item.LastProxyMasked)
	}
	return result, nil
}

func (s *OpsService) GetOpenAICodexTurnStateOperationsSummary(ctx context.Context) (*OpenAICodexTurnStateOperationsSummary, error) {
	repo, err := s.codexTurnStateScannerRepository()
	if err != nil {
		return nil, err
	}
	targetModels := s.codexTurnStateScanner.targetModels()
	result, err := repo.GetOpenAICodexTurnStateOperationsSummary(ctx, targetModels)
	if err != nil {
		return nil, err
	}
	result.TargetModels = int64(len(targetModels))
	result.TotalModelSlots = result.OAuthAccounts * result.TargetModels
	return result, nil
}

func (s *OpsService) EnqueueOpenAICodexTurnStateScan(accountID int64, model string) bool {
	if s == nil || s.codexTurnStateScanner == nil {
		return false
	}
	if strings.TrimSpace(model) == "" {
		model = defaultOpenAICodexTurnStateScanModel
	}
	return s.codexTurnStateScanner.Enqueue(accountID, model, true)
}

func (s *OpsService) EnqueueOpenAICodexTurnStateAccountScans(accountID int64) (int, []string) {
	if s == nil || s.codexTurnStateScanner == nil || accountID <= 0 {
		return 0, nil
	}
	models := s.codexTurnStateScanner.targetModels()
	return s.codexTurnStateScanner.enqueueAccount(accountID, true), models
}

func (s *OpsService) EnqueueAllOpenAICodexTurnStateScans(ctx context.Context) (int, error) {
	if s == nil || s.codexTurnStateScanner == nil {
		return 0, errors.New("Codex turn-state scanner is unavailable")
	}
	now := time.Now()
	accounts, err := s.codexTurnStateScanner.listRecentlyUsedAccounts(ctx, now)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, account := range accounts {
		queued += s.codexTurnStateScanner.enqueueAccount(account.ID, true)
	}
	return queued, nil
}

func (s *OpsService) StartOpenAICodexTurnStateScanner(ctx context.Context) {
	if s != nil && s.codexTurnStateScanner != nil {
		s.codexTurnStateScanner.Start(ctx)
	}
}

func (s *OpsService) StopOpenAICodexTurnStateScanner() {
	if s != nil && s.codexTurnStateScanner != nil {
		s.codexTurnStateScanner.Stop()
	}
}
