<template>
  <BaseDialog
    :show="show"
    :title="t('admin.usage.detail.title')"
    width="extra-wide"
    @close="emit('update:show', false)"
  >
    <div v-if="log" data-testid="usage-detail-dialog" class="space-y-6 text-sm">
      <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div class="min-w-0">
          <p class="text-xs font-medium uppercase text-gray-400 dark:text-gray-500">
            {{ t('admin.usage.detail.logId') }} #{{ log.id }}
          </p>
          <p class="mt-1 break-all font-mono text-sm font-semibold text-gray-900 dark:text-white">
            {{ log.request_id || '-' }}
          </p>
        </div>
        <div class="text-right text-xs text-gray-500 dark:text-gray-400">
          <p>{{ formatDateTime(log.created_at) }}</p>
          <p class="mt-1">{{ requestTypeLabel }} / {{ billingModeLabel }}</p>
        </div>
      </div>

      <section>
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.usage.detail.request') }}
        </h4>
        <dl class="grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
          <DetailItem :label="t('admin.usage.user')" :value="userLabel" />
          <DetailItem :label="t('usage.apiKeyFilter')" :value="log.api_key?.name || '-'" />
          <DetailItem :label="t('admin.usage.account')" :value="accountLabel" />
          <DetailItem :label="t('admin.usage.group')" :value="log.group?.name || '-'" />
          <DetailItem :label="t('usage.requestedModel')" :value="log.model || '-'" mono />
          <DetailItem :label="t('usage.sentUpstreamModel')" :value="log.upstream_model || log.model || '-'" mono />
          <DetailItem :label="t('usage.upstreamResponseModel')" :value="log.upstream_response_model || '-'" mono />
          <DetailItem :label="t('usage.requestedReasoningEffort')" :value="formatReasoningEffort(log.reasoning_effort)" />
          <DetailItem :label="t('usage.reasoningEffort')" :value="formatReasoningEffort(log.upstream_reasoning_effort || log.reasoning_effort)" />
          <DetailItem :label="t('usage.serviceTier')" :value="getUsageServiceTierLabel(log.service_tier, t)" />
          <DetailItem :label="t('usage.inboundEndpoint')" :value="log.inbound_endpoint || '-'" mono />
          <DetailItem :label="t('usage.upstreamEndpoint')" :value="log.upstream_endpoint || '-'" mono />
          <DetailItem :label="t('admin.usage.billingType')" :value="billingTypeLabel" />
          <DetailItem :label="t('admin.usage.billingMode')" :value="billingModeLabel" />
          <DetailItem :label="t('usage.type')" :value="requestTypeLabel" />
        </dl>
      </section>

      <section class="border-t border-gray-200 pt-5 dark:border-dark-700">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.usage.detail.tokensAndBilling') }}
        </h4>
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <dl class="grid grid-cols-2 gap-x-6 gap-y-3">
            <DetailItem :label="t('admin.usage.inputTokens')" :value="formatNumber(log.input_tokens)" />
            <DetailItem :label="t('admin.usage.outputTokens')" :value="formatNumber(log.output_tokens)" />
            <DetailItem :label="t('admin.usage.cacheCreationTokens')" :value="formatNumber(log.cache_creation_tokens)" />
            <DetailItem :label="t('admin.usage.cacheReadTokens')" :value="formatNumber(log.cache_read_tokens)" />
            <DetailItem :label="t('admin.usage.detail.cacheCreation5m')" :value="formatNumber(log.cache_creation_5m_tokens)" />
            <DetailItem :label="t('admin.usage.detail.cacheCreation1h')" :value="formatNumber(log.cache_creation_1h_tokens)" />
            <DetailItem :label="t('usage.imageInputTokens')" :value="formatNumber(log.image_input_tokens)" />
            <DetailItem :label="t('usage.imageOutputTokens')" :value="formatNumber(log.image_output_tokens)" />
            <DetailItem :label="t('usage.totalTokens')" :value="formatNumber(totalTokens)" strong />
            <DetailItem :label="t('usage.imageCount')" :value="formatNumber(log.image_count)" />
          </dl>
          <dl class="grid grid-cols-2 gap-x-6 gap-y-3">
            <DetailItem :label="t('admin.usage.inputCost')" :value="formatCost(log.input_cost)" />
            <DetailItem :label="t('admin.usage.outputCost')" :value="formatCost(log.output_cost)" />
            <DetailItem :label="t('admin.usage.cacheCreationCost')" :value="formatCost(log.cache_creation_cost)" />
            <DetailItem :label="t('admin.usage.cacheReadCost')" :value="formatCost(log.cache_read_cost)" />
            <DetailItem :label="t('usage.original')" :value="formatCost(log.total_cost)" />
            <DetailItem :label="t('usage.userBilled')" :value="formatCost(log.actual_cost)" strong />
            <DetailItem :label="t('usage.rate')" :value="formatMultiplier(log.rate_multiplier)" />
            <DetailItem :label="t('usage.accountMultiplier')" :value="formatMultiplier(log.account_rate_multiplier ?? 1)" />
            <DetailItem :label="t('usage.accountBilled')" :value="formatCost(accountBilled)" />
            <DetailItem
              :label="t('admin.usage.longContext')"
              :value="log.long_context_billing_applied ? t('common.yes') : t('common.no')"
            />
          </dl>
        </div>
      </section>

      <section class="border-t border-gray-200 pt-5 dark:border-dark-700">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.usage.detail.performanceAndClient') }}
        </h4>
        <dl class="grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
          <DetailItem :label="t('usage.firstToken')" :value="formatDuration(log.first_token_ms)" />
          <DetailItem :label="t('usage.duration')" :value="formatDuration(log.duration_ms)" />
          <DetailItem :label="t('usage.outputTps')" :value="formatUsageOutputRate(log)" />
          <DetailItem :label="t('admin.usage.ipAddress')" :value="log.ip_address || '-'" mono />
          <DetailItem :label="t('usage.userAgent')" :value="log.user_agent || '-'" class="sm:col-span-2" />
        </dl>
      </section>

      <section class="border-t border-gray-200 pt-5 dark:border-dark-700">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.usage.detail.trace') }}
        </h4>
        <div class="divide-y divide-gray-100 rounded border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
          <div
            v-for="item in traceItems"
            :key="item.label"
            class="grid grid-cols-[minmax(120px,180px)_minmax(0,1fr)_32px] items-center gap-3 px-3 py-2.5"
          >
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</span>
            <span class="break-all font-mono text-xs text-gray-800 dark:text-gray-200">{{ item.value || '-' }}</span>
            <button
              v-if="item.value"
              type="button"
              class="flex h-7 w-7 items-center justify-center rounded text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
              :title="t('keys.copyToClipboard')"
              @click="copyTrace(item.value)"
            >
              <Icon :name="copiedValue === item.value ? 'check' : 'copy'" size="sm" />
            </button>
            <span v-else></span>
          </div>
        </div>
        <p class="mt-3 text-xs text-gray-400 dark:text-gray-500">
          {{ t('admin.usage.detail.privacyNote') }}
        </p>
      </section>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="emit('update:show', false)">
        {{ t('common.close') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DetailItem from './UsageDetailItem.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import type { AdminUsageLog } from '@/types'
import { getBillingModeLabel, getDisplayBillingMode } from '@/utils/billingMode'
import { formatDateTime, formatReasoningEffort } from '@/utils/format'
import { formatMultiplier } from '@/utils/formatters'
import { formatUsageOutputRate } from '@/utils/latencyHealth'
import { getUsageServiceTierLabel } from '@/utils/usageServiceTier'
import { resolveUsageRequestType } from '@/utils/usageRequestType'

const props = defineProps<{
  show: boolean
  log: AdminUsageLog | null
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const copiedValue = ref<string | null>(null)

const formatNumber = (value: number | null | undefined): string => (value ?? 0).toLocaleString()
const formatCost = (value: number | null | undefined): string => `$${(value ?? 0).toFixed(8)}`
const formatDuration = (value: number | null | undefined): string => {
  if (value == null) return '-'
  if (value < 1000) return `${value}ms`
  if (value < 60_000) return `${(value / 1000).toFixed(2)}s`
  const seconds = Math.round(value / 1000)
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
}

const userLabel = computed(() => {
  if (!props.log) return '-'
  return props.log.user?.email ? `${props.log.user.email} (#${props.log.user_id})` : `#${props.log.user_id}`
})

const accountLabel = computed(() => {
  if (!props.log?.account_id) return '-'
  return props.log.account?.name ? `${props.log.account.name} (#${props.log.account_id})` : `#${props.log.account_id}`
})

const requestTypeLabel = computed(() => {
  if (!props.log) return '-'
  const type = resolveUsageRequestType(props.log)
  if (type === 'cyber') return t('usage.cyber')
  if (type === 'live') return t('usage.live')
  if (type === 'ws_v2') return t('usage.ws')
  if (type === 'stream') return t('usage.stream')
  if (type === 'sync') return t('usage.sync')
  return t('usage.unknown')
})

const billingModeLabel = computed(() =>
  props.log ? getBillingModeLabel(getDisplayBillingMode(props.log), t) : '-'
)

const billingTypeLabel = computed(() => {
  if (!props.log) return '-'
  return props.log.billing_type === 1
    ? t('admin.usage.billingTypeSubscription')
    : t('admin.usage.billingTypeBalance')
})

const totalTokens = computed(() => {
  if (!props.log) return 0
  return props.log.input_tokens + props.log.output_tokens + props.log.cache_creation_tokens + props.log.cache_read_tokens
})

const accountBilled = computed(() => {
  if (!props.log) return 0
  const base = props.log.account_stats_cost ?? props.log.total_cost
  return base * (props.log.account_rate_multiplier ?? 1)
})

const traceItems = computed(() => {
  const log = props.log
  if (!log) return []
  return [
    { label: t('admin.usage.requestId'), value: log.request_id },
    { label: t('admin.usage.upstreamRequestId'), value: log.upstream_request_id || '' },
    { label: t('admin.usage.upstreamResponseId'), value: log.upstream_response_id || '' },
    { label: t('admin.usage.previousResponseId'), value: log.previous_response_id || '' },
    { label: t('admin.usage.detail.channelId'), value: log.channel_id?.toString() || '' },
    { label: t('admin.usage.detail.accountId'), value: log.account_id?.toString() || '' },
    { label: t('admin.usage.detail.apiKeyId'), value: log.api_key_id?.toString() || '' },
    { label: t('admin.usage.detail.subscriptionId'), value: log.subscription_id?.toString() || '' },
  ]
})

const copyTrace = async (value: string) => {
  try {
    await navigator.clipboard.writeText(value)
    copiedValue.value = value
    appStore.showSuccess(t('admin.usage.traceIdCopied'))
    window.setTimeout(() => {
      if (copiedValue.value === value) copiedValue.value = null
    }, 2000)
  } catch {
    appStore.showError(t('common.copyFailed'))
  }
}
</script>
