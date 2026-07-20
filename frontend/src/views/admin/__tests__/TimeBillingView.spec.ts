import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import Select from '@/components/common/Select.vue'
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
  time_billing_rules: [
    { id: 'day', enabled: true, start: '09:00', end: '18:00', rate_multiplier: 1.5 },
    { id: 'night', enabled: true, start: '22:00', end: '02:00', rate_multiplier: 0.8 },
  ],
} as AdminGroup

const unconfiguredGroup = {
  ...group,
  id: 8,
  name: 'Inactive Anthropic',
  platform: 'anthropic',
  status: 'inactive',
  time_billing_rules: [],
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

  it('lists and saves multiple rules including an overnight window', async () => {
    const wrapper = mount(TimeBillingView, {
      global: { stubs: { AppLayout: AppLayoutStub } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Standard OpenAI')
    expect(wrapper.text()).toContain('timeBilling.crossDay')
    expect(wrapper.find('table').classes()).toContain('w-full')

    await wrapper.find('input[type="number"]').setValue('1.6')
    await wrapper.find('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateGroup).toHaveBeenCalledWith(7, {
      time_billing_rules: [
        { id: 'day', enabled: true, repeat_type: 'daily', start: '09:00', end: '18:00', rate_multiplier: 1.6 },
        { id: 'night', enabled: true, repeat_type: 'daily', start: '22:00', end: '02:00', rate_multiplier: 0.8 },
      ],
    })
    expect(showSuccess).toHaveBeenCalled()
  })

  it('combines platform, group status, and rule state filters', async () => {
    getAllIncludingInactive.mockResolvedValue([group, unconfiguredGroup])
    const wrapper = mount(TimeBillingView, {
      global: { stubs: { AppLayout: AppLayoutStub } },
    })
    await flushPromises()

    const selects = wrapper.findAllComponents(Select)
    expect(selects.length).toBeGreaterThanOrEqual(3)
    selects[0].vm.$emit('update:modelValue', 'anthropic')
    selects[1].vm.$emit('update:modelValue', 'inactive')
    selects[2].vm.$emit('update:modelValue', 'unconfigured')
    await nextTick()

    expect(wrapper.text()).toContain('Inactive Anthropic')
    expect(wrapper.text()).not.toContain('Standard OpenAI')
  })

  it('blocks overlapping enabled windows across midnight', async () => {
    const wrapper = mount(TimeBillingView, {
      global: { stubs: { AppLayout: AppLayoutStub } },
    })
    await flushPromises()

    const timeInputs = wrapper.findAll('input[type="time"]')
    expect(timeInputs).toHaveLength(4)
    await timeInputs[1].setValue('23:00')

    expect(wrapper.text()).toContain('timeBilling.overlapError')
    expect(wrapper.find('button.btn-primary').attributes('disabled')).toBeDefined()
    expect(updateGroup).not.toHaveBeenCalled()
  })

  it('saves a weekly cross-week range with weekday selections', async () => {
    getAllIncludingInactive.mockResolvedValue([{
      ...group,
      time_billing_rules: [{
        id: 'weekend', enabled: true, repeat_type: 'weekly', start_weekday: 6, end_weekday: 1,
        start: '00:00', end: '00:00', rate_multiplier: 0.7,
      }],
    }])
    const wrapper = mount(TimeBillingView, {
      global: { stubs: { AppLayout: AppLayoutStub } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('timeBilling.crossWeek')
    expect(wrapper.text()).toContain('timeBilling.durationDays')

    await wrapper.find('input[type="number"]').setValue('0.8')
    await wrapper.find('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateGroup).toHaveBeenCalledWith(7, {
      time_billing_rules: [{
        id: 'weekend', enabled: true, repeat_type: 'weekly', start_weekday: 6, end_weekday: 1,
        start: '00:00', end: '00:00', rate_multiplier: 0.8,
      }],
    })
  })
})
