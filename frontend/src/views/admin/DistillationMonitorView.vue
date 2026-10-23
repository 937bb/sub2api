<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.distillationMonitor.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.description') }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="refreshing" @click="refreshRuntime(false)">
              <Icon name="refresh" size="sm" :class="refreshing ? 'animate-spin' : ''" />
              {{ t('admin.distillationMonitor.refresh') }}
            </button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" @click="saveConfig">
              <Icon name="check" size="sm" />
              {{ saving ? t('common.saving') : t('admin.distillationMonitor.save') }}
            </button>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-8">
          <div v-for="metric in runtimeMetrics" :key="metric.key" class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ metric.label }}</p>
            <p class="mt-1 truncate text-xl font-semibold" :class="metric.valueClass">{{ metric.value }}</p>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(360px,0.9fr)]">
          <section class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.distillationMonitor.settings') }}</h2>
                  <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.settingsHint') }}</p>
                </div>
                <Toggle v-model="form.enabled" data-test="distillation-enabled" />
              </div>
            </div>

            <div class="grid grid-cols-1 gap-5 p-6 md:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.distillationMonitor.mode') }}</label>
                <Select v-model="form.mode" :options="modeOptions" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.distillationMonitor.windowMinutes') }}</label>
                <input v-model.number="form.window_minutes" data-test="distillation-window" type="number" min="5" max="60" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.distillationMonitor.minRequests') }}</label>
                <input v-model.number="form.min_requests" type="number" min="10" max="1000000" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.distillationMonitor.minOutputTokens') }}</label>
                <input v-model.number="form.min_output_tokens" type="number" min="1000" max="10000000000" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.distillationMonitor.observeThreshold') }}</label>
                <input v-model.number="form.observe_threshold" type="number" min="1" max="100" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.distillationMonitor.riskThreshold') }}</label>
                <input v-model.number="form.risk_threshold" type="number" min="1" max="100" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.distillationMonitor.consecutiveWindows') }}</label>
                <input v-model.number="form.consecutive_windows" type="number" min="1" max="12" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.distillationMonitor.minEvidenceClasses') }}</label>
                <input v-model.number="form.min_evidence_classes" type="number" min="2" max="5" class="input" />
              </div>
            </div>
          </section>

          <section class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.distillationMonitor.groups') }}</h2>
                  <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.groupsHint') }}</p>
                </div>
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.selectedGroups', { count: form.group_ids.length }) }}</span>
              </div>
              <div class="relative mt-4">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input v-model.trim="groupSearch" type="search" class="input pl-9" :placeholder="t('admin.distillationMonitor.searchGroups')" />
              </div>
            </div>
            <div class="grid max-h-[390px] grid-cols-1 gap-2 overflow-y-auto p-4 sm:grid-cols-2 xl:grid-cols-1">
              <button
                v-for="group in filteredGroups"
                :key="group.id"
                type="button"
                class="flex min-h-11 items-center justify-between gap-3 rounded-md border px-3 py-2 text-left text-sm transition-colors"
                :class="isGroupSelected(group.id)
                  ? 'border-primary-300 bg-primary-50 text-primary-900 dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
                  : 'border-gray-100 text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:text-gray-200 dark:hover:bg-dark-700/60'"
                @click="toggleGroup(group.id)"
              >
                <span class="min-w-0">
                  <span class="block truncate font-medium">{{ group.name }}</span>
                  <span class="block truncate text-xs text-gray-400">{{ group.platform }}</span>
                </span>
                <Icon v-if="isGroupSelected(group.id)" name="check" size="xs" class="flex-shrink-0" />
              </button>
              <p v-if="filteredGroups.length === 0" class="px-2 py-6 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.noGroups') }}</p>
            </div>
          </section>
        </div>

        <section class="card">
          <div class="flex flex-col gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.distillationMonitor.records') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.recordsHint') }}</p>
            </div>
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="logsLoading" @click="loadLogs">
              <Icon name="refresh" size="sm" :class="logsLoading ? 'animate-spin' : ''" />
              {{ t('admin.distillationMonitor.refresh') }}
            </button>
          </div>

          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.table.time') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.table.group') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.table.user') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.table.model') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.table.score') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.table.action') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                <tr v-if="logsLoading">
                  <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td>
                </tr>
                <tr v-else-if="logs.length === 0">
                  <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.distillationMonitor.emptyRecords') }}</td>
                </tr>
                <template v-else>
                  <tr v-for="row in logs" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.group_name || '-' }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.user_email || '-' }}</div>
                      <div v-if="row.user_id" class="text-xs text-gray-400">UID {{ row.user_id }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 font-mono text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.dominant_model || '-' }}</div>
                      <div class="text-xs text-gray-400">{{ formatNumber(row.metrics?.requests ?? 0) }} req / {{ formatNumber(row.metrics?.output_tokens ?? 0) }} out</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm font-semibold" :class="assessmentStyle(row).scoreClass">
                      <div>{{ formatScore(row.score) }}</div>
                      <div class="text-xs font-normal text-gray-400">{{ row.evidence_classes }} {{ t('admin.distillationMonitor.evidenceClasses') }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4">
                      <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="assessmentStyle(row).badgeClass">
                        {{ assessmentStyle(row).label }}
                      </span>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>

          <Pagination
            v-if="pagination.total > 0"
            :page="pagination.page"
            :total="pagination.total"
            :page-size="pagination.page_size"
            @update:page="onPageChange"
            @update:pageSize="onPageSizeChange"
          />
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI } from '@/api/admin'
import type {
  DistillationAssessment,
  DistillationMonitorConfig,
  DistillationMonitorMode,
  DistillationMonitorRuntimeStatus,
} from '@/api/admin/riskControl'
import type { AdminGroup, SelectOption } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDateTimeValue } from '@/utils/format'

