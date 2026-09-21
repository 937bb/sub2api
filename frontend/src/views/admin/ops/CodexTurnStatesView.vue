<template>
  <AppLayout>
    <TablePageLayout class="codex-state-layout">
      <template #actions>
        <div class="space-y-5">
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div class="flex min-w-0 items-start gap-3">
              <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 text-white shadow-sm dark:bg-white dark:text-gray-900">
                <Icon name="database" size="md" />
              </div>
              <div class="min-w-0">
                <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.turnState.title') }}</h1>
                <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.description') }}</p>
              </div>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <div v-if="summary?.last_scan_at" class="hidden items-center gap-1.5 text-xs text-gray-400 xl:flex">
                <Icon name="clock" size="xs" />
                <span>{{ t('admin.ops.turnState.lastScan') }} {{ formatDateTime(summary.last_scan_at) }}</span>
              </div>
              <button class="btn btn-secondary btn-icon" :disabled="loading" :title="t('common.refresh')" :aria-label="t('common.refresh')" @click="refresh">
                <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
              </button>
              <button v-if="activeTab === 'accounts'" class="btn btn-primary" :disabled="scanningAll" @click="scanAll">
                <Icon name="play" size="sm" class="mr-1.5" />
                {{ t('admin.ops.turnState.scanAll') }}
              </button>
              <button v-if="activeTab === 'proxies'" class="btn btn-primary" @click="showProxyDialog = true">
                <Icon name="plus" size="sm" class="mr-1.5" />
                {{ t('admin.ops.turnState.addProxy') }}
              </button>
              <button class="btn btn-secondary" data-test="state-scan-settings" @click="openScanSettings()">
                {{ t('admin.ops.turnState.scanSettings.title') }}
              </button>
            </div>
          </div>

          <CodexStateRulesPanel ref="stateRulesPanel" @saved="refresh" />

          <div class="grid overflow-hidden rounded-lg border border-gray-200 bg-white grid-cols-2 sm:grid-cols-4 xl:grid-cols-8 dark:border-dark-700 dark:bg-dark-900">
            <div
              v-for="metric in metrics"
              :key="metric.label"
              :class="[
                'relative min-w-0 border-b border-r border-gray-100 px-4 py-3 last:border-r-0 sm:[&:nth-child(4n)]:border-r-0 xl:border-b-0 xl:[&:nth-child(4n)]:border-r xl:[&:nth-child(8n)]:border-r-0 dark:border-dark-700/70',
                metric.accentClass
              ]"
            >
              <div :class="['text-2xl font-semibold tabular-nums leading-none', metric.valueClass]">{{ metric.value }}</div>
              <div class="mt-1.5 truncate text-xs font-medium text-gray-500 dark:text-dark-400" :title="metric.label">{{ metric.label }}</div>
            </div>
          </div>

          <div class="flex border-b border-gray-200 dark:border-dark-700" role="tablist">
            <button
              v-for="tab in tabs"
              :key="tab.value"
              type="button"
              role="tab"
              :aria-selected="activeTab === tab.value"
              :class="[
                '-mb-px inline-flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors',
                activeTab === tab.value
                  ? 'border-primary-500 text-primary-700 dark:border-primary-400 dark:text-primary-300'
                  : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-800 dark:text-dark-400 dark:hover:border-dark-500 dark:hover:text-white'
              ]"
              @click="setTab(tab.value)"
            >
              <Icon :name="tab.icon" size="sm" />
              {{ tab.label }}
            </button>
          </div>
        </div>
      </template>

      <template #filters>
        <div v-if="activeTab === 'history'" class="flex flex-wrap items-center justify-between gap-3">
          <Select v-model="historyStatus" :options="historyStatusOptions" class="w-44" @change="handleHistoryStatusChange" />
          <button v-if="selectedHistoryIDs.length > 0" class="btn btn-danger" @click="requestHistoryDelete(selectedHistoryIDs)">
            <Icon name="trash" size="md" class="mr-1" />
            {{ t('admin.ops.turnState.deleteSelected', { count: selectedHistoryIDs.length }) }}
          </button>
        </div>
        <div v-else class="flex min-h-9 items-center gap-2 text-sm text-gray-500 dark:text-dark-400">
          <Icon name="infoCircle" size="sm" class="shrink-0 text-gray-400" />
          <span>{{ activeTab === 'accounts' ? t('admin.ops.turnState.accountHint') : t('admin.ops.turnState.proxyHint') }}</span>
        </div>
      </template>

      <template #table>
        <DataTable v-if="activeTab === 'accounts'" :columns="accountColumns" :data="accounts" :loading="loading" row-key="row_key">
          <template #cell-account="{ row }">
            <div class="flex min-w-[220px] items-center gap-3">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-gray-100 text-xs font-semibold uppercase text-gray-600 dark:bg-dark-700 dark:text-dark-200">
                {{ accountInitial(row.account_name) }}
              </div>
              <div class="min-w-0">
                <div class="max-w-56 truncate font-medium text-gray-900 dark:text-white" :title="row.account_name">{{ row.account_name }}</div>
                <div class="mt-0.5 flex items-center gap-1.5 text-xs text-gray-500 dark:text-dark-400">
                  <span class="font-mono">#{{ row.account_id }}</span>
                  <span aria-hidden="true">/</span>
                  <span>{{ row.account_type }}</span>
                  <span v-if="row.plan_type" class="font-medium uppercase text-gray-600 dark:text-dark-300">{{ row.plan_type }}</span>
                </div>
              </div>
            </div>
          </template>
          <template #cell-model="{ value }"><span class="inline-flex rounded bg-gray-100 px-2 py-1 font-mono text-xs text-gray-700 dark:bg-dark-700 dark:text-dark-200">{{ value }}</span></template>
          <template #cell-state="{ row }">
            <CodexStateStatus :status="row.status" :state-length="row.state_length" :target-lengths="row.target_lengths" :show-details="true" />
          </template>
          <template #cell-expires_at="{ row }">
            <div v-if="row.expires_at" class="flex items-center gap-2 whitespace-nowrap">
              <Icon name="clock" size="sm" class="shrink-0 text-gray-400" />
              <div>
                <div class="text-xs text-gray-600 dark:text-gray-300">{{ formatDateTime(row.expires_at) }}</div>
                <div class="mt-0.5 font-mono text-[11px] text-gray-400">{{ expiryCountdown(row.expires_at) }}</div>
              </div>
            </div>
            <span v-else class="text-gray-400">-</span>
          </template>
          <template #cell-scan="{ row }">
            <div class="min-w-[130px] text-xs">
              <div class="font-medium tabular-nums text-gray-600 dark:text-gray-300">{{ t('admin.ops.turnState.attempts', { count: row.attempt_count }) }}</div>
              <div v-if="row.last_attempt_at" class="mt-1 whitespace-nowrap text-gray-400">{{ formatDateTime(row.last_attempt_at) }}</div>
              <div v-if="row.last_error" class="mt-1 max-w-64 truncate text-red-500" :title="row.last_error">{{ row.last_error }}</div>
            </div>
          </template>
          <template #cell-proxy="{ row }">
            <div class="flex items-center gap-2">
              <Icon name="globe" size="sm" class="shrink-0 text-gray-400" />
              <span v-if="row.last_proxy_masked" class="max-w-52 truncate font-mono text-xs text-gray-600 dark:text-gray-300" :title="row.last_proxy_masked">{{ row.last_proxy_masked }}</span>
              <span v-else class="text-xs text-gray-400">{{ t('admin.ops.turnState.direct') }}</span>
            </div>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
            <button
              class="flex h-8 w-8 items-center justify-center rounded-md border border-gray-200 text-gray-500 transition-colors hover:border-primary-300 hover:bg-primary-50 hover:text-primary-700 disabled:cursor-wait disabled:opacity-60 dark:border-dark-600 dark:text-dark-300 dark:hover:border-primary-700 dark:hover:bg-primary-950/40 dark:hover:text-primary-300"
              :disabled="scanningKeys.has(accountModelKey(row))"
              :title="t('admin.ops.turnState.scanNow')"
              :aria-label="t('admin.ops.turnState.scanNow')"
              @click="scanAccount(row)"
            >
              <Icon name="refresh" size="sm" :class="scanningKeys.has(accountModelKey(row)) ? 'animate-spin' : ''" />
            </button>
            <button type="button" class="whitespace-nowrap text-xs text-primary-600 hover:text-primary-700 dark:text-primary-400" data-test="edit-state-target" @click="editStateRule(row)">{{ t('admin.ops.turnState.editTarget') }}</button>
            </div>
          </template>
        </DataTable>

        <DataTable v-else-if="activeTab === 'proxies'" :columns="proxyColumns" :data="proxies" :loading="loading" row-key="id">
          <template #cell-proxy="{ row }">
            <div class="flex min-w-[260px] items-center gap-3">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-cyan-50 text-cyan-700 dark:bg-cyan-950/50 dark:text-cyan-300">
                <Icon name="globe" size="sm" />
              </div>
              <div class="min-w-0">
                <div class="truncate font-medium text-gray-900 dark:text-white" :title="row.name">{{ row.name }}</div>
                <div class="mt-0.5 max-w-80 truncate font-mono text-xs text-gray-500" :title="row.masked_url">{{ row.masked_url }}</div>
              </div>
            </div>
          </template>
          <template #cell-enabled="{ row }">
            <button
              type="button"
              role="switch"
              :aria-checked="row.enabled"
              :class="[
                'relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500/40',
                row.enabled ? 'bg-primary-500' : 'bg-gray-200 dark:bg-dark-600'
              ]"
              @click="toggleProxy(row, !row.enabled)"
            >
              <span :class="['pointer-events-none inline-block h-4 w-4 rounded-full bg-white shadow-sm transition-transform', row.enabled ? 'translate-x-4' : 'translate-x-0']" />
            </button>
          </template>
          <template #cell-route_binding_enabled="{ row }">
            <button
              type="button"
              role="switch"
              :aria-checked="row.route_binding_enabled"
              :title="t('admin.ops.turnState.routeBindingHint')"
              :class="[
                'relative inline-flex h-5 w-9 shrink-0 rounded-full border-2 border-transparent transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500/40',
                row.route_binding_enabled ? 'bg-emerald-500' : 'bg-gray-200 dark:bg-dark-600'
              ]"
              @click="toggleRouteBinding(row, !row.route_binding_enabled)"
            >
              <span :class="['pointer-events-none inline-block h-4 w-4 rounded-full bg-white shadow-sm transition-transform', row.route_binding_enabled ? 'translate-x-4' : 'translate-x-0']" />
            </button>
          </template>
          <template #cell-health_status="{ row }">
            <div class="flex items-center gap-2">
              <span :class="['h-2 w-2 shrink-0 rounded-full', proxyHealthDotClass(row.health_status)]" />
              <div>
                <div :class="['text-sm font-medium', proxyHealthTextClass(row.health_status)]">{{ proxyHealthLabel(row.health_status) }}</div>
                <div v-if="row.consecutive_failures" class="mt-0.5 text-xs text-red-500">{{ t('admin.ops.turnState.failures', { count: row.consecutive_failures }) }}</div>
              </div>
            </div>
          </template>
          <template #cell-last_checked_at="{ row }">
            <span v-if="row.last_checked_at" class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">{{ formatDateTime(row.last_checked_at) }}</span>
            <span v-else class="text-gray-400">-</span>
          </template>
          <template #cell-actions="{ row }">
            <button class="flex h-8 w-8 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/40" :title="t('common.delete')" :aria-label="t('common.delete')" @click="deleteProxy(row)">
              <Icon name="trash" size="sm" />
            </button>
          </template>
        </DataTable>

        <DataTable
          v-else
          :columns="historyColumns"
          :data="history"
          :loading="loading"
          row-key="id"
          selectable
          :selected-keys="selectedHistoryIDs"
          @update:selectedKeys="selectedHistoryIDs = $event.map(Number)"
        >
          <template #cell-state="{ row }">
            <div class="min-w-[210px]">
              <div class="flex items-center gap-2">
                <span :class="['h-2 w-2 shrink-0 rounded-full', row.active ? 'bg-emerald-500' : 'bg-gray-400']" />
                <span class="font-mono text-sm font-medium text-gray-900 dark:text-white">{{ row.masked_value }}</span>
              </div>
              <div class="mt-1 max-w-64 truncate pl-4 font-mono text-[11px] text-gray-400" :title="row.state_hash">{{ row.state_hash }}</div>
            </div>
          </template>
          <template #cell-source="{ row }">
            <div class="min-w-[180px]">
              <div class="max-w-56 truncate text-sm font-medium text-gray-700 dark:text-gray-200" :title="row.source_account_name">{{ row.source_account_name || `#${row.source_account_id ?? '-'}` }}</div>
              <div class="mt-1 flex items-center gap-1.5 font-mono text-xs text-gray-500">
                <span>#{{ row.source_account_id ?? '-' }}</span>
                <span aria-hidden="true">/</span>
                <span>{{ row.source_model || '-' }}</span>
              </div>
              <div v-if="row.route_ipv6" class="mt-1 max-w-64 truncate font-mono text-[11px] text-cyan-600 dark:text-cyan-400" :title="row.route_ipv6">IPv6 {{ row.route_ipv6 }}</div>
            </div>
          </template>
          <template #cell-last_seen_at="{ value }"><span class="whitespace-nowrap text-sm">{{ formatDateTime(value) }}</span></template>
          <template #cell-expires_at="{ value }"><span class="whitespace-nowrap text-sm">{{ formatDateTime(value) }}</span></template>
          <template #cell-active="{ value }">
            <span :class="['inline-flex items-center gap-2 text-sm font-medium', value ? 'text-emerald-700 dark:text-emerald-300' : 'text-gray-500 dark:text-dark-400']">
              <span :class="['h-2 w-2 rounded-full', value ? 'bg-emerald-500' : 'bg-gray-400']" />
              {{ value ? t('admin.ops.turnState.active') : t('admin.ops.turnState.expired') }}
            </span>
          </template>
          <template #cell-actions="{ row }">
            <button class="flex h-8 w-8 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/40" :title="t('common.delete')" :aria-label="t('common.delete')" @click="requestHistoryDelete([row.id])"><Icon name="trash" size="sm" /></button>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="activeTab === 'accounts' && accountPagination.total > 0"
          :page="accountPagination.page"
          :total="accountPagination.total"
          :page-size="accountPagination.page_size"
          @update:page="page => { accountPagination.page = page; loadAccounts() }"
          @update:pageSize="size => { accountPagination.page = 1; accountPagination.page_size = size; loadAccounts() }"
        />
        <Pagination
          v-if="activeTab === 'history' && historyPagination.total > 0"
          :page="historyPagination.page"
          :total="historyPagination.total"
          :page-size="historyPagination.page_size"
          @update:page="page => { historyPagination.page = page; loadHistory() }"
          @update:pageSize="size => { historyPagination.page = 1; historyPagination.page_size = size; loadHistory() }"
        />
      </template>
    </TablePageLayout>

    <BaseDialog :show="showProxyDialog" :title="t('admin.ops.turnState.addProxyTitle')" width="wide" @close="closeProxyDialog">
      <div class="space-y-3">
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.addProxyHint') }}</p>
        <textarea v-model="proxyValues" rows="10" class="input font-mono text-sm" :placeholder="t('admin.ops.turnState.addProxyPlaceholder')"></textarea>
        <div class="text-right text-xs text-gray-400">{{ parsedProxyValues.length }} / 500</div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="closeProxyDialog">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving || parsedProxyValues.length === 0 || parsedProxyValues.length > 500" @click="addProxies">{{ saving ? t('common.submitting') : t('common.confirm') }}</button>
      </template>
    </BaseDialog>

    <CodexScanSettingsDialog :show="showScanSettingsDialog" @close="showScanSettingsDialog = false" @saved="onScanSettingsSaved" />

    <ConfirmDialog
      :show="showHistoryDeleteDialog"
      :title="t('admin.ops.turnState.deleteTitle')"
      :message="t('admin.ops.turnState.deleteConfirm', { count: pendingHistoryDeleteIDs.length })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="deleteHistory"
      @cancel="showHistoryDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import CodexStateStatus from '@/components/account/CodexStateStatus.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import CodexScanSettingsDialog from './components/CodexScanSettingsDialog.vue'
