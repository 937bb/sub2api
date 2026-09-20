import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CodexScanSettingsDialog from '../CodexScanSettingsDialog.vue'
import type { CodexTurnStateScanSettings } from '@/api/admin/ops'

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
  parallel_probes: 5,
  dynamic_proxy_enabled: false,
  dynamic_proxy_url: 'https://api.cliproxy.io/white/api?region=Rand&num=1&format=n&type=txt'
}

function render() {
  return mount(CodexScanSettingsDialog, {
    props: { show: true },
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
