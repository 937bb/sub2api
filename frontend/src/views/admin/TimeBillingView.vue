<template>
  <AppLayout>
    <div class="w-full space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 pb-4 dark:border-dark-700">
        <div class="relative w-full sm:w-80">
          <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
          <input v-model.trim="search" class="input pl-9" type="search" :placeholder="t('timeBilling.searchPlaceholder')" />
        </div>
        <div class="flex items-center gap-3">
          <span v-if="timezoneLabel" class="text-sm text-gray-500 dark:text-dark-400">{{ t('timeBilling.timezone') }}: {{ timezoneLabel }}</span>
          <button class="btn btn-secondary px-3" :title="t('timeBilling.reload')" :disabled="loading" @click="loadGroups">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>

      <div v-if="loading" class="flex justify-center py-16">
        <Icon name="refresh" size="lg" class="animate-spin text-primary-500" />
      </div>

      <div v-else-if="filteredRows.length" class="overflow-x-auto rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
        <table class="w-full min-w-[1080px] table-fixed text-left text-sm">
          <colgroup>
            <col class="w-[220px]" />
            <col class="w-[120px]" />
            <col class="w-[110px]" />
            <col class="w-[90px]" />
            <col class="w-[150px]" />
            <col class="w-[150px]" />
            <col class="w-[150px]" />
            <col class="w-[130px]" />
            <col class="w-[100px]" />
          </colgroup>
          <thead class="border-b border-gray-200 bg-gray-50 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400">
            <tr>
              <th class="px-4 py-3">{{ t('timeBilling.group') }}</th>
              <th class="px-4 py-3">{{ t('timeBilling.platform') }}</th>
              <th class="px-4 py-3 text-right">{{ t('timeBilling.baseMultiplier') }}</th>
              <th class="px-4 py-3 text-center">{{ t('timeBilling.enabled') }}</th>
              <th class="px-4 py-3">{{ t('timeBilling.start') }}</th>
              <th class="px-4 py-3">{{ t('timeBilling.end') }}</th>
              <th class="px-4 py-3">{{ t('timeBilling.multiplier') }}</th>
              <th class="px-4 py-3 text-right">{{ t('timeBilling.effectiveMultiplier') }}</th>
              <th class="px-4 py-3 text-right">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in filteredRows" :key="row.id" class="border-b border-gray-100 last:border-b-0 dark:border-dark-800">
              <td class="px-4 py-3">
                <div class="truncate font-medium text-gray-900 dark:text-white" :title="row.name">{{ row.name }}</div>
                <span class="mt-1 inline-flex text-xs" :class="row.status === 'active' ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400 dark:text-dark-500'">
                  {{ row.status === 'active' ? t('common.active') : t('common.inactive') }}
                </span>
              </td>
              <td class="px-4 py-3 text-gray-600 dark:text-dark-300">{{ platformLabel(row.platform) }}</td>
              <td class="px-4 py-3 text-right font-medium text-gray-700 dark:text-dark-200">{{ formatMultiplier(row.baseMultiplier) }}x</td>
              <td class="px-4 py-3 text-center">
                <div class="inline-flex"><Toggle :model-value="row.enabled" @update:model-value="toggleRow(row, $event)" /></div>
              </td>
              <td class="px-4 py-3"><input v-model="row.start" class="input" :class="row.enabled && !isWindowValid(row) ? 'border-red-400 focus:border-red-500 focus:ring-red-500' : ''" type="time" :disabled="!row.enabled" /></td>
              <td class="px-4 py-3"><input v-model="row.end" class="input" :class="row.enabled && !isWindowValid(row) ? 'border-red-400 focus:border-red-500 focus:ring-red-500' : ''" type="time" :disabled="!row.enabled" /></td>
              <td class="px-4 py-3"><input v-model.number="row.multiplier" class="input" type="number" min="0" step="0.001" :disabled="!row.enabled" /></td>
              <td class="px-4 py-3 text-right font-semibold text-primary-600 dark:text-primary-400">{{ effectiveMultiplier(row) }}x</td>
              <td class="px-4 py-3 text-right">
                <button class="btn btn-primary btn-sm min-w-[72px] justify-center" :disabled="!isDirty(row) || !isRowValid(row) || isSaving(row.id)" @click="saveRow(row)">
                  <Icon :name="isSaving(row.id) ? 'refresh' : 'check'" size="sm" :class="isSaving(row.id) ? 'animate-spin' : ''" />
                  <span>{{ isSaving(row.id) ? t('common.saving') : t('common.save') }}</span>
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-else class="py-16 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('common.noData') }}</div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI } from '@/api/admin'
import type { AdminGroup, GroupPlatform } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { serverTimezoneLabel } from '@/utils/peak-rate'

