package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type fakeGroupRepo struct {
	group *service.Group
}

func (f *fakeGroupRepo) Create(context.Context, *service.Group) error { return nil }
func (f *fakeGroupRepo) GetByID(context.Context, int64) (*service.Group, error) {
	return f.group, nil
}
func (f *fakeGroupRepo) GetByIDLite(context.Context, int64) (*service.Group, error) {
	return f.group, nil
}
func (f *fakeGroupRepo) Update(context.Context, *service.Group) error          { return nil }
func (f *fakeGroupRepo) Delete(context.Context, int64) error                   { return nil }
func (f *fakeGroupRepo) DeleteCascade(context.Context, int64) ([]int64, error) { return nil, nil }
func (f *fakeGroupRepo) List(context.Context, pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (f *fakeGroupRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (f *fakeGroupRepo) ListActive(context.Context) ([]service.Group, error) { return nil, nil }
func (f *fakeGroupRepo) ListActiveByPlatform(context.Context, string) ([]service.Group, error) {
	return nil, nil
}
func (f *fakeGroupRepo) ListAllIncludingInactive(context.Context, string) ([]service.Group, error) {
	return nil, nil
}
func (f *fakeGroupRepo) ExistsByName(context.Context, string) (bool, error) { return false, nil }
func (f *fakeGroupRepo) GetAccountCount(context.Context, int64) (int64, int64, error) {
	return 0, 0, nil
}
func (f *fakeGroupRepo) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (f *fakeGroupRepo) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	return nil, nil
}
func (f *fakeGroupRepo) BindAccountsToGroup(context.Context, int64, []int64) error { return nil }
func (f *fakeGroupRepo) UpdateSortOrders(context.Context, []service.GroupSortOrderUpdate) error {
	return nil
}
