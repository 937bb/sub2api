package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type sessionRevalidationAPIKeyRepo struct {
	APIKeyRepository
	current *APIKey
	err     error
}

func (r *sessionRevalidationAPIKeyRepo) GetByID(context.Context, int64) (*APIKey, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.current == nil {
		return nil, ErrAPIKeyNotFound
	}
	current := *r.current
	if r.current.User != nil {
		user := *r.current.User
		current.User = &user
	}
	if r.current.Group != nil {
		group := *r.current.Group
		current.Group = &group
	}
	return &current, nil
}

func TestAPIKeyServiceRevalidateSessionKey(t *testing.T) {
	groupID := int64(7)
	bound := &APIKey{
		ID:      11,
		UserID:  13,
		Key:     "sk-session-bound",
		GroupID: &groupID,
		Status:  StatusAPIKeyActive,
		User:    &User{ID: 13, Status: StatusActive},
		Group:   &Group{ID: groupID, Status: StatusActive},
	}

	t.Run("active binding remains valid", func(t *testing.T) {
		repo := &sessionRevalidationAPIKeyRepo{current: bound}
		svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, nil)

		got, err := svc.RevalidateSessionKey(context.Background(), bound)

		require.NoError(t, err)
		require.Equal(t, bound.ID, got.ID)
		require.NotSame(t, bound, got)
	})

	t.Run("deleted key fails closed", func(t *testing.T) {
		repo := &sessionRevalidationAPIKeyRepo{err: ErrAPIKeyNotFound}
		svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, nil)

		_, err := svc.RevalidateSessionKey(context.Background(), bound)

		require.Error(t, err)
		require.True(t, errors.Is(err, ErrAPIKeyNotFound))
	})

	t.Run("disabled key fails closed", func(t *testing.T) {
		disabled := *bound
		disabled.Status = StatusAPIKeyDisabled
		repo := &sessionRevalidationAPIKeyRepo{current: &disabled}
		svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, nil)

		_, err := svc.RevalidateSessionKey(context.Background(), bound)

		require.ErrorIs(t, err, ErrAPIKeySessionInvalid)
	})

	t.Run("identity rebound fails closed", func(t *testing.T) {
		rebound := *bound
		rebound.UserID = 99
		rebound.User = &User{ID: 99, Status: StatusActive}
		repo := &sessionRevalidationAPIKeyRepo{current: &rebound}
		svc := NewAPIKeyService(repo, nil, nil, nil, nil, nil, nil)

		_, err := svc.RevalidateSessionKey(context.Background(), bound)

		require.ErrorIs(t, err, ErrAPIKeySessionInvalid)
	})

	t.Run("missing repository fails closed", func(t *testing.T) {
		svc := &APIKeyService{}

		_, err := svc.RevalidateSessionKey(context.Background(), bound)

		require.ErrorIs(t, err, ErrAPIKeySessionInvalid)
	})
}
