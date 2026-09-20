import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CodexScanSettingsDialog from '../CodexScanSettingsDialog.vue'
import type { CodexTurnStateScanSettings, CodexTurnStateLengthRule } from '@/api/admin/ops'

const { getSettings, updateSettings, showSuccess, showError } = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getCodexTurnStateScanSettings: getSettings,
    updateCodexTurnStateScanSettings: updateSettings
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const settings: CodexTurnStateScanSettings = {
  target_lengths: [332, 292],
  rules: [
    { plan_type: 'pro', model: '*', target_lengths: [292] },
    { plan_type: 'team', model: 'gpt-5.6-terra', target_lengths: [286] },
    { plan_type: 'team', model: 'gpt-6-astra', target_lengths: [273] }
  ],
  parallel_probes: 5,
  dynamic_proxy_enabled: false,
  dynamic_proxy_url: 'https://api.cliproxy.io/white/api?region=Rand&num=1&format=n&type=txt'
}

function render(selection?: CodexTurnStateLengthRule) {
  return mount(CodexScanSettingsDialog, {
    props: { show: true, selection },
    global: {
      stubs: {
        BaseDialog: { props: ['show'], template: '<section v-if="show"><slot /><slot name="footer" /></section>' }
      }
    }
  })
}

describe('CodexScanSettingsDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getSettings.mockResolvedValue(settings)
    updateSettings.mockImplementation(async value => value)
  })

  it('saves custom lengths in the entered priority order without sorting', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('#codex-scan-lengths').setValue('292， 356, 332')
    await wrapper.get('#codex-scan-parallel').setValue('3')
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledOnce()
    expect(updateSettings).toHaveBeenCalledWith({ ...settings, target_lengths: [292, 356, 332], parallel_probes: 3, dynamic_proxy_enabled: true })
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('edits plan/model lengths while preserving other rules and scan configuration', async () => {
    const wrapper = render({ plan_type: 'team', model: 'gpt-5.6-terra', target_lengths: [286] })
    await flushPromises()
    expect(wrapper.findAll('[data-test="state-rule"]')).toHaveLength(3)
    expect(wrapper.get('[data-test="rule-scope"]').text()).toContain('selectedScope')
    await wrapper.findAll('[data-test="rule-lengths"]')[1].setValue('286, 292')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateSettings).toHaveBeenCalledWith({ ...settings, rules: [settings.rules[0], { ...settings.rules[1], target_lengths: [286, 292] }, settings.rules[2]] })
  })

  it('creates an exact rule for a selected model without changing the inherited plan rule', async () => {
    const wrapper = render({ plan_type: 'pro', model: 'GPT-6-ASTRA', target_lengths: [292] })
    await flushPromises()
    expect(wrapper.findAll('[data-test="state-rule"]')).toHaveLength(4)
    expect(wrapper.findAll<HTMLInputElement>('[data-test="rule-model"]')[0].element.value).toBe('gpt-6-astra')
    await wrapper.findAll('[data-test="rule-lengths"]')[0].setValue('273')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateSettings).toHaveBeenCalledWith({ ...settings, rules: [{ plan_type: 'pro', model: 'gpt-6-astra', target_lengths: [273] }, ...settings.rules] })
  })

  it('normalizes added model names and supports an all-plan model rule', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-test="add-state-rule"]').trigger('click')
    const row = wrapper.findAll('[data-test="state-rule"]')[3]
    await row.get('[data-test="rule-model"]').setValue(' GPT-6-ASTRA ')
    await row.get('[data-test="rule-lengths"]').setValue('273，292')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateSettings).toHaveBeenCalledWith({ ...settings, rules: [...settings.rules, { plan_type: '*', model: 'gpt-6-astra', target_lengths: [273, 292] }] })
  })

  it('rejects duplicate plan/model rules after model normalization', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-test="add-state-rule"]').trigger('click')
    const row = wrapper.findAll('[data-test="state-rule"]')[3]
    await row.get('[data-test="rule-plan"]').setValue('team')
    await row.get('[data-test="rule-model"]').setValue(' GPT-6-ASTRA ')
    await wrapper.get('form').trigger('submit')
    expect(updateSettings).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('duplicateRule')
  })

  it.each(['', '63', '273,273', '273.5'])('rejects invalid rule lengths %s', async value => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('[data-test="rule-lengths"]')[0].setValue(value)
    await wrapper.get('form').trigger('submit')
    expect(updateSettings).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('invalidRuleLengths')
  })

  it('deletes a rule without clearing other rules', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.findAll('[data-test="delete-state-rule"]')[1].trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(updateSettings).toHaveBeenCalledWith({ ...settings, rules: [settings.rules[0], settings.rules[2]] })
  })

  it.each(['', '63', '4097', '332,332', '332, 292.5', 'invalid', Array.from({ length: 17 }, (_, index) => 64 + index).join(',')])('rejects invalid lengths %s before writing', async value => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('#codex-scan-lengths').setValue(value)
    await wrapper.get('form').trigger('submit')

    expect(updateSettings).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('invalidLengths')
  })

  it('does not save default values when loading fails', async () => {
    getSettings.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = render()
    await flushPromises()
    await wrapper.get('form').trigger('submit')

    expect(updateSettings).not.toHaveBeenCalled()
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[role="alert"]').text()).toContain('loadFailed')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('#codex-scan-lengths').element.value).toBe('332, 292')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })

  it('preserves edited configuration when saving or reloading fails', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('#codex-scan-lengths').setValue('356, 332')
    updateSettings.mockRejectedValueOnce(new Error('unavailable'))
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.get<HTMLInputElement>('#codex-scan-lengths').element.value).toBe('356, 332')
    expect(showError).toHaveBeenCalledWith('admin.ops.turnState.scanSettings.saveFailed')
    getSettings.mockRejectedValueOnce(new Error('unavailable'))
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.get<HTMLInputElement>('#codex-scan-lengths').element.value).toBe('356, 332')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
  })

  it('rejects invalid concurrency and unsafe provider URLs', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('#codex-scan-parallel').setValue('6')
    await wrapper.get('form').trigger('submit')
    expect(updateSettings).not.toHaveBeenCalled()
    await wrapper.get('#codex-scan-parallel').setValue('4')
    await wrapper.get('input[type="checkbox"]').setValue(true)
    for (const value of ['http://proxy.example/api', 'https://user:pass@proxy.example/api', 'https://proxy.example/api#part', 'https://proxy.example/api#', `https://proxy.example/${'a'.repeat(2048)}`, 'invalid']) {
      await wrapper.get('#codex-scan-provider').setValue(value)
      await wrapper.get('form').trigger('submit')
    }
    expect(updateSettings).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('invalidUrl')
  })

  it('keeps only one write in flight even on repeated form submission', async () => {
    let resolveSave!: (value: CodexTurnStateScanSettings) => void
    updateSettings.mockReturnValueOnce(new Promise<CodexTurnStateScanSettings>(resolve => { resolveSave = resolve }))
    const wrapper = render()
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    expect(updateSettings).toHaveBeenCalledOnce()
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    resolveSave(settings)
    await flushPromises()
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })
})
