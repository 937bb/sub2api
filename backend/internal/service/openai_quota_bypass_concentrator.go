package service

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	openAIQuotaBypassMinWindowSize = 8
	// Production routing uses a single active target. A larger window is only
	// useful for diagnostics and must not be exposed as parallel candidates.
	openAIQuotaBypassActiveWindowSize = 1
	openAIQuotaBypassFullHintTTL      = 250 * time.Millisecond
	openAIQuotaBypassPoolStateTTL     = 15 * time.Minute
)

type openAIQuotaBypassPoolKey struct {
	groupID   int64
	ungrouped bool
	platform  string
	priority  int
}

func (k openAIQuotaBypassPoolKey) sharedKey() string {
	if k.ungrouped {
		return fmt.Sprintf("ungrouped:%s:%d", k.platform, k.priority)
	}
	return fmt.Sprintf("group:%d:%s:%d", k.groupID, k.platform, k.priority)
}

type openAIQuotaBypassPoolState struct {
	mu        sync.Mutex
	cursorID  int64
	fullUntil map[int64]int64
	lastUsed  atomic.Int64
}

// openAIQuotaBypassConcentrator keeps only a small routing hint per logical
// pool. Redis slot acquisition remains the source of truth, so concurrent
// gateway instances can safely race on the same concentrated account.
type openAIQuotaBypassConcentrator struct {
	pools      sync.Map // map[openAIQuotaBypassPoolKey]*openAIQuotaBypassPoolState
	operations atomic.Uint64
}

type openAIQuotaBypassCursorWindow struct {
	accountIDs []int64
	hasMore    bool
}

func newOpenAIQuotaBypassConcentrator() *openAIQuotaBypassConcentrator {
	return &openAIQuotaBypassConcentrator{}
}

func openAIQuotaBypassWindowSize(topK int) int {
	if topK <= 0 {
		topK = openAIQuotaBypassActiveWindowSize
	}
	if topK > openAIAccountSelectionProbeLimit {
		return openAIAccountSelectionProbeLimit
	}
	return topK
}

func newOpenAIQuotaBypassPoolKey(groupID *int64, platform string, priority int) openAIQuotaBypassPoolKey {
	key := openAIQuotaBypassPoolKey{
		ungrouped: groupID == nil,
		platform:  NormalizeOpenAICompatiblePlatform(platform),
		priority:  priority,
	}
	if groupID != nil {
		key.groupID = *groupID
	}
	return key
}

func (c *openAIQuotaBypassConcentrator) state(key openAIQuotaBypassPoolKey) *openAIQuotaBypassPoolState {
	if c == nil {
		return nil
	}
	created := &openAIQuotaBypassPoolState{}
	actual, _ := c.pools.LoadOrStore(key, created)
	state, _ := actual.(*openAIQuotaBypassPoolState)
	if state == nil {
		state = created
		c.pools.Store(key, state)
	}
	state.lastUsed.Store(time.Now().UnixNano())
	if c.operations.Add(1)%1024 == 0 {
		c.cleanup(time.Now())
	}
	return state
}

// window returns at most limit account IDs in circular ID order starting at
// the active cursor. It scans candidate metadata in memory but never reads the
// whole pool's Redis load, keeping the expensive part bounded by the window.
func (c *openAIQuotaBypassConcentrator) window(
	key openAIQuotaBypassPoolKey,
	accountIDs []int64,
	limit int,
) openAIQuotaBypassCursorWindow {
	if len(accountIDs) == 0 {
		return openAIQuotaBypassCursorWindow{}
	}
	if limit <= 0 {
		limit = openAIQuotaBypassMinWindowSize
	}
	if limit > len(accountIDs) {
		limit = len(accountIDs)
	}

	state := c.state(key)
	if state == nil {
		return openAIQuotaBypassCursorWindow{
			accountIDs: append([]int64(nil), accountIDs[:limit]...),
			hasMore:    len(accountIDs) > limit,
		}
	}

	nowUnixNano := time.Now().UnixNano()
	state.mu.Lock()
	defer state.mu.Unlock()

	for accountID, until := range state.fullUntil {
		if until <= nowUnixNano {
			delete(state.fullUntil, accountID)
		}
	}

	selected := make([]int64, 0, limit)
	eligibleCount := 0
	var earliestFullID int64
	earliestFullUntil := int64(0)
	for _, accountID := range accountIDs {
		if accountID <= 0 {
			continue
		}
		if until := state.fullUntil[accountID]; until > nowUnixNano {
			if earliestFullID == 0 || until < earliestFullUntil || (until == earliestFullUntil && accountID < earliestFullID) {
				earliestFullID = accountID
				earliestFullUntil = until
			}
			continue
		}
		eligibleCount++
		insertOpenAIQuotaBypassCursorID(&selected, accountID, state.cursorID, limit)
	}
	if len(selected) > 0 {
		return openAIQuotaBypassCursorWindow{
			accountIDs: selected,
			hasMore:    eligibleCount > len(selected),
		}
	}
	// All candidates were recently observed full. Keep one waitable target in
	// the order so callers can return a normal WaitPlan instead of a false
	// no-account error; the atomic acquire will also catch an early release.
	if earliestFullID > 0 {
		return openAIQuotaBypassCursorWindow{accountIDs: []int64{earliestFullID}}
	}
	return openAIQuotaBypassCursorWindow{}
}

