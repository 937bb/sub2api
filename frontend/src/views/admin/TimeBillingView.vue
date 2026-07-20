<template>
  <AppLayout>
    <div class="w-full space-y-4">
      <div class="grid gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 md:grid-cols-2 xl:grid-cols-[minmax(240px,1fr)_180px_180px_180px_auto]">
        <div class="relative">
          <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
          <input v-model.trim="search" class="input pl-9" type="search" :placeholder="t('timeBilling.searchPlaceholder')" />
        </div>
        <Select v-model="platformFilter" :options="platformOptions" />
        <Select v-model="groupStatusFilter" :options="groupStatusOptions" />
        <Select v-model="ruleStateFilter" :options="ruleStateOptions" />
        <div class="flex items-center justify-end gap-3">
          <span v-if="timezoneLabel" class="whitespace-nowrap text-sm text-gray-500 dark:text-dark-400">{{ t('timeBilling.timezone') }}: {{ timezoneLabel }}</span>
          <button class="btn btn-secondary px-3" :title="t('timeBilling.reload')" :disabled="loading" @click="loadGroups">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>

      <div v-if="loading" class="flex justify-center py-16">
        <Icon name="refresh" size="lg" class="animate-spin text-primary-500" />
      </div>

      <div v-else-if="filteredGroups.length" class="space-y-3">
        <section v-for="group in filteredGroups" :key="group.id" class="overflow-hidden rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
          <header class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-800">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="group.name">{{ group.name }}</h2>
                <span class="badge badge-info">{{ platformLabel(group.platform) }}</span>
                <span class="text-xs" :class="group.status === 'active' ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400 dark:text-dark-500'">
                  {{ group.status === 'active' ? t('common.active') : t('common.inactive') }}
                </span>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('timeBilling.baseMultiplier') }} {{ formatMultiplier(group.baseMultiplier) }}x · {{ t('timeBilling.ruleCount', { count: group.rules.length }) }}</p>
            </div>
            <div class="flex items-center gap-2">
              <span v-if="groupError(group)" class="text-xs font-medium text-red-600 dark:text-red-400">{{ groupError(group) }}</span>
              <button class="btn btn-secondary btn-sm" @click="addRule(group)"><Icon name="plus" size="sm" />{{ t('timeBilling.addRule') }}</button>
              <button class="btn btn-primary btn-sm min-w-[76px] justify-center" :disabled="!isDirty(group) || !isGroupValid(group) || isSaving(group.id)" @click="saveGroup(group)">
                <Icon :name="isSaving(group.id) ? 'refresh' : 'check'" size="sm" :class="isSaving(group.id) ? 'animate-spin' : ''" />
                <span>{{ isSaving(group.id) ? t('common.saving') : t('common.save') }}</span>
              </button>
            </div>
          </header>

          <div v-if="group.rules.length" class="overflow-x-auto">
            <table class="w-full min-w-[1180px] table-fixed text-left text-sm">
              <colgroup><col class="w-[78px]" /><col class="w-[140px]" /><col class="w-[230px]" /><col class="w-[230px]" /><col class="w-[150px]" /><col class="w-[150px]" /><col class="w-[150px]" /><col class="w-[72px]" /></colgroup>
              <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">
                <tr>
                  <th class="px-4 py-2.5 text-center">{{ t('timeBilling.enabled') }}</th>
                  <th class="px-4 py-2.5">{{ t('timeBilling.repeatType') }}</th>
                  <th class="px-4 py-2.5">{{ t('timeBilling.start') }}</th>
                  <th class="px-4 py-2.5">{{ t('timeBilling.end') }}</th>
                  <th class="px-4 py-2.5">{{ t('timeBilling.multiplier') }}</th>
                  <th class="px-4 py-2.5 text-right">{{ t('timeBilling.effectiveMultiplier') }}</th>
                  <th class="px-4 py-2.5 text-center">{{ t('timeBilling.windowType') }}</th>
                  <th class="px-4 py-2.5 text-right">{{ t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="rule in group.rules" :key="rule.id" class="border-t border-gray-100 dark:border-dark-800">
                  <td class="px-4 py-3 text-center"><div class="inline-flex"><Toggle v-model="rule.enabled" /></div></td>
                  <td class="px-4 py-3"><Select v-model="rule.repeat_type" :options="repeatTypeOptions" /></td>
                  <td class="px-4 py-3">
                    <div class="grid grid-cols-[96px_minmax(0,1fr)] gap-2">
                      <Select v-if="rule.repeat_type === 'weekly'" v-model="rule.start_weekday" :options="weekdayOptions" />
                      <span v-else class="inline-flex h-10 items-center justify-center rounded-md bg-gray-100 px-2 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-dark-300">{{ t('timeBilling.everyDay') }}</span>
                      <input v-model="rule.start" class="input min-w-0" :class="ruleClass(group, rule)" type="time" />
                    </div>
                  </td>
                  <td class="px-4 py-3">
                    <div class="grid grid-cols-[96px_minmax(0,1fr)] gap-2">
                      <Select v-if="rule.repeat_type === 'weekly'" v-model="rule.end_weekday" :options="weekdayOptions" />
                      <span v-else class="inline-flex h-10 items-center justify-center rounded-md px-2 text-xs font-medium" :class="isCrossDay(rule) ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-dark-300'">
                        {{ isCrossDay(rule) ? t('timeBilling.nextDay') : t('timeBilling.sameDay') }}
                      </span>
                      <input v-model="rule.end" class="input min-w-0" :class="ruleClass(group, rule)" type="time" />
                    </div>
                  </td>
                  <td class="px-4 py-3"><input v-model.number="rule.rate_multiplier" class="input" :class="ruleClass(group, rule)" type="number" min="0" step="0.001" /></td>
                  <td class="px-4 py-3 text-right font-semibold text-primary-600 dark:text-primary-400">{{ effectiveMultiplier(group, rule) }}x</td>
                  <td class="px-4 py-3 text-center">
                    <div class="flex flex-col items-center gap-1">
                      <span v-if="isCrossPeriod(rule)" class="badge badge-warning">{{ rule.repeat_type === 'weekly' ? t('timeBilling.crossWeek') : t('timeBilling.crossDay') }}</span>
                      <span v-else class="badge badge-info">{{ rule.repeat_type === 'weekly' ? t('timeBilling.weeklyRecurring') : t('timeBilling.dailyRecurring') }}</span>
                      <span class="text-[11px] text-gray-400 dark:text-dark-500">{{ durationLabel(rule) }}</span>
                    </div>
                  </td>
                  <td class="px-4 py-3 text-right">
                    <button class="btn btn-secondary px-2" :title="t('common.delete')" @click="removeRule(group, rule.id)"><Icon name="trash" size="sm" /></button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-else class="px-4 py-10 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('timeBilling.noRules') }}</div>
        </section>
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
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI } from '@/api/admin'
import type { AdminGroup, GroupPlatform, TimeBillingRepeatType, TimeBillingRule } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { serverTimezoneLabel } from '@/utils/peak-rate'

type RuleStateFilter = 'all' | 'enabled' | 'disabled' | 'unconfigured' | 'cross_day'
type GroupStatusFilter = 'all' | 'active' | 'inactive'

interface TimeBillingGroupRow {
  id: number
  name: string
  platform: GroupPlatform
  status: 'active' | 'inactive'
  baseMultiplier: number
  rules: EditableTimeBillingRule[]
}

interface MinuteRange { start: number; end: number }
interface EditableTimeBillingRule {
  id: string
  enabled: boolean
  repeat_type: TimeBillingRepeatType
  start_weekday: number
  end_weekday: number
  start: string
  end: string
  rate_multiplier: number
}

const MINUTES_PER_DAY = 24 * 60
const MINUTES_PER_WEEK = 7 * MINUTES_PER_DAY

const { t } = useI18n()
const appStore = useAppStore()
const groups = ref<TimeBillingGroupRow[]>([])
const search = ref('')
const platformFilter = ref<GroupPlatform | 'all'>('all')
const groupStatusFilter = ref<GroupStatusFilter>('all')
const ruleStateFilter = ref<RuleStateFilter>('all')
const loading = ref(false)
const savingIds = ref(new Set<number>())
const savedSignatures = ref<Record<number, string>>({})
const timezoneLabel = computed(() => serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset))
const repeatTypeOptions = computed(() => [
  { value: 'daily', label: t('timeBilling.dailyRecurring') },
  { value: 'weekly', label: t('timeBilling.weeklyRecurring') },
])
const weekdayOptions = computed(() => Array.from({ length: 7 }, (_, index) => ({
  value: index + 1,
  label: t(`timeBilling.weekdays.${index + 1}`),
})))

