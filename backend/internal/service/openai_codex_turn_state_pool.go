package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http/httpguts"
)

const (
	openAICodexTurnStateTTL          = 30 * time.Minute
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
	SourceTransport   string    `json:"source_transport"`
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

type openAICodexTurnStateSessionKey struct {
	accountID   int64
	sessionHash string
}

type openAICodexTurnStatePool struct {
	mu                 sync.RWMutex
	entries            map[string]*OpenAICodexTurnStateRecord
	accounts           map[int64]time.Time
	preferredBySession map[openAICodexTurnStateSessionKey]*OpenAICodexTurnStateRecord
	selected           *OpenAICodexTurnStateRecord
	repo               OpenAICodexTurnStateStore
	queue              chan *OpenAICodexTurnStateRecord
	worker             sync.Once
	dropped            atomic.Uint64
	observed           atomic.Uint64
	now                func() time.Time
}

func newOpenAICodexTurnStatePool() *openAICodexTurnStatePool {
	return &openAICodexTurnStatePool{
		entries:            make(map[string]*OpenAICodexTurnStateRecord),
		accounts:           make(map[int64]time.Time),
		preferredBySession: make(map[openAICodexTurnStateSessionKey]*OpenAICodexTurnStateRecord),
		queue:              make(chan *OpenAICodexTurnStateRecord, openAICodexTurnStatePersistQueue),
		now:                time.Now,
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

func (p *openAICodexTurnStatePool) observe(value string, accountID *int64, sessionHash, transport string) {
	if p == nil {
		return
	}
	value = strings.TrimSpace(value)
	if !isValidOpenAICodexTurnState(value) {
		return
	}
	now := p.now()
	record := &OpenAICodexTurnStateRecord{
		StateValue:        value,
		StateHash:         hashOpenAICodexTurnState(value),
		ValueLength:       len(value),
		SourceAccountID:   cloneInt64Pointer(accountID),
		SourceSessionHash: sessionHash,
		SourceTransport:   normalizeOpenAICodexTurnStateTransport(transport),
		FirstSeenAt:       now,
		LastSeenAt:        now,
		ExpiresAt:         now.Add(openAICodexTurnStateTTL),
		Active:            true,
	}

	p.mu.Lock()
	p.mergeLocked(record)
	if p.observed.Add(1)%openAICodexTurnStateSweepEvery == 0 {
		p.selectLongestLocked(now)
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

func (p *openAICodexTurnStatePool) longestActive() (string, bool) {
	if p == nil {
		return "", false
	}
	now := p.now()
	p.mu.RLock()
	selected := p.selected
	if selected != nil && selected.ExpiresAt.After(now) && selected.StateValue != "" {
		value := selected.StateValue
		p.mu.RUnlock()
		return value, true
	}
	p.mu.RUnlock()

	p.mu.Lock()
	p.selectLongestLocked(now)
	selected = p.selected
	if selected == nil {
		p.mu.Unlock()
		return "", false
	}
	value := selected.StateValue
	p.mu.Unlock()
	return value, value != ""
}

func (p *openAICodexTurnStatePool) preferredForSession(accountID int64, sessionHash string) (string, bool) {
	key, ok := newOpenAICodexTurnStateSessionKey(accountID, sessionHash)
	if p == nil || !ok {
		return "", false
	}
	now := p.now()
	p.mu.RLock()
	selected := p.preferredBySession[key]
	if p.isPreferredSessionRecordLocked(selected, key, now) {
		value := selected.StateValue
		p.mu.RUnlock()
		return value, true
	}
	p.mu.RUnlock()

	p.mu.Lock()
	p.selectPreferredForSessionLocked(key, now)
	selected = p.preferredBySession[key]
	if selected == nil {
		p.mu.Unlock()
		return "", false
	}
	value := selected.StateValue
	p.mu.Unlock()
	return value, value != ""
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
		delete(p.entries, strings.TrimSpace(hash))
	}
	p.rebuildAccountsLocked()
	p.selectLongestLocked(p.now())
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
	existing := p.entries[record.StateHash]
	if existing != nil && existing.LastSeenAt.After(record.LastSeenAt) {
		return
	}
	copyRecord := cloneOpenAICodexTurnStateRecord(record)
	p.entries[record.StateHash] = copyRecord
	if copyRecord.SourceAccountID != nil && *copyRecord.SourceAccountID > 0 {
		accountID := *copyRecord.SourceAccountID
		if current := p.accounts[accountID]; copyRecord.ExpiresAt.After(current) {
			p.accounts[accountID] = copyRecord.ExpiresAt
		}
		if key, ok := newOpenAICodexTurnStateSessionKey(accountID, copyRecord.SourceSessionHash); ok && copyRecord.ValueLength == openAICodexPreferredTurnStateLength {
			current := p.preferredBySession[key]
			if !p.isPreferredSessionRecordLocked(current, key, p.now()) || openAICodexTurnStateNewestFirst(copyRecord, current) {
				p.preferredBySession[key] = copyRecord
			}
		}
	}
	if p.selected == nil {
		p.selected = copyRecord
	} else if !p.selected.ExpiresAt.After(p.now()) {
		p.selectLongestLocked(p.now())
	} else if openAICodexTurnStateRanksBefore(copyRecord, p.selected) {
		p.selected = copyRecord
	}
}

func (p *openAICodexTurnStatePool) rebuildAccountsLocked() {
	p.accounts = make(map[int64]time.Time)
	p.preferredBySession = make(map[openAICodexTurnStateSessionKey]*OpenAICodexTurnStateRecord)
	for _, record := range p.entries {
		if record != nil && record.SourceAccountID != nil && *record.SourceAccountID > 0 {
			accountID := *record.SourceAccountID
			if current := p.accounts[accountID]; record.ExpiresAt.After(current) {
				p.accounts[accountID] = record.ExpiresAt
			}
			if key, ok := newOpenAICodexTurnStateSessionKey(accountID, record.SourceSessionHash); ok && record.ValueLength == openAICodexPreferredTurnStateLength && record.ExpiresAt.After(p.now()) {
				current := p.preferredBySession[key]
				if current == nil || openAICodexTurnStateNewestFirst(record, current) {
					p.preferredBySession[key] = record
				}
			}
		}
	}
}

func newOpenAICodexTurnStateSessionKey(accountID int64, sessionHash string) (openAICodexTurnStateSessionKey, bool) {
	sessionHash = strings.TrimSpace(sessionHash)
	key := openAICodexTurnStateSessionKey{accountID: accountID, sessionHash: sessionHash}
	return key, accountID > 0 && sessionHash != ""
}

func (p *openAICodexTurnStatePool) isPreferredSessionRecordLocked(record *OpenAICodexTurnStateRecord, key openAICodexTurnStateSessionKey, now time.Time) bool {
	if record == nil || record.StateValue == "" || record.ValueLength != openAICodexPreferredTurnStateLength || !record.ExpiresAt.After(now) {
		return false
	}
	if record.SourceAccountID == nil || *record.SourceAccountID != key.accountID || record.SourceSessionHash != key.sessionHash {
		return false
	}
	current := p.entries[record.StateHash]
	return current != nil && current.SourceAccountID != nil && *current.SourceAccountID == key.accountID && current.SourceSessionHash == key.sessionHash && current.StateHash == record.StateHash
}

func (p *openAICodexTurnStatePool) selectPreferredForSessionLocked(key openAICodexTurnStateSessionKey, now time.Time) {
	delete(p.preferredBySession, key)
	for _, record := range p.entries {
		if record == nil || record.ValueLength != openAICodexPreferredTurnStateLength || !record.ExpiresAt.After(now) || record.SourceAccountID == nil || *record.SourceAccountID != key.accountID || record.SourceSessionHash != key.sessionHash {
			continue
		}
		current := p.preferredBySession[key]
		if current == nil || openAICodexTurnStateNewestFirst(record, current) {
			p.preferredBySession[key] = record
		}
	}
}

func (p *openAICodexTurnStatePool) selectLongestLocked(now time.Time) {
	p.selected = nil
	for hash, record := range p.entries {
		if record == nil || !record.ExpiresAt.After(now) {
			delete(p.entries, hash)
			continue
		}
		if p.selected == nil || openAICodexTurnStateRanksBefore(record, p.selected) {
			p.selected = record
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
	p.selectLongestLocked(now)
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
	if !left.LastSeenAt.Equal(right.LastSeenAt) {
		return left.LastSeenAt.After(right.LastSeenAt)
	}
	return left.StateHash > right.StateHash
}

func openAICodexTurnStateNewestFirst(left, right *OpenAICodexTurnStateRecord) bool {
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
