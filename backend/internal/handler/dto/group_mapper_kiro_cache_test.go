package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupMapperExposesKiroCachePolicyOnlyToAdmins(t *testing.T) {
	policy := domain.KiroResponseCachePolicy{Enabled: true, TargetPercent: 35, TTLSeconds: 120, MaxEntries: 64}
	group := &service.Group{ID: 7, KiroResponseCache: policy}
	userJSON, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "kiro_response_cache")
	admin := GroupFromServiceAdmin(group)
	require.Equal(t, policy, admin.KiroResponseCache)
	adminJSON, err := json.Marshal(admin)
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), `"kiro_response_cache":{"enabled":true,"target_percent":35,"ttl_seconds":120,"max_entries":64}`)
}
