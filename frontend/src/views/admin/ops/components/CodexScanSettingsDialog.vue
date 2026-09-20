<template>
  <BaseDialog
    :show="show"
    :title="t('admin.ops.turnState.scanSettings.title')"
    :show-close-button="!saving"
    :close-on-escape="!saving"
    width="wide"
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
        <section class="space-y-2" aria-labelledby="codex-rule-title">
          <div class="flex items-center justify-between gap-2">
            <h3 id="codex-rule-title" class="font-medium text-gray-700 dark:text-dark-200">{{ t('admin.ops.turnState.scanSettings.rules') }}</h3>
            <button type="button" class="text-xs font-medium text-primary-600 hover:text-primary-700 disabled:opacity-50 dark:text-primary-400" :disabled="rules.length >= 128" data-test="add-state-rule" @click="addRule">{{ t('admin.ops.turnState.scanSettings.addRule') }}</button>
          </div>
          <p class="text-xs leading-relaxed text-gray-500 dark:text-dark-400">{{ t('admin.ops.turnState.scanSettings.rulesHint') }}</p>
          <p v-if="selection" class="rounded-md bg-primary-50 px-3 py-2 text-xs leading-relaxed text-primary-700 dark:bg-primary-950/40 dark:text-primary-300" data-test="rule-scope">{{ t('admin.ops.turnState.scanSettings.selectedScope', { plan: selection.plan_type === '*' ? t('common.all') : selection.plan_type.toUpperCase(), model: selection.model }) }}</p>
          <div v-if="!rules.length" class="py-2 text-xs text-gray-400">{{ t('admin.ops.turnState.scanSettings.noRules') }}</div>
          <div v-for="(rule, index) in rules" :key="rule.key" :class="['grid grid-cols-2 sm:grid-cols-[96px_minmax(0,1fr)_100px_auto] items-end gap-2 rounded-md p-2', rule.key === selectedRuleKey ? 'bg-primary-50 dark:bg-primary-950/30' : 'bg-gray-50 dark:bg-dark-800']" data-test="state-rule">
            <label class="min-w-0 text-[11px] text-gray-500 dark:text-dark-400">
              {{ t('admin.ops.turnState.scanSettings.plan') }}
              <select v-model="rule.plan_type" class="input mt-1 text-xs" :aria-label="t('admin.ops.turnState.scanSettings.plan')" data-test="rule-plan">
                <option v-for="plan in plans" :key="plan" :value="plan">{{ plan === '*' ? t('common.all') : plan.toUpperCase() }}</option>
              </select>
            </label>
            <label class="min-w-0 text-[11px] text-gray-500 dark:text-dark-400">
              {{ t('admin.ops.turnState.columns.model') }}
              <input v-model="rule.model" list="codex-state-models" class="input mt-1 font-mono text-xs" :aria-label="t('admin.ops.turnState.columns.model')" autocomplete="off" spellcheck="false" placeholder="gpt-6-astra" data-test="rule-model" />
            </label>
            <label class="min-w-0 text-[11px] text-gray-500 dark:text-dark-400">
              {{ t('admin.ops.turnState.scanSettings.ruleLengths') }}
              <input v-model="rule.lengthsText" class="input mt-1 font-mono text-xs" :aria-label="t('admin.ops.turnState.scanSettings.ruleLengths')" placeholder="292" autocomplete="off" data-test="rule-lengths" />
            </label>
            <button type="button" class="mb-2 text-xs text-red-600 hover:text-red-700 dark:text-red-400" :aria-label="t('admin.ops.turnState.scanSettings.deleteRule', { index: index + 1 })" data-test="delete-state-rule" @click="rules.splice(index, 1)">{{ t('common.delete') }}</button>
          </div>
          <datalist id="codex-state-models">
            <option v-for="model in modelSuggestions" :key="model" :value="model" />
          </datalist>
        </section>
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
import { opsAPI, type CodexTurnStateScanSettings, type CodexTurnStateLengthRule } from '@/api/admin/ops'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean; selection?: CodexTurnStateLengthRule | null }>()
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
const plans = ['*', 'pro', 'team', 'plus', 'free', 'enterprise']
const rules = ref<Array<{ key: number; plan_type: string; model: string; lengthsText: string }>>([])
const selectedRuleKey = ref<number | null>(null)
let nextRuleKey = 0
const modelSuggestions = computed(() => [...new Set(['*', 'gpt-5.6-terra', 'gpt-6-astra', ...rules.value.map(rule => rule.model.trim().toLowerCase()).filter(Boolean)])])

function parseLengths(value: string) { return value.trim().split(/[\s,，]+/).filter(Boolean).map(Number) }
function validLengths(lengths: number[]) { return lengths.length >= 1 && lengths.length <= 16 && lengths.every(value => Number.isInteger(value) && value >= 64 && value <= 4096) && new Set(lengths).size === lengths.length }
const targetLengths = computed(() => parseLengths(lengthsText.value))
const normalizedRules = computed<CodexTurnStateLengthRule[]>(() => rules.value.map(rule => ({ plan_type: rule.plan_type, model: rule.model.trim().toLowerCase(), target_lengths: parseLengths(rule.lengthsText) })))
const validationError = computed(() => {
  if (!validLengths(targetLengths.value)) {
    return t('admin.ops.turnState.scanSettings.invalidLengths')
  }
  if (rules.value.length > 128) return t('admin.ops.turnState.scanSettings.tooManyRules')
  const seen = new Set<string>()
  for (const rule of normalizedRules.value) {
    if (!plans.includes(rule.plan_type) || !rule.model || /\s/.test(rule.model) || rule.model.length > 200 || (rule.model.includes('*') && rule.model !== '*')) return t('admin.ops.turnState.scanSettings.invalidRuleModel')
    if (!validLengths(rule.target_lengths)) return t('admin.ops.turnState.scanSettings.invalidRuleLengths')
    const key = `${rule.plan_type}/${rule.model}`
    if (seen.has(key)) return t('admin.ops.turnState.scanSettings.duplicateRule')
    seen.add(key)
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
  rules.value = (settings.rules ?? []).map(rule => ({ key: nextRuleKey++, plan_type: rule.plan_type, model: rule.model, lengthsText: rule.target_lengths.join(', ') }))
}

function addRule() {
  if (rules.value.length >= 128) return
  rules.value.push({ key: nextRuleKey++, plan_type: '*', model: '', lengthsText: targetLengths.value.join(', ') })
}

function selectRule() {
  selectedRuleKey.value = null
  if (!props.selection) return
  const plan = plans.includes(props.selection.plan_type) ? props.selection.plan_type : '*'
  const model = props.selection.model.trim().toLowerCase()
  let rule = rules.value.find(rule => rule.plan_type === plan && rule.model.trim().toLowerCase() === model)
  if (!rule) {
    rule = { key: nextRuleKey++, plan_type: plan, model, lengthsText: props.selection.target_lengths.join(', ') }
    rules.value.unshift(rule)
  }
  selectedRuleKey.value = rule.key
}

async function load() {
  if (loading.value || saving.value) return
  loading.value = true
  loaded.value = false
  loadError.value = false
  try {
    const settings = await opsAPI.getCodexTurnStateScanSettings()
    applySettings(settings)
    selectRule()
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
      rules: normalizedRules.value,
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
