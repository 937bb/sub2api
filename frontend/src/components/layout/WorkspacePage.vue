<template>
  <section class="workbench-page" :class="`workbench-${variant}`">
    <header v-if="!hideHeading" class="workbench-heading">
      <div><h2>{{ title || routeTitle }}</h2><p v-if="description || routeDescription">{{ description || routeDescription }}</p></div>
      <div v-if="$slots.actions" class="workbench-actions"><slot name="actions" /></div>
    </header>
    <slot />
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
withDefaults(defineProps<{ title?: string; description?: string; variant?: string; hideHeading?: boolean }>(), { variant: 'document' })
const route = useRoute()
const { t } = useI18n()
const routeTitle = computed(() => route?.meta?.titleKey ? t(String(route.meta.titleKey)) : String(route?.meta?.title || ''))
const routeDescription = computed(() => route?.meta?.descriptionKey ? t(String(route.meta.descriptionKey)) : '')
</script>
