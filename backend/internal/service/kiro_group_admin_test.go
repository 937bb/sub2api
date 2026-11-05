//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKiroGroupPolicyUpdateAndValidation(t *testing.T) {
	group := &Group{ID: 7, Platform: PlatformAnthropic, RateMultiplier: 1}
	repo := &groupRepoStubForAdmin{getByID: group}
	svc := &adminServiceImpl{groupRepo: repo}
	policy := KiroCachePolicy{Enabled: true, TargetPercent: 42, TTLSeconds: 120, MaxEntries: 16}
	_, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{KiroResponseCache: &policy})
	require.NoError(t, err)
	require.Equal(t, policy, repo.updated.KiroResponseCache)
	_, err = svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{})
	require.NoError(t, err)
	require.Equal(t, policy, repo.updated.KiroResponseCache)
	for _, bad := range []KiroCachePolicy{{TargetPercent: 101, TTLSeconds: 1, MaxEntries: 1}, {TTLSeconds: 0, MaxEntries: 1}, {TTLSeconds: 1, MaxEntries: 1025}} {
		_, err = svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{KiroResponseCache: &bad})
		require.Error(t, err)
	}
	_, err = svc.CreateGroup(context.Background(), &CreateGroupInput{Name: "bad", Platform: PlatformKiro, RateMultiplier: 1})
	require.Error(t, err)
}

func TestKiroMixedGroupSchedulerFindsKiroAccount(t *testing.T) {
	a := fixedKiroAccount()
	a.Status = StatusActive
	a.Schedulable = true
	a.GroupIDs = []int64{7}
	repo := &mockAccountRepoForPlatform{accounts: []Account{*a}}
	groupID := int64(7)
	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformOpenAI} {
		svc := &GatewayService{accountRepo: repo}
		accounts, mixed, err := svc.listSchedulableAccounts(context.Background(), &groupID, platform, false)
		require.NoError(t, err)
		require.True(t, mixed)
		require.Len(t, accounts, 1)
		require.Equal(t, PlatformKiro, accounts[0].Platform)
		snapshot := NewSchedulerSnapshotService(nil, nil, repo, nil, nil)
		accounts, err = snapshot.loadAccountsFromDB(context.Background(), SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeMixed}, true)
		require.NoError(t, err)
		require.Len(t, accounts, 1)
		accounts, err = snapshot.loadAccountsFromDB(context.Background(), SchedulerBucket{GroupID: groupID, Platform: platform, Mode: SchedulerModeForced}, false)
		require.NoError(t, err)
		require.Empty(t, accounts)
	}
	openai := &OpenAIGatewayService{accountRepo: repo}
	accounts, err := openai.listSchedulableAccounts(context.Background(), &groupID, PlatformOpenAI)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
}