const defaultConfig = (): DistillationMonitorConfig => ({
  enabled: false,
  mode: 'observe',
  group_ids: [],
  window_minutes: 15,
  min_requests: 100,
  min_output_tokens: 300000,
  observe_threshold: 60,
  risk_threshold: 85,
  consecutive_windows: 3,
  min_evidence_classes: 3,
  feature_retention_days: 7,
  assessment_retention_days: 180,
})

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const refreshing = ref(false)
const logsLoading = ref(false)
const groupSearch = ref('')
const groups = ref<AdminGroup[]>([])
const logs = ref<DistillationAssessment[]>([])
const runtime = ref<DistillationMonitorRuntimeStatus | null>(null)
const form = reactive<DistillationMonitorConfig>(defaultConfig())
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
let refreshTimer: number | null = null

const modeOptions = computed<SelectOption[]>(() => [
  { value: 'observe', label: t('admin.distillationMonitor.modeObserve') },
  { value: 'enforce', label: t('admin.distillationMonitor.modeEnforce') },
  { value: 'off', label: t('admin.distillationMonitor.modeOff') },
])

const filteredGroups = computed(() => {
  const keyword = groupSearch.value.trim().toLowerCase()
  if (!keyword) return groups.value
  return groups.value.filter(group => group.name.toLowerCase().includes(keyword) || String(group.platform).toLowerCase().includes(keyword))
})

const runtimeMetrics = computed(() => [
  { key: 'status', label: t('admin.distillationMonitor.status'), value: runtimeActive.value ? t('admin.distillationMonitor.running') : t('admin.distillationMonitor.stopped'), valueClass: runtimeActive.value ? 'text-emerald-700 dark:text-emerald-300' : 'text-gray-700 dark:text-gray-300' },
  { key: 'queue', label: t('admin.distillationMonitor.queue'), value: `${runtime.value?.queue_length ?? 0} / ${runtime.value?.queue_size ?? 0}`, valueClass: 'text-gray-900 dark:text-white' },
  { key: 'enqueued', label: t('admin.distillationMonitor.enqueued'), value: formatNumber(runtime.value?.enqueued ?? 0), valueClass: 'text-gray-900 dark:text-white' },
  { key: 'processed', label: t('admin.distillationMonitor.processed'), value: formatNumber(runtime.value?.processed ?? 0), valueClass: 'text-gray-900 dark:text-white' },
  { key: 'dropped', label: t('admin.distillationMonitor.dropped'), value: formatNumber(runtime.value?.dropped ?? 0), valueClass: 'text-amber-700 dark:text-amber-300' },
  { key: 'errors', label: t('admin.distillationMonitor.errors'), value: formatNumber(runtime.value?.errors ?? 0), valueClass: 'text-rose-700 dark:text-rose-300' },
  { key: 'flagged', label: t('admin.distillationMonitor.lastFlagged'), value: formatNumber(runtime.value?.last_flagged ?? 0), valueClass: 'text-amber-700 dark:text-amber-300' },
  { key: 'disabled', label: t('admin.distillationMonitor.lastDisabled'), value: formatNumber(runtime.value?.last_disabled ?? 0), valueClass: 'text-rose-700 dark:text-rose-300' },
])

const runtimeActive = computed(() => Boolean(runtime.value?.enabled && runtime.value.mode !== 'off'))

