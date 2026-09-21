package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http/httpguts"
)

const (
	openAICodexTurnStateTTL          = time.Hour
	openAICodexTurnStatePersistQueue = 1024
	openAICodexTurnStateSweepEvery   = 256
	openAICodexTurnStateCleanupEvery = 5 * time.Minute
	openAICodexTurnStateHistoryTTL   = 24 * time.Hour
)

type OpenAICodexTurnStateRecord struct {
	ID                int64     `json:"id"`
	StateValue        string    `json:"-"`
	StateHash         string    `json:"state_hash"`
	MaskedValue       string    `json:"masked_value"`
	ValueLength       int       `json:"value_length"`
	SourceAccountID   *int64    `json:"source_account_id"`
	SourceAccountName string    `json:"source_account_name,omitempty"`
	SourceSessionHash string    `json:"source_session_hash,omitempty"`
	SourceSessionID   string    `json:"-"`
	SourceProxyID     *int64    `json:"source_proxy_id,omitempty"`
	SourceProxyURL    string    `json:"-"`
	SourceExitIP      string    `json:"source_exit_ip,omitempty"`
	RouteIPv6         string    `json:"route_ipv6,omitempty"`
	SourceModel       string    `json:"source_model,omitempty"`
	SourceTransport   string    `json:"source_transport"`
	IssuedAt          time.Time `json:"issued_at,omitempty"`
	FirstSeenAt       time.Time `json:"first_seen_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	Active            bool      `json:"active"`
}

type OpenAICodexTurnStateList struct {
	Items    []*OpenAICodexTurnStateRecord `json:"items"`
	Total    int64                         `json:"total"`
	Page     int                           `json:"page"`
	PageSize int                           `json:"page_size"`
}

type OpenAICodexTurnStateSummary struct {
	ActiveCount     int64                       `json:"active_count"`
	ExpiredCount    int64                       `json:"expired_count"`
	LongestActive   *OpenAICodexTurnStateRecord `json:"longest_active,omitempty"`
	ReuseTTLSeconds int64                       `json:"reuse_ttl_seconds"`
}

type OpenAICodexTurnStateFilter struct {
	Status    string
	AccountID *int64
	Page      int
	PageSize  int
}

type openAICodexTurnStateRouteTicket struct {
	SessionID string
	ProxyID   *int64
	ProxyURL  string
	ExitIP    string
	RouteIPv6 string
}

type OpenAICodexTurnStateStore interface {
	UpsertOpenAICodexTurnState(ctx context.Context, record *OpenAICodexTurnStateRecord) error
	LoadActiveOpenAICodexTurnStates(ctx context.Context, now time.Time) ([]*OpenAICodexTurnStateRecord, error)
}

// OpenAICodexTurnStateBucketStore is an optional cold-path synchronization
// interface for workers sharing a database but keeping separate memory pools.
type OpenAICodexTurnStateBucketStore interface {
	LoadPreferredOpenAICodexTurnState(ctx context.Context, accountID int64, model string, targetLengths []int, now time.Time) (*OpenAICodexTurnStateRecord, error)
}

type OpenAICodexTurnStateAdminRepository interface {
	OpenAICodexTurnStateStore
	BatchUpsertOpenAICodexTurnStates(ctx context.Context, records []*OpenAICodexTurnStateRecord) error
	DeleteOpenAICodexTurnStates(ctx context.Context, ids []int64) ([]string, error)
	ListOpenAICodexTurnStates(ctx context.Context, filter *OpenAICodexTurnStateFilter) (*OpenAICodexTurnStateList, error)
	GetOpenAICodexTurnStateSummary(ctx context.Context, now time.Time) (*OpenAICodexTurnStateSummary, error)
}

type openAICodexTurnStateBucketKey struct {
	accountID int64
	model     string
}

type openAICodexTurnStatePool struct {
	mu                sync.RWMutex
	entries           map[string]*OpenAICodexTurnStateRecord
	entriesByBucket   map[openAICodexTurnStateBucketKey]map[string]*OpenAICodexTurnStateRecord
	accounts          map[int64]time.Time
	accountPlans      map[int64]string
	preferredByBucket map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateRecord
	scanSettings      *OpenAICodexTurnStateScanSettings
	repo              OpenAICodexTurnStateStore
	queue             chan *OpenAICodexTurnStateRecord
	worker            sync.Once
	dropped           atomic.Uint64
	observed          atomic.Uint64
	now               func() time.Time
}

func newOpenAICodexTurnStatePool() *openAICodexTurnStatePool {
	return &openAICodexTurnStatePool{
		entries:           make(map[string]*OpenAICodexTurnStateRecord),
		entriesByBucket:   make(map[openAICodexTurnStateBucketKey]map[string]*OpenAICodexTurnStateRecord),
		accounts:          make(map[int64]time.Time),
		accountPlans:      make(map[int64]string),
		preferredByBucket: make(map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateRecord),
		scanSettings:      defaultOpenAICodexTurnStateScanSettings(),
		queue:             make(chan *OpenAICodexTurnStateRecord, openAICodexTurnStatePersistQueue),
		now:               time.Now,
	}
}

func (p *openAICodexTurnStatePool) setTargetLengths(lengths []int) {
	if p == nil || len(lengths) == 0 {
		return
	}
	p.setScanSettings(&OpenAICodexTurnStateScanSettings{TargetLengths: slices.Clone(lengths)})
}

func (p *openAICodexTurnStatePool) setScanSettings(settings *OpenAICodexTurnStateScanSettings) {
	if p == nil {
		return
	}
	next := settings.clone()
	p.mu.Lock()
	defer p.mu.Unlock()
	if reflect.DeepEqual(p.scanSettings, next) {
		return
	}
	p.scanSettings = next
	p.rebuildAccountsLocked()
}

func (p *openAICodexTurnStatePool) isStateRequiredBeforeRouting() bool {
	if p == nil {
		return true
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.scanSettings.IsStateRequiredBeforeRouting()
}

// setAccountPlan registers the credential owner's subscription without touching
// persistent storage. Repeated requests with an unchanged plan take only a read lock.
func (p *openAICodexTurnStatePool) setAccountPlan(accountID int64, planType string) {
	if p == nil || accountID <= 0 {
		return
	}
	planType = NormalizeOpenAICodexStatePlanType(planType)
	p.mu.RLock()
	current := p.accountPlans[accountID]
	p.mu.RUnlock()
	if current == planType {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.accountPlans[accountID] == planType {
		return
	}
	p.accountPlans[accountID] = planType
	now := p.now()
	for key := range p.entriesByBucket {
		if key.accountID == accountID {
			p.selectPreferredForBucketLocked(key, now)
		}
	}
}

func (p *openAICodexTurnStatePool) targetLengthsForBucketLocked(key openAICodexTurnStateBucketKey) []int {
	return p.scanSettings.TargetLengthsFor(p.accountPlans[key.accountID], key.model)
}

func (p *openAICodexTurnStatePool) setRepository(ctx context.Context, repo OpenAICodexTurnStateStore) {
	if p == nil || repo == nil {
		return
	}
	p.mu.Lock()
	p.repo = repo
	p.mu.Unlock()

	active, err := repo.LoadActiveOpenAICodexTurnStates(ctx, p.now())
	if err != nil {
		log.WithError(err).Warn("failed to load active Codex turn-state pool")
	} else {
		p.mu.Lock()
		for _, record := range active {
			p.mergeLocked(record)
		}
		p.mu.Unlock()
	}

	p.worker.Do(func() { go p.runPersistenceWorker() })
}

// refreshBucket imports a persisted state without recording another observation
// or extending its lifetime. A missing row leaves unflushed local records intact.
func (p *openAICodexTurnStatePool) refreshBucket(ctx context.Context, accountID int64, model string) error {
	key, validKey := newOpenAICodexTurnStateBucketKey(accountID, model)
	if p == nil || !validKey {
		return nil
	}
	p.mu.RLock()
	repo, supported := p.repo.(OpenAICodexTurnStateBucketStore)
	targetLengths := slices.Clone(p.targetLengthsForBucketLocked(key))
	p.mu.RUnlock()
	if !supported {
		return nil
	}
	if len(targetLengths) == 0 {
		return fmt.Errorf("no configured Codex turn-state lengths for account/model bucket")
	}
	record, err := repo.LoadPreferredOpenAICodexTurnState(ctx, key.accountID, key.model, targetLengths, p.now())
	if err != nil {
		return fmt.Errorf("load persisted Codex turn-state bucket: %w", err)
	}
	if record == nil {
		return nil
	}
	if record.SourceAccountID == nil || *record.SourceAccountID != key.accountID || normalizeOpenAICodexTurnStateModel(record.SourceModel) != key.model {
		return fmt.Errorf("persisted Codex turn-state account/model scope mismatch")
	}
	issuedAt, issuedOK := parseOpenAICodexTurnStateIssuedAt(record.StateValue)
	if !isValidOpenAICodexTurnState(record.StateValue) || record.ValueLength != len(record.StateValue) ||
		record.StateHash != hashOpenAICodexTurnState(record.StateValue) || !issuedOK || !issuedAt.Equal(record.IssuedAt) {
		return fmt.Errorf("persisted Codex turn-state metadata mismatch")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// Settings and time may change during the query. Recheck before merging.
	if p.isReusableRecordLocked(record, key, p.now()) {
		p.mergeLocked(record)
	}
	return nil
}

func (p *openAICodexTurnStatePool) observe(value string, accountID *int64, sessionHash, model, transport string) {
	if p == nil {
		return
	}
	now := p.now()
	record := newObservedOpenAICodexTurnStateRecord(value, accountID, sessionHash, model, transport, now)
	if record == nil {
		return
	}
	// Scanner observations are already model-confirmed. Direct scanner callers
	// (primarily tests and local probes) pass the actual session as sessionHash;
	// the durable production path below overwrites this with its explicit ticket.
	if record.SourceTransport == "scanner" {
		record.SourceSessionID = strings.TrimSpace(sessionHash)
	}

	p.mu.Lock()
	p.mergeLocked(record)
	if p.observed.Add(1)%openAICodexTurnStateSweepEvery == 0 {
		p.pruneExpiredLocked(now)
		p.rebuildAccountsLocked()
	}
	repoReady := p.repo != nil
	p.mu.Unlock()
	if !repoReady {
		return
	}

	select {
	case p.queue <- cloneOpenAICodexTurnStateRecord(record):
	default:
		if dropped := p.dropped.Add(1); dropped == 1 || dropped%128 == 0 {
			log.WithField("dropped_total", dropped).Warn("Codex turn-state persistence queue is full")
		}
	}
}

// observeDurably publishes a scan result only after it is visible to other
// instances. It never uses the asynchronous observation queue.
func (p *openAICodexTurnStatePool) observeDurably(ctx context.Context, value string, accountID int64, sessionHash, model, transport string, ticket ...openAICodexTurnStateRouteTicket) error {
	key, validKey := newOpenAICodexTurnStateBucketKey(accountID, model)
	if p == nil || !validKey {
		return fmt.Errorf("invalid Codex turn-state observation bucket")
	}
	record := newObservedOpenAICodexTurnStateRecord(value, &accountID, sessionHash, key.model, transport, p.now())
	if record == nil {
		return fmt.Errorf("invalid Codex turn-state observation")
	}
	if len(ticket) > 0 {
		record.SourceSessionID = strings.TrimSpace(ticket[0].SessionID)
		record.SourceProxyID = cloneInt64Pointer(ticket[0].ProxyID)
		record.SourceProxyURL = strings.TrimSpace(ticket[0].ProxyURL)
		record.SourceExitIP = strings.TrimSpace(ticket[0].ExitIP)
		record.RouteIPv6 = strings.TrimSpace(ticket[0].RouteIPv6)
	}
	p.mu.RLock()
	repo := p.repo
	usable := p.isReusableRecordLocked(record, key, p.now())
	p.mu.RUnlock()
	if !usable {
		return fmt.Errorf("codex turn-state observation is not reusable under current settings")
	}
	if repo != nil {
		if err := repo.UpsertOpenAICodexTurnState(ctx, record); err != nil {
			return fmt.Errorf("persist Codex turn-state observation: %w", err)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.isReusableRecordLocked(record, key, p.now()) {
		return fmt.Errorf("codex turn-state observation expired or settings changed during persistence")
	}
	p.mergeLocked(record)
	return nil
}

func newObservedOpenAICodexTurnStateRecord(value string, accountID *int64, sessionHash, model, transport string, now time.Time) *OpenAICodexTurnStateRecord {
	value = strings.TrimSpace(value)
	if !isValidOpenAICodexTurnState(value) {
		return nil
	}
	issuedAt, issuedOK := parseOpenAICodexTurnStateIssuedAt(value)
	if issuedOK && issuedAt.After(now) {
		return nil
	}
	expiresAt := now
	if issuedOK && !issuedAt.After(now) {
		expiresAt = issuedAt.Add(openAICodexTurnStateTTL)
	}
	return &OpenAICodexTurnStateRecord{
		StateValue:        value,
		StateHash:         hashOpenAICodexTurnState(value),
		ValueLength:       len(value),
		SourceAccountID:   cloneInt64Pointer(accountID),
		SourceSessionHash: sessionHash,
		SourceModel:       normalizeOpenAICodexTurnStateModel(model),
		SourceTransport:   normalizeOpenAICodexTurnStateTransport(transport),
		IssuedAt:          issuedAt,
		FirstSeenAt:       now,
		LastSeenAt:        now,
		ExpiresAt:         expiresAt,
		Active:            expiresAt.After(now),
	}
}

func (p *openAICodexTurnStatePool) preferredForBucket(accountID int64, model string) (string, bool) {
	record, ok := p.preferredRecordForBucket(accountID, model)
	if !ok {
		return "", false
	}
	return record.StateValue, true
}

func (p *openAICodexTurnStatePool) preferredRecordForBucket(accountID int64, model string) (*OpenAICodexTurnStateRecord, bool) {
	key, ok := newOpenAICodexTurnStateBucketKey(accountID, model)
	if p == nil || !ok {
		return nil, false
	}
	now := p.now()
	p.mu.RLock()
	selected := p.preferredByBucket[key]
	if p.isPreferredBucketRecordLocked(selected, key, now) {
		record := cloneOpenAICodexTurnStateRecord(selected)
		p.mu.RUnlock()
		return record, true
	}
	p.mu.RUnlock()

	p.mu.Lock()
	p.selectPreferredForBucketLocked(key, now)
	selected = p.preferredByBucket[key]
	if selected == nil {
		p.mu.Unlock()
		return nil, false
	}
	record := cloneOpenAICodexTurnStateRecord(selected)
	p.mu.Unlock()
	return record, record.StateValue != ""
}

func (p *openAICodexTurnStatePool) routeProxyForState(accountID int64, state string) (string, bool) {
	proxyURL, _, ok := p.routeForState(accountID, state)
	return proxyURL, ok && proxyURL != ""
}

func (p *openAICodexTurnStatePool) routeForState(accountID int64, state string) (string, string, bool) {
	if p == nil || accountID <= 0 {
		return "", "", false
	}
	state = strings.TrimSpace(state)
	if state == "" {
		return "", "", false
	}
	now := p.now()
	p.mu.RLock()
	defer p.mu.RUnlock()
	for key, records := range p.entriesByBucket {
		if key.accountID != accountID {
			continue
		}
		for _, record := range records {
			if record.StateValue != state || !p.isReusableRecordLocked(record, key, now) {
				continue
			}
			if routeIPv6 := normalizeOpenAICodexRouteIPv6(record.RouteIPv6); routeIPv6 != "" {
				return "", routeIPv6, true
			}
			if proxyURL := strings.TrimSpace(record.SourceProxyURL); proxyURL != "" {
				return proxyURL, "", true
			}
		}
	}
	return "", "", false
}

func normalizeOpenAICodexRouteIPv6(value string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || !addr.Is6() || addr.Is4In6() || addr.IsUnspecified() {
		return ""
	}
	return addr.String()
}

func (p *openAICodexTurnStatePool) preferredExpiryForBucket(accountID int64, model string) (time.Time, bool) {
	key, ok := newOpenAICodexTurnStateBucketKey(accountID, model)
	if p == nil || !ok {
		return time.Time{}, false
	}
	now := p.now()
	p.mu.RLock()
	selected := p.preferredByBucket[key]
	if p.isPreferredBucketRecordLocked(selected, key, now) {
		expiresAt := selected.ExpiresAt
		p.mu.RUnlock()
		return expiresAt, true
	}
	p.mu.RUnlock()

	p.mu.Lock()
	p.selectPreferredForBucketLocked(key, now)
	selected = p.preferredByBucket[key]
	if selected == nil {
		p.mu.Unlock()
		return time.Time{}, false
	}
	expiresAt := selected.ExpiresAt
	p.mu.Unlock()
	return expiresAt, true
}

func (p *openAICodexTurnStatePool) hasReusableStateBeyond(accountID int64, model string, deadline time.Time) bool {
	return p.hasReusableStateOfLengthBeyond(accountID, model, 0, deadline)
}

// hasReusableStateOfLengthBeyond reports whether this account/model has a
// reusable state whose value length is at least minLength and whose signed
// lifetime extends beyond deadline. States are deliberately scoped by both
// account and model; a state observed on another bucket is never a fallback.
func (p *openAICodexTurnStatePool) hasReusableStateOfLengthBeyond(accountID int64, model string, minLength int, deadline time.Time) bool {
	key, ok := newOpenAICodexTurnStateBucketKey(accountID, model)
	if p == nil || !ok {
		return false
	}
	now := p.now()
	p.mu.RLock()
	defer p.mu.RUnlock()
	selected := p.preferredByBucket[key]
	if p.isPreferredBucketRecordLocked(selected, key, now) && selected.ValueLength >= minLength && selected.ExpiresAt.After(deadline) {
		return true
	}
	for _, record := range p.entriesByBucket[key] {
		if p.isReusableRecordLocked(record, key, now) && record.ValueLength >= minLength && record.ExpiresAt.After(deadline) {
			return true
		}
	}
	return false
}

func (p *openAICodexTurnStatePool) hasSampledAccount(accountID int64) bool {
	if p == nil || accountID <= 0 {
		return false
	}
	p.mu.RLock()
	expiresAt, ok := p.accounts[accountID]
	p.mu.RUnlock()
	return ok && expiresAt.After(p.now())
}

func (p *openAICodexTurnStatePool) removeHashes(hashes []string) {
	if p == nil || len(hashes) == 0 {
		return
	}
	p.mu.Lock()
	for _, hash := range hashes {
		hash = strings.TrimSpace(hash)
		for key, record := range p.entries {
			if record != nil && record.StateHash == hash {
				delete(p.entries, key)
			}
		}
	}
	p.rebuildAccountsLocked()
	p.mu.Unlock()
}

func (p *openAICodexTurnStatePool) mergeLocked(record *OpenAICodexTurnStateRecord) {
	if record == nil || record.StateValue == "" {
		return
	}
	if record.StateHash == "" {
		record.StateHash = hashOpenAICodexTurnState(record.StateValue)
	}
	if record.ValueLength <= 0 {
		record.ValueLength = len(record.StateValue)
	}
	record.SourceModel = normalizeOpenAICodexTurnStateModel(record.SourceModel)
	if !record.IssuedAt.IsZero() {
		issuedExpiry := record.IssuedAt.Add(openAICodexTurnStateTTL)
		if record.ExpiresAt.IsZero() || record.ExpiresAt.After(issuedExpiry) {
			record.ExpiresAt = issuedExpiry
		}
	}
	entryKey := openAICodexTurnStateEntryKey(record)
	existing := p.entries[entryKey]
	if existing != nil && existing.LastSeenAt.After(record.LastSeenAt) {
		return
	}
	copyRecord := cloneOpenAICodexTurnStateRecord(record)
	p.entries[entryKey] = copyRecord
	now := p.now()
	if copyRecord.SourceAccountID != nil && *copyRecord.SourceAccountID > 0 {
		accountID := *copyRecord.SourceAccountID
		if current := p.accounts[accountID]; copyRecord.ExpiresAt.After(current) {
			p.accounts[accountID] = copyRecord.ExpiresAt
		}
		if key, ok := newOpenAICodexTurnStateBucketKey(accountID, copyRecord.SourceModel); ok {
			if p.entriesByBucket[key] == nil {
				p.entriesByBucket[key] = make(map[string]*OpenAICodexTurnStateRecord)
			}
			p.entriesByBucket[key][entryKey] = copyRecord
			if !p.isReusableRecordLocked(copyRecord, key, now) {
				return
			}
			current := p.preferredByBucket[key]
			if !p.isPreferredBucketRecordLocked(current, key, now) || p.ranksBeforeLocked(copyRecord, current) {
				p.preferredByBucket[key] = copyRecord
			}
		}
	}
}

func (p *openAICodexTurnStatePool) rebuildAccountsLocked() {
	p.accounts = make(map[int64]time.Time)
	p.preferredByBucket = make(map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateRecord)
	p.entriesByBucket = make(map[openAICodexTurnStateBucketKey]map[string]*OpenAICodexTurnStateRecord)
	now := p.now()
	for entryKey, record := range p.entries {
		if record == nil || !record.ExpiresAt.After(now) {
			continue
		}
		if record != nil && record.SourceAccountID != nil && *record.SourceAccountID > 0 {
			accountID := *record.SourceAccountID
			if current := p.accounts[accountID]; record.ExpiresAt.After(current) {
				p.accounts[accountID] = record.ExpiresAt
			}
			if key, ok := newOpenAICodexTurnStateBucketKey(accountID, record.SourceModel); ok {
				if p.entriesByBucket[key] == nil {
					p.entriesByBucket[key] = make(map[string]*OpenAICodexTurnStateRecord)
				}
				p.entriesByBucket[key][entryKey] = record
				if !p.isReusableRecordLocked(record, key, now) {
					continue
				}
				current := p.preferredByBucket[key]
				if current == nil || p.ranksBeforeLocked(record, current) {
					p.preferredByBucket[key] = record
				}
			}
		}
	}
}

func newOpenAICodexTurnStateBucketKey(accountID int64, model string) (openAICodexTurnStateBucketKey, bool) {
	model = normalizeOpenAICodexTurnStateModel(model)
	key := openAICodexTurnStateBucketKey{accountID: accountID, model: model}
	return key, accountID > 0 && model != ""
}

func (p *openAICodexTurnStatePool) isPreferredBucketRecordLocked(record *OpenAICodexTurnStateRecord, key openAICodexTurnStateBucketKey, now time.Time) bool {
	if !p.isReusableRecordLocked(record, key, now) {
		return false
	}
	current := p.entries[openAICodexTurnStateEntryKey(record)]
	return current != nil && current.StateHash == record.StateHash && p.isReusableRecordLocked(current, key, now)
}

func (p *openAICodexTurnStatePool) selectPreferredForBucketLocked(key openAICodexTurnStateBucketKey, now time.Time) {
	delete(p.preferredByBucket, key)
	for _, record := range p.entriesByBucket[key] {
		if !p.isReusableRecordLocked(record, key, now) {
			continue
		}
		current := p.preferredByBucket[key]
		if current == nil || p.ranksBeforeLocked(record, current) {
			p.preferredByBucket[key] = record
		}
	}
}

func (p *openAICodexTurnStatePool) pruneExpiredLocked(now time.Time) {
	for hash, record := range p.entries {
		if record == nil || !record.ExpiresAt.After(now) {
			delete(p.entries, hash)
		}
	}
}

func (p *openAICodexTurnStatePool) runPersistenceWorker() {
	ticker := time.NewTicker(openAICodexTurnStateCleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case record := <-p.queue:
			p.mu.RLock()
			repo := p.repo
			p.mu.RUnlock()
			if repo == nil {
				continue
			}
			if err := repo.UpsertOpenAICodexTurnState(context.Background(), record); err != nil {
				log.WithError(err).WithField("state_hash", record.StateHash).Warn("failed to persist Codex turn-state")
			}
		case now := <-ticker.C:
			p.cleanupExpired(now)
		}
	}
}

func (p *openAICodexTurnStatePool) cleanupExpired(now time.Time) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.pruneExpiredLocked(now)
	p.rebuildAccountsLocked()
	repo := p.repo
	p.mu.Unlock()

	pruner, ok := repo.(interface {
		DeleteOpenAICodexTurnStatesExpiredBefore(context.Context, time.Time) error
	})
	if !ok {
		return
	}
	if err := pruner.DeleteOpenAICodexTurnStatesExpiredBefore(context.Background(), now.Add(-openAICodexTurnStateHistoryTTL)); err != nil {
		log.WithError(err).Warn("failed to prune expired Codex turn-state history")
	}
}

func (p *openAICodexTurnStatePool) ranksBeforeLocked(left, right *OpenAICodexTurnStateRecord) bool {
	if left.ValueLength != right.ValueLength {
		key := openAICodexTurnStateBucketKey{model: left.SourceModel}
		if left.SourceAccountID != nil {
			key.accountID = *left.SourceAccountID
		}
		lengths := p.targetLengthsForBucketLocked(key)
		return slices.Index(lengths, left.ValueLength) < slices.Index(lengths, right.ValueLength)
	}
	if !left.ExpiresAt.Equal(right.ExpiresAt) {
		return left.ExpiresAt.After(right.ExpiresAt)
	}
	if !left.LastSeenAt.Equal(right.LastSeenAt) {
		return left.LastSeenAt.After(right.LastSeenAt)
	}
	return left.StateHash > right.StateHash
}

func hashOpenAICodexTurnState(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func maskOpenAICodexTurnState(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 12 {
		return value[:min(4, len(value))] + "..."
	}
	return value[:6] + "..." + value[len(value)-6:]
}

func normalizeOpenAICodexTurnStateTransport(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ws", "websocket":
		return "ws"
	case "passthrough":
		return "passthrough"
	case "scanner":
		return "scanner"
	default:
		return "http"
	}
}

func normalizeOpenAICodexTurnStateModel(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func parseOpenAICodexTurnStateIssuedAt(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	encoded := strings.TrimRight(value, "=")
	if len(encoded) < 12 || len(value)-len(encoded) > 2 || strings.IndexFunc(encoded, func(r rune) bool {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return false
		default:
			return true
		}
	}) >= 0 {
		return time.Time{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		// The suffix is opaque and a configured length need not form a complete
		// Base64 block. Reading only the version/timestamp prefix neither
		// authenticates nor renews a state.
		raw, err = base64.RawURLEncoding.DecodeString(encoded[:12])
	}
	if err != nil || len(raw) < 9 || raw[0] != 0x80 {
		return time.Time{}, false
	}
	seconds := binary.BigEndian.Uint64(raw[1:9])
	if seconds > math.MaxInt64 {
		return time.Time{}, false
	}
	return time.Unix(int64(seconds), 0).UTC(), true
}

func (p *openAICodexTurnStatePool) isReusableRecordLocked(record *OpenAICodexTurnStateRecord, key openAICodexTurnStateBucketKey, now time.Time) bool {
	if record == nil || record.StateValue == "" || !slices.Contains(p.targetLengthsForBucketLocked(key), record.ValueLength) {
		return false
	}
	if record.SourceAccountID == nil || *record.SourceAccountID != key.accountID || normalizeOpenAICodexTurnStateModel(record.SourceModel) != key.model {
		return false
	}
	// A reusable route ticket is the pair minted by a scanner request whose
	// response explicitly declared the requested model. Business responses are
	// still recorded for diagnostics, but their state alone is not proof that
	// the request avoided an upstream model downgrade.
	if normalizeOpenAICodexTurnStateTransport(record.SourceTransport) != "scanner" || strings.TrimSpace(record.SourceSessionID) == "" {
		return false
	}
	if p.scanSettings.IsRouteBindingRequired() && normalizeOpenAICodexRouteIPv6(record.RouteIPv6) == "" && strings.TrimSpace(record.SourceProxyURL) == "" {
		return false
	}
	if record.IssuedAt.IsZero() || record.IssuedAt.After(now) || !record.IssuedAt.Add(openAICodexTurnStateTTL).After(now) {
		return false
	}
	return record.ExpiresAt.After(now)
}

func openAICodexTurnStateEntryKey(record *OpenAICodexTurnStateRecord) string {
	if record == nil {
		return ""
	}
	accountID := int64(0)
	if record.SourceAccountID != nil {
		accountID = *record.SourceAccountID
	}
	return strconv.FormatInt(accountID, 10) + "\x00" + normalizeOpenAICodexTurnStateModel(record.SourceModel) + "\x00" + record.StateHash
}

func isValidOpenAICodexTurnState(value string) bool {
	return value != "" && !strings.ContainsAny(value, "\r\n") && httpguts.ValidHeaderFieldValue(value)
}

func cloneOpenAICodexTurnStateRecord(record *OpenAICodexTurnStateRecord) *OpenAICodexTurnStateRecord {
	if record == nil {
		return nil
	}
	copyRecord := *record
	copyRecord.SourceAccountID = cloneInt64Pointer(record.SourceAccountID)
	copyRecord.SourceProxyID = cloneInt64Pointer(record.SourceProxyID)
	return &copyRecord
}
