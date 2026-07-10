import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { AdminUser } from '@/types'
import UsersView from '../UsersView.vue'

const {
  listUsers,
  getAllGroups,
  getAllGroupsIncludingInactive,
  getBatchUsersUsage,
  listEnabledDefinitions,
  getBatchUserAttributes
} = vi.hoisted(() => ({
  listUsers: vi.fn(),
  getAllGroups: vi.fn(),
  getAllGroupsIncludingInactive: vi.fn(),
  getBatchUsersUsage: vi.fn(),
  listEnabledDefinitions: vi.fn(),
  getBatchUserAttributes: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      list: listUsers,
      toggleStatus: vi.fn(),
      delete: vi.fn()
    },
    groups: {
      getAll: getAllGroups,
      getAllIncludingInactive: getAllGroupsIncludingInactive
    },
    dashboard: {
      getBatchUsersUsage
    },
    userAttributes: {
      listEnabledDefinitions,
      getBatchUserAttributes
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const createAdminUser = (overrides: Partial<AdminUser> = {}): AdminUser => ({
  id: 42,
  username: 'scoped-user',
  email: 'scoped@example.com',
  role: 'user',
  balance: 0,
  concurrency: 1,
  status: 'active',
  allowed_groups: [],
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-04-17T00:00:00Z',
  updated_at: '2026-04-17T00:00:00Z',
  notes: '',
  last_active_at: '2026-04-16T02:00:00Z',
  last_used_at: '2026-04-17T02:00:00Z',
  current_concurrency: 0,
  ...overrides
})

const DataTableStub = {
  props: ['columns', 'data'],
  emits: ['sort'],
  template: `
    <div>
      <div data-test="columns">{{ columns.map(col => col.key).join(',') }}</div>
      <div data-test="row-order">{{ data.map(row => row.email).join(',') }}</div>
      <button data-test="sort-last-used" @click="$emit('sort', 'last_used_at', 'desc')">sort</button>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-last_used_at" :value="row.last_used_at" :row="row" />
      </div>
    </div>
  `
}

const PaginationStub = {
  props: ['page', 'total', 'pageSize'],
  emits: ['update:page', 'update:pageSize'],
  template: '<button data-test="page-two" @click="$emit(\'update:page\', 2)">page 2</button>'
}

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['change', 'update:modelValue'],
  template: '<button data-test="select-stub" @click="$emit(\'change\', modelValue, null)">select</button>'
}

const mountUsersView = () => mount(UsersView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: {
        template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
      },
      DataTable: DataTableStub,
      Pagination: PaginationStub,
      ConfirmDialog: true,
      EmptyState: true,
      GroupBadge: true,
      Select: SelectStub,
      UserAttributesConfigModal: true,
      UserConcurrencyCell: true,
      UserCreateModal: true,
      UserEditModal: true,
      UserApiKeysModal: true,
      UserAllowedGroupsModal: true,
      UserBalanceModal: true,
      UserBalanceHistoryModal: true,
      GroupReplaceModal: true,
      Icon: true,
      Teleport: true
    }
  }
})

describe('admin UsersView', () => {
  beforeEach(() => {
    localStorage.clear()

    listUsers.mockReset()
    getAllGroups.mockReset()
    getAllGroupsIncludingInactive.mockReset()
    getBatchUsersUsage.mockReset()
    listEnabledDefinitions.mockReset()
    getBatchUserAttributes.mockReset()

    listUsers.mockResolvedValue({
      items: [createAdminUser()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    getAllGroups.mockResolvedValue([])
    getAllGroupsIncludingInactive.mockResolvedValue([])
    getBatchUsersUsage.mockResolvedValue({ stats: {} })
    listEnabledDefinitions.mockResolvedValue([])
    getBatchUserAttributes.mockResolvedValue({ values: {} })
  })

  it('shows active, used, and created activity columns in order and requests last_used_at sort', async () => {
    const wrapper = mountUsersView()

    await flushPromises()

    const columns = wrapper.get('[data-test="columns"]').text()
    const visibleColumns = columns.split(',')
    expect(visibleColumns.slice(-4, -1)).toEqual(['last_active_at', 'last_used_at', 'created_at'])
    expect(visibleColumns).not.toContain('last_login_at')

    await wrapper.get('[data-test="sort-last-used"]').trigger('click')
    await flushPromises()

    expect(listUsers).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({
        sort_by: 'last_used_at',
        sort_order: 'desc'
      }),
      expect.any(Object)
    )
  })

  it('clears persisted current-page usage sorting before applying server sorting', async () => {
    localStorage.setItem('user-column-settings-version', '3')
    localStorage.setItem(
      'user-hidden-columns',
      JSON.stringify([
        'notes',
        'groups',
        'subscriptions',
        'concurrency',
        'usage_anthropic',
        'usage_openai',
        'usage_gemini',
        'usage_antigravity',
        'balance_platform_quota'
      ])
    )
    localStorage.setItem(
      'admin-users-usage-sort',
      JSON.stringify({ key: 'usage', metric: 'today', order: 'desc' })
    )
    listUsers.mockResolvedValue({
      items: [
        createAdminUser({ id: 1, email: 'server-first@example.com' }),
        createAdminUser({ id: 2, email: 'usage-first@example.com' })
      ],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    getBatchUsersUsage.mockResolvedValue({
      stats: {
        1: { user_id: 1, today_actual_cost: 1, total_actual_cost: 1, by_platform: [] },
        2: { user_id: 2, today_actual_cost: 9, total_actual_cost: 9, by_platform: [] }
      }
    })

    const wrapper = mountUsersView()
    await flushPromises()
    await new Promise(resolve => setTimeout(resolve, 75))
    await flushPromises()

    expect(wrapper.get('[data-test="row-order"]').text()).toBe(
      'usage-first@example.com,server-first@example.com'
    )

    await wrapper.get('[data-test="sort-last-used"]').trigger('click')
    await flushPromises()

    expect(localStorage.getItem('admin-users-usage-sort')).toBeNull()
    expect(wrapper.get('[data-test="row-order"]').text()).toBe(
      'server-first@example.com,usage-first@example.com'
    )
    expect(listUsers).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({ sort_by: 'last_used_at', sort_order: 'desc' }),
      expect.any(Object)
    )
  })

  it('loads inactive-inclusive groups and sends api key group filter when visible', async () => {
    localStorage.setItem('user-visible-filters', JSON.stringify(['apiKeyGroup']))
    localStorage.setItem('user-filter-values', JSON.stringify({ apiKeyGroup: 42 }))

    mountUsersView()

    await flushPromises()

    expect(getAllGroups).not.toHaveBeenCalled()
    expect(getAllGroupsIncludingInactive).toHaveBeenCalledTimes(1)
    expect(listUsers).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({
        api_key_group_id: 42,
        group_name: undefined
      }),
      expect.any(Object)
    )
  })

  it('resets to the first page before applying a filter change', async () => {
    localStorage.setItem('user-visible-filters', JSON.stringify(['apiKeyGroup']))
    listUsers.mockResolvedValue({
      items: [createAdminUser()],
      total: 40,
      page: 1,
      page_size: 20,
      pages: 2
    })

    const wrapper = mountUsersView()
    await flushPromises()

    await wrapper.get('[data-test="page-two"]').trigger('click')
    await flushPromises()
    expect(listUsers).toHaveBeenLastCalledWith(2, 20, expect.any(Object), expect.any(Object))

    await wrapper.get('[data-test="select-stub"]').trigger('click')
    await flushPromises()

    expect(listUsers).toHaveBeenLastCalledWith(1, 20, expect.any(Object), expect.any(Object))
  })
})
