<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { opsAPI } from '@/api/admin/ops'
import Icon from '@/components/icons/Icon.vue'
import OpsOAuthRetryDetailsModal from './OpsOAuthRetryDetailsModal.vue'
import { retryLogQuery, type OAuthRetryLogFilters } from '../utils/oauthRetryLogs'

const props = defineProps<OAuthRetryLogFilters>()
const showDetails = ref(false)
const total = ref<number | null>(null)
const loading = ref(false)
const failed = ref(false)
let generation = 0

async function fetchCount() {
  const current = ++generation
  loading.value = true
  failed.value = false
  try {
    const result = await opsAPI.listSystemLogs({ ...retryLogQuery(props), page: 1, page_size: 1 })
    if (current === generation) total.value = result.total
  } catch {
    if (current === generation) {
      failed.value = true
      total.value = null
    }
  } finally {
    if (current === generation) loading.value = false
  }
}

watch(() => [props.timeRange, props.customStartTime, props.customEndTime, props.platformFilter, props.refreshToken], fetchCount, { immediate: true })
onUnmounted(() => { generation++ })
</script>

<template>
  <section class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900/60">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h3 class="text-sm font-bold text-gray-900 dark:text-white">OAuth 重试拦截日志</h3>
      <button type="button" class="btn btn-secondary btn-sm" title="刷新重试日志" aria-label="刷新重试日志" :disabled="loading" @click="fetchCount">
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
      </button>
    </div>
    <button type="button" class="mt-3 flex w-full flex-wrap items-center justify-between gap-3 rounded-lg py-2 text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500" @click="showDetails = true">
      <span class="flex items-baseline gap-2 text-gray-900 dark:text-white">
        <span class="min-w-12 text-3xl font-semibold tabular-nums">{{ loading ? '…' : total?.toLocaleString() ?? '-' }}</span>
        <span class="text-xs text-gray-500 dark:text-gray-400">日志条数</span>
      </span>
      <span class="text-sm text-primary-600 dark:text-primary-400">查看详情</span>
    </button>
    <p v-if="failed" role="alert" class="mt-2 text-xs text-red-600 dark:text-red-400">重试日志加载失败，请刷新重试。</p>
    <OpsOAuthRetryDetailsModal v-bind="props" :show="showDetails" @close="showDetails = false" />
  </section>
</template>
