import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import type { Account } from '@/types'
import OpenAIQuotaResetCell from '../OpenAIQuotaResetCell.vue'

const { queryOpenAIQuotaMock, resetOpenAIQuotaMock } = vi.hoisted(() => ({
  queryOpenAIQuotaMock: vi.fn(),
  resetOpenAIQuotaMock: vi.fn()
}))

vi.mock('@/api/admin/accounts', () => ({
  queryOpenAIQuota: queryOpenAIQuotaMock,
  resetOpenAIQuota: resetOpenAIQuotaMock
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key} ${JSON.stringify(params)}` : key
    })
  }
})

const ConfirmDialogStub = defineComponent({
  name: 'ConfirmDialog',
  props: {
    show: Boolean,
    title: String,
    message: String,
    confirmText: String,
    cancelText: String,
    danger: Boolean
  },
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" data-testid="confirm-dialog">
      <div data-testid="confirm-title">{{ title }}</div>
      <div data-testid="confirm-message">{{ message }}</div>
      <button type="button" data-testid="confirm-reset" @click="$emit('confirm')">
        {{ confirmText }}
      </button>
      <button type="button" data-testid="cancel-reset" @click="$emit('cancel')">
        {{ cancelText }}
      </button>
    </div>
  `
})

function makeOpenAIOAuthAccount(overrides: Partial<Account> = {}): Account {
  return {
    id: 10,
    name: 'OpenAI OAuth',
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-03-15T00:00:00Z',
    updated_at: '2026-03-15T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides
  }
}

function mountCell(account = makeOpenAIOAuthAccount()) {
  return mount(OpenAIQuotaResetCell, {
    props: { account },
    global: {
      stubs: {
        ConfirmDialog: ConfirmDialogStub
      }
    }
  })
}

describe('OpenAIQuotaResetCell', () => {
  beforeEach(() => {
    queryOpenAIQuotaMock.mockReset()
    resetOpenAIQuotaMock.mockReset()
    queryOpenAIQuotaMock.mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 2
      },
      fetched_at: 1790000000
    })
    resetOpenAIQuotaMock.mockResolvedValue({
      code: 'ok',
      windows_reset: 1
    })
  })

  it('requires confirmation before consuming a weekly reset credit', async () => {
    const wrapper = mountCell()

    const buttons = wrapper.findAll('button')
    await buttons[0].trigger('click')
    await flushPromises()

    await buttons[1].trigger('click')
    await flushPromises()

    expect(resetOpenAIQuotaMock).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="confirm-dialog"]').text()).toContain(
      'admin.accounts.openaiQuotaReset.confirmMessage {"count":2}'
    )

    await wrapper.get('[data-testid="confirm-reset"]').trigger('click')
    await flushPromises()

    expect(resetOpenAIQuotaMock).toHaveBeenCalledTimes(1)
    expect(resetOpenAIQuotaMock).toHaveBeenCalledWith(10)
  })

  it('does not open confirmation or call reset API when no reset credits are loaded', async () => {
    const wrapper = mountCell()

    const resetButton = wrapper.findAll('button')[1]
    await resetButton.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="confirm-dialog"]').exists()).toBe(false)
    expect(resetOpenAIQuotaMock).not.toHaveBeenCalled()
  })
})
