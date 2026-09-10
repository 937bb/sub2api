import { beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Component from './OAuthUpstreamRetrySettings.vue'

const mocks = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), success: vi.fn(), error: vi.fn() }))
vi.mock('@/api/admin/oauthUpstreamRetry', () => ({ getOAuthUpstreamRetrySettings: mocks.get, updateOAuthUpstreamRetrySettings: mocks.put }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showSuccess: mocks.success, showError: mocks.error }) }))
vi.mock('@/utils/apiError', () => ({ extractApiErrorMessage: (_: unknown, fallback: string) => fallback }))

beforeEach(() => {
 vi.clearAllMocks()
 mocks.get.mockResolvedValue({ enabled: true, max_retries: 3, status_codes: [429,502,503,504] })
 mocks.put.mockImplementation(async (value) => value)
})

it('loads and saves settings with numeric error codes', async () => {
 const wrapper=mount(Component)
 expect(wrapper.classes()).toContain('card')
 expect(wrapper.get('#oauth-upstream-retry-title').element.tagName).toBe('H2')
 await flushPromises()
 expect(wrapper.get('fieldset').element.parentElement?.classList.contains('p-6')).toBe(true)
 await wrapper.get('#oauth-upstream-retry-count').setValue(2)
 await wrapper.get('#oauth-upstream-retry-codes').setValue('502, 503, 502')
 await wrapper.findAll('button').at(-1)!.trigger('click')
 await flushPromises()
 expect(mocks.put).toHaveBeenCalledWith({ enabled:true,max_retries:2,status_codes:[502,503] })
})

it('rejects invalid values without sending requests',async()=>{
 const wrapper=mount(Component)
 await flushPromises()
 await wrapper.get('#oauth-upstream-retry-count').setValue(11)
 await wrapper.findAll('button').at(-1)!.trigger('click')
 expect(mocks.put).not.toHaveBeenCalled()
 await wrapper.get('#oauth-upstream-retry-count').setValue(1)
 await wrapper.get('#oauth-upstream-retry-codes').setValue('200')
 await wrapper.findAll('button').at(-1)!.trigger('click')
 expect(mocks.put).not.toHaveBeenCalled()
})

it('prevents saving after load failure',async()=>{
 mocks.get.mockRejectedValue(new Error('failed'))
 const wrapper=mount(Component)
 await flushPromises()
 expect(wrapper.find('fieldset').exists()).toBe(false)
 expect(mocks.put).not.toHaveBeenCalled()
})
