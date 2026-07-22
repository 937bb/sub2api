<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-4">
          <div class="flex flex-wrap items-end justify-between gap-4">
            <div>
              <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('modelMarketplace.title') }}</h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('modelMarketplace.description') }}</p>
            </div>
            <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh', '刷新')" @click="loadCatalog">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>

          <div class="grid gap-3 lg:grid-cols-[minmax(240px,1.5fr)_repeat(4,minmax(140px,1fr))]">
            <label class="relative block">
              <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input v-model="search" class="input pl-10" :placeholder="t('modelMarketplace.searchPlaceholder')" />
            </label>
            <select v-model="platform" class="input"><option value="all">{{ t('modelMarketplace.allPlatforms') }}</option><option v-for="value in platforms" :key="value" :value="value">{{ value }}</option></select>
            <select v-model="group" class="input"><option value="all">{{ t('modelMarketplace.allGroups') }}</option><option v-for="value in groups" :key="value">{{ value }}</option></select>
            <select v-model="billingMode" class="input"><option value="all">{{ t('modelMarketplace.allBillingModes') }}</option><option value="token">{{ t('modelMarketplace.billing.token') }}</option><option value="per_request">{{ t('modelMarketplace.billing.perRequest') }}</option><option value="image">{{ t('modelMarketplace.billing.image') }}</option><option value="video">{{ t('modelMarketplace.billing.video') }}</option></select>
            <select v-model="sortBy" class="input"><option value="name">{{ t('modelMarketplace.sort.name') }}</option><option value="price">{{ t('modelMarketplace.sort.price') }}</option><option value="rate">{{ t('modelMarketplace.sort.rate') }}</option></select>
          </div>
          <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
            <span class="font-medium text-gray-700 dark:text-dark-200">{{ t('modelMarketplace.visibleCount', { count: filteredModels.length }) }}</span>
            <span>{{ t('modelMarketplace.identityHint') }}</span>
          </div>
        </div>
      </template>

      <template #table>
        <div v-if="loading" class="flex justify-center py-16"><Icon name="refresh" size="lg" class="animate-spin text-primary-500" /></div>
        <div v-else-if="filteredModels.length === 0" class="py-16 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('modelMarketplace.empty') }}</div>
        <div v-else class="table-wrapper">
          <table class="w-full min-w-[980px] text-left text-sm">
            <thead class="sticky top-0 z-10 border-b border-gray-100 bg-gray-50/95 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800/95 dark:text-dark-400">
              <tr><th class="px-5 py-3">{{ t('modelMarketplace.columns.model') }}</th><th class="px-5 py-3">{{ t('modelMarketplace.columns.mode') }}</th><th class="px-5 py-3">{{ t('modelMarketplace.columns.groups') }}</th><th class="px-5 py-3">{{ t('modelMarketplace.columns.rate') }}</th><th class="px-5 py-3">{{ t('modelMarketplace.columns.price') }}</th><th class="px-5 py-3 text-right">{{ t('modelMarketplace.columns.channels') }}</th></tr>
            </thead>
            <tbody>
              <tr v-for="model in filteredModels" :key="`${model.platform}:${model.name}`" class="border-b border-gray-100 align-top last:border-b-0 dark:border-dark-800">
                <td class="px-5 py-4"><div class="font-mono font-semibold text-gray-900 dark:text-white">{{ model.name }}</div><div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ model.platform }}</div></td>
                <td class="px-5 py-4"><span class="rounded bg-gray-100 px-2 py-1 text-xs dark:bg-dark-700">{{ billingLabel(model.billing_mode) }}</span></td>
                <td class="px-5 py-4"><div class="flex max-w-[300px] flex-wrap gap-1.5"><span v-for="item in model.groups" :key="item.id" class="rounded-md px-2 py-1 text-xs" :class="item.is_exclusive ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300' : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'">{{ item.name }}</span></div></td>
                <td class="px-5 py-4"><div v-for="item in model.groups" :key="`rate-${item.id}`" class="mb-1 whitespace-nowrap last:mb-0"><span class="font-mono font-semibold text-gray-900 dark:text-white">{{ formatMultiplier(item.effective_rate_multiplier) }}x</span><span v-if="item.user_rate_multiplier != null" class="ml-1 text-[11px] text-primary-600">{{ t('modelMarketplace.customRate') }}</span><span v-if="item.time_rate_multiplier !== 1" class="ml-1 text-[11px] text-amber-600">{{ t('modelMarketplace.timeRate', { value: formatMultiplier(item.time_rate_multiplier) }) }}</span></div></td>
                <td class="px-5 py-4"><div v-for="item in model.groups" :key="`price-${item.id}`" class="mb-1 whitespace-nowrap last:mb-0"><template v-if="item.pricing"><span v-if="item.pricing.billing_mode === 'per_request' || item.pricing.billing_mode === 'image' || item.pricing.billing_mode === 'video'">{{ formatPrice(item.pricing.per_request_price, 1) }}</span><span v-else>{{ formatPrice(item.pricing.input_price, 1000000) }} <span class="text-gray-400">/</span> {{ formatPrice(item.pricing.output_price, 1000000) }}</span></template><span v-else class="text-gray-400">{{ t('modelMarketplace.notConfigured') }}</span></div></td>
                <td class="px-5 py-4 text-right text-gray-500 dark:text-dark-400">{{ model.channel_count }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { getCatalog, type ModelCatalogModel } from '@/api/models'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatMultiplier } from '@/utils/formatters'
import { formatScaled } from '@/utils/pricing'

const { t } = useI18n()
const appStore = useAppStore()
const models = ref<ModelCatalogModel[]>([])
const loading = ref(false)
const search = ref('')
const platform = ref('all')
const group = ref('all')
const billingMode = ref('all')
const sortBy = ref<'name' | 'price' | 'rate'>('name')

const platforms = computed(() => [...new Set(models.value.map((item) => item.platform))].sort())
const groups = computed(() => [...new Set(models.value.flatMap((item) => item.groups.map((group) => group.name)))].sort())
const filteredModels = computed(() => {
  const query = search.value.trim().toLowerCase()
  const result = models.value.filter((item) => {
    const matchesSearch = !query || `${item.name} ${item.platform} ${item.groups.map((group) => group.name).join(' ')}`.toLowerCase().includes(query)
    const matchesPlatform = platform.value === 'all' || item.platform === platform.value
    const matchesGroup = group.value === 'all' || item.groups.some((entry) => entry.name === group.value)
    const matchesMode = billingMode.value === 'all' || item.billing_mode === billingMode.value
    return matchesSearch && matchesPlatform && matchesGroup && matchesMode
  })
  return result.sort((a, b) => {
    if (sortBy.value === 'rate') return lowestRate(a) - lowestRate(b)
    if (sortBy.value === 'price') return lowestPrice(a) - lowestPrice(b)
    return a.name.localeCompare(b.name)
  })
})

function lowestRate(model: ModelCatalogModel): number { return Math.min(...model.groups.map((item) => item.effective_rate_multiplier), Number.POSITIVE_INFINITY) }
function lowestPrice(model: ModelCatalogModel): number { return Math.min(...model.groups.map((item) => item.pricing?.input_price ?? item.pricing?.per_request_price ?? Number.POSITIVE_INFINITY)) }
function formatPrice(value: number | null, scale: number): string { return formatScaled(value, scale) }
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
