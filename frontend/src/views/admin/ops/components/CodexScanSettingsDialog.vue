<template>
  <BaseDialog
    :show="show"
    :title="t('admin.ops.turnState.scanSettings.title')"
    :show-close-button="!saving"
    :close-on-escape="!saving"
    @close="close"
  >
    <form id="codex-scan-settings" class="space-y-4 text-sm" @submit.prevent="save">
      <p class="text-xs leading-relaxed text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.scanSettings.scope') }}</p>
      <div v-if="loadError" role="alert" class="flex items-center justify-between gap-3 text-xs text-red-600 dark:text-red-400">
        <span>{{ t('admin.ops.turnState.scanSettings.loadFailed') }}</span>
        <button type="button" class="btn btn-secondary shrink-0" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
      </div>
      <fieldset :disabled="loading || saving || !loaded" class="space-y-4 disabled:opacity-60">
        <div>
          <label for="codex-scan-lengths" class="mb-1 block font-medium text-gray-700 dark:text-dark-200">{{ t('admin.ops.turnState.scanSettings.lengths') }}</label>
          <input id="codex-scan-lengths" v-model="lengthsText" class="input font-mono text-sm" autocomplete="off" placeholder="332, 292" aria-describedby="codex-scan-lengths-help" />
          <p id="codex-scan-lengths-help" class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.scanSettings.lengthsHint') }}</p>
        </div>
        <div>
          <label for="codex-scan-parallel" class="mb-1 block font-medium text-gray-700 dark:text-dark-200">{{ t('admin.ops.turnState.scanSettings.parallel') }}</label>
          <input id="codex-scan-parallel" v-model.number="parallelProbes" type="number" min="1" max="5" step="1" class="input w-24 text-sm" />
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.scanSettings.parallelHint') }}</p>
        </div>
        <label class="flex items-center gap-2 font-medium text-gray-700 dark:text-dark-200">
          <input v-model="dynamicProxyEnabled" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('admin.ops.turnState.scanSettings.dynamicEnabled') }}
        </label>
        <div v-if="dynamicProxyEnabled">
          <label for="codex-scan-provider" class="mb-1 block font-medium text-gray-700 dark:text-dark-200">{{ t('admin.ops.turnState.scanSettings.providerUrl') }}</label>
          <input id="codex-scan-provider" v-model="dynamicProxyUrl" type="url" class="input font-mono text-xs" autocomplete="off" spellcheck="false" />
          <p class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.scanSettings.providerHint') }}</p>
        </div>
      </fieldset>
      <p v-if="validationError && loaded && !loading" role="alert" class="text-xs text-red-600 dark:text-red-400">{{ validationError }}</p>
    </form>
    <template #footer>
      <button type="button" class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.cancel') }}</button>
      <button type="submit" form="codex-scan-settings" class="btn btn-primary" :disabled="loading || saving || !loaded || !!validationError">
        {{ loading ? t('common.loading') : saving ? t('common.saving') : t('common.save') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { opsAPI, type CodexTurnStateScanSettings } from '@/api/admin/ops'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: []; saved: [settings: CodexTurnStateScanSettings] }>()
const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const loaded = ref(false)
const loadError = ref(false)
const saving = ref(false)
const lengthsText = ref('')
const parallelProbes = ref<number | string>(5)
const dynamicProxyEnabled = ref(false)
const dynamicProxyUrl = ref('')

const targetLengths = computed(() => lengthsText.value.trim().split(/[\s,，]+/).filter(Boolean).map(Number))
const validationError = computed(() => {
  const lengths = targetLengths.value
  if (lengths.length < 1 || lengths.length > 16 || lengths.some(value => !Number.isInteger(value) || value < 64 || value > 4096) || new Set(lengths).size !== lengths.length) {
    return t('admin.ops.turnState.scanSettings.invalidLengths')
  }
  if (!Number.isInteger(parallelProbes.value) || Number(parallelProbes.value) < 1 || Number(parallelProbes.value) > 5) {
    return t('admin.ops.turnState.scanSettings.invalidParallel')
  }
  if (dynamicProxyEnabled.value || dynamicProxyUrl.value.trim()) {
    try {
      const value = dynamicProxyUrl.value.trim()
      const url = new URL(value)
      if (value.length > 2048 || url.protocol !== 'https:' || url.username || url.password || value.includes('#')) return t('admin.ops.turnState.scanSettings.invalidUrl')
    } catch { return t('admin.ops.turnState.scanSettings.invalidUrl') }
  }
  return ''
})

function applySettings(settings: CodexTurnStateScanSettings) {
  lengthsText.value = settings.target_lengths.join(', ')
  parallelProbes.value = settings.parallel_probes
  dynamicProxyEnabled.value = settings.dynamic_proxy_enabled
  dynamicProxyUrl.value = settings.dynamic_proxy_url
}

async function load() {
  if (loading.value || saving.value) return
  loading.value = true
  loaded.value = false
  loadError.value = false
  try {
    const settings = await opsAPI.getCodexTurnStateScanSettings()
    applySettings(settings)
    loaded.value = true
  } catch {
    loadError.value = true
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!loaded.value || loading.value || saving.value || validationError.value) return
  saving.value = true
  try {
    const settings = await opsAPI.updateCodexTurnStateScanSettings({
      target_lengths: [...targetLengths.value],
      parallel_probes: Number(parallelProbes.value),
      dynamic_proxy_enabled: dynamicProxyEnabled.value,
      dynamic_proxy_url: dynamicProxyUrl.value.trim()
    })
    applySettings(settings)
    appStore.showSuccess(t('admin.ops.turnState.scanSettings.saved'))
    emit('saved', settings)
    emit('close')
  } catch {
    appStore.showError(t('admin.ops.turnState.scanSettings.saveFailed'))
  } finally {
    saving.value = false
  }
}

function close() { if (!saving.value) emit('close') }

watch(() => props.show, show => { if (show) void load() }, { immediate: true })
</script>
