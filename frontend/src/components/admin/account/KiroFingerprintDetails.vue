<template>
  <div class="space-y-5">
    <p class="text-sm text-gray-500">{{ t('admin.accounts.fingerprint.kiro.description') }}</p>
    <dl class="grid grid-cols-1 gap-4 text-sm sm:grid-cols-2">
      <div class="sm:col-span-2"><dt class="text-gray-500">Machine ID</dt><dd class="mt-1 break-all font-mono">{{ data.environment.machine_id || '—' }}</dd></div>
      <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.kiro.environment') }}</dt><dd class="mt-1">Kiro {{ data.environment.kiro_version }} · Node {{ data.environment.node_version }}<br>{{ data.environment.system_version }}</dd></div>
      <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.kiro.regions') }}</dt><dd class="mt-1">Auth: {{ data.environment.auth_region }}<br>API: {{ data.environment.api_region }}</dd></div>
      <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.kiro.proxy') }}</dt><dd class="mt-1">{{ data.proxy_id ? `#${data.proxy_id}` : '—' }} · {{ t(`admin.accounts.fingerprint.kiro.${data.proxy_ready ? 'ready' : 'unavailable'}`) }}</dd></div>
      <div><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.kiro.transport') }}</dt><dd class="mt-1">{{ data.http_protocol }} · Connection: close<br>{{ data.tls_backend }} · {{ data.go_version }}</dd></div>
      <div class="sm:col-span-2"><dt class="text-gray-500">{{ t('admin.accounts.fingerprint.kiro.digest') }}</dt><dd class="mt-1 break-all font-mono text-xs">{{ data.environment_key }}</dd></div>
    </dl>
    <p class="text-xs leading-relaxed text-gray-500">{{ t('admin.accounts.fingerprint.kiro.transportNote') }}</p>
    <details v-for="operation in data.operations" :key="operation.operation" class="border-t border-gray-200 pt-3 dark:border-dark-600">
      <summary class="cursor-pointer text-sm font-medium">{{ t(`admin.accounts.fingerprint.kiro.operations.${operation.operation}`) }}</summary>
      <p class="my-3 break-all font-mono text-xs">{{ operation.method }} {{ operation.url }}</p>
      <dl class="space-y-2 text-xs">
        <div v-for="(value, key) in operation.headers" :key="key"><dt class="text-gray-500">{{ key }}</dt><dd class="break-all font-mono">{{ value }}</dd></div>
      </dl>
    </details>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { KiroFingerprint } from '@/api/admin/accounts'

defineProps<{ data: KiroFingerprint }>()
const { t } = useI18n()
</script>
