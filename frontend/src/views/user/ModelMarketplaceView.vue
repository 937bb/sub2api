<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-4">
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div>
              <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('modelMarketplace.title') }}</h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('modelMarketplace.description') }}</p>
            </div>
            <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh', '刷新')" @click="loadCatalog">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>

          <div class="grid gap-3 lg:grid-cols-[minmax(240px,1fr)_minmax(280px,1.35fr)_minmax(180px,0.7fr)_minmax(160px,0.6fr)]">
            <label class="block">
              <span class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('modelMarketplace.selectGroup') }}</span>
              <select v-model.number="selectedGroupId" class="input">
                <option v-for="option in groupOptions" :key="option.group.id" :value="option.group.id">
                  {{ option.group.name }} · {{ option.group.platform }} · {{ t('modelMarketplace.groupModelCount', { count: option.count }) }}
                </option>
              </select>
            </label>
            <label class="block">
              <span class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('common.search', '搜索') }}</span>
              <span class="relative block">
                <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input v-model="search" class="input pl-10" :placeholder="t('modelMarketplace.searchPlaceholder')" />
              </span>
            </label>
            <label class="block">
              <span class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('modelMarketplace.columns.mode') }}</span>
              <select v-model="billingMode" class="input">
                <option value="all">{{ t('modelMarketplace.allBillingModes') }}</option>
                <option value="token">{{ t('modelMarketplace.billing.token') }}</option>
                <option value="per_request">{{ t('modelMarketplace.billing.perRequest') }}</option>
                <option value="image">{{ t('modelMarketplace.billing.image') }}</option>
                <option value="video">{{ t('modelMarketplace.billing.video') }}</option>
              </select>
            </label>
            <label class="block">
              <span class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('modelMarketplace.sort.label') }}</span>
              <select v-model="sortBy" class="input">
                <option value="name">{{ t('modelMarketplace.sort.name') }}</option>
                <option value="price">{{ t('modelMarketplace.sort.price') }}</option>
              </select>
            </label>
          </div>

          <div v-if="selectedGroup" class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-200 pt-3 dark:border-dark-700">
            <div class="flex min-w-0 flex-wrap items-center gap-2">
              <span class="truncate font-semibold text-gray-900 dark:text-white">{{ selectedGroup.name }}</span>
              <span class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-300">{{ selectedGroup.platform }}</span>
              <span class="rounded px-2 py-1 text-xs" :class="selectedGroup.is_exclusive ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300' : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'">
                {{ selectedGroup.is_exclusive ? t('modelMarketplace.exclusiveGroup') : t('modelMarketplace.publicGroup') }}
              </span>
              <span v-if="selectedGroup.subscription_type === 'subscription'" class="rounded bg-sky-50 px-2 py-1 text-xs text-sky-700 dark:bg-sky-900/20 dark:text-sky-300">{{ t('modelMarketplace.subscriptionGroup') }}</span>
            </div>
            <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
              <span>{{ t('modelMarketplace.groupModelCount', { count: filteredRows.length }) }}</span>
              <span>{{ t('modelMarketplace.groupRate') }} <strong class="font-mono text-gray-900 dark:text-white">{{ formatMultiplier(selectedGroup.effective_rate_multiplier) }}x</strong></span>
              <span v-if="selectedGroup.user_rate_multiplier != null" class="text-primary-600 dark:text-primary-400">{{ t('modelMarketplace.customRate') }}</span>
              <span v-if="selectedGroup.time_rate_multiplier !== 1" class="text-amber-600 dark:text-amber-400">{{ t('modelMarketplace.timeRate', { value: formatMultiplier(selectedGroup.time_rate_multiplier) }) }}</span>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <div v-if="loading" class="flex justify-center py-16"><Icon name="refresh" size="lg" class="animate-spin text-primary-500" /></div>
        <div v-else-if="groupOptions.length === 0" class="py-16 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('modelMarketplace.noGroups') }}</div>
        <div v-else-if="filteredRows.length === 0" class="py-16 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('modelMarketplace.empty') }}</div>
        <div v-else class="table-wrapper">
          <table class="w-full min-w-[1180px] table-fixed text-left text-sm">
            <colgroup>
              <col class="w-[25%]" />
              <col class="w-[11%]" />
              <col class="w-[11%]" />
              <col class="w-[11%]" />
              <col class="w-[11%]" />
              <col class="w-[11%]" />
              <col class="w-[11%]" />
              <col class="w-[9%]" />
            </colgroup>
            <thead class="sticky top-0 z-10 border-b border-gray-100 bg-gray-50/95 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800/95 dark:text-dark-400">
              <tr>
                <th class="px-5 py-3">{{ t('modelMarketplace.columns.model') }}</th>
                <th class="px-4 py-3">{{ t('modelMarketplace.columns.mode') }}</th>
                <th class="px-4 py-3 text-right">{{ t('modelMarketplace.columns.rate') }}</th>
                <th class="px-4 py-3 text-right"><span class="block">{{ t('modelMarketplace.columns.input') }}</span><span class="font-normal text-gray-400">{{ t('modelMarketplace.perMillion') }}</span></th>
                <th class="px-4 py-3 text-right"><span class="block">{{ t('modelMarketplace.columns.output') }}</span><span class="font-normal text-gray-400">{{ t('modelMarketplace.perMillion') }}</span></th>
                <th class="px-4 py-3 text-right"><span class="block">{{ t('modelMarketplace.columns.cacheWrite') }}</span><span class="font-normal text-gray-400">{{ t('modelMarketplace.perMillion') }}</span></th>
                <th class="px-4 py-3 text-right"><span class="block">{{ t('modelMarketplace.columns.cacheRead') }}</span><span class="font-normal text-gray-400">{{ t('modelMarketplace.perMillion') }}</span></th>
                <th class="px-4 py-3 text-right"><span class="block">{{ t('modelMarketplace.columns.perRequest') }}</span><span class="font-normal text-gray-400">{{ t('modelMarketplace.perRequest') }}</span></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in filteredRows" :key="`${row.model.platform}:${row.model.name}:${row.group.id}`" class="border-b border-gray-100 last:border-b-0 hover:bg-gray-50/60 dark:border-dark-800 dark:hover:bg-dark-800/40">
                <td class="px-5 py-4">
                  <div class="break-all font-mono font-semibold text-gray-900 dark:text-white">{{ row.model.name }}</div>
                  <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
                    <span>{{ row.model.platform }}</span>
                    <span v-if="row.group.pricing?.intervals?.length" class="rounded bg-amber-50 px-1.5 py-0.5 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">{{ t('modelMarketplace.tiered', { count: row.group.pricing.intervals.length }) }}</span>
                  </div>
                </td>
                <td class="px-4 py-4"><span class="inline-flex rounded bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-700 dark:text-dark-200">{{ billingLabel(row.model.billing_mode) }}</span></td>
                <td class="px-4 py-4 text-right">
                  <span class="font-mono font-semibold text-gray-900 dark:text-white">{{ formatMultiplier(rowRate(row)) }}x</span>
                  <div v-if="row.group.user_rate_multiplier != null || row.group.time_rate_multiplier !== 1" class="mt-1 text-[11px]">
                    <span v-if="row.group.user_rate_multiplier != null" class="text-primary-600 dark:text-primary-400">{{ t('modelMarketplace.customRate') }}</span>
                    <span v-if="row.group.time_rate_multiplier !== 1" class="ml-1 text-amber-600 dark:text-amber-400">{{ t('modelMarketplace.timeRate', { value: formatMultiplier(row.group.time_rate_multiplier) }) }}</span>
                  </div>
                </td>
                <td class="px-4 py-4 text-right font-mono tabular-nums text-gray-800 dark:text-dark-100">{{ tokenPrice(row, 'input_price') }}</td>
                <td class="px-4 py-4 text-right font-mono tabular-nums text-gray-800 dark:text-dark-100">{{ tokenPrice(row, 'output_price') }}</td>
                <td class="px-4 py-4 text-right font-mono tabular-nums text-gray-800 dark:text-dark-100">{{ tokenPrice(row, 'cache_write_price') }}</td>
                <td class="px-4 py-4 text-right font-mono tabular-nums text-gray-800 dark:text-dark-100">{{ tokenPrice(row, 'cache_read_price') }}</td>
                <td class="px-4 py-4 text-right font-mono tabular-nums text-gray-800 dark:text-dark-100">{{ requestPrice(row) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  getCatalog,
  type ModelCatalogGroup,
  type ModelCatalogModel,
  type ModelCatalogPricing
} from '@/api/models'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatMultiplier } from '@/utils/formatters'
import { formatScaled } from '@/utils/pricing'

