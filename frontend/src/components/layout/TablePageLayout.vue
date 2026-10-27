<template>
  <section class="table-page-layout" :class="{ 'mobile-mode': isMobile, 'console-density-compact': compact }">
    <header class="console-data-heading layout-section-fixed">
      <div class="console-collection-title">
        <div class="console-title-line"><h2>{{ pageTitle }}</h2><span v-if="total !== undefined" class="console-record-count" :aria-busy="loading">{{ loading ? '—' : total.toLocaleString() }}</span></div>
        <p v-if="pageDescription">{{ pageDescription }}</p>
      </div>
      <div class="console-data-actions"><slot name="actions" /></div>
    </header>
    <slot name="tabs" />
    <div class="workbench-collection-body">
      <section v-if="$slots.filters" class="workbench-query-dock" :aria-label="t('console.resetFilters')">
        <div class="console-query-heading">
          <span><Icon name="filter" size="sm" />{{ t('console.resetFilters') }}</span>
          <button type="button" class="console-text-button" :aria-expanded="filtersOpen" :aria-controls="filterId" @click="filtersOpen = !filtersOpen">{{ t(filtersOpen ? 'console.hideFilters' : 'common.expand') }}<Icon :name="filtersOpen ? 'chevronUp' : 'chevronDown'" size="xs" /></button>
        </div>
        <div v-show="filtersOpen" :id="filterId" class="console-filter-bar"><slot name="filters" /></div>
      </section>
      <div class="console-table-controls layout-section-fixed">
        <div class="workbench-collection-summary" :aria-busy="loading">
          <span>{{ t('console.details') }}<strong v-if="rows !== undefined">{{ loading ? '—' : rows.length.toLocaleString() }}</strong></span>
          <span v-if="statusCounts.length" class="workbench-status-counts" :title="t('console.currentPageOnly')">
            <span v-for="entry in statusCounts" :key="entry.status" class="console-status-count" :data-status="entry.status">{{ entry.label }} <strong>{{ entry.count }}</strong></span>
          </span>
        </div>
        <div class="console-density-switch" role="group" :aria-label="t('console.tableDensity')">
          <button type="button" :aria-pressed="compact" @click="compact = true">{{ t('console.compact') }}</button>
          <button type="button" :aria-pressed="!compact" @click="compact = false">{{ t('console.comfortable') }}</button>
        </div>
      </div>
      <div class="workbench-collection-results">
        <div class="layout-section-scrollable">
          <div class="table-scroll-container"><slot name="table" /></div>
        </div>
        <footer v-if="$slots.pagination" class="layout-section-fixed console-table-pagination"><slot name="pagination" /></footer>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, useId } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { resolveRouteMetaKeys } from '@/router/title'
import { useLocalStorage } from '@vueuse/core'
import Icon from '@/components/icons/Icon.vue'
const props = defineProps<{ total?: number; rows?: Record<string, any>[]; loading?: boolean }>()
const { t } = useI18n()
const route = useRoute()
const compact = useLocalStorage('b-console-table-compact', true)
const filtersOpen = ref(true)
const filterId = useId()
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
  min-width: 0;
}

.layout-section-fixed {
  @apply flex-shrink-0;
}

.layout-section-scrollable {
  @apply flex-1 min-h-0 flex flex-col;
  min-height: 0;
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
