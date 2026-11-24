<template>
  <div
    v-if="contact"
    class="rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-300"
  >
    <p>{{ t('auth.emailNotReceivedContactSupport') }}</p>
    <a
      v-if="contactUrl"
      :href="contactUrl"
      target="_blank"
      rel="noopener noreferrer"
      class="mt-1 block break-words font-medium underline underline-offset-2"
    >{{ contact }}</a>
    <p v-else class="mt-1 whitespace-pre-wrap break-words font-medium">{{ contact }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ contactInfo: string }>()
const { t } = useI18n()
const contact = computed(() => props.contactInfo.trim())
const contactUrl = computed(() => {
  try {
    const url = new URL(contact.value)
    return ['https:', 'http:'].includes(url.protocol) ? url.href : ''
  } catch {
    return ''
  }
})
</script>

