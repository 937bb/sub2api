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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const (
	openAICodexTurnStateScanWorkers       = 4
	openAICodexTurnStateScanQueueSize     = 256
	openAICodexTurnStateScanFanoutLimit   = 3
	openAICodexTurnStateScanLeaseDuration = 90 * time.Second
	openAICodexTurnStateScanSweepInterval = 30 * time.Second
	openAICodexTurnStateScanRefreshBefore = 15 * time.Minute
	openAICodexTurnStateScanRetryMax      = 30 * time.Second
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
	// A 292 is a valid fallback, but it is not the scanner's terminal target.
	// Only a sufficiently fresh 332 can suppress an upgrade/refresh job.
	return !s.gateway.getOpenAICodexTurnStatePool().hasReusableStateOfLengthBeyond(accountID, model, openAICodexTurnStateLength332, now.Add(openAICodexTurnStateScanRefreshBefore))
}

func (s *openAICodexTurnStateScanner) enqueueAccount(ctx context.Context, account *Account, force bool, usedSince time.Time) (int, []string) {
	if account == nil {
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
		for _, model := range s.modelsForAccount(ctx, account, usedSince) {
			// A healthy 332 is the terminal target for this bucket. A healthy
			// 292 remains a fallback and must still be allowed to enter the
			// scheduled upgrade probe path below.
			pool := s.gateway.getOpenAICodexTurnStatePool()
			if pool.hasReusableStateOfLengthBeyond(account.ID, model, openAICodexTurnStateLength332, time.Now().Add(openAICodexTurnStateScanRefreshBefore)) {
				continue
			}
			scan, scanErr := s.repo.GetOpenAICodexTurnStateScan(ctx, account.ID, model)
			if scanErr != nil {
				continue
			}
			// A fresh 292 is a usable fallback, but it is not the target state.
			// Honour its persisted cooldown before trying to upgrade it to 332;
			// a missing/expired state still bypasses a stale ready deadline.
			now := time.Now()
			hasFresh292 := pool.hasReusableStateOfLengthBeyond(account.ID, model, openAICodexTurnStateLength292, now.Add(openAICodexTurnStateScanRefreshBefore))
			if scan != nil && scan.NextAttemptAt != nil && scan.NextAttemptAt.After(now) &&
				(scan.Status != "ready" || hasFresh292) {
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
	previous, _ := s.repo.GetOpenAICodexTurnStateScan(ctx, account.ID, upstreamModel)
	pool := s.gateway.getOpenAICodexTurnStatePool()
	if !job.force {
		if pool.hasReusableStateOfLengthBeyond(account.ID, upstreamModel, openAICodexTurnStateLength332, now.Add(openAICodexTurnStateScanRefreshBefore)) {
			return
		}
		// Do not turn every 30-second sweep into a probe storm while a 292
		// fallback is still fresh. Once its scheduled cooldown elapses, the
		// next sweep may fan out and look for a 332 upgrade.
		if pool.hasReusableStateOfLengthBeyond(account.ID, upstreamModel, openAICodexTurnStateLength292, now.Add(openAICodexTurnStateScanRefreshBefore)) &&
			previous != nil && previous.NextAttemptAt != nil && previous.NextAttemptAt.After(now) {
			return
		}
	}
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

	proxies := s.selectProxies(ctx, account.ID, upstreamModel, scan.AttemptCount, openAICodexTurnStateScanFanoutLimit)
	results := s.probeWithBoundedFanout(ctx, account, upstreamModel, proxies, scan.AttemptCount > 1 || pool.hasReusableStateOfLengthBeyond(account.ID, upstreamModel, openAICodexTurnStateLength292, now))
	result, proxy := chooseOpenAICodexTurnStateResult(upstreamModel, results)
	if proxy != nil {
		scan.LastProxyURL = maskOpenAICodexTurnStateProxyURL(proxy.ProxyURL)
		if proxy.Source == "state" && proxy.ID > 0 {
			proxyID := proxy.ID
			scan.LastProxyID = &proxyID
		}
	}
	scan.LastStateLength = result.stateLength
	doneAt := time.Now()
	failure := openAICodexTurnStateScanFailure(upstreamModel, result, doneAt)
	if failure == "" {
		accountID := account.ID
		pool.observe(result.stateValue, &accountID, hashOpenAICodexTurnState(result.sessionID), upstreamModel, "scanner")
		scan.Status = "ready"
		scan.LastSuccessAt = &doneAt
		scan.LastError = ""
		// Receiving the same state again does not renew its issuance timestamp.
		issuedAt, _ := parseOpenAICodexTurnStateIssuedAt(result.stateValue)
		next := issuedAt.Add(openAICodexTurnStateTTL - openAICodexTurnStateScanRefreshBefore)
		scan.NextAttemptAt = &next
	} else {
		scan.Status = "retry_wait"
		scan.LastError = failure
		next := doneAt.Add(openAICodexTurnStateRetryDelay(scan.AttemptCount))
		scan.NextAttemptAt = &next
	}
	if err := s.repo.UpsertOpenAICodexTurnStateScan(ctx, scan); err != nil {
		log.WithError(err).WithFields(log.Fields{"account_id": account.ID, "model": upstreamModel}).Warn("failed to persist Codex turn-state scan")
	}
}

type openAICodexTurnStateProbe struct {
	proxy  *OpenAICodexTurnStateProxy
	result openAICodexTurnStateHarvestResult
}

// probeWithBoundedFanout performs one probe first so an invalid OAuth token
// cannot fan out across every proxy. A retry or an existing state may then use
// at most three distinct proxies, with the first valid 332 cancelling the rest.
func (s *openAICodexTurnStateScanner) probeWithBoundedFanout(ctx context.Context, account *Account, model string, proxies []*OpenAICodexTurnStateProxy, allowFanout bool) []openAICodexTurnStateProbe {
	if len(proxies) == 0 {
		return []openAICodexTurnStateProbe{{result: s.gateway.harvestOpenAICodexTurnState(ctx, account, model, "")}}
	}
	first := proxies[0]
	firstResult := s.gateway.harvestOpenAICodexTurnState(ctx, account, model, first.ProxyURL)
	s.updateProxyHealth(ctx, first, firstResult)
	probes := []openAICodexTurnStateProbe{{proxy: first, result: firstResult}}
	firstFailure := openAICodexTurnStateScanFailure(model, firstResult, time.Now())
	// A valid 292 is a fallback, but the scanner is also responsible for
	// upgrading an active bucket to 332. Keep probing in that case. A valid
	// 332 is already the best reusable result, so it ends the fan-out early.
	if !allowFanout || isOpenAICodexTurnStateAuthFailure(firstResult) ||
		(firstFailure == "" && firstResult.stateLength >= openAICodexTurnStateLength332) {
		return probes
	}
	remaining := proxies[1:]
	if len(remaining) > openAICodexTurnStateScanFanoutLimit-1 {
		remaining = remaining[:openAICodexTurnStateScanFanoutLimit-1]
	}
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan openAICodexTurnStateProbe, len(remaining))
	var wait sync.WaitGroup
	for _, proxy := range remaining {
		proxy := proxy
		wait.Add(1)
		go func() {
			defer wait.Done()
			result := s.gateway.harvestOpenAICodexTurnState(probeCtx, account, model, proxy.ProxyURL)
			s.updateProxyHealth(ctx, proxy, result)
			select {
			case results <- openAICodexTurnStateProbe{proxy: proxy, result: result}:
			case <-ctx.Done():
			}
		}()
	}
	go func() {
		wait.Wait()
		close(results)
	}()
	for probe := range results {
		probes = append(probes, probe)
		if openAICodexTurnStateScanFailure(model, probe.result, time.Now()) == "" &&
			probe.result.stateLength >= openAICodexTurnStateLength332 {
			// A valid 332 is the target result. Cancel in-flight probes while
			// still draining their bounded result channel to avoid goroutine leaks.
			cancel()
		}
	}
	return probes
}

func isOpenAICodexTurnStateAuthFailure(result openAICodexTurnStateHarvestResult) bool {
	if result.statusCode == http.StatusUnauthorized {
		return true
	}
	message := strings.ToLower(result.errorMessage)
	return strings.Contains(message, "token revoked") || strings.Contains(message, "invalidated oauth token") || strings.Contains(message, "invalid api key")
}

func chooseOpenAICodexTurnStateResult(model string, probes []openAICodexTurnStateProbe) (openAICodexTurnStateHarvestResult, *OpenAICodexTurnStateProxy) {
	var best openAICodexTurnStateProbe
	var bestFailure openAICodexTurnStateProbe
	bestValid := false
	bestFailureSet := false
	now := time.Now()
	for _, probe := range probes {
		if !bestFailureSet || probe.result.stateLength > bestFailure.result.stateLength {
			bestFailure, bestFailureSet = probe, true
		}
		if openAICodexTurnStateScanFailure(model, probe.result, now) != "" {
			continue
		}
		if !bestValid || openAICodexTurnStateProbeRanksBefore(probe, best) {
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

func openAICodexTurnStateProbeRanksBefore(left, right openAICodexTurnStateProbe) bool {
	if left.result.stateLength != right.result.stateLength {
		return left.result.stateLength > right.result.stateLength
	}
	leftIssued, _ := parseOpenAICodexTurnStateIssuedAt(left.result.stateValue)
	rightIssued, _ := parseOpenAICodexTurnStateIssuedAt(right.result.stateValue)
	return leftIssued.After(rightIssued)
}

func openAICodexTurnStateScanFailure(requested string, result openAICodexTurnStateHarvestResult, now time.Time) string {
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

func (s *openAICodexTurnStateScanner) selectProxy(ctx context.Context, accountID int64, model string, attempt int) *OpenAICodexTurnStateProxy {
	proxies := s.selectProxies(ctx, accountID, model, attempt, 1)
	if len(proxies) == 0 {
		return nil
	}
	return proxies[0]
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
		pageSize = 1000
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
	return result, nil
}

func (s *OpsService) EnqueueOpenAICodexTurnStateScan(accountID int64, model string) bool {
	if s == nil || s.codexTurnStateScanner == nil {
		return false
	}
	if strings.TrimSpace(model) == "" {
		model = defaultOpenAICodexTurnStateScanModel
	}
	return s.codexTurnStateScanner.enqueueModels(accountID, []string{model}, false) > 0
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