const { t } = useI18n()
const appStore = useAppStore()
const models = ref<ModelCatalogModel[]>([])
const loading = ref(false)
const search = ref('')
const selectedGroupId = ref<number | null>(null)
const billingMode = ref('all')
const sortBy = ref<'name' | 'price'>('name')

interface GroupOption {
  group: ModelCatalogGroup
  count: number
}

interface MarketplaceRow {
  model: ModelCatalogModel
  group: ModelCatalogGroup
}

type TokenPriceKey = 'input_price' | 'output_price' | 'cache_write_price' | 'cache_read_price'

const groupOptions = computed<GroupOption[]>(() => {
  const grouped = new Map<number, { group: ModelCatalogGroup; models: Set<string> }>()
  for (const model of models.value) {
    for (const group of model.groups) {
      const current = grouped.get(group.id) ?? { group, models: new Set<string>() }
      current.models.add(`${model.platform}:${model.name.toLowerCase()}`)
      grouped.set(group.id, current)
    }
  }
  return [...grouped.values()]
    .map((entry) => ({ group: entry.group, count: entry.models.size }))
    .sort((a, b) => a.group.name.localeCompare(b.group.name) || a.group.id - b.group.id)
})

watch(
  groupOptions,
  (options) => {
    if (!options.some((option) => option.group.id === selectedGroupId.value)) {
      selectedGroupId.value = options[0]?.group.id ?? null
    }
  },
  { immediate: true }
)