import CodexStateRulesPanel from './components/CodexStateRulesPanel.vue'
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatDateTime } from '@/utils/format'
import {
  opsAPI,
  type CodexTurnStateAccountStatus,
  type CodexTurnStateOperationsSummary,
  type CodexTurnStateProxy,
  type CodexTurnStateRecord,
  type CodexTurnStateStatus
} from '@/api/admin/ops'
import type { Column } from '@/components/common/types'

type Tab = 'accounts' | 'proxies' | 'history'

const { t } = useI18n()
const appStore = useAppStore()
const activeTab = ref<Tab>('accounts')
const loading = ref(false)
const saving = ref(false)
const scanningAll = ref(false)
const summary = ref<CodexTurnStateOperationsSummary | null>(null)
type CodexTurnStateAccountRow = CodexTurnStateAccountStatus & { row_key: string }
const accounts = ref<CodexTurnStateAccountRow[]>([])
const proxies = ref<CodexTurnStateProxy[]>([])
const history = ref<CodexTurnStateRecord[]>([])
const scanningKeys = reactive(new Set<string>())
const now = ref(Date.now())
let clock: number | null = null

const accountPagination = reactive({ page: 1, page_size: 50, total: 0 })
const historyPagination = reactive({ page: 1, page_size: getPersistedPageSize(), total: 0 })
const historyStatus = ref<CodexTurnStateStatus>('active')
const selectedHistoryIDs = ref<number[]>([])
const showHistoryDeleteDialog = ref(false)
const pendingHistoryDeleteIDs = ref<number[]>([])
const showProxyDialog = ref(false)
const showScanSettingsDialog = ref(false)
const stateRulesPanel = ref<InstanceType<typeof CodexStateRulesPanel> | null>(null)
const proxyValues = ref('')

