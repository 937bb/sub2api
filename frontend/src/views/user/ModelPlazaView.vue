<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Search + Platform filter -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-80">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('modelPlaza.searchPlaceholder')"
                class="input pl-10"
              />
            </div>
            <!-- Platform filter pills -->
            <div class="flex flex-wrap items-center gap-1.5">
              <button
                type="button"
                :class="[
                  'rounded-full border px-3 py-1 text-xs font-medium transition-colors',
                  !selectedPlatform
                    ? 'border-primary-500 bg-primary-500/10 text-primary-600 dark:text-primary-400'
                    : 'border-gray-200 text-gray-500 hover:border-gray-300 dark:border-dark-600 dark:text-gray-400 dark:hover:border-dark-500'
                ]"
                @click="selectedPlatform = ''"
              >
                {{ t('modelPlaza.allPlatforms') }}
              </button>
              <button
                v-for="p in availablePlatforms"
                :key="p"
                type="button"
                :class="[
                  'inline-flex items-center gap-1 rounded-full border px-3 py-1 text-xs font-medium transition-colors',
                  selectedPlatform === p
                    ? platformBadgeClass(p)
                    : 'border-gray-200 text-gray-500 hover:border-gray-300 dark:border-dark-600 dark:text-gray-400 dark:hover:border-dark-500'
                ]"
                @click="selectedPlatform = p"
              >
                <PlatformIcon :platform="p as GroupPlatform" size="xs" />
                {{ platformLabel(p) }}
              </button>
            </div>
          </div>

          <!-- Right: stats + refresh -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('modelPlaza.modelCount', { count: filteredModels.length }) }}
            </span>
            <button
              @click="loadData"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <!-- Loading -->
        <div v-if="loading" class="flex items-center justify-center py-16">
          <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
        </div>

        <!-- Empty -->
        <div v-else-if="filteredModels.length === 0" class="py-16 text-center">
          <Icon name="inbox" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-400" />
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('modelPlaza.empty') }}</p>
        </div>

        <!-- Model cards grid -->
        <div v-else class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          <div
            v-for="model in filteredModels"
            :key="`${model.platform}-${model.name}`"
            class="group relative overflow-hidden rounded-xl border transition-all duration-200 hover:shadow-card-hover"
            :class="platformBorderClass(model.platform)"
          >
            <!-- Accent bar top -->
            <div class="h-1" :class="platformAccentBarClass(model.platform)" />

            <div class="p-4">
              <!-- Header: model name + platform badge -->
              <div class="mb-3 flex items-start justify-between gap-2">
                <h3 class="min-w-0 break-all text-sm font-semibold text-gray-900 dark:text-white">
                  {{ model.name }}
                </h3>
                <span
                  :class="[
                    'inline-flex flex-shrink-0 items-center gap-1 rounded-md border px-2 py-0.5 text-[10px] font-medium uppercase',
                    platformBadgeClass(model.platform),
                  ]"
                >
                  <PlatformIcon :platform="model.platform as GroupPlatform" size="xs" />
                  {{ model.platform }}
                </span>
              </div>

              <!-- Pricing section -->
              <div v-if="model.pricing" class="mb-3 space-y-1.5">
                <div class="grid grid-cols-2 gap-x-3 gap-y-1 text-[11px]">
                  <div v-if="model.pricing.input_price != null" class="flex items-baseline justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('modelPlaza.pricing.input') }}</span>
                    <span class="font-medium text-gray-800 dark:text-gray-200">{{ formatPrice(model.pricing.input_price) }}</span>
                  </div>
                  <div v-if="model.pricing.output_price != null" class="flex items-baseline justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('modelPlaza.pricing.output') }}</span>
                    <span class="font-medium text-gray-800 dark:text-gray-200">{{ formatPrice(model.pricing.output_price) }}</span>
                  </div>
                  <div v-if="model.pricing.cache_write_price != null" class="flex items-baseline justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('modelPlaza.pricing.cacheWrite') }}</span>
                    <span class="font-medium text-gray-800 dark:text-gray-200">{{ formatPrice(model.pricing.cache_write_price) }}</span>
                  </div>
                  <div v-if="model.pricing.cache_read_price != null" class="flex items-baseline justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('modelPlaza.pricing.cacheRead') }}</span>
                    <span class="font-medium text-gray-800 dark:text-gray-200">{{ formatPrice(model.pricing.cache_read_price) }}</span>
                  </div>
                  <div v-if="model.pricing.image_output_price != null && model.pricing.image_output_price > 0" class="flex items-baseline justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('modelPlaza.pricing.imageOutput') }}</span>
                    <span class="font-medium text-gray-800 dark:text-gray-200">{{ formatPrice(model.pricing.image_output_price) }}</span>
                  </div>
                </div>
              </div>
              <div v-else class="mb-3 text-[11px] text-gray-400 dark:text-gray-500">
                {{ t('modelPlaza.noPricing') }}
              </div>

              <!-- Groups section -->
              <div class="border-t border-gray-100 pt-2 dark:border-dark-700">
                <div class="mb-1 text-[10px] font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
                  {{ t('modelPlaza.availableGroups') }}
                </div>
                <div class="flex flex-wrap gap-1">
                  <span
                    v-for="group in model.groups"
                    :key="group.id"
                    :class="[
                      'inline-flex items-center gap-0.5 rounded-md border px-1.5 py-0.5 text-[10px] font-medium',
                      group.is_exclusive
                        ? 'border-purple-200 bg-purple-50 text-purple-600 dark:border-purple-800 dark:bg-purple-900/20 dark:text-purple-400'
                        : 'border-gray-200 bg-gray-50 text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400'
                    ]"
                    :title="group.is_exclusive ? t('modelPlaza.exclusiveGroup') : t('modelPlaza.publicGroup')"
                  >
                    <Icon
                      :name="group.is_exclusive ? 'shield' : 'globe'"
                      size="xs"
                      class="h-2.5 w-2.5"
                    />
                    {{ group.name }}
                    <template v-if="group.rate_multiplier !== 1">
                      <span class="opacity-60">×{{ formatRate(group.rate_multiplier) }}</span>
                    </template>
                    <template v-if="userGroupRates[group.id] != null">
                      <span class="text-primary-500">×{{ formatRate(userGroupRates[group.id]) }}</span>
                    </template>
                  </span>
                </div>
              </div>
            </div>
          </div>
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
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import modelPlazaAPI, { type ModelPlazaModel } from '@/api/modelPlaza'
import userGroupsAPI from '@/api/groups'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { platformBadgeClass, platformBorderClass, platformAccentBarClass, platformLabel } from '@/utils/platformColors'
import type { GroupPlatform } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

