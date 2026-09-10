<template>
  <section class="card" aria-labelledby="oauth-upstream-retry-title">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
    <h2 id="oauth-upstream-retry-title" class="text-lg font-semibold text-gray-900 dark:text-white">
      {{ t('admin.settings.oauthUpstreamRetry.title') }}
    </h2>
    </div>
    <div class="space-y-4 p-6">
    <div v-if="loading" role="status" class="text-sm text-gray-500">{{ t('common.loading') }}</div>
    <div v-else-if="loadError" role="alert" class="space-y-3">
      <p class="text-sm text-red-600">{{ loadError }}</p>
      <button type="button" class="btn btn-secondary btn-sm" @click="load">{{ t('admin.settings.oauthUpstreamRetry.reload') }}</button>
    </div>
    <fieldset v-else :disabled="saving" class="space-y-4">
      <div class="flex items-center justify-between gap-4">
        <label id="oauth-upstream-retry-enabled" class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.oauthUpstreamRetry.enabled') }}</label>
        <Toggle v-model="form.enabled" aria-labelledby="oauth-upstream-retry-enabled" />
      </div>
      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label for="oauth-upstream-retry-count" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.oauthUpstreamRetry.maxRetries') }}</label>
          <input id="oauth-upstream-retry-count" v-model.number="form.max_retries" type="number" min="0" max="10" step="1" class="input w-full" @keydown.enter.prevent="save" />
        </div>
        <div>
          <label for="oauth-upstream-retry-codes" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.oauthUpstreamRetry.statusCodes') }}</label>
          <input id="oauth-upstream-retry-codes" v-model="statusCodes" type="text" class="input w-full" placeholder="429,502,503,504" @keydown.enter.prevent="save" />
        </div>
      </div>
      <p v-if="validationError" role="alert" class="text-sm text-red-600">{{ validationError }}</p>
      <button type="button" class="btn btn-primary btn-sm" :disabled="saving" @click="save">
        {{ saving ? t('common.saving') : t('common.save') }}
      </button>
    </fieldset>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getOAuthUpstreamRetrySettings, updateOAuthUpstreamRetrySettings } from '@/api/admin/oauthUpstreamRetry'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const validationError = ref('')
const form = reactive({ enabled: false, max_retries: 3 })
const statusCodes = ref('429,502,503,504')

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const settings = await getOAuthUpstreamRetrySettings()
    Object.assign(form, { enabled: settings.enabled, max_retries: settings.max_retries })
    statusCodes.value = settings.status_codes.join(',')
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, t('admin.settings.failedToLoad'))
  } finally {
    loading.value = false
  }
}

async function save() {
  if (saving.value || loading.value || loadError.value) return
  validationError.value = ''
  if (!Number.isInteger(form.max_retries) || form.max_retries < 0 || form.max_retries > 10) {
    validationError.value = t('admin.settings.oauthUpstreamRetry.invalidRetries')
    return
  }
  const tokens = statusCodes.value.trim().split(/[,\s\uFF0C]+/)
  if (!tokens.every(token => /^[45]\d{2}$/.test(token))) {
    validationError.value = t('admin.settings.oauthUpstreamRetry.invalidCodes')
    return
  }
  saving.value = true
  try {
    const updated = await updateOAuthUpstreamRetrySettings({
      enabled: form.enabled,
      max_retries: form.max_retries,
      status_codes: [...new Set(tokens.map(Number))]
    })
    Object.assign(form, { enabled: updated.enabled, max_retries: updated.max_retries })
    statusCodes.value = updated.status_codes.join(',')
    appStore.showSuccess(t('admin.settings.settingsSaved'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.settings.failedToSave')))
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
