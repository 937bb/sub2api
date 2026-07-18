<template>
  <AppLayout>
    <div class="mx-auto max-w-[1400px] space-y-5">
      <!-- Summary strip -->
      <div class="card grid grid-cols-3 divide-x divide-gray-200/70 dark:divide-dark-700">
        <div class="px-5 py-3">
          <p class="font-display text-2xl font-bold tracking-tight text-gray-900 dark:text-white">{{ summary.channels }}</p>
          <p class="mt-0.5 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('availableChannels.summaryChannels') }}</p>
        </div>
        <div class="px-5 py-3">
          <p class="font-display text-2xl font-bold tracking-tight text-gray-900 dark:text-white">{{ summary.platforms }}</p>
          <p class="mt-0.5 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('availableChannels.summaryPlatforms') }}</p>
        </div>
        <div class="px-5 py-3">
          <p class="font-display text-2xl font-bold tracking-tight text-gray-900 dark:text-white">{{ summary.models }}</p>
          <p class="mt-0.5 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('availableChannels.summaryModels') }}</p>
        </div>
      </div>

      <!-- Toolbar: channel search + refresh -->
      <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
        <div class="flex flex-1 flex-wrap items-center gap-3">
          <SearchInput
            v-model="searchQuery"
            :placeholder="t('availableChannels.searchPlaceholder')"
            class="w-full sm:w-80"
          />
        </div>
        <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
          <button
            @click="loadChannels"
            :disabled="loading"
            class="btn btn-secondary"
            :title="t('common.refresh', 'Refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>

      <!-- Channel cards with inline model pricing -->
      <AvailableChannelsTable
        :columns="columnLabels"
        :rows="filteredChannels"
        :loading="loading"
        :user-group-rates="userGroupRates"
        pricing-key-prefix="availableChannels.pricing"
        :no-pricing-label="t('availableChannels.noPricing')"
        :no-models-label="t('availableChannels.noModels')"
        :empty-label="t('availableChannels.empty')"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import AvailableChannelsTable from '@/components/channels/AvailableChannelsTable.vue'
import userChannelsAPI, { type UserAvailableChannel } from '@/api/channels'
import userGroupsAPI from '@/api/groups'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const channels = ref<UserAvailableChannel[]>([])
const userGroupRates = ref<Record<number, number>>({})
const loading = ref(false)
const searchQuery = ref('')

const columnLabels = computed(() => ({
  name: t('availableChannels.columns.name'),
  description: t('availableChannels.columns.description'),
  platform: t('availableChannels.columns.platform'),
  groups: t('availableChannels.columns.groups'),
  supportedModels: t('availableChannels.columns.supportedModels'),
}))

// Summary counts across all channels: total channels, distinct platforms,
// distinct supported models.
const summary = computed(() => {
  const platforms = new Set<string>()
  const models = new Set<string>()
  for (const ch of channels.value) {
    for (const p of ch.platforms) {
      platforms.add(p.platform)
      for (const m of p.supported_models) models.add(m.name)
    }
  }
  return { channels: channels.value.length, platforms: platforms.size, models: models.size }
})

/**
 * 搜索过滤：
 * - 命中渠道名/描述 → 整个渠道（所有 platforms）都保留
 * - 否则按 platform/group/model 维度在 sections 里过滤，保留有匹配的 section
 * - 所有 sections 都不匹配时，渠道本身被过滤掉
 */
const filteredChannels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return channels.value
  return channels.value
    .map((ch) => {
      const nameHit = ch.name.toLowerCase().includes(q)
      const descHit = (ch.description || '').toLowerCase().includes(q)
      if (nameHit || descHit) return ch
      const matchingSections = ch.platforms.filter(
        (p) =>
          p.platform.toLowerCase().includes(q) ||
          p.groups.some((g) => g.name.toLowerCase().includes(q)) ||
          p.supported_models.some((m) => m.name.toLowerCase().includes(q)),
      )
      if (matchingSections.length === 0) return null
      return { ...ch, platforms: matchingSections }
    })
    .filter((ch): ch is UserAvailableChannel => ch !== null)
})

async function loadChannels() {
  loading.value = true
  try {
    // 渠道列表和用户专属倍率并发拉取。专属倍率失败不阻塞渠道展示——
    // 失败时只是无法渲染专属倍率角标，降级为仅显示默认倍率。
    const [list, rates] = await Promise.all([
      userChannelsAPI.getAvailable(),
      userGroupsAPI.getUserGroupRates().catch((err: unknown) => {
        console.error('Failed to load user group rates:', err)
        return {} as Record<number, number>
      }),
    ])
    channels.value = list
    userGroupRates.value = rates
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(loadChannels)
</script>