// ── State ──
const models = ref<ModelPlazaModel[]>([])
const userGroupRates = ref<Record<number, number>>({})
const loading = ref(false)
const searchQuery = ref('')
const selectedPlatform = ref('')

// ── Available platforms ──
const availablePlatforms = computed(() => {
  const set = new Set<string>()
  for (const m of models.value) {
    set.add(m.platform)
  }
  return Array.from(set).sort()
})

// ── Filter ──
const filteredModels = computed(() => {
  let list = models.value

  // Platform filter
  if (selectedPlatform.value) {
    list = list.filter(m => m.platform === selectedPlatform.value)
  }

  // Search filter
  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    list = list.filter(m =>
      m.name.toLowerCase().includes(q) ||
      m.platform.toLowerCase().includes(q) ||
      m.groups.some(g => g.name.toLowerCase().includes(q))
    )
  }

  return list
})

function formatRate(rate: number): string {
  if (rate === Math.floor(rate)) return String(rate)
  return rate.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
}

// ── Data loading ──
async function loadData() {
  loading.value = true
  try {
    const [resp, rates] = await Promise.all([
      modelPlazaAPI.getModelPlaza(),
      userGroupsAPI.getUserGroupRates().catch((err: unknown) => {
        console.error('Failed to load user group rates:', err)
        return {} as Record<number, number>
      }),
    ])
    models.value = resp.models || []
    userGroupRates.value = rates
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

// ── Price formatting ── (prices are per-token from LiteLLM, scale to per-1M tokens)
function formatPrice(value: number | null | undefined, scale: number = 1e6): string {
  if (value == null) return '-'
  const scaled = value * scale
  if (scaled === 0) return '$0'
  if (scaled >= 1) return `$${scaled.toFixed(2)}`
  if (scaled >= 0.01) return `$${scaled.toFixed(4)}`
  return `$${scaled.toFixed(6)}`
}

onMounted(loadData)
</script>
