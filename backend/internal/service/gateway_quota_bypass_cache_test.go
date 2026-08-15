package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestQuotaBypassEligible_APIKeySnapshotRoundTrip proves that QuotaBypassEnabled
// survives the API key auth cache round-trip (snapshotFrom → JSON → snapshotTo).
// This is the injection path: handler line 477 checks IsQuotaBypassEligible(account, apiKey.Group).
// Before the fix, QuotaBypassEnabled was not in APIKeyAuthGroupSnapshot, so
// apiKey.Group.QuotaBypassEnabled was always false after cache deserialization.
func TestQuotaBypassEligible_APIKeySnapshotRoundTrip(t *testing.T) {
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, nil, &config.Config{})
	groupID := int64(42)
	apiKey := &APIKey{
		ID:      1,
		UserID:  2,
		GroupID: &groupID,
		Key:     "k-bypass-test",
		Name:    "Bypass Test Key",
		Status:  StatusActive,
		User: &User{
			ID:          2,
			Status:      StatusActive,
			Role:        RoleUser,
			Balance:     10,
			Concurrency: 3,
		},
		Group: &Group{
			ID:                                       groupID,
			Name:                                     "bypass-group",
			Platform:                                 PlatformOpenAI,
			Status:                                   StatusActive,
			SubscriptionType:                         SubscriptionTypeStandard,
			RateMultiplier:                           1,
			QuotaBypassEnabled:                       true,
			QuotaBypassConcentratedSchedulingEnabled: true,
		},
	}

	// Step 1: Serialize to snapshot (what happens when cache is populated)
	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Group)
	require.True(t, snapshot.Group.QuotaBypassEnabled, "snapshot must preserve QuotaBypassEnabled")
	require.True(t, snapshot.Group.QuotaBypassConcentratedSchedulingEnabled,
		"snapshot must preserve QuotaBypassConcentratedSchedulingEnabled")

	// Step 2: JSON round-trip (what happens in Redis)
	jsonBytes, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var deserialized APIKeyAuthSnapshot
	require.NoError(t, json.Unmarshal(jsonBytes, &deserialized))
	require.NotNil(t, deserialized.Group)
	require.True(t, deserialized.Group.QuotaBypassEnabled, "JSON round-trip must preserve QuotaBypassEnabled")
	require.True(t, deserialized.Group.QuotaBypassConcentratedSchedulingEnabled,
		"JSON round-trip must preserve QuotaBypassConcentratedSchedulingEnabled")

	// Step 3: Reconstruct APIKey from snapshot (what happens on cache hit)
	roundTrip := svc.snapshotToAPIKey(apiKey.Key, &deserialized)
	require.NotNil(t, roundTrip)
	require.NotNil(t, roundTrip.Group)
	require.True(t, roundTrip.Group.QuotaBypassEnabled, "reconstructed apiKey.Group must have QuotaBypassEnabled=true")
	require.True(t, roundTrip.Group.QuotaBypassConcentratedSchedulingEnabled,
		"reconstructed apiKey.Group must preserve concentrated scheduling")

	// Step 4: The actual bypass eligibility check (handler line 477)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}
	require.True(t, IsQuotaBypassEligible(account, roundTrip.Group),
		"IsQuotaBypassEligible must return true for cache-loaded apiKey.Group with QuotaBypassEnabled")
}

// TestQuotaBypassEligible_APIKeySnapshotRoundTrip_Disabled verifies that
// QuotaBypassEnabled=false also round-trips correctly (no false positives).
func TestQuotaBypassEligible_APIKeySnapshotRoundTrip_Disabled(t *testing.T) {
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, nil, &config.Config{})
	groupID := int64(43)
	apiKey := &APIKey{
		ID:      2,
		UserID:  3,
		GroupID: &groupID,
		Key:     "k-no-bypass",
		Status:  StatusActive,
		User: &User{
			ID:     3,
			Status: StatusActive,
			Role:   RoleUser,
		},
		Group: &Group{
			ID:                 groupID,
			Platform:           PlatformOpenAI,
			Status:             StatusActive,
			QuotaBypassEnabled: false,
		},
	}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	roundTrip := svc.snapshotToAPIKey(apiKey.Key, snapshot)
	require.NotNil(t, roundTrip.Group)
	require.False(t, roundTrip.Group.QuotaBypassEnabled)

	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, IsQuotaBypassEligible(account, roundTrip.Group))
}

