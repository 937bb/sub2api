import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import GrowthView from '@/views/admin/GrowthView.vue'
import type { GrowthConfig } from '@/api/growth'

const { getGrowthConfig, updateGrowthConfig, showSuccess, showError } = vi.hoisted(() => ({
  getGrowthConfig: vi.fn(),
  updateGrowthConfig: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/growth', async () => {
  const actual = await vi.importActual<typeof import('@/api/growth')>('@/api/growth')
  return {
    ...actual,
    getGrowthConfig,
    updateGrowthConfig,
    listGrowthRewards: vi.fn(),
    listGrowthRiskEvents: vi.fn(),
    settleGrowthLeaderboard: vi.fn(),
  }
})

vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const config: GrowthConfig = {
  checkin_enabled: true,
  checkin_reward_mode: 'fixed',
  checkin_fixed_reward: 1,
  checkin_min_reward: 1,
  checkin_max_reward: 2,
  checkin_streak_rewards: [],
  checkin_min_account_age_days: 1,
  checkin_min_total_recharged: 1,
  checkin_max_reward_paid_ratio: 0.2,
  max_total_reward_paid_ratio: 0.2,
  checkin_min_recent_spend: 0.01,
  checkin_recent_spend_days: 30,
  checkin_max_accounts_per_ip: 2,
  checkin_max_accounts_per_device: 1,
  leaderboard_enabled: true,
  leaderboard_anonymous: false,
  leaderboard_display_limit: 20,
  leaderboard_reward_rules: [],
}

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })

describe('GrowthView leaderboard settings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getGrowthConfig.mockResolvedValue(structuredClone(config))
    updateGrowthConfig.mockImplementation(async (value: GrowthConfig) => JSON.parse(JSON.stringify(value)) as GrowthConfig)
  })

  it('shows first-place rules for every period and saves the display limit', async () => {
    const wrapper = mount(GrowthView, { global: { stubs: { AppLayout: AppLayoutStub } } })
    await flushPromises()

    const rankStarts = wrapper.findAll('input[aria-label="growth.admin.rankStart"]')
    expect(rankStarts).toHaveLength(3)
    expect(rankStarts.every((input) => input.element.value === '1')).toBe(true)

    const displayLimit = wrapper.find('input[type="number"][min="1"][max="100"][step="1"]')
    await displayLimit.setValue('10')

    const rewardSections = wrapper.findAll('section').slice(-3)
    await rewardSections[0].find('input[type="checkbox"]').setValue(true)
    await rewardSections[0].find('input[aria-label="growth.admin.rewardAmount"]').setValue('5')
    await wrapper.find('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateGrowthConfig).toHaveBeenCalledOnce()
    const saved = updateGrowthConfig.mock.calls[0][0] as GrowthConfig
    expect(saved.leaderboard_display_limit).toBe(10)
    expect(saved.leaderboard_reward_rules).toHaveLength(3)
    expect(saved.leaderboard_reward_rules).toEqual(expect.arrayContaining([
      expect.objectContaining({ period: 'daily', rank_start: 1, rank_end: 1, reward_amount: 5, enabled: true }),
      expect.objectContaining({ period: 'weekly', rank_start: 1, rank_end: 1, enabled: false }),
      expect.objectContaining({ period: 'monthly', rank_start: 1, rank_end: 1, enabled: false }),
    ]))
    expect(showSuccess).toHaveBeenCalled()
  })
})
