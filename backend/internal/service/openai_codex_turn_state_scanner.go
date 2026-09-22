package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptrelay"
)

const (
	openAICodexTurnStateScanWorkers       = 4
	openAICodexTurnStateScanQueueSize     = 256
	openAICodexTurnStateScanFanoutLimit   = 5
	openAICodexTurnStateInventoryLimit    = 5
	openAICodexTurnStateScanLeaseDuration = 90 * time.Second
	openAICodexTurnStateScanJobTimeout    = 75 * time.Second
	openAICodexTurnStateScanSweepInterval = 30 * time.Second
	openAICodexTurnStateScanRefreshBefore = time.Minute
	// Route affinity has been observed to decay before the signed state expires.
	// Renew proactively without shortening the lifetime of the current ticket.
	openAICodexTurnStateProactiveRefreshAfter = 90 * time.Second
	openAICodexTurnStateScanRetryMax          = 5 * time.Minute
	openAICodexTurnStateSafetyRetryDelay      = 30 * time.Second
	openAICodexDynamicRouteTTL                = 4 * time.Minute
	openAICodexDynamicRouteRefreshBefore      = time.Minute
	openAICodexTurnStateActiveUsageWindow     = time.Hour
	openAICodexTurnStateScanMaxErrorBytes     = 240
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
	routeIPv6     string
	routeCookie   string
	safetyFaster  string
}

type openAICodexTurnStateRefreshRoute struct {
	sessionID   string
	routeCookie string
}

type openAICodexTurnStateRefreshRouteContextKey struct{}

type openAICodexTurnStateScanRoute uint8

const (
	openAICodexTurnStateScanRouteProxy openAICodexTurnStateScanRoute = iota
	openAICodexTurnStateScanRouteIPv6
	openAICodexTurnStateScanRouteDynamicProxy
)

type openAICodexTurnStateScanner struct {
	repo        OpenAICodexTurnStateScannerRepository
	accountRepo AccountRepository
	gateway     *OpenAIGatewayService

	queue    chan openAICodexTurnStateScanJob
	settings atomic.Pointer[OpenAICodexTurnStateScanSettings]
	probe    func(context.Context, *Account, string, string) openAICodexTurnStateHarvestResult

	mu       sync.Mutex
	inFlight map[openAICodexTurnStateBucketKey]struct{}
	// workspaceLocks prevent different local OAuth members of one Team
	// workspace from probing several models at the same time.
	workspaceLocks sync.Map
	cancel         context.CancelFunc
	done           chan struct{}
}

