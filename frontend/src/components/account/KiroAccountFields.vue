<template>
  <section class="space-y-4" data-testid="kiro-account-fields">
    <label class="input-label">{{ t('admin.accounts.kiro.credentials') }}</label>
    <textarea v-model="json" class="input font-mono text-xs" rows="6" spellcheck="false" autocomplete="off" :placeholder="editing ? t('admin.accounts.kiro.keepCredentials') : t('admin.accounts.kiro.importHint')" />
    <input type="file" accept="application/json,.json" @change="loadFile" />
    <p class="input-hint">{{ t('admin.accounts.kiro.importHint') }}</p>
    <p class="input-hint">{{ t('admin.accounts.kiro.proxyHint') }}</p>
    <label class="flex items-center gap-2"><input v-model="policy.enabled" type="checkbox" />{{ t('admin.accounts.kiro.cacheEnabled') }}</label>
    <div v-if="policy.enabled" class="grid grid-cols-1 gap-3 sm:grid-cols-3">
      <label class="input-label">{{ t('admin.accounts.kiro.target') }}<input v-model.number="policy.target_percent" type="number" min="0" max="100" step="1" class="input" /></label>
      <label class="input-label">{{ t('admin.accounts.kiro.ttl') }}<input v-model.number="policy.ttl_seconds" type="number" min="1" max="3600" step="1" class="input" /></label>
      <label class="input-label">{{ t('admin.accounts.kiro.capacity') }}<input v-model.number="policy.max_entries" type="number" min="1" max="1024" step="1" class="input" /></label>
    </div>
    <p class="input-hint">{{ t('admin.accounts.kiro.cacheHint') }}</p>
  </section>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { KiroCachePolicy } from './kiroCredentials'
defineProps<{ editing?: boolean }>()
const json = defineModel<string>('json', { required: true })
const policy = defineModel<KiroCachePolicy>('policy', { required: true })
const { t } = useI18n()
async function loadFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  try { if (file) json.value = await file.text() } finally { input.value = '' }
}
</script>
