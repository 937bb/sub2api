<template>
 <section class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="kiro-group-cache">
  <h3 class="font-medium">{{ t('admin.accounts.kiro.cacheEnabled') }}</h3>
  <label class="flex items-center gap-2"><input v-model="policy.enabled" type="checkbox" />{{ t('admin.accounts.kiro.cacheEnabled') }}</label>
  <label v-if="policy.enabled" class="input-label">{{ t('admin.accounts.kiro.mode') }}
   <select :value="policy.mode || 'response'" class="input" data-testid="kiro-cache-mode" @change="setMode">
    <option value="response">{{ t('admin.accounts.kiro.responseMode') }}</option>
    <option value="prefix">{{ t('admin.accounts.kiro.prefixMode') }}</option>
   </select>
  </label>
  <label v-if="policy.enabled && policy.mode === 'prefix'" class="flex items-center gap-2"><input v-model="policy.auto_prefix" type="checkbox" />{{ t('admin.accounts.kiro.autoPrefix') }}</label>
  <div v-if="policy.enabled" class="grid grid-cols-1 gap-3 sm:grid-cols-3">
   <label class="input-label">{{ t(policy.mode === 'prefix' ? 'admin.accounts.kiro.prefixTarget' : 'admin.accounts.kiro.target') }}<input v-model.number="policy.target_percent" type="number" min="0" max="100" step="1" required class="input" /></label>
   <label class="input-label">{{ t('admin.accounts.kiro.ttl') }}<input v-model.number="policy.ttl_seconds" type="number" min="1" max="3600" step="1" required class="input" /></label>
   <label class="input-label">{{ t('admin.accounts.kiro.capacity') }}<input v-model.number="policy.max_entries" type="number" min="1" max="1024" step="1" required class="input" /></label>
  </div>
  <p class="input-hint">{{ t(policy.mode === 'prefix' ? 'admin.accounts.kiro.prefixHint' : 'admin.accounts.kiro.cacheHint') }}</p>
 </section>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { KiroCachePolicy } from '@/components/account/kiroCredentials'
const policy = defineModel<KiroCachePolicy>({ required: true })
const { t } = useI18n()
function setMode(event: Event) {
 const mode = (event.target as HTMLSelectElement).value === 'prefix' ? 'prefix' : 'response'
 policy.value = { ...policy.value, mode, auto_prefix: mode === 'prefix' ? (policy.value.auto_prefix ?? true) : policy.value.auto_prefix }
}
</script>
