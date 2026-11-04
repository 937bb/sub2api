<template>
  <BaseDialog :show="show" :title="t('admin.accounts.fingerprint.view')" :description="account ? `${account.name} · #${account.id}` : ''" width="wide" @close="emit('close')">
    <div v-if="loading" class="flex justify-center py-10" role="status" :aria-label="t('common.loading')"><LoadingSpinner /></div>
    <div v-else-if="failed" class="space-y-3" role="alert">
      <p>{{ t('admin.accounts.fingerprint.loadFailed') }}</p>
      <button class="btn btn-secondary" @click="load">{{ t('common.retry') }}</button>
    </div>
    <div v-else-if="data" class="space-y-5">
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.fingerprint.description') }}</p>
      <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
        <div class="mb-2 flex items-center justify-between gap-3">
          <span class="font-medium">{{ t('admin.accounts.fingerprint.device') }}</span>
          <button v-if="data.installation_id" class="btn btn-secondary btn-sm" @click="copyToClipboard(data.installation_id)">{{ t('common.copy') }}</button>
        </div>
        <code class="block break-all text-sm">{{ data.installation_id || t('admin.accounts.fingerprint.sources.unavailable') }}</code>
        <p class="mt-2 text-xs text-gray-500">x-codex-installation-id · HTTP / WebSocket</p>
      </div>
      <dl class="grid grid-cols-1 gap-4 text-sm sm:grid-cols-2">
        <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.source') }}</dt><dd class="mt-1">{{ t(`admin.accounts.fingerprint.sources.${data.source}`) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.owner') }}</dt><dd class="mt-1">#{{ data.credential_account_id }} · {{ t(`admin.accounts.fingerprint.${data.inherited ? 'inherited' : 'own'}`) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.convergence') }}</dt><dd class="mt-1">{{ t(`admin.accounts.fingerprint.modes.${data.convergence_mode}`) }}</dd></div>
        <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.ws') }}</dt><dd class="mt-1">{{ data.ws_account_mode === 'default' ? t('admin.accounts.fingerprint.runtimeDefault') : data.ws_account_mode }}</dd></div>
        <div class="sm:col-span-2"><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.userAgent') }}</dt><dd class="mt-1 break-all font-mono text-xs">{{ data.user_agent_override || t('admin.accounts.fingerprint.runtimeDefault') }}</dd></div>
      </dl>
      <section v-if="data.runtime" class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-600">
        <h3 class="font-medium">{{ t('admin.accounts.fingerprint.runtimeTitle') }}</h3>
        <p class="text-xs text-gray-500">{{ t('admin.accounts.fingerprint.runtimeScope') }}</p>
        <dl class="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
          <div class="sm:col-span-2"><dt class="text-gray-500">User-Agent</dt><dd class="mt-1 break-all font-mono text-xs">{{ data.runtime.user_agent }}</dd></div>
          <div><dt class="text-gray-500">originator / version</dt><dd class="mt-1 font-mono text-xs">{{ data.runtime.originator }} / {{ data.runtime.version }}</dd></div>
          <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.binding') }}</dt><dd class="mt-1">{{ t(`admin.accounts.fingerprint.${data.runtime.strict_conversation_binding ? 'strict' : 'soft'}`) }}</dd></div>
          <div class="sm:col-span-2"><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.httpProfile') }}</dt><dd class="mt-1 break-all font-mono text-xs">{{ data.runtime.local_cli_transport ? data.runtime.http_profile : t('admin.accounts.fingerprint.otherTransport') }}</dd></div>
          <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.enforcement') }}</dt><dd>{{ data.runtime.identity_enforced ? t('common.enabled') : t('common.disabled') }}</dd></div>
          <div><dt class="text-gray-500">Force HTTP / WS router</dt><dd class="font-mono text-xs">{{ data.runtime.force_http }} / {{ data.runtime.ws_router_enabled }}</dd></div>
        </dl>
        <p class="text-xs text-gray-500">{{ t('admin.accounts.fingerprint.dynamicHeaders') }}: {{ data.runtime.dynamic_headers.join(', ') }}</p>
      </section>
      <section v-if="data.transports?.length" class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-600">
        <h3 class="font-medium">{{ t('admin.accounts.fingerprint.transportTitle') }}</h3>
        <p class="text-xs text-gray-500">{{ t('admin.accounts.fingerprint.evidenceNote') }}</p>
        <details v-for="transport in data.transports" :key="transport.tls.Name" class="border-b border-gray-200 pb-3 dark:border-dark-600" :open="transport.selected_by_current_http">
          <summary class="cursor-pointer break-all text-sm font-medium">{{ transport.protocol }} · {{ transport.tls.Name }} <span v-if="transport.selected_by_current_http" class="text-xs text-primary-600">{{ t('admin.accounts.fingerprint.selected') }}</span></summary>
          <dl class="mt-3 space-y-2 break-words text-xs">
            <div><dt class="text-gray-500">TLS version / ALPN</dt><dd class="font-mono">{{ tlsVersion(transport.tls.MinVersion) }}–{{ tlsVersion(transport.tls.MaxVersion) }} / {{ transport.tls.ALPNProtocols?.join(', ') || t('admin.accounts.fingerprint.none') }}</dd></div>
            <div><dt class="text-gray-500">Cipher suites</dt><dd class="font-mono">{{ transport.tls.CipherSuites.join(', ') }}</dd></div>
            <div><dt class="text-gray-500">Extensions</dt><dd class="font-mono">{{ transport.tls.Extensions.join(', ') }} · shuffle={{ transport.tls.ShuffleExtensions }}</dd></div>
            <div><dt class="text-gray-500">Groups / key shares</dt><dd class="font-mono">{{ transport.tls.Curves.join(', ') }} / {{ transport.tls.KeyShareGroups?.join(', ') || t('admin.accounts.fingerprint.none') }}</dd></div>
            <div><dt class="text-gray-500">Signature algorithms</dt><dd class="font-mono">{{ transport.tls.SignatureAlgorithms.join(', ') }}</dd></div>
            <div><dt class="text-gray-500">Supported versions / PSK modes / point formats / GREASE</dt><dd class="font-mono">{{ transport.tls.SupportedVersions?.map(tlsVersion).join(', ') || t('admin.accounts.fingerprint.none') }} / {{ transport.tls.PSKModes?.join(', ') || t('admin.accounts.fingerprint.none') }} / {{ transport.tls.PointFormats.join(', ') }} / {{ transport.tls.EnableGREASE }}</dd></div>
            <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.headerOrder') }}</dt><dd class="font-mono">{{ transport.header_order.join(' → ') }}</dd></div>
            <template v-if="transport.http2">
              <div><dt class="text-gray-500">HTTP/2 SETTINGS</dt><dd class="font-mono">{{ transport.http2.settings.map(([id, value]) => `${id}=${value}`).join(', ') }}</dd></div>
              <div><dt class="text-gray-500">WINDOW_UPDATE / pseudo headers</dt><dd class="font-mono">{{ transport.http2.connection_window_update }} / {{ transport.http2.pseudo_header_order.join(' → ') }}</dd></div>
            </template>
            <div v-if="transport.compression"><dt class="text-gray-500">WebSocket compression</dt><dd>{{ transport.compression }}</dd></div>
            <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.configHash') }}</dt><dd class="break-all font-mono">{{ transport.configuration_hash }}</dd></div>
          </dl>
        </details>
      </section>
      <p v-if="!data.convergence_ready" class="text-sm text-amber-600 dark:text-amber-400" role="status">{{ t('admin.accounts.fingerprint.notReady') }}</p>
      <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('admin.accounts.fingerprint.transportNote') }}</p>
    </div>
    <template #footer>
      <button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
      <button class="btn btn-primary" @click="emit('close')">{{ t('common.close') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { getFingerprint, type AccountFingerprint } from '@/api/admin/accounts'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{ show: boolean; account: { id: number; name: string } | null }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const data = ref<AccountFingerprint | null>(null)
const loading = ref(false)
const failed = ref(false)
let request: AbortController | undefined

function tlsVersion(value: number) { return ({ 771: 'TLS 1.2', 772: 'TLS 1.3' } as Record<number, string>)[value] || `0x${value.toString(16)}` }

async function load() {
  request?.abort()
  data.value = null
  failed.value = false
  loading.value = false
  if (!props.show || !props.account) return
  const current = new AbortController()
  request = current
  loading.value = true
  try {
    const result = await getFingerprint(props.account.id, current.signal)
    if (!current.signal.aborted) data.value = result
  } catch {
    if (!current.signal.aborted) failed.value = true
  } finally {
    if (!current.signal.aborted) loading.value = false
  }
}

watch(() => [props.show, props.account?.id], load, { immediate: true })
onBeforeUnmount(() => request?.abort())
</script>
