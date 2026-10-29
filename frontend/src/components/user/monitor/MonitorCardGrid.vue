<template>
  <div>
    <div
      v-if="loading && items.length === 0"
      class="workbench-monitor-list"
    >
      <div
        v-for="i in 6"
        :key="i"
        class="p-5 rounded-xl bg-white dark:bg-dark-800 border border-gray-200 dark:border-dark-700"
      >
        <div class="flex items-start gap-3">
          <div class="skeleton-block w-9 h-9 rounded-xl"></div>
          <div class="flex-1 space-y-2">
            <div class="skeleton-block h-4 w-2/3 rounded"></div>
            <div class="skeleton-block h-3 w-1/2 rounded"></div>
          </div>
          <div class="skeleton-block h-6 w-16 rounded-full"></div>
        </div>
        <div class="mt-5 grid grid-cols-2 gap-2">
          <div class="skeleton-block h-16 rounded-xl"></div>
          <div class="skeleton-block h-16 rounded-xl"></div>
        </div>
        <div class="skeleton-block mt-6 h-5 w-full rounded"></div>
      </div>
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      :title="t('channelStatus.empty.title')"
      :description="t('channelStatus.empty.description')"
    />

    <div
      v-else
      class="workbench-monitor-list"
    >
      <MonitorCard
        v-for="item in items"
        :key="item.id"
        :item="item"
        :window="window"
        :availability-value="resolveAvailability(item)"
        :countdown-seconds="countdownSeconds"
        @click="emit('cardClick', item)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UserMonitorView, UserMonitorDetail } from '@/api/channelMonitor'
import EmptyState from '@/components/common/EmptyState.vue'
import MonitorCard from './MonitorCard.vue'

const props = defineProps<{
  items: UserMonitorView[]
  window: '7d' | '15d' | '30d'
  countdownSeconds: number
  loading: boolean
  detailCache: Record<number, UserMonitorDetail>
}>()

const emit = defineEmits<{
  (e: 'cardClick', item: UserMonitorView): void
}>()

const { t } = useI18n()

function resolveAvailability(item: UserMonitorView): number | null {
  if (props.window === '7d') {
    return item.availability_7d ?? null
  }
  const detail = props.detailCache[item.id]
  if (!detail) return null
  const primary = detail.models.find(m => m.model === item.primary_model)
  if (!primary) return null
  return props.window === '15d' ? primary.availability_15d ?? null : primary.availability_30d ?? null
}
</script>