const platformOptions = computed(() => [
  { value: 'all', label: t('timeBilling.allPlatforms') },
  ...(['anthropic', 'openai', 'gemini', 'antigravity', 'grok'] as GroupPlatform[]).map((value) => ({ value, label: platformLabel(value) })),
])
const groupStatusOptions = computed(() => [
  { value: 'all', label: t('timeBilling.allGroupStatuses') },
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') },
])
const ruleStateOptions = computed(() => [
  { value: 'all', label: t('timeBilling.allRuleStates') },
  { value: 'enabled', label: t('timeBilling.enabledRules') },
  { value: 'disabled', label: t('timeBilling.disabledRules') },
  { value: 'unconfigured', label: t('timeBilling.unconfigured') },
  { value: 'cross_day', label: t('timeBilling.crossDayRules') },
])

const filteredGroups = computed(() => groups.value.filter((group) => {
  if (platformFilter.value !== 'all' && group.platform !== platformFilter.value) return false
  if (groupStatusFilter.value !== 'all' && group.status !== groupStatusFilter.value) return false
  if (ruleStateFilter.value === 'enabled' && !group.rules.some((rule) => rule.enabled)) return false
  if (ruleStateFilter.value === 'disabled' && !(group.rules.length > 0 && group.rules.every((rule) => !rule.enabled))) return false
  if (ruleStateFilter.value === 'unconfigured' && group.rules.length > 0) return false
  if (ruleStateFilter.value === 'cross_day' && !group.rules.some((rule) => rule.enabled && isCrossPeriod(rule))) return false
  const query = search.value.toLocaleLowerCase()
  if (!query) return true
  const ruleText = group.rules.map((rule) => `${rule.repeat_type} ${rule.start_weekday} ${rule.start} ${rule.end_weekday} ${rule.end} ${rule.rate_multiplier}`).join(' ')
  return `${group.name} ${platformLabel(group.platform)} ${ruleText}`.toLocaleLowerCase().includes(query)
}))

