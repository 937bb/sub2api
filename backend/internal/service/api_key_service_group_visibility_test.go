//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type apiKeyGroupVisibilityRepoStub struct {
	groupRepoNoop
	activeGroups []Group
}

func (s *apiKeyGroupVisibilityRepoStub) ListActive(context.Context) ([]Group, error) {
	out := make([]Group, len(s.activeGroups))
	copy(out, s.activeGroups)
	return out, nil
}

type apiKeyGroupVisibilitySubRepoStub struct {
	userSubRepoNoop
	activeSubs []UserSubscription
}

func (s *apiKeyGroupVisibilitySubRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	out := make([]UserSubscription, len(s.activeSubs))
	copy(out, s.activeSubs)
	return out, nil
}

func TestAPIKeyService_GetAvailableGroups_PublicGroupsRemainBindableWithWhitelist(t *testing.T) {
	svc := NewAPIKeyService(
		nil,
		&mockUserRepo{getByIDUser: &User{ID: 1, AllowedGroups: []int64{2}}},
		&apiKeyGroupVisibilityRepoStub{activeGroups: []Group{
			{ID: 1, Name: "public", IsExclusive: false, SubscriptionType: SubscriptionTypeStandard},
			{ID: 2, Name: "exclusive-allowed", IsExclusive: true, SubscriptionType: SubscriptionTypeStandard},
			{ID: 3, Name: "exclusive-hidden", IsExclusive: true, SubscriptionType: SubscriptionTypeStandard},
			{ID: 4, Name: "subscribed", IsExclusive: false, SubscriptionType: SubscriptionTypeSubscription},
		}},
		&apiKeyGroupVisibilitySubRepoStub{activeSubs: []UserSubscription{{UserID: 1, GroupID: 4}}},
		nil,
		nil,
		&config.Config{},
	)

	groups, err := svc.GetAvailableGroups(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 4}, groupIDs(groups))
}

func TestAPIKeyService_GetVisibleGroups_WhitelistHidesUnlistedPublicGroups(t *testing.T) {
	svc := NewAPIKeyService(
		nil,
		&mockUserRepo{getByIDUser: &User{ID: 1, AllowedGroups: []int64{2}}},
		&apiKeyGroupVisibilityRepoStub{activeGroups: []Group{
			{ID: 1, Name: "public", IsExclusive: false, SubscriptionType: SubscriptionTypeStandard},
			{ID: 2, Name: "exclusive-allowed", IsExclusive: true, SubscriptionType: SubscriptionTypeStandard},
			{ID: 4, Name: "subscribed", IsExclusive: false, SubscriptionType: SubscriptionTypeSubscription},
		}},
		&apiKeyGroupVisibilitySubRepoStub{activeSubs: []UserSubscription{{UserID: 1, GroupID: 4}}},
		nil,
		nil,
		&config.Config{},
	)

	groups, err := svc.GetVisibleGroups(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, []int64{2, 4}, groupIDs(groups))
}

func TestAPIKeyService_GetVisibleGroups_WithoutWhitelistShowsPublicGroupsOnly(t *testing.T) {
	svc := NewAPIKeyService(
		nil,
		&mockUserRepo{getByIDUser: &User{ID: 1}},
		&apiKeyGroupVisibilityRepoStub{activeGroups: []Group{
			{ID: 1, Name: "public", IsExclusive: false, SubscriptionType: SubscriptionTypeStandard},
			{ID: 2, Name: "exclusive", IsExclusive: true, SubscriptionType: SubscriptionTypeStandard},
			{ID: 4, Name: "subscribed", IsExclusive: false, SubscriptionType: SubscriptionTypeSubscription},
		}},
		&apiKeyGroupVisibilitySubRepoStub{activeSubs: []UserSubscription{{UserID: 1, GroupID: 4}}},
		nil,
		nil,
		&config.Config{},
	)

	groups, err := svc.GetVisibleGroups(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 4}, groupIDs(groups))
}

func groupIDs(groups []Group) []int64 {
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return ids
}
