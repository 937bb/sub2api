package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupFromServicePreservesQuotaBypassConcentratedScheduling(t *testing.T) {
	group := &service.Group{
		ID:                                       42,
		Name:                                     "openai-bypass-concentrated",
		Platform:                                 service.PlatformOpenAI,
		QuotaBypassEnabled:                       true,
		QuotaBypassConcentratedSchedulingEnabled: true,
	}

	for name, mapped := range map[string]*Group{
		"public":  GroupFromService(group),
		"shallow": GroupFromServiceShallow(group),
	} {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, mapped)
			require.True(t, mapped.QuotaBypassEnabled)
			require.True(t, mapped.QuotaBypassConcentratedSchedulingEnabled)
		})
	}

	admin := GroupFromServiceAdmin(group)
	require.NotNil(t, admin)
	require.True(t, admin.QuotaBypassEnabled)
	require.True(t, admin.QuotaBypassConcentratedSchedulingEnabled)
}