func (s *openAICodexTurnStateScanner) scanSettings() *OpenAICodexTurnStateScanSettings {
	if s != nil {
		if settings := s.settings.Load(); settings != nil {
			return settings
		}
	}
	return defaultOpenAICodexTurnStateScanSettings()
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

func isObservedOpenAICodexTurnStateModel(model string) bool {
	model = normalizeOpenAICodexTurnStateModel(model)
	return strings.HasPrefix(model, "gpt-5") || strings.HasPrefix(model, "gpt-6")
}

func (s *openAICodexTurnStateScanner) modelsForAccount(ctx context.Context, account *Account, usedSince time.Time) []string {
	models := append([]string(nil), s.targetModels()...)
	if s != nil && s.repo != nil && account != nil {
		observed, err := s.repo.ListObservedOpenAICodexTurnStateModels(ctx, account.ID, usedSince)
		if err != nil {
			log.WithError(err).WithField("account_id", account.ID).Warn("failed to list observed Codex turn-state models")
		} else {
			for _, model := range observed {
				if isObservedOpenAICodexTurnStateModel(model) {
					models = append(models, model)
				}
			}
		}
	}

	result := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = openAICodexTurnStateUpstreamModel(account, model)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	return result
}

func (s *openAICodexTurnStateScanner) enqueueModels(accountID int64, models []string, force bool) int {
	queued := 0
	for _, model := range models {
		if !force && !s.stateNeedsRefresh(accountID, model, time.Now()) {
			continue
		}
		if s.Enqueue(accountID, model, force) {
			queued++
		}
	}
	return queued
}

func (s *openAICodexTurnStateScanner) stateNeedsRefresh(accountID int64, model string, now time.Time) bool {
	if s == nil || s.gateway == nil {
		return true
	}
	pool := s.gateway.getOpenAICodexTurnStatePool()
	record, ok := pool.preferredRecordForBucket(accountID, model)
	if ok && isOpenAICodexDynamicRouteRecord(record) {
		return !record.ExpiresAt.After(now.Add(openAICodexDynamicRouteRefreshBefore))
	}
	if !pool.hasReusableStateBeyond(accountID, model, now.Add(openAICodexTurnStateScanRefreshBefore)) {
		return true
	}
	// A healthier fallback already covers the bucket when the preferred
	// higher-ranked ticket is near expiry. Let selection roll over naturally
	// instead of probing solely to preserve the older rank.
	if ok && !record.ExpiresAt.After(now.Add(openAICodexTurnStateScanRefreshBefore)) {
		return false
	}
	return ok && !record.IssuedAt.IsZero() && now.After(record.IssuedAt.Add(openAICodexTurnStateProactiveRefreshAfter))
}

func isOpenAICodexDynamicRouteRecord(record *OpenAICodexTurnStateRecord) bool {
	return record != nil && strings.TrimSpace(record.SourceProxyURL) != "" && record.SourceProxyID == nil &&
		normalizeOpenAICodexRouteIPv6(record.RouteIPv6) == ""
}

func openAICodexTurnStateWorkspaceKey(account *Account) string {
	if account == nil {
		return ""
	}
	if workspaceID := strings.TrimSpace(account.GetChatGPTAccountID()); workspaceID != "" {
		return "workspace:" + workspaceID
	}
	return "account:" + strconv.FormatInt(account.ID, 10)
}

func (s *openAICodexTurnStateScanner) lockWorkspace(account *Account) func() {
	if s == nil {
		return func() {}
	}
	key := openAICodexTurnStateWorkspaceKey(account)
	lock, _ := s.workspaceLocks.LoadOrStore(key, &sync.Mutex{})
	workspaceLock := lock.(*sync.Mutex)
	workspaceLock.Lock()
	return workspaceLock.Unlock
}

func selectOpenAICodexTurnStateScanRoute(settings *OpenAICodexTurnStateScanSettings, ipv6BindingEnabled bool) openAICodexTurnStateScanRoute {
	if settings != nil && settings.DynamicProxyEnabled {
		return openAICodexTurnStateScanRouteDynamicProxy
	}
	if ipv6BindingEnabled {
		return openAICodexTurnStateScanRouteIPv6
	}
	return openAICodexTurnStateScanRouteProxy
}

func (s *openAICodexTurnStateScanner) enqueueAccount(ctx context.Context, account *Account, force bool, usedSince time.Time) (int, []string) {
	if account == nil {
		return 0, nil
	}
	plan := OpenAICodexStatePlanType(account)
	s.gateway.getOpenAICodexTurnStatePool().setAccountPlan(account.ID, plan)
	if !s.scanSettings().IsPlanScanEnabled(plan) {
		return 0, nil
	}
	models := s.modelsForAccount(ctx, account, usedSince)
	return s.enqueueModels(account.ID, models, force), models
}

func (s *openAICodexTurnStateScanner) enqueueSweep(ctx context.Context) {
	now := time.Now()
	accounts, err := s.listRecentlyUsedAccounts(ctx, now)
	if err != nil {
		log.WithError(err).Warn("failed to list Codex turn-state scan accounts")
		return
	}
	usedSince := now.Add(-openAICodexTurnStateActiveUsageWindow)
	for _, account := range accounts {
		plan := OpenAICodexStatePlanType(account)
		s.gateway.getOpenAICodexTurnStatePool().setAccountPlan(account.ID, plan)
		if !s.scanSettings().IsPlanScanEnabled(plan) {
			continue
		}
		for _, model := range s.modelsForAccount(ctx, account, usedSince) {
			if !s.stateNeedsRefresh(account.ID, model, now) {
				continue
			}
			scan, scanErr := s.repo.GetOpenAICodexTurnStateScan(ctx, account.ID, model)
			if scanErr != nil {
				continue
			}
			// Missing and expiring states bypass an obsolete ready deadline,
			// but upstream failures always respect their retry backoff.
			if scan != nil && scan.Status != "ready" && scan.NextAttemptAt != nil && scan.NextAttemptAt.After(now) {
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
	if pendingRepo, ok := s.repo.(OpenAICodexTurnStatePendingAccountRepository); ok {
		pendingIDs, pendingErr := pendingRepo.ListPendingOpenAICodexTurnStateAccountIDs(ctx)
		if pendingErr != nil {
			return nil, pendingErr
		}
		ids = append(ids, pendingIDs...)
	}
	seen := make(map[int64]struct{}, len(ids))
	uniqueIDs := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	loaded, err := s.accountRepo.GetByIDs(ctx, uniqueIDs)
	if err != nil {
		return nil, fmt.Errorf("load Codex turn-state scan accounts: %w", err)
	}
	accounts := make([]*Account, 0, len(loaded))
	for _, account := range loaded {
		if isRecentlyUsedOpenAICodexAccount(account, now) ||
			(account != nil && account.RequiresOpenAICodexStateRouting() && isEligibleOpenAICodexTurnStateAccount(account, now)) {
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

func shouldRunOpenAICodexTurnStateScanJob(account *Account, job openAICodexTurnStateScanJob, settings *OpenAICodexTurnStateScanSettings, now time.Time) bool {
	if !isEligibleOpenAICodexTurnStateAccount(account, now) {
		return false
	}
	if job.force || isRecentlyUsedOpenAICodexAccount(account, now) || account.RequiresOpenAICodexStateRouting() {
		return true
	}
	// A strict route-binding miss is enqueued by the live request path. The
	// periodic sweep filters dormant accounts before enqueueing them.
	return settings.IsRouteBindingRequired()
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
	if err != nil || !shouldRunOpenAICodexTurnStateScanJob(account, job, s.scanSettings(), now) {
		return
	}
	plan := OpenAICodexStatePlanType(account)
	if !s.scanSettings().IsPlanScanEnabled(plan) {
		return
	}
	upstreamModel := openAICodexTurnStateUpstreamModel(account, job.model)
	if upstreamModel == "" {
		return
	}
	unlockWorkspace := s.lockWorkspace(account)
	defer unlockWorkspace()
	pool := s.gateway.getOpenAICodexTurnStatePool()
	pool.setAccountPlan(account.ID, plan)
	if !job.force && !s.stateNeedsRefresh(account.ID, upstreamModel, now) {
		return
	}
	// Bound acquisition below the database lease, including proxy discovery.
	// The normal business request context and timeouts are not changed.
	ctx, cancel := context.WithTimeout(ctx, openAICodexTurnStateScanJobTimeout)
	defer cancel()
	leaseRepo, hasLease := s.repo.(OpenAICodexTurnStateScanLeaseRepository)
	leaseID := ""
	if hasLease {
		leaseID = uuid.NewString()
		claimed, claimErr := leaseRepo.ClaimOpenAICodexTurnStateScan(ctx, account.ID, upstreamModel, leaseID, time.Now().Add(openAICodexTurnStateScanLeaseDuration))
		if claimErr != nil {
			log.WithError(claimErr).WithFields(log.Fields{"account_id": account.ID, "model": upstreamModel}).Warn("failed to claim Codex turn-state scan lease")
			return
		}
		if !claimed {
			return
		}
		defer func() {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer releaseCancel()
			if releaseErr := leaseRepo.ReleaseOpenAICodexTurnStateScan(releaseCtx, account.ID, upstreamModel, leaseID); releaseErr != nil {
				log.WithError(releaseErr).WithFields(log.Fields{"account_id": account.ID, "model": upstreamModel}).Warn("failed to release Codex turn-state scan lease")
			}
		}()
	}
	// Read after claiming: another instance may have completed a scan while
	// this job was queued. Never overwrite its cooldown with a stale snapshot.
	if err := pool.refreshBucket(ctx, account.ID, upstreamModel); err != nil {
		log.WithError(err).WithFields(log.Fields{"account_id": account.ID, "model": upstreamModel}).Warn("failed to refresh persisted Codex turn-state bucket")
		return
	}
	previous, previousErr := s.repo.GetOpenAICodexTurnStateScan(ctx, account.ID, upstreamModel)
	if previousErr != nil {
		return
	}
	if !job.force {
		if !s.stateNeedsRefresh(account.ID, upstreamModel, time.Now()) {
			return
		}
		if previous != nil && previous.NextAttemptAt != nil && previous.NextAttemptAt.After(time.Now()) && previous.Status != "ready" {
			return
		}
	}

	scan := &OpenAICodexTurnStateScan{AccountID: account.ID, Model: upstreamModel, Status: "running"}
	if previous != nil {
		*scan = *previous
		scan.Status = "running"
	}
	scan.LeaseID = leaseID
	attemptAt := time.Now()
	scan.AttemptCount++
	scan.LastAttemptAt = &attemptAt
	scan.LastError = ""
	scan.NextAttemptAt = nil
	if err := s.repo.UpsertOpenAICodexTurnStateScan(ctx, scan); err != nil {
		log.WithError(err).WithFields(log.Fields{"account_id": account.ID, "model": upstreamModel}).Warn("failed to begin Codex turn-state scan")
		return
	}

	settings := s.scanSettings().forAccountModel(account, upstreamModel)
	var results []openAICodexTurnStateProbe
	reusedBoundRoute := false
	desiredInventory := min(max(settings.ParallelProbes, 1), openAICodexTurnStateInventoryLimit)
	hasInventory := pool.reusableStateCountBeyond(account.ID, upstreamModel, time.Now().Add(openAICodexTurnStateScanRefreshBefore)) >= desiredInventory
	if !job.force {
		current, ok := pool.preferredRecordForBucket(account.ID, upstreamModel)
		if ok && current.RouteCookie != "" {
			target := openAICodexTurnStateProbeTarget{
				proxy:        openAICodexTurnStateProxyForRecord(current),
				routeIPv6:    current.RouteIPv6,
				refreshRoute: openAICodexTurnStateRefreshRoute{sessionID: current.SourceSessionID, routeCookie: current.RouteCookie},
			}
			results = s.probeTargetsWithBoundedFanout(ctx, account, upstreamModel, []openAICodexTurnStateProbeTarget{target}, settings)
			if len(results) == 1 && openAICodexTurnStateScanFailure(upstreamModel, results[0].result, time.Now(), settings) == "" {
				reusedBoundRoute = true
			}
		}
	}
	prefix, ipv6BindingEnabled := s.gateway.openAIChatGPTIPv6BindingPrefix()
	if !reusedBoundRoute || !hasInventory {
		switch selectOpenAICodexTurnStateScanRoute(settings, ipv6BindingEnabled) {
		case openAICodexTurnStateScanRouteDynamicProxy:
			proxies, proxyErr := s.scanProxies(ctx, account.ID, upstreamModel, scan.AttemptCount, settings)
			if proxyErr != nil {
				results = append(results, openAICodexTurnStateProbe{result: openAICodexTurnStateHarvestResult{errorMessage: proxyErr.Error()}})
			} else {
				results = append(results, s.probeWithBoundedFanout(ctx, account, upstreamModel, proxies, settings)...)
			}
		case openAICodexTurnStateScanRouteIPv6:
			routeIPv6s, routeErr := randomOpenAICodexRouteIPv6s(prefix, min(max(settings.ParallelProbes, 1), openAICodexTurnStateScanFanoutLimit))
			if routeErr != nil {
				results = append(results, openAICodexTurnStateProbe{result: openAICodexTurnStateHarvestResult{errorMessage: routeErr.Error()}})
			} else {
				results = append(results, s.probeWithIPv6Fanout(ctx, account, upstreamModel, routeIPv6s, settings)...)
			}
		default:
			proxies, proxyErr := s.scanProxies(ctx, account.ID, upstreamModel, scan.AttemptCount, settings)
			if proxyErr != nil {
				results = append(results, openAICodexTurnStateProbe{result: openAICodexTurnStateHarvestResult{errorMessage: proxyErr.Error()}})
			} else {
				results = append(results, s.probeWithBoundedFanout(ctx, account, upstreamModel, proxies, settings)...)
			}
		}
	}
	// A setting can change during a job. Do not mark a result reusable under
	// a policy which has already been replaced by the administrator.
	settings = s.scanSettings().forAccountModel(account, upstreamModel)
	result, proxy := chooseOpenAICodexTurnStateResult(upstreamModel, results, settings)
	scan.LastProxyID = nil
	scan.LastProxyURL = ""
	if proxy != nil {
		scan.LastProxyURL = maskOpenAICodexTurnStateProxyURL(proxy.ProxyURL)
		if proxy.Source == "state" && proxy.ID > 0 {
			proxyID := proxy.ID
			scan.LastProxyID = &proxyID
		}
	}
	scan.LastStateLength = result.stateLength
	doneAt := time.Now()
	failure := openAICodexTurnStateScanFailure(upstreamModel, result, doneAt, settings)
	if failure == "" {
		scan.Status = "ready"
		scan.LastSuccessAt = &doneAt
		scan.LastError = ""
		// Receiving the same state again does not renew its issuance timestamp.
		issuedAt, _ := parseOpenAICodexTurnStateIssuedAt(result.stateValue)
		next := issuedAt.Add(openAICodexTurnStateProactiveRefreshAfter)
		if proxy != nil && proxy.Source == "dynamic" {
			next = doneAt.Add(openAICodexDynamicRouteTTL - openAICodexDynamicRouteRefreshBefore)
		}
		scan.NextAttemptAt = &next
	} else {
		scan.Status = "retry_wait"
		scan.LastError = failure
		retryDelay := openAICodexTurnStateResultRetryDelay(upstreamModel, result, scan.AttemptCount)
		next := doneAt.Add(retryDelay)
		scan.NextAttemptAt = &next
	}
	// Persist even after an acquisition deadline, while the fenced lease is
	// still valid. A failed or superseded lease must not publish to the pool.
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer persistCancel()
	if err := s.repo.UpsertOpenAICodexTurnStateScan(persistCtx, scan); err != nil {
		log.WithError(err).WithFields(log.Fields{"account_id": account.ID, "model": upstreamModel}).Warn("failed to persist Codex turn-state scan")
		return
	}
	if failure == "" {
		persisted, persistErr := persistOpenAICodexTurnStateProbes(persistCtx, pool, account.ID, upstreamModel, results, settings)
		if persistErr != nil {
			log.WithError(persistErr).WithFields(log.Fields{"account_id": account.ID, "model": upstreamModel, "persisted": persisted}).Warn("failed to persist one or more acquired Codex turn-states")
		}
		if persisted == 0 {
			scan.Status = "retry_wait"
			scan.LastError = "acquired state could not be persisted; retry is required"
			scan.LastSuccessAt = nil
			if previous != nil {
				scan.LastSuccessAt = previous.LastSuccessAt
			}
			next := time.Now().Add(openAICodexTurnStateRetryDelay(scan.AttemptCount))
			scan.NextAttemptAt = &next
			if saveErr := s.repo.UpsertOpenAICodexTurnStateScan(persistCtx, scan); saveErr != nil {
				log.WithError(saveErr).WithField("account_id", account.ID).Warn("failed to persist Codex turn-state storage failure")
			}
		}
	}
}

func persistOpenAICodexTurnStateProbes(ctx context.Context, pool *openAICodexTurnStatePool, accountID int64, model string, probes []openAICodexTurnStateProbe, settings *OpenAICodexTurnStateScanSettings) (int, error) {
	if pool == nil {
		return 0, fmt.Errorf("nil Codex turn-state pool")
	}
	selected := verifiedOpenAICodexTurnStateProbes(model, probes, settings, openAICodexTurnStateInventoryLimit)
	persisted := 0
	errs := make([]error, 0)
	for _, probe := range selected {
		ticket := openAICodexTurnStateRouteTicketForProxy(probe.result.sessionID, probe.proxy, probe.result.routeIPv6)
		ticket.RouteCookie = probe.result.routeCookie
		if err := pool.observeDurably(ctx, probe.result.stateValue, accountID, hashOpenAICodexTurnState(probe.result.sessionID), model, "scanner", ticket); err != nil {
			errs = append(errs, err)
			continue
		}
		persisted++
	}
	if persisted > 0 {
		if err := pool.trimBucketInventory(ctx, accountID, model, openAICodexTurnStateInventoryLimit); err != nil {
			errs = append(errs, err)
		}
	}
	return persisted, errors.Join(errs...)
}

func openAICodexTurnStateRouteTicketForProxy(sessionID string, proxy *OpenAICodexTurnStateProxy, routeIPv6 ...string) openAICodexTurnStateRouteTicket {
	ticket := openAICodexTurnStateRouteTicket{SessionID: sessionID}
	if len(routeIPv6) > 0 {
		ticket.RouteIPv6 = normalizeOpenAICodexRouteIPv6(routeIPv6[0])
	}
	if proxy == nil {
		return ticket
	}
	ticket.ExitIP = proxy.ExitIP
	if proxy.Source == "dynamic" {
		// A model-confirmed state is only useful while requests retain the exact
		// rotating-proxy session that minted it. Dynamic credentials are short
		// lived, so persist the route and expire it before the provider rotates.
		ticket.ProxyURL = proxy.ProxyURL
		ticket.ExpiresAt = time.Now().Add(openAICodexDynamicRouteTTL)
		return ticket
	}
	if proxy.Source != "state" || proxy.ID <= 0 {
		return ticket
	}
	proxyID := proxy.ID
	ticket.ProxyID = &proxyID
	if proxy.RouteBindingEnabled {
		ticket.ProxyURL = proxy.ProxyURL
	}
	return ticket
}

func openAICodexTurnStateProxyForRecord(record *OpenAICodexTurnStateRecord) *OpenAICodexTurnStateProxy {
	if record == nil || strings.TrimSpace(record.SourceProxyURL) == "" {
		return nil
	}
	proxy := &OpenAICodexTurnStateProxy{
		Source:              "dynamic",
		ProxyURL:            record.SourceProxyURL,
		ExitIP:              record.SourceExitIP,
		RouteBindingEnabled: true,
	}
	if record.SourceProxyID != nil && *record.SourceProxyID > 0 {
		proxy.ID = *record.SourceProxyID
		proxy.Source = "state"
	}
	return proxy
}

type openAICodexTurnStateProbe struct {
	proxy  *OpenAICodexTurnStateProxy
	result openAICodexTurnStateHarvestResult
}

type openAICodexTurnStateProbeTarget struct {
	proxy        *OpenAICodexTurnStateProxy
	routeIPv6    string
	refreshRoute openAICodexTurnStateRefreshRoute
}

// probeWithBoundedFanout starts the bounded batch immediately. Every verified
// success is collected as backup inventory; only an account-wide auth failure
// cancels sibling probes.
func (s *openAICodexTurnStateScanner) probeWithBoundedFanout(ctx context.Context, account *Account, model string, proxies []*OpenAICodexTurnStateProxy, settings *OpenAICodexTurnStateScanSettings) []openAICodexTurnStateProbe {
	if len(proxies) == 0 {
		return s.probeTargetsWithBoundedFanout(ctx, account, model, []openAICodexTurnStateProbeTarget{{}}, settings)
	}
	if settings == nil {
		settings = s.scanSettings().forAccountModel(account, model)
	}
	limit := min(max(settings.ParallelProbes, 1), openAICodexTurnStateScanFanoutLimit)
	proxies = mergeOpenAICodexTurnStateScanProxies(proxies)
	if len(proxies) > limit {
		proxies = proxies[:limit]
	}
	targets := make([]openAICodexTurnStateProbeTarget, 0, len(proxies))
	for _, proxy := range proxies {
		targets = append(targets, openAICodexTurnStateProbeTarget{proxy: proxy})
	}
	return s.probeTargetsWithBoundedFanout(ctx, account, model, targets, settings)
}

func (s *openAICodexTurnStateScanner) probeWithIPv6Fanout(ctx context.Context, account *Account, model string, routeIPv6s []string, settings *OpenAICodexTurnStateScanSettings) []openAICodexTurnStateProbe {
	targets := make([]openAICodexTurnStateProbeTarget, 0, len(routeIPv6s))
	for _, routeIPv6 := range routeIPv6s {
		targets = append(targets, openAICodexTurnStateProbeTarget{routeIPv6: routeIPv6})
	}
	return s.probeTargetsWithBoundedFanout(ctx, account, model, targets, settings)
}

func (s *openAICodexTurnStateScanner) probeTargetsWithBoundedFanout(ctx context.Context, account *Account, model string, targets []openAICodexTurnStateProbeTarget, settings *OpenAICodexTurnStateScanSettings) []openAICodexTurnStateProbe {
	probeFunc := s.probe
	if probeFunc == nil {
		probeFunc = s.gateway.harvestOpenAICodexTurnState
	}
	if len(targets) == 0 {
		return nil
	}
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan openAICodexTurnStateProbe, len(targets))
	probes := make([]openAICodexTurnStateProbe, 0, len(targets))
	var wait sync.WaitGroup
	for _, target := range targets {
		target := target
		wait.Add(1)
		go func() {
			defer wait.Done()
			targetCtx := probeCtx
			proxyURL := ""
			if target.proxy != nil {
				proxyURL = target.proxy.ProxyURL
			}
			if target.routeIPv6 != "" {
				targetCtx = chatgptrelay.WithSourceIPv6(targetCtx, target.routeIPv6)
			}
			if target.refreshRoute.sessionID != "" || target.refreshRoute.routeCookie != "" {
				targetCtx = context.WithValue(targetCtx, openAICodexTurnStateRefreshRouteContextKey{}, target.refreshRoute)
			}
			result := probeFunc(targetCtx, account, model, proxyURL)
			if result.routeIPv6 == "" {
				result.routeIPv6 = normalizeOpenAICodexRouteIPv6(target.routeIPv6)
			}
			// A sibling winner or shutdown is not a failed proxy health check.
			if probeCtx.Err() == nil {
				s.updateProxyHealth(ctx, target.proxy, result)
			}
			results <- openAICodexTurnStateProbe{proxy: target.proxy, result: result}
		}()
	}
	go func() {
		wait.Wait()
		close(results)
	}()
	for probe := range results {
		probes = append(probes, probe)
		if isOpenAICodexTurnStateAuthFailure(probe.result) {
			cancel()
		}
	}
	return probes
}

func (s *OpenAIGatewayService) openAIChatGPTIPv6BindingPrefix() (netip.Prefix, bool) {
	if s == nil || s.cfg == nil || !s.cfg.Gateway.OpenAIChatGPTIPv6Only {
		return netip.Prefix{}, false
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(s.cfg.Gateway.OpenAIChatGPTIPv6Prefix))
	if err != nil || !prefix.Addr().Is6() || prefix.Addr().Is4In6() || prefix.Bits() != 64 {
		return netip.Prefix{}, false
	}
	return prefix.Masked(), true
}

func randomOpenAICodexRouteIPv6s(prefix netip.Prefix, count int) ([]string, error) {
	prefix = prefix.Masked()
	if !prefix.Addr().Is6() || prefix.Addr().Is4In6() || prefix.Bits() != 64 {
		return nil, fmt.Errorf("Codex route prefix must be an IPv6 /64")
	}
	if count < 1 {
		count = 1
	}
	result := make([]string, 0, count)
	seen := make(map[netip.Addr]struct{}, count)
	base := prefix.Addr().As16()
	for len(result) < count {
		var host [8]byte
		if _, err := rand.Read(host[:]); err != nil {
			return nil, fmt.Errorf("generate Codex route IPv6: %w", err)
		}
		raw := base
		copy(raw[8:], host[:])
		addr := netip.AddrFrom16(raw)
		if addr == prefix.Addr() {
			continue
		}
		if _, exists := seen[addr]; exists {
			continue
		}
		seen[addr] = struct{}{}
		result = append(result, addr.String())
	}
	return result, nil
}

func isOpenAICodexTurnStateAuthFailure(result openAICodexTurnStateHarvestResult) bool {
	if result.statusCode == http.StatusUnauthorized {
		return true
	}
	message := strings.ToLower(result.errorMessage)
	return strings.Contains(message, "token revoked") || strings.Contains(message, "invalidated oauth token") || strings.Contains(message, "invalid api key")
}

func chooseOpenAICodexTurnStateResult(model string, probes []openAICodexTurnStateProbe, policy ...*OpenAICodexTurnStateScanSettings) (openAICodexTurnStateHarvestResult, *OpenAICodexTurnStateProxy) {
	var best openAICodexTurnStateProbe
	var bestFailure openAICodexTurnStateProbe
	bestValid := false
	bestFailureSet := false
	now := time.Now()
	for _, probe := range probes {
		if !bestFailureSet ||
			(isOpenAICodexTurnStateAuthFailure(probe.result) && !isOpenAICodexTurnStateAuthFailure(bestFailure.result)) ||
			(!isOpenAICodexTurnStateAuthFailure(bestFailure.result) && probe.result.stateLength > bestFailure.result.stateLength) {
			bestFailure, bestFailureSet = probe, true
		}
		if openAICodexTurnStateScanFailure(model, probe.result, now, policy...) != "" {
			continue
		}
		if !bestValid || openAICodexTurnStateProbeRanksBefore(probe, best, policy...) {
			best, bestValid = probe, true
		}
	}
	if !bestValid {
		if bestFailureSet {
			return bestFailure.result, bestFailure.proxy
		}
		return openAICodexTurnStateHarvestResult{}, nil
	}
	return best.result, best.proxy
}

func verifiedOpenAICodexTurnStateProbes(model string, probes []openAICodexTurnStateProbe, settings *OpenAICodexTurnStateScanSettings, limit int) []openAICodexTurnStateProbe {
	if settings == nil {
		settings = defaultOpenAICodexTurnStateScanSettings()
	}
	if limit < 1 {
		return nil
	}
	now := time.Now()
	valid := make([]openAICodexTurnStateProbe, 0, min(len(probes), limit))
	seen := make(map[string]struct{}, len(probes))
	for _, probe := range probes {
		if openAICodexTurnStateScanFailure(model, probe.result, now, settings) != "" {
			continue
		}
		stateHash := hashOpenAICodexTurnState(probe.result.stateValue)
		if _, exists := seen[stateHash]; exists {
			continue
		}
		seen[stateHash] = struct{}{}
		valid = append(valid, probe)
	}
	sort.SliceStable(valid, func(i, j int) bool {
		return openAICodexTurnStateProbeRanksBefore(valid[i], valid[j], settings)
	})
	if len(valid) > limit {
		valid = valid[:limit]
	}
	return valid
}

func openAICodexTurnStateProbeRanksBefore(left, right openAICodexTurnStateProbe, policy ...*OpenAICodexTurnStateScanSettings) bool {
	settings := openAICodexTurnStateProbePolicy(policy)
	if left.result.stateLength != right.result.stateLength {
		leftRank := settings.lengthRank(left.result.stateLength)
		rightRank := settings.lengthRank(right.result.stateLength)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
	}
	leftIssued, _ := parseOpenAICodexTurnStateIssuedAt(left.result.stateValue)
	rightIssued, _ := parseOpenAICodexTurnStateIssuedAt(right.result.stateValue)
	return leftIssued.After(rightIssued)
}

func openAICodexTurnStateProbePolicy(policy []*OpenAICodexTurnStateScanSettings) *OpenAICodexTurnStateScanSettings {
	if len(policy) > 0 && policy[0] != nil {
		return policy[0]
	}
	return defaultOpenAICodexTurnStateScanSettings()
}

func isOpenAICodexTurnStateSafetyBuffered(requested string, result openAICodexTurnStateHarvestResult) bool {
	return result.safetyFaster != "" &&
		openAICodexTurnStateModelsMatch(result.safetyFaster, result.officialModel) &&
		!openAICodexTurnStateModelsMatch(requested, result.officialModel)
}

func openAICodexTurnStateScanFailure(requested string, result openAICodexTurnStateHarvestResult, now time.Time, policy ...*OpenAICodexTurnStateScanSettings) string {
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
	settings := openAICodexTurnStateProbePolicy(policy)
	if result.stateLength != len(result.stateValue) || !settings.acceptsLength(result.stateLength) {
		return fmt.Sprintf("received turn-state length %d, expected one of %v", result.stateLength, settings.TargetLengths)
	}
	if strings.TrimSpace(result.officialModel) == "" {
		return "upstream response did not declare an official model"
	}
	if isOpenAICodexTurnStateSafetyBuffered(requested, result) {
		return fmt.Sprintf("upstream safety-buffering routed %s to %s", requested, result.officialModel)
	}
	if !openAICodexTurnStateModelsMatch(requested, result.officialModel) {
		return fmt.Sprintf("upstream model mismatch: requested %s, received %s", requested, result.officialModel)
	}
	issuedAt, ok := parseOpenAICodexTurnStateIssuedAt(result.stateValue)
	if !ok || issuedAt.IsZero() {
		return "received turn-state with an invalid issuance timestamp"
	}
	if issuedAt.After(now) {
		return "received turn-state with a future issuance timestamp"
	}
	expiresAt := issuedAt.Add(openAICodexTurnStateTTL)
	if !expiresAt.After(now) {
		return "received turn-state is already expired"
	}
	if !expiresAt.After(now.Add(openAICodexTurnStateScanRefreshBefore)) {
		return "received turn-state expires within the refresh window; refresh is still required"
	}
	return ""
}

func openAICodexTurnStateRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 5 * time.Second
	for i := 1; i < attempt && delay < openAICodexTurnStateScanRetryMax; i++ {
		delay *= 2
	}
	if delay > openAICodexTurnStateScanRetryMax {
		return openAICodexTurnStateScanRetryMax
	}
	return delay
}

func openAICodexTurnStateResultRetryDelay(requested string, result openAICodexTurnStateHarvestResult, attempt int) time.Duration {
	if isOpenAICodexTurnStateSafetyBuffered(requested, result) {
		return openAICodexTurnStateSafetyRetryDelay
	}
	return openAICodexTurnStateRetryDelay(attempt)
}

func (s *openAICodexTurnStateScanner) selectProxy(ctx context.Context, accountID int64, model string, attempt int) *OpenAICodexTurnStateProxy {
	proxies := s.selectProxies(ctx, accountID, model, attempt, 1)
	if len(proxies) == 0 {
		return nil
	}
	return proxies[0]
}

func (s *openAICodexTurnStateScanner) scanProxies(ctx context.Context, accountID int64, model string, attempt int, settings *OpenAICodexTurnStateScanSettings) ([]*OpenAICodexTurnStateProxy, error) {
	limit := min(max(settings.ParallelProbes, 1), openAICodexTurnStateScanFanoutLimit)
	if !settings.DynamicProxyEnabled {
		return s.selectProxies(ctx, accountID, model, attempt, limit), nil
	}
	proxies, summary, err := fetchOpenAICodexTurnStateDynamicProxies(ctx, settings.DynamicProxyURL, limit)
	log.WithFields(log.Fields{
		"account_id": accountID, "model": model, "attempt": attempt,
		"requested": summary.Requested, "candidates": summary.Candidates,
		"verified": summary.Verified, "selected": summary.Selected,
		"countries": summary.Countries, "failures": summary.Failures,
	}).Info("Codex turn-state dynamic scan proxy selection")
	if err != nil {
		return nil, err
	}
	if len(proxies) == 0 {
		return nil, errors.New("dynamic proxy source returned no verified scanning exits")
	}
	return proxies, nil
}

func (s *openAICodexTurnStateScanner) selectProxies(ctx context.Context, accountID int64, model string, attempt, limit int) []*OpenAICodexTurnStateProxy {
	if limit <= 0 {
		return nil
	}
	dedicated, dedicatedErr := s.repo.ListOpenAICodexTurnStateProxies(ctx, true)
	shared, sharedErr := s.repo.ListReusableOpenAICodexTurnStateProxies(ctx)
	if dedicatedErr != nil && sharedErr != nil {
		return nil
	}
	proxies := mergeOpenAICodexTurnStateScanProxies(dedicated, shared)
	if len(proxies) == 0 {
		return nil
	}
	sortOpenAICodexTurnStateScanProxies(proxies)
	start := openAICodexTurnStateProxyIndex(accountID, model, attempt, len(proxies))
	if limit > len(proxies) {
		limit = len(proxies)
	}
	selected := make([]*OpenAICodexTurnStateProxy, 0, limit)
	for offset := 0; offset < len(proxies) && len(selected) < limit; offset++ {
		selected = append(selected, proxies[(start+offset)%len(proxies)])
	}
	return selected
}

func sortOpenAICodexTurnStateScanProxies(proxies []*OpenAICodexTurnStateProxy) {
	sort.SliceStable(proxies, func(left, right int) bool {
		leftProxy, rightProxy := proxies[left], proxies[right]
		if leftProxy.Source != rightProxy.Source {
			return leftProxy.Source == "state"
		}
		leftID, rightID := leftProxy.SourceID, rightProxy.SourceID
		if leftID == 0 {
			leftID = leftProxy.ID
		}
		if rightID == 0 {
			rightID = rightProxy.ID
		}
		if leftID != rightID {
			return leftID < rightID
		}
		return leftProxy.ProxyURL < rightProxy.ProxyURL
	})
}

func openAICodexTurnStateProxyIndex(accountID int64, model string, attempt, proxyCount int) int {
	if proxyCount <= 1 {
		return 0
	}
	if attempt < 1 {
		attempt = 1
	}

	// Give each account/model a stable starting point, then advance one proxy
	// for every persisted attempt so concurrent jobs cannot pin it to one IP.
	seed := uint64(accountID) ^ 14695981039346656037
	for index := 0; index < len(model); index++ {
		seed ^= uint64(model[index])
		seed *= 1099511628211
	}
	return int((seed + uint64(attempt-1)) % uint64(proxyCount))
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
			key := normalized
			if candidate.Source == "dynamic" && candidate.ExitIP != "" {
				// A rotating gateway can return the same endpoint for distinct
				// independently verified exits. Preserve those parallel slots.
				key = "dynamic:" + candidate.ExitIP
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			copyCandidate := *candidate
			copyCandidate.ProxyURL = normalized
			proxies = append(proxies, &copyCandidate)
		}
	}
	return proxies
}

func (s *openAICodexTurnStateScanner) updateProxyHealth(ctx context.Context, proxy *OpenAICodexTurnStateProxy, result openAICodexTurnStateHarvestResult) {
	// Dynamic exits are ephemeral. Shared/business proxies belong to a
	// different health system and must never be updated by state scanning.
	if s == nil || s.repo == nil || proxy == nil || proxy.Source != "state" || proxy.ID <= 0 || ctx.Err() != nil {
		return
	}
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
	result := openAICodexTurnStateHarvestResult{routeIPv6: chatgptrelay.SourceIPv6FromContext(ctx)}
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
	sessionID := scopeCodexAccountIdentityValue(account, 0, "session", uuid.NewString())
	routeCookie := ""
	if refresh, ok := ctx.Value(openAICodexTurnStateRefreshRouteContextKey{}).(openAICodexTurnStateRefreshRoute); ok {
		if stagedSessionID := strings.TrimSpace(refresh.sessionID); stagedSessionID != "" {
			sessionID = stagedSessionID
		}
		routeCookie = normalizeOpenAICodexAffinityCookieHeader(refresh.routeCookie)
	}
	result = s.requestOpenAICodexTurnState(ctx, account, token, model, proxyURL, sessionID, "", routeCookie)
	if result.errorMessage != "" || result.statusCode < http.StatusOK || result.statusCode >= http.StatusMultipleChoices ||
		result.stateValue == "" || !openAICodexTurnStateModelsMatch(model, result.officialModel) {
		return result
	}

	// A length alone is not a route guarantee. Replay the freshly minted state
	// with the same credential, harvest session, and egress. Only the state from
	// a second response that still declares the requested model is publishable.
	verified := s.requestOpenAICodexTurnState(ctx, account, token, model, proxyURL, sessionID, result.stateValue, result.routeCookie)
	replayConfirmedWithoutRefresh := verified.stateValue == "" &&
		strings.Contains(verified.errorMessage, "x-codex-turn-state") &&
		openAICodexTurnStateModelsMatch(model, verified.officialModel)
	if (verified.errorMessage != "" && !replayConfirmedWithoutRefresh) ||
		verified.statusCode < http.StatusOK || verified.statusCode >= http.StatusMultipleChoices ||
		!openAICodexTurnStateModelsMatch(model, verified.officialModel) {
		reason := openAICodexTurnStateReplayFailure(model, verified)
		if strings.TrimSpace(reason) == "" {
			reason = "upstream did not preserve the requested model"
		}
		result.errorMessage = truncateString("route ticket replay verification failed: "+reason, openAICodexTurnStateScanMaxErrorBytes)
		return result
	}
	if verified.stateValue == "" {
		// Some Team Codex turns confirm the requested model on replay but do not
		// mint a replacement state when the previous state is supplied. The replay
		// still proves the state/session/egress tuple keeps the requested route, so
		// publish the original signed state instead of treating the missing refresh
		// header as a downgrade.
		result.routeCookie = verified.routeCookie
		return result
	}
	return verified
}

func openAICodexTurnStateReplayFailure(requested string, result openAICodexTurnStateHarvestResult) string {
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
	if strings.TrimSpace(result.officialModel) == "" {
		return "upstream response did not declare an official model"
	}
	if !openAICodexTurnStateModelsMatch(requested, result.officialModel) {
		return fmt.Sprintf("upstream model mismatch: requested %s, received %s", requested, result.officialModel)
	}
	return ""
}

func (s *OpenAIGatewayService) requestOpenAICodexTurnState(ctx context.Context, account *Account, token, model, proxyURL, sessionID, state, routeCookie string) openAICodexTurnStateHarvestResult {
	result := openAICodexTurnStateHarvestResult{upstreamModel: model, sessionID: sessionID, routeIPv6: chatgptrelay.SourceIPv6FromContext(ctx)}
	threadID := scopeCodexAccountIdentityValue(account, 0, "thread", uuid.NewString())
	body := []byte(fmt.Sprintf(`{"model":%s,"stream":true,"store":false,"instructions":"Reply with exactly: pong","input":[{"role":"user","content":[{"type":"input_text","text":"ping"}]}],"client_metadata":{"session_id":%s,"thread_id":%s}}`,
		strconv.Quote(model), strconv.Quote(sessionID), strconv.Quote(threadID)))
	body, _, err := applyCodexClientEnvironmentRaw(body, account)
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
	if state = strings.TrimSpace(state); state != "" {
		req.Header.Set(openAICodexTurnStateHeader, state)
	}
	if routeCookie = normalizeOpenAICodexAffinityCookieHeader(routeCookie); routeCookie != "" {
		req.Header.Set("Cookie", routeCookie)
	}
	s.applyOpenAICodexInfrastructureCookies(req.Header)
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
	// A rotating gateway can share one URL across all probe slots. Its harvest
	// transport must allow the batch to dial concurrently even when the business
	// account limit is one. The separate harvest profile never changes that pool.
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, max(account.Mode1EffectiveConcurrency(), openAICodexTurnStateScanFanoutLimit))
	if err != nil {
		result.errorMessage = truncateString(err.Error(), openAICodexTurnStateScanMaxErrorBytes)
		return result
	}
	if resp == nil {
		result.errorMessage = "empty upstream response"
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	s.captureOpenAICodexInfrastructureCookies(resp.Header)
	result.upstreamOK = true
	result.statusCode = resp.StatusCode
	result.routeCookie = mergeOpenAICodexAffinityCookies(routeCookie, resp.Cookies())
	result.safetyFaster = normalizeOpenAICodexTurnStateModel(resp.Header.Get("x-codex-safety-buffering-faster-model"))
	result.officialModel = firstNonEmptyCodexHeader(resp.Header, "OpenAI-Model", "openai-model")
	responseState := extractOpenAICodexTurnState(resp.Header)
	result.stateValue = responseState
	result.stateLength = len(responseState)
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
	if responseState == "" {
		result.errorMessage = "upstream response did not include x-codex-turn-state"
	} else if result.officialModel == "" {
		result.errorMessage = "upstream response did not declare an official model"
	}
	return result
}

var openAICodexAffinityCookieNames = []string{
	"__cflb",
	"__oailb",
	"__cf_bm",
	"__cfruid",
	"__cfseq",
	"__cfwaitingroom",
	"_cfuvid",
	"cf_clearance",
	"cf_ob_info",
	"cf_use_ob",
}

func isOpenAICodexAffinityCookieName(name string) bool {
	for _, allowed := range openAICodexAffinityCookieNames {
		if name == allowed {
			return true
		}
	}
	return strings.HasPrefix(name, "cf_chl_")
}

func normalizeOpenAICodexAffinityCookieHeader(value string) string {
	return mergeOpenAICodexAffinityCookies(value, nil)
}

func mergeOpenAICodexAffinityCookies(existing string, updates []*http.Cookie) string {
	values := make(map[string]string, len(openAICodexAffinityCookieNames))
	if strings.TrimSpace(existing) != "" {
		request := &http.Request{Header: http.Header{"Cookie": []string{existing}}}
		for _, cookie := range request.Cookies() {
			if isOpenAICodexAffinityCookieName(cookie.Name) && cookie.Valid() == nil {
				values[cookie.Name] = cookie.Value
			}
		}
	}
	now := time.Now()
	for _, cookie := range updates {
		if cookie == nil {
			continue
		}
		if !isOpenAICodexAffinityCookieName(cookie.Name) {
			continue
		}
		if cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && !cookie.Expires.After(now)) {
			delete(values, cookie.Name)
			continue
		}
		if cookie.Valid() == nil {
			values[cookie.Name] = cookie.Value
		}
	}
	parts := make([]string, 0, len(values))
	for _, name := range openAICodexAffinityCookieNames {
		if value, ok := values[name]; ok {
			parts = append(parts, (&http.Cookie{Name: name, Value: value}).String())
			delete(values, name)
		}
	}
	extraNames := make([]string, 0, len(values))
	for name := range values {
		extraNames = append(extraNames, name)
	}
	sort.Strings(extraNames)
	for _, name := range extraNames {
		parts = append(parts, (&http.Cookie{Name: name, Value: values[name]}).String())
	}
	return strings.Join(parts, "; ")
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

func (s *OpsService) SetOpenAICodexTurnStateProxyRouteBinding(ctx context.Context, id int64, enabled bool) error {
	repo, err := s.codexTurnStateScannerRepository()
	if err != nil {
		return err
	}
	return repo.SetOpenAICodexTurnStateProxyRouteBinding(ctx, id, enabled)
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
		pageSize = 1000
	}
	result, err := repo.ListOpenAICodexTurnStateAccountStatuses(ctx, accountIDs, targetModels, s.GetOpenAICodexTurnStateScanSettings(), page, pageSize)
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
	result, err := repo.GetOpenAICodexTurnStateOperationsSummary(ctx, targetModels, s.GetOpenAICodexTurnStateScanSettings())
	if err != nil {
		return nil, err
	}
	result.TargetModels = int64(len(targetModels))
	return result, nil
}

func (s *OpsService) EnqueueOpenAICodexTurnStateScan(ctx context.Context, accountID int64, model string) bool {
	if s == nil || s.codexTurnStateScanner == nil {
		return false
	}
	if strings.TrimSpace(model) == "" {
		model = defaultOpenAICodexTurnStateScanModel
	}
	// Check the current account tier before enqueueing so a disabled plan does
	// not leave a misleading job in the queue. The worker repeats this check
	// after loading the account to cover settings changes while queued.
	if scanner := s.codexTurnStateScanner; scanner.accountRepo != nil {
		account, err := scanner.accountRepo.GetByID(ctx, accountID)
		if err != nil || !scanner.scanSettings().IsPlanScanEnabled(OpenAICodexStatePlanType(account)) {
			return false
		}
	}
	return s.codexTurnStateScanner.Enqueue(accountID, model, true)
}

func (s *OpsService) EnqueueOpenAICodexTurnStateAccountScans(ctx context.Context, accountID int64) (int, []string, error) {
	if s == nil || s.codexTurnStateScanner == nil || accountID <= 0 {
		return 0, nil, errors.New("Codex turn-state scanner is unavailable")
	}
	account, err := s.codexTurnStateScanner.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return 0, nil, fmt.Errorf("load Codex account %d: %w", accountID, err)
	}
	if !isEligibleOpenAICodexTurnStateAccount(account, time.Now()) {
		return 0, nil, errors.New("Codex account is not eligible for scanning")
	}
	queued, models := s.codexTurnStateScanner.enqueueAccount(ctx, account, false, time.Now().Add(-openAICodexTurnStateActiveUsageWindow))
	return queued, models, nil
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
	usedSince := now.Add(-openAICodexTurnStateActiveUsageWindow)
	for _, account := range accounts {
		accountQueued, _ := s.codexTurnStateScanner.enqueueAccount(ctx, account, false, usedSince)
		queued += accountQueued
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
