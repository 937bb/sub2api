<template>
  <BaseDialog :show="show" :title="t('admin.groups.smartWS.title')" @close="close">
    <div class="space-y-4">
      <p class="font-medium text-gray-900 dark:text-white">{{ group?.name }}</p>
      <p class="text-sm text-gray-600 dark:text-gray-400">{{ t('admin.groups.smartWS.description') }}</p>
      <p v-if="loading" role="status">{{ t('common.loading') }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <label class="flex items-center gap-3 text-sm">
        <input v-model="enabled" type="checkbox" :disabled="loading || saving || !policy" />
        {{ t('admin.groups.smartWS.enable') }}
      </label>
      <p class="text-sm text-gray-500">{{ t('admin.groups.smartWS.hint') }}</p>
      <section v-if="policy?.runtime?.accounts" class="space-y-2" data-testid="smartws-runtime">
        <h3 class="text-sm font-medium">{{ t('admin.groups.smartWS.runtimeTitle') }}</h3>
        <p class="text-xs text-gray-500">{{ t('admin.groups.smartWS.runtimeHint') }}</p>
        <p v-if="policy.runtime.tuning_error" role="alert" class="text-sm text-amber-600">{{ t('admin.groups.smartWS.tuningError') }} {{ policy.runtime.tuning_error }}</p>
        <div class="max-h-64 overflow-auto">
          <table class="w-full whitespace-nowrap text-left text-xs">
            <thead><tr>
              <th v-for="key in runtimeColumns" :key="key" class="px-2 py-2 font-medium">{{ t(`admin.groups.smartWS.${key}`) }}</th>
            </tr></thead>
            <tbody><tr v-for="row in policy.runtime.accounts" :key="row.account_hash" class="border-t border-gray-200 dark:border-dark-600">
              <td class="px-2 py-2">{{ row.account_ids.length ? row.account_ids.map(id => `#${id}`).join(', ') : row.account_hash.slice(0, 12) }}</td>
              <td class="px-2 py-2">{{ row.active }}</td>
              <td class="px-2 py-2">{{ row.idle }}</td>
              <td class="px-2 py-2">{{ row.connecting }}</td>
              <td class="px-2 py-2">{{ row.waiting }}</td>
              <td class="px-2 py-2">{{ reuseRate(row.counters) }}</td>
              <td class="px-2 py-2">{{ row.counters.completed || 0 }}</td>
              <td class="px-2 py-2">{{ row.counters.handshake_429 || 0 }}</td>
            </tr></tbody>
          </table>
          <p v-if="!policy.runtime.accounts.length" class="py-2 text-xs text-gray-500">{{ t('admin.groups.smartWS.runtimeEmpty') }}</p>
        </div>
      </section>
    </div>
    <template #footer>
      <button class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.cancel') }}</button>
      <button class="btn btn-primary" :disabled="loading || saving || !policy" @click="save">{{ t('common.save') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { getSmartWS, setSmartWS, type SmartWSPolicy } from '@/api/admin/groups'
import type { AdminGroup } from '@/types'

const props = defineProps<{ show: boolean; group: AdminGroup | null }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const policy = ref<SmartWSPolicy | null>(null)
const enabled = ref(false)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
let generation = 0
const runtimeColumns = ['runtimeAccount', 'runtimeActive', 'runtimeIdle', 'runtimeConnecting', 'runtimeWaiting', 'runtimeReuse', 'runtimeCompleted', 'runtime429']
function reuseRate(counters: Record<string, number>): string {
  const reused = counters.reused || 0
  const total = reused + (counters.opened || 0)
  return total ? `${(reused / total * 100).toFixed(1)}%` : '—'
}

watch(() => [props.show, props.group?.id] as const, async ([show, id]) => {
  const current = ++generation
  policy.value = null
  enabled.value = false
  error.value = ''
  loading.value = false
  if (!show || !id) return
  loading.value = true
  try {
    const result = await getSmartWS(id)
    if (current !== generation) return
    policy.value = result
    enabled.value = result.enabled
  } catch {
    if (current === generation) error.value = t('admin.groups.smartWS.loadFailed')
  } finally {
    if (current === generation) loading.value = false
  }
}, { immediate: true })

function close() {
  if (!saving.value) emit('close')
}

async function save() {
  if (!props.group || !policy.value || saving.value) return
  const id = props.group.id
  const current = generation
  saving.value = true
  error.value = ''
  try {
    await setSmartWS(id, { enabled: enabled.value, revision: policy.value.revision })
    if (current === generation) emit('close')
  } catch (cause: unknown) {
    if (current === generation) {
      const status = (cause as { response?: { status?: number } })?.response?.status
      error.value = t(status === 409 ? 'admin.groups.smartWS.conflict' : 'admin.groups.smartWS.saveFailed')
      // A timeout may follow a committed write. Reload before retrying a revision.
      policy.value = null
      try {
        const latest = await getSmartWS(id)
        if (current === generation) {
          policy.value = latest
          enabled.value = latest.enabled
        }
      } catch { /* Keep saving disabled until the dialog is reopened. */ }
    }
  } finally {
    saving.value = false
  }
}
</script>