const selectedGroup = computed(() => groupOptions.value.find((option) => option.group.id === selectedGroupId.value)?.group ?? null)

const filteredRows = computed<MarketplaceRow[]>(() => {
  const query = search.value.trim().toLowerCase()
  const result: MarketplaceRow[] = []
  if (selectedGroupId.value == null) return result

  for (const model of models.value) {
    const group = model.groups.find((entry) => entry.id === selectedGroupId.value)
    if (!group) continue
    if (query && !`${model.name} ${model.platform}`.toLowerCase().includes(query)) continue
    if (billingMode.value !== 'all' && model.billing_mode !== billingMode.value) continue
    result.push({ model, group })
  }

  return result.sort((a, b) => {
    if (sortBy.value === 'price') return sortablePrice(a) - sortablePrice(b)
    return a.model.name.localeCompare(b.model.name)
  })
})

function rowRate(row: MarketplaceRow): number {
  return row.group.billing_rate_multiplier ?? row.group.effective_rate_multiplier
}

function tokenPrice(row: MarketplaceRow, key: TokenPriceKey): string {
  if (row.model.billing_mode !== 'token') return '-'
  return formatScaled(row.group.pricing?.[key] ?? null, 1_000_000)
}

function requestPriceValue(row: MarketplaceRow): number | null {
  const pricing = row.group.pricing
  if (!pricing || row.model.billing_mode === 'token') return null
  return pricing.per_request_price ?? pricing.image_output_price ?? null
}

function requestPrice(row: MarketplaceRow): string {
  return formatScaled(requestPriceValue(row), 1)
}

function sortablePrice(row: MarketplaceRow): number {
  if (row.model.billing_mode !== 'token') return requestPriceValue(row) ?? Number.POSITIVE_INFINITY
  return firstPrice(row.group.pricing)
}

function firstPrice(pricing: ModelCatalogPricing | null): number {
  return pricing?.input_price ?? pricing?.output_price ?? pricing?.cache_write_price ?? pricing?.cache_read_price ?? Number.POSITIVE_INFINITY
}

function billingLabel(mode: string): string {
  const labels: Record<string, string> = { token: t('modelMarketplace.billing.token'), per_request: t('modelMarketplace.billing.perRequest'), image: t('modelMarketplace.billing.image'), video: t('modelMarketplace.billing.video') }
  return labels[mode] || mode
}

async function loadCatalog(): Promise<void> {
  loading.value = true
  try { const data = await getCatalog(); models.value = data.models }
  catch (error) { appStore.showError(extractApiErrorMessage(error, t('modelMarketplace.loadFailed'))) }
  finally { loading.value = false }
}
onMounted(loadCatalog)
</script>
