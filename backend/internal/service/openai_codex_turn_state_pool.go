package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"math"
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

type OpenAICodexTurnStateStore interface {
	UpsertOpenAICodexTurnState(ctx context.Context, record *OpenAICodexTurnStateRecord) error
	LoadActiveOpenAICodexTurnStates(ctx context.Context, now time.Time) ([]*OpenAICodexTurnStateRecord, error)
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
	accounts          map[int64]time.Time
	preferredByBucket map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateRecord
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
		accounts:          make(map[int64]time.Time),
		preferredByBucket: make(map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateRecord),
		queue:             make(chan *OpenAICodexTurnStateRecord, openAICodexTurnStatePersistQueue),
		now:               time.Now,
	}
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

func (p *openAICodexTurnStatePool) observe(value string, accountID *int64, sessionHash, model, transport string) {
	if p == nil {
		return
	}
	value = strings.TrimSpace(value)
	if !isValidOpenAICodexTurnState(value) {
		return
	}
	now := p.now()
	issuedAt, issuedOK := parseOpenAICodexTurnStateIssuedAt(value)
	if issuedOK && issuedAt.After(now) {
		return
	}
	expiresAt := now
	if issuedOK && !issuedAt.After(now) {
		expiresAt = issuedAt.Add(openAICodexTurnStateTTL)
	}
	record := &OpenAICodexTurnStateRecord{
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

func (p *openAICodexTurnStatePool) preferredForBucket(accountID int64, model string) (string, bool) {
	key, ok := newOpenAICodexTurnStateBucketKey(accountID, model)
	if p == nil || !ok {
		return "", false
	}
	now := p.now()
	p.mu.RLock()
	selected := p.preferredByBucket[key]
	if p.isPreferredBucketRecordLocked(selected, key, now) {
		value := selected.StateValue
		p.mu.RUnlock()
		return value, true
	}
	p.mu.RUnlock()

	p.mu.Lock()
	p.selectPreferredForBucketLocked(key, now)
	selected = p.preferredByBucket[key]
	if selected == nil {
		p.mu.Unlock()
		return "", false
	}
	value := selected.StateValue
	p.mu.Unlock()
	return value, value != ""
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
	key, ok := newOpenAICodexTurnStateBucketKey(accountID, model)
	if p == nil || !ok {
		return false
	}
	now := p.now()
	p.mu.RLock()
	defer p.mu.RUnlock()
	selected := p.preferredByBucket[key]
	if p.isPreferredBucketRecordLocked(selected, key, now) && selected.ExpiresAt.After(deadline) {
		return true
	}
	for _, record := range p.entries {
		if isReusableOpenAICodexTurnStateRecord(record, key, now) && record.ExpiresAt.After(deadline) {
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
		record.ExpiresAt = record.IssuedAt.Add(openAICodexTurnStateTTL)
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
		if key, ok := newOpenAICodexTurnStateBucketKey(accountID, copyRecord.SourceModel); ok && isReusableOpenAICodexTurnStateRecord(copyRecord, key, now) {
			current := p.preferredByBucket[key]
			if !p.isPreferredBucketRecordLocked(current, key, now) || openAICodexTurnStateRanksBefore(copyRecord, current) {
				p.preferredByBucket[key] = copyRecord
			}
		}
	}
}

func (p *openAICodexTurnStatePool) rebuildAccountsLocked() {
	p.accounts = make(map[int64]time.Time)
	p.preferredByBucket = make(map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateRecord)
	now := p.now()
	for _, record := range p.entries {
		if record == nil || !record.ExpiresAt.After(now) {
			continue
		}
		if record != nil && record.SourceAccountID != nil && *record.SourceAccountID > 0 {
			accountID := *record.SourceAccountID
			if current := p.accounts[accountID]; record.ExpiresAt.After(current) {
				p.accounts[accountID] = record.ExpiresAt
			}
			if key, ok := newOpenAICodexTurnStateBucketKey(accountID, record.SourceModel); ok && isReusableOpenAICodexTurnStateRecord(record, key, now) {
				current := p.preferredByBucket[key]
				if current == nil || openAICodexTurnStateRanksBefore(record, current) {
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
	if !isReusableOpenAICodexTurnStateRecord(record, key, now) {
		return false
	}
	current := p.entries[openAICodexTurnStateEntryKey(record)]
	return current != nil && current.StateHash == record.StateHash && isReusableOpenAICodexTurnStateRecord(current, key, now)
}

func (p *openAICodexTurnStatePool) selectPreferredForBucketLocked(key openAICodexTurnStateBucketKey, now time.Time) {
	delete(p.preferredByBucket, key)
	for _, record := range p.entries {
		if !isReusableOpenAICodexTurnStateRecord(record, key, now) {
			continue
		}
		current := p.preferredByBucket[key]
		if current == nil || openAICodexTurnStateRanksBefore(record, current) {
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

func openAICodexTurnStateRanksBefore(left, right *OpenAICodexTurnStateRecord) bool {
	if left.ValueLength != right.ValueLength {
		return left.ValueLength > right.ValueLength
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
	default:
		return "http"
	}
}

func normalizeOpenAICodexTurnStateModel(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func parseOpenAICodexTurnStateIssuedAt(value string) (time.Time, bool) {
	encoded := strings.TrimRight(strings.TrimSpace(value), "=")
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) < 9 || raw[0] != 0x80 {
		return time.Time{}, false
	}
	seconds := binary.BigEndian.Uint64(raw[1:9])
	if seconds > math.MaxInt64 {
		return time.Time{}, false
	}
	return time.Unix(int64(seconds), 0).UTC(), true
}

func isReusableOpenAICodexTurnStateRecord(record *OpenAICodexTurnStateRecord, key openAICodexTurnStateBucketKey, now time.Time) bool {
	if record == nil || record.StateValue == "" || !isReusableOpenAICodexTurnStateLength(record.ValueLength) {
		return false
	}
	if record.SourceAccountID == nil || *record.SourceAccountID != key.accountID || normalizeOpenAICodexTurnStateModel(record.SourceModel) != key.model {
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
	return &copyRecord
}