function normalizeTime(value: string): string { return String(value || '').slice(0, 5) }
function cloneRule(rule: TimeBillingRule): EditableTimeBillingRule {
  return {
    id: rule.id,
    enabled: Boolean(rule.enabled),
    repeat_type: rule.repeat_type === 'weekly' ? 'weekly' : 'daily',
    start_weekday: Number(rule.start_weekday || 1),
    end_weekday: Number(rule.end_weekday || 1),
    start: normalizeTime(rule.start),
    end: normalizeTime(rule.end),
    rate_multiplier: Number(rule.rate_multiplier ?? 1),
  }
}
function serializeRule(rule: EditableTimeBillingRule): TimeBillingRule {
  const serialized: TimeBillingRule = {
    id: rule.id,
    enabled: Boolean(rule.enabled),
    repeat_type: rule.repeat_type,
    start: normalizeTime(rule.start),
    end: normalizeTime(rule.end),
    rate_multiplier: Number(rule.rate_multiplier ?? 1),
  }
  if (rule.repeat_type === 'weekly') {
    serialized.start_weekday = Number(rule.start_weekday)
    serialized.end_weekday = Number(rule.end_weekday)
  }
  return serialized
}
function groupFromAPI(group: AdminGroup): TimeBillingGroupRow {
  let rules = (group.time_billing_rules || []).map(cloneRule)
  if (!rules.length && group.peak_rate_enabled && group.peak_start && group.peak_end) {
    rules = [cloneRule({ id: `legacy-${group.id}`, enabled: true, start: normalizeTime(group.peak_start), end: normalizeTime(group.peak_end), rate_multiplier: Number(group.peak_rate_multiplier ?? 1) })]
  }
  return { id: group.id, name: group.name, platform: group.platform, status: group.status, baseMultiplier: Number(group.rate_multiplier || 0), rules }
}
function signature(group: TimeBillingGroupRow): string { return JSON.stringify(group.rules) }
function isDirty(group: TimeBillingGroupRow): boolean { return savedSignatures.value[group.id] !== signature(group) }
function isSaving(id: number): boolean { return savingIds.value.has(id) }
function parseMinute(value: string): number | null {
  const match = /^(\d{2}):(\d{2})$/.exec(value)
  if (!match) return null
  const hour = Number(match[1]); const minute = Number(match[2])
  return hour <= 23 && minute <= 59 ? hour * 60 + minute : null
}
function splitRange(start: number, end: number): MinuteRange[] {
  if (start < end) return [{ start, end }]
  return end > 0 ? [{ start, end: MINUTES_PER_WEEK }, { start: 0, end }] : [{ start, end: MINUTES_PER_WEEK }]
}
function ruleRanges(rule: EditableTimeBillingRule): MinuteRange[] {
  const start = parseMinute(rule.start); const end = parseMinute(rule.end)
  if (start === null || end === null) return []
  if (rule.repeat_type === 'weekly') {
    if (rule.start_weekday < 1 || rule.start_weekday > 7 || rule.end_weekday < 1 || rule.end_weekday > 7) return []
    const absoluteStart = (rule.start_weekday - 1) * MINUTES_PER_DAY + start
    const absoluteEnd = (rule.end_weekday - 1) * MINUTES_PER_DAY + end
    if (absoluteStart === absoluteEnd) return []
    return splitRange(absoluteStart, absoluteEnd)
  }
  if (start === end) return []
  let duration = end - start
  if (duration <= 0) duration += MINUTES_PER_DAY
  const ranges: MinuteRange[] = []
  for (let day = 0; day < 7; day++) {
    const absoluteStart = day * MINUTES_PER_DAY + start
    const absoluteEnd = absoluteStart + duration
    if (absoluteEnd <= MINUTES_PER_WEEK) ranges.push({ start: absoluteStart, end: absoluteEnd })
    else ranges.push({ start: absoluteStart, end: MINUTES_PER_WEEK }, { start: 0, end: absoluteEnd - MINUTES_PER_WEEK })
  }
  return ranges
}
function rulesOverlap(left: EditableTimeBillingRule, right: EditableTimeBillingRule): boolean {
  return ruleRanges(left).some((a) => ruleRanges(right).some((b) => a.start < b.end && b.start < a.end))
}
function overlappingIDs(group: TimeBillingGroupRow): Set<string> {
  const result = new Set<string>()
  const enabled = group.rules.filter((rule) => rule.enabled)
  for (let i = 0; i < enabled.length; i++) {
    for (let j = i + 1; j < enabled.length; j++) {
      if (rulesOverlap(enabled[i], enabled[j])) { result.add(enabled[i].id); result.add(enabled[j].id) }
    }
  }
  return result
}
function isRuleValid(rule: EditableTimeBillingRule): boolean {
  return ruleRanges(rule).length > 0 && Number.isFinite(Number(rule.rate_multiplier)) && Number(rule.rate_multiplier) >= 0
}
function isGroupValid(group: TimeBillingGroupRow): boolean { return group.rules.every(isRuleValid) && overlappingIDs(group).size === 0 }
function groupError(group: TimeBillingGroupRow): string {
  if (overlappingIDs(group).size > 0) return t('timeBilling.overlapError')
  if (!group.rules.every(isRuleValid)) return t('timeBilling.invalidRule')
  return ''
}
function ruleClass(group: TimeBillingGroupRow, rule: EditableTimeBillingRule): string {
  return !isRuleValid(rule) || overlappingIDs(group).has(rule.id) ? 'border-red-400 focus:border-red-500 focus:ring-red-500' : ''
}
function isCrossDay(rule: EditableTimeBillingRule): boolean {
  const start = parseMinute(rule.start); const end = parseMinute(rule.end)
  return rule.repeat_type === 'daily' && start !== null && end !== null && start > end
}
function isCrossWeek(rule: EditableTimeBillingRule): boolean {
  if (rule.repeat_type !== 'weekly') return false
  const start = parseMinute(rule.start); const end = parseMinute(rule.end)
  if (start === null || end === null) return false
  return (rule.end_weekday - 1) * MINUTES_PER_DAY + end < (rule.start_weekday - 1) * MINUTES_PER_DAY + start
}
function isCrossPeriod(rule: EditableTimeBillingRule): boolean { return isCrossDay(rule) || isCrossWeek(rule) }
function ruleDuration(rule: EditableTimeBillingRule): number {
  const start = parseMinute(rule.start); const end = parseMinute(rule.end)
  if (start === null || end === null) return 0
  if (rule.repeat_type === 'daily') {
    if (start === end) return 0
    return (end - start + MINUTES_PER_DAY) % MINUTES_PER_DAY
  }
  const absoluteStart = (rule.start_weekday - 1) * MINUTES_PER_DAY + start
  const absoluteEnd = (rule.end_weekday - 1) * MINUTES_PER_DAY + end
  if (absoluteStart === absoluteEnd) return 0
  return (absoluteEnd - absoluteStart + MINUTES_PER_WEEK) % MINUTES_PER_WEEK
}
function durationLabel(rule: EditableTimeBillingRule): string {
  const duration = ruleDuration(rule)
  if (!duration) return t('timeBilling.invalidDuration')
  const days = Math.floor(duration / MINUTES_PER_DAY)
  const hours = Math.floor((duration % MINUTES_PER_DAY) / 60)
  const minutes = duration % 60
  if (days > 0) return t('timeBilling.durationDays', { days, hours, minutes })
  return t('timeBilling.durationHours', { hours, minutes })
}
function formatMultiplier(value: number): string { return Number(value || 0).toFixed(3) }
function effectiveMultiplier(group: TimeBillingGroupRow, rule: EditableTimeBillingRule): string { return formatMultiplier(group.baseMultiplier * Number(rule.rate_multiplier || 0)) }
function platformLabel(platform: GroupPlatform): string { return t(`admin.groups.platforms.${platform}`) }
function addRule(group: TimeBillingGroupRow): void {
  group.rules.push({ id: crypto.randomUUID(), enabled: false, repeat_type: 'daily', start_weekday: 1, end_weekday: 1, start: '09:00', end: '18:00', rate_multiplier: 1 })
}
function removeRule(group: TimeBillingGroupRow, id: string): void { group.rules = group.rules.filter((rule) => rule.id !== id) }
function setSaving(id: number, saving: boolean): void {
  const next = new Set(savingIds.value)
  if (saving) next.add(id); else next.delete(id)
  savingIds.value = next
}
async function loadGroups(): Promise<void> {
  loading.value = true
  try {
    groups.value = (await adminAPI.groups.getAllIncludingInactive()).map(groupFromAPI)
    savedSignatures.value = Object.fromEntries(groups.value.map((group) => [group.id, signature(group)]))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('timeBilling.loadFailed')))
  } finally { loading.value = false }
}
async function saveGroup(group: TimeBillingGroupRow): Promise<void> {
  if (!isGroupValid(group)) { appStore.showError(groupError(group)); return }
  setSaving(group.id, true)
  try {
    const updated = await adminAPI.groups.update(group.id, { time_billing_rules: group.rules.map(serializeRule) })
    const next = groupFromAPI(updated)
    Object.assign(group, next)
    savedSignatures.value = { ...savedSignatures.value, [group.id]: signature(group) }
    appStore.showSuccess(t('timeBilling.saveSuccess', { group: group.name }))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('timeBilling.saveFailed')))
  } finally { setSaving(group.id, false) }
}

onMounted(() => void loadGroups())
</script>