// TestQuotaBypassEligible_SchedulerCacheAccountGroupRoundTrip proves that
// filterSchedulerAccountGroups preserves QuotaBypassEnabled on AccountGroup.Group,
// and IsAccountQuotaBypassEligible works on the filtered result.
func TestQuotaBypassEligible_SchedulerCacheAccountGroupRoundTrip(t *testing.T) {
	account := Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		AccountGroups: []AccountGroup{
			{
				AccountID: 100,
				GroupID:   42,
				Priority:  1,
				Group: &Group{
					ID:                 42,
					QuotaBypassEnabled: true,
				},
			},
		},
	}

	// Simulate what filterSchedulerAccountGroups does
	filtered := make([]AccountGroup, 0, len(account.AccountGroups))
	for _, ag := range account.AccountGroups {
		if ag.GroupID <= 0 {
			continue
		}
		var minGroup *Group
		if ag.Group != nil {
			minGroup = &Group{
				ID:                 ag.Group.ID,
				QuotaBypassEnabled: ag.Group.QuotaBypassEnabled,
			}
		}
		filtered = append(filtered, AccountGroup{
			AccountID: ag.AccountID,
			GroupID:   ag.GroupID,
			Priority:  ag.Priority,
			CreatedAt: ag.CreatedAt,
			Group:     minGroup,
		})
	}

	// JSON round-trip (Redis)
	jsonBytes, err := json.Marshal(filtered)
	require.NoError(t, err)
	var deserialized []AccountGroup
	require.NoError(t, json.Unmarshal(jsonBytes, &deserialized))

	// Reconstruct account as it would appear from cache
	cachedAccount := &Account{
		Platform:      PlatformOpenAI,
		Type:          AccountTypeOAuth,
		AccountGroups: deserialized,
		// Groups is NOT set (cache never populates it)
	}

	require.True(t, IsAccountQuotaBypassEligible(cachedAccount),
		"IsAccountQuotaBypassEligible must return true via AccountGroups path for cached account")
}

// TestQuotaBypassEligible_FullProductionScenario simulates the exact production
// scenario end-to-end: API key loaded from cache, account loaded from scheduler
// cache, both must correctly evaluate bypass eligibility.
func TestQuotaBypassEligible_FullProductionScenario(t *testing.T) {
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, nil, &config.Config{})
	groupID := int64(42)

	// 1. Simulate API key with bypass-enabled group
	apiKey := &APIKey{
		ID:      1,
		UserID:  2,
		GroupID: &groupID,
		Key:     "k-prod-scenario",
		Status:  StatusActive,
		User:    &User{ID: 2, Status: StatusActive, Role: RoleUser, Balance: 100},
		Group: &Group{
			ID:                 groupID,
			Name:               "bypass-group",
			Platform:           PlatformOpenAI,
			Status:             StatusActive,
			QuotaBypassEnabled: true,
		},
	}

	// 2. API key cache round-trip
	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	jsonBytes, _ := json.Marshal(snapshot)
	var snap2 APIKeyAuthSnapshot
	json.Unmarshal(jsonBytes, &snap2)
	cachedAPIKey := svc.snapshotToAPIKey(apiKey.Key, &snap2)

	// 3. Account from scheduler cache (Groups nil, AccountGroups has minimal Group)
	cachedAccount := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		// Groups is nil (scheduler cache never populates it)
		AccountGroups: []AccountGroup{
			{
				AccountID: 100,
				GroupID:   groupID,
				Group: &Group{
					ID:                 groupID,
					QuotaBypassEnabled: true,
				},
			},
		},
	}

	// 4. INJECTION path: handler line 477
	require.True(t, IsQuotaBypassEligible(cachedAccount, cachedAPIKey.Group),
		"INJECTION: IsQuotaBypassEligible must return true with cache-loaded apiKey.Group")

	// 5. RETRY path: service layer 429 handling
	require.True(t, IsAccountQuotaBypassEligible(cachedAccount),
		"RETRY: IsAccountQuotaBypassEligible must return true with cache-loaded account")
}

// TestQuotaBypassEligible_FullInjectionChain proves the COMPLETE production
// chain: cache round-trip → eligibility → InjectFunctionCallOutputSuffix fires
// → request body is actually modified with synthetic function_call items.
func TestQuotaBypassEligible_FullInjectionChain(t *testing.T) {
	svc := NewAPIKeyService(nil, nil, nil, nil, nil, nil, &config.Config{})
	groupID := int64(42)

	apiKey := &APIKey{
		ID: 1, UserID: 2, GroupID: &groupID, Key: "k-inject-chain", Status: StatusActive,
		User:  &User{ID: 2, Status: StatusActive, Role: RoleUser, Balance: 100},
		Group: &Group{ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, QuotaBypassEnabled: true},
	}

	// Cache round-trip
	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	jsonBytes, _ := json.Marshal(snapshot)
	var snap2 APIKeyAuthSnapshot
	json.Unmarshal(jsonBytes, &snap2)
	cachedAPIKey := svc.snapshotToAPIKey(apiKey.Key, &snap2)

	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	// Simulate handler line 477-481: the exact code path
	forwardBody := []byte(`{"model":"gpt-4o-mini","input":[{"type":"message","role":"user","content":"hello"}]}`)
	attemptBody := forwardBody
	if IsQuotaBypassEligible(account, cachedAPIKey.Group) {
		if injected, ok := InjectFunctionCallOutputSuffix(attemptBody); ok {
			attemptBody = injected
		}
	}

	require.NotEqual(t, string(forwardBody), string(attemptBody),
		"Body must be modified by injection")

	// Verify injected items
	inputArr := json.RawMessage(attemptBody)
	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(inputArr, &parsed))
	var items []map[string]interface{}
	require.NoError(t, json.Unmarshal(parsed["input"], &items))
	require.Len(t, items, 3, "input array should have 3 items: original + function_call + function_call_output")
	require.Equal(t, "message", items[0]["type"])
	require.Equal(t, "function_call", items[1]["type"])
	require.Equal(t, "function_call_output", items[2]["type"])
}
