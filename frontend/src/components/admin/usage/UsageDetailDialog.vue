<template>
  <BaseDialog
    :show="show"
    :title="t('admin.usage.requestDetail')"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="loading" class="flex min-h-56 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="error" class="flex min-h-56 flex-col items-center justify-center gap-3 text-center">
      <span class="text-sm text-red-600 dark:text-red-400">{{ error }}</span>
      <button type="button" class="btn btn-secondary" @click="loadDetail">
        <Icon name="refresh" size="sm" />
        {{ t('admin.usage.retry') }}
      </button>
    </div>

    <div v-else-if="detail" class="divide-y divide-gray-200 dark:divide-dark-700">
      <section class="pb-5">
        <div class="mb-3 flex flex-wrap items-center gap-2">
          <span
            class="inline-flex items-center rounded px-2 py-1 text-xs font-semibold"
            :class="detail.quota_bypass_applied
              ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-200'
              : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-200'"
          >
            {{ detail.quota_bypass_applied
              ? t('admin.usage.quotaBypassApplied', { count: detail.quota_bypass_inject_pairs || 0 })
              : t('admin.usage.regularRequest') }}
          </span>
          <span class="text-xs text-gray-500 dark:text-gray-400">#{{ detail.id }}</span>
        </div>
        <dl class="grid gap-x-6 gap-y-3 sm:grid-cols-2">
          <DetailItem :label="t('admin.usage.requestId')" :value="detail.request_id || '-'" mono />
          <DetailItem :label="t('usage.time')" :value="formatDateTime(detail.created_at)" />
          <DetailItem :label="t('admin.usage.user')" :value="detail.user?.email || `#${detail.user_id}`" />
          <DetailItem :label="t('usage.apiKeyFilter')" :value="detail.api_key?.name || `#${detail.api_key_id}`" />
          <DetailItem :label="t('admin.usage.account')" :value="detail.account?.name || `#${detail.account_id}`" />
          <DetailItem :label="t('admin.usage.group')" :value="detail.group?.name || '-'" />
        </dl>
      </section>

      <section class="py-5">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.usage.routingDetail') }}</h4>
        <dl class="grid gap-x-6 gap-y-3 sm:grid-cols-2">
          <DetailItem :label="t('usage.model')" :value="detail.model || '-'" mono />
          <DetailItem :label="t('admin.usage.upstreamModel')" :value="detail.upstream_model || detail.model || '-'" mono />
          <DetailItem :label="t('usage.inbound')" :value="detail.inbound_endpoint || '-'" mono />
          <DetailItem :label="t('usage.upstream')" :value="detail.upstream_endpoint || '-'" mono />
          <DetailItem :label="t('usage.type')" :value="requestTypeLabel" />
          <DetailItem :label="t('usage.reasoningEffort')" :value="formatReasoningEffort(detail.reasoning_effort)" />
        </dl>
      </section>

      <section class="py-5">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.usage.usageDetail') }}</h4>
        <dl class="grid gap-x-6 gap-y-3 sm:grid-cols-2">
          <DetailItem :label="t('admin.usage.inputTokens')" :value="detail.input_tokens.toLocaleString()" />
          <DetailItem :label="t('admin.usage.outputTokens')" :value="detail.output_tokens.toLocaleString()" />
          <DetailItem :label="t('usage.cost')" :value="`$${detail.actual_cost.toFixed(6)}`" />
          <DetailItem :label="t('usage.latency')" :value="detail.duration_ms == null ? '-' : `${detail.duration_ms}ms`" />
          <DetailItem :label="t('admin.usage.quotaBypassStatus')" :value="detail.quota_bypass_applied ? t('common.yes') : t('common.no')" />
          <DetailItem :label="t('admin.usage.quotaBypassPairs')" :value="detail.quota_bypass_applied ? String(detail.quota_bypass_inject_pairs || 0) : '-'" />
        </dl>
      </section>

      <section class="pt-5">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.usage.clientDetail') }}</h4>
        <dl class="grid gap-x-6 gap-y-3 sm:grid-cols-2">
          <DetailItem :label="t('admin.usage.ipAddress')" :value="detail.ip_address || '-'" mono />
          <DetailItem :label="t('admin.usage.sessionId')" :value="detail.session_id || '-'" mono />
          <div class="sm:col-span-2">
            <DetailItem :label="t('usage.userAgent')" :value="detail.user_agent || '-'" mono />
          </div>
        </dl>
      </section>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('close')">
        {{ t('common.close') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminUsageAPI } from '@/api/admin/usage'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DetailItem from '@/components/common/DetailItem.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime, formatReasoningEffort } from '@/utils/format'
import { resolveUsageRequestType } from '@/utils/usageRequestType'
import type { AdminUsageLog } from '@/types'

const props = defineProps<{
  show: boolean
  usageId: number | null
  summary?: AdminUsageLog | null
}>()

const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const loading = ref(false)
const error = ref('')
const detail = ref<AdminUsageLog | null>(null)
let requestSequence = 0

const requestTypeLabel = computed(() => {
  if (!detail.value) return '-'
  const type = resolveUsageRequestType(detail.value)
  if (type === 'cyber') return t('usage.cyber')
  if (type === 'live') return t('usage.live')
  if (type === 'ws_v2') return t('usage.ws')
  if (type === 'stream') return t('usage.stream')
  if (type === 'sync') return t('usage.sync')
  return t('usage.unknown')
})

const loadDetail = async () => {
  if (!props.show || !props.usageId) return
  const sequence = ++requestSequence
  loading.value = true
  error.value = ''
  try {
    const loaded = await adminUsageAPI.getById(props.usageId)
    if (sequence === requestSequence) {
      detail.value = props.summary ? { ...props.summary, ...loaded } : loaded
    }
  } catch (err: any) {
    if (sequence === requestSequence) {
      error.value = err?.response?.data?.message || err?.message || t('admin.usage.failedToLoadDetail')
    }
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

watch(
  () => [props.show, props.usageId] as const,
  ([show, usageId]) => {
    if (!show || !usageId) {
      requestSequence++
      detail.value = null
      error.value = ''
      loading.value = false
      return
    }
    void loadDetail()
  },
  { immediate: true }
)
</script>
