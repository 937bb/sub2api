<template>
  <section class="space-y-4" data-testid="kiro-account-fields">
    <label class="input-label">{{ t('admin.accounts.kiro.credentials') }}</label>
    <textarea v-model="json" class="input font-mono text-xs" rows="6" spellcheck="false" autocomplete="off" :placeholder="editing ? t('admin.accounts.kiro.keepCredentials') : t('admin.accounts.kiro.importHint')" />
    <input type="file" accept="application/json,.json" @change="loadFile" />
    <p class="input-hint">{{ t('admin.accounts.kiro.importHint') }}</p>
    <p class="input-hint">{{ t('admin.accounts.kiro.proxyHint') }}</p>
  </section>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ editing?: boolean }>()
const json = defineModel<string>('json', { required: true })
const { t } = useI18n()
async function loadFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  try { if (file) json.value = await file.text() } finally { input.value = '' }
}
</script>