function openScanSettings() {
  showScanSettingsDialog.value = true
}
function editStateRule(row: CodexTurnStateAccountStatus) {
  void stateRulesPanel.value?.editRule({ plan_type: row.plan_type || '*', model: row.model, target_lengths: row.target_lengths ?? [] })
}
function onScanSettingsSaved() { void stateRulesPanel.value?.load(); void refresh() }

const tabs = computed(() => [
  { value: 'accounts' as Tab, label: t('admin.ops.turnState.tabs.accounts'), icon: 'users' as const },
  { value: 'proxies' as Tab, label: t('admin.ops.turnState.tabs.proxies'), icon: 'globe' as const },
  { value: 'history' as Tab, label: t('admin.ops.turnState.tabs.history'), icon: 'database' as const }
])

const metrics = computed(() => [
  { label: t('admin.ops.turnState.metrics.oauthAccounts'), value: summary.value?.oauth_accounts ?? 0, valueClass: 'text-gray-900 dark:text-white', accentClass: 'border-t-2 border-t-gray-500' },
  { label: t('admin.ops.turnState.metrics.readyAccounts'), value: summary.value?.ready_accounts ?? 0, valueClass: 'text-emerald-600 dark:text-emerald-400', accentClass: 'border-t-2 border-t-emerald-500' },
  { label: t('admin.ops.turnState.metrics.missingAccounts'), value: summary.value?.missing_accounts ?? 0, valueClass: 'text-amber-600 dark:text-amber-400', accentClass: 'border-t-2 border-t-amber-500' },
  { label: t('admin.ops.turnState.metrics.readyModelSlots'), value: `${summary.value?.ready_model_slots ?? 0}/${summary.value?.total_model_slots ?? 0}`, valueClass: 'text-cyan-600 dark:text-cyan-400', accentClass: 'border-t-2 border-t-cyan-500' },
  { label: t('admin.ops.turnState.metrics.runningJobs'), value: summary.value?.running_jobs ?? 0, valueClass: 'text-blue-600 dark:text-blue-400', accentClass: 'border-t-2 border-t-blue-500' },
  { label: t('admin.ops.turnState.metrics.enabledProxies'), value: summary.value?.enabled_proxies ?? 0, valueClass: 'text-gray-900 dark:text-white', accentClass: 'border-t-2 border-t-gray-500' },
  { label: t('admin.ops.turnState.metrics.healthyProxies'), value: summary.value?.healthy_proxies ?? 0, valueClass: 'text-emerald-600 dark:text-emerald-400', accentClass: 'border-t-2 border-t-emerald-500' },
  { label: t('admin.ops.turnState.metrics.sharedProxies'), value: summary.value?.shared_proxies ?? 0, valueClass: 'text-cyan-600 dark:text-cyan-400', accentClass: 'border-t-2 border-t-cyan-500' }
])

