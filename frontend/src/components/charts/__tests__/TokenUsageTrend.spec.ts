import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import TokenUsageTrend from '../TokenUsageTrend.vue'

const messages: Record<string, string> = {
  'admin.dashboard.tokenUsageTrend': 'Token Usage Trend',
  'admin.dashboard.noDataAvailable': 'No data available',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

vi.mock('vue-chartjs', () => ({
  Bar: { props: ['data', 'options'], template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>' },
  Line: {
    props: ['data', 'options'],
    template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>',
  },
}))

describe('TokenUsageTrend', () => {
  it('keeps bars opaque when switching metrics and reserves transparency for area fills', async () => {
    const wrapper = mount(TokenUsageTrend, { props: { trendData: [
      { date: '2026-10-28', requests: 8, input_tokens: 200, output_tokens: 100, cache_creation_tokens: 300, cache_read_tokens: 500, cost: 5, actual_cost: 1.5 }
    ] } })
    const data = () => JSON.parse(wrapper.find('.chart-data').text())
    for (const metric of ['console.requests', 'console.spending', 'console.cacheRate']) {
      await wrapper.findAll('button').find(button => button.text() === metric)!.trigger('click')
      expect(data().datasets.every((dataset: { backgroundColor: string }) => /^#[0-9a-f]{6}$/i.test(dataset.backgroundColor))).toBe(true)
    }
    await wrapper.findAll('button').find(button => button.text() === 'console.areaChart')!.trigger('click')
    expect(data().datasets[0].backgroundColor).toMatch(/^#[0-9a-f]{8}$/i)
    expect(data().datasets[0].borderColor).toMatch(/^#[0-9a-f]{6}$/i)
    wrapper.unmount()
  })

  it('switches metric units without mixing billed charges with standard cost', async () => {
    const wrapper = mount(TokenUsageTrend, { props: { trendData: [
      { date: '2026-10-26', requests: 8, input_tokens: 200, output_tokens: 100, cache_creation_tokens: 300, cache_read_tokens: 500, cost: 5, actual_cost: 1.5 },
      { date: '2026-10-27', requests: 12, input_tokens: 0, output_tokens: 100, cache_creation_tokens: 0, cache_read_tokens: 100, cost: 3, actual_cost: 0.9 }
    ] } })
    const data = () => JSON.parse(wrapper.find('.chart-data').text())
    expect(data().datasets.map((item: { data: number[] }) => item.data)).toEqual([[200, 0], [100, 100], [500, 100], [300, 0]])
    await wrapper.findAll('button')[1].trigger('click')
    expect(wrapper.find('.console-chart-total strong').text()).toBe('20')
    expect(data().datasets[0].data).toEqual([8, 12])
    await wrapper.findAll('button')[2].trigger('click')
    expect(wrapper.find('.console-chart-total strong').text()).toBe('$2.40')
    expect(data().datasets.map((item: { data: number[] }) => item.data)).toEqual([[1.5, 0.9], [5, 3]])
    await wrapper.findAll('button')[3].trigger('click')
    // Use the weighted prompt total, not the arithmetic mean of daily rates.
    expect(wrapper.find('.console-chart-total strong').text()).toBe('54.5%')
    expect(data().datasets[0].data).toEqual([50, 100])
  })

  it('returns to tokens when financial display is disabled', async () => {
    const wrapper = mount(TokenUsageTrend, { props: { trendData: [] } })
    await wrapper.findAll('button').find(button => button.text() === 'console.spending')!.trigger('click')
    await wrapper.setProps({ showCosts: false })
    expect(wrapper.text()).not.toContain('console.spending')
    expect(wrapper.find('.console-chart-total').text()).not.toContain('$')
    expect(wrapper.findAll('button').find(button => button.text() === 'console.tokenComposition')!.attributes('aria-pressed')).toBe('true')
  })

  it('calculates cache hit rate against all prompt tokens', async () => {
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-08',
            requests: 1,
            input_tokens: 500,
            output_tokens: 100,
            cache_creation_tokens: 0,
            cache_read_tokens: 1500,
            cost: 0.01,
            actual_cost: 0.005,
          },
        ],
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })

    await wrapper.findAll('button').find(button => button.text() === 'console.cacheRate')!.trigger('click')
    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    const hitRateDataset = chartData.datasets.find(
      (ds: any) => ds.label === 'console.cacheRate'
    )
    // Hit rate = 1500 / (500 + 1500 + 0) * 100 = 75%
    expect(hitRateDataset.data[0]).toBe(75)
  })

  it('returns 0 hit rate when all prompt tokens are zero', async () => {
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-08',
            requests: 0,
            input_tokens: 0,
            output_tokens: 0,
            cache_creation_tokens: 0,
            cache_read_tokens: 0,
            cost: 0,
            actual_cost: 0,
          },
        ],
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })

    await wrapper.findAll('button').find(button => button.text() === 'console.cacheRate')!.trigger('click')
    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    const hitRateDataset = chartData.datasets.find(
      (ds: any) => ds.label === 'console.cacheRate'
    )
    expect(hitRateDataset.data[0]).toBe(0)
  })

  it('includes cache_creation_tokens in denominator for Anthropic models', async () => {
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-08',
            requests: 1,
            input_tokens: 200,
            output_tokens: 50,
            cache_creation_tokens: 300,
            cache_read_tokens: 500,
            cost: 0.02,
            actual_cost: 0.01,
          },
        ],
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })

    await wrapper.findAll('button').find(button => button.text() === 'console.cacheRate')!.trigger('click')
    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    const hitRateDataset = chartData.datasets.find(
      (ds: any) => ds.label === 'console.cacheRate'
    )
    // Hit rate = 500 / (200 + 500 + 300) * 100 = 50%
    expect(hitRateDataset.data[0]).toBe(50)
  })
})
