import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import KiroCacheFields from '../KiroCacheFields.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('Kiro cache billing modes', () => {
 it('keeps legacy response policies and explicitly opts into prefix billing', async () => {
  const original = { enabled: true, target_percent: 90, ttl_seconds: 3600, max_entries: 128 }
  const wrapper = mount(KiroCacheFields, { props: { modelValue: original } })
  expect((wrapper.get('select').element as HTMLSelectElement).value).toBe('response')
  await wrapper.get('select').setValue('prefix')
  const next = wrapper.emitted('update:modelValue')?.at(-1)?.[0]
  expect(next).toEqual({ ...original, mode: 'prefix', auto_prefix: true })
  await wrapper.setProps({ modelValue: next as typeof original })
  expect(wrapper.text()).toContain('admin.accounts.kiro.prefixHint')
  expect(wrapper.text()).toContain('admin.accounts.kiro.prefixTarget')
  expect(wrapper.text()).toContain('admin.accounts.kiro.autoPrefix')
 })
})