const accountColumns = computed<Column[]>(() => [
  { key: 'account', label: t('admin.ops.turnState.columns.account') },
  { key: 'model', label: t('admin.ops.turnState.columns.model') },
  { key: 'state', label: t('admin.ops.turnState.columns.state') },
  { key: 'expires_at', label: t('admin.ops.turnState.columns.expiresAt') },
  { key: 'scan', label: t('admin.ops.turnState.columns.scan') },
  { key: 'proxy', label: t('admin.ops.turnState.columns.proxy') },
  { key: 'actions', label: t('common.actions') }
])

const proxyColumns = computed<Column[]>(() => [
  { key: 'proxy', label: t('admin.ops.turnState.columns.proxy') },
  { key: 'enabled', label: t('admin.ops.turnState.columns.enabled') },
  { key: 'route_binding_enabled', label: t('admin.ops.turnState.columns.routeBinding') },
  { key: 'health_status', label: t('admin.ops.turnState.columns.health') },
  { key: 'last_checked_at', label: t('admin.ops.turnState.columns.lastChecked') },
  { key: 'actions', label: t('common.actions') }
])

const historyColumns = computed<Column[]>(() => [
  { key: 'state', label: t('admin.ops.turnState.columns.state') },
  { key: 'value_length', label: t('admin.ops.turnState.columns.length') },
  { key: 'source', label: t('admin.ops.turnState.columns.source') },
  { key: 'last_seen_at', label: t('admin.ops.turnState.columns.lastSeen') },
  { key: 'expires_at', label: t('admin.ops.turnState.columns.expiresAt') },
  { key: 'active', label: t('admin.ops.turnState.columns.status') },
  { key: 'actions', label: t('common.actions') }
])