function normalizeConfig(value?: Partial<DistillationMonitorConfig>): DistillationMonitorConfig {
  const defaults = defaultConfig()
  const mode: DistillationMonitorMode = value?.mode === 'enforce' || value?.mode === 'off' ? value.mode : 'observe'
  return {
    enabled: value?.enabled ?? defaults.enabled,
    mode,
    group_ids: Array.isArray(value?.group_ids) ? [...new Set(value.group_ids.filter(id => Number.isInteger(id) && id > 0))] : [],
    window_minutes: Number(value?.window_minutes) || defaults.window_minutes,
    min_requests: Number(value?.min_requests) || defaults.min_requests,
    min_output_tokens: Number(value?.min_output_tokens) || defaults.min_output_tokens,
    observe_threshold: Number(value?.observe_threshold) || defaults.observe_threshold,
    risk_threshold: Number(value?.risk_threshold) || defaults.risk_threshold,
    consecutive_windows: Number(value?.consecutive_windows) || defaults.consecutive_windows,
    min_evidence_classes: Number(value?.min_evidence_classes) || defaults.min_evidence_classes,
    feature_retention_days: Number(value?.feature_retention_days) || defaults.feature_retention_days,
    assessment_retention_days: Number(value?.assessment_retention_days) || defaults.assessment_retention_days,
  }
}

function applyConfig(value?: Partial<DistillationMonitorConfig>) {
  Object.assign(form, normalizeConfig(value))
}

async function loadAll() {
  loading.value = true
  try {
    const [config, groupItems, status] = await Promise.all([
      adminAPI.riskControl.getConfig(),
      adminAPI.groups.getAll(),
      adminAPI.riskControl.getStatus(),
    ])
    applyConfig(config.distillation_monitor)
    groups.value = groupItems
    runtime.value = status.distillation
    await loadLogs()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.distillationMonitor.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function refreshRuntime(showError = true) {
  refreshing.value = true
  try {
    const status = await adminAPI.riskControl.getStatus()
    runtime.value = status.distillation
  } catch (err: unknown) {
    if (showError) appStore.showError(extractApiErrorMessage(err, t('admin.distillationMonitor.statusFailed')))
  } finally {
    refreshing.value = false
  }
}

async function loadLogs() {
  logsLoading.value = true
  try {
    const result = await adminAPI.riskControl.listDistillationAssessments({
      page: pagination.page,
      page_size: pagination.page_size,
    })
    logs.value = result.items
    pagination.total = result.total
    pagination.page = result.page
    pagination.page_size = result.page_size
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.distillationMonitor.logsFailed')))
  } finally {
    logsLoading.value = false
  }
}

async function saveConfig() {
  const payload = normalizeConfig(form)
  if (payload.enabled && payload.group_ids.length === 0) {
    appStore.showError(t('admin.distillationMonitor.groupsRequired'))
    return
  }
  if (payload.observe_threshold > payload.risk_threshold) {
    appStore.showError(t('admin.distillationMonitor.thresholdOrderInvalid'))
    return
  }
  saving.value = true
  try {
    const updated = await adminAPI.riskControl.updateConfig({ distillation_monitor: payload })
    applyConfig(updated.distillation_monitor)
    appStore.showSuccess(t('admin.distillationMonitor.saved'))
    await refreshRuntime(false)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.distillationMonitor.saveFailed')))
  } finally {
    saving.value = false
  }
}

function toggleGroup(groupID: number) {
  const index = form.group_ids.indexOf(groupID)
  if (index >= 0) form.group_ids.splice(index, 1)
  else form.group_ids.push(groupID)
}

function isGroupSelected(groupID: number): boolean {
  return form.group_ids.includes(groupID)
}

function onPageChange(page: number) {
  pagination.page = page
  void loadLogs()
}

function onPageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  void loadLogs()
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value)
}

function formatDateTime(value: string): string {
  return formatDateTimeValue(value) || '-'
}

function formatScore(value: number): string {
  return `${Math.round(Math.max(0, value))} / 100`
}

function assessmentStyle(row: DistillationAssessment) {
  if (row.auto_disabled) {
    return {
      label: t('admin.distillationMonitor.actionDisabled'),
      badgeClass: 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300',
      scoreClass: 'text-rose-700 dark:text-rose-300',
    }
  }
  if (row.score >= form.risk_threshold && row.evidence_classes >= form.min_evidence_classes) {
    return {
      label: t('admin.distillationMonitor.actionHighRisk'),
      badgeClass: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
      scoreClass: 'text-orange-700 dark:text-orange-300',
    }
  }
  if (row.score >= form.observe_threshold) {
    return {
      label: t('admin.distillationMonitor.actionFlagged'),
      badgeClass: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
      scoreClass: 'text-amber-700 dark:text-amber-300',
    }
  }
  return {
    label: t('admin.distillationMonitor.actionEvaluated'),
    badgeClass: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
    scoreClass: 'text-gray-700 dark:text-gray-300',
  }
}

onMounted(() => {
  void loadAll()
  refreshTimer = window.setInterval(() => void refreshRuntime(false), 15000)
})

onUnmounted(() => {
  if (refreshTimer !== null) window.clearInterval(refreshTimer)
})
</script>
