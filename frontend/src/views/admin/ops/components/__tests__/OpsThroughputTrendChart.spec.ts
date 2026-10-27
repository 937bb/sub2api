import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { useDark } from '@vueuse/core'

import OpsThroughputTrendChart from '../OpsThroughputTrendChart.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('vue-chartjs', () => ({
  Line: {
    props: ['data', 'options'],
    template: '<div class="line-chart" />',
  },
}))

describe('OpsThroughputTrendChart', () => {
  it('updates readable chart labels when the theme changes while mounted', async () => {
    localStorage.setItem('theme', 'light')
    const dark = useDark({ storageKey: 'theme' })
    dark.value = false
    await nextTick()
    const wrapper = mount(OpsThroughputTrendChart, {
      props: { points: [{ bucket_start: '2026-10-27T08:00:00Z', request_count: 1, token_consumed: 10, qps: 1, tps: 10 }], loading: false, timeRange: '1h' },
      global: { stubs: { EmptyState: true, HelpTooltip: true } },
    })
    const chart = wrapper.getComponent('.line-chart')
    expect(chart.props('options').animation).toBe(false)
    expect(chart.props('options').scales.x.ticks.color).toBe('#626b78')
    dark.value = true
    await nextTick()
    await nextTick()
    expect(chart.props('options').scales.x.ticks.color).toBe('#aebed2')
    expect(chart.props('options').plugins.legend.labels.color).toBe('#aebed2')
    wrapper.unmount()
    dark.value = false
  })

  it('allows the header controls to wrap on narrow screens', () => {
    const wrapper = mount(OpsThroughputTrendChart, {
      props: {
        points: [],
        loading: false,
        timeRange: '1h',
      },
      global: {
        stubs: {
          EmptyState: true,
          HelpTooltip: true,
        },
      },
    })

    const header = wrapper.get('[data-testid="throughput-chart-header"]')
    expect(header.classes()).toEqual(expect.arrayContaining(['flex-col', 'sm:flex-row']))

    const toolbar = wrapper.get('[data-testid="throughput-chart-toolbar"]')
    expect(toolbar.classes()).toEqual(expect.arrayContaining(['w-full', 'flex-wrap', 'sm:w-auto']))
    expect(toolbar.findAll('button')).toHaveLength(3)
    toolbar.findAll('button').forEach((button) => {
      expect(button.classes()).toContain('shrink-0')
      expect(button.classes()).not.toContain('ml-2')
    })
  })
})
