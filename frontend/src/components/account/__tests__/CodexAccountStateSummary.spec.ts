import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CodexAccountStateSummary from '../CodexAccountStateSummary.vue'
import type { CodexTurnStateAccountStatus } from '@/api/admin/ops'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (key === 'admin.ops.turnState.readyProgress') return `${params?.ready}/${params?.total} models ready`
      if (key === 'admin.ops.turnState.failedCount') return `${params?.count} failed`
      if (key === 'admin.ops.turnState.attempts') return `${params?.count} attempts`
      return key.split('.').at(-1) || key
    }
  })
}))

function state(model: string, status: CodexTurnStateAccountStatus['status'], length = 0): CodexTurnStateAccountStatus {
  return {
    account_id: 41,
    account_name: 'account@example.com',
    account_type: 'oauth',
    model,
    status,
    state_length: length,
    attempt_count: 2
  }
}

describe('CodexAccountStateSummary', () => {
  it('shows every target model and only counts ready rows', () => {
    const wrapper = mount(CodexAccountStateSummary, {
      props: {
        states: [
          state('gpt-6-astra', 'ready', 332),
          state('gpt-5.6-sol', 'retry_wait')
        ]
      }
    })

    expect(wrapper.text()).toContain('1/2 models ready')
    expect(wrapper.text()).toContain('6 astra')
    expect(wrapper.text()).toContain('332')
    expect(wrapper.text()).toContain('5.6 sol')
    expect(wrapper.text()).toContain('1 failed')
    expect(wrapper.findAll('[data-test^="codex-model-state-"]')).toHaveLength(2)
  })

  it('does not mark an account ready when one target model is missing', () => {
    const wrapper = mount(CodexAccountStateSummary, {
      props: { states: [state('gpt-6-astra', 'ready', 292), state('gpt-5.6-sol', 'missing')] }
    })

    expect(wrapper.text()).toContain('1/2 models ready')
    expect(wrapper.find('.bg-emerald-500').exists()).toBe(true)
    expect(wrapper.find('.bg-amber-500').exists()).toBe(true)
  })
})
