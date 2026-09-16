<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.turnState.title') }}</h1>
            </div>
            <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh')" @click="refresh">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="showAddDialog = true">
              <Icon name="plus" size="md" class="mr-1" />
              {{ t('admin.ops.turnState.add') }}
            </button>
          </div>

          <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.longestLength') }}</div>
              <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ summary?.longest_active?.value_length ?? 0 }}</div>
            </div>
            <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.remaining') }}</div>
              <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ remainingLifetime }}</div>
            </div>
            <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.activeCount') }}</div>
              <div class="mt-2 text-2xl font-semibold text-emerald-600 dark:text-emerald-400">{{ summary?.active_count ?? 0 }}</div>
            </div>
            <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.expiredCount') }}</div>
              <div class="mt-2 text-2xl font-semibold text-gray-700 dark:text-gray-200">{{ summary?.expired_count ?? 0 }}</div>
            </div>
          </div>

          <div v-if="summary?.longest_active" class="flex flex-wrap items-center gap-x-6 gap-y-2 rounded-lg border border-primary-200 bg-primary-50 px-4 py-3 text-sm dark:border-primary-800 dark:bg-primary-900/20">
            <span class="font-mono text-primary-700 dark:text-primary-300">{{ summary.longest_active.masked_value }}</span>
            <span class="text-gray-600 dark:text-gray-300">{{ summary.longest_active.source_account_name || `#${summary.longest_active.source_account_id ?? '-'}` }}</span>
            <span class="uppercase text-gray-500 dark:text-dark-400">{{ summary.longest_active.source_transport }}</span>
          </div>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <Select v-model="status" :options="statusOptions" class="w-44" @change="handleStatusChange" />
          <button v-if="selectedIDs.length > 0" class="btn btn-danger" @click="requestDelete(selectedIDs)">
            <Icon name="trash" size="md" class="mr-1" />
            {{ t('admin.ops.turnState.deleteSelected', { count: selectedIDs.length }) }}
          </button>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="items"
          :loading="loading"
          row-key="id"
          selectable
          :selected-keys="selectedIDs"
          @update:selectedKeys="selectedIDs = $event.map(Number)"
        >
          <template #cell-state="{ row }">
            <div class="min-w-0">
              <div class="font-mono text-sm text-gray-900 dark:text-white">{{ row.masked_value }}</div>
              <div class="mt-1 max-w-56 truncate font-mono text-xs text-gray-400" :title="row.state_hash">{{ row.state_hash }}</div>
            </div>
          </template>
          <template #cell-source="{ row }">
            <div class="text-sm text-gray-700 dark:text-gray-200">{{ row.source_account_name || `#${row.source_account_id ?? '-'}` }}</div>
            <div class="mt-1 text-xs uppercase text-gray-400">{{ row.source_transport }}</div>
          </template>
          <template #cell-session="{ row }">
            <span class="font-mono text-xs text-gray-500" :title="row.source_session_hash">{{ compactHash(row.source_session_hash) }}</span>
          </template>
          <template #cell-last_seen_at="{ value }">
            <span class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">{{ formatDateTime(value) }}</span>
          </template>
          <template #cell-expires_at="{ value }">
            <span class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">{{ formatDateTime(value) }}</span>
          </template>
          <template #cell-active="{ value }">
            <span :class="['badge', value ? 'badge-success' : 'badge-gray']">
              {{ value ? t('admin.ops.turnState.active') : t('admin.ops.turnState.expired') }}
            </span>
          </template>
          <template #cell-actions="{ row }">
            <button class="rounded-lg p-2 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20" :title="t('common.delete')" @click="requestDelete([row.id])">
              <Icon name="trash" size="sm" />
            </button>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <BaseDialog :show="showAddDialog" :title="t('admin.ops.turnState.addTitle')" width="wide" @close="closeAddDialog">
      <div class="space-y-3">
        <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.addHint') }}</p>
        <textarea v-model="batchValues" rows="12" class="input font-mono text-sm" :placeholder="t('admin.ops.turnState.addPlaceholder')"></textarea>
        <div class="text-right text-xs text-gray-400">{{ parsedBatchValues.length }} / 500</div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="closeAddDialog">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving || parsedBatchValues.length === 0 || parsedBatchValues.length > 500" @click="addValues">
          {{ saving ? t('common.submitting') : t('common.confirm') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.ops.turnState.deleteTitle')"
      :message="t('admin.ops.turnState.deleteConfirm', { count: pendingDeleteIDs.length })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="deleteValues"
      @cancel="showDeleteDialog = false"
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
import { opsAPI, type CodexTurnStateRecord, type CodexTurnStateStatus, type CodexTurnStateSummary } from '@/api/admin/ops'
import type { Column } from '@/components/common/types'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const items = ref<CodexTurnStateRecord[]>([])
const summary = ref<CodexTurnStateSummary | null>(null)
const status = ref<CodexTurnStateStatus>('active')
const selectedIDs = ref<number[]>([])
const showAddDialog = ref(false)
const showDeleteDialog = ref(false)
const batchValues = ref('')
const pendingDeleteIDs = ref<number[]>([])
const saving = ref(false)
const now = ref(Date.now())
let clock: number | null = null

const pagination = reactive({ page: 1, page_size: getPersistedPageSize(), total: 0, pages: 0 })

const statusOptions = computed(() => [
  { value: 'active', label: t('admin.ops.turnState.active') },
  { value: 'expired', label: t('admin.ops.turnState.expired') },
  { value: 'all', label: t('admin.ops.turnState.all') }
])

const columns = computed<Column[]>(() => [
  { key: 'state', label: t('admin.ops.turnState.columns.state') },
  { key: 'value_length', label: t('admin.ops.turnState.columns.length') },
  { key: 'source', label: t('admin.ops.turnState.columns.source') },
  { key: 'session', label: t('admin.ops.turnState.columns.session') },
  { key: 'last_seen_at', label: t('admin.ops.turnState.columns.lastSeen') },
  { key: 'expires_at', label: t('admin.ops.turnState.columns.expiresAt') },
  { key: 'active', label: t('admin.ops.turnState.columns.status') },
  { key: 'actions', label: t('common.actions') }
])

const parsedBatchValues = computed(() => Array.from(new Set(
  batchValues.value.split(/\r?\n/).map(value => value.trim()).filter(Boolean)
)))

const remainingLifetime = computed(() => {
  const expiresAt = summary.value?.longest_active?.expires_at
  if (!expiresAt) return '0:00'
  const totalSeconds = Math.max(0, Math.floor((new Date(expiresAt).getTime() - now.value) / 1000))
  return `${Math.floor(totalSeconds / 60)}:${String(totalSeconds % 60).padStart(2, '0')}`
})

function compactHash(value?: string) {
  if (!value) return '-'
  return value.length > 16 ? `${value.slice(0, 8)}...${value.slice(-8)}` : value
}

async function loadData() {
  loading.value = true
  try {
    const [list, currentSummary] = await Promise.all([
      opsAPI.listCodexTurnStates({ page: pagination.page, page_size: pagination.page_size, status: status.value }),
      opsAPI.getCodexTurnStateSummary()
    ])
    items.value = list.items
    pagination.page = list.page
    pagination.page_size = list.page_size
    pagination.total = list.total
    pagination.pages = list.pages
    summary.value = currentSummary
    selectedIDs.value = []
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.ops.turnState.loadFailed'))
  } finally {
    loading.value = false
  }
}

function refresh() { now.value = Date.now(); loadData() }
function handleStatusChange() { pagination.page = 1; loadData() }
function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.page = 1; pagination.page_size = pageSize; loadData() }

function closeAddDialog() {
  showAddDialog.value = false
  batchValues.value = ''
}

async function addValues() {
  if (parsedBatchValues.value.length === 0 || parsedBatchValues.value.length > 500) return
  saving.value = true
  try {
    const result = await opsAPI.addCodexTurnStates(parsedBatchValues.value)
    appStore.showSuccess(t('admin.ops.turnState.addSuccess', { count: result.added }))
    closeAddDialog()
    await loadData()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || error?.message || t('admin.ops.turnState.addFailed'))
  } finally {
    saving.value = false
  }
}

function requestDelete(ids: number[]) {
  pendingDeleteIDs.value = ids
  showDeleteDialog.value = true
}

async function deleteValues() {
  saving.value = true
  try {
    const result = await opsAPI.deleteCodexTurnStates(pendingDeleteIDs.value)
    appStore.showSuccess(t('admin.ops.turnState.deleteSuccess', { count: result.deleted }))
    showDeleteDialog.value = false
    pendingDeleteIDs.value = []
    selectedIDs.value = []
    await loadData()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || error?.message || t('admin.ops.turnState.deleteFailed'))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadData()
  clock = window.setInterval(() => { now.value = Date.now() }, 1000)
})

onUnmounted(() => {
  if (clock !== null) window.clearInterval(clock)
})
</script>
