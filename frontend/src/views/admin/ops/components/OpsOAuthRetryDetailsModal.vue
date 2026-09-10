<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { opsAPI, type OpsSystemLog } from '@/api/admin/ops'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { retryEventLabel, retryReasonLabel, retryLogQuery, retryLogTime, retryLogValue, type OAuthRetryLogFilters } from '../utils/oauthRetryLogs'

const props = defineProps<OAuthRetryLogFilters & { show: boolean }>()
defineEmits<{ (event: 'close'): void }>()
const logs = ref<OpsSystemLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const requestId = ref('')
const appliedRequestId = ref('')
const loading = ref(false)
const failed = ref(false)
let generation = 0

async function fetchLogs() {
  if (!props.show) return
  const current = ++generation
  loading.value = true
  failed.value = false
  try {
    const result = await opsAPI.listSystemLogs({
      ...retryLogQuery(props), page: page.value, page_size: pageSize.value,
      request_id: appliedRequestId.value || undefined
    })
    if (current !== generation) return
    logs.value = result.items || []
    total.value = result.total || 0
  } catch {
    if (current === generation) {
      failed.value = true
      logs.value = []
      total.value = 0
    }
  } finally {
    if (current === generation) loading.value = false
  }
}

function search() {
  appliedRequestId.value = requestId.value.trim()
  page.value = 1
  void fetchLogs()
}

function changePage(value: number) {
  page.value = value
  void fetchLogs()
}

function changePageSize(value: number) {
  pageSize.value = value
  page.value = 1
  void fetchLogs()
}

watch(() => [props.show, props.timeRange, props.customStartTime, props.customEndTime, props.platformFilter], () => {
  generation++
  page.value = 1
  void fetchLogs()
})
watch(() => props.refreshToken, fetchLogs)
onUnmounted(() => { generation++ })
</script>

<template>
  <BaseDialog :show="show" title="OAuth 重试拦截日志" width="extra-wide" @close="$emit('close')">
    <form class="mb-4 flex flex-wrap items-end gap-2" @submit.prevent="search">
      <label class="min-w-0 flex-1 text-xs text-gray-600 dark:text-gray-300">
        请求 ID
        <input v-model="requestId" type="text" class="input mt-1" placeholder="request_id" />
      </label>
      <button type="submit" class="btn btn-primary" :disabled="loading">查询</button>
      <button type="button" class="btn btn-secondary" title="刷新重试日志" aria-label="刷新重试日志" :disabled="loading" @click="fetchLogs">
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
      </button>
    </form>
    <div v-if="loading" role="status" class="py-8 text-center text-sm text-gray-500">加载中…</div>
    <div v-else-if="failed" role="alert" class="py-8 text-center text-sm text-red-600 dark:text-red-400">重试日志加载失败，请刷新重试。</div>
    <div v-else-if="!logs.length" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">当前筛选范围内暂无重试日志</div>
    <div v-else class="divide-y divide-gray-200 dark:divide-dark-700">
      <details v-for="row in logs" :key="row.id" class="py-3 text-sm text-gray-700 dark:text-gray-300">
        <summary class="cursor-pointer break-words rounded py-1 focus-visible:outline focus-visible:outline-primary-500">
          <span class="mr-2 font-medium">{{ retryEventLabel(row.extra?.event) }}</span>
          <span class="mr-2 font-mono">{{ retryLogValue(row.extra?.status) }}</span>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ retryLogTime(row.created_at) }}</span>
          <span class="mt-1 block break-all font-mono text-xs">{{ row.request_id || '-' }}</span>
        </summary>
        <dl class="mt-3 grid grid-cols-1 gap-x-6 gap-y-3 text-xs sm:grid-cols-2">
          <div><dt class="text-gray-500">时间</dt><dd class="mt-1 break-all">{{ retryLogTime(row.created_at) }}</dd></div>
          <div><dt class="text-gray-500">节点</dt><dd class="mt-1 break-all">{{ row.host || '-' }}</dd></div>
          <div><dt class="text-gray-500">请求 ID</dt><dd class="mt-1 break-all font-mono">{{ row.request_id || '-' }}</dd></div>
          <div><dt class="text-gray-500">账号 ID</dt><dd class="mt-1">{{ row.account_id ?? '-' }}</dd></div>
          <div class="sm:col-span-2"><dt class="text-gray-500">重试链 ID</dt><dd class="mt-1 break-all font-mono">{{ retryLogValue(row.extra?.retry_id) }}</dd></div>
          <div><dt class="text-gray-500">事件</dt><dd class="mt-1 break-all">{{ retryEventLabel(row.extra?.event) }} ({{ retryLogValue(row.extra?.event) }})</dd></div>
          <div><dt class="text-gray-500">状态码</dt><dd class="mt-1">{{ retryLogValue(row.extra?.status) }}</dd></div>
          <div><dt class="text-gray-500">重试序号 / 最大额外重试次数</dt><dd class="mt-1">{{ retryLogValue(row.extra?.retry) }} / {{ retryLogValue(row.extra?.max_retries) }}</dd></div>
          <div><dt class="text-gray-500">来源</dt><dd class="mt-1 break-all">{{ retryLogValue(row.extra?.source) }}</dd></div>
          <div class="sm:col-span-2"><dt class="text-gray-500">原因</dt><dd class="mt-1 break-all">{{ retryReasonLabel(row.extra?.reason) }}</dd></div>
        </dl>
        <p v-if="row.extra?.event === 'response_received'" class="mt-3 text-xs text-amber-700 dark:text-amber-400">仅确认收到响应头，不代表流完整成功或扣费成功。</p>
      </details>
    </div>
    <Pagination v-if="!failed" :total="total" :page="page" :page-size="pageSize" @update:page="changePage" @update:page-size="changePageSize" />
  </BaseDialog>
</template>
