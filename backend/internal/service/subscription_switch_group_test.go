package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type switchSubscriptionRepoStub struct {
	userSubRepoNoop
	testT        *testing.T
	sub          UserSubscription
	targetExists bool
	lockErr      error
	updateErr    error
	updateCalls  int
}

func (r *switchSubscriptionRepoStub) GetByIDForUpdate(context.Context, int64) (*UserSubscription, error) {
	if r.lockErr != nil {
		return nil, r.lockErr
	}
	cp := r.sub
	return &cp, nil
}

func (r *switchSubscriptionRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	cp := r.sub
	return &cp, nil
}

func (r *switchSubscriptionRepoStub) ExistsByUserIDAndGroupID(_ context.Context, userID, groupID int64) (bool, error) {
	require.Equal(r.testT, r.sub.UserID, userID)
	require.NotEqual(r.testT, r.sub.GroupID, groupID)
	return r.targetExists, nil
}

func (r *switchSubscriptionRepoStub) Update(_ context.Context, sub *UserSubscription) error {
	r.updateCalls++
	if r.updateErr != nil {
		return r.updateErr
	}
	r.sub = *sub
	return nil
}

func newSwitchSubscriptionRepoStub(t *testing.T) *switchSubscriptionRepoStub {
	now := time.Now().UTC().Truncate(time.Second)
	repo := &switchSubscriptionRepoStub{
		sub: UserSubscription{
			ID:              7,
			UserID:          11,
			GroupID:         13,
			Status:          SubscriptionStatusActive,
			StartsAt:        now.Add(-24 * time.Hour),
			ExpiresAt:       now.Add(30 * 24 * time.Hour),
			DailyUsageUSD:   1.25,
			WeeklyUsageUSD:  2.5,
			MonthlyUsageUSD: 3.75,
		},
	}
	repo.testT = t
	return repo
}

func TestSwitchSubscriptionGroupPreservesTermAndUsage(t *testing.T) {
	repo := newSwitchSubscriptionRepoStub(t)
	before := repo.sub
	target := &Group{ID: 17, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: target}, repo, nil, nil, nil)

	got, err := svc.SwitchSubscriptionGroup(context.Background(), before.ID, target.ID)
	require.NoError(t, err)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, target.ID, got.GroupID)
	require.Equal(t, before.StartsAt, got.StartsAt)
	require.Equal(t, before.ExpiresAt, got.ExpiresAt)
	require.Equal(t, before.DailyUsageUSD, got.DailyUsageUSD)
	require.Equal(t, before.WeeklyUsageUSD, got.WeeklyUsageUSD)
	require.Equal(t, before.MonthlyUsageUSD, got.MonthlyUsageUSD)
}

func TestSwitchSubscriptionGroupRejectsExistingTargetSubscription(t *testing.T) {
	repo := newSwitchSubscriptionRepoStub(t)
	repo.targetExists = true
	target := &Group{ID: 17, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: target}, repo, nil, nil, nil)

	_, err := svc.SwitchSubscriptionGroup(context.Background(), repo.sub.ID, target.ID)
	require.ErrorIs(t, err, ErrSubscriptionSwitchConflict)
	require.Zero(t, repo.updateCalls)
}

func TestSwitchSubscriptionGroupMapsUniqueRaceToConflict(t *testing.T) {
	repo := newSwitchSubscriptionRepoStub(t)
	repo.updateErr = ErrSubscriptionAlreadyExists
	target := &Group{ID: 17, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: target}, repo, nil, nil, nil)

	_, err := svc.SwitchSubscriptionGroup(context.Background(), repo.sub.ID, target.ID)
	require.True(t, errors.Is(err, ErrSubscriptionSwitchConflict))
}

func TestSwitchSubscriptionGroupIsIdempotentForSameGroup(t *testing.T) {
	repo := newSwitchSubscriptionRepoStub(t)
	target := &Group{ID: repo.sub.GroupID, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: target}, repo, nil, nil, nil)

	got, err := svc.SwitchSubscriptionGroup(context.Background(), repo.sub.ID, target.ID)
	require.NoError(t, err)
	require.Equal(t, target.ID, got.GroupID)
	require.Zero(t, repo.updateCalls)
}

func TestSwitchSubscriptionGroupPropagatesLockFailure(t *testing.T) {
	repo := newSwitchSubscriptionRepoStub(t)
	repo.lockErr = errors.New("database unavailable")
	target := &Group{ID: 17, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: target}, repo, nil, nil, nil)

	_, err := svc.SwitchSubscriptionGroup(context.Background(), repo.sub.ID, target.ID)
	require.EqualError(t, err, "database unavailable")
	require.Zero(t, repo.updateCalls)
}

func TestSwitchSubscriptionGroupRejectsNonSubscriptionGroup(t *testing.T) {
	repo := newSwitchSubscriptionRepoStub(t)
	target := &Group{ID: 17, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: target}, repo, nil, nil, nil)

	_, err := svc.SwitchSubscriptionGroup(context.Background(), repo.sub.ID, target.ID)
	require.ErrorIs(t, err, ErrGroupNotSubscriptionType)
	require.Zero(t, repo.updateCalls)
}

func TestSwitchSubscriptionGroupReturnsNotFound(t *testing.T) {
	repo := newSwitchSubscriptionRepoStub(t)
	repo.lockErr = ErrSubscriptionNotFound
	target := &Group{ID: 17, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: target}, repo, nil, nil, nil)

	_, err := svc.SwitchSubscriptionGroup(context.Background(), repo.sub.ID, target.ID)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
}
