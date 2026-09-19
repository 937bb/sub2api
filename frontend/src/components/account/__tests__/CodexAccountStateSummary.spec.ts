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
  it('shows every target model as a compact vertical row and only counts ready rows', () => {
    const wrapper = mount(CodexAccountStateSummary, {
      props: {
        states: [
          state('gpt-6-astra', 'ready', 332),
          state('gpt-5.6-sol', 'expiring', 292),
          state('gpt-5.6-terra', 'retry_wait')
        ]
      }
    })

    expect(wrapper.text()).toContain('1/3 models ready')
    expect(wrapper.text()).toContain('6 astra')
    expect(wrapper.text()).toContain('332')
    expect(wrapper.text()).toContain('5.6 sol')
    expect(wrapper.text()).toContain('292 · expiring')
    expect(wrapper.text()).toContain('5.6 terra')
    expect(wrapper.text()).toContain('1 failed')
    expect(wrapper.findAll('[data-test^="codex-model-state-"]')).toHaveLength(3)
    expect(wrapper.get('[data-status="ready"]').classes()).toContain('text-emerald-700')
    expect(wrapper.get('[data-status="expiring"]').classes()).toContain('text-amber-700')
    expect(wrapper.get('[data-status="retry_wait"]').classes()).toContain('text-red-600')
  })

  it('does not mark an account ready when one target model is missing', () => {
    const wrapper = mount(CodexAccountStateSummary, {
      props: { states: [state('gpt-6-astra', 'ready', 292), state('gpt-5.6-sol', 'missing')] }
    })

    expect(wrapper.text()).toContain('1/2 models ready')
    expect(wrapper.get('[data-status="ready"]').classes()).toContain('text-emerald-700')
    expect(wrapper.get('[data-status="missing"]').classes()).toContain('text-red-600')
    expect(wrapper.get('[data-status="missing"]').text()).toContain('missing')
  })
})
