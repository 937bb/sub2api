import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import Toggle from '@/components/common/Toggle.vue'
import type { AdminGroup } from '@/types'
import TimeBillingView from '@/views/admin/TimeBillingView.vue'

const { getAllIncludingInactive, updateGroup, showSuccess, showError } = vi.hoisted(() => ({
  getAllIncludingInactive: vi.fn(),
  updateGroup: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { groups: { getAllIncludingInactive, update: updateGroup } },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const group = {
  id: 7,
  name: 'Standard OpenAI',
  platform: 'openai',
  status: 'active',
  subscription_type: 'standard',
  rate_multiplier: 0.8,
  peak_rate_enabled: false,
  peak_start: '',
  peak_end: '',
  peak_rate_multiplier: 1,
} as AdminGroup

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })

describe('TimeBillingView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getAllIncludingInactive.mockResolvedValue([group])
    updateGroup.mockImplementation(async (_id: number, payload: Record<string, unknown>) => ({
      ...group,
      ...payload,
    }))
  })

  it('lists standard groups and saves a time billing rule', async () => {
    const wrapper = mount(TimeBillingView, {
      global: { stubs: { AppLayout: AppLayoutStub } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Standard OpenAI')
    expect(wrapper.find('table').classes()).toContain('w-full')

    wrapper.findComponent(Toggle).vm.$emit('update:modelValue', true)
    await nextTick()
    await wrapper.find('input[type="number"]').setValue('1.5')
    await wrapper.find('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateGroup).toHaveBeenCalledWith(7, {
      peak_rate_enabled: true,
      peak_start: '09:00',
      peak_end: '18:00',
      peak_rate_multiplier: 1.5,
    })
    expect(showSuccess).toHaveBeenCalled()
  })
})
