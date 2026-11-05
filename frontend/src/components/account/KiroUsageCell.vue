<template>
  <div class="space-y-1" data-testid="kiro-usage">
    <span v-if="loading && !usage" class="text-xs text-gray-400">{{ t('common.loading') }}</span>
    <span v-else-if="error || usage?.error" class="block max-w-[220px] truncate text-xs text-amber-600" :title="error || usage?.error || ''">{{ error || usage?.error }}</span>
    <template v-else-if="quota">
      <div class="text-[10px] text-gray-500">{{ quota.plan || 'Kiro' }}</div>
      <UsageProgressBar v-if="quota.utilization != null" :label="t('admin.accounts.kiro.quota')" :utilization="quota.utilization" :resets-at="quota.resets_at" color="purple" />
      <span v-else class="text-xs text-gray-400">{{ t('admin.accounts.kiro.quotaUnknown') }}</span>
      <div class="text-[10px] text-gray-500">{{ format(quota.used) }} / {{ format(quota.limit) }}</div>
    </template>
    <span v-else class="text-xs text-gray-400">{{ t('admin.accounts.kiro.quotaUnknown') }}</span>
    <button type="button" class="block text-[10px] text-blue-600 disabled:opacity-50" :disabled="loading" @click="$emit('refresh')">{{ t('admin.accounts.usageWindow.activeQuery') }}</button>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountUsageInfo } from '@/types'
import UsageProgressBar from './UsageProgressBar.vue'
const props = defineProps<{ usage: AccountUsageInfo | null; loading: boolean; error: string | null }>()
defineEmits<{ refresh: [] }>()
const { t } = useI18n()
const quota = computed(() => props.usage?.kiro_quota)
const format = (value: number) => new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(value)
</script>
