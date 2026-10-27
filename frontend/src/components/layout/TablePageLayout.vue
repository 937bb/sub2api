<template>
  <div class="table-page-layout" :class="{ 'mobile-mode': isMobile, 'console-density-compact': compact }">
    <div class="console-data-heading layout-section-fixed">
      <div><h2>{{ pageTitle }}</h2><p v-if="pageDescription">{{ pageDescription }}</p></div>
      <div class="console-data-actions"><slot name="actions" /></div>
    </div>

    <div v-if="total !== undefined || rows !== undefined" class="workbench-collection-summary" :aria-busy="loading">
      <span v-if="total !== undefined">{{ t('console.matchingRecords') }} <strong>{{ loading ? '—' : total.toLocaleString() }}</strong></span>
      <span v-if="rows !== undefined">{{ t('console.visibleRecords') }} <strong>{{ loading ? '—' : rows.length.toLocaleString() }}</strong></span>
      <span v-if="statusCounts.length" class="workbench-status-counts">
        <span v-for="entry in statusCounts" :key="entry.status">{{ entry.label }} <strong>{{ entry.count }}</strong></span>
        <small>{{ t('console.currentPageOnly') }}</small>
      </span>
    </div>
    <div class="workbench-collection-body" :class="{ 'query-open': filtersOpen && $slots.filters }">
      <aside v-if="$slots.filters" v-show="filtersOpen" class="workbench-query-dock" :aria-label="t('console.resetFilters')">
        <div class="workbench-query-title"><h3>{{ t('console.resetFilters') }}</h3><button type="button" :aria-label="t('console.hideFilters')" @click="filtersOpen = false"><Icon name="x" size="sm" /></button></div>
        <div class="console-filter-bar"><slot name="filters" /></div>
      </aside>
      <div class="workbench-collection-results">
    <div class="console-table-controls layout-section-fixed">
      <div class="flex items-center gap-3"><button v-if="$slots.filters" type="button" class="workbench-filter-toggle" :aria-expanded="filtersOpen" @click="filtersOpen = !filtersOpen"><Icon name="filter" size="sm" />{{ t('console.resetFilters') }}</button><span>{{ t('console.details') }}</span></div>
      <div class="console-density-switch" role="group" :aria-label="t('console.tableDensity')">
        <button type="button" :aria-pressed="compact" @click="compact = true">{{ t('console.compact') }}</button>
        <button type="button" :aria-pressed="!compact" @click="compact = false">{{ t('console.comfortable') }}</button>
      </div>
    </div>
    <!-- Scrollable table -->
    <div class="layout-section-scrollable">
      <div class="card table-scroll-container">
        <slot name="table" />
      </div>
    </div>

    <!-- Fixed pagination -->
    <div v-if="$slots.pagination" class="layout-section-fixed">
      <slot name="pagination" />
    </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { resolveRouteMetaKeys } from '@/router/title'
import { useLocalStorage } from '@vueuse/core'
import Icon from '@/components/icons/Icon.vue'
const props = defineProps<{ total?: number; rows?: Record<string, any>[]; loading?: boolean }>()
const { t } = useI18n()
const route = useRoute()
const compact = useLocalStorage('b-console-table-compact', true)
const filtersOpen = ref(false)
const statusCounts = computed(() => {
  const labels: Record<string, string> = { active: t('common.active'), inactive: t('common.inactive'), error: t('common.error'), disabled: t('common.disabled') }
  return Object.entries(labels).map(([status, label]) => ({ status, label, count: props.rows?.filter(row => row.status === status).length || 0 })).filter(entry => entry.count > 0)
})
const pageTitle = computed(() => route.meta.titleKey ? t(String(route.meta.titleKey)) : String(route.meta.title || ''))
const pageDescription = computed(() => { const keys = resolveRouteMetaKeys(route); return keys.descriptionKey ? t(keys.descriptionKey) : '' })

const isMobile = ref(false)

const checkMobile = () => {
  isMobile.value = window.innerWidth < 1024
}

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<style scoped>
/* Desktop table workspace. */
.table-page-layout {
  @apply flex flex-col gap-0;
  min-height: calc(100dvh - 64px - 4rem);
}

.layout-section-fixed {
  @apply flex-shrink-0;
}

.layout-section-scrollable {
  @apply flex-1 min-h-0 flex flex-col;
  min-height: 320px;
}

/* Preserve both scrolling axes. */
.table-scroll-container {
  @apply flex flex-col overflow-hidden h-full bg-white dark:bg-dark-800 rounded-xl border border-gray-200 dark:border-dark-700;
}

.table-scroll-container :deep(.table-wrapper) {
  @apply flex-1 overflow-x-auto overflow-y-auto;
  /* Reserve scrollbar space to avoid column movement. */
  scrollbar-gutter: stable;
}

.table-scroll-container :deep(table) {
  @apply w-full;
  min-width: max-content;
  display: table; /* Preserve sticky column behavior. */
}

.table-scroll-container :deep(thead) {
  @apply bg-gray-50 dark:bg-dark-800;
}

.table-scroll-container :deep(tbody) {
  /* Retain the native table-row-group display. */
}

.table-scroll-container :deep(th) {
  @apply px-4 py-3 text-left text-xs font-semibold text-gray-600 dark:text-dark-300 border-b border-gray-200 dark:border-dark-700;
}

.table-scroll-container :deep(td) {
  @apply px-4 py-3 text-sm text-gray-700 dark:text-gray-300 border-b border-gray-100 dark:border-dark-800;
  font-variant-numeric: tabular-nums;
}

/* Mobile pages use document scrolling. */
.table-page-layout.mobile-mode {
  height: auto;
}

.table-page-layout.mobile-mode .table-scroll-container {
  @apply h-auto overflow-visible border-none shadow-none bg-transparent;
}

.table-page-layout.mobile-mode .layout-section-scrollable {
  @apply flex-none min-h-fit;
}

.table-page-layout.mobile-mode .table-scroll-container :deep(table) {
  @apply flex-none;
  display: table;
  min-width: 100%;
}
</style>