interface TimeBillingRow {
  id: number
  name: string
  platform: GroupPlatform
  status: 'active' | 'inactive'
  baseMultiplier: number
  enabled: boolean
  start: string
  end: string
  multiplier: number
}

const { t } = useI18n()
const appStore = useAppStore()
const rows = ref<TimeBillingRow[]>([])
const search = ref('')
const loading = ref(false)
const savingIds = ref(new Set<number>())
const savedSignatures = ref<Record<number, string>>({})
const timezoneLabel = computed(() => serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))

const filteredRows = computed(() => {
  const query = search.value.toLocaleLowerCase()
  if (!query) return rows.value
  return rows.value.filter((row) => row.name.toLocaleLowerCase().includes(query) || platformLabel(row.platform).toLocaleLowerCase().includes(query))
})

function normalizeTime(value: string): string { return String(value || '').slice(0, 5) }
function rowFromGroup(group: AdminGroup): TimeBillingRow {
  return {
    id: group.id,
    name: group.name,
    platform: group.platform,
    status: group.status,
    baseMultiplier: Number(group.rate_multiplier || 0),
    enabled: Boolean(group.peak_rate_enabled),
    start: normalizeTime(group.peak_start),
    end: normalizeTime(group.peak_end),
    multiplier: Number(group.peak_rate_multiplier ?? 1),
  }
}
function signature(row: TimeBillingRow): string { return JSON.stringify([row.enabled, row.start, row.end, Number(row.multiplier)]) }
function isDirty(row: TimeBillingRow): boolean { return savedSignatures.value[row.id] !== signature(row) }
function isSaving(id: number): boolean { return savingIds.value.has(id) }
function isWindowValid(row: TimeBillingRow): boolean { return !row.enabled || Boolean(row.start && row.end && row.start < row.end) }
function isRowValid(row: TimeBillingRow): boolean { return isWindowValid(row) && Number.isFinite(Number(row.multiplier)) && Number(row.multiplier) >= 0 }
function formatMultiplier(value: number): string { return Number(value || 0).toFixed(3) }
function effectiveMultiplier(row: TimeBillingRow): string { return formatMultiplier(row.baseMultiplier * (row.enabled ? Number(row.multiplier) : 1)) }
function platformLabel(platform: GroupPlatform): string { return t(`admin.groups.platforms.${platform}`) }
function toggleRow(row: TimeBillingRow, enabled: boolean): void {
  row.enabled = enabled
  if (enabled && !row.start) row.start = '09:00'
  if (enabled && !row.end) row.end = '18:00'
}
function setSaving(id: number, saving: boolean): void {
  const next = new Set(savingIds.value)
  if (saving) next.add(id)
  else next.delete(id)
  savingIds.value = next
}
async function loadGroups(): Promise<void> {
  loading.value = true
  try {
    const groups = await adminAPI.groups.getAllIncludingInactive()
    rows.value = groups.map(rowFromGroup)
    savedSignatures.value = Object.fromEntries(rows.value.map((row) => [row.id, signature(row)]))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('timeBilling.loadFailed')))
  } finally {
    loading.value = false
  }
}
async function saveRow(row: TimeBillingRow): Promise<void> {
  if (!isRowValid(row)) {
    appStore.showError(t('timeBilling.invalidRule'))
    return
  }
  setSaving(row.id, true)
  try {
    const updated = await adminAPI.groups.update(row.id, {
      peak_rate_enabled: row.enabled,
      peak_start: row.start,
      peak_end: row.end,
      peak_rate_multiplier: Number(row.multiplier),
    })
    const next = rowFromGroup(updated)
    Object.assign(row, next)
    savedSignatures.value = { ...savedSignatures.value, [row.id]: signature(row) }
    appStore.showSuccess(t('timeBilling.saveSuccess', { group: row.name }))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('timeBilling.saveFailed')))
  } finally {
    setSaving(row.id, false)
  }
}

onMounted(() => void loadGroups())
</script>
