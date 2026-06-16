import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const {
  updateAccountMock,
  clearErrorMock,
  applyOAuthCredentialsMock,
  exchangeCodeMock
} = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  clearErrorMock: vi.fn(),
  applyOAuthCredentialsMock: vi.fn(),
  exchangeCodeMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      clearError: clearErrorMock,
      applyOAuthCredentials: applyOAuthCredentialsMock,
      generateAuthUrl: vi.fn(),
      exchangeCode: exchangeCodeMock
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import ReAuthAccountModal from '../ReAuthAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  template: '<div data-testid="oauth-flow"></div>',
  setup() {
    return {
      authCode: 'auth-code',
      oauthState: 'state-value',
      inputMethod: 'manual',
      reset: vi.fn()
    }
  }
})

function buildOpenAIAccount(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    name: 'OpenAI account',
    notes: '',
    platform: 'openai',
    type: 'setup-token',
    credentials: {},
    extra: {
      openai_oauth_ws_mode: 'managed_session',
      openai_oauth_passthrough: true,
      openai_oauth_responses_websockets_v2_mode: 'passthrough',
      openai_oauth_responses_websockets_v2_enabled: true,
      openai_apikey_responses_websockets_v2_mode: 'ctx_pool',
      openai_apikey_responses_websockets_v2_enabled: true,
      email: 'old@example.com'
    },
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'error',
    error_message: 'expired',
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides
  } as any
}

function mountModal(account = buildOpenAIAccount()) {
  return mount(ReAuthAccountModal, {
    props: {
      show: true,
      account
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Icon: true,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub
      }
    }
  })
}

describe('Admin ReAuthAccountModal', () => {
  beforeEach(() => {
    updateAccountMock.mockReset()
    clearErrorMock.mockReset()
    applyOAuthCredentialsMock.mockReset()
    exchangeCodeMock.mockReset()

    updateAccountMock.mockResolvedValue({ ...buildOpenAIAccount(), status: 'error' })
    clearErrorMock.mockResolvedValue({ ...buildOpenAIAccount(), status: 'active' })
    applyOAuthCredentialsMock.mockResolvedValue({ ...buildOpenAIAccount(), type: 'oauth' })
    exchangeCodeMock.mockResolvedValue({
      access_token: 'redacted-access-token',
      refresh_token: 'redacted-refresh-token',
      expires_at: 1790000000,
      email: 'new@example.com',
      name: 'New User',
      privacy_mode: 'training_disabled'
    })
  })

  it('preserves OpenAI setup-token type and removes legacy passthrough keys during re-auth', async () => {
    const wrapper = mountModal()

    const vm = wrapper.vm as any
    vm.openaiOAuth.sessionId.value = 'session-id'
    vm.openaiOAuth.oauthState.value = 'generated-state'
    vm.oauthFlowRef.value = {
      authCode: 'auth-code',
      oauthState: 'stale-callback-state',
      inputMethod: 'manual',
      reset: vi.fn(),
      projectId: '',
      sessionKey: ''
    }
    await flushPromises()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateAccountMock).not.toHaveBeenCalled()
    expect(exchangeCodeMock).toHaveBeenCalledWith('/admin/openai/exchange-code', {
      session_id: 'session-id',
      code: 'auth-code',
      state: 'generated-state'
    })
    expect(applyOAuthCredentialsMock).toHaveBeenCalledTimes(1)
    const payload = applyOAuthCredentialsMock.mock.calls[0]?.[1]
    expect(payload.type).toBe('setup-token')
    expect(payload.extra_delete_keys).toContain('openai_oauth_passthrough')
    expect(payload.extra_delete_keys).toContain('openai_apikey_responses_websockets_v2_mode')
    expect(payload.extra).toMatchObject({
      openai_oauth_ws_mode: 'managed_session',
      email: 'new@example.com',
      name: 'New User',
      privacy_mode: 'training_disabled'
    })
    expect(payload.extra).not.toHaveProperty('openai_oauth_passthrough')
    expect(payload.extra).not.toHaveProperty('openai_oauth_responses_websockets_v2_mode')
    expect(payload.extra).not.toHaveProperty('openai_oauth_responses_websockets_v2_enabled')
    expect(payload.extra).not.toHaveProperty('openai_passthrough')
    expect(payload.extra).not.toHaveProperty('openai_apikey_responses_websockets_v2_mode')
    expect(payload.extra).not.toHaveProperty('openai_apikey_responses_websockets_v2_enabled')
    expect(clearErrorMock).not.toHaveBeenCalled()
  })
})