func insertOpenAIQuotaBypassCursorID(selected *[]int64, accountID int64, cursorID int64, limit int) {
	values := *selected
	position := sort.Search(len(values), func(i int) bool {
		return openAIQuotaBypassCursorIDBefore(accountID, values[i], cursorID)
	})
	if len(values) >= limit && position >= limit {
		return
	}
	if len(values) < limit {
		values = append(values, 0)
	} else {
		values = values[:limit]
	}
	copy(values[position+1:], values[position:len(values)-1])
	values[position] = accountID
	*selected = values
}

func openAIQuotaBypassCursorIDBefore(left int64, right int64, cursorID int64) bool {
	if cursorID <= 0 {
		return left < right
	}
	leftWrapped := left < cursorID
	rightWrapped := right < cursorID
	if leftWrapped != rightWrapped {
		return !leftWrapped
	}
	return left < right
}

func (c *openAIQuotaBypassConcentrator) markFull(key openAIQuotaBypassPoolKey, accountID int64) {
	if c == nil || accountID <= 0 {
		return
	}
	state := c.state(key)
	state.mu.Lock()
	if state.fullUntil == nil {
		state.fullUntil = make(map[int64]int64)
	}
	state.fullUntil[accountID] = time.Now().Add(openAIQuotaBypassFullHintTTL).UnixNano()
	if state.cursorID == 0 {
		state.cursorID = accountID
	}
	state.mu.Unlock()
}

func (c *openAIQuotaBypassConcentrator) markAcquired(key openAIQuotaBypassPoolKey, accountID int64) {
	if c == nil || accountID <= 0 {
		return
	}
	state := c.state(key)
	state.mu.Lock()
	state.cursorID = accountID
	delete(state.fullUntil, accountID)
	state.mu.Unlock()
}

func (c *openAIQuotaBypassConcentrator) markReleased(key openAIQuotaBypassPoolKey, accountID int64) {
	if c == nil || accountID <= 0 {
		return
	}
	state := c.state(key)
	state.mu.Lock()
	delete(state.fullUntil, accountID)
	// Release only clears the short full hint. New-session traffic stays on the
	// current cursor until that account reaches its soft limit.
	state.mu.Unlock()
}

func (c *openAIQuotaBypassConcentrator) isFullHinted(key openAIQuotaBypassPoolKey, accountID int64) bool {
	if c == nil || accountID <= 0 {
		return false
	}
	state := c.state(key)
	nowUnixNano := time.Now().UnixNano()
	state.mu.Lock()
	defer state.mu.Unlock()
	until := state.fullUntil[accountID]
	if until <= nowUnixNano {
		delete(state.fullUntil, accountID)
		return false
	}
	return true
}

func (c *openAIQuotaBypassConcentrator) cleanup(now time.Time) {
	if c == nil {
		return
	}
	cutoff := now.Add(-openAIQuotaBypassPoolStateTTL).UnixNano()
	c.pools.Range(func(key any, value any) bool {
		state, _ := value.(*openAIQuotaBypassPoolState)
		if state != nil && state.lastUsed.Load() < cutoff {
			c.pools.Delete(key)
		}
		return true
	})
}

func (s *OpenAIGatewayService) quotaBypassConcentrator() *openAIQuotaBypassConcentrator {
	if s == nil {
		return nil
	}
	s.openaiQuotaBypassConcentratorOnce.Do(func() {
		s.openaiQuotaBypassConcentrator = newOpenAIQuotaBypassConcentrator()
	})
	return s.openaiQuotaBypassConcentrator
}

