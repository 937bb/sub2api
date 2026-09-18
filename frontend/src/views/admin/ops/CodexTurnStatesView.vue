<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.turnState.title') }}</h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.description') }}</p>
            </div>
            <div class="flex items-center gap-2">
              <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh')" @click="refresh">
                <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
              </button>
              <button v-if="activeTab === 'accounts'" class="btn btn-primary" :disabled="scanningAll" @click="scanAll">
                <Icon name="play" size="md" class="mr-1" />
                {{ t('admin.ops.turnState.scanAll') }}
              </button>
              <button v-if="activeTab === 'proxies'" class="btn btn-primary" @click="showProxyDialog = true">
                <Icon name="plus" size="md" class="mr-1" />
                {{ t('admin.ops.turnState.addProxy') }}
              </button>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-3 lg:grid-cols-7">
            <div v-for="metric in metrics" :key="metric.label" class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ metric.label }}</div>
              <div :class="['mt-1 text-xl font-semibold', metric.className]">{{ metric.value }}</div>
            </div>
          </div>

          <div class="inline-flex rounded-md border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900">
            <button
              v-for="tab in tabs"
              :key="tab.value"
              type="button"
              :class="[
                'rounded px-3 py-1.5 text-sm font-medium transition-colors',
                activeTab === tab.value
                  ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
                  : 'text-gray-500 hover:text-gray-800 dark:text-dark-300 dark:hover:text-white'
              ]"
              @click="setTab(tab.value)"
            >
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
        <div v-else class="text-sm text-gray-500 dark:text-dark-400">
          {{ activeTab === 'accounts' ? t('admin.ops.turnState.accountHint') : t('admin.ops.turnState.proxyHint') }}
        </div>
      </template>

      <template #table>
        <DataTable v-if="activeTab === 'accounts'" :columns="accountColumns" :data="accounts" :loading="loading" row-key="row_key">
          <template #cell-account="{ row }">
            <div>
              <div class="font-medium text-gray-900 dark:text-white">{{ row.account_name }}</div>
              <div class="mt-0.5 text-xs text-gray-500">#{{ row.account_id }} · {{ row.account_type }}<span v-if="row.plan_type"> · {{ row.plan_type }}</span></div>
            </div>
          </template>
          <template #cell-model="{ value }"><span class="font-mono text-xs">{{ value }}</span></template>
          <template #cell-state="{ row }">
            <span :class="['badge', stateBadgeClass(row.status)]">{{ stateStatusLabel(row.status) }}</span>
            <span v-if="row.state_length" class="ml-2 font-mono text-xs text-gray-500">{{ row.state_length }}</span>
          </template>
          <template #cell-expires_at="{ row }">
            <div v-if="row.expires_at" class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">
              {{ formatDateTime(row.expires_at) }}
              <div class="text-xs text-gray-400">{{ expiryCountdown(row.expires_at) }}</div>
            </div>
            <span v-else class="text-gray-400">-</span>
          </template>
          <template #cell-scan="{ row }">
            <div class="text-xs text-gray-600 dark:text-gray-300">{{ t('admin.ops.turnState.attempts', { count: row.attempt_count }) }}</div>
            <div v-if="row.last_attempt_at" class="mt-1 whitespace-nowrap text-xs text-gray-400">{{ formatDateTime(row.last_attempt_at) }}</div>
            <div v-if="row.last_error" class="mt-1 max-w-72 truncate text-xs text-red-500" :title="row.last_error">{{ row.last_error }}</div>
          </template>
          <template #cell-proxy="{ row }">
            <span v-if="row.last_proxy_masked" class="font-mono text-xs text-gray-600 dark:text-gray-300">{{ row.last_proxy_masked }}</span>
            <span v-else class="text-xs text-gray-400">{{ t('admin.ops.turnState.direct') }}</span>
          </template>
          <template #cell-actions="{ row }">
            <button class="btn btn-secondary px-2 py-1 text-xs" :disabled="scanningKeys.has(accountModelKey(row))" @click="scanAccount(row)">
              <Icon name="refresh" size="xs" :class="scanningKeys.has(accountModelKey(row)) ? 'mr-1 animate-spin' : 'mr-1'" />
              {{ t('admin.ops.turnState.scanNow') }}
            </button>
          </template>
        </DataTable>

        <DataTable v-else-if="activeTab === 'proxies'" :columns="proxyColumns" :data="proxies" :loading="loading" row-key="id">
          <template #cell-proxy="{ row }">
            <div class="font-medium text-gray-900 dark:text-white">{{ row.name }}</div>
            <div class="mt-1 font-mono text-xs text-gray-500">{{ row.masked_url }}</div>
          </template>
          <template #cell-enabled="{ row }">
            <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" :checked="row.enabled" @change="toggleProxy(row, ($event.target as HTMLInputElement).checked)" />
          </template>
          <template #cell-health_status="{ row }">
            <span :class="['badge', proxyHealthClass(row.health_status)]">{{ proxyHealthLabel(row.health_status) }}</span>
            <div v-if="row.consecutive_failures" class="mt-1 text-xs text-red-500">{{ t('admin.ops.turnState.failures', { count: row.consecutive_failures }) }}</div>
          </template>
          <template #cell-last_checked_at="{ row }">
            <span v-if="row.last_checked_at" class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">{{ formatDateTime(row.last_checked_at) }}</span>
            <span v-else class="text-gray-400">-</span>
          </template>
          <template #cell-actions="{ row }">
            <button class="rounded p-2 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20" :title="t('common.delete')" @click="deleteProxy(row)">
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
            <div class="font-mono text-sm text-gray-900 dark:text-white">{{ row.masked_value }}</div>
            <div class="mt-1 max-w-56 truncate font-mono text-xs text-gray-400" :title="row.state_hash">{{ row.state_hash }}</div>
          </template>
          <template #cell-source="{ row }">
            <div class="text-sm text-gray-700 dark:text-gray-200">{{ row.source_account_name || `#${row.source_account_id ?? '-'}` }}</div>
            <div class="mt-1 font-mono text-xs text-gray-500">{{ row.source_model || '-' }}</div>
          </template>
          <template #cell-last_seen_at="{ value }"><span class="whitespace-nowrap text-sm">{{ formatDateTime(value) }}</span></template>
          <template #cell-expires_at="{ value }"><span class="whitespace-nowrap text-sm">{{ formatDateTime(value) }}</span></template>
          <template #cell-active="{ value }"><span :class="['badge', value ? 'badge-success' : 'badge-gray']">{{ value ? t('admin.ops.turnState.active') : t('admin.ops.turnState.expired') }}</span></template>
          <template #cell-actions="{ row }">
            <button class="rounded p-2 text-gray-500 hover:bg-red-50 hover:text-red-600" :title="t('common.delete')" @click="requestHistoryDelete([row.id])"><Icon name="trash" size="sm" /></button>
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
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatDateTime } from '@/utils/format'
import {
  opsAPI,
  type CodexTurnStateAccountStatus,
  type CodexTurnStateAccountStatusValue,
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
const proxyValues = ref('')

const tabs = computed(() => [
  { value: 'accounts' as Tab, label: t('admin.ops.turnState.tabs.accounts') },
  { value: 'proxies' as Tab, label: t('admin.ops.turnState.tabs.proxies') },
  { value: 'history' as Tab, label: t('admin.ops.turnState.tabs.history') }
])

const metrics = computed(() => [
  { label: t('admin.ops.turnState.metrics.oauthAccounts'), value: summary.value?.oauth_accounts ?? 0, className: 'text-gray-900 dark:text-white' },
  { label: t('admin.ops.turnState.metrics.readyAccounts'), value: summary.value?.ready_accounts ?? 0, className: 'text-emerald-600 dark:text-emerald-400' },
  { label: t('admin.ops.turnState.metrics.missingAccounts'), value: summary.value?.missing_accounts ?? 0, className: 'text-amber-600 dark:text-amber-400' },
  { label: t('admin.ops.turnState.metrics.runningJobs'), value: summary.value?.running_jobs ?? 0, className: 'text-blue-600 dark:text-blue-400' },
  { label: t('admin.ops.turnState.metrics.enabledProxies'), value: summary.value?.enabled_proxies ?? 0, className: 'text-gray-900 dark:text-white' },
  { label: t('admin.ops.turnState.metrics.healthyProxies'), value: summary.value?.healthy_proxies ?? 0, className: 'text-emerald-600 dark:text-emerald-400' },
  { label: t('admin.ops.turnState.metrics.sharedProxies'), value: summary.value?.shared_proxies ?? 0, className: 'text-cyan-600 dark:text-cyan-400' }
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
function stateStatusLabel(status: CodexTurnStateAccountStatusValue) { return t(`admin.ops.turnState.status.${status}`) }
function stateBadgeClass(status: CodexTurnStateAccountStatusValue) {
  if (status === 'ready') return 'badge-success'
  if (status === 'running' || status === 'pending') return 'badge-info'
  if (status === 'expiring' || status === 'retry_wait') return 'badge-warning'
  if (status === 'failed') return 'badge-danger'
  return 'badge-gray'
}
function proxyHealthLabel(status: string) { return t(`admin.ops.turnState.proxyHealth.${status}`, status) }
function proxyHealthClass(status: string) {
  if (status === 'healthy') return 'badge-success'
  if (status === 'unhealthy') return 'badge-danger'
  return 'badge-gray'
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
  scanningKeys.add(key)
  try {
    const result = await opsAPI.scanCodexTurnState(row.account_id, row.model)
    appStore.showSuccess(result.queued ? t('admin.ops.turnState.scanQueued') : t('admin.ops.turnState.scanAlreadyQueued'))
    window.setTimeout(refresh, 800)
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
