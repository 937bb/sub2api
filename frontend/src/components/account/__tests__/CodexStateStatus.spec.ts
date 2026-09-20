import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CodexStateStatus from '../CodexStateStatus.vue'

const labels: Record<string, string> = {
  'admin.ops.turnState.status.ready': 'Target State ready',
  'admin.ops.turnState.status.running': 'Scanning',
  'admin.ops.turnState.status.missing': 'Missing'
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: { lengths?: string }) => key === 'admin.ops.turnState.targetLengths' ? `Target ${params?.lengths}` : labels[key] || key
  })
}))

function render(props: Record<string, unknown>) {
  return mount(CodexStateStatus, {
    props
  })
}

describe('CodexStateStatus', () => {
  it('shows a ready state with its length and model', () => {
    const wrapper = render({ status: 'ready', stateLength: 332, model: 'gpt-5.6-sol' })

    expect(wrapper.text()).toContain('Target State ready')
    expect(wrapper.text()).toContain('332')
    expect(wrapper.text()).toContain('gpt-5.6-sol')
    expect(wrapper.find('.bg-emerald-500').exists()).toBe(true)
  })

  it('animates active scans and keeps compact details readable', () => {
    const wrapper = render({ status: 'running', compact: true, stateLength: 292 })

    expect(wrapper.text()).toContain('Scanning')
    expect(wrapper.find('.animate-pulse').exists()).toBe(true)
    expect(wrapper.find('.h-6.w-6').exists()).toBe(true)
  })

  it('distinguishes the observed state length from the effective target rule', () => {
    const wrapper = render({ status: 'missing', stateLength: 292, targetLengths: [286, 273] })
    expect(wrapper.text()).toContain('292')
    expect(wrapper.get('[data-test="codex-state-target"]').text()).toBe('Target 286 / 273')
  })

  it('hides detail metadata when requested', () => {
    const wrapper = render({ status: 'missing', stateLength: 332, model: 'gpt-5.6-terra', showDetails: false })

    expect(wrapper.text()).toBe('Missing')
  })
})