const historyStatusOptions = computed(() => [
  { value: 'active', label: t('admin.ops.turnState.active') },
  { value: 'expired', label: t('admin.ops.turnState.expired') },
  { value: 'all', label: t('admin.ops.turnState.all') }
])

const parsedProxyValues = computed(() => Array.from(new Set(proxyValues.value.split(/\r?\n/).map(value => value.trim()).filter(Boolean))))

function accountModelKey(row: CodexTurnStateAccountStatus) { return `${row.account_id}:${row.model}` }
function accountInitial(name: string) { return Array.from(name.trim())[0] || '#' }
function proxyHealthLabel(status: string) { return t(`admin.ops.turnState.proxyHealth.${status}`, status) }
function proxyHealthDotClass(status: string) {
  if (status === 'healthy') return 'bg-emerald-500'
  if (status === 'unhealthy') return 'bg-red-500'
  return 'bg-gray-400'
}
function proxyHealthTextClass(status: string) {
  if (status === 'healthy') return 'text-emerald-700 dark:text-emerald-300'
  if (status === 'unhealthy') return 'text-red-700 dark:text-red-300'
  return 'text-gray-600 dark:text-gray-300'
}
function expiryCountdown(value: string) {
  const seconds = Math.max(0, Math.floor((new Date(value).getTime() - now.value) / 1000))
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

async function loadSummary() { summary.value = await opsAPI.getCodexTurnStateOperationsSummary() }
async function loadAccounts() {
  const result = await opsAPI.listCodexTurnStateAccounts({ page: accountPagination.page, page_size: accountPagination.page_size })
  accounts.value = result.items.map(item => ({ ...item, row_key: `${item.account_id}:${item.model}` }))
  accountPagination.page = result.page
  accountPagination.page_size = result.page_size
  accountPagination.total = result.total
}
async function loadAccountRow(accountId: number, model: string) {
  const result = await opsAPI.listCodexTurnStateAccounts({ page: 1, page_size: 200, account_ids: String(accountId) })
  const updated = result.items.find(item => item.account_id === accountId && item.model === model)
  if (!updated) return undefined
  const key = `${accountId}:${model}`
  accounts.value = accounts.value.map(item => item.row_key === key ? { ...updated, row_key: key } : item)
  return updated
}
async function loadProxies() { proxies.value = await opsAPI.listCodexTurnStateProxies() }
async function loadHistory() {
  const result = await opsAPI.listCodexTurnStates({ page: historyPagination.page, page_size: historyPagination.page_size, status: historyStatus.value })
  history.value = result.items
  historyPagination.page = result.page
  historyPagination.page_size = result.page_size
  historyPagination.total = result.total
  selectedHistoryIDs.value = []
}
async function loadCurrentTab() {
  if (activeTab.value === 'accounts') await loadAccounts()
  else if (activeTab.value === 'proxies') await loadProxies()
  else await loadHistory()
}
async function refresh() {
  loading.value = true
  try { await Promise.all([loadSummary(), loadCurrentTab()]) }
  catch (error: any) { appStore.showError(error?.response?.data?.detail || t('admin.ops.turnState.loadFailed')) }
  finally { loading.value = false }
}
function setTab(tab: Tab) { activeTab.value = tab; refresh() }
function handleHistoryStatusChange() { historyPagination.page = 1; loadHistory() }

async function scanAccount(row: CodexTurnStateAccountStatus) {
  const key = accountModelKey(row)
  if (scanningKeys.has(key)) return
  scanningKeys.add(key)
  try {
    const result = await opsAPI.scanCodexTurnState(row.account_id, row.model)
    appStore.showSuccess(result.queued ? t('admin.ops.turnState.scanQueued') : t('admin.ops.turnState.scanAlreadyQueued'))
    if (result.queued) {
      accounts.value = accounts.value.map(item => item.row_key === key ? { ...item, status: 'pending' } : item)
    }
    for (let attempt = 0; attempt < 20; attempt++) {
      await new Promise(resolve => window.setTimeout(resolve, 1000))
      const updated = await loadAccountRow(row.account_id, row.model)
      if (updated && updated.status !== 'pending' && updated.status !== 'running') break
    }
    await loadSummary()
  } catch (error: any) { appStore.showError(error?.response?.data?.detail || t('admin.ops.turnState.scanFailed')) }
  finally { scanningKeys.delete(key) }
}
async function scanAll() {
  scanningAll.value = true
  try {
    const result = await opsAPI.scanAllCodexTurnStates()
    appStore.showSuccess(t('admin.ops.turnState.scanAllQueued', { count: result.queued }))
    window.setTimeout(refresh, 800)
  } catch (error: any) { appStore.showError(error?.response?.data?.detail || t('admin.ops.turnState.scanFailed')) }
  finally { scanningAll.value = false }
}

function closeProxyDialog() { showProxyDialog.value = false; proxyValues.value = '' }
async function addProxies() {
  if (!parsedProxyValues.value.length) return
  saving.value = true
  try {
    const result = await opsAPI.addCodexTurnStateProxies(parsedProxyValues.value)
    appStore.showSuccess(t('admin.ops.turnState.addProxySuccess', { count: result.added }))
    closeProxyDialog()
    await refresh()
  } catch (error: any) { appStore.showError(error?.response?.data?.detail || error?.message || t('admin.ops.turnState.addProxyFailed')) }
  finally { saving.value = false }
}
async function toggleProxy(proxy: CodexTurnStateProxy, enabled: boolean) {
  try { await opsAPI.setCodexTurnStateProxyEnabled(proxy.id, enabled); proxy.enabled = enabled; await loadSummary() }
  catch (error: any) { appStore.showError(error?.response?.data?.detail || t('admin.ops.turnState.proxyUpdateFailed')); await loadProxies() }
}
async function toggleRouteBinding(proxy: CodexTurnStateProxy, enabled: boolean) {
  try { await opsAPI.setCodexTurnStateProxyRouteBinding(proxy.id, enabled); proxy.route_binding_enabled = enabled }
  catch (error: any) { appStore.showError(error?.response?.data?.detail || t('admin.ops.turnState.proxyUpdateFailed')); await loadProxies() }
}
async function deleteProxy(proxy: CodexTurnStateProxy) {
  if (!window.confirm(t('admin.ops.turnState.deleteProxyConfirm', { name: proxy.name }))) return
  try { await opsAPI.deleteCodexTurnStateProxy(proxy.id); await refresh() }
  catch (error: any) { appStore.showError(error?.response?.data?.detail || t('admin.ops.turnState.deleteProxyFailed')) }
}

function requestHistoryDelete(ids: number[]) { pendingHistoryDeleteIDs.value = ids; showHistoryDeleteDialog.value = true }
async function deleteHistory() {
  saving.value = true
  try {
    const result = await opsAPI.deleteCodexTurnStates(pendingHistoryDeleteIDs.value)
    appStore.showSuccess(t('admin.ops.turnState.deleteSuccess', { count: result.deleted }))
    showHistoryDeleteDialog.value = false
    pendingHistoryDeleteIDs.value = []
    await refresh()
  } catch (error: any) { appStore.showError(error?.response?.data?.detail || t('admin.ops.turnState.deleteFailed')) }
  finally { saving.value = false }
}

onMounted(() => {
  refresh()
  clock = window.setInterval(() => { now.value = Date.now() }, 1000)
})
onUnmounted(() => { if (clock !== null) window.clearInterval(clock) })
</script>

<style scoped>
.codex-state-layout {
  height: auto;
  min-height: calc(100vh - 128px);
}
.codex-state-layout :deep(.layout-section-scrollable) {
  flex: none;
  min-height: 320px;
}
.codex-state-layout :deep(.table-scroll-container) {
  max-height: 65vh;
  min-height: 320px;
}
@media (max-width: 1023px) {
  .codex-state-layout :deep(.table-scroll-container) {
    max-height: none;
  }
}
</style>
