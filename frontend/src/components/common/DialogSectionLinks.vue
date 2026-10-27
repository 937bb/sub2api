<template>
  <nav class="console-detail-tabs" :aria-label="t('console.configDirectory')">
    <button v-for="(section, index) in sections" :key="section" type="button" @click="goToSection($event, index)">{{ t(section) }}</button>
  </nav>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ sections: string[] }>()
const { t } = useI18n()
function goToSection(event: Event, index: number) {
  const panel = (event.currentTarget as HTMLElement).closest('.console-dialog')
  const section = panel?.querySelectorAll<HTMLElement>('[data-dialog-section]')[index]
  section?.scrollIntoView({ block: 'start', behavior: 'auto' })
}
</script>
