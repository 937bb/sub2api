import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import KiroUsageCell from '../KiroUsageCell.vue'
import type { AccountUsageInfo } from '@/types'
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
const render = (usage: Partial<AccountUsageInfo> | null) => mount(KiroUsageCell, { props: { usage: usage as AccountUsageInfo | null, loading: false, error: null }, global: { stubs: { UsageProgressBar: true } } })
describe('Kiro quota', () => {
 it('shows real usage and reset with a progress bar', () => {
  const w = render({ kiro_quota: { plan: 'KIRO FREE', used: 2.5, limit: 50, utilization: 5, resets_at: '2026-12-01T00:00:00Z' } })
  expect(w.text()).toContain('KIRO FREE'); expect(w.text()).toContain('2.5 / 50')
  expect(w.findComponent({ name: 'UsageProgressBar' }).props('utilization')).toBe(5)
 })
 it('never renders an invented zero percent for unknown limits', () => {
  const w = render({ kiro_quota: { plan: 'Kiro', used: 0, limit: 0, utilization: null } })
  expect(w.findComponent({ name: 'UsageProgressBar' }).exists()).toBe(false)
  expect(w.text()).toContain('quotaUnknown')
 })
 it('shows upstream errors and supports explicit refresh', async () => {
  const w = render({ error: 'Kiro quota HTTP 403' })
  expect(w.text()).toContain('HTTP 403'); await w.get('button').trigger('click'); expect(w.emitted('refresh')).toHaveLength(1)
 })
})
