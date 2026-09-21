import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CodexStateRulesPanel from '../CodexStateRulesPanel.vue'
import type { CodexTurnStateScanSettings } from '@/api/admin/ops'

const { getSettings, updateSettings, showError } = vi.hoisted(() => ({ getSettings: vi.fn(), updateSettings: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/admin/ops', () => ({ opsAPI: { getCodexTurnStateScanSettings: getSettings, updateCodexTurnStateScanSettings: updateSettings } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const settings: CodexTurnStateScanSettings = {
  target_lengths: [332, 292],
  rules: [{ plan_type: 'pro', model: '*', target_lengths: [332, 292] }, { plan_type: 'team', model: '*', target_lengths: [332, 292] }],
  plan_scan_enabled: { pro: true, team: true, plus: true, free: true, enterprise: true },
  require_state_before_routing: true,
  require_route_binding: false,
  parallel_probes: 5, dynamic_proxy_enabled: true, dynamic_proxy_url: 'https://proxy.example/api'
}
describe('CodexStateRulesPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getSettings.mockResolvedValue(settings)
    updateSettings.mockImplementation(async value => value)
  })
  it('shows editable account type, model and State fields immediately on page load', async () => {
    const wrapper = mount(CodexStateRulesPanel)
    await flushPromises()
    expect(wrapper.findAll('[data-test="inline-state-rule"]')).toHaveLength(2)
    expect(wrapper.findAll<HTMLSelectElement>('[data-test="inline-rule-plan"]').map(item => item.element.value)).toEqual(['pro', 'team'])
    expect(wrapper.findAll<HTMLInputElement>('[data-test="inline-rule-lengths"]').map(item => item.element.value)).toEqual(['332, 292', '332, 292'])
    expect(wrapper.get('[data-test="inline-save-rules"]').isVisible()).toBe(true)
  })
  it('saves changes while preserving the latest unrelated scanner configuration', async () => {
    const wrapper = mount(CodexStateRulesPanel)
    await flushPromises()
    await wrapper.findAll('[data-test="inline-rule-lengths"]')[1].setValue('286,292')
    getSettings.mockResolvedValueOnce({ ...settings, parallel_probes: 3, dynamic_proxy_url: 'https://new-proxy.example/api', dynamic_proxy_enabled: false })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateSettings).toHaveBeenCalledWith({ ...settings, parallel_probes: 3, dynamic_proxy_url: 'https://new-proxy.example/api', dynamic_proxy_enabled: false, rules: [settings.rules[0], { ...settings.rules[1], target_lengths: [286, 292] }] })
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })
  it('can disable Pro scanning independently of its State length rule', async () => {
    const wrapper = mount(CodexStateRulesPanel)
    await flushPromises()
    await wrapper.get('[data-test="inline-plan-scan-pro"]').trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateSettings).toHaveBeenCalledWith({ ...settings, plan_scan_enabled: { ...settings.plan_scan_enabled, pro: false } })
  })
  it('shows and persists the new-account routing guard', async () => {
    const wrapper = mount(CodexStateRulesPanel)
    await flushPromises()
    expect(wrapper.get('[data-test="state-routing-guard-status"]').text()).toContain('routingGuardEnabled')
    await wrapper.get('[data-test="state-routing-guard-toggle"]').trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateSettings).toHaveBeenCalledWith({ ...settings, require_state_before_routing: false })
  })
  it('shows and persists strict State acquisition exit binding', async () => {
    const wrapper = mount(CodexStateRulesPanel)
    await flushPromises()
    expect(wrapper.get('[data-test="state-route-binding-status"]').text()).toContain('routeBindingDisabled')
    await wrapper.get('[data-test="state-route-binding-toggle"]').trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateSettings).toHaveBeenCalledWith({ ...settings, require_route_binding: true })
  })
  it('adds a normalized model rule and deletes the selected row only', async () => {
    const wrapper = mount(CodexStateRulesPanel)
    await flushPromises()
    await wrapper.get('[data-test="inline-add-rule"]').trigger('click')
    const row = wrapper.findAll('[data-test="inline-state-rule"]')[2]
    await row.get('[data-test="inline-rule-plan"]').setValue('team')
    await row.get('[data-test="inline-rule-model"]').setValue(' GPT-6-ASTRA ')
    await row.get('[data-test="inline-rule-lengths"]').setValue('273')
    await wrapper.findAll('[data-test="inline-delete-rule"]')[0].trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateSettings).toHaveBeenCalledWith({ ...settings, rules: [settings.rules[1], { plan_type: 'team', model: 'gpt-6-astra', target_lengths: [273] }] })
  })
  it('blocks duplicate plan/model pairs', async () => {
    const wrapper = mount(CodexStateRulesPanel)
    await flushPromises()
    await wrapper.findAll('[data-test="inline-rule-plan"]')[1].setValue('pro')
    await wrapper.get('form').trigger('submit')
    expect(updateSettings).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('duplicateRule')
  })
  it('preserves input and refuses writes when the fresh settings read fails', async () => {
    const wrapper = mount(CodexStateRulesPanel)
    await flushPromises()
    await wrapper.findAll('[data-test="inline-rule-lengths"]')[0].setValue('332')
    getSettings.mockRejectedValueOnce(new Error('unavailable'))
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateSettings).not.toHaveBeenCalled()
    expect(wrapper.findAll<HTMLInputElement>('[data-test="inline-rule-lengths"]')[0].element.value).toBe('332')
    expect(showError).toHaveBeenCalled()
  })
  it('does not expose writable defaults when initial loading fails', async () => {
    getSettings.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mount(CodexStateRulesPanel)
    await flushPromises()
    expect(wrapper.get('[data-test="inline-save-rules"]').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(updateSettings).not.toHaveBeenCalled()
  })
  it('scrolls an existing model rule into the panel viewport before focusing it', async () => {
    getSettings.mockResolvedValueOnce({ ...settings, rules: [...settings.rules, ...Array.from({ length: 8 }, (_, index) => ({ plan_type: 'team', model: `gpt-model-${index}`, target_lengths: [332, 292] }))] })
    const wrapper = mount(CodexStateRulesPanel)
    await flushPromises()
    const input = wrapper.findAll<HTMLInputElement>('[data-test="inline-rule-lengths"]')[9].element
    const scrollIntoView = vi.fn()
    Object.defineProperty(input, 'scrollIntoView', { configurable: true, value: scrollIntoView })
    const focus = vi.spyOn(input, 'focus')
    await wrapper.vm.editRule({ plan_type: 'team', model: 'gpt-model-7', target_lengths: [332, 292] })
    expect(scrollIntoView).toHaveBeenCalledWith({ block: 'nearest', inline: 'nearest' })
    expect(focus).toHaveBeenCalledWith({ preventScroll: true })
    expect(scrollIntoView.mock.invocationCallOrder[0]).toBeLessThan(focus.mock.invocationCallOrder[0])
    expect(wrapper.findAll('[data-test="inline-state-rule"]')).toHaveLength(10)
  })
})