func (s *OpenAIGatewayService) quotaBypassSelectionWindow(
	groupID *int64,
	platform string,
	priority int,
	accountIDs []int64,
	topK int,
) openAIQuotaBypassCursorWindow {
	concentrator := s.quotaBypassConcentrator()
	if concentrator == nil {
		return openAIQuotaBypassCursorWindow{}
	}
	key := newOpenAIQuotaBypassPoolKey(groupID, platform, priority)
	window := concentrator.window(key, accountIDs, openAIQuotaBypassWindowSize(topK))
	if len(window.accountIDs) == 0 || s.concurrencyService == nil {
		return window
	}

	sharedKey := key.sharedKey()
	sharedID, sharedOK := s.concurrencyService.getQuotaBypassActiveAccount(sharedKey)
	if sharedOK &&
		containsOpenAIQuotaBypassAccountID(accountIDs, sharedID) &&
		!concentrator.isFullHinted(key, sharedID) {
		concentrator.markAcquired(key, sharedID)
		return openAIQuotaBypassCursorWindow{
			accountIDs: []int64{sharedID},
			hasMore:    len(accountIDs) > 1,
		}
	}

	activeID := window.accountIDs[0]
	if sharedOK {
		if currentID, ok := s.concurrencyService.advanceQuotaBypassActiveAccount(
			sharedKey,
			sharedID,
			activeID,
			openAIQuotaBypassPoolStateTTL,
		); ok && containsOpenAIQuotaBypassAccountID(accountIDs, currentID) && !concentrator.isFullHinted(key, currentID) {
			activeID = currentID
		}
	}
	concentrator.markAcquired(key, activeID)
	if !sharedOK {
		s.concurrencyService.setQuotaBypassActiveAccount(sharedKey, activeID, openAIQuotaBypassPoolStateTTL)
	}
	return openAIQuotaBypassCursorWindow{
		accountIDs: []int64{activeID},
		hasMore:    len(accountIDs) > 1,
	}
}

func containsOpenAIQuotaBypassAccountID(accountIDs []int64, accountID int64) bool {
	for _, candidateID := range accountIDs {
		if candidateID == accountID {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) markQuotaBypassAccountFull(groupID *int64, platform string, account *Account) {
	if s == nil || account == nil {
		return
	}
	key := newOpenAIQuotaBypassPoolKey(groupID, platform, openAIAccountSchedulingPriority(account))
	s.quotaBypassConcentrator().markFull(key, account.ID)
}

// quotaBypassConcentratedMaxConcurrency protects first-token latency without
// changing the account's hard concurrency setting. It is only used after the
// request has resolved this account as a concentrated quota-bypass candidate.
func (s *OpenAIGatewayService) quotaBypassConcentratedMaxConcurrency(account *Account) int {
	if account == nil {
		return 0
	}
	maxConcurrency := account.Concurrency
	softLimit := 0
	if s != nil && s.cfg != nil {
		softLimit = s.cfg.Gateway.OpenAIScheduler.QuotaBypassSoftConcurrency
	}
	if maxConcurrency <= 0 {
		if softLimit > 0 {
			return softLimit
		}
		return maxConcurrency
	}
	if maxConcurrency == 1 {
		return 1
	}
	if softLimit > 0 && (maxConcurrency <= 0 || softLimit < maxConcurrency) {
		return softLimit
	}
	if softLimit > 0 {
		return maxConcurrency
	}

	// With no explicit soft limit, reserve 15% of hard capacity for existing
	// sticky sessions. ceil(3*n/20) is calculated without floating point.
	reserve := (maxConcurrency/20)*3 + ((maxConcurrency%20)*3+19)/20
	automaticSoftLimit := maxConcurrency - reserve
	if automaticSoftLimit < 1 {
		return 1
	}
	return automaticSoftLimit
}

func (s *OpenAIGatewayService) openAISelectionMaxConcurrency(account *Account, concentrated bool) int {
	if concentrated {
		return s.quotaBypassConcentratedMaxConcurrency(account)
	}
	if account == nil {
		return 0
	}
	return account.Concurrency
}

// OpenAISelectionMaxConcurrency exposes the resolved slot limit to handlers
// that reacquire later turns on an already selected WebSocket account.
func (s *OpenAIGatewayService) OpenAISelectionMaxConcurrency(account *Account, concentrated bool) int {
	return s.openAISelectionMaxConcurrency(account, concentrated)
}

func (s *OpenAIGatewayService) wrapQuotaBypassAccountRelease(
	groupID *int64,
	platform string,
	account *Account,
	release func(),
) func() {
	if s == nil || account == nil {
		return release
	}
	key := newOpenAIQuotaBypassPoolKey(groupID, platform, openAIAccountSchedulingPriority(account))
	concentrator := s.quotaBypassConcentrator()
	concentrator.markAcquired(key, account.ID)
	var once sync.Once
	return func() {
		once.Do(func() {
			if release != nil {
				release()
			}
			concentrator.markReleased(key, account.ID)
		})
	}
}

// WrapOpenAIQuotaBypassConcentratedRelease applies the same cursor update to
// slots acquired from a handler WaitPlan as slots acquired atomically inside
// the scheduler.
func (s *OpenAIGatewayService) WrapOpenAIQuotaBypassConcentratedRelease(
	groupID *int64,
	platform string,
	account *Account,
	release func(),
) func() {
	if !IsAccountQuotaBypassConcentrated(account) {
		return release
	}
	return s.wrapQuotaBypassAccountRelease(groupID, platform, account, release)
}
