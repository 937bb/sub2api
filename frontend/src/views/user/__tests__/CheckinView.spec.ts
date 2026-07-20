import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CheckinView from '@/views/user/CheckinView.vue'

const { getCheckinStatus, claimCheckin, showError, refreshUser } = vi.hoisted(() => ({
  getCheckinStatus: vi.fn(),
  claimCheckin: vi.fn(),
  showError: vi.fn(),
  refreshUser: vi.fn(),
}))

vi.mock('@/api/growth', () => ({ getCheckinStatus, claimCheckin }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ refreshUser }) }))
vi.mock('@/utils/deviceIdentity', () => ({ getBrowserDeviceID: vi.fn().mockResolvedValue('device-id-for-tests') }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'zh-CN' },
      t: (key: string) => key === 'growth.checkin.errors.GROWTH_RECHARGE_TOO_LOW' ? '累计真实充值不足' : key,
    }),
  }
})

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })

describe('CheckinView errors', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getCheckinStatus.mockResolvedValue({
      config: {
        checkin_enabled: true,
        checkin_reward_mode: 'fixed',
        checkin_fixed_reward: 1,
        checkin_min_reward: 0,
        checkin_max_reward: 1,
        checkin_streak_rewards: [],
        leaderboard_enabled: true,
      },
      today: '2026-07-20',
      checked_in_today: false,
      current_streak: 0,
      total_checkins: 0,
      month_checkins: [],
    })
  })

  it('shows the localized rejection reason returned by the API', async () => {
    claimCheckin.mockRejectedValue({ reason: 'GROWTH_RECHARGE_TOO_LOW', message: 'lifetime real recharge is below the configured minimum' })
    const wrapper = mount(CheckinView, { global: { stubs: { AppLayout: AppLayoutStub } } })
    await flushPromises()

    await wrapper.find('button.btn-primary').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('累计真实充值不足')
  })
})
